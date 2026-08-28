package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/course-service/internal/repository"
)

// decodeDefinition reads a resource definition from the request body,
// accepting either a nested "spec" object or flat top-level fields — the
// admin UI sends the former, hand-written calls tend to send the latter.
//
// kind names the resource in error messages ("course", "path").
func decodeDefinition[T any](req *http.Request, kind string) (slug string, spec T, err error) { //nolint:nonamedreturns,ireturn // named for gocritic(unnamedResult); T is a type parameter, not an interface
	var envelope struct {
		Slug string          `json:"slug"`
		Spec json.RawMessage `json:"spec"`
	}

	body, err := readAllBody(req)
	if err != nil {
		return "", spec, err
	}

	err = json.Unmarshal(body, &envelope)
	if err != nil {
		return "", spec, fmt.Errorf("decode %s request: %w", kind, err)
	}

	payload := envelope.Spec
	if len(payload) == 0 {
		payload = body
	}

	err = json.Unmarshal(payload, &spec)
	if err != nil {
		return "", spec, fmt.Errorf("decode %s spec: %w", kind, err)
	}

	return envelope.Slug, spec, nil
}

// definitionMessages carries the client-facing wording for one resource
// kind, so the shared handlers below can stay generic without producing
// vague errors.
type definitionMessages struct {
	// Kind names the resource in log lines and decode errors.
	Kind string
	// InvalidSpec is returned for an undecodable body.
	InvalidSpec string
	// Conflict is returned when the slug is already taken.
	Conflict string
	// NotFound is returned when the slug matches nothing.
	NotFound string
	// CreateFailed and UpdateFailed are returned for storage failures.
	CreateFailed string
	UpdateFailed string
}

// createDefinition decodes a create request, requires a slug, and hands the
// resulting resource to create. It writes the whole response itself.
func createDefinition[T any, R any](
	state *State, writer http.ResponseWriter, req *http.Request,
	messages definitionMessages,
	build func(slug string, spec T) R,
	create func(ctx context.Context, resource R) error,
) {
	slug, spec, err := decodeDefinition[T](req, messages.Kind)
	if err != nil {
		zap.L().Debug("invalid "+messages.Kind+" payload", zap.Error(err))
		state.Error(writer, http.StatusBadRequest, messages.InvalidSpec)

		return
	}

	if slug == "" {
		state.Error(writer, http.StatusBadRequest, "slug is required")

		return
	}

	err = create(req.Context(), build(slug, spec))

	switch {
	case errors.Is(err, repository.ErrConflict):
		state.Error(writer, http.StatusConflict, messages.Conflict)
	case err != nil:
		zap.L().Error("create "+messages.Kind+" failed", zap.String("slug", slug), zap.Error(err))
		state.Error(writer, http.StatusInternalServerError, messages.CreateFailed)
	default:
		state.JSON(writer, http.StatusCreated, map[string]any{slugJSONKey: slug})
	}
}

// updateDefinition decodes a replace request, confirms the resource exists,
// and hands the replacement to update. It writes the whole response itself.
//
// The existence check is deliberate: a PUT to an unknown slug is a 404
// rather than an implicit create, so a typo cannot quietly add a resource.
func updateDefinition[T any, R any](
	state *State, writer http.ResponseWriter, req *http.Request,
	messages definitionMessages,
	build func(slug string, spec T) R,
	exists func(ctx context.Context, slug string) error,
	update func(ctx context.Context, resource R) error,
) {
	slug := param(req, "slug")

	_, spec, err := decodeDefinition[T](req, messages.Kind)
	if err != nil {
		zap.L().Debug("invalid "+messages.Kind+" payload", zap.String("slug", slug), zap.Error(err))
		state.Error(writer, http.StatusBadRequest, messages.InvalidSpec)

		return
	}

	err = exists(req.Context(), slug)
	if err != nil {
		state.writeRepoError(writer, err, messages.NotFound, "load "+messages.Kind, zap.String("slug", slug))

		return
	}

	err = update(req.Context(), build(slug, spec))
	if err != nil {
		zap.L().Error("update "+messages.Kind+" failed", zap.String("slug", slug), zap.Error(err))
		state.Error(writer, http.StatusInternalServerError, messages.UpdateFailed)

		return
	}

	state.JSON(writer, http.StatusOK, map[string]string{slugJSONKey: slug})
}

// deleteDefinition removes a resource by slug and writes the response.
func deleteDefinition(
	state *State, writer http.ResponseWriter, req *http.Request,
	messages definitionMessages, deletedMsg string,
	remove func(ctx context.Context, slug string) error,
) {
	slug := param(req, "slug")

	err := remove(req.Context(), slug)
	if err != nil {
		state.writeRepoError(writer, err, messages.NotFound, "delete "+messages.Kind, zap.String("slug", slug))

		return
	}

	state.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: deletedMsg})
}
