package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elearning/api-go/internal/metrics"
	"github.com/elearning/api-go/internal/middleware"
)

// ── OAuth CSRF state JWT ──────────────────────────────────────────────────────

type oauthStateClaims struct {
	Provider string `json:"provider"`
	jwt.RegisteredClaims
}

func makeOAuthState(provider, secret string) (string, error) {
	claims := oauthStateClaims{
		Provider: provider,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func decodeOAuthState(stateToken, secret string) (string, bool) {
	token, err := jwt.ParseWithClaims(stateToken, &oauthStateClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
	if err != nil || !token.Valid {
		return "", false
	}
	if c, ok := token.Claims.(*oauthStateClaims); ok {
		return c.Provider, true
	}
	return "", false
}

// GET /api/auth/oauth/providers
func (s *State) ListProviders(w http.ResponseWriter, r *http.Request) {
	var providers []map[string]string
	if s.Config.GitLabClientID != "" {
		providers = append(providers, map[string]string{"id": "gitlab", "name": "GitLab"})
	}
	if s.Config.GitHubClientID != "" {
		providers = append(providers, map[string]string{"id": "github", "name": "GitHub"})
	}
	if providers == nil {
		providers = []map[string]string{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"providers": providers})
}

// GET /api/auth/oauth/{provider}/authorize
func (s *State) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := param(r, "provider")
	stateToken, err := makeOAuthState(provider, s.Config.JWTSecret)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "State token error")
		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"
	var authURL string

	switch provider {
	case "gitlab":
		if s.Config.GitLabClientID == "" {
			s.Error(w, http.StatusBadRequest, "GitLab OAuth not configured")
			return
		}
		gitlabBase := strings.TrimRight(ReadSetting(r.Context(), s.Pool, "gitlab_url", s.Config.GitLabURL), "/")
		u, _ := url.Parse(gitlabBase + "/oauth/authorize")
		q := url.Values{}
		q.Set("client_id", s.Config.GitLabClientID)
		q.Set("redirect_uri", redirectURI)
		q.Set("response_type", "code")
		q.Set("scope", "read_user")
		q.Set("state", stateToken)
		u.RawQuery = q.Encode()
		authURL = u.String()

	case "github":
		if s.Config.GitHubClientID == "" {
			s.Error(w, http.StatusBadRequest, "GitHub OAuth not configured")
			return
		}
		u, _ := url.Parse("https://github.com/login/oauth/authorize")
		q := url.Values{}
		q.Set("client_id", s.Config.GitHubClientID)
		q.Set("redirect_uri", redirectURI)
		q.Set("scope", "user:email read:user")
		q.Set("state", stateToken)
		u.RawQuery = q.Encode()
		authURL = u.String()

	default:
		s.Error(w, http.StatusBadRequest, "Unknown provider: "+provider)
		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"url": authURL, "state": stateToken})
}

// POST /api/auth/oauth/callback
func (s *State) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	provider, ok := decodeOAuthState(req.State, s.Config.JWTSecret)
	if !ok {
		s.Error(w, http.StatusUnauthorized, "Invalid or expired OAuth state")
		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"
	var email, displayName, providerUserID string
	var avatarURL *string
	var err error

	switch provider {
	case "gitlab":
		email, displayName, avatarURL, providerUserID, err = s.fetchGitLab(r.Context(), req.Code, redirectURI)
	case "github":
		email, displayName, avatarURL, providerUserID, err = s.fetchGitHub(req.Code, redirectURI)
	default:
		s.Error(w, http.StatusBadRequest, "Unknown provider: "+provider)
		return
	}
	if err != nil {
		s.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	user, err := upsertSSOUser(r.Context(), s.Pool, email, displayName, avatarURL, provider, providerUserID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())
		return
	}

	token, err := middleware.CreateToken(user.ID, user.Email, user.Role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}
	s.JSON(w, http.StatusOK, authResponse{Token: token, User: *user})
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func doGet(client *http.Client, rawURL, bearer string, out any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LearnLab-SSO/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, out)
}

// ── Provider fetch helpers ────────────────────────────────────────────────────

