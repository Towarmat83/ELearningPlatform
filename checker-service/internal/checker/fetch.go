package checker

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// GitLabFetcher fetches project, merge request, and file state from GitLab
// for policy evaluation.
type GitLabFetcher struct {
	client *gitlab.Client
}

// redirectRewriteTransport rewrites redirect URLs that point to the external
// GitLab host back to the internal cluster host, so that 301 redirects from
// GitLab (e.g. gitlab.local:8880 → path-based project lookup) are followed
// correctly from inside the cluster.
type redirectRewriteTransport struct {
	externalHost string // e.g. "gitlab.local:8880"
	internalHost string // e.g. "gitlab-http.gitlab.svc.cluster.local"
	base         http.RoundTripper
}

// RoundTrip rewrites the request host when it matches the external GitLab
// hostname, then delegates to the base transport.
func (t *redirectRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.externalHost {
		cloned := req.Clone(req.Context())
		cloned.URL.Host = t.internalHost
		req = cloned
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}

	return resp, nil
}

// NewFetcher creates a GitLabFetcher authenticated against the given GitLab
// instance. It installs a redirect-rewriting transport so that GitLab's 301
// redirects (which use its external hostname) are resolved via the internal
// cluster service instead.
func NewFetcher(token, baseURL string) (*GitLabFetcher, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	transport := &redirectRewriteTransport{
		externalHost: "gitlab.local:8880",
		internalHost: parsedURL.Host,
		base:         http.DefaultTransport,
	}

	httpClient := &http.Client{Transport: transport}

	client, err := gitlab.NewClient(token,
		gitlab.WithBaseURL(baseURL+"/api/v4"),
		gitlab.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab new client: %w", err)
	}

	return &GitLabFetcher{client: client}, nil
}

// Fetch gathers project info, merge request state, and file contents for the
// given project from GitLab.
func (f *GitLabFetcher) Fetch(project string, files []string) (*gitLabState, error) {
	state := &gitLabState{Files: make(map[string]string)}

	proj, _, err := f.client.Projects.GetProject(project, &gitlab.GetProjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("GetProject %q: %w", project, err)
	}

	state.Project = &projectInfo{
		ID:            strconv.FormatInt(proj.ID, 10),
		Name:          proj.Name,
		Path:          proj.PathWithNamespace,
		DefaultBranch: proj.DefaultBranch,
	}

	state.MergedMRCount = f.countMergedMRs(project)
	state.OpenMRs = f.fetchOpenMRs(project)

	// File contents — check on the source branch of the first open MR (if any), else default branch
	ref := proj.DefaultBranch
	if len(state.OpenMRs) > 0 {
		ref = state.OpenMRs[0].SourceBranch
	}

	state.Files = f.fetchFiles(project, ref, files)

	return state, nil
}

// countMergedMRs returns the number of merged merge requests for the project.
func (f *GitLabFetcher) countMergedMRs(project string) int {
	merged := "merged"

	mergedMRs, _, err := f.client.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
		State: &merged,
	})
	if err != nil {
		return 0
	}

	return len(mergedMRs)
}

// fetchOpenMRs returns open merge requests for the project, enriched with
// pipeline status and commits.
func (f *GitLabFetcher) fetchOpenMRs(project string) []openMRInfo {
	openMRs, _, err := f.client.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
		State: mrOpened(),
	})
	if err != nil {
		return nil
	}

	infos := make([]openMRInfo, 0, len(openMRs))
	for _, mergeRequest := range openMRs {
		infos = append(infos, f.buildOpenMRInfo(project, mergeRequest))
	}

	return infos
}

// buildOpenMRInfo enriches a merge request with pipeline status and commits.
func (f *GitLabFetcher) buildOpenMRInfo(project string, mergeRequest *gitlab.BasicMergeRequest) openMRInfo {
	info := openMRInfo{
		IID:          mergeRequest.IID,
		Title:        mergeRequest.Title,
		SourceBranch: mergeRequest.SourceBranch,
	}

	// ListProjectMergeRequests does not populate HeadPipeline; fetch individually
	fullMR, _, err := f.client.MergeRequests.GetMergeRequest(project, mergeRequest.IID, &gitlab.GetMergeRequestsOptions{})
	if err == nil && fullMR.HeadPipeline != nil {
		info.PipelineStatus = fullMR.HeadPipeline.Status
	}

	commits, _, err := f.client.MergeRequests.GetMergeRequestCommits(project, mergeRequest.IID, nil)
	if err == nil {
		for _, commit := range commits {
			info.Commits = append(info.Commits, commitInfo{Message: commit.Message})
		}
	}

	return info
}

// fetchFiles reads the given file paths from the given ref, skipping any
// that fail to fetch or decode.
func (f *GitLabFetcher) fetchFiles(project, ref string, files []string) map[string]string {
	contents := make(map[string]string, len(files))

	for _, filePath := range files {
		file, _, err := f.client.RepositoryFiles.GetFile(project, filePath, &gitlab.GetFileOptions{
			Ref: &ref,
		})
		if err != nil {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			decoded = []byte(file.Content)
		}

		contents[filePath] = string(decoded)
	}

	return contents
}
