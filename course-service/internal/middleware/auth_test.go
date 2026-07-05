package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testSecret is the JWT signing secret used across this file's tests.
const testSecret = "test-jwt-secret-key"

// TestCreateToken verifies that CreateToken returns a non-empty token
// without error.
func TestCreateToken(t *testing.T) {
	t.Parallel()

	token, err := CreateToken("user-123", "test@example.com", "student", testSecret, 24)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

// TestVerifyToken_Valid verifies that VerifyToken returns claims matching
// what was passed to CreateToken.
func TestVerifyToken_Valid(t *testing.T) {
	t.Parallel()

	token, err := CreateToken("user-abc", "foo@bar.com", roleAdmin, testSecret, 24)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := VerifyToken(token, testSecret)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if claims.Subject != "user-abc" {
		t.Errorf("expected subject=user-abc, got %q", claims.Subject)
	}

	if claims.Email != "foo@bar.com" {
		t.Errorf("expected email=foo@bar.com, got %q", claims.Email)
	}

	if claims.Role != roleAdmin {
		t.Errorf("expected role=admin, got %q", claims.Role)
	}
}

// TestVerifyToken_WrongSecret verifies that VerifyToken rejects a token
// signed with a different secret.
func TestVerifyToken_WrongSecret(t *testing.T) {
	t.Parallel()

	token, err := CreateToken("u1", "a@b.com", "student", testSecret, 24)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyToken(token, "wrong-secret")
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

// TestVerifyToken_Malformed verifies that VerifyToken rejects a malformed
// token string.
func TestVerifyToken_Malformed(t *testing.T) {
	t.Parallel()

	_, err := VerifyToken("not.a.token", testSecret)
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

// TestVerifyToken_Empty verifies that VerifyToken rejects an empty token
// string.
func TestVerifyToken_Empty(t *testing.T) {
	t.Parallel()

	_, err := VerifyToken("", testSecret)
	if err == nil {
		t.Error("expected error for empty token")
	}
}

// TestGetClaims_WithContext verifies that GetClaims returns nil when the
// request context has no claims set.
func TestGetClaims_WithContext(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	claims := GetClaims(req)
	if claims != nil {
		t.Error("expected nil claims from request without context")
	}
}

// TestAuthMiddleware_NoHeader verifies that Auth rejects requests without an
// Authorization header.
func TestAuthMiddleware_NoHeader(t *testing.T) {
	t.Parallel()

	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_InvalidToken verifies that Auth rejects requests
// bearing an invalid token.
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	t.Parallel()

	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer invalid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_ValidToken verifies that Auth accepts a valid token and
// stores its claims in the request context passed to the next handler.
func TestAuthMiddleware_ValidToken(t *testing.T) {
	t.Parallel()

	token, _ := CreateToken("user-1", "a@b.com", "student", testSecret, 24)

	var gotClaims *Claims

	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	if gotClaims == nil {
		t.Error("expected claims in context")
	} else if gotClaims.Subject != "user-1" {
		t.Errorf("expected subject=user-1, got %q", gotClaims.Subject)
	}
}

// TestAdminMiddleware_StudentRole verifies that Admin rejects a valid token
// belonging to a non-admin role.
func TestAdminMiddleware_StudentRole(t *testing.T) {
	t.Parallel()

	token, _ := CreateToken("user-2", "a@b.com", "student", testSecret, 24)

	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", rr.Code)
	}
}

// TestAdminMiddleware_AdminRole verifies that Admin accepts a valid token
// belonging to the admin role.
func TestAdminMiddleware_AdminRole(t *testing.T) {
	t.Parallel()

	token, _ := CreateToken("user-3", "admin@b.com", roleAdmin, testSecret, 24)

	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", rr.Code)
	}
}

// TestAdminMiddleware_NoHeader verifies that Admin rejects requests without
// an Authorization header.
func TestAdminMiddleware_NoHeader(t *testing.T) {
	t.Parallel()

	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
