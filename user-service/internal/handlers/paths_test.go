package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elearning/user-service/fake"
	"github.com/elearning/user-service/internal/config"
)

// ── MyPaths ───────────────────────────────────────────────────────────────────

// TestMyPaths_DBError verifies my paths DB error behavior.
func TestMyPaths_DBError(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushRows(errors.New("db down"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/paths", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// TestMyPaths_Empty verifies my paths empty behavior.
func TestMyPaths_Empty(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushRows(nil) // path_enrollments → empty
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/paths", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	paths := htSliceField(t, resp, "paths")
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

// TestMyPaths_CourseServiceError verifies my paths course service error
// behavior.
func TestMyPaths_CourseServiceError(t *testing.T) {
	t.Parallel()

	// Enrollment found but course-service unreachable → fallback slug-only entry
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"devops-path", time.Now()})
	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: "http://127.0.0.1:0"},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/my/paths", http.NoBody)
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
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Paths) != 1 || resp.Paths[0].Slug != "devops-path" {
		t.Errorf("expected fallback entry for devops-path, got %v", resp.Paths)
	}
}

// TestMyPaths_WithCourseService verifies my paths with course service
// behavior.
func TestMyPaths_WithCourseService(t *testing.T) {
	t.Parallel()

	// Mock course-service returns path detail
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/my/paths", http.NoBody)
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
	json.NewDecoder(rec.Body).Decode(&resp)

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

// TestMyPaths_RequiresAuth verifies my paths requires auth behavior.
func TestMyPaths_RequiresAuth(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/paths", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// ── AdminListPathEnrollments ──────────────────────────────────────────────────

// TestAdminListPathEnrollments_DBError verifies admin list path enrollments
// DB error behavior.
func TestAdminListPathEnrollments_DBError(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushRows(errors.New("db down"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// TestAdminListPathEnrollments_Empty verifies admin list path enrollments
// empty behavior.
func TestAdminListPathEnrollments_Empty(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushRows(nil) // no users enrolled
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	users := htSliceField(t, resp, "users")
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

// TestAdminListPathEnrollments_WithUsers verifies admin list path
// enrollments with users behavior.
func TestAdminListPathEnrollments_WithUsers(t *testing.T) {
	t.Parallel()

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
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Users) != 1 || resp.Users[0].Email != "user@test.com" {
		t.Errorf("expected 1 user user@test.com, got %v", resp.Users)
	}
}

// TestAdminListPathEnrollments_RequiresAdmin verifies admin list path
// enrollments requires admin behavior.
func TestAdminListPathEnrollments_RequiresAdmin(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/paths/devops-path/enrollments", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── AdminEnrollUserInPath ─────────────────────────────────────────────────────

// TestAdminEnrollUserInPath_MissingUserID verifies admin enroll user in path
// missing user ID behavior.
func TestAdminEnrollUserInPath_MissingUserID(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments", `{}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// TestAdminEnrollUserInPath_DBError verifies admin enroll user in path DB
// error behavior.
func TestAdminEnrollUserInPath_DBError(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db down"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"userId":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// TestAdminEnrollUserInPath_Success verifies admin enroll user in path
// success behavior.
func TestAdminEnrollUserInPath_Success(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"userId":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminEnrollUserInPath_RequiresAdmin verifies admin enroll user in path
// requires admin behavior.
func TestAdminEnrollUserInPath_RequiresAdmin(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/paths/devops-path/enrollments",
		`{"userId":"user-uuid-1"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── AdminUnenrollUserFromPath ─────────────────────────────────────────────────

// TestAdminUnenrollUserFromPath_DBError verifies admin unenroll user from
// path DB error behavior.
func TestAdminUnenrollUserFromPath_DBError(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db down"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// TestAdminUnenrollUserFromPath_Success verifies admin unenroll user from
// path success behavior.
func TestAdminUnenrollUserFromPath_Success(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// TestAdminUnenrollUserFromPath_RequiresAdmin verifies admin unenroll user
// from path requires admin behavior.
func TestAdminUnenrollUserFromPath_RequiresAdmin(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/paths/devops-path/enrollments/user-uuid-1", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ── fetchPathDetail ───────────────────────────────────────────────────────────

// TestFetchPathDetail_InvalidSlug verifies fetch path detail invalid slug
// behavior.
func TestFetchPathDetail_InvalidSlug(t *testing.T) {
	t.Parallel()

	s := &State{Config: &config.Config{CourseServiceURL: "http://localhost"}}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	cases := []string{"../etc/passwd", "UPPERCASE", "-leading", "with space", "a/b"}
	for _, slug := range cases {
		_, err := s.fetchPathDetail(req, slug)
		if err == nil {
			t.Errorf("expected error for slug %q, got nil", slug)
		}
	}
}

// TestFetchPathDetail_ValidSlug verifies fetch path detail valid slug
// behavior.
func TestFetchPathDetail_ValidSlug(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"slug": "my-path", "title": "My Path", "courses": []string{}})
	}))
	defer srv.Close()

	s := &State{Config: &config.Config{CourseServiceURL: srv.URL}}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	detail, err := s.fetchPathDetail(req, "my-path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Title != "My Path" {
		t.Errorf("want title=My Path, got %q", detail.Title)
	}
}

// ── completedCoursesCtx ───────────────────────────────────────────────────────

// TestCompletedCoursesCtx_EmptySlugs verifies completed courses ctx empty
// slugs behavior.
func TestCompletedCoursesCtx_EmptySlugs(t *testing.T) {
	t.Parallel()

	s := &State{Pool: &fake.Pool{}}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	if result := s.completedCoursesCtx(req, "user-uuid-1", nil); result != nil {
		t.Errorf("expected nil for empty slugs, got %v", result)
	}
}

// TestCompletedCoursesCtx_DBError verifies completed courses ctx DB error
// behavior.
func TestCompletedCoursesCtx_DBError(t *testing.T) {
	t.Parallel()

	pool := &fake.Pool{}
	pool.PushRows(errors.New("db down"))
	s := &State{Pool: pool}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	if result := s.completedCoursesCtx(req, "user-uuid-1", []string{"course-a"}); result != nil {
		t.Errorf("expected nil on DB error, got %v", result)
	}
}

// ── AdminListPathEnrollments with path detail ─────────────────────────────────

// TestAdminListPathEnrollments_WithDetail verifies admin list path
// enrollments with detail behavior.
func TestAdminListPathEnrollments_WithDetail(t *testing.T) {
	t.Parallel()

	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/admin/paths/devops-path/enrollments", http.NoBody)
	req.Header.Set("Authorization", htAuthHeader(t, "admin"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Users []struct {
			Email            string `json:"email"`
			TotalCourses     int    `json:"totalCourses"`
			CompletedCourses int    `json:"completedCourses"`
			Courses          []struct {
				Slug   string `json:"slug"`
				Status string `json:"status"`
			} `json:"courses"`
		} `json:"users"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(resp.Users))
	}

	u := resp.Users[0]
	if u.TotalCourses != 2 {
		t.Errorf("want totalCourses=2, got %d", u.TotalCourses)
	}

	if u.CompletedCourses != 1 {
		t.Errorf("want completedCourses=1, got %d", u.CompletedCourses)
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

// TestParsePagination verifies parse pagination behavior.
func TestParsePagination(t *testing.T) {
	t.Parallel()

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
		{"positive limit and offset applied", "?limit=10&offset=5", new(10), 5},
		{"non-numeric limit ignored", "?limit=abc", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/my/paths"+tc.query, http.NoBody)

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
