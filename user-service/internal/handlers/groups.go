package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/genesary/pupitre/user-service/internal/db"
)

// defaultGroupName is the platform-wide group every user is added to.
const defaultGroupName = "everyone"

// Platform role names used when deriving a user's role from group
// membership synced from an IdP.
const (
	groupsRoleAdmin   = "admin"
	groupsRoleStudent = "student"
)

// JSON response field/message literals reused across this file's handlers.
const (
	groupsRespKeyMessage = "message"
	groupsRespKeyGroups  = "groups"
)

// syncGroupEnrollments ensures the user is enrolled in every course
// that any of their groups are enrolled in. Called after every login.
func syncGroupEnrollments(ctx context.Context, pool db.Pool, userID string) {
	_, _ = pool.Exec(ctx,
		`INSERT INTO enrollments (userId, courseSlug)
		 SELECT $1::uuid, ge.courseSlug
		 FROM group_enrollments ge
		 JOIN user_groups ug ON ug.groupId = ge.groupId
		 WHERE ug.userId = $1::uuid
		 ON CONFLICT DO NOTHING`,
		userID)
}

// addToDefaultGroup ensures the user belongs to the platform default group.
// Called after every login regardless of auth method.
func addToDefaultGroup(ctx context.Context, pool db.Pool, userID string) {
	var groupID string

	_ = pool.QueryRow(ctx,
		`INSERT INTO groups (name, source, description)
		 VALUES ($1, 'local', 'Default group — all users are members automatically')
		 ON CONFLICT (name) DO UPDATE SET updatedAt = NOW()
		 RETURNING id::text`, defaultGroupName).Scan(&groupID)
	if groupID == "" {
		return
	}

	_, _ = pool.Exec(ctx,
		`INSERT INTO user_groups (userId, groupId)
		 VALUES ($1::uuid, $2::uuid)
		 ON CONFLICT DO NOTHING`, userID, groupID)
}

// syncGroupsAndDeriveRole upserts groups from an IdP into the groups table,
// updates the user's group memberships, and returns the highest platform
// role found in group_role_mappings ('admin' beats 'student'). Defaults to
// 'student'.
func syncGroupsAndDeriveRole(ctx context.Context, pool db.Pool, userID string, groupNames []string, source string) (string, error) {
	// Start from the user's current role so SSO logins without group mappings
	// never silently downgrade an existing admin.
	var role string

	err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid`, userID).Scan(&role)
	if err != nil {
		role = groupsRoleStudent
	}

	// Clear current memberships for this user
	_, err = pool.Exec(ctx,
		`DELETE FROM user_groups WHERE userId = $1::uuid`, userID)
	if err != nil {
		return role, fmt.Errorf("clear group memberships: %w", err)
	}

	roleMapped := false

	for _, name := range groupNames {
		if name == "" {
			continue
		}

		updatedRole, mapped, membershipErr := groupsSyncMembership(ctx, pool, userID, name, source, role)
		if membershipErr != nil {
			return role, membershipErr
		}

		role = updatedRole

		if mapped {
			roleMapped = true
		}
	}

	// Only persist the role when group mappings explicitly provided guidance.
	// Without mappings, keep the existing role unchanged.
	if roleMapped {
		_, err = pool.Exec(ctx,
			`UPDATE users SET role = $1, updatedAt = NOW() WHERE id = $2::uuid`,
			role, userID)
		if err != nil {
			return role, fmt.Errorf("persist derived role: %w", err)
		}
	}

	return role, nil
}

// groupsSyncMembership upserts a single group, links userID to it, and
// derives the platform role implied by that group's role mapping (if any).
// currentRole is returned unchanged when the group has no mapping. Results
// are (derivedRole, wasMapped, err).
//
//nolint:gocritic // unnamedResult: named returns are disallowed repo-wide by nonamedreturns
func groupsSyncMembership(
	ctx context.Context, pool db.Pool, userID, name, source, currentRole string,
) (string, bool, error) {
	var groupID string

	err := pool.QueryRow(ctx,
		`INSERT INTO groups (name, source, updatedAt)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (name) DO UPDATE SET source = $2, updatedAt = NOW()
		 RETURNING id::text`,
		name, source).Scan(&groupID)
	if err != nil {
		return currentRole, false, fmt.Errorf("upsert group %q: %w", name, err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO user_groups (userId, groupId)
		 VALUES ($1::uuid, $2::uuid)
		 ON CONFLICT DO NOTHING`,
		userID, groupID)
	if err != nil {
		return currentRole, false, fmt.Errorf("link user to group %q: %w", name, err)
	}

	var mappedRole string

	scanErr := pool.QueryRow(ctx,
		`SELECT platformRole FROM group_role_mappings WHERE groupName = $1`, name).
		Scan(&mappedRole)
	if scanErr != nil {
		//nolint:nilerr // no group_role_mappings row is the expected outcome for
		// an unmapped group, not a query failure that should propagate.
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

// ── Admin group handlers ───────────────────────────────────────────────────────

// CreateGroup godoc
// @Summary   Create a local group manually
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  object  true  "name"
// @Success   201  {object}  map[string]string
// @Failure   400  {object}  map[string]string
// @Failure   409  {object}  map[string]string
// @Router    /api/admin/groups [post].
func (s *State) CreateGroup(writer http.ResponseWriter, req *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	err := decode(req, &body)
	if err != nil || body.Name == "" {
		s.Error(writer, http.StatusBadRequest, "name required")

		return
	}

	var groupID string

	err = s.Pool.QueryRow(req.Context(),
		`INSERT INTO groups (name, source)
		 VALUES ($1, 'local')
		 ON CONFLICT (name) DO NOTHING
		 RETURNING id::text`,
		body.Name).Scan(&groupID)
	if err != nil || groupID == "" {
		s.Error(writer, http.StatusConflict, "A group with this name already exists")

		return
	}

	s.JSON(writer, http.StatusCreated, map[string]string{"id": groupID, groupsRespKeyMessage: "Group created"})
}

// DeleteGroup godoc
// @Summary   Delete a local group
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Param     groupId  path  string  true  "Group UUID"
// @Success   200  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Router    /api/admin/groups/{groupId} [delete].
func (s *State) DeleteGroup(writer http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "groupId")

	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM groups WHERE id = $1::uuid AND source = 'local'`, id)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if tag.RowsAffected() == 0 {
		s.Error(writer, http.StatusNotFound, "Group not found or not a local group")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Group deleted"})
}

// ListGroups godoc
// @Summary   List all known groups synced from IdP
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/groups [get].
func (s *State) ListGroups(writer http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT g.id::text, g.name, g.source, g.createdAt::text,
		        COUNT(ug.userId) AS memberCount,
		        COALESCE(grm.platformRole, '') AS mappedRole
		 FROM groups g
		 LEFT JOIN user_groups ug ON ug.groupId = g.id
		 LEFT JOIN group_role_mappings grm ON grm.groupName = g.name
		 GROUP BY g.id, g.name, g.source, g.createdAt, grm.platformRole
		 ORDER BY g.name`)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type groupRow struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		CreatedAt   string `json:"createdAt"`
		MemberCount int64  `json:"memberCount"`
		MappedRole  string `json:"mappedRole"`
	}

	var groups []groupRow

	for rows.Next() {
		var groupItem groupRow

		err := rows.Scan(&groupItem.ID, &groupItem.Name, &groupItem.Source, &groupItem.CreatedAt,
			&groupItem.MemberCount, &groupItem.MappedRole)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Scan error")

			return
		}

		groups = append(groups, groupItem)
	}

	if groups == nil {
		groups = []groupRow{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{groupsRespKeyGroups: groups})
}

