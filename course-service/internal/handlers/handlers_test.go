package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/content"
	apimiddleware "github.com/elearning/course-service/internal/middleware"
)

func newTestState(t *testing.T, userSrv *httptest.Server) *State {
	store := content.NewStore()

	store.Put(&content.Course{
		Slug:        "kubernetes-basics",
		Title:       "Kubernetes Basics",
		Description: "Intro to Kubernetes",
		Category:    "kubernetes",
		Difficulty:  "beginner",
		IsPublic:    true,
		Modules: []content.Module{
			{Name: "What is K8s", Type: "text"},
			{Name: "Architecture", Type: "video", Src: "/uploads/arch.mp4"},
			{Name: "Pods Explained", Type: "image", Src: "/uploads/pods.png"},
		},
	})

	store.Put(&content.Course{
		Slug:        "docker-fundamentals",
		Title:       "Docker Fundamentals",
		Description: "Learn Docker from scratch",
		Category:    "docker",
		Difficulty:  "beginner",
		IsPublic:    false,
	})

	store.Put(&content.Course{
		Slug:        "advanced-kubernetes",
		Title:       "Advanced Kubernetes",
		Description: "Deep dive into K8s networking and scheduling",
		Category:    "kubernetes",
		Difficulty:  "advanced",
		IsPublic:    true,
	})

	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTExpiryH:         24,
		UploadsDir:         "./testdata",
		K8sNamespace:       "default",
		UserServiceURL:     userSrv.URL,
		GitCredentialsPath: "",
	}

	return NewState(cfg, store)
}

// newUserServiceMock returns a test server with sensible defaults for all
// internal user-service endpoints used by the course-service.
func newUserServiceMock() *httptest.Server {
	return newUserServiceMockWith(nil)
}

// newUserServiceMockWith returns a test server where the caller can override
// individual path handlers via the overrides map.  Any path not overridden
// falls back to the standard default response.
func newUserServiceMockWith(overrides map[string]http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fn, ok := overrides[r.URL.Path]; ok {
			fn(w, r)
			return
		}
		switch r.URL.Path {
		case "/internal/enrollments/auto":
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/internal/enrollments/check":
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": true})
		case "/internal/progress/viewed":
			json.NewEncoder(w).Encode(map[string]any{"viewed": []string{}})
		case "/internal/progress/complete":
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/internal/progress/modules":
			json.NewEncoder(w).Encode(map[string]any{"progress": []any{}})
		case "/internal/progress/course-summary":
			json.NewEncoder(w).Encode(map[string]any{
				"total_score":    0,
				"passed_modules": []string{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func authHeader(t *testing.T, secret string) string {
	token, err := apimiddleware.CreateToken(
		"00000000-0000-0000-0000-000000000001",
		"test@test.com",
		"student",
		secret,
		24,
	)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return "Bearer " + token
}

func adminAuthHeader(t *testing.T, secret string) string {
	token, err := apimiddleware.CreateToken(
		"00000000-0000-0000-0000-000000000000",
		"admin@test.com",
		"admin",
		secret,
		24,
	)
	if err != nil {
		t.Fatalf("create admin token: %v", err)
	}
	return "Bearer " + token
}

func TestHealth(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
	if body["service"] != "course-service" {
		t.Errorf("expected service=course-service, got %s", body["service"])
	}
}

func TestListCourses(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Courses []courseResponse `json:"courses"`
		Total   int              `json:"total"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Total != 2 {
		t.Fatalf("expected 2 published courses, got %d", resp.Total)
	}

	slugs := make(map[string]bool)
	for _, c := range resp.Courses {
		slugs[c.Slug] = true
	}
	if !slugs["kubernetes-basics"] {
		t.Error("expected kubernetes-basics in list")
	}
	if !slugs["advanced-kubernetes"] {
		t.Error("expected advanced-kubernetes in list")
	}
	if slugs["docker-fundamentals"] {
		t.Error("docker-fundamentals is unpublished, should not appear")
	}
}

func TestListCoursesFiltered(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses?category=kubernetes", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var resp struct {
		Courses []courseResponse `json:"courses"`
		Total   int              `json:"total"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Total != 2 {
		t.Fatalf("expected 2 kubernetes courses, got %d", resp.Total)
	}
	slugs := make(map[string]bool)
	for _, c := range resp.Courses {
		slugs[c.Slug] = true
	}
	if !slugs["kubernetes-basics"] {
		t.Error("expected kubernetes-basics in filtered results")
	}
	if !slugs["advanced-kubernetes"] {
		t.Error("expected advanced-kubernetes in filtered results")
	}
}

func TestGetCourse(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var c courseResponse
	json.NewDecoder(rec.Body).Decode(&c)

	if c.Slug != "kubernetes-basics" {
		t.Errorf("expected slug=kubernetes-basics, got %s", c.Slug)
	}
	if c.ModuleCount != 3 {
		t.Errorf("expected 3 modules, got %d", c.ModuleCount)
	}
	if c.Category != "kubernetes" {
		t.Errorf("expected category=kubernetes, got %s", c.Category)
	}
}

func TestGetCourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetCourseUnpublished(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unpublished course, got %d", rec.Code)
	}
}

func TestModulesListed(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/modules", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Modules []moduleResponse `json:"modules"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Modules) != 3 {
		t.Fatalf("expected 3 modules, got %d", len(resp.Modules))
	}

	expected := []struct {
		name string
		typ  string
	}{
		{"What is K8s", "text"},
		{"Architecture", "video"},
		{"Pods Explained", "image"},
	}
	for i, e := range expected {
		if resp.Modules[i].Name != e.name {
			t.Errorf("module %d: expected name %s, got %s", i, e.name, resp.Modules[i].Name)
		}
		if resp.Modules[i].Type != e.typ {
			t.Errorf("module %d: expected type %s, got %s", i, e.typ, resp.Modules[i].Type)
		}
		if resp.Modules[i].Slug == "" {
			t.Errorf("module %d: expected non-empty slug", i)
		}
	}
}

func TestModulesAuthRequired(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/modules", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestAdminBypassUnpublishedModule(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/modules", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin on unpublished, got %d", rec.Code)
	}
}

func TestGetModuleContent(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/modules/1", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Type != "video" {
		t.Errorf("expected type video, got %s", resp.Type)
	}
	if resp.Content != "/uploads/arch.mp4" {
		t.Errorf("expected content /uploads/arch.mp4, got %s", resp.Content)
	}
}

func TestGetModuleNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/modules/99", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLessonsListedFromModules(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/lessons", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Lessons []lessonSummary `json:"lessons"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Lessons) != 3 {
		t.Fatalf("expected 3 lessons from modules, got %d", len(resp.Lessons))
	}

	expected := []string{"what-is-k8s", "architecture", "pods-explained"}
	for i, slug := range expected {
		if resp.Lessons[i].Slug != slug {
			t.Errorf("lesson %d: expected slug %s, got %s", i, slug, resp.Lessons[i].Slug)
		}
	}
}

func TestLessonsAuthRequired(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/lessons", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestLessonComplete(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/kubernetes-basics/lessons/what-is-k8s/complete", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Message string `json:"message"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Message != "Lesson marked as complete" {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}

func TestMetrics(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	var lastBody string
	for _, path := range []string{"/metrics", "/health", "/api/courses"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		lastBody = rec.Body.String()
		_ = lastBody
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "elearning_active_courses_total") {
		t.Error("expected elearning_active_courses_total metric")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("expected go_goroutines metric")
	}
}

// ── Helpers for cross-course lock tests ─────────────────────────────────────

// courseSummaryHandler returns an HTTP handler that serves a fixed
// /internal/progress/course-summary response.
func courseSummaryHandler(totalScore int, passedModules []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"total_score":    totalScore,
			"passed_modules": passedModules,
		})
	}
}

