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
		IsPublished: true,
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
		IsPublished: false,
	})

	store.Put(&content.Course{
		Slug:        "advanced-kubernetes",
		Title:       "Advanced Kubernetes",
		Description: "Deep dive into K8s networking and scheduling",
		Category:    "kubernetes",
		Difficulty:  "advanced",
		IsPublished: true,
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

func newUserServiceMock() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/enrollments/check":
			json.NewEncoder(w).Encode(map[string]bool{"enrolled": true})
		case "/internal/progress/viewed":
			json.NewEncoder(w).Encode(map[string]any{"viewed": []string{}})
		case "/internal/progress/complete":
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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
