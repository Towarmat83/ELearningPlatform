package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSecret = "test-jwt-secret-key"

func TestCreateToken(t *testing.T) {
	token, err := CreateToken("user-123", "test@example.com", "student", testSecret, 24)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestVerifyToken_Valid(t *testing.T) {
	token, err := CreateToken("user-abc", "foo@bar.com", "admin", testSecret, 24)
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
	if claims.Role != "admin" {
		t.Errorf("expected role=admin, got %q", claims.Role)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	token, err := CreateToken("u1", "a@b.com", "student", testSecret, 24)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyToken(token, "wrong-secret")
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestVerifyToken_Malformed(t *testing.T) {
	_, err := VerifyToken("not.a.token", testSecret)
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

func TestVerifyToken_Empty(t *testing.T) {
	_, err := VerifyToken("", testSecret)
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestGetClaims_WithContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := GetClaims(req)
	if claims != nil {
		t.Error("expected nil claims from request without context")
	}
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	token, _ := CreateToken("user-1", "a@b.com", "student", testSecret, 24)

	var gotClaims *Claims
	handler := Auth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
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

func TestAdminMiddleware_StudentRole(t *testing.T) {
	token, _ := CreateToken("user-2", "a@b.com", "student", testSecret, 24)

	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", rr.Code)
	}
}

func TestAdminMiddleware_AdminRole(t *testing.T) {
	token, _ := CreateToken("user-3", "admin@b.com", "admin", testSecret, 24)

	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", rr.Code)
	}
}

func TestAdminMiddleware_NoHeader(t *testing.T) {
	handler := Admin(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