// newCrossCourseLockState returns a State seeded with:
//   - "linux-intro"      — no prerequisites (the prerequisite course)
//   - "kubernetes-adv"   — requires linux-intro with min_score=30 AND module "quiz-bases-linux"
//   - "network-basics"   — requires linux-intro with min_score=50 only (no specific module)
//   - "free-course"      — requires linux-intro with no conditions (any progress)
func newCrossCourseLockState(t *testing.T, userSrv *httptest.Server) *State {
	t.Helper()
	store := content.NewStore()

	store.Put(&content.Course{
		Slug:     "linux-intro",
		Title:    "Intro Linux",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "What is Linux", Type: "text"},
			{
				Name: "Quiz Bases Linux",
				Type: "quiz",
				Questions: []content.Question{
					{ID: "q1", Type: "single", Points: 1,
						Question: "Q?",
						Answers:  []content.Answer{{ID: "a", Text: "A", Correct: true}}},
				},
				PassingScore: 1,
			},
		},
	})

	store.Put(&content.Course{
		Slug:     "kubernetes-adv",
		Title:    "Advanced Kubernetes",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "linux-intro", MinScore: 30, Modules: []string{"quiz-bases-linux"}},
		},
		Modules: []content.Module{
			{Name: "K8s Networking", Type: "text"},
		},
	})

	store.Put(&content.Course{
		Slug:     "network-basics",
		Title:    "Network Basics",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "linux-intro", MinScore: 50},
		},
		Modules: []content.Module{
			{Name: "OSI Model", Type: "text"},
		},
	})

	store.Put(&content.Course{
		Slug:     "free-course",
		Title:    "Free Course (any linux-intro progress)",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "linux-intro"},
		},
		Modules: []content.Module{
			{Name: "Intro", Type: "text"},
		},
	})

	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UserServiceURL: userSrv.URL,
	}
	return NewState(cfg, store)
}

