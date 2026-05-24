package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultGroupName = "everyone"

// syncGroupEnrollments ensures the user is enrolled in every course
// that any of their groups are enrolled in. Called after every login.
func syncGroupEnrollments(ctx context.Context, pool *pgxpool.Pool, userID string) {
	pool.Exec(ctx,
		`INSERT INTO enrollments (user_id, course_slug)
		 SELECT $1::uuid, ge.course_slug
		 FROM group_enrollments ge
		 JOIN user_groups ug ON ug.group_id = ge.group_id
		 WHERE ug.user_id = $1::uuid
		 ON CONFLICT DO NOTHING`,
		userID)
}

// addToDefaultGroup ensures the user belongs to the platform default group.
// Called after every login regardless of auth method.
func addToDefaultGroup(ctx context.Context, pool *pgxpool.Pool, userID string) {
	var groupID string
	pool.QueryRow(ctx,
		`INSERT INTO groups (name, source, description)
		 VALUES ($1, 'local', 'Default group — all users are members automatically')
		 ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
		 RETURNING id::text`, defaultGroupName).Scan(&groupID)
	if groupID == "" {
		return
	}
	pool.Exec(ctx,
		`INSERT INTO user_groups (user_id, group_id)
		 VALUES ($1::uuid, $2::uuid)
		 ON CONFLICT DO NOTHING`, userID, groupID)
}

// syncGroupsAndDeriveRole upserts groups from an IdP into the groups table,
// updates the user's group memberships, and returns the highest platform role
// found in group_role_mappings ('admin' beats 'student'). Defaults to 'student'.
func syncGroupsAndDeriveRole(ctx context.Context, pool *pgxpool.Pool, userID string, groupNames []string, source string) (string, error) {
	role := "student"

	// Clear current memberships for this user
	_, err := pool.Exec(ctx,
		`DELETE FROM user_groups WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return role, err
	}

	for _, name := range groupNames {
		if name == "" {
			continue
		}
		// Upsert group
		var groupID string
		err := pool.QueryRow(ctx,
			`INSERT INTO groups (name, source, updated_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (name) DO UPDATE SET source = $2, updated_at = NOW()
			 RETURNING id::text`,
			name, source).Scan(&groupID)
		if err != nil {
			return role, err
		}

		// Link user → group
		_, err = pool.Exec(ctx,
			`INSERT INTO user_groups (user_id, group_id)
			 VALUES ($1::uuid, $2::uuid)
			 ON CONFLICT DO NOTHING`,
			userID, groupID)
		if err != nil {
			return role, err
		}

		// Check if this group maps to a role
		var mappedRole string
		err = pool.QueryRow(ctx,
			`SELECT platform_role FROM group_role_mappings WHERE group_name = $1`, name).
			Scan(&mappedRole)
		if err == nil && mappedRole == "admin" {
			role = "admin"
		}
	}

	// Apply derived role to user
	_, err = pool.Exec(ctx,
		`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2::uuid`,
		role, userID)
	if err != nil {
		return role, err
	}

	return role, nil
}

// ── Admin group handlers ───────────────────────────────────────────────────────

// CreateGroup godoc
// @Summary   Create a local group manually
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  body  object  true  "name"
// @Success   201  {object}  map[string]string
// @Failure   400  {object}  map[string]string
// @Failure   409  {object}  map[string]string
// @Router    /api/admin/groups [post]
func (s *State) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		s.Error(w, http.StatusBadRequest, "name required")
		return
	}

	var groupID string
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO groups (name, source)
		 VALUES ($1, 'local')
		 ON CONFLICT (name) DO NOTHING
		 RETURNING id::text`,
		body.Name).Scan(&groupID)
	if err != nil || groupID == "" {
		s.Error(w, http.StatusConflict, "A group with this name already exists")
		return
	}
	s.JSON(w, http.StatusCreated, map[string]string{"id": groupID, "message": "Group created"})
}

// DeleteGroup godoc
// @Summary   Delete a local group
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Param     group_id  path  string  true  "Group UUID"
// @Success   200  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Router    /api/admin/groups/{group_id} [delete]
func (s *State) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "group_id")
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM groups WHERE id = $1::uuid AND source = 'local'`, id)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	if tag.RowsAffected() == 0 {
		s.Error(w, http.StatusNotFound, "Group not found or not a local group")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Group deleted"})
}

// ListGroups godoc
// @Summary   List all known groups synced from IdP
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/groups [get]
func (s *State) ListGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT g.id::text, g.name, g.source, g.created_at::text,
		        COUNT(ug.user_id) AS member_count,
		        COALESCE(grm.platform_role, '') AS mapped_role
		 FROM groups g
		 LEFT JOIN user_groups ug ON ug.group_id = g.id
		 LEFT JOIN group_role_mappings grm ON grm.group_name = g.name
		 GROUP BY g.id, g.name, g.source, g.created_at, grm.platform_role
		 ORDER BY g.name`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type groupRow struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		CreatedAt   string `json:"created_at"`
		MemberCount int64  `json:"member_count"`
		MappedRole  string `json:"mapped_role"`
	}
	var groups []groupRow
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Source, &g.CreatedAt, &g.MemberCount, &g.MappedRole); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []groupRow{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"groups": groups})
}

// ListGroupMappings godoc
// @Summary   List group → role mappings
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/groups/mappings [get]
func (s *State) ListGroupMappings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT group_name, platform_role FROM group_role_mappings ORDER BY group_name`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type mapping struct {
		GroupName    string `json:"group_name"`
		PlatformRole string `json:"platform_role"`
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.GroupName, &m.PlatformRole); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		mappings = append(mappings, m)
	}
	if mappings == nil {
		mappings = []mapping{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

// UpsertGroupMapping godoc
// @Summary   Create or update a group → role mapping
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  body  object  true  "group_name and platform_role"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/groups/mappings [post]
func (s *State) UpsertGroupMapping(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupName    string `json:"group_name"`
		PlatformRole string `json:"platform_role"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.GroupName == "" {
		s.Error(w, http.StatusBadRequest, "group_name required")
		return
	}
	if body.PlatformRole != "admin" && body.PlatformRole != "student" {
		s.Error(w, http.StatusBadRequest, "platform_role must be 'admin' or 'student'")
		return
	}

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO group_role_mappings (group_name, platform_role)
		 VALUES ($1, $2)
		 ON CONFLICT (group_name) DO UPDATE SET platform_role = $2`,
		body.GroupName, body.PlatformRole)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Mapping saved"})
}

// DeleteGroupMapping godoc
// @Summary   Delete a group → role mapping
// @Tags      Admin - Groups
// @Security  BearerAuth
// @Produce   json
// @Param     group_name  path  string  true  "Group name"
// @Success   200  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Router    /api/admin/groups/mappings/{group_name} [delete]
func (s *State) DeleteGroupMapping(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "group_name")
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM group_role_mappings WHERE group_name = $1`, name)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	if tag.RowsAffected() == 0 {
		s.Error(w, http.StatusNotFound, "Mapping not found")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Mapping deleted"})
}
