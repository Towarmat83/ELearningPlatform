package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// aomRun drives the AdminOrManager middleware with the given Authorization
// header value and returns the recorded status code.
func aomRun(t *testing.T, authHeader string) int {
	t.Helper()

	handler := AdminOrManager(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return rr.Code
}

// TestAdminOrManager_Admin lets an admin token through.
func TestAdminOrManager_Admin(t *testing.T) {
	t.Parallel()

	tok, _ := CreateToken("u", "a@b.com", roleAdmin, testSecret, 24)

	if code := aomRun(t, "Bearer "+tok); code != http.StatusOK {
		t.Errorf("admin: want 200, got %d", code)
	}
}

// TestAdminOrManager_Manager lets a manager token through.
func TestAdminOrManager_Manager(t *testing.T) {
	t.Parallel()

	tok, _ := CreateToken("u", "m@b.com", roleManager, testSecret, 24)

	if code := aomRun(t, "Bearer "+tok); code != http.StatusOK {
		t.Errorf("manager: want 200, got %d", code)
	}
}

// TestAdminOrManager_StudentForbidden rejects a student token.
func TestAdminOrManager_StudentForbidden(t *testing.T) {
	t.Parallel()

	tok, _ := CreateToken("u", "s@b.com", "student", testSecret, 24)

	if code := aomRun(t, "Bearer "+tok); code != http.StatusForbidden {
		t.Errorf("student: want 403, got %d", code)
	}
}

// TestAdminOrManager_MissingHeader rejects a request with no header.
func TestAdminOrManager_MissingHeader(t *testing.T) {
	t.Parallel()

	if code := aomRun(t, ""); code != http.StatusUnauthorized {
		t.Errorf("missing header: want 401, got %d", code)
	}
}

// TestAdminOrManager_InvalidToken rejects an unparsable token.
func TestAdminOrManager_InvalidToken(t *testing.T) {
	t.Parallel()

	if code := aomRun(t, "Bearer nonsense"); code != http.StatusUnauthorized {
		t.Errorf("invalid token: want 401, got %d", code)
	}
}
