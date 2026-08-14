package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
	"github.com/genesary/pupitre/course-service/internal/config"
	apimiddleware "github.com/genesary/pupitre/course-service/internal/middleware"
)

// managerAuthHeader returns a Bearer token header for a manager user.
func managerAuthHeader(t *testing.T, secret string) string {
	t.Helper()

	token, err := apimiddleware.CreateToken(
		"00000000-0000-0000-0000-000000000002",
		"manager@test.com",
		"manager",
		secret,
		24,
	)
	if err != nil {
		t.Fatalf("create manager token: %v", err)
	}

	return "Bearer " + token
}

// ── generateSessionID ─────────────────────────────────────────────────────────

// TestGenerateSessionID verifies that generated IDs have the expected format.
func TestGenerateSessionID(t *testing.T) {
	t.Parallel()

	id, err := generateSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(id, "sess-") {
		t.Errorf("expected id to start with sess-, got %q", id)
	}

	// "sess-" (5) + 8 hex chars = 13 total
	if len(id) != 13 {
		t.Errorf("expected length 13, got %d (%q)", len(id), id)
	}

	hex := id[5:]

	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("id contains non-hex char %q in %q", c, id)
		}
	}
}

// TestGenerateSessionID_Unique verifies two consecutive calls return
// different IDs.
func TestGenerateSessionID_Unique(t *testing.T) {
	t.Parallel()

	first, err := generateSessionID()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	second, err := generateSessionID()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if first == second {
		t.Errorf("expected unique IDs, got %q twice", first)
	}
}

// ── applySessionUpdate ────────────────────────────────────────────────────────

// TestApplySessionUpdate_Found verifies that the matching session is updated.
func TestApplySessionUpdate_Found(t *testing.T) {
	t.Parallel()

	sessions := []coursev1.CourseSession{
		{ID: "sess-aaa", Title: "Old Title", Date: "2026-09-01T10:00:00Z", Location: "Room A", Capacity: 10},
		{ID: "sess-bbb", Title: "Other", Date: "2026-10-01T10:00:00Z", Location: "Room B", Capacity: 5},
	}

	body := sessionBody{Title: "New Title", Date: "2026-09-15T09:00:00Z", Location: "Room C", Capacity: 20}

	updated, found := applySessionUpdate(sessions, "sess-aaa", body)

	if !found {
		t.Fatal("expected found=true")
	}

	if updated[0].Title != "New Title" {
		t.Errorf("expected title=New Title, got %q", updated[0].Title)
	}

	if updated[0].Location != "Room C" {
		t.Errorf("expected location=Room C, got %q", updated[0].Location)
	}

	if updated[0].Capacity != 20 {
		t.Errorf("expected capacity=20, got %d", updated[0].Capacity)
	}
}

// TestApplySessionUpdate_NotFound verifies that a missing ID returns
// found=false.
func TestApplySessionUpdate_NotFound(t *testing.T) {
	t.Parallel()

	sessions := []coursev1.CourseSession{
		{ID: "sess-aaa", Title: "Existing"},
	}

	_, found := applySessionUpdate(sessions, "sess-zzz", sessionBody{Title: "X", Date: "2026-01-01T00:00:00Z"})

	if found {
		t.Error("expected found=false for unknown session ID")
	}
}

// TestApplySessionUpdate_PreservesOthers verifies that non-matching sessions
// are not modified.
func TestApplySessionUpdate_PreservesOthers(t *testing.T) {
	t.Parallel()

	sessions := []coursev1.CourseSession{
		{ID: "sess-aaa", Title: "Keep Me"},
		{ID: "sess-bbb", Title: "Update Me"},
	}

	body := sessionBody{Title: "Updated", Date: "2026-09-01T10:00:00Z", Location: "X", Capacity: 1}

	updated, found := applySessionUpdate(sessions, "sess-bbb", body)

	if !found {
		t.Fatal("expected found=true")
	}

	if updated[0].Title != "Keep Me" {
		t.Errorf("expected first session unchanged, got %q", updated[0].Title)
	}

	if updated[1].Title != "Updated" {
		t.Errorf("expected second session updated, got %q", updated[1].Title)
	}
}

// ── Middleware enforcement ────────────────────────────────────────────────────

// TestCreateSession_RequiresAuth verifies that unauthenticated requests are
// rejected with 401.
func TestCreateSession_RequiresAuth(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

// TestCreateSession_StudentForbidden verifies that a student cannot create
// sessions.
func TestCreateSession_StudentForbidden(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions", http.NoBody)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", rec.Code)
	}
}

// TestCreateSession_AdminPassesMiddleware verifies that an admin is not blocked
// by the middleware (K8s failure expected beyond that point).
func TestCreateSession_AdminPassesMiddleware(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	body := `{"title":"Test","date":"2026-09-01T10:00:00Z","location":"Room A","capacity":10}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", adminAuthHeader(t, "test-secret"))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// K8s not available in tests — 500 is expected, not 401/403
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("admin should not be blocked by middleware, got %d", rec.Code)
	}
}

// TestCreateSession_ManagerPassesMiddleware verifies that a manager is not
// blocked by the middleware.
func TestCreateSession_ManagerPassesMiddleware(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	body := `{"title":"Test","date":"2026-09-01T10:00:00Z","location":"Room A","capacity":10}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", managerAuthHeader(t, "test-secret"))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("manager should not be blocked by middleware, got %d", rec.Code)
	}
}

// ── Input validation ──────────────────────────────────────────────────────────

// TestCreateSession_BadJSON verifies that malformed JSON returns 400.
func TestCreateSession_BadJSON(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions", strings.NewReader(`{bad json`))
	req.Header.Set("Authorization", adminAuthHeader(t, "test-secret"))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", rec.Code)
	}
}

// TestCreateSession_MissingTitle verifies that a missing title returns 400.
func TestCreateSession_MissingTitle(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions",
		strings.NewReader(`{"date":"2026-09-01T10:00:00Z","location":"Room A","capacity":10}`))
	req.Header.Set("Authorization", adminAuthHeader(t, "test-secret"))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing title, got %d", rec.Code)
	}
}

// TestCreateSession_MissingDate verifies that a missing date returns 400.
func TestCreateSession_MissingDate(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/admin/courses/devops-lab-demo/sessions",
		strings.NewReader(`{"title":"Session","location":"Room A","capacity":10}`))
	req.Header.Set("Authorization", adminAuthHeader(t, "test-secret"))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing date, got %d", rec.Code)
	}
}

// TestUpdateSession_RequiresAuth verifies that unauthenticated requests are
// rejected with 401.
func TestUpdateSession_RequiresAuth(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/admin/courses/devops-lab-demo/sessions/sess-abc", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

// TestUpdateSession_StudentForbidden verifies that a student cannot update
// sessions.
func TestUpdateSession_StudentForbidden(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/admin/courses/devops-lab-demo/sessions/sess-abc", http.NoBody)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", rec.Code)
	}
}

// TestDeleteSession_RequiresAuth verifies that unauthenticated requests are
// rejected with 401.
func TestDeleteSession_RequiresAuth(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"/api/admin/courses/devops-lab-demo/sessions/sess-abc", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

// TestDeleteSession_StudentForbidden verifies that a student cannot delete
// sessions.
func TestDeleteSession_StudentForbidden(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	s := newTestState(t, mock)
	r := BuildRouter(s, &config.Config{JWTSecret: "test-secret"}, false)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"/api/admin/courses/devops-lab-demo/sessions/sess-abc", http.NoBody)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", rec.Code)
	}
}
