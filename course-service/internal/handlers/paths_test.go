package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/content"
)

func newPathTestState(t *testing.T) *State {
	t.Helper()
	store := content.NewStore()
	paths := content.NewPathStore()
	paths.Put(&content.Path{
		Slug:        "devops-path",
		Title:       "DevOps Path",
		Description: "From Linux to Kubernetes",
		Courses:     []string{"linux-intro", "docker-fundamentals", "kubernetes-basics"},
	})
	paths.Put(&content.Path{
		Slug:    "python-path",
		Title:   "Python Path",
		Courses: []string{"python-basics", "python-advanced"},
	})
	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24}
	return NewState(cfg, store, paths)
}

func TestListPaths(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	s := newPathTestState(t)
	r := BuildRouter(s, s.Config, false)

	req := httptest.NewRequest(http.MethodGet, "/api/paths", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	paths, ok := resp["paths"].([]any)
	if !ok {
		t.Fatal("expected paths array")
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestGetPath(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	s := newPathTestState(t)
	r := BuildRouter(s, s.Config, false)

	req := httptest.NewRequest(http.MethodGet, "/api/paths/devops-path", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var p content.Path
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Slug != "devops-path" {
		t.Errorf("expected slug=devops-path, got %q", p.Slug)
	}
	if len(p.Courses) != 3 {
		t.Errorf("expected 3 courses, got %d", len(p.Courses))
	}
}

func TestGetPathNotFound(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	s := newPathTestState(t)
	r := BuildRouter(s, s.Config, false)

	req := httptest.NewRequest(http.MethodGet, "/api/paths/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListPathsSorted(t *testing.T) {
	mock := newUserServiceMock()
	defer mock.Close()

	s := newPathTestState(t)
	r := BuildRouter(s, s.Config, false)

	req := httptest.NewRequest(http.MethodGet, "/api/paths", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	paths := resp["paths"].([]any)

	first := paths[0].(map[string]any)["title"].(string)
	if first != "DevOps Path" {
		t.Errorf("expected sorted by title (DevOps Path first), got %q", first)
	}
}
