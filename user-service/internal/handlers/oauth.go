package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/metrics"
	"github.com/elearning/user-service/internal/middleware"
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

// ListProviders godoc
// @Summary  List configured OAuth providers
// @Tags     OAuth
// @Produce  json
// @Success  200  {object}  map[string]interface{}
// @Router   /api/auth/oauth/providers [get]
func (s *State) ListProviders(w http.ResponseWriter, r *http.Request) {
	var providers []map[string]string
	for _, p := range s.Config.Providers {
		if p.ClientID != "" {
			providers = append(providers, map[string]string{"id": p.ID, "name": p.Name})
		}
	}
	if providers == nil {
		providers = []map[string]string{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"providers": providers})
}

// OAuthAuthorize godoc
// @Summary  Get OAuth authorization URL
// @Tags     OAuth
// @Produce  json
// @Param    provider  path  string  true  "OAuth provider id"
// @Success  200  {object}  map[string]string
// @Failure  400  {object}  map[string]string
// @Router   /api/auth/oauth/{provider}/authorize [get]
func (s *State) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	providerID := param(r, "provider")
	p := s.Config.FindProvider(providerID)
	if p == nil || p.ClientID == "" {
		s.Error(w, http.StatusBadRequest, "Unknown or unconfigured provider: "+providerID)
		return
	}

	stateToken, err := makeOAuthState(providerID, s.Config.JWTSecret)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "State token error")
		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"
	var authURL string

	if providerID == "github" {
		u, _ := url.Parse("https://github.com/login/oauth/authorize")
		q := url.Values{}
		q.Set("client_id", p.ClientID)
		q.Set("redirect_uri", redirectURI)
		q.Set("scope", "user:email read:user")
		q.Set("state", stateToken)
		u.RawQuery = q.Encode()
		authURL = u.String()
	} else {
		issuerURL := resolveIssuerURL(p)
		if issuerURL == "" {
			s.Error(w, http.StatusBadRequest, "Provider "+providerID+" requires issuer_url")
			return
		}
		oidcProvider, err := gooidc.NewProvider(r.Context(), issuerURL)
		if err != nil {
			s.Error(w, http.StatusBadGateway, "Cannot reach OIDC provider: "+err.Error())
			return
		}
		oauth2Cfg := oauth2.Config{
			ClientID:     p.ClientID,
			ClientSecret: p.ClientSecret,
			RedirectURL:  redirectURI,
			Endpoint:     oidcProvider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
		}
		authURL = oauth2Cfg.AuthCodeURL(stateToken, oauth2.AccessTypeOnline)
	}

	s.JSON(w, http.StatusOK, map[string]string{"url": authURL, "state": stateToken})
}

