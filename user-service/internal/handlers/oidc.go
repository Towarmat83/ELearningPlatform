package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/elearning/user-service/internal/middleware"
)

type oidcSettings struct {
	Enabled        bool
	ProviderURL    string
	ClientID       string
	ClientSecret   string
	Scopes         []string
	GroupClaim     string
	BrowserBaseURL string // optional: rewrite internal base URL to this for browser redirects
}

func (s *State) loadOIDCSettings(ctx context.Context) (oidcSettings, error) {
	cfg := oidcSettings{
		Enabled:        ReadSetting(ctx, s.Pool, "oidc_enabled", "false") == "true",
		ProviderURL:    ReadSetting(ctx, s.Pool, "oidc_provider_url", ""),
		ClientID:       ReadSetting(ctx, s.Pool, "oidc_client_id", ""),
		ClientSecret:   ReadSetting(ctx, s.Pool, "oidc_client_secret", ""),
		GroupClaim:     ReadSetting(ctx, s.Pool, "oidc_group_claim", "groups"),
		BrowserBaseURL: ReadSetting(ctx, s.Pool, "oidc_browser_base_url", ""),
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

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"

	provider, err := gooidc.NewProvider(r.Context(), cfg.ProviderURL)
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

	oidcProvider, err := gooidc.NewProvider(ctx, cfg.ProviderURL)
	if err != nil {
		s.Error(w, http.StatusBadGateway, "Cannot reach OIDC provider: "+err.Error())
		return
	}

	redirectURI := s.Config.OAuthRedirectBase + "/auth/callback"
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
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		s.Error(w, http.StatusUnauthorized, "ID token verification failed: "+err.Error())
		return
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		s.Error(w, http.StatusInternalServerError, "Claims extraction failed")
		return
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

	// Encode provider URL as part of the unique key so multiple OIDC providers don't collide
	providerKey := "oidc:" + url.QueryEscape(cfg.ProviderURL)
	user, err := upsertSSOUser(ctx, s.Pool, email, name, avatarURL, providerKey, sub)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())
		return
	}

	role, err := syncGroupsAndDeriveRole(ctx, s.Pool, user.ID, groups, "oidc")
	if err != nil {
		// Non-fatal: log but continue with existing role
		_ = err
		role = user.Role
	}

	jwtToken, err := middleware.CreateToken(user.ID, user.Email, role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}
	user.Role = role
	s.JSON(w, http.StatusOK, authResponse{Token: jwtToken, User: *user})
}
