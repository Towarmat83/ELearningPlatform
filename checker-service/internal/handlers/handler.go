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

type Handler struct {
	config *config.Config
}

func New(cfg *config.Config) *Handler {
	return &Handler{config: cfg}
}

func (h *Handler) BuildRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(cors.New(cors.Options{
		AllowedOrigins: h.config.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}).Handler)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

	r.Post("/evaluate", h.Evaluate)

	return r
}

func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var req checker.EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Project == "" || req.Policy == "" {
		httpErr(w, http.StatusBadRequest, "username, project and policy are required")
		return
	}

	if h.config.GitLabToken == "" {
		httpErr(w, http.StatusInternalServerError, "GITLAB_TOKEN not configured")
		return
	}

	fetcher, err := checker.NewFetcher(h.config.GitLabToken, h.config.GitLabBaseURL)
	if err != nil {
		slog.Error("gitlab client init", "err", err)
		httpErr(w, http.StatusInternalServerError, "gitlab client error")
		return
	}

	state, err := fetcher.Fetch(req.Project, req.Files)
	if err != nil {
		slog.Error("gitlab fetch", "project", req.Project, "err", err)
		httpErr(w, http.StatusBadGateway, "failed to fetch GitLab state: "+err.Error())
		return
	}

	result, err := checker.Evaluate(r.Context(), req.Policy, state)
	if err != nil {
		slog.Error("rego eval", "err", err)
		httpErr(w, http.StatusInternalServerError, "policy evaluation error: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}
