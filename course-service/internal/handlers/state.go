package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/content"
	"github.com/elearning/course-service/internal/middleware"
)

// State is the shared application state passed to every handler.
type State struct {
	Config         *config.Config
	Content        *content.Store
	UserServiceURL string
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
