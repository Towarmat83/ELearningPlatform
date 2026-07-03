package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	patternv1 "github.com/elearning/user-service/api/v1"

	"github.com/elearning/user-service/fake"
	"github.com/elearning/user-service/internal/config"
	apimiddleware "github.com/elearning/user-service/internal/middleware"
)

const (
	htSecret = "test-jwt-signing-secret-32bytes!!"
	htExpiry = 24
)

var (
	htLoginPassword = "TestPass99"
	htLoginHash     string
)

func TestMain(m *testing.M) {
	h, err := bcrypt.GenerateFromPassword([]byte(htLoginPassword), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}

	htLoginHash = string(h)

	os.Exit(m.Run())
}

func newTestRouter(pool *fake.Pool) http.Handler {
	cfg := &config.Config{
		JWTSecret:   htSecret,
		JWTExpiryH:  htExpiry,
		CORSOrigins: []string{"*"},
	}
	s := &State{Pool: pool, Config: cfg}

	return BuildRouter(s, cfg, pool, false)
}

func htAuthHeader(t *testing.T, role string) string {
	t.Helper()

	tok, err := apimiddleware.CreateToken("user-uuid-1", "user@test.com", role, htSecret, htExpiry)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	return "Bearer " + tok
}

// skipErrRows pushes n error rows so ReadSetting calls consume them and return their fallbacks.
func skipErrRows(pool *fake.Pool, n int) {
	for range n {
		pool.PushRow(errors.New("no value"))
	}
}

func htDo(t *testing.T, handler http.Handler, method, path, body, auth string) *httptest.ResponseRecorder {
	t.Helper()

	var buf *bytes.Reader
	if body != "" {
		buf = bytes.NewReader([]byte(body))
	} else {
		buf = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, buf)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

// ── Health ────────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/health", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("Health: want 200, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
}

// ── PublicSettings ────────────────────────────────────────────────────────────

func TestPublicSettings(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/settings/public", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("PublicSettings: want 200, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)

	if _, ok := resp["registration_enabled"]; !ok {
		t.Error("expected registration_enabled in response")
	}
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/register", "not-json", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestRegister_RegistrationDisabled(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "false") // registration_enabled → "false"
	r := newTestRouter(pool)
	body := `{"username":"testuser","email":"t@test.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRegister_EmailDomainBlocked(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")        // registration_enabled → "true"
	pool.PushRow(nil, "example.com") // registration_email_whitelist → "example.com"
	r := newTestRouter(pool)
	body := `{"username":"testuser","email":"t@other.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 (domain blocked), got %d", rec.Code)
	}
}

func TestRegister_UsernameTooShort(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"username":"ab","email":"t@test.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestRegister_PasswordTooShort(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"short"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestRegister_PasswordNeedsUppercase(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 2)      // registration_enabled → fallback, email_whitelist → fallback
	pool.PushRow(nil, "8")    // password_min_length → "8"
	pool.PushRow(nil, "true") // password_require_uppercase → "true"
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"nouppercase"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (no uppercase), got %d", rec.Code)
	}
}

func TestRegister_PasswordNeedsNumber(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 2)       // registration_enabled, email_whitelist → fallbacks
	pool.PushRow(nil, "8")     // password_min_length → "8"
	pool.PushRow(nil, "false") // password_require_uppercase → "false"
	pool.PushRow(nil, "true")  // password_require_number → "true"
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"NoNumbersHere"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (no number), got %d", rec.Code)
	}
}

func TestRegister_PasswordMinLenFallback(t *testing.T) {
	// When the min_length setting returns "0" (< 1), it falls back to 8
	pool := &fake.Pool{}
	skipErrRows(pool, 2)   // registration_enabled, email_whitelist
	pool.PushRow(nil, "0") // password_min_length="0" → triggers minLen<1 fallback → minLen=8
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"short"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (too short after fallback), got %d", rec.Code)
	}
}

func TestRegister_PasswordHasUppercasePasses(t *testing.T) {
	// password_require_uppercase=true, password HAS uppercase → passes that check
	pool := &fake.Pool{}
	skipErrRows(pool, 2)       // registration_enabled, email_whitelist
	pool.PushRow(nil, "8")     // password_min_length
	pool.PushRow(nil, "true")  // password_require_uppercase=true
	pool.PushRow(nil, "false") // password_require_number=false
	// After validation passes, hit a DB error on COUNT(*) to stop
	pool.PushRow(errors.New("db down"))
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"ValidPassWord"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (DB error after valid password), got %d", rec.Code)
	}
}

func TestRegister_PasswordHasNumberPasses(t *testing.T) {
	// password_require_number=true, password HAS number → passes that check
	pool := &fake.Pool{}
	skipErrRows(pool, 2)       // registration_enabled, email_whitelist
	pool.PushRow(nil, "8")     // password_min_length
	pool.PushRow(nil, "false") // password_require_uppercase=false
	pool.PushRow(nil, "true")  // password_require_number=true
	// After validation passes, hit a DB error on COUNT(*) to stop
	pool.PushRow(errors.New("db down"))
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"validpassword1"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (DB error after valid password), got %d", rec.Code)
	}
}

func TestRegister_Conflict(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 5)        // 2 before validation + 3 in validatePasswordPolicy
	pool.PushRow(nil, int64(1)) // COUNT(*) → existing=1
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rec.Code)
	}
}

func TestRegister_DBError(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 5)
	pool.PushRow(errors.New("connection failed")) // COUNT(*) error → 500
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/login", "not-json", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestLogin_LocalLoginDisabled(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "false") // sso_local_login_enabled → "false"
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/login", `{"email":"t@test.com","password":"TestPass99"}`, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	pool := &fake.Pool{}
	// sso_local_login_enabled → empty → fallback "true"; user query → empty → error → 401
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/login", `{"email":"t@test.com","password":"TestPass99"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestLogin_NullPasswordHash(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 1) // sso_local_login_enabled → fallback "true"
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com",
		nil, // password_hash (nil → SSO user)
		"student", nil, nil, true, "github", "2024-01-01")
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/login", `{"email":"t@test.com","password":"TestPass99"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (SSO user), got %d", rec.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 1) // sso_local_login_enabled → fallback "true"
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com",
		htLoginHash, "student", nil, nil, true, "local", "2024-01-01")
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/login", `{"email":"t@test.com","password":"WrongPassword"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (wrong password), got %d", rec.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 1) // sso_local_login_enabled → fallback "true"
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com",
		htLoginHash, "student", nil, nil, true, "local", "2024-01-01")
	r := newTestRouter(pool)
	body := fmt.Sprintf(`{"email":"t@test.com","password":%q}`, htLoginPassword)

	rec := htDo(t, r, "POST", "/api/auth/login", body, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["token"] == nil || resp["token"] == "" {
		t.Error("expected token in response")
	}
}

// ── Me ────────────────────────────────────────────────────────────────────────

func TestMe_Unauthorized(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/me", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestMe_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/me", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestMe_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com", "student",
		nil, nil, true, "local", "2024-01-01")
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/me", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── UpdateProfile ─────────────────────────────────────────────────────────────

func TestUpdateProfile_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", "bad-json", htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateProfile_UsernameTooShort(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true") // profile_allow_username_change → "true"
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", `{"username":"ab"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateProfile_UsernameDisallowed(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "false") // profile_allow_username_change → "false"
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", `{"username":"newname"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestUpdateProfile_BioOnly(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com", "student",
		nil, nil, true, "local", "2024-01-01")
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", `{"bio":"My bio"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── ChangePassword ────────────────────────────────────────────────────────────

