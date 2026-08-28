package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/genesary/pupitre/course-service/internal/definition"
)

// pathNotFoundMessage is returned when a slug matches no learning path.
const pathNotFoundMessage = "Path not found"

// pathMessages is the client-facing wording for path definitions.
//
//nolint:gochecknoglobals // static response wording, read-only
var pathMessages = definitionMessages{
	Kind:         "path",
	InvalidSpec:  "Invalid path spec",
	Conflict:     "Path already exists",
	NotFound:     pathNotFoundMessage,
	CreateFailed: "Failed to create path",
	UpdateFailed: "Failed to update path",
}

// CreatePath godoc
// @Summary  Create a learning path (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths [post].
func (s *State) CreatePath(writer http.ResponseWriter, req *http.Request) {
	createDefinition(s, writer, req, pathMessages, definition.Path.ToPath, s.Repos.Paths.Create)
}

// UpdatePath godoc
// @Summary  Replace a learning path definition (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths/{slug}/definition [put].
func (s *State) UpdatePath(writer http.ResponseWriter, req *http.Request) {
	updateDefinition(s, writer, req, pathMessages, definition.Path.ToPath, s.pathExists, s.Repos.Paths.Upsert)
}

// DeletePath godoc
// @Summary  Delete a learning path (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths/{slug}/definition [delete].
func (s *State) DeletePath(writer http.ResponseWriter, req *http.Request) {
	deleteDefinition(s, writer, req, pathMessages, "Path deleted", s.Repos.Paths.Delete)
}

// pathExists reports whether slug names a stored path, returning
// repository.ErrNotFound when it does not.
func (s *State) pathExists(ctx context.Context, slug string) error {
	_, err := s.Repos.Paths.Get(ctx, slug)
	if err != nil {
		return fmt.Errorf("look up path %s: %w", slug, err)
	}

	return nil
}