// ── Cross-course lock: ListModules ────────────────────────────────────────────

// TestCrossCourseLock_ListModules_Blocked verifies that a student who has NOT
// completed the prerequisite course receives 403 on ListModules.
func TestCrossCourseLock_ListModules_Blocked(t *testing.T) {
	mock := newUserServiceMockWith(nil) // course-summary returns 0 score by default
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when prereq not met, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossCourseLock_ListModules_AdminBypass verifies that an admin can always
// list modules regardless of cross-course prerequisites.
func TestCrossCourseLock_ListModules_AdminBypass(t *testing.T) {
	mock := newUserServiceMockWith(nil)
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("admin should bypass prereq check, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossCourseLock_ListModules_MetScoreAndModule verifies that a student who
// has both the required score AND the required module passed is allowed through.
func TestCrossCourseLock_ListModules_MetScoreAndModule(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": courseSummaryHandler(35, []string{"quiz-bases-linux"}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when prereq met, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossCourseLock_ScoreMetButModuleMissing verifies that meeting the score
// threshold alone is not enough when specific modules are also required.
func TestCrossCourseLock_ScoreMetButModuleMissing(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		// Score is enough but the required module slug is absent.
		"/internal/progress/course-summary": courseSummaryHandler(40, []string{}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when required module not passed, got %d", rec.Code)
	}
}

// TestCrossCourseLock_MinScoreOnly_Met verifies a course that only requires a
// minimum score (no specific modules) unlocks when the threshold is reached.
func TestCrossCourseLock_MinScoreOnly_Met(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": courseSummaryHandler(50, []string{}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/network-basics/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when min_score met, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossCourseLock_MinScoreOnly_NotMet verifies that a score below the
// threshold is rejected.
func TestCrossCourseLock_MinScoreOnly_NotMet(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": courseSummaryHandler(20, []string{}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/network-basics/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when score below threshold, got %d", rec.Code)
	}
}

// TestCrossCourseLock_AnyProgress_Met verifies that a course that only requires
// "any progress" in the prereq is unlocked when total_score > 0.
func TestCrossCourseLock_AnyProgress_Met(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": courseSummaryHandler(1, []string{}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/free-course/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when any progress exists, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossCourseLock_AnyProgress_NotMet verifies that zero progress on the
// prereq course blocks access even when no specific score/module is required.
func TestCrossCourseLock_AnyProgress_NotMet(t *testing.T) {
	mock := newUserServiceMockWith(nil) // returns total_score=0 by default
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/free-course/modules", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when no progress in prereq, got %d", rec.Code)
	}
}

// ── Cross-course lock: GetModule ──────────────────────────────────────────────

// TestCrossCourseLock_GetModule_Blocked verifies that GetModule also enforces
// cross-course prerequisites (not just ListModules).
func TestCrossCourseLock_GetModule_Blocked(t *testing.T) {
	mock := newUserServiceMockWith(nil)
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on GetModule when prereq not met, got %d", rec.Code)
	}
}

// TestCrossCourseLock_GetModule_Met verifies that GetModule succeeds once the
// cross-course prerequisite is satisfied.
func TestCrossCourseLock_GetModule_Met(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": courseSummaryHandler(35, []string{"quiz-bases-linux"}),
	})
	defer mock.Close()
	s := newCrossCourseLockState(t, mock)

	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-adv/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GetModule when prereq met, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── ListAdminCourses ──────────────────────────────────────────────────────────

func TestListAdminCourses(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/admin/courses", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Courses []courseResponse `json:"courses"`
		Total   int              `json:"total"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	// All 3 courses including private docker-fundamentals
	if resp.Total != 3 {
		t.Errorf("ListAdminCourses: expected 3 courses, got %d", resp.Total)
	}
}

func TestListAdminCourses_Unauthorized(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/admin/courses", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

// ── Labs ──────────────────────────────────────────────────────────────────────

func TestListLabs(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Labs []labResponse `json:"labs"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Labs) != 3 {
		t.Errorf("expected 3 labs, got %d", len(resp.Labs))
	}
	if resp.Labs[0].CourseID != "kubernetes-basics" {
		t.Errorf("expected CourseID=kubernetes-basics, got %q", resp.Labs[0].CourseID)
	}
}

func TestListLabs_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/no-such-course/labs", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetLab_Found(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// "Architecture" module → slug "architecture"
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lab labResponse `json:"lab"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Lab.Title != "Architecture" {
		t.Errorf("expected title=Architecture, got %q", resp.Lab.Title)
	}
	if resp.Lab.LabType != "interactive" {
		t.Errorf("expected lab_type=interactive for video, got %q", resp.Lab.LabType)
	}
}

func TestGetLab_TextType(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// "What is K8s" text module → slug "what-is-k8s"
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs/what-is-k8s", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for text lab, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lab labResponse `json:"lab"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Lab.LabType != "form" {
		t.Errorf("expected lab_type=form for text module, got %q", resp.Lab.LabType)
	}
}

