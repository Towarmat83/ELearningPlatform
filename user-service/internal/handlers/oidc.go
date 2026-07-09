package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// oidcProviderKey identifies the dedicated single-instance OIDC login flow,
// as distinct from the generic multi-provider OAuth flow in oauth.go.
const oidcProviderKey = "oidc"

// oidcSettings holds the platform-configured settings for the dedicated OIDC
// provider instance, as read from platform_settings by loadOIDCSettings.
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

// loadOIDCSettings reads OIDC configuration from platform settings and
// validates that OIDC is enabled and fully configured.
func (s *State) loadOIDCSettings(ctx context.Context) (oidcSettings, error) {
	cfg := oidcSettings{
		Enabled:            ReadSetting(ctx, s.Pool, "oidc_enabled", "false") == authSettingTrue,
		ProviderURL:        ReadSetting(ctx, s.Pool, "oidc_provider_url", ""),
		IssuerURL:          ReadSetting(ctx, s.Pool, "oidc_issuer_url", ""),
		ClientID:           ReadSetting(ctx, s.Pool, "oidc_client_id", ""),
		ClientSecret:       ReadSetting(ctx, s.Pool, "oidc_client_secret", ""),
		GroupClaim:         ReadSetting(ctx, s.Pool, "oidc_group_claim", "groups"),
		RedirectBase:       ReadSetting(ctx, s.Pool, "oidc_redirect_base", s.Config.OAuthRedirectBase),
		BrowserBaseURL:     ReadSetting(ctx, s.Pool, "oidc_browser_base_url", ""),
		InsecureSkipVerify: ReadSetting(ctx, s.Pool, "oidc_insecure_skip_verify", "false") == authSettingTrue,
	}

	scopes := ReadSetting(ctx, s.Pool, "oidc_scopes", "openid email profile groups")
	for sc := range strings.SplitSeq(scopes, " ") {
		if sc = strings.TrimSpace(sc); sc != "" {
			cfg.Scopes = append(cfg.Scopes, sc)
		}
	}

	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, oauthFieldEmail, oauthScopeProfile}
	}

	if !cfg.Enabled {
		return cfg, errors.New("OIDC is not enabled")
	}

	if cfg.ProviderURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return cfg, errors.New("OIDC not fully configured (provider_url, clientId, clientSecret required)")
	}

	return cfg, nil
}

// oidcContext sets InsecureIssuerURLContext when cfg.IssuerURL is given, so
// discovery (ProviderURL) may differ from the issuer claim in tokens. This
// supports split-horizon setups (e.g. KinD) where internal DNS != public URL.
func oidcContext(ctx context.Context, cfg oidcSettings) context.Context {
	if cfg.InsecureSkipVerify {
		insecureClient := &http.Client{
			Transport: &http.Transport{
				//nolint:gosec // operator opt-in via oidc_insecure_skip_verify setting, for internal/self-signed CAs
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		ctx = gooidc.ClientContext(ctx, insecureClient)
	}

	if cfg.IssuerURL != "" {
		return gooidc.InsecureIssuerURLContext(ctx, cfg.IssuerURL)
	}

	return ctx
}

// extractURLBase returns the scheme+host portion of a URL string.
func extractURLBase(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}

	return parsed.Scheme + "://" + parsed.Host
}

// OIDCAuthorize godoc
// @Summary  Get OIDC authorization URL
// @Tags     OIDC
// @Produce  json
// @Success  200  {object}  map[string]string
// @Failure  400  {object}  map[string]string
// @Router   /api/auth/oidc/authorize [get].
func (s *State) OIDCAuthorize(writer http.ResponseWriter, request *http.Request) {
	cfg, err := s.loadOIDCSettings(request.Context())
	if err != nil {
		zap.S().Errorw("load OIDC settings", "err", err)
		s.Error(writer, http.StatusBadRequest, "OIDC not configured")

		return
	}

	stateToken, err := makeOAuthState(oidcProviderKey, s.oauthStateSecret())
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "State token error")

		return
	}

	redirectURI := strings.TrimRight(cfg.RedirectBase, "/") + "/auth/callback"

	providerCtx := oidcContext(request.Context(), cfg)

	provider, err := gooidc.NewProvider(providerCtx, cfg.ProviderURL)
	if err != nil {
		zap.S().Errorw("OIDC provider unreachable", "err", err)
		s.Error(writer, http.StatusInternalServerError, "OIDC provider call failed")

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

	s.JSON(writer, http.StatusOK, map[string]string{"url": authURL, "state": stateToken})
}

