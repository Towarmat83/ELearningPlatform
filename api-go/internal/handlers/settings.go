package handlers

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

var allowedSettingKeys = map[string]bool{
	"gitlab_url":                   true,
	"registration_enabled":         true,
	"registration_email_whitelist": true,
	"password_min_length":          true,
	"password_require_uppercase":   true,
	"password_require_number":      true,
	"profile_allow_username_change": true,
	"sso_local_login_enabled":      true,
}

// ReadSetting fetches a single setting from the DB, returning fallback if absent.
func ReadSetting(ctx context.Context, pool *pgxpool.Pool, key, fallback string) string {
	var val string
	err := pool.QueryRow(ctx, "SELECT value FROM platform_settings WHERE key = $1", key).Scan(&val)
	if err != nil {
		return fallback
	}
	return val
}

// GET /api/settings/public
func (s *State) PublicSettings(w http.ResponseWriter, r *http.Request) {
	const pubKeys = "registration_enabled,sso_local_login_enabled,password_min_length,password_require_uppercase,password_require_number"
	keys := []string{
		"registration_enabled", "sso_local_login_enabled",
		"password_min_length", "password_require_uppercase", "password_require_number",
	}
	_ = pubKeys
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ReadSetting(r.Context(), s.Pool, k, "")
	}
	s.JSON(w, http.StatusOK, out)
}

// GET /api/admin/settings
func (s *State) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		"SELECT key, value, description FROM platform_settings ORDER BY key")
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
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
		var se setting
		if err := rows.Scan(&se.Key, &se.Value, &se.Description); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		settings = append(settings, se)
	}
	s.JSON(w, http.StatusOK, map[string]any{"settings": settings})
}

// PUT /api/admin/settings — body: {"key": "value", ...}
func (s *State) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(body) == 0 {
		s.Error(w, http.StatusBadRequest, "No settings provided")
		return
	}
	for k, v := range body {
		if !allowedSettingKeys[k] {
			s.Error(w, http.StatusBadRequest, "Unknown setting key: '"+k+"'")
			return
		}
		val, ok := v.(string)
		if !ok {
			s.Error(w, http.StatusBadRequest, "Setting '"+k+"' must be a string")
			return
		}
		_, err := s.Pool.Exec(r.Context(),
			`INSERT INTO platform_settings (key, value, updated_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
			k, val)
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
	}
	s.GetSettings(w, r)
}