func TestGetLab_NotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs/no-such-lab", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetLab_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/no-such-course/labs/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ── GetCourseProgress ─────────────────────────────────────────────────────────

func TestGetCourseProgress(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/progress", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["course_id"] != "kubernetes-basics" {
		t.Errorf("expected course_id=kubernetes-basics, got %v", resp["course_id"])
	}
	if resp["total_labs"] == nil {
		t.Error("expected total_labs in response")
	}
}

func TestGetCourseProgress_UnknownCourse(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/unknown-course/progress", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (placeholder), got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	// total_labs = 0 for unknown course
	if resp["total_labs"] != float64(0) {
		t.Errorf("expected total_labs=0 for unknown course, got %v", resp["total_labs"])
	}
}

// ── Cache endpoints ───────────────────────────────────────────────────────────

func TestClearCache(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ClearCache: expected 200, got %d", rec.Code)
	}
}

func TestClearCourseCache_Found(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ClearCourseCache: expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
}

func TestClearCourseCache_NotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/nonexistent/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("ClearCourseCache: expected 404 for missing course, got %d", rec.Code)
	}
}

func TestClearModuleCache_Found(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/modules/0/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ClearModuleCache: expected 200, got %d", rec.Code)
	}
}

func TestClearModuleCache_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/nosuchcourse/modules/0/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("ClearModuleCache: expected 404, got %d", rec.Code)
	}
}

func TestClearModuleCache_InvalidIndex(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/modules/99/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("ClearModuleCache: expected 404 for invalid index, got %d", rec.Code)
	}
}

// ── ServeUpload ───────────────────────────────────────────────────────────────

func TestServeUpload_InvalidFilename(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/uploads/../../etc/passwd", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Chi router sanitizes path traversal, resulting in /uploads/etc/passwd
	// which doesn't exist → 404, OR the handler catches ".." → 400
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("ServeUpload traversal: expected 400 or 404, got %d", rec.Code)
	}
}

func TestServeUpload_ValidFile(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/uploads/sample.txt", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// sample.txt exists in testdata, so we expect 200
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeUpload valid file: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── GetLesson ─────────────────────────────────────────────────────────────────

func TestGetLesson_Found(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// "Architecture" video module
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/lessons/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetLesson: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lesson lessonDetail `json:"lesson"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Lesson.Title != "Architecture" {
		t.Errorf("GetLesson: expected title=Architecture, got %q", resp.Lesson.Title)
	}
}

func TestGetLesson_NotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/lessons/no-such-lesson", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GetLesson: expected 404, got %d", rec.Code)
	}
}

func TestGetLesson_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/no-course/lessons/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GetLesson course not found: expected 404, got %d", rec.Code)
	}
}

func TestGetLesson_NonPublicEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": true})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// docker-fundamentals is non-public, no modules → lesson not found
	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/lessons/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GetLesson non-public enrolled: expected 404, got %d", rec.Code)
	}
}

func TestGetLesson_NonPublicNotEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": false})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/lessons/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("GetLesson non-public unenrolled: expected 403, got %d", rec.Code)
	}
}

// ── ListLessons for non-public course ─────────────────────────────────────────

func TestListLessons_NonPublicEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": true})
		},
		"/internal/progress/viewed": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"viewed": []string{}})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/lessons", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListLessons enrolled: expected 200, got %d", rec.Code)
	}
}

func TestListLessons_NonPublicNotEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": false})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/lessons", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("ListLessons not enrolled: expected 403, got %d", rec.Code)
	}
}

// ── SubmitModule ──────────────────────────────────────────────────────────────

func TestSubmitModule_NotAQuiz(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// Module 0 is text (not quiz)
	req := httptest.NewRequest("POST", "/api/courses/kubernetes-basics/modules/0/submit", strings.NewReader(`{}`))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("SubmitModule non-quiz: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitModule_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/no-such/modules/0/submit", strings.NewReader(`{}`))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("SubmitModule course not found: expected 404, got %d", rec.Code)
	}
}

func TestSubmitModule_InvalidIndex(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/kubernetes-basics/modules/99/submit", strings.NewReader(`{}`))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("SubmitModule invalid index: expected 404, got %d", rec.Code)
	}
}

// ── Utility function unit tests ───────────────────────────────────────────────

// ── Additional GetModule paths ────────────────────────────────────────────────

func TestGetModule_CourseNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/nonexistent/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetModule_PrivateNotEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": false})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for private not enrolled, got %d", rec.Code)
	}
}

func TestGetModule_ImageType(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	// Module index 2 is image type in kubernetes-basics
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/modules/2", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "image" {
		t.Errorf("expected type=image, got %q", resp.Type)
	}
}

// ── Additional ListModules paths ──────────────────────────────────────────────

func TestListModules_PrivateNotEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": false})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals/modules", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// ── Additional MarkLessonComplete paths ──────────────────────────────────────

func TestMarkLessonComplete_PrivateNotEnrolled(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/enrollments/check": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": false})
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/docker-fundamentals/lessons/intro/complete", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestMarkLessonComplete_LessonNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/kubernetes-basics/lessons/nonexistent/complete", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMarkLessonComplete_UserServiceError(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/complete": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/kubernetes-basics/lessons/what-is-k8s/complete", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 from user-service error, got %d", rec.Code)
	}
}

// ── ListLessons from c.Lessons ────────────────────────────────────────────────

func TestListLessons_FromLessonsList(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "lesson-course",
		Title:    "Lesson Course",
		IsPublic: true,
		Lessons: []content.Lesson{
			{Slug: "intro", Title: "Introduction", Order: 1},
			{Slug: "advanced", Title: "Advanced", Order: 2},
		},
	})
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UserServiceURL: mock.URL,
	}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/lesson-course/lessons", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lessons []lessonSummary `json:"lessons"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Lessons) != 2 {
		t.Errorf("expected 2 lessons, got %d", len(resp.Lessons))
	}
}

