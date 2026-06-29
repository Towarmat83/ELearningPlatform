package checker

import (
	"encoding/base64"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type GitLabFetcher struct {
	client *gitlab.Client
}

func NewFetcher(token, baseURL string) (*GitLabFetcher, error) {
	git, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, err
	}
	return &GitLabFetcher{client: git}, nil
}

func (f *GitLabFetcher) Fetch(project string, files []string) (*gitLabState, error) {
	state := &gitLabState{Files: make(map[string]string)}

	// Project info
	proj, _, err := f.client.Projects.GetProject(project, &gitlab.GetProjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("GetProject %q: %w", project, err)
	}
	state.Project = &projectInfo{
		ID:            fmt.Sprint(proj.ID),
		Name:          proj.Name,
		Path:          proj.PathWithNamespace,
		DefaultBranch: proj.DefaultBranch,
	}

	// Merged MRs count
	merged := "merged"
	mergedMRs, _, err := f.client.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
		State: &merged,
	})
	if err == nil {
		state.MergedMRCount = len(mergedMRs)
	}

	// Open MRs with pipeline status and commits
	opened := "opened"
	openMRs, _, err := f.client.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
		State: &opened,
	})
	if err == nil {
		state.OpenMRs = make([]openMRInfo, 0, len(openMRs))
		for _, mr := range openMRs {
			info := openMRInfo{
				IID:          mr.IID,
				Title:        mr.Title,
				SourceBranch: mr.SourceBranch,
			}
			// ListProjectMergeRequests does not populate HeadPipeline; fetch individually
			fullMR, _, merr := f.client.MergeRequests.GetMergeRequest(project, mr.IID, &gitlab.GetMergeRequestsOptions{})
			if merr == nil && fullMR.HeadPipeline != nil {
				info.PipelineStatus = fullMR.HeadPipeline.Status
			}
			commits, _, cerr := f.client.MergeRequests.GetMergeRequestCommits(project, mr.IID, nil)
			if cerr == nil {
				for _, c := range commits {
					info.Commits = append(info.Commits, commitInfo{Message: c.Message})
				}
			}
			state.OpenMRs = append(state.OpenMRs, info)
		}
	}

	// File contents — check on the source branch of the first open MR (if any), else default branch
	ref := proj.DefaultBranch
	if len(state.OpenMRs) > 0 {
		ref = state.OpenMRs[0].SourceBranch
	}
	for _, filePath := range files {
		file, _, err := f.client.RepositoryFiles.GetFile(project, filePath, &gitlab.GetFileOptions{
			Ref: gitlab.Ptr(ref),
		})
		if err != nil {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			decoded = []byte(file.Content)
		}
		state.Files[filePath] = string(decoded)
	}

	return state, nil
}