func TestChangePassword_MissingOldPassword(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", `{"new_password":"NewPass99"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestChangePassword_MissingNewPassword(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", `{"old_password":"OldPass99"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestChangePassword_NewPasswordTooShort(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", `{"old_password":"OldPass99","new_password":"short"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestChangePassword_SSOUser(t *testing.T) {
	pool := &fake.Pool{}
	// validatePasswordPolicy: 3 ReadSettings → all empty → fallbacks; then QueryRow → empty → error → 400
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", `{"old_password":"OldPass99","new_password":"NewPass9999"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (SSO user), got %d", rec.Code)
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 3)           // 3 ReadSettings in validatePasswordPolicy → fallbacks
	pool.PushRow(nil, htLoginHash) // password_hash
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", `{"old_password":"WrongOld","new_password":"NewPass9999"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestChangePassword_Success(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 3)           // validatePasswordPolicy ReadSettings → defaults
	pool.PushRow(nil, htLoginHash) // SELECT password_hash
	pool.PushExec(1, nil)          // UPDATE password_hash
	r := newTestRouter(pool)
	body := fmt.Sprintf(`{"old_password":%q,"new_password":"NewPass9999"}`, htLoginPassword)

	rec := htDo(t, r, "PUT", "/api/auth/password", body, htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePassword_DBUpdateError(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 3)                    // validatePasswordPolicy ReadSettings
	pool.PushRow(nil, htLoginHash)          // SELECT password_hash
	pool.PushExec(0, errors.New("db down")) // UPDATE fails
	r := newTestRouter(pool)
	body := fmt.Sprintf(`{"old_password":%q,"new_password":"NewPass9999"}`, htLoginPassword)

	rec := htDo(t, r, "PUT", "/api/auth/password", body, htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePassword_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/password", "not-json", htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateProfile_UsernameConflict(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")   // ReadSetting(profile_allow_username_change) → "true"
	pool.PushRow(nil, int64(1)) // COUNT(*) → taken=1 → conflict
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", `{"username":"takenname"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rec.Code)
	}
}

func TestUpdateProfile_UsernameSuccess(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")   // ReadSetting → "true"
	pool.PushRow(nil, int64(0)) // COUNT → 0 (not taken)
	pool.PushRow(nil, "user-uuid-1", "newname", "t@test.com", "student",
		nil, nil, true, "local", "2024-01-01") // UPDATE RETURNING
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/auth/profile", `{"username":"newname"}`, htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── Admin Stats ───────────────────────────────────────────────────────────────

func TestAdminStats(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/stats", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if _, ok := resp["total_users"]; !ok {
		t.Error("expected total_users in response")
	}
}

func TestAdminStats_Unauthorized(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/stats", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 for non-admin, got %d", rec.Code)
	}
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

func TestListUsers_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil) // empty result
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestListUsers_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListUsers_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"user-uuid-1", "alice", "alice@test.com", "student", true, nil, nil, "local", "2024-01-01", int64(2)},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	users := resp["users"].([]any)
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
}

func TestListUsers_WithProviderFilter(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"user-uuid-2", "bob", "bob@test.com", "student", true, nil, nil, "github", "2024-01-01", int64(0)},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users?provider=github", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	users := resp["users"].([]any)
	if len(users) != 1 {
		t.Errorf("expected 1 user with github provider, got %d", len(users))
	}
}

// ── GetUser ───────────────────────────────────────────────────────────────────

func TestGetUser_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestGetUser_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com", "student", true,
		nil, nil, "local", "2024-01-01", int64(3), int64(10))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── UpdateUser ────────────────────────────────────────────────────────────────

func TestUpdateUser_InvalidRole(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/users/user-uuid-1", `{"role":"superuser"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateUser_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/users/user-uuid-1", `{"role":"student"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestUpdateUser_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "user-uuid-1", "testuser", "t@test.com", "admin",
		nil, nil, true, "local", "2024-01-01")
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/users/user-uuid-1", `{"role":"admin"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── DeleteUser ────────────────────────────────────────────────────────────────

func TestDeleteUser_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/users/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestDeleteUser_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/users/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── SearchUsers ───────────────────────────────────────────────────────────────

func TestSearchUsers_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/search?q=alice", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestSearchUsers_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"user-uuid-1", "alice", "alice@test.com"})
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/search?q=alice", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── ListAuthProviders ─────────────────────────────────────────────────────────

func TestListAuthProviders_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/providers", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── AdminLeaderboard ──────────────────────────────────────────────────────────

func TestAdminLeaderboard_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/leaderboard", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminLeaderboard_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"user-uuid-1", "alice", "alice@test.com", nil, int64(100), int64(5), int64(3)},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/leaderboard", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	lb := resp["leaderboard"].([]any)
	if len(lb) != 1 {
		t.Errorf("expected 1 entry, got %d", len(lb))
	}
}

func TestAdminLeaderboard_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/leaderboard", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── GetSettings ───────────────────────────────────────────────────────────────

func TestGetSettings_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/settings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestGetSettings_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/settings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestGetSettings_WithData(t *testing.T) {
	pool := &fake.Pool{}
	desc := "enable registration"
	pool.PushRows(nil,
		[]any{"registration_enabled", "true", &desc},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/settings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	settings := resp["settings"].([]any)
	if len(settings) != 1 {
		t.Errorf("expected 1 setting, got %d", len(settings))
	}
}

// ── UpdateSettings ────────────────────────────────────────────────────────────

func TestUpdateSettings_EmptyBody(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", `{}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateSettings_UnknownKey(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", `{"unknown_setting":"val"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateSettings_NonStringValue(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", `{"registration_enabled":true}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpdateSettings_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", `{"registration_enabled":"false"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestUpdateSettings_Success(t *testing.T) {
	pool := &fake.Pool{}
	// Exec → empty → OK, nil; GetSettings → Query → empty rows
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", `{"registration_enabled":"true"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── Groups ────────────────────────────────────────────────────────────────────

func TestCreateGroup_MissingName(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups", `{"name":""}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestCreateGroup_Conflict(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil) // INSERT returns no id (conflict → DO NOTHING, no RETURNING)
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups", `{"name":"existing-group"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rec.Code)
	}
}

func TestCreateGroup_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "new-group-uuid")
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups", `{"name":"new-group"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", rec.Code)
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	// Exec → empty → "OK", nil → RowsAffected=0 → 404
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/group-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/group-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestDeleteGroup_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/group-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListGroups_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListGroups_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestListGroups_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"group-uuid-1", "admins", "local", "2024-01-01", int64(3), "admin"},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	groups := resp["groups"].([]any)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
}

// ── GroupMappings ─────────────────────────────────────────────────────────────

func TestListGroupMappings_ScanError(t *testing.T) {
	pool := &fake.Pool{}
	// struct{} cannot be converted to string → assignOne returns error → Scan error
	pool.PushRows(nil, []any{struct{}{}, "admin"})
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups/mappings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 from scan error, got %d", rec.Code)
	}
}

func TestListGroupMappings_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups/mappings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestListGroupMappings_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"devs", "admin"})
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/groups/mappings", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestUpsertGroupMapping_MissingGroupName(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups/mappings", `{"group_name":"","platform_role":"admin"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpsertGroupMapping_InvalidRole(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups/mappings", `{"group_name":"devs","platform_role":"superadmin"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestUpsertGroupMapping_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups/mappings", `{"group_name":"devs","platform_role":"admin"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestDeleteGroupMapping_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/mappings/devs", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestDeleteGroupMapping_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/mappings/devs", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── Enrollments ───────────────────────────────────────────────────────────────

func TestEnroll_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/courses/my-course/enroll", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestEnroll_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/courses/my-course/enroll", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestUnenroll_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/courses/my-course/unenroll", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestUnenroll_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/courses/my-course/unenroll", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── MarkLessonComplete ────────────────────────────────────────────────────────

func TestMarkLessonComplete_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/courses/my-course/lessons/intro/complete", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestMarkLessonComplete_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/courses/my-course/lessons/intro/complete", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── MyCourses ─────────────────────────────────────────────────────────────────

func TestEnroll_ForeignKeyConstraint(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("foreign key constraint violation"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/courses/test-course/enroll", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (FK constraint = expired session), got %d", rec.Code)
	}
}

func TestMyCourses_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/courses", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestMyCourses_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/courses", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestMyCourses_WithEnrollments(t *testing.T) {
	// Push one enrollment row; fetchCourseDetails will fail (empty URL → bad request)
	// but MyCourses gracefully falls back to slug-only data
	pool := &fake.Pool{}
	activity := "2024-01-15T10:00:00Z"
	pool.PushRows(nil, []any{"my-course-slug", int64(2), int64(50), &activity})
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/my/courses", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Courses []struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"courses"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Courses) != 1 {
		t.Errorf("expected 1 course, got %d", len(resp.Courses))
	}
}

