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

	mod, idx, success := s.resolveLabModule(writer, req, course, indexStr) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param
	if !success {
		return
	}

	spec, policyData, success := s.fetchCheckSpec(writer, mod) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param
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

	s.storeLabCheck(req.Context(), displayName, courseSlug, idx, mod.Name, result)

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
func (s *State) fetchCheckSpec(writer http.ResponseWriter, mod content.Module) (checkSpec, []byte, bool) {
	token := s.tokenForRepo(mod.Src)
	checkDir := path.Dir(mod.Path)

	// Fetch check.yaml co-located with module content
	specData, err := s.GitCache.FetchModuleContent(mod.Src, mod.Ref, path.Join(checkDir, "check.yaml"), token)
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
	policyData, err := s.GitCache.FetchModuleContent(mod.Src, mod.Ref, path.Join(checkDir, spec.Policy), token)
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
	payload, err := json.Marshal(body)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to build checker request")

		return CheckResponse{}, false
	}

	httpReq, err := http.NewRequestWithContext(
		req.Context(), http.MethodPost, s.Config.CheckerServiceURL+"/evaluate", bytes.NewReader(payload),
	)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to build checker request")

		return CheckResponse{}, false
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		s.Error(writer, http.StatusBadGateway, "checker-service unavailable: "+err.Error())

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

// storeLabCheck persists a lab check result for later review, logging a
// warning on failure without interrupting the response to the client.
func (s *State) storeLabCheck(
	ctx context.Context, username, courseSlug string, moduleIndex int, moduleName string, result CheckResponse,
) {
	if s.DB == nil {
		return
	}

	violations := result.Violations
	if violations == nil {
		violations = []string{}
	}

	_, err := s.DB.Exec(ctx,
		`INSERT INTO lab_checks (username, courseSlug, moduleIndex, moduleName, allow, violations)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		username, courseSlug, moduleIndex, moduleName, result.Allow, violations,
	)
	if err != nil {
		slog.Warn("failed to store lab check", "err", err)
	}
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

	s.storeLabCheck(req.Context(), displayName, courseSlug, idx, mod.Name, result)
	s.JSON(writer, http.StatusOK, result)
}
