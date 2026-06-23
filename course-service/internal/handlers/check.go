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
)

// checkSpec is the structure of check.yaml in the course git repo.
// Placed in the same directory as the module content file.
type checkSpec struct {
	Provider string   `yaml:"provider"`
	Project  string   `yaml:"project"` // template, e.g. "e-learning/{{ .Username }}"
	Files    []string `yaml:"files"`
	Policy   string   `yaml:"policy"` // relative path to .rego file
}

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

func (s *State) CheckModule(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	indexStr := param(r, "index")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
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

	modules := s.visibleModules(c, r)
	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(w, http.StatusNotFound, "Module not found")
		return
	}
	m := modules[idx]

	if m.Type != "lab" {
		s.Error(w, http.StatusBadRequest, "Module is not a lab")
		return
	}
	if !m.HasGitContent() {
		s.Error(w, http.StatusBadRequest, "Lab module has no git content configured")
		return
	}

	token := s.tokenForRepo(m.Src)
	checkDir := path.Dir(m.Path)

	// Fetch check.yaml co-located with module content
	specData, err := s.GitCache.FetchModuleContent(m.Src, m.Ref, path.Join(checkDir, "check.yaml"), token)
	if err != nil {
		s.Error(w, http.StatusNotFound, "No check.yaml found for this lab")
		return
	}

	var spec checkSpec
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to parse check.yaml")
		return
	}

	// Fetch Rego policy from same directory
	policyData, err := s.GitCache.FetchModuleContent(m.Src, m.Ref, path.Join(checkDir, spec.Policy), token)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Policy file not found: "+spec.Policy)
		return
	}

	// Resolve username template in project path
	project := strings.ReplaceAll(spec.Project, "{{ .Username }}", username)

	// Call checker-service
	payload, _ := json.Marshal(evaluateRequest{
		Username: username,
		Project:  project,
		Files:    spec.Files,
		Policy:   string(policyData),
	})

	resp, err := http.Post(
		s.Config.CheckerServiceURL+"/evaluate",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		s.Error(w, http.StatusBadGateway, "checker-service unavailable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody) //nolint:errcheck
		msg := errBody["error"]
		if msg == "" {
			msg = "checker-service error"
		}
		s.Error(w, http.StatusBadGateway, msg)
		return
	}

	var result CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to decode checker response")
		return
	}

	s.storeLabCheck(r.Context(), displayName, courseSlug, idx, m.Name, result)

	s.JSON(w, http.StatusOK, result)
}

func (s *State) storeLabCheck(ctx context.Context, username, courseSlug string, moduleIndex int, moduleName string, result CheckResponse) {
	if s.DB == nil {
		return
	}
	violations := result.Violations
	if violations == nil {
		violations = []string{}
	}
	_, err := s.DB.Exec(ctx,
		`INSERT INTO lab_checks (username, course_slug, module_index, module_name, allow, violations)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		username, courseSlug, moduleIndex, moduleName, result.Allow, violations,
	)
	if err != nil {
		slog.Warn("failed to store lab check", "err", err)
	}
}