func TestMyCourses_WithCourseService(t *testing.T) {
	// Mock course service that returns valid course data
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"slug": "test-course", "id": "test-course", "title": "Test Course",
			"is_public": true, "lab_count": 3,
		})
	}))
	defer courseSvc.Close()

	pool := &fake.Pool{}
	activity := "2024-01-15T10:00:00Z"
	pool.PushRows(nil, []any{"test-course", int64(1), int64(80), &activity})
	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: courseSvc.URL},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest(http.MethodGet, "/api/my/courses", http.NoBody)
	req.Header.Set("Authorization", htAuthHeader(t, "student"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Courses []struct {
			Title string `json:"title"`
		} `json:"courses"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Courses) != 1 || resp.Courses[0].Title != "Test Course" {
		t.Errorf("expected 1 course with title 'Test Course', got %v", resp.Courses)
	}
}

func TestMyCourses_CourseServiceNon200(t *testing.T) {
	// Course service returns 404 → fetchCourseDetails returns err → fallback slug-only row
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer courseSvc.Close()

	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"missing-course", int64(0), int64(0), nil})
	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: courseSvc.URL},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest(http.MethodGet, "/api/my/courses", http.NoBody)
	req.Header.Set("Authorization", htAuthHeader(t, "student"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestMyCourses_CourseServiceBadJSON(t *testing.T) {
	// Course service returns 200 but invalid JSON → fetchCourseDetails decode fails → fallback
	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-json"))
	}))
	defer courseSvc.Close()

	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"bad-json-course", int64(0), int64(0), nil})
	s := &State{
		Pool:   pool,
		Config: &config.Config{JWTSecret: htSecret, JWTExpiryH: htExpiry, CourseServiceURL: courseSvc.URL},
	}
	r := BuildRouter(s, s.Config, pool, false)

	req := httptest.NewRequest(http.MethodGet, "/api/my/courses", http.NoBody)
	req.Header.Set("Authorization", htAuthHeader(t, "student"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── Admin Courses ─────────────────────────────────────────────────────────────

func TestListCourseEnrollments_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminEnrollUser_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db down"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments", `{"user_id":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminEnrollUser_MissingUserID(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments", `{"user_id":""}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestAdminEnrollUser_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments", `{"user_id":"user-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminUnenrollUser_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/courses/my-course/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminEnrollGroup_MissingGroupID(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments/groups", `{"group_id":""}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestAdminEnrollGroup_Success(t *testing.T) {
	pool := &fake.Pool{}
	// 2 Exec calls: group_enrollments link + backfill enrollments → both from empty → OK
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments/groups", `{"group_id":"group-uuid-1"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminUnenrollGroup_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/courses/my-course/enrollments/groups/group-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestAdminListGroupEnrollments_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── SyncProgress ──────────────────────────────────────────────────────────────

func TestSyncProgress_MissingFields(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"user_id":"uuid-1","course_slug":""}`

	rec := htDo(t, r, "POST", "/api/admin/sync-progress", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestSyncProgress_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"user_id":"uuid-1","course_slug":"my-course","lesson_slug":"intro"}`

	rec := htDo(t, r, "POST", "/api/admin/sync-progress", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestSyncProgress_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/sync-progress", "not-json", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestSyncProgress_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)
	body := `{"user_id":"uuid-1","course_slug":"my-course","lesson_slug":"intro"}`

	rec := htDo(t, r, "POST", "/api/admin/sync-progress", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListCourseEnrollments_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"user-uuid-1", "alice", "alice@example.com", "2024-01-01"},
		[]any{"user-uuid-2", "bob", "bob@example.com", "2024-01-02"},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	enrollments := resp["enrollments"].([]any)
	if len(enrollments) != 2 {
		t.Errorf("want 2 enrollments, got %d", len(enrollments))
	}
}

func TestListCourseEnrollments_DBError(t *testing.T) {
	pool := &fake.Pool{} // no rows queued → Query returns error
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminUnenrollUser_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/courses/my-course/enrollments/user-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminEnrollGroup_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)
	body := `{"group_id":"group-uuid-1"}`

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments/groups", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListAuthProviders_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"local", int64(10)},
		[]any{"github", int64(3)},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/providers", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	providers := resp["providers"].([]any)
	if len(providers) != 2 {
		t.Errorf("want 2 providers, got %d", len(providers))
	}
}

func TestListAuthProviders_DBError(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/providers", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminListGroupEnrollments_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"group-uuid-1", "devops-team", "local", int64(5), "2024-01-01"},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	groups := resp["groups"].([]any)
	if len(groups) != 1 {
		t.Errorf("want 1 group, got %d", len(groups))
	}
}

func TestAdminListGroupEnrollments_DBError(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/courses/my-course/enrollments/groups", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestAdminUnenrollGroup_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/courses/my-course/enrollments/groups/group-uuid-1", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestRegister_Success(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 2)                                               // registration_enabled, email_whitelist
	skipErrRows(pool, 3)                                               // validatePasswordPolicy: min_length, uppercase, number
	pool.PushRow(nil, int64(0))                                        // COUNT(*) → no conflict
	pool.PushRow(nil, "new-user-uuid", "testuser", "test@example.com", // INSERT RETURNING
		"student", nil, nil, true, "local", "2024-01-01")
	pool.PushRow(nil, "group-uuid-1") // addToDefaultGroup: INSERT INTO groups RETURNING
	r := newTestRouter(pool)
	body := `{"username":"testuser","email":"test@example.com","password":"ValidPass9"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMyCourses_WithCourseData(t *testing.T) {
	mockCourse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"slug":             "kubernetes",
			"id":               "k8s-1",
			"title":            "Kubernetes",
			"description":      "Learn K8s",
			"category":         "devops",
			"difficulty":       "beginner",
			"is_public":        true,
			"lab_count":        3,
			"enrollment_count": 100,
		})
	}))
	defer mockCourse.Close()

	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"kubernetes", int64(2), int64(150), "2024-01-15"},
	)

	cfg := &config.Config{
		JWTSecret:        htSecret,
		JWTExpiryH:       htExpiry,
		CORSOrigins:      []string{"*"},
		CourseServiceURL: mockCourse.URL,
	}
	s := &State{Pool: pool, Config: cfg}
	r := BuildRouter(s, cfg, pool, false)

	rec := htDo(t, r, "GET", "/api/my/courses", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	courses := resp["courses"].([]any)
	if len(courses) != 1 {
		t.Errorf("want 1 course, got %d", len(courses))
	}
}

// ── Internal: Enrollments ─────────────────────────────────────────────────────

func TestInternalAutoEnroll_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/internal/enrollments/auto", "bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestInternalAutoEnroll_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)
	body := `{"user_id":"user-uuid-1","course_slug":"my-course"}`

	rec := htDo(t, r, "POST", "/internal/enrollments/auto", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestInternalAutoEnroll_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"user_id":"user-uuid-1","course_slug":"my-course"}`

	rec := htDo(t, r, "POST", "/internal/enrollments/auto", body, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestInternalCheckEnrollment_Enrolled(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, true)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/enrollments/check?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]bool
	json.NewDecoder(rec.Body).Decode(&resp)

	if !resp["enrolled"] {
		t.Error("expected enrolled=true")
	}
}

func TestInternalCheckEnrollment_NotEnrolled(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, false)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/enrollments/check?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]bool
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["enrolled"] {
		t.Error("expected enrolled=false")
	}
}

func TestInternalCheckEnrollment_DBError(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/enrollments/check?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── Internal: Progress ────────────────────────────────────────────────────────

func TestInternalViewedLessons_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/viewed?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	viewed := resp["viewed"].([]any)
	if len(viewed) != 0 {
		t.Errorf("expected 0 viewed lessons, got %d", len(viewed))
	}
}

func TestInternalViewedLessons_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil, []any{"intro"}, []any{"part-2"})
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/viewed?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	viewed := resp["viewed"].([]any)
	if len(viewed) != 2 {
		t.Errorf("expected 2 viewed, got %d", len(viewed))
	}
}

func TestInternalMarkComplete_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/internal/progress/complete", "bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestInternalMarkComplete_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"user_id":"uuid-1","course_slug":"my-course","lesson_slug":"intro"}`

	rec := htDo(t, r, "POST", "/internal/progress/complete", body, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestInternalRecordModuleProgress_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/internal/progress/module", "bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestInternalRecordModuleProgress_Success(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"user_id":"uuid-1","course_slug":"my-course","module_index":0,"score":80,"max_score":100,"passed":true}`

	rec := htDo(t, r, "POST", "/internal/progress/module", body, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestInternalGetModuleProgress_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/modules?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestInternalGetModuleProgress_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{0, 80, 100, true, 2},
		[]any{1, 60, 100, false, 1},
	)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/modules?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	progress := resp["progress"].([]any)
	if len(progress) != 2 {
		t.Errorf("expected 2 progress entries, got %d", len(progress))
	}
}

