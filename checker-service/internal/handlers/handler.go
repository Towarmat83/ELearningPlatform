// Package handlers wires up the checker-service HTTP API.
package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/elearning/checker-service/internal/checker"
	"github.com/elearning/checker-service/internal/config"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

// internalSecretHeader is the HTTP header used to authenticate
// service-to-service calls on /evaluate and /check-step.
const internalSecretHeader = "X-Internal-Secret"

// maxRequestBodyBytes caps the size of accepted request bodies (1 MB).
const maxRequestBodyBytes = 1 << 20

// remoteIP extracts the IP from r.RemoteAddr (already set to the real client
// IP by chiMiddleware.RealIP) to use as the rate-limit key.
func remoteIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, nil //nolint:nilerr // best-effort: fall back to raw addr
	}

	return ip, nil
}

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
	router.Use(chiMiddleware.RequestSize(maxRequestBodyBytes))

	if h.config.RateLimitRequests > 0 {
		router.Use(httprate.LimitBy(h.config.RateLimitRequests, time.Duration(h.config.RateLimitWindowSeconds)*time.Second, remoteIP))
	}

	router.Use(cors.New(cors.Options{
		AllowedOrigins: h.config.CORSOrigins,
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}).Handler)

	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		if err != nil {
			zap.L().Error("encode health response", zap.Error(err))
		}
	})

	router.Group(func(r chi.Router) {
		r.Use(h.internalAuth)
		r.Post("/evaluate", h.Evaluate)
		r.Post("/check-step", h.CheckStep)
	})

	return router
}

// Evaluate handles POST /evaluate: it fetches the Rego policy from the
// course git repository, fetches GitLab state for the request, then
// precompiles and evaluates the policy with restricted OPA capabilities.
func (h *Handler) Evaluate(resp http.ResponseWriter, httpReq *http.Request) {
	httpReq.Body = http.MaxBytesReader(resp, httpReq.Body, maxRequestBodyBytes)

	req, valid := decodeEvaluateRequest(resp, httpReq)
	if !valid {
		return // error response and log written by callee
	}

	if h.config.GitLabToken == "" {
		httpErr(resp, http.StatusInternalServerError, "GITLAB_TOKEN not configured")

		return
	}

	err := h.validateEvaluateRequest(req)
	if err != nil {
		httpErr(resp, http.StatusBadRequest, err.Error())

		return
	}

	policy, valid := h.fetchCourseCheckPolicy(resp, httpReq, req)
	if !valid {
		return // error response and log written by callee
	}

	state, valid := h.fetchStudentGitLabProjectState(resp, req)
	if !valid {
		return // error response and log written by callee
	}

	result, err := checker.Evaluate(httpReq.Context(), policy, state)
	if err != nil {
		zap.L().Error("rego eval", zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "policy evaluation error")

		return
	}

	resp.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(resp).Encode(result)
	if err != nil {
		zap.L().Error("encode evaluate response", zap.Error(err))
	}
}

// CheckStep handles POST /check-step: runs a single named GitLab check
// without a full OPA policy evaluation.
func (h *Handler) CheckStep(resp http.ResponseWriter, httpReq *http.Request) {
	var req checker.StepRequest

	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		httpErr(resp, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.Username == "" || req.Project == "" || req.CheckType == "" {
		httpErr(resp, http.StatusBadRequest, "username, project and checkType are required")

		return
	}

	if h.config.GitLabToken == "" {
		httpErr(resp, http.StatusInternalServerError, "GITLAB_TOKEN not configured")

		return
	}

	fetcher, err := checker.NewFetcher(h.config.GitLabToken, h.config.GitLabBaseURL)
	if err != nil {
		zap.L().Error("gitlab client init", zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "gitlab client error")

		return
	}

	result, err := fetcher.CheckStep(req)
	if err != nil {
		zap.L().Error("step check", zap.String("checkType", req.CheckType), zap.String("project", req.Project), zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "step check error")

		return
	}

	resp.Header().Set("Content-Type", "application/json")

	encErr := json.NewEncoder(resp).Encode(result)
	if encErr != nil {
		zap.L().Error("encode check-step response", zap.Error(encErr))
	}
}

// internalAuth is middleware that rejects requests whose X-Internal-Secret
// header does not match the configured shared secret.
func (h *Handler) internalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		got := req.Header.Get(internalSecretHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(h.config.InternalSecret)) != 1 {
			httpErr(resp, http.StatusUnauthorized, "invalid internal secret")

			return
		}

		next.ServeHTTP(resp, req)
	})
}

