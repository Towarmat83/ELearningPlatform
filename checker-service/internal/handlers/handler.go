// Package handlers wires up the checker-service HTTP API.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/elearning/checker-service/internal/checker"
	"github.com/elearning/checker-service/internal/config"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Handler serves the checker-service HTTP API.
type Handler struct {
	config *config.Config
}

// New creates a Handler using the given configuration.
func New(cfg *config.Config) *Handler {
	return &Handler{config: cfg}
}

// BuildRouter builds the chi router for the checker-service HTTP API.
func (h *Handler) BuildRouter() *chi.Mux {
	router := chi.NewRouter()
	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)
	router.Use(chiMiddleware.Logger)
	router.Use(chiMiddleware.Recoverer)
	router.Use(cors.New(cors.Options{
		AllowedOrigins: h.config.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}).Handler)

	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		if err != nil {
			slog.Error("encode health response", "err", err)
		}
	})

	router.Post("/evaluate", h.Evaluate)

	return router
}

// Evaluate handles POST /evaluate: it fetches GitLab state for the request
// and evaluates it against the request's OPA policy.
func (h *Handler) Evaluate(resp http.ResponseWriter, httpReq *http.Request) {
	var req checker.EvaluateRequest

	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		httpErr(resp, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.Username == "" || req.Project == "" || req.Policy == "" {
		httpErr(resp, http.StatusBadRequest, "username, project and policy are required")

		return
	}

	if h.config.GitLabToken == "" {
		httpErr(resp, http.StatusInternalServerError, "GITLAB_TOKEN not configured")

		return
	}

	fetcher, err := checker.NewFetcher(h.config.GitLabToken, h.config.GitLabBaseURL)
	if err != nil {
		slog.Error("gitlab client init", "err", err)
		httpErr(resp, http.StatusInternalServerError, "gitlab client error")

		return
	}

	state, err := fetcher.Fetch(req.Project, req.Files)
	if err != nil {
		slog.Error("gitlab fetch", "project", req.Project, "err", err)
		httpErr(resp, http.StatusBadGateway, "failed to fetch GitLab state: "+err.Error())

		return
	}

	result, err := checker.Evaluate(httpReq.Context(), req.Policy, state)
	if err != nil {
		slog.Error("rego eval", "err", err)
		httpErr(resp, http.StatusInternalServerError, "policy evaluation error: "+err.Error())

		return
	}

	resp.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(resp).Encode(result)
	if err != nil {
		slog.Error("encode evaluate response", "err", err)
	}
}

// httpErr writes a JSON error response with the given status and message.
func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(map[string]string{"error": msg})
	if err != nil {
		slog.Error("encode error response", "err", err)
	}
}
