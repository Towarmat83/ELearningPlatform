package fake

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

const (
	groupsRoleAdmin   = "admin"
	groupsRoleStudent = "student"
)

// GroupRepository is an in-memory repository.GroupRepository for tests.
//
// UserRoles seeds each user's current platform role, read/written by
// SyncGroupsAndDeriveRole the same way the real repository reads/writes
// users.role. AddToDefault and SyncEnrollments are no-ops beyond error
// injection: no test exercises their cross-aggregate business logic, since
// the real callers (auth.go/oauth.go) already treat them as fire-and-forget.
type GroupRepository struct {
	mu sync.Mutex

	Groups           []models.Group
	UserGroup        []models.UserGroup
	Mappings         []models.GroupRoleMapping
	GroupEnrollments []models.GroupEnrollment
	UserRoles        map[string]string

	// Err, when set, is returned by every method call (simulates a DB failure).
	// For SyncGroupsAndDeriveRole, it is checked before any state is mutated,
	// mirroring the real repository's transactional rollback: a failed sync
	// leaves the user's prior groups/role untouched.
	Err error
}

// NewGroupRepository builds a fake GroupRepository seeded with groups.
func NewGroupRepository(seed ...models.Group) *GroupRepository {
	return &GroupRepository{
		Groups:    append([]models.Group{}, seed...),
		UserRoles: make(map[string]string),
	}
}

func (f *GroupRepository) Create(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return "", f.Err
	}

	for _, g := range f.Groups {
		if g.Name == name {
			return "", nil
		}
	}

	id := uuid.New()
	f.Groups = append(f.Groups, models.Group{ID: id, Name: name, Source: "local"})

	return id.String(), nil
}

func (f *GroupRepository) Delete(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for i, g := range f.Groups {
		if g.ID.String() == id && g.Source == "local" {
			f.Groups = append(f.Groups[:i], f.Groups[i+1:]...)

			return true, nil
		}
	}

	return false, nil
}

func (f *GroupRepository) List(_ context.Context) ([]repository.GroupRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	rows := make([]repository.GroupRow, 0, len(f.Groups))

	for _, g := range f.Groups {
		memberCount := int64(0)

		for _, ug := range f.UserGroup {
			if ug.GroupID == g.ID {
				memberCount++
			}
		}

		mappedRole := ""

		for _, m := range f.Mappings {
			if m.GroupName == g.Name {
				mappedRole = m.PlatformRole

				break
			}
		}

		rows = append(rows, repository.GroupRow{
			ID: g.ID.String(), Name: g.Name, Source: g.Source,
			CreatedAt: g.CreatedAt.String(), MemberCount: memberCount, MappedRole: mappedRole,
		})
	}

	return rows, nil
}

func (f *GroupRepository) ListMappings(_ context.Context) ([]repository.GroupMapping, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	rows := make([]repository.GroupMapping, 0, len(f.Mappings))
	for _, m := range f.Mappings {
		rows = append(rows, repository.GroupMapping{GroupName: m.GroupName, PlatformRole: m.PlatformRole})
	}

	return rows, nil
}

func (f *GroupRepository) UpsertMapping(_ context.Context, groupName, platformRole string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	for i, m := range f.Mappings {
		if m.GroupName == groupName {
			f.Mappings[i].PlatformRole = platformRole

			return nil
		}
	}

	f.Mappings = append(f.Mappings, models.GroupRoleMapping{GroupName: groupName, PlatformRole: platformRole})

	return nil
}

func (f *GroupRepository) DeleteMapping(_ context.Context, groupName string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for i, m := range f.Mappings {
		if m.GroupName == groupName {
			f.Mappings = append(f.Mappings[:i], f.Mappings[i+1:]...)

			return true, nil
		}
	}

	return false, nil
}

func (f *GroupRepository) AddToDefault(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.Err
}

func (f *GroupRepository) SyncEnrollments(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.Err
}

func (f *GroupRepository) SyncGroupsAndDeriveRole(
	_ context.Context, userID string, groupNames []string, source string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	role := f.UserRoles[userID]
	if role == "" {
		role = groupsRoleStudent
	}

	if f.Err != nil {
		return role, f.Err
	}

	userGroups := make([]models.UserGroup, 0, len(f.UserGroup))

	for _, ug := range f.UserGroup {
		if ug.UserID != userID {
			userGroups = append(userGroups, ug)
		}
	}

	roleMapped := false

	for _, name := range groupNames {
		if name == "" {
			continue
		}

		groupID := f.findOrCreateGroup(name, source)
		userGroups = append(userGroups, models.UserGroup{UserID: userID, GroupID: groupID})

		for _, m := range f.Mappings {
			if m.GroupName != name {
				continue
			}

			roleMapped = true
			if m.PlatformRole == groupsRoleAdmin {
				role = groupsRoleAdmin
			} else if role != groupsRoleAdmin {
				role = m.PlatformRole
			}

			break
		}
	}

	f.UserGroup = userGroups
	if roleMapped {
		f.UserRoles[userID] = role
	}

	return role, nil
}

func (f *GroupRepository) EnrollCourse(_ context.Context, groupID, courseSlug string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return 0, f.Err
	}

	gid, err := uuid.Parse(groupID)
	if err != nil {
		return 0, nil
	}

	found := false

	for _, ge := range f.GroupEnrollments {
		if ge.GroupID == gid && ge.CourseSlug == courseSlug {
			found = true

			break
		}
	}

	if !found {
		f.GroupEnrollments = append(f.GroupEnrollments, models.GroupEnrollment{GroupID: gid, CourseSlug: courseSlug})
	}

	var backfilled int64

	for _, ug := range f.UserGroup {
		if ug.GroupID != gid {
			continue
		}

		backfilled++
	}

	return backfilled, nil
}

// ListCourseEnrollments reports each enrolled group's member count from
// UserGroup, matching the real repository's LEFT JOIN against user_groups.
func (f *GroupRepository) ListCourseEnrollments(_ context.Context, courseSlug string) ([]repository.GroupEnrollmentRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	rows := make([]repository.GroupEnrollmentRow, 0)

	for _, ge := range f.GroupEnrollments {
		if ge.CourseSlug != courseSlug {
			continue
		}

		var group models.Group

		for _, g := range f.Groups {
			if g.ID == ge.GroupID {
				group = g

				break
			}
		}

		memberCount := int64(0)

		for _, ug := range f.UserGroup {
			if ug.GroupID == ge.GroupID {
				memberCount++
			}
		}

		rows = append(rows, repository.GroupEnrollmentRow{
			ID: group.ID.String(), Name: group.Name, Source: group.Source, MemberCount: memberCount,
		})
	}

	return rows, nil
}

func (f *GroupRepository) UnenrollCourse(_ context.Context, groupID, courseSlug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	for i, ge := range f.GroupEnrollments {
		if ge.GroupID.String() == groupID && ge.CourseSlug == courseSlug {
			f.GroupEnrollments = append(f.GroupEnrollments[:i], f.GroupEnrollments[i+1:]...)

			break
		}
	}

	return nil
}

// findOrCreateGroup returns the ID of the group named name, upserting it
// (and its source) if it doesn't already exist.
func (f *GroupRepository) findOrCreateGroup(name, source string) uuid.UUID {
	for i, g := range f.Groups {
		if g.Name == name {
			f.Groups[i].Source = source

			return g.ID
		}
	}

	id := uuid.New()
	f.Groups = append(f.Groups, models.Group{ID: id, Name: name, Source: source})

	return id
}
