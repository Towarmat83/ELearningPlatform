package handlers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/elearning/user-service/internal/middleware"
)

type oidcSettings struct {
	Enabled            bool
	ProviderURL        string
	IssuerURL          string // when set, bypasses issuer check (split-horizon: internal discovery URL ≠ public issuer)
	ClientID           string
	ClientSecret       string
	Scopes             []string
	GroupClaim         string
	RedirectBase       string // overrides config.OAuthRedirectBase for redirect_uri sent to the provider
	BrowserBaseURL     string // optional: rewrite internal base URL to this for browser redirects
	InsecureSkipVerify bool   // skip TLS verification for custom CA / self-signed OIDC provider
}

func (s *State) loadOIDCSettings(ctx context.Context) (oidcSettings, error) {
	cfg := oidcSettings{
		Enabled:            ReadSetting(ctx, s.Pool, "oidc_enabled", "false") == "true",
		ProviderURL:        ReadSetting(ctx, s.Pool, "oidc_provider_url", ""),
		IssuerURL:          ReadSetting(ctx, s.Pool, "oidc_issuer_url", ""),
		ClientID:           ReadSetting(ctx, s.Pool, "oidc_client_id", ""),
		ClientSecret:       ReadSetting(ctx, s.Pool, "oidc_client_secret", ""),
		GroupClaim:         ReadSetting(ctx, s.Pool, "oidc_group_claim", "groups"),
		RedirectBase:       ReadSetting(ctx, s.Pool, "oidc_redirect_base", s.Config.OAuthRedirectBase),
		BrowserBaseURL:     ReadSetting(ctx, s.Pool, "oidc_browser_base_url", ""),
		InsecureSkipVerify: ReadSetting(ctx, s.Pool, "oidc_insecure_skip_verify", "false") == "true",
	}
	scopes := ReadSetting(ctx, s.Pool, "oidc_scopes", "openid email profile groups")
	for _, sc := range strings.Split(scopes, " ") {
		if sc = strings.TrimSpace(sc); sc != "" {
			cfg.Scopes = append(cfg.Scopes, sc)
		}
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "email", "profile"}
	}
	if !cfg.Enabled {
		return cfg, fmt.Errorf("OIDC is not enabled")
	}
	if cfg.ProviderURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return cfg, fmt.Errorf("OIDC not fully configured (provider_url, client_id, client_secret required)")
	}
	return cfg, nil
}

// oidcContext returns a context with InsecureIssuerURLContext set when cfg.IssuerURL is provided.
// This allows the discovery endpoint (ProviderURL) to differ from the issuer claim in tokens,
// which is necessary in split-horizon setups (e.g. KinD where the internal DNS ≠ browser URL).
func oidcContext(ctx context.Context, cfg oidcSettings) context.Context {
	if cfg.InsecureSkipVerify {
		insecureClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		}
		ctx = gooidc.ClientContext(ctx, insecureClient)
	}
	if cfg.IssuerURL != "" {
		return gooidc.InsecureIssuerURLContext(ctx, cfg.IssuerURL)
	}
	return ctx
}

// extractURLBase returns scheme+host from a URL string (e.g. "http://host:80/path" → "http://host:80").
func extractURLBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
}

// OIDCAuthorize godoc
// @Summary  Get OIDC authorization URL
// @Tags     OIDC
// @Produce  json
// @Success  200  {object}  map[string]string
// @Failure  400  {object}  map[string]string
// @Router   /api/auth/oidc/authorize [get]
func (s *State) OIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadOIDCSettings(r.Context())
	if err != nil {
		s.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	stateToken, err := makeOAuthState("oidc", s.Config.JWTSecret)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "State token error")
		return
	}

	redirectURI := strings.TrimRight(cfg.RedirectBase, "/") + "/auth/callback"

	providerCtx := oidcContext(r.Context(), cfg)
	provider, err := gooidc.NewProvider(providerCtx, cfg.ProviderURL)
	if err != nil {
		s.Error(w, http.StatusBadGateway, "Cannot reach OIDC provider: "+err.Error())
		return
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	authURL := oauth2Cfg.AuthCodeURL(stateToken, oauth2.AccessTypeOnline)

	// If oidc_browser_base_url is set, rewrite the internal base URL in authURL
	// to the browser-accessible one (split-horizon: pod uses internal DNS, browser uses external).
	if cfg.BrowserBaseURL != "" {
		internalBase := extractURLBase(cfg.ProviderURL)
		authURL = strings.Replace(authURL, internalBase, strings.TrimRight(cfg.BrowserBaseURL, "/"), 1)
	}

	s.JSON(w, http.StatusOK, map[string]string{"url": authURL, "state": stateToken})
}

