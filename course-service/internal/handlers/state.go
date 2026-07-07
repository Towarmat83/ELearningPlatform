package handlers

import (
	"encoding/json"
	"fmt"
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

// statusOKValue is the JSON value reported for successful status fields.
const statusOKValue = "ok"

// statusJSONKey is the JSON key used for status fields in responses.
const statusJSONKey = "status"

// errorJSONKey is the JSON key used for error fields in responses.
const errorJSONKey = "error"

// modulesTypeValue identifies a module index entry of type "modules".
const modulesTypeValue = "modules"

// reposClearedJSONKey is the JSON key used for the repos-cleared count.
const reposClearedJSONKey = "reposCleared"

// roleAdmin is the claims role value granting administrative access.
const roleAdmin = "admin"

// courseServiceName identifies this service in health check responses.
const courseServiceName = "course-service"

// moduleTypeLab identifies a module whose content is an interactive lab.
const moduleTypeLab = "lab"

// moduleTypeQuiz identifies a module whose content is a graded quiz.
const moduleTypeQuiz = "quiz"

// moduleTypeText identifies a module whose content is plain text/markdown.
const moduleTypeText = "text"

// moduleTypeImage identifies a module whose content is an image.
const moduleTypeImage = "image"

// checkProviderLocal identifies a lab module verified locally via Tauri.
const checkProviderLocal = "local"

// checkProviderGitLab identifies a lab module verified via GitLab step checks.
const checkProviderGitLab = "gitlab"

// labTypeForm is the legacy labType reported for text-content modules.
const labTypeForm = "form"

// labTypeInteractive is the legacy labType reported for interactive
// modules (video, image, or anything without a dedicated labType).
const labTypeInteractive = "interactive"

// coursesJSONKey is the JSON key used for course-list response bodies.
const coursesJSONKey = "courses"

// progressJSONKey is the JSON key used for progress response fields.
const progressJSONKey = "progress"

// userIDJSONKey is the JSON key used for user-id response/request fields.
const userIDJSONKey = "userId"

// courseSlugJSONKey is the JSON key used for course-slug fields.
const courseSlugJSONKey = "courseSlug"

// slugJSONKey is the JSON key used for course-slug response fields.
const slugJSONKey = "slug"

// messageJSONKey is the JSON key used for human-readable message fields.
const messageJSONKey = "message"

// lessonsJSONKey is the JSON key used for lesson-list response bodies.
const lessonsJSONKey = "lessons"

// lessonJSONKey is the JSON key used for single-lesson response bodies.
const lessonJSONKey = "lesson"

// lessonCompleteMessage is returned when a lesson is marked complete.
const lessonCompleteMessage = "Lesson marked as complete"

// totalJSONKey is the JSON key used for total-count response fields.
const totalJSONKey = "total"

// moduleTypeVideo identifies a module whose content is a video.
const moduleTypeVideo = "video"

// State holds the shared dependencies used by the course-service HTTP
// handlers, such as content stores, git caches, and the database pool.
type State struct {
	Config          *config.Config
	Content         *content.Store
	Paths           *content.PathStore
	GitCreds        *content.GitCredentialStore
	CooldownTracker *content.CooldownTracker
	GitCache        *content.GitCache
	DB              *pgxpool.Pool
}

// NewState builds a State from the given config and content stores,
// wiring up the git cache and optional git credentials.
func NewState(cfg *config.Config, store *content.Store, paths *content.PathStore) *State {
	gitCache := content.NewGitCache("/tmp/elearning-git-cache", time.Duration(cfg.GitCacheTTL)*time.Minute)

	state := &State{
		Config:          cfg,
		Content:         store,
		Paths:           paths,
		CooldownTracker: content.NewCooldownTracker(),
		GitCache:        gitCache,
	}

	if cfg.GitCredentialsPath != "" {
		creds, err := content.LoadCredentials(cfg.GitCredentialsPath)

		switch {
		case err == nil:
			state.GitCreds = creds
		case !os.IsNotExist(err):
			slog.Warn("failed to load git credentials", "path", cfg.GitCredentialsPath, "err", err)
		}
	}

	return state
}

// JSON writes v as JSON with the given status code.
func (s *State) JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		slog.Error("encode JSON response", "err", err)
	}
}

