package fake

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

const (
	// groupsRoleAdmin is the platform role derived from a group mapped to "admin".
	groupsRoleAdmin = "admin"
	// groupsRoleStudent is the default platform role when no mapping applies.
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

// Create adds a new local group named name, returning its generated ID.
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
	f.Groups = append(f.Groups, models.Group{ID: id, Name: name, Source: authProviderLocal})

	return id.String(), nil
}

// Delete removes the local group with groupID, a no-op for non-local groups.
func (f *GroupRepository) Delete(_ context.Context, groupID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for i, g := range f.Groups {
		if g.ID.String() == groupID && g.Source == authProviderLocal {
			f.Groups = append(f.Groups[:i], f.Groups[i+1:]...)

			return true, nil
		}
	}

	return false, nil
}

// List returns every group along with its member count and mapped role.
func (f *GroupRepository) List(_ context.Context) ([]repository.GroupRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	rows := make([]repository.GroupRow, 0, len(f.Groups))

	for _, group := range f.Groups {
		memberCount := int64(0)

		for _, ug := range f.UserGroup {
			if ug.GroupID == group.ID {
				memberCount++
			}
		}

		mappedRole := ""

		for _, m := range f.Mappings {
			if m.GroupName == group.Name {
				mappedRole = m.PlatformRole

				break
			}
		}

		rows = append(rows, repository.GroupRow{
			ID: group.ID.String(), Name: group.Name, Source: group.Source,
			CreatedAt: group.CreatedAt.String(), MemberCount: memberCount, MappedRole: mappedRole,
		})
	}

	return rows, nil
}

// ListMappings returns every group-to-role mapping.
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

// UpsertMapping creates or updates the role mapping for groupName.
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

// DeleteMapping removes the role mapping for groupName, if present.
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

// AddToDefault is a no-op in the fake; it exists to satisfy the interface.
func (f *GroupRepository) AddToDefault(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.Err
}

// SyncEnrollments is a no-op in the fake; it exists to satisfy the interface.
func (f *GroupRepository) SyncEnrollments(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.Err
}

// SyncGroupsAndDeriveRole replaces userID's group memberships with groupNames
// and derives the platform role along the way.
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

		var mapped bool

		userGroups, role, mapped = f.applyGroupMembership(userGroups, userID, name, source, role)
		if mapped {
			roleMapped = true
		}
	}

	f.UserGroup = userGroups
	if roleMapped {
		f.UserRoles[userID] = role
	}

	return role, nil
}

// EnrollCourse enrolls the group in courseSlug and returns the count of
// members backfilled into it.
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

	for _, enrollment := range f.GroupEnrollments {
		if enrollment.CourseSlug != courseSlug {
			continue
		}

		var group models.Group

		for _, g := range f.Groups {
			if g.ID == enrollment.GroupID {
				group = g

				break
			}
		}

		memberCount := int64(0)

		for _, ug := range f.UserGroup {
			if ug.GroupID == enrollment.GroupID {
				memberCount++
			}
		}

		rows = append(rows, repository.GroupEnrollmentRow{
			ID: group.ID.String(), Name: group.Name, Source: group.Source, MemberCount: memberCount,
		})
	}

	return rows, nil
}

// UnenrollCourse removes the group's enrollment in courseSlug, if present.
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

// applyGroupMembership adds userID to the group named name (creating it if
// absent) and returns the updated userGroups slice, the (possibly updated)
// role, and whether a group-role mapping applied.
func (f *GroupRepository) applyGroupMembership(
	userGroups []models.UserGroup, userID, name, source, role string,
) ([]models.UserGroup, string, bool) {
	groupID := f.findOrCreateGroup(name, source)
	userGroups = append(userGroups, models.UserGroup{UserID: userID, GroupID: groupID})

	for _, mapping := range f.Mappings {
		if mapping.GroupName != name {
			continue
		}

		if mapping.PlatformRole == groupsRoleAdmin {
			role = groupsRoleAdmin
		} else if role != groupsRoleAdmin {
			role = mapping.PlatformRole
		}

		return userGroups, role, true
	}

	return userGroups, role, false
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
