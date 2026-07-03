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

func TestAdminUnenrollUserFromPath_RequiresAdmin(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── fetchPathDetail ───────────────────────────────────────────────────────────

func TestFetchPathDetail_InvalidSlug(t *testing.T) {
	s := &State{Config: &config.Config{CourseServiceURL: "http://localhost"}}
	req := httptest.NewRequest("GET", "/", nil)

	cases := []string{"../etc/passwd", "UPPERCASE", "-leading", "with space", "a/b"}
	for _, slug := range cases {
		if _, err := s.fetchPathDetail(req, slug); err == nil {
			t.Errorf("expected error for slug %q, got nil", slug)
		}
	}
}

func TestFetchPathDetail_ValidSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"slug": "my-path", "title": "My Path", "courses": []string{}}) //nolint:errcheck
	}))
	defer srv.Close()

	s := &State{Config: &config.Config{CourseServiceURL: srv.URL}}
	req := httptest.NewRequest("GET", "/", nil)
	detail, err := s.fetchPathDetail(req, "my-path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Title != "My Path" {
		t.Errorf("want title=My Path, got %q", detail.Title)
	}
}

// ── completedCoursesCtx ───────────────────────────────────────────────────────

func TestCompletedCoursesCtx_EmptySlugs(t *testing.T) {
	s := &State{Pool: &fake.Pool{}}
	req := httptest.NewRequest("GET", "/", nil)
	if result := s.completedCoursesCtx(req, "user-uuid-1", nil); result != nil {
		t.Errorf("expected nil for empty slugs, got %v", result)
	}
}

func TestCompletedCoursesCtx_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(fmt.Errorf("db down"))
	s := &State{Pool: pool}
	req := httptest.NewRequest("GET", "/", nil)
	if result := s.completedCoursesCtx(req, "user-uuid-1", []string{"course-a"}); result != nil {
		t.Errorf("expected nil on DB error, got %v", result)
	}
}

// ── AdminListPathEnrollments with path detail ─────────────────────────────────

func TestAdminListPathEnrollments_WithDetail(t *testing.T) {
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"slug":    "devops-path",
			"title":   "DevOps Path",
			"courses": []string{"linux-intro", "docker-fundamentals"},
		})
	}))
	defer courseSvc.Close()

	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"user-uuid-1", "user@test.com", "student", time.Now()})
	pool.PushRows(nil, []any{"linux-intro"}) // completedCoursesCtx → linux-intro done

	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: courseSvc.URL},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest("GET", "/api/admin/paths/devops-path/enrollments", nil)
	req.Header.Set("Authorization", htAuthHeader(t, "admin"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Users []struct {
			Email            string `json:"email"`
			TotalCourses     int    `json:"total_courses"`
			CompletedCourses int    `json:"completed_courses"`
			Courses          []struct {
				Slug   string `json:"slug"`
				Status string `json:"status"`
			} `json:"courses"`
		} `json:"users"`
	}
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(resp.Users))
	}
	u := resp.Users[0]
	if u.TotalCourses != 2 {
		t.Errorf("want total_courses=2, got %d", u.TotalCourses)
	}
	if u.CompletedCourses != 1 {
		t.Errorf("want completed_courses=1, got %d", u.CompletedCourses)
	}
	if len(u.Courses) != 2 {
		t.Fatalf("want 2 courses, got %d", len(u.Courses))
	}
	if u.Courses[0].Status != "completed" {
		t.Errorf("linux-intro: want completed, got %q", u.Courses[0].Status)
	}
	if u.Courses[1].Status != "available" {
		t.Errorf("docker-fundamentals: want available, got %q", u.Courses[1].Status)
	}
}

// ── parsePagination ──────────────────────────────────────────────────────────

func TestParsePagination(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  *int
		wantOffset int
	}{
		{"omitted means unlimited", "", nil, 0},
		{"zero limit means unlimited", "?limit=0", nil, 0},
		{"negative limit bypasses pagination", "?limit=-1", nil, 0},
		{"negative offset ignored", "?offset=-5", nil, 0},
		{"positive limit and offset applied", "?limit=10&offset=5", intPtr(10), 5},
		{"non-numeric limit ignored", "?limit=abc", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/my/paths"+tc.query, nil)
			gotLimit, gotOffset := parsePagination(req)
			if (gotLimit == nil) != (tc.wantLimit == nil) || (gotLimit != nil && *gotLimit != *tc.wantLimit) {
				t.Errorf("limit: got %v, want %v", gotLimit, tc.wantLimit)
			}
			if gotOffset != tc.wantOffset {
				t.Errorf("offset: got %d, want %d", gotOffset, tc.wantOffset)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