// Error writes a JSON error response.
func (s *State) Error(w http.ResponseWriter, status int, msg string) {
	s.JSON(w, status, map[string]string{errorJSONKey: msg})
}

// param retrieves a chi URL parameter by name.
func param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// decode unmarshals the request body into v.
func decode(r *http.Request, v any) error {
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}

	return nil
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
// @Router    /api/admin/cache/clear [post].
func (s *State) ClearCache(w http.ResponseWriter, r *http.Request) {
	s.GitCache.Clear()
	slog.Info("git cache cleared by admin")
	s.JSON(w, http.StatusOK, map[string]string{statusJSONKey: statusOKValue})
}

// ClearCourseCache godoc
// @Summary   Clear the git cache for all repos used by a course (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/cache/clear [post].
func (s *State) ClearCourseCache(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	course := s.Content.Get(slug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	cleared := 0
	seen := make(map[string]bool)

	for _, mod := range course.Modules {
		if mod.Src == "" || mod.Ref == "" {
			continue
		}

		key := mod.Src + ":" + mod.Ref
		if !seen[key] {
			seen[key] = true

			s.GitCache.ClearRepo(mod.Src, mod.Ref)

			cleared++
		}
	}

	slog.Info("course cache cleared", "slug", slug, "repos", cleared)
	s.JSON(writer, http.StatusOK, map[string]any{statusJSONKey: statusOKValue, reposClearedJSONKey: cleared})
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
// @Router    /api/admin/courses/{slug}/modules/{index}/cache/clear [post].
func (s *State) ClearModuleCache(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	course := s.Content.Get(slug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	modules := s.visibleModules(course, req)

	idx, err := strconv.Atoi(param(req, "index"))
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(writer, http.StatusNotFound, "Module not found")

		return
	}

	mod := modules[idx]
	if mod.Src == "" || mod.Ref == "" {
		s.JSON(writer, http.StatusOK, map[string]any{statusJSONKey: statusOKValue, reposClearedJSONKey: 0})

		return
	}

	s.GitCache.ClearRepo(mod.Src, mod.Ref)
	slog.Info("module cache cleared", "slug", slug, "index", idx, "module", mod.Name)
	s.JSON(writer, http.StatusOK, map[string]any{statusJSONKey: statusOKValue, reposClearedJSONKey: 1})
}

// ServeUpload serves a previously uploaded file by filename from the
// configured uploads directory.
// GET /uploads/{filename}.
func (s *State) ServeUpload(writer http.ResponseWriter, req *http.Request) {
	filename := chi.URLParam(req, "filename")
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		s.Error(writer, http.StatusBadRequest, "Invalid filename")

		return
	}

	path := filepath.Join(s.Config.UploadsDir, filename)
	http.ServeFile(writer, req, path) //nolint:gosec // filename validated above to reject path traversal ('..' or '/')
}

// visibleModules expands type:modules index entries, then returns all
// modules for admins or only non-hidden modules for regular users.
func (s *State) visibleModules(c *content.Course, req *http.Request) []content.Module {
	expanded := make([]content.Module, 0, len(c.Modules))

	for _, mod := range c.Modules {
		if mod.Type == modulesTypeValue {
			subs, err := content.FetchModuleIndex(req.Context(), s.GitCache, mod, s.tokenForRepo(mod.Src))
			if err != nil {
				slog.Warn("failed to expand module index, skipping", "module", mod.Name, "err", err)

				continue
			}

			expanded = append(expanded, subs...)
		} else {
			expanded = append(expanded, mod)
		}
	}

	claims := s.claims(req)
	if claims != nil && claims.Role == roleAdmin {
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

// tokenForRepo returns the git token to use for the given repo URL, falling
// back to the service-wide token when no repo-specific credential matches.
func (s *State) tokenForRepo(repoURL string) string {
	if s.GitCreds != nil {
		if t := s.GitCreds.Match(repoURL); t != "" {
			return t
		}
	}

	return s.Config.GitToken
}

// claims retrieves the authenticated user's claims from the request context.
func (s *State) claims(r *http.Request) *middleware.Claims {
	return middleware.GetClaims(r)
}