// ── GetLesson additional paths ────────────────────────────────────────────────

func TestGetLesson_QuizModule(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// Module index 0 of quiz-course is a quiz type - GetLesson should redirect it
	req := httptest.NewRequest("GET", "/api/courses/quiz-course/lessons/inline-quiz", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for quiz module via lessons, got %d", rec.Code)
	}
}

func TestGetLesson_FromLessonList(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "lesson-course2",
		Title:    "Lesson Course 2",
		IsPublic: true,
		Lessons: []content.Lesson{
			{Slug: "intro", Title: "Introduction", Order: 1, Content: "## Intro\n\nHello"},
		},
	})
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UserServiceURL: mock.URL,
	}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/lesson-course2/lessons/intro", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── NewState with credential path ─────────────────────────────────────────────

func TestNewState_InvalidCredentialsPath(t *testing.T) {
	store := content.NewStore()
	cfg := &config.Config{
		JWTSecret:          "test-secret",
		GitCredentialsPath: "/nonexistent/git-creds.json",
	}
	// Should not panic even when credentials file doesn't exist
	s := NewState(cfg, store)
	if s == nil {
		t.Error("expected non-nil State")
	}
}

// ── GetModule with course prerequisites not met ───────────────────────────────

func TestGetModule_PrerequisitesNotMet(t *testing.T) {
	mock := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"total_score":    0,
				"viewed_count":   0,
				"passed_modules": []string{},
			})
		},
	})
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "prereq-course",
		Title:    "Prereq Course",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "basic-course", MinScore: 100},
		},
		Modules: []content.Module{
			{Name: "Advanced Lesson", Type: "text"},
		},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24, UserServiceURL: mock.URL}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/prereq-course/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 when prerequisites not met, got %d", rec.Code)
	}
}

func TestTokenForRepo_NoCredentials(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	// GitCreds is nil (no credentials path set) → returns config.GitToken
	tok := s.tokenForRepo("https://github.com/org/repo")
	if tok != "" {
		t.Errorf("expected empty token (GitToken not set), got %q", tok)
	}
}

func TestTokenForRepo_WithConfigToken(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	s.Config.GitToken = "my-git-token"
	tok := s.tokenForRepo("https://github.com/org/repo")
	if tok != "my-git-token" {
		t.Errorf("expected my-git-token, got %q", tok)
	}
}

