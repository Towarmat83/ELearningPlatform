package checker

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// maxPolicyBytes is the maximum size of a Rego policy file fetched from git.
const maxPolicyBytes = 128 * 1024 // 128 KB

// gitOAuth2Username is the conventional basic-auth username for OAuth2 tokens.
const gitOAuth2Username = "oauth2"

// FetchCourseCheckPolicyContent fetches a Rego policy file from a git repo.
// src is the git clone URL (e.g. https://gitlab.example.com/group/repo.git),
// ref is the branch/tag, filePath is the relative path to the .rego file,
// and token is the optional OAuth2 access token.
func FetchCourseCheckPolicyContent(ctx context.Context, src, ref, filePath, token string) (string, error) {
	// Reject non-HTTP(S) schemes to prevent SSRF via file://, gopher://, etc.
	parsed, err := url.Parse(src)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid policy source URL: %q", src)
	}

	// Build GitLab raw file URL: strip .git suffix, append /-/raw/ref/path
	base := strings.TrimSuffix(strings.TrimRight(src, "/"), ".git")
	rawURL := base + "/-/raw/" + ref + "/" + strings.TrimLeft(filePath, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build policy request: %w", err)
	}

	if token != "" {
		req.SetBasicAuth(gitOAuth2Username, token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch policy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is non-actionable

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("policy fetch returned HTTP %d for %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPolicyBytes))
	if err != nil {
		return "", fmt.Errorf("read policy body: %w", err)
	}

	return string(data), nil
}

// GitLabFetcher fetches project, merge request, and file state from GitLab
// for policy evaluation.
type GitLabFetcher struct {
	client *gitlab.Client
}

// NewFetcher creates a GitLabFetcher authenticated against the given GitLab
// instance.
func NewFetcher(token, baseURL string) (*GitLabFetcher, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, fmt.Errorf("gitlab new client: %w", err)
	}

	return &GitLabFetcher{client: client}, nil
}

// Fetch gathers project info, merge request state, and file contents for the
// given project from GitLab.
func (f *GitLabFetcher) Fetch(project string, files []string) (*GitLabState, error) {
	state := &GitLabState{Files: make(map[string]string)}

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

// countMergedMRs returns the total number of merged merge requests for the
// project, paginating to ensure an accurate count.
func (f *GitLabFetcher) countMergedMRs(project string) int {
	merged := "merged"
	total := 0
	page := int64(1)

	for {
		mrs, resp, err := f.client.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
			State:       &merged,
			ListOptions: gitlab.ListOptions{PerPage: gitLabPageSize, Page: page},
		})
		if err != nil {
			slog.Warn("list merged MRs failed", "project", project, "err", err)

			break
		}

		total += len(mrs)

		if resp.NextPage == 0 {
			break
		}

		page = resp.NextPage
	}

	return total
}

// fetchOpenMRs returns all open merge requests for the project, enriched with
// pipeline status and commits, paginating to retrieve every result.
func (f *GitLabFetcher) fetchOpenMRs(project string) []openMRInfo {
	openMRs, err := f.listAllOpenMRs(project)
	if err != nil {
		slog.Warn("list open MRs failed", "project", project, "err", err)

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
			slog.Warn("fetch file failed", "project", project, "path", filePath, "err", err)

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
