package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// Platform role names used when deriving a user's role from group
// membership synced from an IdP.
const (
	groupsRoleAdmin   = "admin"
	groupsRoleManager = "manager"
	groupsRoleStudent = "student"
)

// JSON response field/message literals reused across this file's handlers.
const (
	groupsRespKeyMessage = "message"
	groupsRespKeyGroups  = "groups"
)

// syncGroupEnrollments ensures the user is enrolled in every course
// that any of their groups are enrolled in. Called after every login.
func syncGroupEnrollments(ctx context.Context, groups repository.GroupRepository, userID string) {
	_ = groups.SyncEnrollments(ctx, userID)
}

// addToDefaultGroup ensures the user belongs to the platform default group.
// Called after every login regardless of auth method.
func addToDefaultGroup(ctx context.Context, groups repository.GroupRepository, userID string) {
	_ = groups.AddToDefault(ctx, userID)
}

// syncGroupsAndDeriveRole upserts groups from an IdP into the groups table,
// updates the user's group memberships, and returns the highest platform
// role found in group_role_mappings ('admin' beats 'student'). Defaults to
// the user's current role, falling back to 'student'.
func syncGroupsAndDeriveRole(
	ctx context.Context, groups repository.GroupRepository, userID string, groupNames []string, source string,
) (string, error) {
	role, err := groups.SyncGroupsAndDeriveRole(ctx, userID, groupNames, source)
	if err != nil {
		return role, fmt.Errorf("sync groups and derive role: %w", err)
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

	groupID, err := s.Repos.Groups.Create(req.Context(), body.Name)
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

	deleted, err := s.Repos.Groups.Delete(r.Context(), id)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !deleted {
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
	rows, err := s.Repos.Groups.List(r.Context())
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	type groupRow struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		CreatedAt   string `json:"createdAt"`
		MemberCount int64  `json:"memberCount"`
		MappedRole  string `json:"mappedRole"`
	}

	groups := make([]groupRow, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, groupRow{
			ID: row.ID, Name: row.Name, Source: row.Source, CreatedAt: row.CreatedAt,
			MemberCount: row.MemberCount, MappedRole: row.MappedRole,
		})
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
	rows, err := s.Repos.Groups.ListMappings(r.Context())
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	type mapping struct {
		GroupName    string `json:"groupName"`
		PlatformRole string `json:"platformRole"`
	}

	mappings := make([]mapping, 0, len(rows))
	for _, row := range rows {
		mappings = append(mappings, mapping{GroupName: row.GroupName, PlatformRole: row.PlatformRole})
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

	if body.PlatformRole != groupsRoleAdmin && body.PlatformRole != groupsRoleStudent && body.PlatformRole != groupsRoleManager {
		s.Error(writer, http.StatusBadRequest, "platformRole must be 'admin', 'manager' or 'student'")

		return
	}

	err = s.Repos.Groups.UpsertMapping(req.Context(), body.GroupName, body.PlatformRole)
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

	deleted, err := s.Repos.Groups.DeleteMapping(r.Context(), name)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !deleted {
		s.Error(writer, http.StatusNotFound, "Mapping not found")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Mapping deleted"})
}