// OAuthCallback godoc
// @Summary  Complete OAuth login flow
// @Tags     OAuth
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "code and state from provider"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/oauth/callback [post]
func (s *State) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	providerID, ok := decodeOAuthState(req.State, s.Config.JWTSecret)
	if !ok {
		s.Error(w, http.StatusUnauthorized, "Invalid or expired OAuth state")
		return
	}

	p := s.Config.FindProvider(providerID)
	if p == nil || p.ClientID == "" {
		s.Error(w, http.StatusBadRequest, "Unknown or unconfigured provider: "+providerID)
		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"
	var email, displayName, providerUserID string
	var avatarURL *string
	var err error

	if providerID == "github" {
		email, displayName, avatarURL, providerUserID, err = fetchGitHub(p, req.Code, redirectURI)
	} else {
		email, displayName, avatarURL, providerUserID, err = fetchOIDCProvider(r.Context(), p, req.Code, redirectURI)
	}
	if err != nil {
		s.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	user, err := upsertSSOUser(r.Context(), s.Pool, email, displayName, avatarURL, providerID, providerUserID)
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

// resolveIssuerURL returns the OIDC issuer URL for a provider, falling back to
// well-known defaults when issuer_url is not explicitly configured.
func resolveIssuerURL(p *config.ProviderConfig) string {
	if p.IssuerURL != "" {
		return p.IssuerURL
	}
	switch p.ID {
	case "gitlab":
		return "https://gitlab.com"
	case "google":
		return "https://accounts.google.com"
	}
	return ""
}

// ── Generic OIDC fetch (GitLab, Google, Authentik, Keycloak, …) ──────────────

func fetchOIDCProvider(ctx context.Context, p *config.ProviderConfig, code, redirectURI string) (email, name string, avatar *string, sub string, err error) {
	oidcProvider, err := gooidc.NewProvider(ctx, resolveIssuerURL(p))
	if err != nil {
		return "", "", nil, "", fmt.Errorf("cannot reach OIDC provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
	}

	token, err := oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		return "", "", nil, "", fmt.Errorf("token exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", "", nil, "", fmt.Errorf("no id_token in response")
	}

	verifier := oidcProvider.Verifier(&gooidc.Config{ClientID: p.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", nil, "", fmt.Errorf("ID token verification failed: %w", err)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return "", "", nil, "", fmt.Errorf("claims extraction failed: %w", err)
	}

	email, _ = claims["email"].(string)
	if email == "" {
		return "", "", nil, "", fmt.Errorf("no email in OIDC token")
	}
	name, _ = claims["name"].(string)
	if name == "" {
		name, _ = claims["preferred_username"].(string)
	}
	if name == "" {
		name = email
	}
	if pic, ok := claims["picture"].(string); ok && pic != "" {
		avatar = &pic
	}
	return email, name, avatar, idToken.Subject, nil
}

// ── GitHub fetch (OAuth2 only, no OIDC discovery) ─────────────────────────────

func fetchGitHub(p *config.ProviderConfig, code, redirectURI string) (email, name string, avatar *string, id string, err error) {
	client := &http.Client{}

	tokenReq, _ := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token",
		strings.NewReader(url.Values{
			"client_id":     {p.ClientID},
			"client_secret": {p.ClientSecret},
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
	json.Unmarshal(body, &tokenRes)

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
		doGet(client, "https://api.github.com/user/emails", accessToken, &emails)
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

// ── HTTP helper ───────────────────────────────────────────────────────────────

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

// ── User upsert ───────────────────────────────────────────────────────────────

func upsertSSOUser(ctx context.Context, pool *pgxpool.Pool, email, displayName string, avatarURL *string, provider, providerUserID string) (*userPublicRow, error) {
	const sel = `SELECT id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text FROM users`

	u, err := scanUserPublic(pool.QueryRow(ctx,
		sel+` WHERE auth_provider = $1 AND provider_user_id = $2`, provider, providerUserID))
	if err == nil {
		u2, err2 := scanUserPublic(pool.QueryRow(ctx,
			`UPDATE users SET avatar_url = COALESCE($1, avatar_url), updated_at = NOW()
			 WHERE id = $2::uuid
			 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
			avatarURL, u.ID))
		if err2 != nil {
			slog.Warn("upsertSSOUser: UPDATE by provider failed, using SELECT result", "err", err2, "user_id", u.ID)
			return &u, nil
		}
		return &u2, nil
	}

	u, err = scanUserPublic(pool.QueryRow(ctx, sel+` WHERE email = $1`, email))
	if err == nil {
		u2, err2 := scanUserPublic(pool.QueryRow(ctx,
			`UPDATE users SET auth_provider = $1, provider_user_id = $2,
			  avatar_url = COALESCE($3, avatar_url), updated_at = NOW()
			 WHERE id = $4::uuid
			 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
			provider, providerUserID, avatarURL, u.ID))
		if err2 != nil {
			slog.Warn("upsertSSOUser: UPDATE by email failed, using SELECT result", "err", err2, "user_id", u.ID)
			return &u, nil
		}
		return &u2, nil
	}

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