// ListGroupMappings godoc
// @Summary   List group → role mappings
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/groups/mappings [get].
func (s *State) ListGroupMappings(writer http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT groupName, platformRole FROM group_role_mappings ORDER BY groupName`)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type mapping struct {
		GroupName    string `json:"groupName"`
		PlatformRole string `json:"platformRole"`
	}

	var mappings []mapping

	for rows.Next() {
		var mappingItem mapping

		err := rows.Scan(&mappingItem.GroupName, &mappingItem.PlatformRole)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Scan error")

			return
		}

		mappings = append(mappings, mappingItem)
	}

	if mappings == nil {
		mappings = []mapping{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"mappings": mappings})
}

// UpsertGroupMapping godoc
// @Summary   Create or update a group → role mapping
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  object  true  "groupName and platformRole"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/groups/mappings [post].
func (s *State) UpsertGroupMapping(writer http.ResponseWriter, req *http.Request) {
	var body struct {
		GroupName    string `json:"groupName"`
		PlatformRole string `json:"platformRole"`
	}

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.GroupName == "" {
		s.Error(writer, http.StatusBadRequest, "groupName required")

		return
	}

	if body.PlatformRole != groupsRoleAdmin && body.PlatformRole != groupsRoleStudent {
		s.Error(writer, http.StatusBadRequest, "platformRole must be 'admin' or 'student'")

		return
	}

	_, err = s.Pool.Exec(req.Context(),
		`INSERT INTO group_role_mappings (groupName, platformRole)
		 VALUES ($1, $2)
		 ON CONFLICT (groupName) DO UPDATE SET platformRole = $2`,
		body.GroupName, body.PlatformRole)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Mapping saved"})
}

// DeleteGroupMapping godoc
// @Summary   Delete a group → role mapping
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Param     groupName  path  string  true  "Group name"
// @Success   200  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Router    /api/admin/groups/mappings/{groupName} [delete].
func (s *State) DeleteGroupMapping(writer http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "groupName")

	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM group_role_mappings WHERE groupName = $1`, name)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if tag.RowsAffected() == 0 {
		s.Error(writer, http.StatusNotFound, "Mapping not found")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Mapping deleted"})
}