func TestDecode_ValidJSON(t *testing.T) {
	var body struct{ Name string }
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
	if err := decode(req, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "test" {
		t.Errorf("expected name=test, got %q", body.Name)
	}
}

func TestDecode_InvalidJSON(t *testing.T) {
	var body struct{ Name string }
	req := httptest.NewRequest("POST", "/", strings.NewReader("not-json"))
	if err := decode(req, &body); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNullStr_Empty(t *testing.T) {
	if nullStr("") != nil {
		t.Error("expected nil for empty string")
	}
}

func TestNullStr_NonEmpty(t *testing.T) {
	s := nullStr("hello")
	if s == nil || *s != "hello" {
		t.Errorf("expected pointer to 'hello', got %v", s)
	}
}

func TestDerefStr_Nil(t *testing.T) {
	if derefStr(nil) != "" {
		t.Error("expected empty string for nil pointer")
	}
}

func TestDerefStr_NonNil(t *testing.T) {
	s := "world"
	if derefStr(&s) != "world" {
		t.Errorf("expected 'world', got %q", derefStr(&s))
	}
}

func TestSanitizeQuestions_NonAdmin(t *testing.T) {
	qq := []content.Question{
		{
			ID:   "q1",
			Type: "single",
			Answers: []content.Answer{
				{ID: "a1", Text: "Right answer", Correct: true},
				{ID: "a2", Text: "Wrong answer", Correct: false},
			},
		},
	}
	out := sanitizeQuestions(qq, false)
	if len(out) != 1 {
		t.Fatalf("expected 1 question, got %d", len(out))
	}
	pq := out[0].(publicQuestion)
	for _, a := range pq.Answers {
		if a.Correct {
			t.Error("non-admin should not see Correct=true answers")
		}
	}
}

func TestSanitizeQuestions_Admin(t *testing.T) {
	qq := []content.Question{
		{
			ID:   "q1",
			Type: "single",
			Answers: []content.Answer{
				{ID: "a1", Text: "Right answer", Correct: true},
				{ID: "a2", Text: "Wrong answer", Correct: false},
			},
		},
	}
	out := sanitizeQuestions(qq, true)
	pq := out[0].(publicQuestion)
	found := false
	for _, a := range pq.Answers {
		if a.Correct {
			found = true
		}
	}
	if !found {
		t.Error("admin should see at least one Correct=true answer")
	}
}

func TestSanitizeQuestions_Empty(t *testing.T) {
	out := sanitizeQuestions(nil, false)
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %d", len(out))
	}
}

func TestCompletedSlugs_ViewedAndPassed(t *testing.T) {
	modules := []content.Module{
		{Name: "Intro", Type: "text"},    // slug: intro
		{Name: "Quiz One", Type: "quiz"}, // slug: quiz-one
	}
	viewedMap := map[string]bool{"intro": true}
	progressMap := map[int]moduleProgressData{
		1: {Passed: true, BestScore: 80, MaxScore: 100},
	}
	result := completedSlugs(modules, viewedMap, progressMap)
	if !result["intro"] {
		t.Error("expected 'intro' in completed slugs from viewedMap")
	}
	if !result["quiz-one"] {
		t.Error("expected 'quiz-one' in completed slugs from progressMap")
	}
}

func TestCompletedSlugs_NotPassed(t *testing.T) {
	modules := []content.Module{
		{Name: "Quiz One", Type: "quiz"},
	}
	progressMap := map[int]moduleProgressData{
		0: {Passed: false, BestScore: 40},
	}
	result := completedSlugs(modules, map[string]bool{}, progressMap)
	if result["quiz-one"] {
		t.Error("failed quiz should not appear in completed slugs")
	}
}

func TestCompletedSlugs_Empty(t *testing.T) {
	result := completedSlugs(nil, map[string]bool{}, nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestIsLocked_AllPrereqsDone(t *testing.T) {
	done := map[string]bool{"intro": true, "basics": true}
	if isLocked([]string{"intro", "basics"}, done) {
		t.Error("expected not locked when all prereqs done")
	}
}

func TestIsLocked_SomePrereqsMissing(t *testing.T) {
	done := map[string]bool{"intro": true}
	if !isLocked([]string{"intro", "basics"}, done) {
		t.Error("expected locked when some prereqs are missing")
	}
}

func TestIsLocked_EmptyPrereqs(t *testing.T) {
	if isLocked(nil, map[string]bool{}) {
		t.Error("expected not locked for empty prereqs")
	}
}

// ── GetModule additional paths ────────────────────────────────────────────────

func newTestStateWithQuiz(t *testing.T, mock *httptest.Server) *State {
	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "quiz-course",
		Title:    "Quiz Course",
		IsPublic: true,
		Modules: []content.Module{
			{
				Name: "Inline Quiz", Type: "quiz",
				PassingScore: 50,
				Questions: []content.Question{
					{
						ID: "q1", Type: "single", Points: 1,
						Answers: []content.Answer{
							{ID: "a1", Text: "Right", Correct: true},
							{ID: "a2", Text: "Wrong", Correct: false},
						},
					},
				},
			},
			{Name: "Text Module", Type: "text"}, // no git content → returns empty string
			{Name: "Lab Module", Type: "lab", Src: "https://labs.example.com/lab1"},
		},
	})
	store.Put(&content.Course{
		Slug:     "quiz-course-no-questions",
		Title:    "Empty Quiz",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Empty Quiz", Type: "quiz"}, // no questions, no git
		},
	})
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UploadsDir:     "./testdata",
		UserServiceURL: mock.URL,
	}
	return NewState(cfg, store)
}

func TestGetModule_TextInline(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/quiz-course/modules/1", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "text" {
		t.Errorf("expected type=text, got %q", resp.Type)
	}
}

func TestGetModule_LabType(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/quiz-course/modules/2", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "lab" {
		t.Errorf("expected type=lab, got %q", resp.Type)
	}
	if resp.Content != "https://labs.example.com/lab1" {
		t.Errorf("expected lab URL, got %q", resp.Content)
	}
}

