package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/fake"
	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// htManagerID is a fixed UUID used as the manager's subject in manager tests.
const htManagerID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

// htStudentID is a fixed UUID used as a scope-member student in manager tests.
const htStudentID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

// htOutsideID is a fixed UUID for a user outside the manager's scope.
const htOutsideID = "cccccccc-cccc-cccc-cccc-cccccccccccc"

// htGroupID is a fixed group UUID used in manager tests.
const htGroupID = "dddddddd-dddd-dddd-dddd-dddddddddddd"

// seedManagerRepos builds repos with a manager, a scoped student, an outside
// user, and a group containing both the manager and the student.
func seedManagerRepos(t *testing.T) *repository.Repositories {
	t.Helper()

	repos := fake.NewRepositories()

	managerUUID := uuid.MustParse(htManagerID)
	studentUUID := uuid.MustParse(htStudentID)
	outsideUUID := uuid.MustParse(htOutsideID)
	groupUUID := uuid.MustParse(htGroupID)

	repos.Users = fake.NewUserRepository(
		models.User{ID: managerUUID, Username: "manager", Email: "mgr@test.com", Role: "manager", IsActive: true, AuthProvider: "local"},
		models.User{ID: studentUUID, Username: "student", Email: "stu@test.com", Role: "student", IsActive: true, AuthProvider: "local"},
		models.User{ID: outsideUUID, Username: "outside", Email: "out@test.com", Role: "student", IsActive: true, AuthProvider: "local"},
	)

	groups := fake.NewGroupRepository(models.Group{ID: groupUUID, Name: "team-rh", Source: "local"})
	groups.UserGroup = []models.UserGroup{
		{UserID: htManagerID, GroupID: groupUUID},
		{UserID: htStudentID, GroupID: groupUUID},
	}
	repos.Groups = groups

	return repos
}

// ── ManagerListUsers ──────────────────────────────────────────────────────────

// TestManagerListUsers_Unauthorized verifies that missing auth returns 401.
func TestManagerListUsers_Unauthorized(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "GET", "/api/manager/users", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// TestManagerListUsers_ForbiddenStudent verifies that a student is rejected.
func TestManagerListUsers_ForbiddenStudent(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "GET", "/api/manager/users", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestManagerListUsers_ForbiddenAdmin verifies that an admin is rejected by the
// manager middleware.
func TestManagerListUsers_ForbiddenAdmin(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "GET", "/api/manager/users", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestManagerListUsers_Empty verifies 200 with empty lists when the manager
// has no groups.
func TestManagerListUsers_Empty(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "GET", "/api/manager/users", "", htAuthHeader(t, "manager"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["total"] == nil {
		t.Error("expected total in response")
	}
}

// TestManagerListUsers_WithGroups verifies 200 with users when the manager
// belongs to a group.
func TestManagerListUsers_WithGroups(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "GET", "/api/manager/users", "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if _, ok := resp["groups"]; !ok {
		t.Error("expected groups in response")
	}
}

// TestManagerListUsers_DBError verifies 500 when group lookup fails.
func TestManagerListUsers_DBError(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	repos.Groups = &fake.GroupRepository{Err: errors.New("db down")}
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "GET", "/api/manager/users", "", htAuthHeader(t, "manager"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── ManagerEnrollUser ─────────────────────────────────────────────────────────

// TestManagerEnrollUser_MissingUserID verifies 400 when userId is absent.
func TestManagerEnrollUser_MissingUserID(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "POST", "/api/manager/courses/my-course/enrollments", `{"userId":""}`, htAuthHeader(t, "manager"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// TestManagerEnrollUser_InvalidJSON verifies 400 for malformed body.
func TestManagerEnrollUser_InvalidJSON(t *testing.T) {
	t.Parallel()

	r := newTestRouter()

	rec := htDo(t, r, "POST", "/api/manager/courses/my-course/enrollments", "not-json", htAuthHeader(t, "manager"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// TestManagerEnrollUser_OutOfScope verifies 403 when target user is not in any
// of the manager's groups.
func TestManagerEnrollUser_OutOfScope(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	body := `{"userId":"` + htOutsideID + `"}`

	rec := htDo(t, r, "POST", "/api/manager/courses/my-course/enrollments", body, htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 (out of scope), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestManagerEnrollUser_Success verifies 200 when target user is in scope.
func TestManagerEnrollUser_Success(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	body := `{"userId":"` + htStudentID + `"}`

	rec := htDo(t, r, "POST", "/api/manager/courses/my-course/enrollments", body, htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestManagerEnrollUser_DBError verifies 500 when group repo fails.
func TestManagerEnrollUser_DBError(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	repos.Groups = &fake.GroupRepository{Err: errors.New("db down")}
	r := newTestRouterWithRepos(repos)

	body := `{"userId":"` + htStudentID + `"}`

	rec := htDo(t, r, "POST", "/api/manager/courses/my-course/enrollments", body, htAuthHeader(t, "manager"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── ManagerUnenrollUser ───────────────────────────────────────────────────────

// TestManagerUnenrollUser_OutOfScope verifies 403 when target user is not in
// scope.
func TestManagerUnenrollUser_OutOfScope(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "DELETE", "/api/manager/courses/my-course/enrollments/"+htOutsideID, "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestManagerUnenrollUser_Success verifies 200 when target user is in scope.
func TestManagerUnenrollUser_Success(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "DELETE", "/api/manager/courses/my-course/enrollments/"+htStudentID, "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestManagerUnenrollUser_DBErrorGroups verifies 500 when group lookup fails.
func TestManagerUnenrollUser_DBErrorGroups(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	repos.Groups = &fake.GroupRepository{Err: errors.New("db down")}
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "DELETE", "/api/manager/courses/my-course/enrollments/"+htStudentID, "", htAuthHeader(t, "manager"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// TestManagerUnenrollUser_DBErrorEnrollment verifies 500 when the enrollment
// delete fails.
func TestManagerUnenrollUser_DBErrorEnrollment(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	enrollments := fake.NewEnrollmentRepository()
	enrollments.Err = errors.New("db down")
	repos.Enrollments = enrollments
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "DELETE", "/api/manager/courses/my-course/enrollments/"+htStudentID, "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── ManagerGetUserEnrollments ─────────────────────────────────────────────────

// TestManagerGetUserEnrollments_OutOfScope verifies 403 when target user is
// not in scope.
func TestManagerGetUserEnrollments_OutOfScope(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "GET", "/api/manager/users/"+htOutsideID+"/enrollments", "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestManagerGetUserEnrollments_Success verifies 200 when target user is in
// scope.
func TestManagerGetUserEnrollments_Success(t *testing.T) {
	t.Parallel()

	repos := seedManagerRepos(t)
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "GET", "/api/manager/users/"+htStudentID+"/enrollments", "", htAuthHeaderForSubject(t, "manager", htManagerID))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestManagerGetUserEnrollments_DBError verifies 500 when group lookup fails.
func TestManagerGetUserEnrollments_DBError(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	repos.Groups = &fake.GroupRepository{Err: errors.New("db down")}
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, "GET", "/api/manager/users/"+htStudentID+"/enrollments", "", htAuthHeader(t, "manager"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}
