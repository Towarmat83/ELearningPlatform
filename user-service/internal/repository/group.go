package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// defaultGroupName is the name of the group every user is automatically
// made a member of via AddToDefault.
const defaultGroupName = "everyone"

// groupSourceLocal is the source value for groups created directly by an
// admin, as opposed to one mirrored from an IdP during SSO login.
const groupSourceLocal = "local"

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
	// CreateAndJoin creates a local group named name and adds ownerID as a
	// member in a single transaction, preventing a group from existing
	// without an owner if the membership step fails.
	CreateAndJoin(ctx context.Context, name, ownerID string) (string, error)
	// CreateSubgroup creates a local group as a child of parentID.
	CreateSubgroup(ctx context.Context, name, parentID string) (string, error)
	Delete(ctx context.Context, id string) (bool, error)
	List(ctx context.Context) ([]GroupRow, error)
	ListMappings(ctx context.Context) ([]GroupMapping, error)
	UpsertMapping(ctx context.Context, groupName, platformRole string) error
	DeleteMapping(ctx context.Context, groupName string) (bool, error)

	// Hierarchy queries (groups.go handlers)
	GetByID(ctx context.Context, id string) (*models.Group, error)
	GetSubgroups(ctx context.Context, parentID string) ([]GroupRow, error)
	GetAncestors(ctx context.Context, id string) ([]GroupRow, error)
	GetDescendants(ctx context.Context, id string) ([]GroupRow, error)
	MoveGroup(ctx context.Context, id string, newParentID *string) error
	HasChildren(ctx context.Context, id string) (bool, error)

	// Login-time sync (auth.go/oauth.go)
	AddToDefault(ctx context.Context, userID string) error
	SyncEnrollments(ctx context.Context, userID string) error
	SyncGroupsAndDeriveRole(ctx context.Context, userID string, groupNames []string, source string) (string, error)

	// Admin course-enrollment cascade (admin.go)
	EnrollCourse(ctx context.Context, groupID, courseSlug string) (int64, error)
	ListCourseEnrollments(ctx context.Context, courseSlug string) ([]GroupEnrollmentRow, error)
	UnenrollCourse(ctx context.Context, groupID, courseSlug string) error

	// Admin member management (admin.go)
	ListMembers(ctx context.Context, groupID string) ([]AdminUserRow, error)
	AddMember(ctx context.Context, groupID, userID string) error
	RemoveMember(ctx context.Context, groupID, userID string) error

	// Manager scope queries (manager.go)
	GetGroupsByUserID(ctx context.Context, userID string) ([]GroupRow, error)
	UserInAnyGroup(ctx context.Context, userID string, groupIDs []string) (bool, error)
	ListMemberIDs(ctx context.Context, groupID string) ([]string, error)

	// User-facing group queries (my_groups.go)
	GetGroupCourses(ctx context.Context, groupID string) ([]string, error)
}

// gormGroupRepository is the GORM-backed GroupRepository implementation.
type gormGroupRepository struct {
	db *gorm.DB
}

// NewGormGroupRepository builds a GroupRepository backed by db.
//
//nolint:ireturn // repository constructors return the interface type by design
func NewGormGroupRepository(db *gorm.DB) GroupRepository {
	return &gormGroupRepository{db: db}
}

// Create inserts a new root local group named name and sets its path to its
// own ID. Does nothing if a group with that name already exists.
func (r *gormGroupRepository) Create(ctx context.Context, name string) (string, error) {
	var newID string

	err := r.db.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
		group := models.Group{Name: name, Source: groupSourceLocal}

		createErr := txDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&group).Error
		if createErr != nil {
			return fmt.Errorf("create group: %w", createErr)
		}

		if group.ID == uuid.Nil {
			return nil
		}

		group.Path = group.ID.String()

		updateErr := txDB.Model(&group).Update("path", group.Path).Error
		if updateErr != nil {
			return fmt.Errorf("set group path: %w", updateErr)
		}

		newID = group.ID.String()

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("create group: %w", err)
	}

	return newID, nil
}

