package handlers

import (
	"fmt"
	"net/http"

	"github.com/elearning/course-service/internal/content"
)

// ListPaths returns all learning paths known to the service.
// @Summary  List all learning paths
// @Tags     Paths
// @Produce  json
// @Success  200  {object}  map[string]interface{}
// @Router   /api/paths [get]
func (s *State) ListPaths(w http.ResponseWriter, r *http.Request) {
	paths := s.Paths.List()
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
