package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

const (
	defaultGroupName = "everyone"

	groupsRoleAdmin   = "admin"
	groupsRoleStudent = "student"
)

// GroupRow is a single row of the admin group listing, joined with its
// member count and any mapped platform role.
type GroupRow struct {
	ID          string
	Name        string
	Source      string
	CreatedAt   string
	MemberCount int64
	MappedRole  string
}

// GroupMapping is a single groupName -> platformRole mapping row.
type GroupMapping struct {
	GroupName    string
	PlatformRole string
}

// GroupEnrollmentRow is a single row of the admin group-enrollment listing
// for a course, joined with the group's member count.
type GroupEnrollmentRow struct {
	ID          string
	Name        string
	Source      string
	MemberCount int64
	EnrolledAt  string
}

// GroupRepository is the persistence boundary for groups, group membership,
// and IdP group/role sync.
type GroupRepository interface {
	// Admin CRUD (groups.go handlers)
	Create(ctx context.Context, name string) (string, error)
	Delete(ctx context.Context, id string) (bool, error)
	List(ctx context.Context) ([]GroupRow, error)
	ListMappings(ctx context.Context) ([]GroupMapping, error)
	UpsertMapping(ctx context.Context, groupName, platformRole string) error
	DeleteMapping(ctx context.Context, groupName string) (bool, error)

	// Login-time sync (auth.go/oauth.go)
	AddToDefault(ctx context.Context, userID string) error
	SyncEnrollments(ctx context.Context, userID string) error
	SyncGroupsAndDeriveRole(ctx context.Context, userID string, groupNames []string, source string) (string, error)

	// Admin course-enrollment cascade (admin.go)
	EnrollCourse(ctx context.Context, groupID, courseSlug string) (int64, error)
	ListCourseEnrollments(ctx context.Context, courseSlug string) ([]GroupEnrollmentRow, error)
	UnenrollCourse(ctx context.Context, groupID, courseSlug string) error
}

type gormGroupRepository struct {
	db *gorm.DB
}

// NewGormGroupRepository builds a GroupRepository backed by db.
func NewGormGroupRepository(db *gorm.DB) GroupRepository {
	return &gormGroupRepository{db: db}
}

func (r *gormGroupRepository) Create(ctx context.Context, name string) (string, error) {
	var id string

	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO groups (name, source) VALUES (?, 'local') ON CONFLICT (name) DO NOTHING RETURNING id::text`,
		name).Scan(&id).Error
	if err != nil {
		return "", fmt.Errorf("create group: %w", err)
	}

	return id, nil
}

func (r *gormGroupRepository) Delete(ctx context.Context, id string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`DELETE FROM groups WHERE id = ?::uuid AND source = 'local'`, id)
	if result.Error != nil {
		return false, fmt.Errorf("delete group: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (r *gormGroupRepository) List(ctx context.Context) ([]GroupRow, error) {
	var rows []GroupRow

	err := r.db.WithContext(ctx).Raw(`
		SELECT g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
		       COUNT(ug.userid) AS member_count,
		       COALESCE(grm.platformrole, '') AS mapped_role
		FROM groups g
		LEFT JOIN user_groups ug ON ug.groupid = g.id
		LEFT JOIN group_role_mappings grm ON grm.groupname = g.name
		GROUP BY g.id, g.name, g.source, g.createdat, grm.platformrole
		ORDER BY g.name`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	return rows, nil
}

func (r *gormGroupRepository) ListMappings(ctx context.Context) ([]GroupMapping, error) {
	var rows []GroupMapping

	err := r.db.WithContext(ctx).Raw(
		`SELECT groupname AS group_name, platformrole AS platform_role FROM group_role_mappings ORDER BY groupname`,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list group mappings: %w", err)
	}

	return rows, nil
}

func (r *gormGroupRepository) UpsertMapping(ctx context.Context, groupName, platformRole string) error {
	err := r.db.WithContext(ctx).Exec(
		`INSERT INTO group_role_mappings (groupname, platformrole) VALUES (?, ?)
		 ON CONFLICT (groupname) DO UPDATE SET platformrole = ?`,
		groupName, platformRole, platformRole).Error
	if err != nil {
		return fmt.Errorf("upsert group mapping: %w", err)
	}

	return nil
}

func (r *gormGroupRepository) DeleteMapping(ctx context.Context, groupName string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`DELETE FROM group_role_mappings WHERE groupname = ?`, groupName)
	if result.Error != nil {
		return false, fmt.Errorf("delete group mapping: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (r *gormGroupRepository) AddToDefault(ctx context.Context, userID string) error {
	var groupID string

	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO groups (name, source, description) VALUES (?, 'local', 'Default group — all users are members automatically')
		 ON CONFLICT (name) DO UPDATE SET updatedat = NOW() RETURNING id::text`,
		defaultGroupName).Scan(&groupID).Error
	if err != nil {
		return fmt.Errorf("upsert default group: %w", err)
	}

	if groupID == "" {
		return nil
	}

	err = r.db.WithContext(ctx).Exec(
		`INSERT INTO user_groups (userid, groupid) VALUES (?::uuid, ?::uuid) ON CONFLICT DO NOTHING`,
		userID, groupID).Error
	if err != nil {
		return fmt.Errorf("link user to default group: %w", err)
	}

	return nil
}

func (r *gormGroupRepository) SyncEnrollments(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Exec(
		`INSERT INTO enrollments (userid, courseslug)
		 SELECT ?::uuid, ge.courseslug
		 FROM group_enrollments ge
		 JOIN user_groups ug ON ug.groupid = ge.groupid
		 WHERE ug.userid = ?::uuid
		 ON CONFLICT DO NOTHING`,
		userID, userID).Error
	if err != nil {
		return fmt.Errorf("sync group enrollments: %w", err)
	}

	return nil
}