func TestGetModule_QuizInline(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/quiz-course/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Type          string `json:"type"`
		QuestionCount int    `json:"question_count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "quiz" {
		t.Errorf("expected type=quiz, got %q", resp.Type)
	}
}

// ── SubmitModule additional paths ─────────────────────────────────────────────

func TestSubmitModule_InlineQuiz_Success(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	body := `{"answers":{"q1":{"selected_id":"a1"}}}`
	req := httptest.NewRequest("POST", "/api/courses/quiz-course/modules/0/submit", strings.NewReader(body))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitModule_InlineQuiz_InvalidJSON(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/quiz-course/modules/0/submit", strings.NewReader("not-json"))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSubmitModule_NoQuestions(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	body := `{"answers":{}}`
	req := httptest.NewRequest("POST", "/api/courses/quiz-course-no-questions/modules/0/submit", strings.NewReader(body))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitModule_InvalidBody(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/courses/quiz-course/modules/0/submit", strings.NewReader("not-json"))
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON body, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── GetCourse private course paths ────────────────────────────────────────────

func TestGetCourse_PrivateNoAuth(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for private course with no auth, got %d", rec.Code)
	}
}

func TestGetCourse_PrivateEnrolled(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/docker-fundamentals", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for private course enrolled, got %d", rec.Code)
	}
}

func TestGetCourse_WithPrerequisites(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "advanced-course",
		Title:    "Advanced Course",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "basic-course", MinScore: 80, Modules: []string{"intro", "basics"}},
		},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/advanced-course", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp courseResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Prerequisites) != 1 {
		t.Errorf("expected 1 prereq, got %d", len(resp.Prerequisites))
	}
}

// ── GetLab additional paths ───────────────────────────────────────────────────

func TestGetLab_VideoType(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	// Module "Architecture" → slug "architecture", type "video" → labType "interactive"
	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs/architecture", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lab struct {
			LabType string `json:"lab_type"`
		} `json:"lab"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Lab.LabType != "interactive" {
		t.Errorf("expected lab_type=interactive for video module, got %q", resp.Lab.LabType)
	}
}

func TestListLabs_Success(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/labs", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Labs []labResponse `json:"labs"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Labs) != 3 {
		t.Errorf("expected 3 labs, got %d", len(resp.Labs))
	}
}

// ── ClearCourseCache and ClearModuleCache ─────────────────────────────────────

func TestClearCourseCache_Success(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestClearModuleCache_Success(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/modules/0/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestClearModuleCache_NotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/kubernetes-basics/modules/99/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── ServeUpload ───────────────────────────────────────────────────────────────

func TestServeUpload_PathTraversal(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret"}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/uploads/..%2Fetc%2Fpasswd", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d", rec.Code)
	}
}

func TestClearCourseCache_WithGitModules(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "git-course",
		Title:    "Git Course",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Lesson 1", Type: "text", Src: "https://github.com/org/repo", Ref: "main", Path: "lesson1.md"},
			{Name: "Lesson 2", Type: "text", Src: "https://github.com/org/repo", Ref: "main", Path: "lesson2.md"},
		},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/git-course/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ReposCleared int `json:"repos_cleared"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.ReposCleared != 1 {
		t.Errorf("expected 1 repo cleared (deduped), got %d", resp.ReposCleared)
	}
}

func TestClearModuleCache_WithGitModule(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "git-course2",
		Title:    "Git Course 2",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Lesson 1", Type: "text", Src: "https://github.com/org/repo2", Ref: "main", Path: "l1.md"},
		},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("POST", "/api/admin/courses/git-course2/modules/0/cache/clear", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		ReposCleared int `json:"repos_cleared"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.ReposCleared != 1 {
		t.Errorf("expected repos_cleared=1, got %d", resp.ReposCleared)
	}
}

func TestGetLab_LabModuleType(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "lab-type-course",
		Title:    "Lab Type Course",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Hands-on Lab", Type: "lab", Src: "http://lab.example.com"},
		},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24, UserServiceURL: mock.URL}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/lab-type-course/labs/hands-on-lab", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lab struct {
			LabType string `json:"lab_type"`
		} `json:"lab"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Lab.LabType != "interactive" {
		t.Errorf("expected lab_type=interactive for lab module, got %q", resp.Lab.LabType)
	}
}

// ── visibleModules with admin ─────────────────────────────────────────────────

func TestListModules_AdminSeesHidden(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "hidden-course",
		Title:    "Hidden Course",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Visible Module", Type: "text"},
			{Name: "Hidden Module", Type: "text", Hidden: true},
		},
	})
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UserServiceURL: mock.URL,
	}
	s := NewState(cfg, store)
	r := BuildRouter(s, cfg, false)

	// Admin sees hidden modules
	req := httptest.NewRequest("GET", "/api/courses/hidden-course/modules", nil)
	req.Header.Set("Authorization", adminAuthHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var adminResp struct {
		Modules []moduleResponse `json:"modules"`
	}
	json.NewDecoder(rec.Body).Decode(&adminResp)
	if len(adminResp.Modules) != 2 {
		t.Errorf("admin: expected 2 modules (including hidden), got %d", len(adminResp.Modules))
	}

	// Student doesn't see hidden modules
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/courses/hidden-course/modules", nil)
	req2.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	r.ServeHTTP(rec2, req2)
	var studentResp struct {
		Modules []moduleResponse `json:"modules"`
	}
	json.NewDecoder(rec2.Body).Decode(&studentResp)
	if len(studentResp.Modules) != 1 {
		t.Errorf("student: expected 1 module (non-hidden), got %d", len(studentResp.Modules))
	}
}

// ── ListCourses search filter ─────────────────────────────────────────────────

func TestListCourses_SearchFilter(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses?search=kubernetes", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Total int `json:"total"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Errorf("expected 2 kubernetes courses, got %d", resp.Total)
	}
}

