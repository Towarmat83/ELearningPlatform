package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	DB              *pgxpool.Pool
}

func NewState(cfg *config.Config, store *content.Store) *State {
	gc := content.NewGitCache("/tmp/elearning-git-cache", time.Duration(cfg.GitCacheTTL)*time.Minute)
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

// visibleModules expands type:modules index entries, then returns all modules
// for admins or only non-hidden modules for regular users.
func (s *State) visibleModules(c *content.Course, r *http.Request) []content.Module {
	expanded := make([]content.Module, 0, len(c.Modules))
	for _, m := range c.Modules {
		if m.Type == "modules" {
			subs, err := content.FetchModuleIndex(s.GitCache, m, s.tokenForRepo(m.Src))
			if err != nil {
				slog.Warn("failed to expand module index, skipping", "module", m.Name, "err", err)
				continue
			}
			expanded = append(expanded, subs...)
		} else {
			expanded = append(expanded, m)
		}
	}
	claims := s.claims(r)
	if claims != nil && claims.Role == "admin" {
		return expanded
	}
	var out []content.Module
	for _, m := range expanded {
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

// ClearCache godoc
// @Summary   Clear the entire git content cache (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]string
// @Router    /api/admin/cache/clear [post]
func (s *State) ClearCache(w http.ResponseWriter, r *http.Request) {
	s.GitCache.Clear()
	slog.Info("git cache cleared by admin")
	s.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ClearCourseCache godoc
// @Summary   Clear the git cache for all repos used by a course (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/cache/clear [post]
func (s *State) ClearCourseCache(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	cleared := 0
	seen := make(map[string]bool)
	for _, m := range c.Modules {
		if m.Src == "" || m.Ref == "" {
			continue
		}
		key := m.Src + ":" + m.Ref
		if !seen[key] {
			seen[key] = true
			s.GitCache.ClearRepo(m.Src, m.Ref)
			cleared++
		}
	}
	slog.Info("course cache cleared", "slug", slug, "repos", cleared)
	s.JSON(w, http.StatusOK, map[string]any{"status": "ok", "repos_cleared": cleared})
}

// ClearModuleCache godoc
// @Summary   Clear the git cache for a specific module (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Param     slug   path  string  true  "Course slug"
// @Param     index  path  int     true  "Module index (0-based)"
// @Success   200    {object}  map[string]interface{}
// @Failure   404    {object}  map[string]string
// @Router    /api/admin/courses/{slug}/modules/{index}/cache/clear [post]
func (s *State) ClearModuleCache(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	modules := s.visibleModules(c, r)
	idx, err := strconv.Atoi(param(r, "index"))
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(w, http.StatusNotFound, "Module not found")
		return
	}
	m := modules[idx]
	if m.Src == "" || m.Ref == "" {
		s.JSON(w, http.StatusOK, map[string]any{"status": "ok", "repos_cleared": 0})
		return
	}
	s.GitCache.ClearRepo(m.Src, m.Ref)
	slog.Info("module cache cleared", "slug", slug, "index", idx, "module", m.Name)
	s.JSON(w, http.StatusOK, map[string]any{"status": "ok", "repos_cleared": 1})
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