func (s *State) fetchGitLab(ctx context.Context, code, redirectURI string) (email, name string, avatar *string, id string, err error) {
	if s.Config.GitLabClientID == "" || s.Config.GitLabClientSecret == "" {
		return "", "", nil, "", fmt.Errorf("GitLab OAuth not configured")
	}
	gitlabBase := strings.TrimRight(ReadSetting(ctx, s.Pool, "gitlab_url", s.Config.GitLabURL), "/")
	client := &http.Client{}

	resp, err := client.PostForm(gitlabBase+"/oauth/token", url.Values{
		"client_id":     {s.Config.GitLabClientID},
		"client_secret": {s.Config.GitLabClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		return "", "", nil, "", fmt.Errorf("GitLab token request failed: %w", err)
	}
	defer resp.Body.Close()
	var tokenRes map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &tokenRes) //nolint:errcheck

	accessToken, _ := tokenRes["access_token"].(string)
	if accessToken == "" {
		return "", "", nil, "", fmt.Errorf("GitLab did not return an access token")
	}

	var info map[string]any
	if err = doGet(client, gitlabBase+"/api/v4/user", accessToken, &info); err != nil {
		return "", "", nil, "", fmt.Errorf("GitLab user request failed: %w", err)
	}

	idStr := fmt.Sprintf("%v", info["id"])
	emailStr, _ := info["email"].(string)
	nameStr, _ := info["name"].(string)
	if nameStr == "" {
		nameStr, _ = info["username"].(string)
	}
	if nameStr == "" {
		nameStr = emailStr
	}
	var avPtr *string
	if av, ok := info["avatar_url"].(string); ok && av != "" {
		avPtr = &av
	}
	return emailStr, nameStr, avPtr, idStr, nil
}

func (s *State) fetchGitHub(code, redirectURI string) (email, name string, avatar *string, id string, err error) {
	if s.Config.GitHubClientID == "" || s.Config.GitHubClientSecret == "" {
		return "", "", nil, "", fmt.Errorf("GitHub OAuth not configured")
	}
	client := &http.Client{}

	tokenReq, _ := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token",
		strings.NewReader(url.Values{
			"client_id":     {s.Config.GitHubClientID},
			"client_secret": {s.Config.GitHubClientSecret},
			"code":          {code},
			"redirect_uri":  {redirectURI},
		}.Encode()))
	tokenReq.Header.Set("Accept", "application/json")
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("User-Agent", "LearnLab-SSO/1.0")

	resp, err := client.Do(tokenReq)
	if err != nil {
		return "", "", nil, "", fmt.Errorf("GitHub token request failed: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var tokenRes map[string]any
	json.Unmarshal(body, &tokenRes) //nolint:errcheck

	accessToken, _ := tokenRes["access_token"].(string)
	if accessToken == "" {
		return "", "", nil, "", fmt.Errorf("GitHub did not return an access token")
	}

	var profile map[string]any
	if err = doGet(client, "https://api.github.com/user", accessToken, &profile); err != nil {
		return "", "", nil, "", fmt.Errorf("GitHub user request failed: %w", err)
	}

	ghID := fmt.Sprintf("%v", profile["id"])
	login, _ := profile["login"].(string)
	nameStr, _ := profile["name"].(string)
	if nameStr == "" {
		nameStr = login
	}
	var avPtr *string
	if av, ok := profile["avatar_url"].(string); ok && av != "" {
		avPtr = &av
	}

	emailStr, _ := profile["email"].(string)
	if emailStr == "" {
		var emails []map[string]any
		doGet(client, "https://api.github.com/user/emails", accessToken, &emails) //nolint:errcheck
		for _, e := range emails {
			if prim, _ := e["primary"].(bool); !prim {
				continue
			}
			if ver, _ := e["verified"].(bool); ver {
				emailStr, _ = e["email"].(string)
				break
			}
		}
		if emailStr == "" && len(emails) > 0 {
			emailStr, _ = emails[0]["email"].(string)
		}
	}
	if emailStr == "" {
		return "", "", nil, "", fmt.Errorf("could not retrieve a verified GitHub email address")
	}
	return emailStr, nameStr, avPtr, ghID, nil
}

// ── User upsert ───────────────────────────────────────────────────────────────

func upsertSSOUser(ctx context.Context, pool *pgxpool.Pool, email, displayName string, avatarURL *string, provider, providerUserID string) (*userPublicRow, error) {
	const sel = `SELECT id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text FROM users`

	// 1. Match by (provider, provider_user_id)
	u, err := scanUserPublic(pool.QueryRow(ctx,
		sel+` WHERE auth_provider = $1 AND provider_user_id = $2`, provider, providerUserID))
	if err == nil {
		// Update avatar in case it changed
		u2, _ := scanUserPublic(pool.QueryRow(ctx,
			`UPDATE users SET avatar_url = COALESCE($1, avatar_url), updated_at = NOW()
			 WHERE id = $2::uuid
			 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
			avatarURL, u.ID))
		return &u2, nil
	}

	// 2. Match by email (link existing local account)
	u, err = scanUserPublic(pool.QueryRow(ctx, sel+` WHERE email = $1`, email))
	if err == nil {
		u2, _ := scanUserPublic(pool.QueryRow(ctx,
			`UPDATE users SET auth_provider = $1, provider_user_id = $2,
			  avatar_url = COALESCE($3, avatar_url), updated_at = NOW()
			 WHERE id = $4::uuid
			 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
			provider, providerUserID, avatarURL, u.ID))
		return &u2, nil
	}

	// 3. Create new user
	username := sanitizeUsername(displayName)
	var taken int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE username = $1", username).Scan(&taken)
	if taken > 0 {
		username = username + "_" + uuid.New().String()[:6]
	}

	u, err = scanUserPublic(pool.QueryRow(ctx,
		`INSERT INTO users (username, email, auth_provider, provider_user_id, role, avatar_url)
		 VALUES ($1, $2, $3, $4, 'student', $5)
		 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
		username, email, provider, providerUserID, avatarURL))
	if err != nil {
		return nil, err
	}
	metrics.ActiveUsers.Inc()
	return &u, nil
}

func sanitizeUsername(name string) string {
	var b strings.Builder
	for _, c := range name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '-' {
			b.WriteRune(c)
		}
		if b.Len() >= 32 {
			break
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}