// OIDCCallback godoc
// @Summary  Complete OIDC login flow
// @Tags     OIDC
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "code and state"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/oidc/callback [post]
func (s *State) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	provider, ok := decodeOAuthState(req.State, s.Config.JWTSecret)
	if !ok || provider != "oidc" {
		s.Error(w, http.StatusUnauthorized, "Invalid or expired OIDC state")
		return
	}

	ctx := r.Context()
	cfg, err := s.loadOIDCSettings(ctx)
	if err != nil {
		s.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	providerCtx := oidcContext(ctx, cfg)
	oidcProvider, err := gooidc.NewProvider(providerCtx, cfg.ProviderURL)
	if err != nil {
		s.Error(w, http.StatusBadGateway, "Cannot reach OIDC provider: "+err.Error())
		return
	}

	redirectURI := strings.TrimRight(cfg.RedirectBase, "/") + "/auth/callback"
	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	token, err := oauth2Cfg.Exchange(ctx, req.Code)
	if err != nil {
		s.Error(w, http.StatusUnauthorized, "Token exchange failed: "+err.Error())
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.Error(w, http.StatusUnauthorized, "No id_token in response")
		return
	}

	verifier := oidcProvider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})
	idToken, err := verifier.Verify(providerCtx, rawIDToken)
	if err != nil {
		s.Error(w, http.StatusUnauthorized, "ID token verification failed: "+err.Error())
		return
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		s.Error(w, http.StatusInternalServerError, "Claims extraction failed")
		return
	}

	// Enrich claims from the UserInfo endpoint (takes priority over ID token).
	userInfo, uiErr := oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if uiErr == nil {
		var uiClaims map[string]any
		if uiErr2 := userInfo.Claims(&uiClaims); uiErr2 == nil {
			for k, v := range uiClaims {
				claims[k] = v
			}
		}
	}

	email, _ := claims["email"].(string)
	if email == "" {
		s.Error(w, http.StatusUnauthorized, "No email in OIDC token")
		return
	}
	sub := idToken.Subject
	name, _ := claims["name"].(string)
	if name == "" {
		name, _ = claims["preferred_username"].(string)
	}
	if name == "" {
		name = email
	}
	var avatarURL *string
	if pic, ok := claims["picture"].(string); ok && pic != "" {
		avatarURL = &pic
	}
	// Extract bio from common non-standard OIDC attributes.
	var bio *string
	for _, key := range []string{"bio", "description", "about", "profile"} {
		if v, ok := claims[key].(string); ok && v != "" {
			bio = &v
			break
		}
	}

	// Extract group names from configurable claim
	var groups []string
	if raw, ok := claims[cfg.GroupClaim]; ok {
		switch v := raw.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					groups = append(groups, s)
				}
			}
		case string:
			for _, g := range strings.Split(v, ",") {
				if g = strings.TrimSpace(g); g != "" {
					groups = append(groups, g)
				}
			}
		}
	}

	providerKey := "oidc"
	user, err := upsertSSOUser(ctx, s.Pool, email, name, avatarURL, bio, providerKey, sub)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())
		return
	}

	role, err := syncGroupsAndDeriveRole(ctx, s.Pool, user.ID, groups, "oidc")
	if err != nil {
		_ = err
		role = user.Role
	}
	addToDefaultGroup(ctx, s.Pool, user.ID)
	syncGroupEnrollments(ctx, s.Pool, user.ID)

	jwtToken, err := middleware.CreateToken(user.ID, user.Email, role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}
	user.Role = role
	s.JSON(w, http.StatusOK, authResponse{Token: jwtToken, User: *user})
}
