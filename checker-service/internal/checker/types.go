package checker

// EvaluateRequest is sent by course-service to POST /evaluate.
// The project template has already been resolved ({{ .Username }} replaced).
type EvaluateRequest struct {
	Username string   `json:"username"`
	Project  string   `json:"project"`
	Files    []string `json:"files"`
	Policy   string   `json:"policy"`
}

// EvaluateResponse is returned to course-service.
type EvaluateResponse struct {
	Allow      bool     `json:"allow"`
	Violations []string `json:"violations"`
}

// gitLabState is the JSON input passed to OPA.
type gitLabState struct {
	Project       *projectInfo      `json:"project"`
	MergedMRCount int               `json:"merged_mr_count"`
	OpenMRs       []openMRInfo      `json:"open_mrs"`
	Files         map[string]string `json:"files"`
}

type projectInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path_with_namespace"`
	DefaultBranch string `json:"default_branch"`
}

type openMRInfo struct {
	IID            int64        `json:"iid"`
	Title          string       `json:"title"`
	SourceBranch   string       `json:"source_branch"`
	PipelineStatus string       `json:"pipeline_status"`
	Commits        []commitInfo `json:"commits"`
}

type commitInfo struct {
	Message string `json:"message"`
}
