package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"

	"github.com/elearning/user-service/internal/middleware"
)

const ldapTimeout = 5 * time.Minute

type ldapSettings struct {
	Enabled      bool
	ServerURL    string
	BindDN       string
	BindPassword string
	UserBaseDN   string
	UserFilter   string
	GroupBaseDN  string
	GroupFilter  string
}

func (s *State) loadLDAPSettings(ctx context.Context) (ldapSettings, error) {
	cfg := ldapSettings{
		Enabled:      ReadSetting(ctx, s.Pool, "ldap_enabled", "false") == "true",
		ServerURL:    ReadSetting(ctx, s.Pool, "ldap_server_url", ""),
		BindDN:       ReadSetting(ctx, s.Pool, "ldap_bind_dn", ""),
		BindPassword: ReadSetting(ctx, s.Pool, "ldap_bind_password", ""),
		UserBaseDN:   ReadSetting(ctx, s.Pool, "ldap_user_base_dn", ""),
		UserFilter:   ReadSetting(ctx, s.Pool, "ldap_user_filter", "(mail=%s)"),
		GroupBaseDN:  ReadSetting(ctx, s.Pool, "ldap_group_base_dn", ""),
		GroupFilter:  ReadSetting(ctx, s.Pool, "ldap_group_filter", "(|(member=%s)(uniqueMember=%s)(memberUid=%s))"),
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
// @Success  200   {object}  authResponse
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

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			s.Error(w, http.StatusBadGateway, "LDAP service bind failed: "+err.Error())
			return
		}
	}

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

	if err := conn.Bind(userDN, req.Password); err != nil {
		s.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	if cfg.BindDN != "" {
		_ = conn.Bind(cfg.BindDN, cfg.BindPassword)
	}

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

	var groups []string
	if cfg.GroupBaseDN != "" {
		escapedDN := ldap.EscapeFilter(userDN)
		groupFilter := fmt.Sprintf(cfg.GroupFilter, escapedDN, escapedDN, escapedDN)
		groupReq := ldap.NewSearchRequest(
			cfg.GroupBaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false,
			groupFilter,
			[]string{"cn"},
			nil,
		)
		if groupResult, err := conn.Search(groupReq); err == nil {
			for _, g := range groupResult.Entries {
				if cn := g.GetAttributeValue("cn"); cn != "" {
					groups = append(groups, cn)
				}
			}
		}
	}

	user, err := upsertSSOUser(ctx, s.Pool, req.Email, name, nil, "ldap", userDN)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())
		return
	}

	role, err := syncGroupsAndDeriveRole(ctx, s.Pool, user.ID, groups, "ldap")
	if err != nil {
		role = user.Role
	}
	addToDefaultGroup(ctx, s.Pool, user.ID)
	syncGroupEnrollments(ctx, s.Pool, user.ID)

	token, err := middleware.CreateToken(user.ID, user.Email, role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}
	user.Role = role
	s.JSON(w, http.StatusOK, authResponse{Token: token, User: *user})
}