func TestListCourses_DifficultyFilter(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses?difficulty=advanced", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Total int `json:"total"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 advanced course, got %d", resp.Total)
	}
}

// ── GetCourseProgress ─────────────────────────────────────────────────────────

func TestGetCourseProgress_Success(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/kubernetes-basics/progress", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── tokenForRepo with matching credentials ───────────────────────────────────

func TestTokenForRepo_WithMatchingCredentials(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newTestState(t, mock)

	var creds *content.GitCredentialStore
	tmp, err := os.CreateTemp("", "creds-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString("credentials:\n  - url: \"github.com/org/*\"\n    token: \"my-credential-token\"\n")
	tmp.Close()
	creds, err = content.LoadCredentials(tmp.Name())
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	s.GitCreds = creds

	tok := s.tokenForRepo("https://github.com/org/repo")
	if tok != "my-credential-token" {
		t.Errorf("expected my-credential-token, got %q", tok)
	}
}

// ── GetModule with inline quiz ───────────────────────────────────────────────

func newStateWithQuiz(t *testing.T, mock *httptest.Server) *State {
	t.Helper()
	store := content.NewStore()
	store.Put(&content.Course{
		Slug:     "quiz-course",
		Title:    "Quiz Course",
		IsPublic: true,
		Modules: []content.Module{
			{Name: "Intro", Type: "text"},
			{
				Name:         "Knowledge Check",
				Type:         "quiz",
				PassingScore: 50,
				Questions: []content.Question{
					{
						ID:     "q1",
						Type:   "single",
						Points: 10,
						Answers: []content.Answer{
							{ID: "a", Text: "Right", Correct: true},
							{ID: "b", Text: "Wrong", Correct: false},
						},
						Feedback: content.Feedback{Correct: "yes", Wrong: "no"},
					},
				},
			},
			{
				Name:         "Inline Check",
				Type:         "quiz",
				Inline:       true,
				PassingScore: 50,
				Questions: []content.Question{
					{
						ID:       "iq1",
						Type:     "boolean",
						Points:   5,
						Feedback: content.Feedback{Correct: "yes", Wrong: "no"},
					},
				},
			},
		},
	})
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		JWTExpiryH:     24,
		UserServiceURL: mock.URL,
	}
	return NewState(cfg, store)
}

func TestGetModule_InlineQuiz(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	req := httptest.NewRequest("GET", "/api/courses/quiz-course/modules/1", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "quiz" {
		t.Errorf("expected type=quiz, got %q", resp.Type)
	}
	if len(resp.Questions) == 0 {
		t.Error("expected questions in quiz module")
	}
	if resp.InlineQuiz == nil {
		t.Error("expected inline quiz in response (next module is inline)")
	}
}

func TestGetModule_QuizModuleCount(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()
	s := newStateWithQuiz(t, mock)
	cfg := &config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}
	r := BuildRouter(s, cfg, false)

	// Module 0 is text with no inline next quiz (next is quiz at index 1, not inline)
	req := httptest.NewRequest("GET", "/api/courses/quiz-course/modules/0", nil)
	req.Header.Set("Authorization", authHeader(t, cfg.JWTSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp moduleResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Type != "text" {
		t.Errorf("expected type=text, got %q", resp.Type)
	}
}

// ── CooldownTracker.CheckModule when entry expired ───────────────────────────

func TestCooldownTracker_CheckModule_Expired(t *testing.T) {
	ct := content.NewCooldownTracker()
	// Record with 0-second cooldown → immediately expired
	spec := content.CooldownSpec{Strategy: "fixed", BaseSeconds: 0}
	ct.RecordModule("user1", "course1", 0, "q1", spec, nil, false)

	remaining, attempts := ct.CheckModule("user1", "course1", 0, "q1")
	if remaining != 0 {
		t.Errorf("expected 0 remaining for expired cooldown, got %v", remaining)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}