// CreateAndJoin creates name as a local group and links ownerID to it in a
// single transaction. If the membership insert fails the group creation is
// rolled back, preventing a group from existing without an owner.
func (r *gormGroupRepository) CreateAndJoin(ctx context.Context, name, ownerID string) (string, error) {
	var groupID string

	err := r.db.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
		group := models.Group{Name: name, Source: groupSourceLocal}

		createErr := txDB.Create(&group).Error
		if createErr != nil {
			return fmt.Errorf("create group: %w", createErr)
		}

		if group.ID == uuid.Nil {
			return errors.New("group name already taken")
		}

		link := models.UserGroup{UserID: ownerID, GroupID: group.ID}

		addErr := txDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error
		if addErr != nil {
			return fmt.Errorf("add owner to group: %w", addErr)
		}

		groupID = group.ID.String()

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("create group and join: %w", err)
	}

	return groupID, nil
}

// Delete removes the local group identified by id, reporting whether a row
// was deleted.
func (r *gormGroupRepository) Delete(ctx context.Context, id string) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("id = ?::uuid AND source = ?", id, groupSourceLocal).
		Delete(&models.Group{})
	if result.Error != nil {
		return false, fmt.Errorf("delete group: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

// List returns every group with its member count and mapped role, ordered
// by name.
func (r *gormGroupRepository) List(ctx context.Context) ([]GroupRow, error) {
	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
			COUNT(ug.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("LEFT JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Group("g.id, g.name, g.source, g.createdat, grm.platformrole").
		Order("g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	return rows, nil
}

// ListMappings returns every group-to-platform-role mapping, ordered by
// group name.
func (r *gormGroupRepository) ListMappings(ctx context.Context) ([]GroupMapping, error) {
	var rows []GroupMapping

	err := r.db.WithContext(ctx).Model(&models.GroupRoleMapping{}).
		Select("groupname AS group_name, platformrole AS platform_role").
		Order("groupname").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list group mappings: %w", err)
	}

	return rows, nil
}

// UpsertMapping creates or updates the platform role mapped to groupName.
func (r *gormGroupRepository) UpsertMapping(ctx context.Context, groupName, platformRole string) error {
	mapping := models.GroupRoleMapping{GroupName: groupName, PlatformRole: platformRole}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "groupname"}},
		DoUpdates: clause.AssignmentColumns([]string{"platformrole"}),
	}).Create(&mapping).Error
	if err != nil {
		return fmt.Errorf("upsert group mapping: %w", err)
	}

	return nil
}

// DeleteMapping removes the role mapping for groupName, reporting whether a
// row was deleted.
func (r *gormGroupRepository) DeleteMapping(ctx context.Context, groupName string) (bool, error) {
	result := r.db.WithContext(ctx).Where("groupname = ?", groupName).Delete(&models.GroupRoleMapping{})
	if result.Error != nil {
		return false, fmt.Errorf("delete group mapping: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

// AddToDefault creates the shared "everyone" group if absent and links
// userID to it.
func (r *gormGroupRepository) AddToDefault(ctx context.Context, userID string) error {
	description := "Default group — all users are members automatically"
	group := models.Group{Name: defaultGroupName, Source: groupSourceLocal, Description: &description}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: colName}},
		DoUpdates: clause.Assignments(map[string]any{colUpdatedAt: gorm.Expr("NOW()")}),
	}).Create(&group).Error
	if err != nil {
		return fmt.Errorf("upsert default group: %w", err)
	}

	if group.ID == uuid.Nil {
		return nil
	}

	userGroup := models.UserGroup{UserID: userID, GroupID: group.ID}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&userGroup).Error
	if err != nil {
		return fmt.Errorf("link user to default group: %w", err)
	}

	return nil
}

