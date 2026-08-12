package handlers

import (
	"net/http"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// defaultGroupName is the platform-wide group every user belongs to;
// it must not count as a manager scope group.
const defaultGroupName = "everyone"

// groupInfo holds the id, name and member list of a manager's scope group.
type groupInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	MemberIDs []string `json:"memberIds"`
}

// managerScopeIDs returns the IDs of a manager's groups, excluding the
// platform-wide default group which should not define scope.
func managerScopeIDs(groups []repository.GroupRow) []string {
	ids := make([]string, 0, len(groups))

	for _, grp := range groups {
		if grp.Name != defaultGroupName {
			ids = append(ids, grp.ID)
		}
	}

	return ids
}

// ManagerListUsers godoc
// @Summary   List users in the manager's groups
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/manager/users [get].
func (s *State) ManagerListUsers(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	users, err := s.Repos.Users.ListByGroupIDs(ctx, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	scopeGroups := make([]groupInfo, 0, len(groups))

	for _, grp := range groups {
		if grp.Name == defaultGroupName {
			continue
		}

		memberIDs, _ := s.Repos.Groups.ListMemberIDs(ctx, grp.ID)
		if memberIDs == nil {
			memberIDs = []string{}
		}

		scopeGroups = append(scopeGroups, groupInfo{ID: grp.ID, Name: grp.Name, MemberIDs: memberIDs})
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		adminJSONKeyUsers:   users,
		adminJSONKeyTotal:   len(users),
		groupsRespKeyGroups: scopeGroups,
	})
}

// ManagerEnrollUser godoc
// @Summary   Enroll a user in a course (manager, scope-restricted)
// @Tags      Manager
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path    string  true  "Course slug"
// @Param     body  object  true    "userId (UUID)"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Failure   403   {object}  map[string]string
// @Router    /api/manager/courses/{slug}/enrollments [post].
func (s *State) ManagerEnrollUser(writer http.ResponseWriter, request *http.Request) { //nolint:dupl // same scope check as ManagerEnrollUserPath; different repository (Enrollments vs Paths)
	ctx := request.Context()
	slug := param(request, "slug")
	claims := s.claims(request)

	var body struct {
		UserID string `json:"userId"`
	}

	err := decode(request, &body)
	if err != nil || body.UserID == "" {
		s.Error(writer, http.StatusBadRequest, "userId is required")

		return
	}

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, body.UserID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	err = s.Repos.Enrollments.Create(ctx, body.UserID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: adminMsgUserEnrolled})
}

// ManagerGetUserEnrollments godoc
// @Summary   Get enrollments of a user in the manager's scope
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Param     userId  path  string  true  "User UUID"
// @Success   200     {object}  map[string]interface{}
// @Failure   403     {object}  map[string]string
// @Router    /api/manager/users/{userId}/enrollments [get].
func (s *State) ManagerGetUserEnrollments(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	userID := param(request, "userId")
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, userID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	enrollments, err := s.Repos.Enrollments.MyEnrollments(ctx, userID)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{adminJSONKeyEnrollments: enrollments})
}

// ManagerUnenrollUser godoc
// @Summary   Unenroll a user from a course (manager, scope-restricted)
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Param     slug    path  string  true  "Course slug"
// @Param     userId  path  string  true  "User UUID"
// @Success   200     {object}  map[string]string
// @Failure   403     {object}  map[string]string
// @Router    /api/manager/courses/{slug}/enrollments/{userId} [delete].
func (s *State) ManagerUnenrollUser(writer http.ResponseWriter, request *http.Request) { //nolint:dupl // same scope check as ManagerUnenrollUserPath; different repository (Enrollments vs Paths)
	ctx := request.Context()
	slug := param(request, "slug")
	userID := param(request, "userId")
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, userID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	err = s.Repos.Enrollments.Delete(ctx, userID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: adminMsgUserUnenrolled})
}

// ManagerGetUserPathEnrollments godoc
// @Summary   Get path enrollments of a user in the manager's scope
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Param     userId  path  string  true  "User UUID"
// @Success   200     {object}  map[string]interface{}
// @Failure   403     {object}  map[string]string
// @Router    /api/manager/users/{userId}/path-enrollments [get].
func (s *State) ManagerGetUserPathEnrollments(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	userID := param(request, "userId")
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, userID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	enrollments, err := s.Repos.Paths.MyEnrollments(ctx, userID, nil, 0)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if enrollments == nil {
		enrollments = []repository.PathEnrollmentRow{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"pathEnrollments": enrollments})
}

// ManagerEnrollUserPath godoc
// @Summary   Enroll a user in a learning path (manager, scope-restricted)
// @Tags      Manager
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path    string  true  "Path slug"
// @Param     body  object  true    "userId (UUID)"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Failure   403   {object}  map[string]string
// @Router    /api/manager/paths/{slug}/enrollments [post].
func (s *State) ManagerEnrollUserPath(writer http.ResponseWriter, request *http.Request) { //nolint:dupl // same scope check as ManagerEnrollUser; different repository (Paths vs Enrollments)
	ctx := request.Context()
	slug := param(request, "slug")
	claims := s.claims(request)

	var body struct {
		UserID string `json:"userId"`
	}

	err := decode(request, &body)
	if err != nil || body.UserID == "" {
		s.Error(writer, http.StatusBadRequest, "userId is required")

		return
	}

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, body.UserID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	err = s.Repos.Paths.Enroll(ctx, body.UserID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: adminMsgUserEnrolled})
}

// ManagerUnenrollUserPath godoc
// @Summary   Unenroll a user from a learning path (manager, scope-restricted)
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Param     slug    path  string  true  "Path slug"
// @Param     userId  path  string  true  "User UUID"
// @Success   200     {object}  map[string]string
// @Failure   403     {object}  map[string]string
// @Router    /api/manager/paths/{slug}/enrollments/{userId} [delete].
func (s *State) ManagerUnenrollUserPath(writer http.ResponseWriter, request *http.Request) { //nolint:dupl // same scope check as ManagerUnenrollUser; different repository (Paths vs Enrollments)
	ctx := request.Context()
	slug := param(request, "slug")
	userID := param(request, "userId")
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	inScope, err := s.Repos.Groups.UserInAnyGroup(ctx, userID, groupIDs)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if !inScope {
		s.Error(writer, http.StatusForbidden, "User not in manager scope")

		return
	}

	err = s.Repos.Paths.Unenroll(ctx, userID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: adminMsgUserUnenrolled})
}
