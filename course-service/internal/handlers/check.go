// Package handlers implements the course-service HTTP API endpoints.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/elearning/course-service/internal/content"
)

// checkSpec is the structure of check.yaml in the course git repo.
// Placed in the same directory as the module content file.
type checkSpec struct {
	Provider string   `yaml:"provider"`
	Project  string   `yaml:"project"` // template, e.g. "e-learning/{{ .Username }}"
	Files    []string `yaml:"files"`
	Policy   string   `yaml:"policy"` // relative path to .rego file
}

// evaluateRequest is the payload sent to checker-service's /evaluate route.
type evaluateRequest struct {
	Username string   `json:"username"`
	Project  string   `json:"project"`
	Files    []string `json:"files"`
	Policy   string   `json:"policy"`
}

// CheckResponse mirrors checker.EvaluateResponse.
type CheckResponse struct {
	Allow      bool     `json:"allow"`
	Violations []string `json:"violations"`
}

// CheckModule runs the OPA policy check for a lab module against the
// learner's GitLab project and stores the result.
// POST /api/courses/{slug}/modules/{index}/check.
func (s *State) CheckModule(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	indexStr := param(req, "index")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	// preferred_username from Keycloak maps to the GitLab username
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Subject
	}
	// display name for traceability: prefer email over opaque UUID
	displayName := claims.Email
	if displayName == "" {
		displayName = username
	}

	mod, idx, success := s.resolveLabModule(writer, req, course, indexStr)
	if !success {
		return
	}

	spec, policyData, success := s.fetchCheckSpec(req.Context(), writer, mod)
	if !success {
		return
	}

	// Resolve username template in project path
	project := strings.ReplaceAll(spec.Project, "{{ .Username }}", username)

	result, success := s.callChecker(writer, req, evaluateRequest{
		Username: username,
		Project:  project,
		Files:    spec.Files,
		Policy:   string(policyData),
	})
	if !success {
		return
	}

	s.storeLabCheck(req.Context(), displayName, courseSlug, idx, mod.Name, result, true)

	s.JSON(writer, http.StatusOK, result)
}

// resolveLabModule looks up the module at indexStr among the modules
// visible to the requester and confirms it is a checkable lab.
func (s *State) resolveLabModule(
	writer http.ResponseWriter, req *http.Request, course *content.Course, indexStr string,
) (content.Module, int, bool) {
	modules := s.visibleModules(course, req)

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(writer, http.StatusNotFound, "Module not found")

		return content.Module{}, 0, false
	}

	mod := modules[idx]

	if mod.Type != moduleTypeLab {
		s.Error(writer, http.StatusBadRequest, "Module is not a lab")

		return content.Module{}, 0, false
	}

	if !mod.HasGitContent() {
		s.Error(writer, http.StatusBadRequest, "Lab module has no git content configured")

		return content.Module{}, 0, false
	}

	return mod, idx, true
}

// fetchCheckSpec fetches and parses check.yaml plus its Rego policy file
// from the same git directory as the module content.
func (s *State) fetchCheckSpec(ctx context.Context, writer http.ResponseWriter, mod content.Module) (checkSpec, []byte, bool) {
	token := s.tokenForRepo(mod.Src)
	checkDir := path.Dir(mod.Path)

	// Fetch check.yaml co-located with module content
	specData, err := s.GitCache.FetchModuleContent(ctx, mod.Src, mod.Ref, path.Join(checkDir, "check.yaml"), token)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "No check.yaml found for this lab")

		return checkSpec{}, nil, false
	}

	var spec checkSpec

	err = yaml.Unmarshal(specData, &spec)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to parse check.yaml")

		return checkSpec{}, nil, false
	}

	// Fetch Rego policy from same directory
	policyData, err := s.GitCache.FetchModuleContent(ctx, mod.Src, mod.Ref, path.Join(checkDir, spec.Policy), token)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "Policy file not found: "+spec.Policy)

		return checkSpec{}, nil, false
	}

	return spec, policyData, true
}

// callChecker sends req to checker-service's /evaluate route and returns
// the decoded response, writing an HTTP error and returning ok=false on
// any failure.
func (s *State) callChecker(writer http.ResponseWriter, req *http.Request, body evaluateRequest) (CheckResponse, bool) {
	return s.callCheckerRoute(writer, req, "/evaluate", body)
}

