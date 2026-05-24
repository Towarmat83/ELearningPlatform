package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		IsPublic: true,
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
		IsPublic: false,
	})

	store.Put(&content.Course{
		Slug:        "advanced-kubernetes",
		Title:       "Advanced Kubernetes",
		Description: "Deep dive into K8s networking and scheduling",
		Category:    "kubernetes",
		Difficulty:  "advanced",
		IsPublic: true,
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
		Slug:        "linux-intro",
		Title:       "Intro Linux",
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
		Slug:        "kubernetes-adv",
		Title:       "Advanced Kubernetes",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "linux-intro", MinScore: 30, Modules: []string{"quiz-bases-linux"}},
		},
		Modules: []content.Module{
			{Name: "K8s Networking", Type: "text"},
		},
	})

	store.Put(&content.Course{
		Slug:        "network-basics",
		Title:       "Network Basics",
		IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "linux-intro", MinScore: 50},
		},
		Modules: []content.Module{
			{Name: "OSI Model", Type: "text"},
		},
	})

	store.Put(&content.Course{
		Slug:        "free-course",
		Title:       "Free Course (any linux-intro progress)",
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