// SyncEnrollments enrolls userID in every course reachable through their
// group enrollments, doing nothing for enrollments that already exist.
func (r *gormGroupRepository) SyncEnrollments(ctx context.Context, userID string) error {
	var courseSlugs []string

	err := r.db.WithContext(ctx).Table("group_enrollments AS ge").
		Joins("JOIN user_groups ug ON ug.groupid = ge.groupid").
		Where("ug.userid = ?::uuid", userID).
		Pluck("ge.courseslug", &courseSlugs).Error
	if err != nil {
		return fmt.Errorf("list group course slugs: %w", err)
	}

	if len(courseSlugs) == 0 {
		return nil
	}

	enrollments := make([]models.Enrollment, len(courseSlugs))
	for i, slug := range courseSlugs {
		enrollments[i] = models.Enrollment{UserID: userID, CourseSlug: slug}
	}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&enrollments).Error
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

	role := roleStudent

	err := r.db.WithContext(ctx).Transaction(func(groupTx *gorm.DB) error {
		selErr := groupTx.Model(&models.User{}).Select("role").Where("id = ?::uuid", userID).Scan(&dbRole).Error
		if selErr == nil && dbRole != "" {
			role = dbRole
		}

		// Only remove memberships in groups from the same SSO source so that
		// manually managed local group memberships are preserved across logins.
		delErr := groupTx.Delete(&models.UserGroup{}, "userid = ?::uuid AND groupid IN (SELECT id FROM groups WHERE source = ?)", userID, source).Error
		if delErr != nil {
			return fmt.Errorf("clear group memberships: %w", delErr)
		}

		updatedRole, roleMapped, syncErr := applyGroupMemberships(groupTx, userID, groupNames, source, role)
		if syncErr != nil {
			return syncErr
		}

		role = updatedRole

		if roleMapped {
			updErr := groupTx.Model(&models.User{}).Where("id = ?::uuid", userID).Update("role", role).Error
			if updErr != nil {
				return fmt.Errorf("persist derived role: %w", updErr)
			}
		}

		return nil
	})
	if err != nil {
		return role, fmt.Errorf("sync groups and derive role: %w", err)
	}

	return role, nil
}

// applyGroupMemberships syncs userID's membership in every non-empty name in
// groupNames, deriving the platform role along the way. It returns the
// (possibly updated) role, whether any group mapping was applied, and any
// error.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func applyGroupMemberships(groupTx *gorm.DB, userID string, groupNames []string, source, role string) (string, bool, error) {
	roleMapped := false

	for _, name := range groupNames {
		if name == "" {
			continue
		}

		updatedRole, mapped, err := syncGroupMembership(groupTx, userID, name, source, role)
		if err != nil {
			return role, false, err
		}

		role = updatedRole

		if mapped {
			roleMapped = true
		}
	}

	return role, roleMapped, nil
}

// EnrollCourse enrolls groupID in courseSlug and backfills enrollments for
// every existing member, returning the number of members newly enrolled.
func (r *gormGroupRepository) EnrollCourse(ctx context.Context, groupID, courseSlug string) (int64, error) {
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return 0, fmt.Errorf("parse group id: %w", err)
	}

	enrollment := models.GroupEnrollment{GroupID: groupUUID, CourseSlug: courseSlug}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&enrollment).Error
	if err != nil {
		return 0, fmt.Errorf("enroll group in course: %w", err)
	}

	var userIDs []string

	err = r.db.WithContext(ctx).Model(&models.UserGroup{}).
		Where("groupid = ?::uuid", groupID).
		Pluck("userid", &userIDs).Error
	if err != nil {
		return 0, fmt.Errorf("list group members: %w", err)
	}

	if len(userIDs) == 0 {
		return 0, nil
	}

	enrollments := make([]models.Enrollment, len(userIDs))
	for i, uid := range userIDs {
		enrollments[i] = models.Enrollment{UserID: uid, CourseSlug: courseSlug}
	}

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&enrollments)
	if result.Error != nil {
		return 0, fmt.Errorf("backfill group course enrollments: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// ListCourseEnrollments mirrors the original hand-rolled join exactly.
func (r *gormGroupRepository) ListCourseEnrollments(ctx context.Context, courseSlug string) ([]GroupEnrollmentRow, error) {
	var rows []GroupEnrollmentRow

	err := r.db.WithContext(ctx).Table("group_enrollments AS ge").
		Select("g.id::text AS id, g.name, g.source, COUNT(ug.userid) AS member_count, ge.createdat::text AS enrolled_at").
		Joins("JOIN groups g ON g.id = ge.groupid").
		Joins("LEFT JOIN user_groups ug ON ug.groupid = ge.groupid").
		Where("ge.courseslug = ?", courseSlug).
		Group("g.id, g.name, g.source, ge.createdat").
		Order("g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list course group enrollments: %w", err)
	}

	return rows, nil
}

// UnenrollCourse removes groupID's enrollment in courseSlug.
func (r *gormGroupRepository) UnenrollCourse(ctx context.Context, groupID, courseSlug string) error {
	err := r.db.WithContext(ctx).
		Where("groupid = ?::uuid AND courseslug = ?", groupID, courseSlug).
		Delete(&models.GroupEnrollment{}).Error
	if err != nil {
		return fmt.Errorf("unenroll group from course: %w", err)
	}

	return nil
}

// ListMembers returns every user belonging to the group identified by groupID.
func (r *gormGroupRepository) ListMembers(ctx context.Context, groupID string) ([]AdminUserRow, error) {
	var rows []AdminUserRow

	err := r.db.WithContext(ctx).Table("users AS u").
		Select(`u.id::text AS id, u.username, u.email, u.role, u.isactive AS is_active,
			u.avatarurl AS avatar_url, u.bio, u.authprovider AS auth_provider,
			u.createdat::text AS created_at,
			COUNT(e.courseslug) AS enrolled_courses`).
		Joins("JOIN user_groups ug ON ug.userid = u.id").
		Joins("LEFT JOIN enrollments e ON e.userid = u.id").
		Where("ug.groupid = ?::uuid", groupID).
		Group("u.id, u.username, u.email, u.role, u.isactive, u.avatarurl, u.bio, u.authprovider, u.createdat").
		Order("u.username").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}

	return rows, nil
}