func TestInternalCourseSummary_DBError(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/course-summary?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestInternalCourseSummary_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, 180) // total_score
	pool.PushRows(nil,     // passed module slugs
		[]any{"module-1"},
		[]any{"module-2"},
	)
	pool.PushRow(nil, 5) // viewed_count
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/course-summary?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["total_score"] == nil {
		t.Error("expected total_score in response")
	}

	modules := resp["passed_modules"].([]any)
	if len(modules) != 2 {
		t.Errorf("expected 2 passed modules, got %d", len(modules))
	}
}

// ── OAuth handlers ────────────────────────────────────────────────────────────

func newOAuthRouter(pool *fake.Pool, providers ...config.ProviderConfig) http.Handler {
	cfg := &config.Config{
		JWTSecret:         htSecret,
		JWTExpiryH:        htExpiry,
		CORSOrigins:       []string{"*"},
		OAuthRedirectBase: "http://localhost:3000",
		Providers:         providers,
	}
	s := &State{Pool: pool, Config: cfg}

	return BuildRouter(s, cfg, pool, false)
}

func TestOAuthAuthorize_UnknownProvider(t *testing.T) {
	pool := &fake.Pool{}
	r := newOAuthRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/oauth/unknown/authorize", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOAuthAuthorize_GitHub(t *testing.T) {
	pool := &fake.Pool{}
	r := newOAuthRouter(pool, config.ProviderConfig{
		ID: "github", Name: "GitHub", ClientID: "gh-client-id",
	})

	rec := htDo(t, r, "GET", "/api/auth/oauth/github/authorize", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["url"] == "" {
		t.Error("expected non-empty authorize URL")
	}
}

func TestOAuthAuthorize_OIDCMissingIssuer(t *testing.T) {
	pool := &fake.Pool{}
	r := newOAuthRouter(pool, config.ProviderConfig{
		ID: "custom-oidc", Name: "Custom OIDC", ClientID: "oidc-id",
		// No IssuerURL, no well-known mapping → empty
	})

	rec := htDo(t, r, "GET", "/api/auth/oauth/custom-oidc/authorize", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOAuthCallback_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newOAuthRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/oauth/callback", "not-json", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOAuthCallback_InvalidState(t *testing.T) {
	pool := &fake.Pool{}
	r := newOAuthRouter(pool)
	body := `{"code":"some-code","state":"invalid-state-token"}`

	rec := htDo(t, r, "POST", "/api/auth/oauth/callback", body, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestResolveIssuerURL_KnownProviders(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"gitlab", "https://gitlab.com"},
		{"google", "https://accounts.google.com"},
	}
	for _, tt := range tests {
		p := &config.ProviderConfig{ID: tt.id}

		got := resolveIssuerURL(p)
		if got != tt.want {
			t.Errorf("resolveIssuerURL(%q): want %q, got %q", tt.id, tt.want, got)
		}
	}
}

func TestResolveIssuerURL_CustomIssuer(t *testing.T) {
	p := &config.ProviderConfig{ID: "keycloak", IssuerURL: "https://auth.example.com/realm/test"}

	got := resolveIssuerURL(p)
	if got != "https://auth.example.com/realm/test" {
		t.Errorf("expected custom issuer, got %q", got)
	}
}

func TestResolveIssuerURL_Unknown(t *testing.T) {
	p := &config.ProviderConfig{ID: "unknown-provider"}

	got := resolveIssuerURL(p)
	if got != "" {
		t.Errorf("expected empty for unknown provider, got %q", got)
	}
}

// ── Patterns ──────────────────────────────────────────────────────────────────

// patternRow returns 13 fake column values for scanPattern.
func patternRow() []any {
	return []any{"uuid-ptn-1", "callout", "Callout", "A callout box", "", "<div>{{content}}</div>", ".callout{}", "", "global", false, nil, nil, nil}
}

func TestListPatterns_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil, patternRow())
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	patterns := resp["patterns"].([]any)
	if len(patterns) != 1 {
		t.Errorf("want 1 pattern, got %d", len(patterns))
	}
}

func TestListPatterns_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil) // no rows
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	patterns := resp["patterns"].([]any)
	if len(patterns) != 0 {
		t.Errorf("want empty patterns, got %d", len(patterns))
	}
}