// storeLabCheck persists a lab check result for later review, logging a
// warning on failure without interrupting the response to the client.
func (s *State) storeLabCheck(
	ctx context.Context, username, courseSlug string, moduleIndex int, moduleName string, result CheckResponse, verified bool,
) {
	if s.DB == nil {
		return
	}

	violations := result.Violations
	if violations == nil {
		violations = []string{}
	}

	_, err := s.DB.Exec(ctx,
		`INSERT INTO lab_checks (username, courseSlug, moduleIndex, moduleName, allow, violations, verified)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		username, courseSlug, moduleIndex, moduleName, result.Allow, violations, verified,
	)
	if err != nil {
		slog.Warn("failed to store lab check", "err", err)
	}
}

// stepCheckRequest is sent to checker-service's /check-step route.
type stepCheckRequest struct {
	Username    string         `json:"username"`
	Project     string         `json:"project"`
	CheckType   string         `json:"checkType"`
	CheckParams map[string]any `json:"checkParams"`
}

// CheckModuleStep runs a single named step check for a lab module.
// POST /api/courses/{slug}/modules/{index}/steps/{stepIndex}/check.
func (s *State) CheckModuleStep(writer http.ResponseWriter, req *http.Request) { //nolint:gocyclo,cyclop,funlen // single-responsibility handler with multiple early-exit guard clauses
	courseSlug := param(req, "slug")
	indexStr := param(req, "index")
	stepIndexStr := param(req, "stepIndex")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Subject
	}

	mod, _, success := s.resolveLabModule(writer, req, course, indexStr)
	if !success {
		return
	}

	if mod.CheckProvider != checkProviderGitLab || len(mod.Steps) == 0 {
		s.Error(writer, http.StatusBadRequest, "Module does not have GitLab step checks")

		return
	}

	stepIdx, err := strconv.Atoi(stepIndexStr)
	if err != nil || stepIdx < 0 || stepIdx >= len(mod.Steps) {
		s.Error(writer, http.StatusNotFound, "Step not found")

		return
	}

	step := mod.Steps[stepIdx]

	// Resolve {{ .Username }} template in every string value of checkParams.
	resolved := make(map[string]any, len(step.CheckParams))
	for k, v := range step.CheckParams {
		if sv, ok := v.(string); ok {
			resolved[k] = strings.ReplaceAll(sv, "{{ .Username }}", username)
		} else {
			resolved[k] = v
		}
	}

	// Derive project from checkParams if present; otherwise skip.
	project, ok := resolved["project"].(string)
	if !ok || project == "" {
		s.Error(writer, http.StatusBadRequest, "Step checkParams missing project")

		return
	}

	result, success := s.callCheckerStep(writer, req, stepCheckRequest{
		Username:    username,
		Project:     project,
		CheckType:   step.CheckType,
		CheckParams: resolved,
	})
	if !success {
		return
	}

	s.JSON(writer, http.StatusOK, result)
}

// callCheckerStep sends a step check request to checker-service's /check-step
// route and returns the decoded response.
func (s *State) callCheckerStep(writer http.ResponseWriter, req *http.Request, body stepCheckRequest) (CheckResponse, bool) {
	return s.callCheckerRoute(writer, req, "/check-step", body)
}

// callCheckerRoute marshals body, POSTs it to checker-service at the given
// path, and decodes the CheckResponse. Writes an HTTP error and returns
// ok=false on any failure.
func (s *State) callCheckerRoute(writer http.ResponseWriter, req *http.Request, route string, body any) (CheckResponse, bool) {
	payload, err := json.Marshal(body)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to build checker request")

		return CheckResponse{}, false
	}

	httpReq, err := http.NewRequestWithContext(
		req.Context(), http.MethodPost, s.Config.CheckerServiceURL+route, bytes.NewReader(payload),
	)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to build checker request")

		return CheckResponse{}, false
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		slog.Error("checker-service call failed", "err", err)
		s.Error(writer, http.StatusInternalServerError, "error when reaching for checker service")

		return CheckResponse{}, false
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]string

		_ = json.NewDecoder(resp.Body).Decode(&errBody)

		msg := errBody[errorJSONKey]
		if msg == "" {
			msg = "checker-service error"
		}

		s.Error(writer, http.StatusBadGateway, msg)

		return CheckResponse{}, false
	}

	var result CheckResponse

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to decode checker response")

		return CheckResponse{}, false
	}

	return result, true
}

// RecordLocalCheck persists the result of a Tauri-side local check
// (podman, etc.) without re-running any verification server-side.
// POST /api/courses/{slug}/modules/{index}/record.
func (s *State) RecordLocalCheck(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	indexStr := param(req, "index")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(course.Modules) {
		s.Error(writer, http.StatusBadRequest, "Invalid module index")

		return
	}

	mod := course.Modules[idx]

	var result CheckResponse

	err = json.NewDecoder(req.Body).Decode(&result)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid body")

		return
	}

	displayName := claims.Email
	if displayName == "" {
		displayName = "unknown"
	}

	s.storeLabCheck(req.Context(), displayName, courseSlug, idx, mod.Name, result, false)
	s.JSON(writer, http.StatusOK, result)
}
