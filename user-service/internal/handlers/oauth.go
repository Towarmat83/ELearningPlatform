package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/elearning/user-service/internal/db"

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/metrics"
	"github.com/elearning/user-service/internal/middleware"
)

// Repeated OAuth/OIDC literals and tunables, pulled out to satisfy goconst
// and mnd. The "providers" JSON key is shared with adminJSONKeyProviders
// in admin.go to avoid a duplicate constant in the package.
const (
	oauthProviderGitHub   = "github"
	oauthFieldEmail       = "email"
	oauthStateKey         = "state"
	oauthProviderGitLab   = "gitlab"
	oauthIssuerGitLab     = "https://gitlab.com"
	oauthProviderGoogle   = "google"
	oauthIssuerGoogle     = "https://accounts.google.com"
	oauthClaimAbout       = "about"
	oauthClaimDescription = "description"
	oauthDefaultUsername  = "user"
	oauthScopeProfile     = "profile"
	oauthFieldBio         = "bio"
	oauthJSONKeyURL       = "url"

	oauthStateTokenTTL  = 10 * time.Minute
	oauthMaxUsernameLen = 32
)

// oauthStateSecret returns the secret used to sign OAuth CSRF state tokens.
// Falls back to JWTSecret when OAuthStateSecret is not explicitly set, so
// existing deployments and tests that only configure JWTSecret keep working.
func (s *State) oauthStateSecret() string {
	if s.Config.OAuthStateSecret != "" {
		return s.Config.OAuthStateSecret
	}

	return s.Config.JWTSecret
}

// ── OAuth CSRF state JWT ──────────────────────────────────────────────────────

// oauthStateClaims is the JWT payload carried in the OAuth "state"
// parameter, used to prevent CSRF and remember which provider a login
// flow was started with.
type oauthStateClaims struct {
	jwt.RegisteredClaims

	Provider     string `json:"provider"`
	PKCEVerifier string `json:"pkceVerifier,omitempty"`
}

// makeOAuthState creates a short-lived, signed JWT to use as the OAuth
// "state" parameter for the given provider. pkceVerifier is embedded so the
// callback can recover it without server-side storage.
func makeOAuthState(provider, pkceVerifier, secret string) (string, error) {
	claims := oauthStateClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(oauthStateTokenTTL)),
		},
		Provider:     provider,
		PKCEVerifier: pkceVerifier,
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign oauth state: %w", err)
	}

	return token, nil
}

// decodeOAuthState verifies and parses an OAuth state JWT, returning the
// provider id, the embedded PKCE verifier, and whether the token is valid.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func decodeOAuthState(stateToken, secret string) (string, string, bool) {
	token, err := jwt.ParseWithClaims(stateToken, &oauthStateClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}

			return []byte(secret), nil
		})
	if err != nil || !token.Valid {
		return "", "", false
	}

	if c, ok := token.Claims.(*oauthStateClaims); ok {
		return c.Provider, c.PKCEVerifier, true
	}

	return "", "", false
}

// ListProviders godoc
// @Summary  List configured OAuth providers
// @Tags     OAuth
// @Produce  json
// @Success  200  {object}  map[string]interface{}
// @Router   /api/auth/oauth/providers [get].
func (s *State) ListProviders(writer http.ResponseWriter, _ *http.Request) {
	var providers []map[string]string

	for _, p := range s.Config.Providers {
		if p.ClientID != "" {
			providers = append(providers, map[string]string{"id": p.ID, "name": p.Name})
		}
	}

	if providers == nil {
		providers = []map[string]string{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{adminJSONKeyProviders: providers})
}

// OAuthAuthorize godoc
// @Summary  Get OAuth authorization URL
// @Tags     OAuth
// @Produce  json
// @Param    provider  path  string  true  "OAuth provider id"
// @Success  200  {object}  map[string]string
// @Failure  400  {object}  map[string]string
// @Router   /api/auth/oauth/{provider}/authorize [get].
func (s *State) OAuthAuthorize(writer http.ResponseWriter, request *http.Request) {
	providerID := param(request, "provider")

	providerCfg := s.Config.FindProvider(providerID)
	if providerCfg == nil || providerCfg.ClientID == "" {
		s.Error(writer, http.StatusBadRequest, "Unknown or unconfigured provider: "+providerID)

		return
	}

	verifier := oauth2.GenerateVerifier()

	stateToken, err := makeOAuthState(providerID, verifier, s.oauthStateSecret())
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "State token error")

		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"

	var authURL string

	if providerID == oauthProviderGitHub {
		parsedURL, _ := url.Parse("https://github.com/login/oauth/authorize")
		params := url.Values{}
		params.Set("client_id", providerCfg.ClientID)
		params.Set("redirect_uri", redirectURI)
		params.Set("scope", "user:email read:user")
		params.Set(oauthStateKey, stateToken)
		params.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
		params.Set("code_challenge_method", "S256")
		parsedURL.RawQuery = params.Encode()
		authURL = parsedURL.String()
	} else {
		issuerURL := resolveIssuerURL(providerCfg)
		if issuerURL == "" {
			s.Error(writer, http.StatusBadRequest, "Provider "+providerID+" requires issuerUrl")

			return
		}

		oidcProvider, err := gooidc.NewProvider(request.Context(), issuerURL)
		if err != nil {
			zap.L().Error("OIDC provider unreachable", zap.Error(err))
			s.Error(writer, http.StatusInternalServerError, "OIDC provider call failed")

			return
		}

		oauth2Cfg := oauth2.Config{
			ClientID:     providerCfg.ClientID,
			ClientSecret: providerCfg.ClientSecret,
			RedirectURL:  redirectURI,
			Endpoint:     oidcProvider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, oauthFieldEmail, oauthScopeProfile},
		}
		authURL = oauth2Cfg.AuthCodeURL(stateToken, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(verifier))
	}

	s.JSON(writer, http.StatusOK, map[string]string{oauthJSONKeyURL: authURL, oauthStateKey: stateToken})
}

