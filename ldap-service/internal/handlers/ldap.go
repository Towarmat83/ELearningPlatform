package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
)

// ldapTimeout is the max duration for a complete LDAP authentication (including
// Authentik auth-flow evaluation which can be slow on a dev cluster).
const ldapTimeout = 5 * time.Minute

type ldapSettings struct {
	Enabled       bool
	ServerURL     string
	BindDN        string
	BindPassword  string
	UserBaseDN    string
	UserFilter    string // %s → email
	GroupBaseDN   string
	GroupFilter   string // %s → user DN
}

func (s *State) loadLDAPSettings(ctx context.Context) (ldapSettings, error) {
	cfg := ldapSettings{
		Enabled:      s.readSetting(ctx, "ldap_enabled", "false") == "true",
		ServerURL:    s.readSetting(ctx, "ldap_server_url", ""),
		BindDN:       s.readSetting(ctx, "ldap_bind_dn", ""),
		BindPassword: s.readSetting(ctx, "ldap_bind_password", ""),
		UserBaseDN:   s.readSetting(ctx, "ldap_user_base_dn", ""),
		UserFilter:   s.readSetting(ctx, "ldap_user_filter", "(mail=%s)"),
		GroupBaseDN:  s.readSetting(ctx, "ldap_group_base_dn", ""),
		GroupFilter:  s.readSetting(ctx, "ldap_group_filter", "(|(member=%s)(uniqueMember=%s)(memberUid=%s))"),
	}
	if !cfg.Enabled {
		return cfg, fmt.Errorf("LDAP authentication is not enabled")
	}
	if cfg.ServerURL == "" || cfg.UserBaseDN == "" {
		return cfg, fmt.Errorf("LDAP not fully configured (ldap_server_url and ldap_user_base_dn required)")
	}
	return cfg, nil
}

// LDAPLogin godoc
// @Summary  Authenticate via LDAP and receive a JWT
// @Tags     LDAP
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "email and password"
// @Success  200   {object}  map[string]interface{}
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/ldap/login [post]
func (s *State) LDAPLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Email == "" || req.Password == "" {
		s.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ldapTimeout)
	defer cancel()

	cfg, err := s.loadLDAPSettings(ctx)
	if err != nil {
		s.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	conn, err := ldap.DialURL(cfg.ServerURL)
	if err != nil {
		s.Error(w, http.StatusBadGateway, "Cannot connect to LDAP server: "+err.Error())
		return
	}
	defer conn.Close()

	// Service bind (to search for the user)
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			s.Error(w, http.StatusBadGateway, "LDAP service bind failed: "+err.Error())
			return
		}
	}

	// Search for user by email
	filter := fmt.Sprintf(cfg.UserFilter, ldap.EscapeFilter(req.Email))
	searchReq := ldap.NewSearchRequest(
		cfg.UserBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		1, 0, false,
		filter,
		[]string{"dn", "cn", "mail", "displayName", "givenName", "sn"},
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil || len(result.Entries) == 0 {
		s.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	entry := result.Entries[0]
	userDN := entry.DN

	// Verify password by binding as the user
	if err := conn.Bind(userDN, req.Password); err != nil {
		s.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Re-bind as service account to search groups
	if cfg.BindDN != "" {
		_ = conn.Bind(cfg.BindDN, cfg.BindPassword)
	}

	// Resolve display name
	name := entry.GetAttributeValue("displayName")
	if name == "" {
		name = entry.GetAttributeValue("cn")
	}
	if name == "" {
		first := entry.GetAttributeValue("givenName")
		last := entry.GetAttributeValue("sn")
		name = strings.TrimSpace(first + " " + last)
	}
	if name == "" {
		name = req.Email
	}

	// Search LDAP groups
	var groups []string
	if cfg.GroupBaseDN != "" {
		escapedDN := ldap.EscapeFilter(userDN)
		// The filter may have multiple %s placeholders (e.g. member=%s OR uniqueMember=%s OR memberUid=%s)
		groupFilterFormatted := fmt.Sprintf(cfg.GroupFilter, escapedDN, escapedDN, escapedDN)
		groupReq := ldap.NewSearchRequest(
			cfg.GroupBaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false,
			groupFilterFormatted,
			[]string{"cn"},
			nil,
		)
		groupResult, err := conn.Search(groupReq)
		if err == nil {
			for _, g := range groupResult.Entries {
				if cn := g.GetAttributeValue("cn"); cn != "" {
					groups = append(groups, cn)
				}
			}
		}
	}

	// Delegate user creation + JWT issuance to user-service
	payload := map[string]any{
		"email":            req.Email,
		"name":             name,
		"provider":         "ldap",
		"provider_user_id": userDN,
		"groups":           groups,
		"group_source":     "ldap",
	}
	rawResp, err := s.callUserServiceSSOLogin(ctx, payload)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "User service error: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(rawResp)
}