// AddMember links userID to the group identified by groupID, doing nothing
// if the membership already exists.
func (r *gormGroupRepository) AddMember(ctx context.Context, groupID, userID string) error {
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return fmt.Errorf("parse group id: %w", err)
	}

	link := models.UserGroup{UserID: userID, GroupID: groupUUID}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("add group member: %w", err)
	}

	return nil
}

// RemoveMember unlinks userID from the group identified by groupID.
func (r *gormGroupRepository) RemoveMember(ctx context.Context, groupID, userID string) error {
	err := r.db.WithContext(ctx).
		Where("groupid = ?::uuid AND userid = ?::uuid", groupID, userID).
		Delete(&models.UserGroup{}).Error
	if err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}

	return nil
}

// GetGroupsByUserID returns all groups the given user belongs to.
func (r *gormGroupRepository) GetGroupsByUserID(ctx context.Context, userID string) ([]GroupRow, error) {
	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
			COUNT(ug2.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN user_groups ug2 ON ug2.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Where("ug.userid = ?::uuid", userID).
		Group("g.id, g.name, g.source, g.createdat, grm.platformrole").
		Order("g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get groups by user id: %w", err)
	}

	return rows, nil
}

// GetGroupCourses returns the course slugs that groupID is enrolled in.
func (r *gormGroupRepository) GetGroupCourses(ctx context.Context, groupID string) ([]string, error) {
	var slugs []string

	err := r.db.WithContext(ctx).Table("group_enrollments").
		Where("groupid = ?::uuid", groupID).
		Pluck("courseslug", &slugs).Error
	if err != nil {
		return nil, fmt.Errorf("get group courses: %w", err)
	}

	return slugs, nil
}

// ListMemberIDs returns the user IDs (as strings) of every member of groupID.
func (r *gormGroupRepository) ListMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	var ids []string

	err := r.db.WithContext(ctx).Table("user_groups").
		Select("userid::text").
		Where("groupid = ?::uuid", groupID).
		Pluck("userid::text", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("list member ids: %w", err)
	}

	return ids, nil
}

// UserInAnyGroup reports whether userID belongs to at least one of the
// groups listed in groupIDs.
func (r *gormGroupRepository) UserInAnyGroup(ctx context.Context, userID string, groupIDs []string) (bool, error) {
	if len(groupIDs) == 0 {
		return false, nil
	}

	var count int64

	err := r.db.WithContext(ctx).Model(&models.UserGroup{}).
		Where("userid = ?::uuid AND groupid::text = ANY(?)", userID, pq.Array(groupIDs)).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check group membership: %w", err)
	}

	return count > 0, nil
}

