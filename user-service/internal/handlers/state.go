package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"

	"github.com/genesary/pupitre/user-service/internal/config"
	"github.com/genesary/pupitre/user-service/internal/middleware"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// maxRequestBodyBytes caps the size of accepted request bodies (1 MB).
const maxRequestBodyBytes = 1 << 20

// State holds the shared dependencies used by every HTTP handler in
// this package: the GORM-backed repositories, application config, and the
// batched read-through view of the course catalog.
type State struct {
	Repos  *repository.Repositories
	Config *config.Config

	// Catalog resolves course, path and skill metadata from
	// course-service. Nil until catalog is first called, which builds it
	// from Config — so a State assembled as a bare struct literal (as
	// main and the tests both do) still gets one.
	Catalog *Catalog

	catalogOnce sync.Once
}

// Health godoc
// @Summary  Liveness check
// @Tags     Health
// @Produce  json
// @Success  200  {object}  map[string]string
// @Router   /health [get].
func (s *State) Health(w http.ResponseWriter, r *http.Request) {
	s.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "user-service",
		"version": "1.0.0",
	})
}

// JSON writes v to w as a JSON response with the given HTTP status.
func (s *State) JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		zap.L().Error("failed to encode JSON response", zap.Error(err))
	}
}

// Error writes a JSON error response with the given HTTP status and msg.
func (s *State) Error(w http.ResponseWriter, status int, msg string) {
	s.JSON(w, status, map[string]string{"error": msg})
}

// claims returns the authenticated claims stored on the request context.
func (s *State) claims(r *http.Request) *middleware.Claims {
	return middleware.GetClaims(r)
}

// param returns the chi URL parameter identified by key.
func param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// decode reads and decodes the JSON request body into v.
func decode(r *http.Request, v any) error {
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}

	return nil
}

// nullStr returns nil for an empty string, otherwise a pointer to s.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// derefStr returns "" for a nil pointer, otherwise the pointed-to value.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// catalog returns the shared Catalog, building it on first use.
func (s *State) catalog() *Catalog {
	s.catalogOnce.Do(func() {
		if s.Catalog == nil {
			s.Catalog = NewCatalog(s.Config.CourseServiceURL)
		}
	})

	return s.Catalog
}