// decodeEvaluateRequest decodes and validates the JSON body of POST /evaluate.
// It writes an appropriate error response and returns false on failure.
func decodeEvaluateRequest(resp http.ResponseWriter, httpReq *http.Request) (checker.EvaluateRequest, bool) {
	var req checker.EvaluateRequest

	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		httpErr(resp, http.StatusBadRequest, "invalid request body")

		return checker.EvaluateRequest{}, false
	}

	if req.Username == "" || req.Project == "" {
		httpErr(resp, http.StatusBadRequest, "username and project are required")

		return checker.EvaluateRequest{}, false
	}

	if req.PolicySrc == "" || req.PolicyRef == "" || req.PolicyPath == "" {
		httpErr(resp, http.StatusBadRequest, "policySrc, policyRef and policyPath are required")

		return checker.EvaluateRequest{}, false
	}

	return req, true
}

// validateEvaluateRequest guards against SSRF by ensuring the policy source
// points to the configured GitLab instance and that ref/path are safe.
func (h *Handler) validateEvaluateRequest(req checker.EvaluateRequest) error {
	if !strings.HasPrefix(req.PolicySrc, h.config.GitLabBaseURL) {
		return errors.New("policySrc must point to the configured GitLab instance")
	}

	if strings.Contains(req.PolicyRef, "..") {
		return errors.New("policyRef must not contain path traversal sequences")
	}

	cleaned := path.Clean(req.PolicyPath)
	if strings.HasPrefix(cleaned, "..") {
		return errors.New("policyPath must not escape the repository root")
	}

	return nil
}

// fetchCourseCheckPolicy fetches the Rego policy from the course git repo.
func (h *Handler) fetchCourseCheckPolicy(resp http.ResponseWriter, httpReq *http.Request, req checker.EvaluateRequest) (string, bool) {
	token := ""
	if req.PolicyToken != nil {
		token = *req.PolicyToken
	}

	policy, err := checker.FetchCourseCheckPolicyContent(httpReq.Context(), req.PolicySrc, req.PolicyRef, req.PolicyPath, token)
	if err != nil {
		zap.L().Error("policy fetch", zap.String("src", req.PolicySrc), zap.String("ref", req.PolicyRef), zap.String("path", req.PolicyPath), zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "failed to fetch policy")

		return "", false
	}

	return policy, true
}

// fetchStudentGitLabProjectState fetches the student's GitLab project and MR
// state using the checker-service's GitLab token.
func (h *Handler) fetchStudentGitLabProjectState(resp http.ResponseWriter, req checker.EvaluateRequest) (*checker.GitLabState, bool) {
	fetcher, err := checker.NewFetcher(h.config.GitLabToken, h.config.GitLabBaseURL)
	if err != nil {
		zap.L().Error("gitlab client init", zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "gitlab client error")

		return nil, false
	}

	state, err := fetcher.Fetch(req.Project, req.Files)
	if err != nil {
		zap.L().Error("gitlab fetch", zap.String("project", req.Project), zap.Error(err))
		httpErr(resp, http.StatusInternalServerError, "failed to fetch GitLab state")

		return nil, false
	}

	return state, true
}

// httpErr writes a JSON error response with the given status and message.
func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(map[string]string{"error": msg})
	if err != nil {
		zap.L().Error("encode error response", zap.Error(err))
	}
}