func TestListPatterns_DBError(t *testing.T) {
	pool := &fake.Pool{} // no rows queued → Query returns error
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestGetPattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, patternRow()...)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns/550e8400-e29b-41d4-a716-446655440000", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPattern_InvalidUUID(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns/not-a-uuid", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestGetPattern_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("no rows"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/patterns/550e8400-e29b-41d4-a716-446655440000", "", htAuthHeader(t, "student"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestCreatePattern_Unauthorized(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"name":"callout","label":"Callout","html":"<div></div>"}`

	rec := htDo(t, r, "POST", "/api/admin/patterns", body, htAuthHeader(t, "student"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestCreatePattern_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/patterns", "not-json{", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestCreatePattern_MissingFields(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"name":"callout","label":"Callout"}` // missing html

	rec := htDo(t, r, "POST", "/api/admin/patterns", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestCreatePattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, patternRow()...)
	r := newTestRouter(pool)
	body := `{"name":"callout","label":"Callout","html":"<div>{{content}}</div>"}`

	rec := htDo(t, r, "POST", "/api/admin/patterns", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusCreated {
		t.Errorf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePattern_Conflict(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("unique violation"))
	r := newTestRouter(pool)
	body := `{"name":"callout","label":"Callout","html":"<div></div>"}`

	rec := htDo(t, r, "POST", "/api/admin/patterns", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rec.Code)
	}
}

func TestUpdatePattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, patternRow()...)
	r := newTestRouter(pool)
	body := `{"label":"Updated Callout","html":"<span>{{content}}</span>"}`

	rec := htDo(t, r, "PUT", "/api/admin/patterns/callout", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdatePattern_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("no rows"))
	r := newTestRouter(pool)
	body := `{"label":"Updated","html":"<span></span>"}`

	rec := htDo(t, r, "PUT", "/api/admin/patterns/nonexistent", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestUpdatePattern_MissingHTML(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	body := `{"label":"Updated"}`

	rec := htDo(t, r, "PUT", "/api/admin/patterns/callout", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestDeletePattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/patterns/callout", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rec.Code)
	}
}

func TestDeletePattern_NotFound(t *testing.T) {
	pool := &fake.Pool{} // empty exec queue → "OK" tag → RowsAffected()=0 → 404
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/patterns/nonexistent", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

// ── OAuth helpers ─────────────────────────────────────────────────────────────

func TestMakeOAuthState(t *testing.T) {
	tok, err := makeOAuthState("github", htSecret)
	if err != nil {
		t.Fatalf("makeOAuthState: %v", err)
	}

	if tok == "" {
		t.Error("expected non-empty state token")
	}
}

func TestDecodeOAuthState_Valid(t *testing.T) {
	tok, err := makeOAuthState("github", htSecret)
	if err != nil {
		t.Fatal(err)
	}

	provider, ok := decodeOAuthState(tok, htSecret)
	if !ok {
		t.Error("expected ok=true for valid state token")
	}

	if provider != "github" {
		t.Errorf("want provider=github, got %q", provider)
	}
}

func TestDecodeOAuthState_WrongSecret(t *testing.T) {
	tok, _ := makeOAuthState("github", htSecret)

	_, ok := decodeOAuthState(tok, "wrong-secret-key-32-bytes-long!!!")
	if ok {
		t.Error("expected ok=false for wrong secret")
	}
}

func TestDecodeOAuthState_Garbage(t *testing.T) {
	_, ok := decodeOAuthState("not.a.valid.token", htSecret)
	if ok {
		t.Error("expected ok=false for garbage token")
	}
}

func TestDecodeOAuthState_Empty(t *testing.T) {
	_, ok := decodeOAuthState("", htSecret)
	if ok {
		t.Error("expected ok=false for empty token")
	}
}

func TestSanitizeUsername_Normal(t *testing.T) {
	if got := sanitizeUsername("John Doe"); got != "JohnDoe" {
		t.Errorf("want JohnDoe, got %q", got)
	}
}

func TestSanitizeUsername_Empty(t *testing.T) {
	if got := sanitizeUsername(""); got != "user" {
		t.Errorf("want user, got %q", got)
	}
}

func TestSanitizeUsername_SpecialChars(t *testing.T) {
	got := sanitizeUsername("user@example.com!")
	if got != "userexamplecom" {
		t.Errorf("want userexamplecom, got %q", got)
	}
}

func TestSanitizeUsername_Truncated(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyzabcdefghijklmno" // 41 chars

	got := sanitizeUsername(long)
	if len(got) > 32 {
		t.Errorf("expected max 32 chars, got %d: %q", len(got), got)
	}
}

func TestSanitizeUsername_ValidChars(t *testing.T) {
	if got := sanitizeUsername("user_name-123"); got != "user_name-123" {
		t.Errorf("want user_name-123, got %q", got)
	}
}

func TestListProviders_NoProviders(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/oauth/providers", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	providers := resp["providers"].([]any)
	if len(providers) != 0 {
		t.Errorf("want empty providers list, got %d", len(providers))
	}
}

func TestListProviders_WithProviders(t *testing.T) {
	pool := &fake.Pool{}
	cfg := &config.Config{
		JWTSecret:   htSecret,
		JWTExpiryH:  htExpiry,
		CORSOrigins: []string{"*"},
		Providers: []config.ProviderConfig{
			{ID: "github", Name: "GitHub", ClientID: "gh-client-id"},
			{ID: "empty-provider", Name: "No Client", ClientID: ""},
		},
	}
	s := &State{Pool: pool, Config: cfg}
	r := BuildRouter(s, cfg, pool, false)

	rec := htDo(t, r, "GET", "/api/auth/oauth/providers", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	providers := resp["providers"].([]any)
	if len(providers) != 1 {
		t.Errorf("want 1 provider (empty filtered), got %d", len(providers))
	}

	p := providers[0].(map[string]any)
	if p["id"] != "github" {
		t.Errorf("want id=github, got %v", p["id"])
	}
}

// ── Pure helper function tests ────────────────────────────────────────────────

func TestNullStr_Empty(t *testing.T) {
	if nullStr("") != nil {
		t.Error("expected nil for empty string")
	}
}

func TestNullStr_NonEmpty(t *testing.T) {
	s := nullStr("hello")
	if s == nil || *s != "hello" {
		t.Errorf("expected *string='hello', got %v", s)
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

// ── extractURLBase (oidc.go) ──────────────────────────────────────────────────

func TestExtractURLBase_Valid(t *testing.T) {
	got := extractURLBase("https://sso.example.com/auth/realms/master")
	if got != "https://sso.example.com" {
		t.Errorf("want https://sso.example.com, got %q", got)
	}
}

func TestExtractURLBase_NoHost(t *testing.T) {
	got := extractURLBase("not-a-url")
	if got != "not-a-url" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestExtractURLBase_JustHost(t *testing.T) {
	got := extractURLBase("http://localhost:8080")
	if got != "http://localhost:8080" {
		t.Errorf("want http://localhost:8080, got %q", got)
	}
}

// ── viewedLessons direct call ─────────────────────────────────────────────────

func TestViewedLessons_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	s := &State{Pool: pool, Config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	result := viewedLessons(s, req, "some-course", "user-id")
	if result != nil {
		t.Errorf("expected nil map on error, got %v", result)
	}
}

func TestViewedLessons_WithData(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"intro"},
		[]any{"advanced"},
	)
	s := &State{Pool: pool, Config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	result := viewedLessons(s, req, "my-course", "user-id")
	if result == nil {
		t.Fatal("expected non-nil map")
	}

	if !result["intro"] || !result["advanced"] {
		t.Errorf("expected intro and advanced in result: %v", result)
	}
}

func TestViewedLessons_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	s := &State{Pool: pool, Config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	result := viewedLessons(s, req, "my-course", "user-id")
	if result == nil {
		t.Fatal("expected non-nil map (empty)")
	}

	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// ── Course patterns (direct handler calls with chi context) ───────────────────

func newStateWithPool(pool *fake.Pool) *State {
	return &State{
		Pool: pool,
		Config: &config.Config{
			JWTSecret:  htSecret,
			JWTExpiryH: htExpiry,
		},
	}
}

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func withAuthAndChiParam(r *http.Request, role, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	claims := &apimiddleware.Claims{Email: "test@example.com", Role: role}
	claims.Subject = "user-uuid-1"
	ctx = context.WithValue(ctx, apimiddleware.ClaimsKey, claims)

	return r.WithContext(ctx)
}

func TestListCoursePatterns_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil,
		[]any{"uuid-p1", "callout", "Callout", "A callout", "", "<div>{{content}}</div>", "", "", "my-course", false, nil, nil, nil},
	)
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/courses/my-course/patterns", http.NoBody), "slug", "my-course")
	s.ListCoursePatterns(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListCoursePatterns_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/courses/my-course/patterns", http.NoBody), "slug", "my-course")
	s.ListCoursePatterns(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestListCoursePatterns_Empty(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/courses/my-course/patterns", http.NoBody), "slug", "my-course")
	s.ListCoursePatterns(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestCreateCoursePattern_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	req := withAuthAndChiParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("bad json")), "admin", "slug", "my-course")
	s.CreateCoursePattern(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestCreateCoursePattern_MissingFields(t *testing.T) {
	pool := &fake.Pool{}
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	req := withAuthAndChiParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`)), "admin", "slug", "my-course")
	s.CreateCoursePattern(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateCoursePattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "uuid-1", "callout", "Callout", "A callout", "", "<div>{{content}}</div>", "", "", "my-course", false, nil, nil, nil)
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	body := `{"name":"callout","label":"Callout","html":"<div>{{content}}</div>"}`
	req := withAuthAndChiParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), "admin", "slug", "my-course")
	s.CreateCoursePattern(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateCoursePattern_Conflict(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("unique constraint violation"))
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	body := `{"name":"callout","label":"Callout","html":"<div>{{content}}</div>"}`
	req := withAuthAndChiParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), "admin", "slug", "my-course")
	s.CreateCoursePattern(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rec.Code)
	}
}

func TestDeleteCoursePattern_InvalidUUID(t *testing.T) {
	pool := &fake.Pool{}
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "my-course")
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest(http.MethodDelete, "/", http.NoBody).WithContext(
		context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	s.DeleteCoursePattern(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestDeleteCoursePattern_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil)
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "my-course")
	rctx.URLParams.Add("id", "550e8400-e29b-41d4-a716-446655440000")
	req := httptest.NewRequest(http.MethodDelete, "/", http.NoBody).WithContext(
		context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	s.DeleteCoursePattern(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rec.Code)
	}
}

func TestDeleteCoursePattern_NotFound(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, nil)
	s := newStateWithPool(pool)
	rec := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "my-course")
	rctx.URLParams.Add("id", "550e8400-e29b-41d4-a716-446655440000")
	req := httptest.NewRequest(http.MethodDelete, "/", http.NoBody).WithContext(
		context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	s.DeleteCoursePattern(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

// ── upsertSSOUser direct calls ────────────────────────────────────────────────

func TestUpsertSSOUser_NewUser(t *testing.T) {
	pool := &fake.Pool{}
	// First SELECT by provider/providerUserID → not found
	pool.PushRow(errors.New("not found"))
	// Second SELECT by email → not found
	pool.PushRow(errors.New("not found"))
	// COUNT(*) for username conflict → 0
	pool.PushRow(nil, int64(0))
	// INSERT RETURNING
	pool.PushRow(nil, "new-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-123")
	if err != nil {
		t.Fatalf("upsertSSOUser: %v", err)
	}

	if u.Email != "john@example.com" {
		t.Errorf("email: want john@example.com, got %q", u.Email)
	}
}

func TestUpsertSSOUser_ExistingByProvider(t *testing.T) {
	pool := &fake.Pool{}
	// SELECT by provider → found
	pool.PushRow(nil, "existing-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")
	// UPDATE (sync avatar/bio) → success
	pool.PushRow(nil, "existing-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-123")
	if err != nil {
		t.Fatalf("upsertSSOUser: %v", err)
	}

	if u.ID != "existing-uuid" {
		t.Errorf("ID: want existing-uuid, got %q", u.ID)
	}
}

func TestUpsertSSOUser_ExistingByEmail_SameProvider(t *testing.T) {
	pool := &fake.Pool{}
	// SELECT by provider → not found
	pool.PushRow(errors.New("not found"))
	// SELECT by email → found (same provider)
	pool.PushRow(nil, "email-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")
	// UPDATE → success
	pool.PushRow(nil, "email-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-456")
	if err != nil {
		t.Fatalf("upsertSSOUser: %v", err)
	}

	if u.ID != "email-uuid" {
		t.Errorf("ID: want email-uuid, got %q", u.ID)
	}
}

func TestUpsertSSOUser_ExistingByEmail_DifferentProvider(t *testing.T) {
	pool := &fake.Pool{}
	// SELECT by provider → not found
	pool.PushRow(errors.New("not found"))
	// SELECT by email → found (different provider: "local")
	pool.PushRow(nil, "local-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "local", "2024-01-01")

	_, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-123")
	if err == nil {
		t.Error("expected error for different auth provider")
	}
}

func TestUpsertSSOUser_UsernameConflict(t *testing.T) {
	pool := &fake.Pool{}
	// SELECT by provider → not found
	pool.PushRow(errors.New("not found"))
	// SELECT by email → not found
	pool.PushRow(errors.New("not found"))
	// COUNT for username → taken
	pool.PushRow(nil, int64(1))
	// INSERT RETURNING (with suffixed username)
	pool.PushRow(nil, "new-uuid", "johndoe_abc123", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-789")
	if err != nil {
		t.Fatalf("upsertSSOUser: %v", err)
	}

	if u.Email != "john@example.com" {
		t.Errorf("email mismatch: %q", u.Email)
	}
}

// ── syncGroupsAndDeriveRole direct calls ──────────────────────────────────────

func TestSyncGroupsAndDeriveRole_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("db error")) // SELECT role

	_, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"admins"}, "oidc")
	if err == nil {
		t.Error("expected error when SELECT role fails")
	}
}

func TestSyncGroupsAndDeriveRole_NoGroups(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student") // SELECT role

	role, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{}, "oidc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if role != "student" {
		t.Errorf("expected student, got %q", role)
	}
}

func TestSyncGroupsAndDeriveRole_WithGroup(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student") // SELECT role
	// For each group name: INSERT group, INSERT user_groups, SELECT mapped_role
	pool.PushRow(nil, "group-uuid-1") // INSERT/SELECT group
	pool.PushExec(1, nil)             // INSERT user_groups
	pool.PushRow(nil, "admin")        // SELECT mapped_role → admin
	pool.PushExec(1, nil)             // UPDATE users SET role

	role, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"admins"}, "oidc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if role != "admin" {
		t.Errorf("expected admin, got %q", role)
	}
}

// ── OIDCAuthorize (disabled) ──────────────────────────────────────────────────

func TestOIDCAuthorize_Disabled(t *testing.T) {
	pool := &fake.Pool{}
	// loadOIDCSettings calls ReadSetting 9 times; all return defaults (push error rows)
	for range 10 {
		pool.PushRow(errors.New("no row"))
	}

	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/oidc/authorize", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (OIDC disabled), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCCallback_InvalidState(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/oidc/callback", `{"code":"c","state":"bad-state"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestOIDCCallback_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/oidc/callback", "not-json", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── OAuthCallback provider not found path ─────────────────────────────────────

func TestOAuthCallback_GithubProviderFetchFails(t *testing.T) {
	// GitHub provider is configured but fetchGitHub will fail (invalid code → 401)
	pool := &fake.Pool{}
	s := &State{
		Pool: pool,
		Config: &config.Config{
			JWTSecret:  htSecret,
			JWTExpiryH: htExpiry,
			Providers: []config.ProviderConfig{
				{ID: "github", Name: "GitHub", ClientID: "fake-client-id", ClientSecret: "fake-secret"},
			},
		},
	}
	r := BuildRouter(s, s.Config, pool, false)

	state, err := makeOAuthState("github", htSecret)
	if err != nil {
		t.Fatalf("makeOAuthState: %v", err)
	}

	body := fmt.Sprintf(`{"code":"invalid-code","state":%q}`, state)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	// fetchGitHub will fail (network error or bad code response)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when fetchGitHub fails, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOAuthCallback_ValidStateUnknownProvider(t *testing.T) {
	pool := &fake.Pool{}
	cfg := &config.Config{
		JWTSecret:   htSecret,
		JWTExpiryH:  htExpiry,
		CORSOrigins: []string{"*"},
		Providers:   []config.ProviderConfig{},
	}
	s := &State{Pool: pool, Config: cfg}
	r := BuildRouter(s, cfg, pool, false)

	// Create a valid state token for an unknown provider
	stateToken, _ := makeOAuthState("unknown-provider", htSecret)
	body := fmt.Sprintf(`{"code":"test-code","state":%q}`, stateToken)

	rec := htDo(t, r, "POST", "/api/auth/oauth/callback", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (unknown provider), got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── LoadPatternsFromConfig ────────────────────────────────────────────────────

func TestLoadPatternsFromConfig_FileNotFound(t *testing.T) {
	pool := &fake.Pool{}

	err := LoadPatternsFromConfig(context.Background(), pool, "/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadPatternsFromConfig_InvalidYAML(t *testing.T) {
	pool := &fake.Pool{}

	tmpFile, err := os.CreateTemp(t.TempDir(), "patterns-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("not: valid: yaml: :")
	tmpFile.Close()

	err = LoadPatternsFromConfig(context.Background(), pool, tmpFile.Name())
	if err != nil {
		t.Logf("got error (may be OK for YAML parse): %v", err)
	}
}

func TestLoadPatternsFromConfig_Success(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil) // INSERT for one pattern

	yaml := `patterns:
  - name: callout
    label: Callout
    description: A callout box
    html: "<div class=\"callout\">{{content}}</div>"
    scope: global
`

	tmpFile, err := os.CreateTemp(t.TempDir(), "patterns-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(yaml)
	tmpFile.Close()

	err = LoadPatternsFromConfig(context.Background(), pool, tmpFile.Name())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadPatternsFromConfig_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))

	yaml := `patterns:
  - name: callout
    label: Callout
    html: "<div>{{content}}</div>"
`

	tmpFile, err := os.CreateTemp(t.TempDir(), "patterns-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(yaml)
	tmpFile.Close()

	err = LoadPatternsFromConfig(context.Background(), pool, tmpFile.Name())
	if err == nil {
		t.Error("expected DB error")
	}
}

// ── doGet (oauth.go) ─────────────────────────────────────────────────────────

func TestDoGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"johndoe","id":12345}`))
	}))
	defer srv.Close()

	var result map[string]any

	err := doGet(&http.Client{}, srv.URL, "mytoken", &result)
	if err != nil {
		t.Fatalf("doGet: %v", err)
	}

	if result["login"] != "johndoe" {
		t.Errorf("expected login=johndoe, got %v", result["login"])
	}
}

func TestDoGet_NoBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var result map[string]any

	err := doGet(&http.Client{}, srv.URL, "", &result)
	if err != nil {
		t.Fatalf("doGet no bearer: %v", err)
	}
}

func TestDoGet_BadURL(t *testing.T) {
	var result map[string]any

	err := doGet(&http.Client{}, "http://127.0.0.1:0/invalid", "", &result)
	if err == nil {
		t.Error("expected error for bad URL")
	}
}

func TestDoGet_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	var result map[string]any

	err := doGet(&http.Client{}, srv.URL, "", &result)
	if err == nil {
		t.Error("expected JSON parse error")
	}
}

// ── oidcContext (oidc.go) ─────────────────────────────────────────────────────

func TestOIDCContext_WithIssuerURL(t *testing.T) {
	ctx := context.Background()
	cfg := oidcSettings{IssuerURL: "https://issuer.example.com"}

	result := oidcContext(ctx, cfg)
	if result == ctx {
		t.Error("expected modified context when IssuerURL is set")
	}
}

func TestOIDCContext_WithoutIssuerURL(t *testing.T) {
	ctx := context.Background()
	cfg := oidcSettings{}

	result := oidcContext(ctx, cfg)
	if result != ctx {
		t.Error("expected same context when IssuerURL is empty")
	}
}

// ── loadOIDCSettings (oidc.go) ────────────────────────────────────────────────

func TestLoadOIDCSettings_EnabledAndConfigured(t *testing.T) {
	pool := &fake.Pool{}
	// Push "true" for oidc_enabled
	pool.PushRow(nil, "true")
	// oidc_provider_url
	pool.PushRow(nil, "https://sso.example.com/realms/master")
	// oidc_issuer_url
	pool.PushRow(nil, "https://sso.example.com/realms/master")
	// oidc_client_id
	pool.PushRow(nil, "my-client")
	// oidc_client_secret
	pool.PushRow(nil, "my-secret")
	// oidc_group_claim
	pool.PushRow(nil, "groups")
	// oidc_redirect_base
	pool.PushRow(nil, "http://localhost:3000")
	// oidc_browser_base_url
	pool.PushRow(nil, "")
	// oidc_scopes
	pool.PushRow(nil, "openid email profile")
	s := &State{Pool: pool, Config: &config.Config{OAuthRedirectBase: "http://localhost:3000"}}

	cfg, err := s.loadOIDCSettings(context.Background())
	if err != nil {
		t.Fatalf("loadOIDCSettings: %v", err)
	}

	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}

	if cfg.ClientID != "my-client" {
		t.Errorf("ClientID: want my-client, got %q", cfg.ClientID)
	}
}

func TestLoadOIDCSettings_MissingClientID(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")                    // oidc_enabled
	pool.PushRow(nil, "https://sso.example.com") // oidc_provider_url
	pool.PushRow(errors.New("no row"))           // oidc_issuer_url → empty
	pool.PushRow(errors.New("no row"))           // oidc_client_id → empty
	pool.PushRow(nil, "my-secret")               // oidc_client_secret
	pool.PushRow(errors.New("no row"))           // oidc_group_claim → default
	pool.PushRow(errors.New("no row"))           // oidc_redirect_base → default
	pool.PushRow(errors.New("no row"))           // oidc_browser_base_url → empty
	pool.PushRow(errors.New("no row"))           // oidc_scopes → default
	s := &State{Pool: pool, Config: &config.Config{OAuthRedirectBase: "http://localhost:3000"}}

	_, err := s.loadOIDCSettings(context.Background())
	if err == nil {
		t.Error("expected error for missing client_id")
	}
}

// ── More groups handler tests ─────────────────────────────────────────────────

func TestUpsertGroupMapping_InvalidPlatformRole(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups/mappings", `{"group_name":"admins","platform_role":"superuser"}`, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid platform_role, got %d", rec.Code)
	}
}

func TestUpsertGroupMapping_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/admin/groups/mappings", "bad-json", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestDeleteGroupMapping_DBError2(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "DELETE", "/api/admin/groups/mappings/testers", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── More internal handlers ────────────────────────────────────────────────────

func TestInternalMarkComplete_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/internal/progress/complete", `{"user_id":"uid","course_slug":"c","lesson_slug":"l"}`, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestInternalRecordModuleProgress_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(0, errors.New("db error"))
	r := newTestRouter(pool)
	body := `{"user_id":"uid","course_slug":"c","module_index":0,"score":10,"max_score":10,"passed":true}`

	rec := htDo(t, r, "POST", "/internal/progress/module", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestInternalGetModuleProgress_NoProgress(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(nil)
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/modules?user_id=uid&course_slug=c", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── Register additional coverage ──────────────────────────────────────────────

func TestRegister_InsertError(t *testing.T) {
	pool := &fake.Pool{}
	skipErrRows(pool, 5)
	pool.PushRow(nil, int64(0))
	pool.PushRow(errors.New("db error"))
	r := newTestRouter(pool)
	body := `{"username":"validuser","email":"t@test.com","password":"TestPass99"}`

	rec := htDo(t, r, "POST", "/api/auth/register", body, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── UpdateUser additional coverage ───────────────────────────────────────────

func TestUpdateUser_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/users/user-uuid-1", "not-json", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── SearchUsers additional coverage ──────────────────────────────────────────

func TestSearchUsers_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/admin/users/search?q=alice", "", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── AdminEnrollGroup additional coverage ─────────────────────────────────────

func TestAdminEnrollGroup_BackfillError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushExec(1, nil)
	pool.PushExec(0, errors.New("backfill err"))
	r := newTestRouter(pool)
	body := `{"group_id":"group-uuid-1"}`

	rec := htDo(t, r, "POST", "/api/admin/courses/my-course/enrollments/groups", body, htAuthHeader(t, "admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── UpdateSettings additional coverage ───────────────────────────────────────

func TestUpdateSettings_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "PUT", "/api/admin/settings", "not-json", htAuthHeader(t, "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── syncGroupsAndDeriveRole additional coverage ───────────────────────────────

func TestSyncGroupsAndDeriveRole_EmptyGroupName(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student")
	pool.PushExec(1, nil) // DELETE

	role, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{""}, "oidc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if role != "student" {
		t.Errorf("expected student, got %q", role)
	}
}

func TestSyncGroupsAndDeriveRole_DeleteError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student")
	pool.PushExec(0, errors.New("delete error"))

	_, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"admins"}, "oidc")
	if err == nil {
		t.Error("expected error when DELETE fails")
	}
}

func TestSyncGroupsAndDeriveRole_UserGroupsInsertError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student")
	pool.PushRow(nil, "group-uuid")
	pool.PushExec(1, nil)                      // DELETE
	pool.PushExec(0, errors.New("link error")) // INSERT user_groups

	_, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"group1"}, "oidc")
	if err == nil {
		t.Error("expected error when INSERT user_groups fails")
	}
}

func TestSyncGroupsAndDeriveRole_NonAdminRoleMapping(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student")
	pool.PushRow(nil, "group-uuid")
	pool.PushRow(nil, "student") // mapped_role
	pool.PushExec(1, nil)        // DELETE
	pool.PushExec(1, nil)        // INSERT user_groups
	pool.PushExec(1, nil)        // UPDATE users

	role, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"students"}, "oidc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if role != "student" {
		t.Errorf("expected student, got %q", role)
	}
}

func TestSyncGroupsAndDeriveRole_UpdateRoleError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "student")
	pool.PushRow(nil, "group-uuid")
	pool.PushRow(nil, "admin")                   // mapped_role
	pool.PushExec(1, nil)                        // DELETE
	pool.PushExec(1, nil)                        // INSERT user_groups
	pool.PushExec(0, errors.New("update error")) // UPDATE users

	_, err := syncGroupsAndDeriveRole(context.Background(), pool, "user-uuid", []string{"admins"}, "oidc")
	if err == nil {
		t.Error("expected error when UPDATE users fails")
	}
}

// ── upsertSSOUser additional coverage ────────────────────────────────────────

func TestUpsertSSOUser_ExistingByProvider_UpdateError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "existing-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "github", "2024-01-01")
	pool.PushRow(errors.New("update error"))

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.ID != "existing-uuid" {
		t.Errorf("expected existing-uuid, got %q", u.ID)
	}
}

func TestUpsertSSOUser_ExistingByEmail_UpdateError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("not found"))
	pool.PushRow(nil, "email-uuid", "johndoe", "john@example.com", "student", nil, nil, true, "", "2024-01-01")
	pool.PushRow(errors.New("update error"))

	u, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.ID != "email-uuid" {
		t.Errorf("expected email-uuid, got %q", u.ID)
	}
}

func TestUpsertSSOUser_InsertError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(errors.New("not found"))
	pool.PushRow(errors.New("not found"))
	pool.PushRow(nil, int64(0))
	pool.PushRow(errors.New("insert error"))

	_, err := upsertSSOUser(context.Background(), pool, "john@example.com", "John Doe", nil, nil, "github", "gh-789")
	if err == nil {
		t.Error("expected error when INSERT fails")
	}
}

// ── OIDCCallback additional coverage ─────────────────────────────────────────

func TestOIDCCallback_OIDCDisabled(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)
	state, _ := makeOAuthState("oidc", htSecret)
	body := fmt.Sprintf(`{"code":"auth-code","state":%q}`, state)

	rec := htDo(t, r, "POST", "/api/auth/oidc/callback", body, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (OIDC disabled), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCCallback_ProviderUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	pool := &fake.Pool{}
	pool.PushRow(nil, "true")
	pool.PushRow(nil, srv.URL)
	pool.PushRow(nil, "")
	pool.PushRow(nil, "my-client")
	pool.PushRow(nil, "my-secret")
	pool.PushRow(nil, "groups")
	pool.PushRow(nil, "")
	pool.PushRow(nil, "")
	pool.PushRow(nil, "openid email profile")
	r := newTestRouter(pool)
	state, _ := makeOAuthState("oidc", htSecret)
	body := fmt.Sprintf(`{"code":"code","state":%q}`, state)

	rec := htDo(t, r, "POST", "/api/auth/oidc/callback", body, "")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("want 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── loadLDAPSettings ──────────────────────────────────────────────────────────

func TestLoadLDAPSettings_Disabled(t *testing.T) {
	pool := &fake.Pool{}
	s := &State{Pool: pool, Config: &config.Config{}}

	_, err := s.loadLDAPSettings(context.Background())
	if err == nil {
		t.Error("expected error when LDAP is disabled")
	}
}

func TestLoadLDAPSettings_NotConfigured(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")
	s := &State{Pool: pool, Config: &config.Config{}}

	_, err := s.loadLDAPSettings(context.Background())
	if err == nil {
		t.Error("expected error when LDAP not fully configured")
	}
}

func TestLoadLDAPSettings_Configured(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, "true")
	pool.PushRow(nil, "ldap://ldap.example.com")
	pool.PushRow(nil, "cn=admin,dc=example,dc=com")
	pool.PushRow(nil, "secret")
	pool.PushRow(nil, "ou=users,dc=example,dc=com")
	pool.PushRow(nil, "(mail=%s)")
	pool.PushRow(nil, "")
	pool.PushRow(nil, "")
	s := &State{Pool: pool, Config: &config.Config{}}

	cfg, err := s.loadLDAPSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServerURL != "ldap://ldap.example.com" {
		t.Errorf("ServerURL: want ldap://ldap.example.com, got %q", cfg.ServerURL)
	}
}

// ── LDAPLogin early paths ─────────────────────────────────────────────────────

func TestLDAPLogin_InvalidJSON(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/ldap/login", "not-json", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestLDAPLogin_EmptyCredentials(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/ldap/login", `{"email":"","password":""}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestLDAPLogin_LDAPDisabled(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "POST", "/api/auth/ldap/login", `{"email":"user@example.com","password":"pass123"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 (LDAP disabled), got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── buildRestConfig (pattern_watcher.go) ─────────────────────────────────────

func TestBuildRestConfig_InvalidKubeconfig(t *testing.T) {
	_, err := buildRestConfig("/nonexistent/kubeconfig.yaml")
	if err == nil {
		t.Error("expected error for nonexistent kubeconfig")
	}
}

func TestBuildRestConfig_NoKubeconfig(t *testing.T) {
	_, err := buildRestConfig("")
	if err == nil {
		t.Error("expected error when not in k8s cluster")
	}
}

// ── PatternWatcher (pattern_watcher.go) ──────────────────────────────────────

func TestPatternWatcher_Upsert_NoSpec(t *testing.T) {
	pool := &fake.Pool{}
	w := &PatternWatcher{pool: pool}
	cr := &patternv1.MarkdownPattern{}
	cr.SetName("test-pattern")
	w.upsert(context.Background(), cr)
}

func TestPatternWatcher_Upsert_WithSpec(t *testing.T) {
	pool := &fake.Pool{}
	w := &PatternWatcher{pool: pool}
	cr := &patternv1.MarkdownPattern{
		Spec: patternv1.MarkdownPatternSpec{
			Name:  "my-pattern",
			Label: "My Pattern",
			HTML:  "<div>{{content}}</div>",
			Scope: "global",
		},
	}
	w.upsert(context.Background(), cr)
}

func TestPatternWatcher_Upsert_DefaultsApplied(t *testing.T) {
	pool := &fake.Pool{}
	w := &PatternWatcher{pool: pool}
	cr := &patternv1.MarkdownPattern{
		Spec: patternv1.MarkdownPatternSpec{
			HTML: "<div>test</div>",
		},
	}
	cr.SetName("fallback-name")
	w.upsert(context.Background(), cr)
}

func TestPatternWatcher_Delete_WithSpec(t *testing.T) {
	pool := &fake.Pool{}
	w := &PatternWatcher{pool: pool}
	cr := &patternv1.MarkdownPattern{
		Spec: patternv1.MarkdownPatternSpec{
			Name:  "my-pattern",
			Scope: "course",
		},
	}
	w.delete(context.Background(), cr)
}

func TestPatternWatcher_Delete_NoSpec(t *testing.T) {
	pool := &fake.Pool{}
	w := &PatternWatcher{pool: pool}
	cr := &patternv1.MarkdownPattern{}
	cr.SetName("fallback-pattern")
	w.delete(context.Background(), cr)
}

// ── InternalCourseSummary additional coverage ─────────────────────────────────

func TestInternalCourseSummary_QueryError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRow(nil, 0)
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/course-summary?user_id=uuid-1&course_slug=my-course", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── InternalGetModuleProgress additional coverage ─────────────────────────────

func TestInternalGetModuleProgress_DBError(t *testing.T) {
	pool := &fake.Pool{}
	pool.PushRows(errors.New("db error"))
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/internal/progress/modules?user_id=uid&course_slug=c", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

// ── sanitizeUsername (extra cases) ───────────────────────────────────────────

func TestSanitizeUsername_AllSpecial(t *testing.T) {
	if got := sanitizeUsername("!@#$%^&*()"); got != "user" {
		t.Errorf("want user for all-special input, got %q", got)
	}
}

func TestSanitizeUsername_Long(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz_extra_chars_here"

	got := sanitizeUsername(long)
	if len(got) > 32 {
		t.Errorf("expected max 32 chars, got %d: %q", len(got), got)
	}
}

func TestSanitizeUsername_WithDashUnderscore(t *testing.T) {
	got := sanitizeUsername("user-name_ok")
	if got != "user-name_ok" {
		t.Errorf("want user-name_ok, got %q", got)
	}
}

// ── extractURLBase ───────────────────────────────────────────────────────────

func TestExtractURLBase_Normal(t *testing.T) {
	got := extractURLBase("http://localhost:8080/auth/realms/test")
	if got != "http://localhost:8080" {
		t.Errorf("want http://localhost:8080, got %q", got)
	}
}

func TestExtractURLBase_NoPath(t *testing.T) {
	got := extractURLBase("https://example.com")
	if got != "https://example.com" {
		t.Errorf("want https://example.com, got %q", got)
	}
}

func TestExtractURLBase_InvalidURL(t *testing.T) {
	got := extractURLBase("://bad-url")
	if got == "" {
		t.Error("expected non-empty result for invalid URL")
	}
}

func TestExtractURLBase_EmptyHost(t *testing.T) {
	got := extractURLBase("relative-path/only")
	if got != "relative-path/only" {
		t.Errorf("want raw URL returned, got %q", got)
	}
}

// ── OIDCAuthorize provider unreachable ───────────────────────────────────────

func TestOIDCAuthorize_ProviderUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	pool := &fake.Pool{}
	pool.PushRow(nil, "true")
	pool.PushRow(nil, srv.URL)
	pool.PushRow(nil, "")
	pool.PushRow(nil, "my-client")
	pool.PushRow(nil, "my-secret")
	pool.PushRow(nil, "groups")
	pool.PushRow(nil, "")
	pool.PushRow(nil, "")
	pool.PushRow(nil, "openid email profile")
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/oidc/authorize", "", "")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("want 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── decodeOAuthState (extra cases) ───────────────────────────────────────────

func TestDecodeOAuthState_Invalid(t *testing.T) {
	_, ok := decodeOAuthState("not-a-jwt", htSecret)
	if ok {
		t.Error("expected invalid state to fail")
	}
}

// ── ListProviders (extra cases) ───────────────────────────────────────────────

func TestListProviders_Empty(t *testing.T) {
	pool := &fake.Pool{}
	r := newTestRouter(pool)

	rec := htDo(t, r, "GET", "/api/auth/oauth/providers", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	providers, ok := resp["providers"]
	if !ok {
		t.Error("expected providers key in response")
	}

	_ = providers
}

func TestListProviders_WithProvider(t *testing.T) {
	pool := &fake.Pool{}
	s := &State{
		Pool: pool,
		Config: &config.Config{
			JWTSecret:  htSecret,
			JWTExpiryH: htExpiry,
			Providers: []config.ProviderConfig{
				{ID: "github", Name: "GitHub", ClientID: "gh-id"},
			},
		},
	}
	r := BuildRouter(s, s.Config, pool, false)

	rec := htDo(t, r, "GET", "/api/auth/oauth/providers", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	providers, _ := resp["providers"].([]any)
	if len(providers) != 1 {
		t.Errorf("want 1 provider, got %d", len(providers))
	}
}
