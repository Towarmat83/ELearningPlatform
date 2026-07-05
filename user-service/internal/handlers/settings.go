package handlers

import (
	"context"
	"net/http"

	"github.com/elearning/user-service/internal/db"
)

// Platform setting keys read from more than one file; pulled out to satisfy
// goconst.
const (
	settingKeyRegistrationEnabled      = "registration_enabled"
	settingKeySSOLocalLoginEnabled     = "sso_local_login_enabled"
	settingKeyPasswordMinLength        = "password_min_length"
	settingKeyPasswordRequireUppercase = "password_require_uppercase"
	settingKeyPasswordRequireNumber    = "password_require_number"
	settingKeyOIDCEnabled              = "oidc_enabled"
	settingKeyLDAPEnabled              = "ldap_enabled"
)

// allowedSettingKeys is the set of platform setting keys clients may write.
//
//nolint:gochecknoglobals // static allow-list of settings keys, read-only
var allowedSettingKeys = map[string]bool{
	"gitlab_url":                       true,
	settingKeyRegistrationEnabled:      true,
	"registration_email_whitelist":     true,
	settingKeyPasswordMinLength:        true,
	settingKeyPasswordRequireUppercase: true,
	settingKeyPasswordRequireNumber:    true,
	"profile_allow_username_change":    true,
	settingKeySSOLocalLoginEnabled:     true,
	// OIDC
	settingKeyOIDCEnabled:       true,
	"oidc_provider_url":         true,
	"oidc_issuer_url":           true,
	"oidc_redirect_base":        true,
	"oidc_browser_base_url":     true,
	"oidc_client_id":            true,
	"oidc_client_secret":        true,
	"oidc_scopes":               true,
	"oidc_group_claim":          true,
	"oidc_insecure_skip_verify": true,
	// LDAP
	settingKeyLDAPEnabled: true,
	"ldap_server_url":     true,
	"ldap_bind_dn":        true,
	"ldap_bind_password":  true,
	"ldap_user_base_dn":   true,
	"ldap_user_filter":    true,
	"ldap_group_base_dn":  true,
	"ldap_group_filter":   true,
}

// ReadSetting reads a platform setting value by key, returning fallback when
// the key is unset or the query fails.
func ReadSetting(ctx context.Context, pool db.Pool, key, fallback string) string {
	var val string

	err := pool.QueryRow(ctx, "SELECT value FROM platform_settings WHERE key = $1", key).Scan(&val)
	if err != nil {
		return fallback
	}

	return val
}

// PublicSettings godoc
// @Summary  Get public platform settings
// @Tags     Settings
// @Produce  json
// @Success  200  {object}  map[string]string
// @Router   /api/settings/public [get].
func (s *State) PublicSettings(writer http.ResponseWriter, request *http.Request) {
	keys := []string{
		settingKeyRegistrationEnabled, settingKeySSOLocalLoginEnabled,
		settingKeyPasswordMinLength, settingKeyPasswordRequireUppercase, settingKeyPasswordRequireNumber,
		settingKeyOIDCEnabled, settingKeyLDAPEnabled,
	}

	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = ReadSetting(request.Context(), s.Pool, key, "")
	}

	s.JSON(writer, http.StatusOK, out)
}

// GetSettings godoc
// @Summary   Get all platform settings (admin)
// @Tags      Admin - Settings
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/settings [get].
func (s *State) GetSettings(writer http.ResponseWriter, request *http.Request) {
	rows, err := s.Pool.Query(request.Context(),
		"SELECT key, value, description FROM platform_settings ORDER BY key")
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type setting struct {
		Key         string  `json:"key"`
		Value       string  `json:"value"`
		Description *string `json:"description"`
	}

	var settings []setting

	for rows.Next() {
		var current setting

		err := rows.Scan(&current.Key, &current.Value, &current.Description)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Scan error")

			return
		}

		settings = append(settings, current)
	}

	s.JSON(writer, http.StatusOK, map[string]any{"settings": settings})
}

// UpdateSettings godoc
// @Summary   Update platform settings (admin)
// @Tags      Admin - Settings
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  map[string]string  true  "Key/value settings map"
// @Success   200   {object}  map[string]interface{}
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/settings [put].
func (s *State) UpdateSettings(writer http.ResponseWriter, request *http.Request) {
	var body map[string]any

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if len(body) == 0 {
		s.Error(writer, http.StatusBadRequest, "No settings provided")

		return
	}

	for key, value := range body {
		if !allowedSettingKeys[key] {
			s.Error(writer, http.StatusBadRequest, "Unknown setting key: '"+key+"'")

			return
		}

		val, ok := value.(string)
		if !ok {
			s.Error(writer, http.StatusBadRequest, "Setting '"+key+"' must be a string")

			return
		}

		_, err := s.Pool.Exec(request.Context(),
			`INSERT INTO platform_settings (key, value, updatedAt)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updatedAt = NOW()`,
			key, val)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Database error")

			return
		}
	}

	s.GetSettings(writer, request)
}
