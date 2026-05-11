package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/content"
	"github.com/elearning/course-service/internal/middleware"
)

type State struct {
	Config          *config.Config
	Content         *content.Store
	GitCreds        *content.GitCredentialStore
	CooldownTracker *content.CooldownTracker
	GitCache        *content.GitCache
}

func NewState(cfg *config.Config, store *content.Store) *State {
	gc := content.NewGitCache("/tmp/elearning-git-cache", 10*time.Minute)
	content.SetGlobalGitCache(gc)

	s := &State{
		Config:          cfg,
		Content:         store,
		CooldownTracker: content.NewCooldownTracker(),
		GitCache:        gc,
	}
	if cfg.GitCredentialsPath != "" {
		if creds, err := content.LoadCredentials(cfg.GitCredentialsPath); err == nil {
			s.GitCreds = creds
		} else if !os.IsNotExist(err) {
			slog.Warn("failed to load git credentials", "path", cfg.GitCredentialsPath, "err", err)
		}
	}
	return s
}

// visibleModules returns all modules for admins, or only non-hidden for regular users.
func (s *State) visibleModules(c *content.Course, r *http.Request) []content.Module {
	claims := s.claims(r)
	if claims != nil && claims.Role == "admin" {
		return c.Modules
	}
	var out []content.Module
	for _, m := range c.Modules {
		if !m.Hidden {
			out = append(out, m)
		}
	}
	return out
}

func (s *State) tokenForRepo(repoURL string) string {
	if s.GitCreds != nil {
		if t := s.GitCreds.Match(repoURL); t != "" {
			return t
		}
	}
	return s.Config.GitToken
}

// JSON writes v as JSON with the given status code.
func (s *State) JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// Error writes a JSON error response.
func (s *State) Error(w http.ResponseWriter, status int, msg string) {
	s.JSON(w, status, map[string]string{"error": msg})
}

// claims retrieves the authenticated user's claims from the request context.
func (s *State) claims(r *http.Request) *middleware.Claims {
	return middleware.GetClaims(r)
}

// param retrieves a chi URL parameter by name.
func param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// decode unmarshals the request body into v.
func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// nullStr converts a potentially empty string to *string (nil if empty).
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// derefStr returns "" if s is nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GET /uploads/{filename}
func (s *State) ServeUpload(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		s.Error(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	path := filepath.Join(s.Config.UploadsDir, filename)
	http.ServeFile(w, r, path)
}