// oidcGroupsFromClaims extracts group names from the configured group claim,
// which may be either a JSON array of strings or a comma-separated string.
func oidcGroupsFromClaims(claims map[string]any, groupClaim string) []string {
	raw, ok := claims[groupClaim]
	if !ok {
		return nil
	}

	var groups []string

	switch rawValue := raw.(type) {
	case []any:
		for _, item := range rawValue {
			if group, ok := item.(string); ok {
				groups = append(groups, group)
			}
		}
	case string:
		for group := range strings.SplitSeq(rawValue, ",") {
			if group = strings.TrimSpace(group); group != "" {
				groups = append(groups, group)
			}
		}
	}

	return groups
}

// exchangeOIDCToken performs the OIDC token exchange and ID-token
// verification for an authorization code, returning the merged claims (ID
// token plus UserInfo endpoint) and the token subject. On failure it writes
// the HTTP error response itself and returns ok=false.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func (s *State) exchangeOIDCToken(
	writer http.ResponseWriter, ctx context.Context, cfg oidcSettings, code string,
) (map[string]any, string, bool) {
	providerCtx := oidcContext(ctx, cfg)

	oidcProvider, err := gooidc.NewProvider(providerCtx, cfg.ProviderURL)
	if err != nil {
		zap.S().Errorw("OIDC provider unreachable", "err", err)
		s.Error(writer, http.StatusInternalServerError, "OIDC provider call failed")

		return nil, "", false
	}

	redirectURI := strings.TrimRight(cfg.RedirectBase, "/") + "/auth/callback"
	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	token, err := oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		zap.S().Errorw("OIDC token exchange failed", "err", err)
		s.Error(writer, http.StatusUnauthorized, "Token exchange failed")

		return nil, "", false
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.Error(writer, http.StatusUnauthorized, "No id_token in response")

		return nil, "", false
	}

	verifier := oidcProvider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	idToken, err := verifier.Verify(providerCtx, rawIDToken)
	if err != nil {
		zap.S().Errorw("OIDC ID token verification failed", "err", err)
		s.Error(writer, http.StatusUnauthorized, "Token verification failed")

		return nil, "", false
	}

	var claims map[string]any

	err = idToken.Claims(&claims)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Claims extraction failed")

		return nil, "", false
	}

	enrichClaimsFromUserInfo(ctx, oidcProvider, token, claims)

	return claims, idToken.Subject, true
}

// OIDCCallback godoc
// @Summary  Complete OIDC login flow
// @Tags     OIDC
// @Accept   json
// @Produce  json
// @Param    body  object  true  "code and state"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/oidc/callback [post].
func (s *State) OIDCCallback(writer http.ResponseWriter, request *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}

	err := decode(request, &req)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	provider, validState := decodeOAuthState(req.State, s.oauthStateSecret())
	if !validState || provider != oidcProviderKey {
		s.Error(writer, http.StatusUnauthorized, "Invalid or expired OIDC state")

		return
	}

	ctx := request.Context()

	cfg, err := s.loadOIDCSettings(ctx)
	if err != nil {
		zap.S().Errorw("load OIDC settings", "err", err)
		s.Error(writer, http.StatusBadRequest, "OIDC not configured")

		return
	}

	claims, sub, ok := s.exchangeOIDCToken(writer, ctx, cfg, req.Code)
	if !ok {
		return
	}

	email, _ := claims[oauthFieldEmail].(string)
	if email == "" {
		s.Error(writer, http.StatusUnauthorized, "No email in OIDC token")

		return
	}

	name := oidcDisplayName(claims, email)

	var avatarURL *string
	if pic, ok := claims["picture"].(string); ok && pic != "" {
		avatarURL = &pic
	}

	bio := oidcBioFromClaims(claims)
	groups := oidcGroupsFromClaims(claims, cfg.GroupClaim)

	s.completeSSOLogin(ctx, writer, email, name, avatarURL, bio, oidcProviderKey, sub, groups)
}