// OAuthCallback godoc
// @Summary  Complete OAuth login flow
// @Tags     OAuth
// @Accept   json
// @Produce  json
// @Param    body  object  true  "code and state from provider"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/oauth/callback [post].
func (s *State) OAuthCallback(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	providerID, pkceVerifier, ok := decodeOAuthState(body.State, s.oauthStateSecret())
	if !ok {
		s.Error(writer, http.StatusUnauthorized, "Invalid or expired OAuth state")

		return
	}

	providerCfg := s.Config.FindProvider(providerID)
	if providerCfg == nil || providerCfg.ClientID == "" {
		s.Error(writer, http.StatusBadRequest, "Unknown or unconfigured provider: "+providerID)

		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"

	email, displayName, avatarURL, bioStr, providerUserID, err := fetchOAuthProfile(
		request.Context(), providerID, providerCfg, body.Code, redirectURI, pkceVerifier)
	if err != nil {
		zap.L().Error("OAuth profile fetch failed", zap.String("provider", providerID), zap.Error(err))
		s.Error(writer, http.StatusUnauthorized, "Authentication failed")

		return
	}

	user, err := upsertSSOUser(request.Context(), s.Pool, email, displayName, avatarURL, bioStr, providerID, providerUserID)
	if err != nil {
		zap.L().Error("upsert SSO user failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "failed to update user identity from SSO")

		return
	}

	s.oauthFinishLogin(writer, request, user)
}

// fetchOAuthProfile fetches the authenticated user's profile from the
// given OAuth/OIDC provider, dispatching to the GitHub or generic OIDC
// fetcher. It returns (email, name, avatar, bio, providerUserID, err).
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func fetchOAuthProfile(
	ctx context.Context, providerID string, providerCfg *config.ProviderConfig, code, redirectURI, pkceVerifier string,
) (string, string, *string, *string, string, error) {
	if providerID == oauthProviderGitHub {
		return fetchGitHub(ctx, providerCfg, code, redirectURI, pkceVerifier)
	}

	return fetchOIDCProvider(ctx, providerCfg, code, redirectURI, pkceVerifier)
}

// oauthFinishLogin enrolls the user in their default/group courses, issues
// a session JWT, and writes the login response.
func (s *State) oauthFinishLogin(writer http.ResponseWriter, request *http.Request, user *userPublicRow) {
	addToDefaultGroup(request.Context(), s.Pool, user.ID)
	syncGroupEnrollments(request.Context(), s.Pool, user.ID)

	token, err := middleware.CreateToken(user.ID, user.Email, user.Role, user.Username, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Token error")

		return
	}

	s.JSON(writer, http.StatusOK, authResponse{Token: token, User: *user})
}

// resolveIssuerURL returns the OIDC issuer URL for a provider, falling back to
// well-known defaults when issuerUrl is not explicitly configured.
func resolveIssuerURL(providerCfg *config.ProviderConfig) string {
	if providerCfg.IssuerURL != "" {
		return providerCfg.IssuerURL
	}

	switch providerCfg.ID {
	case oauthProviderGitLab:
		return oauthIssuerGitLab
	case oauthProviderGoogle:
		return oauthIssuerGoogle
	}

	return ""
}

// ── Generic OIDC fetch (GitLab, Google, Authentik, Keycloak, …) ──────────────

// fetchOIDCProvider exchanges an OAuth2 code for the user's profile via
// generic OIDC discovery. It returns (email, name, avatar, bio, sub, err).
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func fetchOIDCProvider(
	ctx context.Context, providerCfg *config.ProviderConfig, code, redirectURI, pkceVerifier string,
) (string, string, *string, *string, string, error) {
	oidcProvider, err := gooidc.NewProvider(ctx, resolveIssuerURL(providerCfg))
	if err != nil {
		return "", "", nil, nil, "", fmt.Errorf("cannot reach OIDC provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     providerCfg.ClientID,
		ClientSecret: providerCfg.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, oauthFieldEmail, oauthScopeProfile},
	}

	token, err := oauth2Cfg.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return "", "", nil, nil, "", fmt.Errorf("token exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", "", nil, nil, "", errors.New("no id_token in response")
	}

	verifier := oidcProvider.Verifier(&gooidc.Config{ClientID: providerCfg.ClientID})

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", nil, nil, "", fmt.Errorf("ID token verification failed: %w", err)
	}

	var claims map[string]any

	err = idToken.Claims(&claims)
	if err != nil {
		return "", "", nil, nil, "", fmt.Errorf("claims extraction failed: %w", err)
	}

	enrichClaimsFromUserInfo(ctx, oidcProvider, token, claims)

	email, _ := claims[oauthFieldEmail].(string)
	if email == "" {
		return "", "", nil, nil, "", errors.New("no email in OIDC token")
	}

	nameStr := oidcDisplayName(claims, email)

	var avatar *string

	if pic, ok := claims["picture"].(string); ok && pic != "" {
		avatar = &pic
	}

	bio := oidcBioFromClaims(claims)

	return email, nameStr, avatar, bio, idToken.Subject, nil
}

// enrichClaimsFromUserInfo merges claims fetched from the OIDC UserInfo
// endpoint into claims; UserInfo values take priority over the ID token.
func enrichClaimsFromUserInfo(ctx context.Context, oidcProvider *gooidc.Provider, token *oauth2.Token, claims map[string]any) {
	userInfo, uiErr := oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if uiErr != nil {
		return
	}

	var uiClaims map[string]any

	if userInfo.Claims(&uiClaims) == nil {
		maps.Copy(claims, uiClaims)
	}
}

// oidcDisplayName resolves a human display name from OIDC claims, falling
// back to preferred_username and finally to email when name is absent.
func oidcDisplayName(claims map[string]any, email string) string {
	name, _ := claims["name"].(string)
	if name == "" {
		name, _ = claims["preferred_username"].(string)
	}

	if name == "" {
		name = email
	}

	return name
}

// oidcBioFromClaims extracts a bio/description from common non-standard
// OIDC claim keys, returning nil when none of them are present.
func oidcBioFromClaims(claims map[string]any) *string {
	for _, key := range []string{oauthFieldBio, oauthClaimDescription, oauthClaimAbout, oauthScopeProfile} {
		if v, ok := claims[key].(string); ok && v != "" {
			return &v
		}
	}

	return nil
}

// ── GitHub fetch (OAuth2 only, no OIDC discovery) ─────────────────────────────

// fetchGitHub exchanges an OAuth2 code for a GitHub access token and
// returns the authenticated user's profile fields as
// (email, name, avatar, bio, id, err).
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func fetchGitHub(
	ctx context.Context, providerCfg *config.ProviderConfig, code, redirectURI, pkceVerifier string,
) (string, string, *string, *string, string, error) {
	client := &http.Client{}

	accessToken, err := githubAccessToken(ctx, client, providerCfg, code, redirectURI, pkceVerifier)
	if err != nil {
		return "", "", nil, nil, "", err
	}

	var profile map[string]any

	err = doGet(client, "https://api.github.com/user", accessToken, &profile) //nolint:contextcheck // doGet's signature is pinned by pre-existing whitebox tests; see doGet's doc comment
	if err != nil {
		return "", "", nil, nil, "", fmt.Errorf("GitHub user request failed: %w", err)
	}

	ghID := fmt.Sprintf("%v", profile["id"])
	login, _ := profile["login"].(string)

	nameStr, _ := profile["name"].(string)
	if nameStr == "" {
		nameStr = login
	}

	avatarPtr, bioPtr := githubProfileMedia(profile)

	emailStr, _ := profile[oauthFieldEmail].(string)
	if emailStr == "" {
		emailStr = githubPrimaryEmail(client, accessToken) //nolint:contextcheck // githubPrimaryEmail calls doGet, whose signature is pinned by pre-existing tests
	}

	if emailStr == "" {
		return "", "", nil, nil, "", errors.New("could not retrieve a verified GitHub email address")
	}

	return emailStr, nameStr, avatarPtr, bioPtr, ghID, nil
}

// githubAccessToken exchanges an OAuth2 authorization code for a GitHub
// access token. pkceVerifier is sent as code_verifier when non-empty.
func githubAccessToken(
	ctx context.Context, client *http.Client, providerCfg *config.ProviderConfig, code, redirectURI, pkceVerifier string,
) (string, error) {
	formValues := url.Values{
		"client_id":     {providerCfg.ClientID},
		"client_secret": {providerCfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	if pkceVerifier != "" {
		formValues.Set("code_verifier", pkceVerifier)
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token",
		strings.NewReader(formValues.Encode()))
	if err != nil {
		return "", fmt.Errorf("build github token request: %w", err)
	}

	tokenReq.Header.Set("Accept", "application/json")
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("User-Agent", "LearnLab-SSO/1.0")

	resp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("GitHub token request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, _ := io.ReadAll(resp.Body)

	var tokenRes map[string]any

	_ = json.Unmarshal(body, &tokenRes)

	accessToken, _ := tokenRes["access_token"].(string)
	if accessToken == "" {
		return "", errors.New("GitHub did not return an access token")
	}

	return accessToken, nil
}

// githubPrimaryEmail finds the user's primary, verified email address via
// the GitHub emails API, falling back to the first listed address.
func githubPrimaryEmail(client *http.Client, accessToken string) string {
	var emails []map[string]any

	_ = doGet(client, "https://api.github.com/user/emails", accessToken, &emails)

	for _, emailEntry := range emails {
		primary, _ := emailEntry["primary"].(bool)
		if !primary {
			continue
		}

		verified, _ := emailEntry["verified"].(bool)
		if verified {
			email, _ := emailEntry[oauthFieldEmail].(string)

			return email
		}
	}

	if len(emails) > 0 {
		email, _ := emails[0][oauthFieldEmail].(string)

		return email
	}

	return ""
}

// githubProfileMedia extracts the avatar URL and bio from a GitHub user
// profile payload, when present. It returns (avatar, bio).
func githubProfileMedia(profile map[string]any) (*string, *string) { //nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment for the meaning of each value
	var avatar, bio *string

	// GitHub exposes bio directly in the user profile.
	if av, ok := profile["avatarUrl"].(string); ok && av != "" {
		avatar = &av
	}

	if b, ok := profile[oauthFieldBio].(string); ok && b != "" {
		bio = &b
	}

	return avatar, bio
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

// doGet issues an authenticated GET request against rawURL and decodes the
// JSON response body into out. It has no caller-supplied context because
// call sites (and existing tests) invoke it without one; it still avoids
// noctx by building the request through NewRequestWithContext.
func doGet(client *http.Client, rawURL, bearer string, out any) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", rawURL, err)
	}

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LearnLab-SSO/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", rawURL, err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, _ := io.ReadAll(resp.Body)

	unmarshalErr := json.Unmarshal(body, out)
	if unmarshalErr != nil {
		return fmt.Errorf("decode response from %s: %w", rawURL, unmarshalErr)
	}

	return nil
}

// ── User upsert ───────────────────────────────────────────────────────────────

// upsertSSOUser finds or creates the local user for an SSO login, trying a
// provider-identity match first, then an email match, and finally creating
// a brand-new account.
func upsertSSOUser(
	ctx context.Context, pool db.Pool, email, displayName string, avatarURL, bio *string, provider, providerUserID string,
) (*userPublicRow, error) {
	const sel = `SELECT id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text FROM users`

	user := upsertSSOUserByProvider(ctx, pool, sel, provider, providerUserID, avatarURL, bio)
	if user != nil {
		return user, nil
	}

	user, err := upsertSSOUserByEmail(ctx, pool, sel, email, provider, providerUserID, avatarURL, bio)
	if user != nil || err != nil {
		return user, err //nolint:nilnil // cascading lookup: (nil,nil) means "no match, fall through to create a new user"
	}

	username := sanitizeUsername(displayName)

	var taken int64

	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE username = $1", username).Scan(&taken)
	if taken > 0 {
		username = username + "_" + uuid.New().String()[:6]
	}

	created, err := scanUserPublic(pool.QueryRow(ctx,
		`INSERT INTO users (username, email, authProvider, provider_user_id, role, avatarUrl, bio)
		 VALUES ($1, $2, $3, $4, 'student', $5, $6)
		 RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		username, email, provider, providerUserID, avatarURL, bio))
	if err != nil {
		return nil, err
	}

	metrics.ActiveUsers.Inc()

	return &created, nil
}

// completeSSOLogin provisions/refreshes the local user for an SSO identity,
// syncs their group membership and role, issues a JWT, and writes the auth
// response. Shared by every SSO login flow (OIDC, LDAP, ...).
func (s *State) completeSSOLogin(
	ctx context.Context, writer http.ResponseWriter,
	email, displayName string, avatarURL, bio *string, provider, providerUserID string, groups []string,
) {
	user, err := upsertSSOUser(ctx, s.Pool, email, displayName, avatarURL, bio, provider, providerUserID)
	if err != nil {
		zap.L().Error("upsert SSO user failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "failed to update user identity from SSO")

		return
	}

	role, err := syncGroupsAndDeriveRole(ctx, s.Pool, user.ID, groups, provider)
	if err != nil {
		role = user.Role
	}

	addToDefaultGroup(ctx, s.Pool, user.ID)
	syncGroupEnrollments(ctx, s.Pool, user.ID)

	token, err := middleware.CreateToken(user.ID, user.Email, role, user.Username, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Token error")

		return
	}

	user.Role = role
	s.JSON(writer, http.StatusOK, authResponse{Token: token, User: *user})
}

// upsertSSOUserByProvider looks up a user already linked to this
// provider+providerUserID and refreshes their avatar/bio. It returns nil
// when no such user exists yet; lookup/update failures are not fatal to
// the overall SSO login, so no error is returned here.
func upsertSSOUserByProvider(
	ctx context.Context, pool db.Pool, sel, provider, providerUserID string, avatarURL, bio *string,
) *userPublicRow {
	existing, err := scanUserPublic(pool.QueryRow(ctx,
		sel+` WHERE authProvider = $1 AND provider_user_id = $2`, provider, providerUserID))
	if err != nil {
		return nil
	}

	updated, err := scanUserPublic(pool.QueryRow(ctx,
		`UPDATE users SET
		   avatarUrl = COALESCE($1, avatarUrl),
		   bio        = CASE WHEN (bio IS NULL OR bio = '') THEN COALESCE($2, bio) ELSE bio END,
		   updatedAt = NOW()
		 WHERE id = $3::uuid
		 RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		avatarURL, bio, existing.ID))
	if err != nil {
		zap.L().Warn("upsertSSOUser: UPDATE by provider failed, using SELECT result", zap.Error(err), zap.String("userId", existing.ID))

		return &existing
	}

	return &updated
}

// upsertSSOUserByEmail looks up a user by email and, unless it is already
// linked to a different provider, links it to this provider and refreshes
// their avatar/bio. It returns nil, nil when no such user exists yet.
func upsertSSOUserByEmail(
	ctx context.Context, pool db.Pool, sel, email, provider, providerUserID string, avatarURL, bio *string,
) (*userPublicRow, error) {
	existing, err := scanUserPublic(pool.QueryRow(ctx, sel+` WHERE email = $1`, email))
	if err != nil {
		return nil, nil //nolint:nilerr,nilnil // sentinel: no user matched this email yet, try creating one
	}

	// Never silently take over a local account — that would lock out password login.
	// Only link when the account has no provider yet or already belongs to this provider.
	if existing.AuthProvider != "" && existing.AuthProvider != provider {
		return nil, fmt.Errorf("an account with this email already exists with a different login method (%s)", existing.AuthProvider)
	}

	updated, err := scanUserPublic(pool.QueryRow(ctx,
		`UPDATE users SET
		   authProvider    = $1,
		   provider_user_id = $2,
		   avatarUrl       = COALESCE($3, avatarUrl),
		   bio              = CASE WHEN (bio IS NULL OR bio = '') THEN COALESCE($4, bio) ELSE bio END,
		   updatedAt       = NOW()
		 WHERE id = $5::uuid
		 RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		provider, providerUserID, avatarURL, bio, existing.ID))
	if err != nil {
		zap.L().Warn("upsertSSOUser: UPDATE by email failed, using SELECT result", zap.Error(err), zap.String("userId", existing.ID))

		return &existing, nil
	}

	return &updated, nil
}

// sanitizeUsername derives a URL/DB-safe username from an OAuth display
// name, keeping only letters, digits, underscores, and hyphens.
func sanitizeUsername(name string) string {
	var builder strings.Builder

	for _, c := range name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '-' {
			builder.WriteRune(c)
		}

		if builder.Len() >= oauthMaxUsernameLen {
			break
		}
	}

	if builder.Len() == 0 {
		return oauthDefaultUsername
	}

	return builder.String()
}
