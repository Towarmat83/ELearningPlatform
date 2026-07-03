package middleware

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-signing-secret-32bytes-long!!"

func TestCreateToken(t *testing.T) {
	token, err := CreateToken("user-1", "user@example.com", "student", testSecret, 24)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestVerifyToken_Valid(t *testing.T) {
	token, err := CreateToken("user-1", "user@example.com", "student", testSecret, 24)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	claims, err := VerifyToken(token, testSecret)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}

	if claims.Subject != "user-1" {
		t.Errorf("Subject: want user-1, got %q", claims.Subject)
	}

	if claims.Email != "user@example.com" {
		t.Errorf("Email: want user@example.com, got %q", claims.Email)
	}

	if claims.Role != "student" {
		t.Errorf("Role: want student, got %q", claims.Role)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	token, _ := CreateToken("user-1", "user@example.com", "student", testSecret, 24)

	_, err := VerifyToken(token, "wrong-secret")
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
}

func TestVerifyToken_Malformed(t *testing.T) {
	_, err := VerifyToken("not.a.valid.token", testSecret)
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	// Create token with -1 hour expiry (already expired).
	claims := Claims{
		Email: "user@example.com",
		Role:  "student",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}

	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyToken(tok, testSecret)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestVerifyToken_WrongSigningMethod(t *testing.T) {
	// Create a token signed with RS256 (not HMAC) — should be rejected.
	_, err := VerifyToken("header.payload.sig", testSecret)
	if err == nil {
		t.Error("expected error for invalid token structure")
	}
}

func TestGetClaims_Set(t *testing.T) {
	claims := &Claims{Email: "a@b.com", Role: "admin"}
	claims.Subject = "uid-123"

	ctx := context.WithValue(context.Background(), ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody).WithContext(ctx)

	got := GetClaims(req)
	if got == nil {
		t.Fatal("GetClaims returned nil")
	}

	if got.Email != "a@b.com" {
		t.Errorf("Email: want a@b.com, got %q", got.Email)
	}
}

func TestGetClaims_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := GetClaims(req); got != nil {
		t.Errorf("GetClaims with no claims: want nil, got %+v", got)
	}
}

func TestAuth_Valid(t *testing.T) {
	token, _ := CreateToken("user-1", "user@example.com", "student", testSecret, 24)

	mw := Auth(nil, testSecret)

	var captured *Claims

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetClaims(r)

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Auth valid: want 200, got %d", rec.Code)
	}

	if captured == nil || captured.Subject != "user-1" {
		t.Errorf("claims not propagated: %+v", captured)
	}
}

func TestAuth_MissingHeader(t *testing.T) {
	mw := Auth(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Auth missing header: want 401, got %d", rec.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	mw := Auth(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer bad-token")

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Auth invalid token: want 401, got %d", rec.Code)
	}
}

func TestAuth_MissingSubject(t *testing.T) {
	// Token without subject.
	claims := Claims{
		Email: "user@example.com",
		Role:  "student",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))

	mw := Auth(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Auth missing subject: want 401, got %d", rec.Code)
	}
}

func TestAdmin_ValidAdminToken(t *testing.T) {
	token, _ := CreateToken("admin-1", "admin@example.com", "admin", testSecret, 24)

	mw := Admin(nil, testSecret)

	var captured *Claims

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetClaims(r)

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Admin valid: want 200, got %d", rec.Code)
	}

	if captured == nil || captured.Role != "admin" {
		t.Errorf("admin claims not propagated: %+v", captured)
	}
}

func TestAdmin_StudentForbidden(t *testing.T) {
	token, _ := CreateToken("user-1", "user@example.com", "student", testSecret, 24)

	mw := Admin(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Admin student: want 403, got %d", rec.Code)
	}
}

func TestAdmin_MissingHeader(t *testing.T) {
	mw := Admin(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Admin missing header: want 401, got %d", rec.Code)
	}
}

func TestVerifyToken_NonHMACMethod(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1","email":"u@test.com","role":"student"}`))
	fake := header + "." + payload + ".invalidsig"

	_, err := VerifyToken(fake, testSecret)
	if err == nil {
		t.Error("expected error for non-HMAC signing method")
	}
}

func TestAdmin_InvalidToken(t *testing.T) {
	mw := Admin(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer invalid-token")

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Admin invalid token: want 401, got %d", rec.Code)
	}
}

func TestAdmin_MissingSubject(t *testing.T) {
	claims := Claims{
		Email: "user@example.com",
		Role:  "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	mw := Admin(nil, testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Admin missing subject: want 401, got %d", rec.Code)
	}
}
