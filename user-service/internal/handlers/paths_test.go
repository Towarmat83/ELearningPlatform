package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elearning/user-service/fake"
	"github.com/elearning/user-service/internal/config"
)

// ── MyPaths ───────────────────────────────────────────────────────────────────

func TestMyPaths_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(fmt.Errorf("db down"))
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/my/paths", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestMyPaths_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil) // path_enrollments → empty
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/my/paths", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	paths := resp["paths"].([]any)
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestMyPaths_CourseServiceError(t *testing.T) {
	// Enrollment found but course-service unreachable → fallback slug-only entry
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"devops-path", time.Now()})
	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: "http://127.0.0.1:0"},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest("GET", "/api/my/paths", nil)
	req.Header.Set("Authorization", htAuthHeader(t, "student"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Paths []struct {
			Slug string `json:"slug"`
		} `json:"paths"`
	}
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Paths) != 1 || resp.Paths[0].Slug != "devops-path" {
		t.Errorf("expected fallback entry for devops-path, got %v", resp.Paths)
	}
}

func TestMyPaths_WithCourseService(t *testing.T) {
	// Mock course-service returns path detail
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"slug":    "devops-path",
			"title":   "Parcours DevOps",
			"courses": []string{"lab1", "lab2", "lab3"},
		})
	}))
	defer courseSvc.Close()

	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"devops-path", time.Now()}) // path_enrollments
	pool.PushRows(nil, []any{"lab1"})                    // completedCoursesCtx → lab1 completed

	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: courseSvc.URL},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest("GET", "/api/my/paths", nil)
	req.Header.Set("Authorization", htAuthHeader(t, "student"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Paths []struct {
			Slug    string `json:"slug"`
			Title   string `json:"title"`
			Courses []struct {
				Slug   string `json:"slug"`
				Status string `json:"status"`
			} `json:"courses"`
		} `json:"paths"`
	}
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(resp.Paths))
	}
	p := resp.Paths[0]
	if p.Title != "Parcours DevOps" {
		t.Errorf("expected title=Parcours DevOps, got %q", p.Title)
	}
	if len(p.Courses) != 3 {
		t.Fatalf("expected 3 courses, got %d", len(p.Courses))
	}
	// lab1 completed, lab2 available (previous completed), lab3 locked
	if p.Courses[0].Status != "completed" {
		t.Errorf("lab1: want completed, got %q", p.Courses[0].Status)
	}
	if p.Courses[1].Status != "available" {
		t.Errorf("lab2: want available, got %q", p.Courses[1].Status)
	}
	if p.Courses[2].Status != "locked" {
		t.Errorf("lab3: want locked, got %q", p.Courses[2].Status)
	}
}

func TestMyPaths_RequiresAuth(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/my/paths", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// ── AdminListPathEnrollments ──────────────────────────────────────────────────

func TestAdminListPathEnrollments_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(fmt.Errorf("db down"))
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminListPathEnrollments_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil) // no users enrolled
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	users := resp["users"].([]any)
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestAdminListPathEnrollments_WithUsers(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"user-uuid-1", "user@test.com", "student", time.Now()})
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Users []struct {
			Email string `json:"email"`
		} `json:"users"`
	}
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Users) != 1 || resp.Users[0].Email != "user@test.com" {
		t.Errorf("expected 1 user user@test.com, got %v", resp.Users)
	}
}

func TestAdminListPathEnrollments_RequiresAdmin(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── AdminEnrollUserInPath ─────────────────────────────────────────────────────

func TestAdminEnrollUserInPath_MissingUserID(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments", `{}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestAdminEnrollUserInPath_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, fmt.Errorf("db down"))
	r := newTestRouter(pool)
	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"user_id":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminEnrollUserInPath_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"user_id":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminEnrollUserInPath_RequiresAdmin(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"user_id":"user-uuid-1"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── AdminUnenrollUserFromPath ─────────────────────────────────────────────────

func TestAdminUnenrollUserFromPath_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, fmt.Errorf("db down"))
	r := newTestRouter(pool)
	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminUnenrollUserFromPath_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}
