package checker

import (
	"fmt"
	"regexp"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Step check type identifiers.
const (
	StepGitLabBranch                     = "gitlab_branch"
	StepGitLabMROpen                     = "gitlab_mr_open"
	StepGitLabConventionalCommit         = "gitlab_conventional_commit"
	StepGitLabPipelinePassed             = "gitlab_pipeline_passed"
	StepGitLabCommitOnBranch             = "gitlab_commit_on_branch"
	StepGitLabFileOnBranch               = "gitlab_file_on_branch"
	StepGitLabConventionalCommitOnBranch = "gitlab_conventional_commit_on_branch"
)

// mrStateOpened is the GitLab MR state filter for open merge requests.
const mrStateOpened = "opened"

// defaultBaseBranch is used when no base branch is specified in checkParams.
const defaultBaseBranch = "main"

// noOpenMRMessage is returned when a check requires an open MR but none exists.
const noOpenMRMessage = "Aucune MR ouverte. Ouvrez d'abord une Merge Request."

// commitMessageParts is the limit passed to SplitN when extracting the first
// line of a commit message.
const commitMessageParts = 2

// conventionalCommitRe matches the first line of a conventional commit message.
var conventionalCommitRe = regexp.MustCompile(
	`^(feat|fix|chore|docs|style|refactor|test|perf|ci|build|revert)(\([^)]+\))?: .+`,
)

// StepRequest is sent to POST /check-step.
type StepRequest struct {
	Username    string         `json:"username"`
	Project     string         `json:"project"`
	CheckType   string         `json:"checkType"`
	CheckParams map[string]any `json:"checkParams"`
}

// StepResponse is returned by POST /check-step.
type StepResponse = EvaluateResponse

// CheckStep dispatches to the appropriate named step check.
func (f *GitLabFetcher) CheckStep(req StepRequest) (*StepResponse, error) {
	switch req.CheckType {
	case StepGitLabBranch:
		return f.checkBranch(req)
	case StepGitLabMROpen:
		return f.checkMROpen(req)
	case StepGitLabConventionalCommit:
		return f.checkConventionalCommit(req)
	case StepGitLabPipelinePassed:
		return f.checkPipelinePassed(req)
	case StepGitLabCommitOnBranch:
		return f.checkCommitOnBranch(req)
	case StepGitLabFileOnBranch:
		return f.checkFileOnBranch(req)
	case StepGitLabConventionalCommitOnBranch:
		return f.checkConventionalCommitOnBranch(req)
	default:
		return &StepResponse{
			Allow:      false,
			Violations: []string{"Type de vérification inconnu : " + req.CheckType},
		}, nil
	}
}

// strParam reads a string value from params.
func strParam(params map[string]any, key string) string {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

// mrOpened returns a pointer to the "opened" state string for GitLab API calls.
func mrOpened() *string {
	s := mrStateOpened

	return &s
}

// checkBranch verifies that at least one branch matching pattern exists.
func (f *GitLabFetcher) checkBranch(req StepRequest) (*StepResponse, error) {
	pattern := strParam(req.CheckParams, "pattern")

	branches, _, err := f.client.Branches.ListBranches(req.Project, nil)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	for _, b := range branches {
		if strings.HasPrefix(b.Name, pattern) {
			return &StepResponse{Allow: true}, nil
		}
	}

	return &StepResponse{
		Allow: false,
		Violations: []string{fmt.Sprintf(
			"Aucune branche commençant par %q trouvée. Créez une branche de type feature/ et poussez-la.",
			pattern,
		)},
	}, nil
}

// checkMROpen verifies that at least one MR is open in the project.
func (f *GitLabFetcher) checkMROpen(req StepRequest) (*StepResponse, error) {
	mrs, _, err := f.client.MergeRequests.ListProjectMergeRequests(req.Project, &gitlab.ListProjectMergeRequestsOptions{
		State: mrOpened(),
	})
	if err != nil {
		return nil, fmt.Errorf("list MRs: %w", err)
	}

	if len(mrs) == 0 {
		return &StepResponse{
			Allow:      false,
			Violations: []string{"Aucune Merge Request ouverte trouvée. Créez une MR sans la fusionner."},
		}, nil
	}

	return &StepResponse{Allow: true}, nil
}

// checkConventionalCommit verifies that at least one commit on an open MR
// follows conventional format.
func (f *GitLabFetcher) checkConventionalCommit(req StepRequest) (*StepResponse, error) {
	mrs, _, err := f.client.MergeRequests.ListProjectMergeRequests(req.Project, &gitlab.ListProjectMergeRequestsOptions{
		State: mrOpened(),
	})
	if err != nil {
		return nil, fmt.Errorf("list MRs: %w", err)
	}

	if len(mrs) == 0 {
		return &StepResponse{
			Allow:      false,
			Violations: []string{noOpenMRMessage},
		}, nil
	}

	for _, mr := range mrs {
		commits, _, err := f.client.MergeRequests.GetMergeRequestCommits(req.Project, mr.IID, nil)
		if err != nil {
			continue
		}

		for _, commit := range commits {
			firstLine := strings.SplitN(commit.Message, "\n", commitMessageParts)[0]
			if conventionalCommitRe.MatchString(firstLine) {
				return &StepResponse{Allow: true}, nil
			}
		}
	}

	return &StepResponse{
		Allow:      false,
		Violations: []string{"Aucun commit conventionnel trouvé sur la MR. Format attendu : feat: description, fix: description, etc."},
	}, nil
}

// checkPipelinePassed verifies that the pipeline on the open MR passed.
func (f *GitLabFetcher) checkPipelinePassed(req StepRequest) (*StepResponse, error) {
	mrs, _, err := f.client.MergeRequests.ListProjectMergeRequests(req.Project, &gitlab.ListProjectMergeRequestsOptions{
		State: mrOpened(),
	})
	if err != nil {
		return nil, fmt.Errorf("list MRs: %w", err)
	}

	if len(mrs) == 0 {
		return &StepResponse{
			Allow:      false,
			Violations: []string{noOpenMRMessage},
		}, nil
	}

	for _, mr := range mrs {
		fullMR, _, err := f.client.MergeRequests.GetMergeRequest(req.Project, mr.IID, nil)
		if err != nil {
			continue
		}

		if fullMR.HeadPipeline == nil {
			return &StepResponse{
				Allow:      false,
				Violations: []string{"Aucune pipeline trouvée sur la MR. Poussez un commit pour déclencher la CI."},
			}, nil
		}

		if fullMR.HeadPipeline.Status == "success" {
			return &StepResponse{Allow: true}, nil
		}

		return &StepResponse{
			Allow:      false,
			Violations: []string{fmt.Sprintf("Pipeline non validée (statut : %s). Vérifiez les logs CI.", fullMR.HeadPipeline.Status)},
		}, nil
	}

	return &StepResponse{
		Allow:      false,
		Violations: []string{"Impossible de lire le statut de la pipeline."},
	}, nil
}

// checkCommitOnBranch verifies that a branch matching pattern has at least
// one commit.
func (f *GitLabFetcher) checkCommitOnBranch(req StepRequest) (*StepResponse, error) {
	pattern := strParam(req.CheckParams, "pattern")
	if pattern == "" {
		pattern = "feature/"
	}

	branches, _, err := f.client.Branches.ListBranches(req.Project, nil)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	for _, b := range branches {
		if strings.HasPrefix(b.Name, pattern) && b.Commit != nil {
			return &StepResponse{Allow: true}, nil
		}
	}

	return &StepResponse{
		Allow: false,
		Violations: []string{fmt.Sprintf(
			"Aucune branche %s* avec commit trouvée. Créez une branche, faites un commit et poussez.",
			pattern,
		)},
	}, nil
}

// findBranchByPattern returns the first branch whose name starts with pattern.
func (f *GitLabFetcher) findBranchByPattern(project, pattern string) (string, error) {
	branches, _, err := f.client.Branches.ListBranches(project, nil)
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}

	for _, b := range branches {
		if strings.HasPrefix(b.Name, pattern) {
			return b.Name, nil
		}
	}

	return "", nil
}

// checkFileOnBranch verifies that a specific file was modified on a branch
// vs the base branch.
// checkParams: pattern (branch prefix), file (path), base (default: "main").
func (f *GitLabFetcher) checkFileOnBranch(req StepRequest) (*StepResponse, error) {
	pattern := strParam(req.CheckParams, "pattern")
	file := strParam(req.CheckParams, "file")

	base := strParam(req.CheckParams, "base")
	if base == "" {
		base = defaultBaseBranch
	}

	branch, err := f.findBranchByPattern(req.Project, pattern)
	if err != nil {
		return nil, err
	}

	if branch == "" {
		return &StepResponse{
			Allow:      false,
			Violations: []string{fmt.Sprintf("Aucune branche commençant par %q trouvée. Créez et poussez votre branche d'abord.", pattern)},
		}, nil
	}

	cmp, _, err := f.client.Repositories.Compare(req.Project, &gitlab.CompareOptions{
		From: &base,
		To:   &branch,
	})
	if err != nil {
		return nil, fmt.Errorf("compare branches: %w", err)
	}

	for _, diff := range cmp.Diffs {
		if diff.NewPath == file || diff.OldPath == file {
			return &StepResponse{Allow: true}, nil
		}
	}

	return &StepResponse{
		Allow:      false,
		Violations: []string{fmt.Sprintf("Le fichier %q n'a pas été modifié sur la branche. Ouvrez lab.py, modifiez le message et commitez.", file)},
	}, nil
}

// checkConventionalCommitOnBranch verifies that all commits on a branch
// (vs base) follow conventional format.
// checkParams: pattern (branch prefix), base (default: "main").
func (f *GitLabFetcher) checkConventionalCommitOnBranch(req StepRequest) (*StepResponse, error) {
	pattern := strParam(req.CheckParams, "pattern")

	base := strParam(req.CheckParams, "base")
	if base == "" {
		base = defaultBaseBranch
	}

	branch, err := f.findBranchByPattern(req.Project, pattern)
	if err != nil {
		return nil, err
	}

	if branch == "" {
		return &StepResponse{
			Allow:      false,
			Violations: []string{fmt.Sprintf("Aucune branche commençant par %q trouvée. Créez et poussez votre branche d'abord.", pattern)},
		}, nil
	}

	cmp, _, err := f.client.Repositories.Compare(req.Project, &gitlab.CompareOptions{
		From: &base,
		To:   &branch,
	})
	if err != nil {
		return nil, fmt.Errorf("compare branches: %w", err)
	}

	if len(cmp.Commits) == 0 {
		return &StepResponse{
			Allow:      false,
			Violations: []string{"Aucun commit trouvé sur la branche. Faites un commit et poussez-le."},
		}, nil
	}

	for _, commit := range cmp.Commits {
		firstLine := strings.SplitN(commit.Message, "\n", commitMessageParts)[0]
		if !conventionalCommitRe.MatchString(firstLine) {
			return &StepResponse{
				Allow:      false,
				Violations: []string{fmt.Sprintf("Commit non conventionnel : %q. Format attendu : feat: description, fix: description, etc.", firstLine)},
			}, nil
		}
	}

	return &StepResponse{Allow: true}, nil
}