// CreateSubgroup creates a local child group under parentID, enforcing the
// maximum nesting depth. Path and depth are derived from the parent.
func (r *gormGroupRepository) CreateSubgroup(ctx context.Context, name, parentID string) (string, error) {
	var newID string

	err := r.db.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
		var parent models.Group

		findErr := txDB.Where("id = ?::uuid", parentID).First(&parent).Error
		if findErr != nil {
			return fmt.Errorf("parent group not found: %w", findErr)
		}

		if parent.Depth >= models.GroupMaxDepth {
			return fmt.Errorf("maximum subgroup depth (%d) exceeded", models.GroupMaxDepth)
		}

		group := models.Group{
			Name:     name,
			Source:   groupSourceLocal,
			ParentID: &parent.ID,
			Depth:    parent.Depth + 1,
			Path:     "",
		}

		createErr := txDB.Create(&group).Error
		if createErr != nil {
			return fmt.Errorf("create subgroup: %w", createErr)
		}

		group.Path = parent.Path + "/" + group.ID.String()

		updateErr := txDB.Model(&group).Update("path", group.Path).Error
		if updateErr != nil {
			return fmt.Errorf("update subgroup path: %w", updateErr)
		}

		newID = group.ID.String()

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("create subgroup: %w", err)
	}

	return newID, nil
}

// GetByID returns the group with the given ID.
func (r *gormGroupRepository) GetByID(ctx context.Context, id string) (*models.Group, error) {
	var group models.Group

	err := r.db.WithContext(ctx).Where("id = ?::uuid", id).First(&group).Error
	if err != nil {
		return nil, fmt.Errorf("get group by id: %w", err)
	}

	return &group, nil
}

// GetSubgroups returns the direct children of parentID with member counts.
func (r *gormGroupRepository) GetSubgroups(ctx context.Context, parentID string) ([]GroupRow, error) {
	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
			COUNT(ug.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("LEFT JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Where("g.parent_id = ?::uuid", parentID).
		Group("g.id, g.name, g.source, g.createdat, grm.platformrole").
		Order("g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get subgroups: %w", err)
	}

	return rows, nil
}

// GetAncestors returns all ancestors of id ordered from root to parent,
// using the path column to avoid recursive queries.
func (r *gormGroupRepository) GetAncestors(ctx context.Context, groupID string) ([]GroupRow, error) {
	var group models.Group

	findErr := r.db.WithContext(ctx).Where("id = ?::uuid", groupID).First(&group).Error
	if findErr != nil {
		return nil, fmt.Errorf("get group for ancestors: %w", findErr)
	}

	parts := splitPath(group.Path)
	if len(parts) <= 1 {
		return []GroupRow{}, nil
	}

	ancestorIDs := parts[:len(parts)-1]

	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
			COUNT(ug.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("LEFT JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Where("g.id::text = ANY(?)", pq.Array(ancestorIDs)).
		Group("g.id, g.name, g.source, g.createdat, grm.platformrole").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get ancestors: %w", err)
	}

	return rows, nil
}

// GetDescendants returns all descendants of id using a path prefix query.
func (r *gormGroupRepository) GetDescendants(ctx context.Context, groupID string) ([]GroupRow, error) {
	var group models.Group

	findErr := r.db.WithContext(ctx).Where("id = ?::uuid", groupID).First(&group).Error
	if findErr != nil {
		return nil, fmt.Errorf("get group for descendants: %w", findErr)
	}

	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source, g.createdat::text AS created_at,
			COUNT(ug.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("LEFT JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Where("g.path LIKE ?", group.Path+"/%").
		Group("g.id, g.name, g.source, g.createdat, grm.platformrole").
		Order("g.depth, g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get descendants: %w", err)
	}

	return rows, nil
}

// MoveGroup moves groupID under newParentID (nil = promote to root), updating
// path and depth for the group and all its descendants atomically.
//
//nolint:funlen,gocyclo,cyclop // path rewriting + cycle detection require multiple steps; extraction would hurt readability
func (r *gormGroupRepository) MoveGroup(ctx context.Context, groupID string, newParentID *string) error {
	err := r.db.WithContext(ctx).Transaction(func(txDB *gorm.DB) error {
		var group models.Group

		findErr := txDB.Where("id = ?::uuid", groupID).First(&group).Error
		if findErr != nil {
			return fmt.Errorf("group not found: %w", findErr)
		}

		var newDepth int

		var newPath string

		if newParentID != nil {
			var parent models.Group

			parentErr := txDB.Where("id = ?::uuid", *newParentID).First(&parent).Error
			if parentErr != nil {
				return fmt.Errorf("new parent not found: %w", parentErr)
			}

			// Prevent cycle: new parent must not be a descendant of the group being moved.
			if len(parent.Path) >= len(group.Path) && parent.Path[:len(group.Path)] == group.Path {
				return errors.New("cannot move group under one of its own descendants")
			}

			subtreeDepth := maxDescendantDepth(txDB, group.Path)
			if parent.Depth+1+subtreeDepth > models.GroupMaxDepth {
				return fmt.Errorf("move would exceed maximum depth (%d)", models.GroupMaxDepth)
			}

			newDepth = parent.Depth + 1
			newPath = parent.Path + "/" + groupID
		} else {
			newDepth = 0
			newPath = groupID
		}

		depthDelta := newDepth - group.Depth
		oldPrefix := group.Path

		updateErr := txDB.Exec(`
			UPDATE groups
			SET path  = ? || substring(path FROM ?)::text,
			    depth = depth + ?
			WHERE path LIKE ?`,
			newPath, len(oldPrefix)+1, depthDelta, oldPrefix+"/%",
		).Error
		if updateErr != nil {
			return fmt.Errorf("update descendants paths: %w", updateErr)
		}

		parentUUID := (*uuid.UUID)(nil)

		if newParentID != nil {
			parsed, parseErr := uuid.Parse(*newParentID)
			if parseErr != nil {
				return fmt.Errorf("parse new parent id: %w", parseErr)
			}

			parentUUID = &parsed
		}

		return txDB.Model(&group).Updates(map[string]any{
			"parent_id": parentUUID,
			"depth":     newDepth,
			"path":      newPath,
		}).Error
	})
	if err != nil {
		return fmt.Errorf("move group: %w", err)
	}

	return nil
}

// HasChildren reports whether the group identified by id has direct children.
func (r *gormGroupRepository) HasChildren(ctx context.Context, id string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.Group{}).
		Where("parent_id = ?::uuid", id).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check children: %w", err)
	}

	return count > 0, nil
}

// splitPath splits a path string on "/" and returns the parts.
func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := make([]string, 0)
	start := 0

	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}

	return parts
}

