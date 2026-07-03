package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/elearning/course-service/internal/content"
)

// ListPaths returns learning paths known to the service, optionally paginated
// via the limit/offset query params. A zero, negative, or omitted limit means
// unlimited — callers can pass e.g. limit=-1 to explicitly request everything.
// @Summary  List all learning paths
// @Tags     Paths
// @Produce  json
// @Param    limit   query  int  false  "Max number of paths to return (<=0 or omitted means unlimited)"
// @Param    offset  query  int  false  "Number of paths to skip"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/paths [get]
func (s *State) ListPaths(w http.ResponseWriter, r *http.Request) {
	paths := s.Paths.List()

	if offset, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && offset > 0 {
		if offset > len(paths) {
			offset = len(paths)
		}
		paths = paths[offset:]
	}
	if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && limit > 0 && limit < len(paths) {
		paths = paths[:limit]
	}

	s.JSON(w, http.StatusOK, map[string][]*content.Path{"paths": paths})
}

// GetPath returns a single learning path by its slug.
// @Summary  Get a learning path by slug
// @Tags     Paths
// @Produce  json
// @Param    slug  path  string  true  "Path slug"
// @Success  200   {object}  content.Path
// @Failure  404   {object}  map[string]string
// @Router   /api/paths/{slug} [get]
func (s *State) GetPath(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	p := s.Paths.Get(slug)
	if p == nil {
		s.Error(w, http.StatusNotFound, fmt.Sprintf("Path %s not found", slug))
		return
	}
	s.JSON(w, http.StatusOK, p)
}