// SyncGroupsAndDeriveRole upserts groups from an IdP into the groups table,
// replaces the user's group memberships, and returns the highest platform
// role found in group_role_mappings ('admin' beats 'student'). Defaults to
// the user's current role, falling back to 'student' if unknown. The whole
// operation runs in a transaction so a failure partway through leaves the
// user's prior groups/role untouched instead of landing in a broken
// partial-sync state.
func (r *gormGroupRepository) SyncGroupsAndDeriveRole(
	ctx context.Context, userID string, groupNames []string, source string,
) (string, error) {
	var dbRole string

	role := groupsRoleStudent

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		selErr := tx.Raw(`SELECT role FROM users WHERE id = ?::uuid`, userID).Scan(&dbRole).Error
		if selErr == nil && dbRole != "" {
			role = dbRole
		}

		delErr := tx.Exec(`DELETE FROM user_groups WHERE userid = ?::uuid`, userID).Error
		if delErr != nil {
			return fmt.Errorf("clear group memberships: %w", delErr)
		}

		roleMapped := false

		for _, name := range groupNames {
			if name == "" {
				continue
			}

			updatedRole, mapped, membershipErr := syncGroupMembership(tx, userID, name, source, role)
			if membershipErr != nil {
				return membershipErr
			}

			role = updatedRole
			if mapped {
				roleMapped = true
			}
		}

		if roleMapped {
			updErr := tx.Exec(`UPDATE users SET role = ?, updatedat = NOW() WHERE id = ?::uuid`, role, userID).Error
			if updErr != nil {
				return fmt.Errorf("persist derived role: %w", updErr)
			}
		}

		return nil
	})
	if err != nil {
		return role, err
	}

	return role, nil
}

func (r *gormGroupRepository) EnrollCourse(ctx context.Context, groupID, courseSlug string) (int64, error) {
	err := r.db.WithContext(ctx).Exec(
		`INSERT INTO group_enrollments (groupid, courseslug)
		 VALUES (?::uuid, ?)
		 ON CONFLICT DO NOTHING`,
		groupID, courseSlug).Error
	if err != nil {
		return 0, fmt.Errorf("enroll group in course: %w", err)
	}

	result := r.db.WithContext(ctx).Exec(
		`INSERT INTO enrollments (userid, courseslug)
		 SELECT ug.userid, ?
		 FROM user_groups ug
		 WHERE ug.groupid = ?::uuid
		 ON CONFLICT DO NOTHING`,
		courseSlug, groupID)
	if result.Error != nil {
		return 0, fmt.Errorf("backfill group course enrollments: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// ListCourseEnrollments mirrors the original hand-rolled join exactly (kept
// as raw SQL, not the query builder, per the ORM migration plan's guidance
// on reporting queries).
func (r *gormGroupRepository) ListCourseEnrollments(ctx context.Context, courseSlug string) ([]GroupEnrollmentRow, error) {
	var rows []GroupEnrollmentRow

	err := r.db.WithContext(ctx).Raw(`
		SELECT g.id::text AS id, g.name, g.source, COUNT(ug.userid) AS member_count, ge.createdat::text AS enrolled_at
		FROM group_enrollments ge
		JOIN groups g ON g.id = ge.groupid
		LEFT JOIN user_groups ug ON ug.groupid = ge.groupid
		WHERE ge.courseslug = ?
		GROUP BY g.id, g.name, g.source, ge.createdat
		ORDER BY g.name`, courseSlug).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list course group enrollments: %w", err)
	}

	return rows, nil
}

func (r *gormGroupRepository) UnenrollCourse(ctx context.Context, groupID, courseSlug string) error {
	err := r.db.WithContext(ctx).Exec(
		`DELETE FROM group_enrollments WHERE groupid = ?::uuid AND courseslug = ?`,
		groupID, courseSlug).Error
	if err != nil {
		return fmt.Errorf("unenroll group from course: %w", err)
	}

	return nil
}

// syncGroupMembership upserts a single group, links userID to it, and
// derives the platform role implied by that group's role mapping (if any).
func syncGroupMembership(tx *gorm.DB, userID, name, source, currentRole string) (string, bool, error) {
	var groupID string

	err := tx.Raw(
		`INSERT INTO groups (name, source, updatedat) VALUES (?, ?, NOW())
		 ON CONFLICT (name) DO UPDATE SET source = ?, updatedat = NOW() RETURNING id::text`,
		name, source, source).Scan(&groupID).Error
	if err != nil {
		return currentRole, false, fmt.Errorf("upsert group %q: %w", name, err)
	}

	err = tx.Exec(
		`INSERT INTO user_groups (userid, groupid) VALUES (?::uuid, ?::uuid) ON CONFLICT DO NOTHING`,
		userID, groupID).Error
	if err != nil {
		return currentRole, false, fmt.Errorf("link user to group %q: %w", name, err)
	}

	var mappedRole string

	scanErr := tx.Raw(`SELECT platformrole FROM group_role_mappings WHERE groupname = ?`, name).Scan(&mappedRole).Error
	if scanErr != nil || mappedRole == "" {
		return currentRole, false, nil
	}

	updatedRole := currentRole
	if mappedRole == groupsRoleAdmin {
		updatedRole = groupsRoleAdmin
	} else if updatedRole != groupsRoleAdmin {
		updatedRole = mappedRole
	}

	return updatedRole, true, nil
}