// maxDescendantDepth returns the maximum relative depth within the subtree
// rooted at pathPrefix (0 if no descendants).
func maxDescendantDepth(tx *gorm.DB, pathPrefix string) int {
	var maxDepth int

	tx.Model(&models.Group{}).
		Where("path LIKE ?", pathPrefix+"/%").
		Select("COALESCE(MAX(depth), 0)").
		Scan(&maxDepth)

	return maxDepth
}

// syncGroupMembership upserts a single group, links userID to it, and
// derives the platform role implied by that group's role mapping (if any).
// It returns the (possibly updated) role, whether a mapping was applied,
// and any error.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func syncGroupMembership(groupTx *gorm.DB, userID, name, source, currentRole string) (string, bool, error) {
	group := models.Group{Name: name, Source: source}

	err := groupTx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: colName}},
		DoUpdates: clause.Assignments(map[string]any{colSource: source, colUpdatedAt: gorm.Expr("NOW()")}),
	}).Create(&group).Error
	if err != nil {
		return currentRole, false, fmt.Errorf("upsert group %q: %w", name, err)
	}

	userGroup := models.UserGroup{UserID: userID, GroupID: group.ID}

	err = groupTx.Clauses(clause.OnConflict{DoNothing: true}).Create(&userGroup).Error
	if err != nil {
		return currentRole, false, fmt.Errorf("link user to group %q: %w", name, err)
	}

	var mappedRole string

	scanErr := groupTx.Model(&models.GroupRoleMapping{}).
		Select("platformrole").Where("groupname = ?", name).Scan(&mappedRole).Error
	if scanErr != nil {
		return currentRole, false, fmt.Errorf("lookup group mapping %q: %w", name, scanErr)
	}

	if mappedRole == "" {
		return currentRole, false, nil
	}

	updatedRole := currentRole
	if mappedRole == roleAdmin {
		updatedRole = roleAdmin
	} else if updatedRole != roleAdmin {
		updatedRole = mappedRole
	}

	return updatedRole, true, nil
}
