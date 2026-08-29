package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/course-service/internal/content"
	"github.com/genesary/pupitre/course-service/internal/httpx"
)

// courseProgress is one learner's state in one course, as course-service
// needs it while rendering: which lessons are viewed, what each quiz module
// scored, and the aggregates used to evaluate prerequisites.
//
// It is fetched once per request from a single user-service endpoint. The
// handlers used to assemble the same information from four separate calls
// (enrollment check, viewed lessons, module progress, course summary), some
// of them issued more than once while rendering one page.
type courseProgress struct {
	Enrolled      bool
	Viewed        map[string]bool
	Modules       map[int]moduleProgressData
	TotalScore    int
	PassedModules map[string]bool
	ViewedCount   int
}

// emptyCourseProgress is what every failed lookup returns, so callers can
// treat an unreachable user-service as "no progress recorded" without
// special-casing nil maps.
func emptyCourseProgress() *courseProgress {
	return &courseProgress{
		Viewed:        map[string]bool{},
		Modules:       map[int]moduleProgressData{},
		PassedModules: map[string]bool{},
	}
}

// progressOverviewBody is the wire shape of GET /internal/progress/overview.
type progressOverviewBody struct {
	Enrolled bool     `json:"enrolled"`
	Viewed   []string `json:"viewed"`
	Modules  []struct {
		ModuleIndex int  `json:"moduleIndex"`
		BestScore   int  `json:"bestScore"`
		MaxScore    int  `json:"maxScore"`
		Passed      bool `json:"passed"`
		Attempts    int  `json:"attempts"`
	} `json:"modules"`
	TotalScore    int      `json:"totalScore"`
	PassedModules []string `json:"passedModules"`
	ViewedCount   int      `json:"viewedCount"`
}

// courseSummariesBody is the wire shape of
// GET /internal/progress/course-summaries.
type courseSummariesBody struct {
	Summaries map[string]struct {
		TotalScore    int      `json:"totalScore"`
		PassedModules []string `json:"passedModules"`
		ViewedCount   int      `json:"viewedCount"`
	} `json:"summaries"`
}

// internalURL builds a user-service internal URL with the given query
// parameters.
func (s *State) internalURL(path string, params map[string]string) (string, error) {
	target, err := url.Parse(s.Config.UserServiceURL + path)
	if err != nil {
		return "", fmt.Errorf("build user-service URL %s: %w", path, err)
	}

	query := target.Query()
	for key, value := range params {
		query.Set(key, value)
	}

	target.RawQuery = query.Encode()

	return target.String(), nil
}

// getInternalJSON performs an authenticated GET against a user-service
// internal endpoint and decodes the response into dest.
func (s *State) getInternalJSON(ctx context.Context, path string, params map[string]string, dest any) error {
	target, err := s.internalURL(path, params)
	if err != nil {
		return err
	}

	err = httpx.GetJSON(ctx, target, s.setInternalHeader, dest)
	if err != nil {
		return fmt.Errorf("user-service %s: %w", path, err)
	}

	return nil
}

// courseProgress fetches userID's full progress in courseSlug in one call.
//
// A failure is reported as empty progress rather than an error: every
// caller renders the course either way, and treating a user-service blip as
// "nothing completed yet" is what the per-call helpers did before.
func (s *State) courseProgress(ctx context.Context, courseSlug, userID string) *courseProgress {
	var body progressOverviewBody

	err := s.getInternalJSON(ctx, "/internal/progress/overview",
		map[string]string{userIDJSONKey: userID, courseSlugJSONKey: courseSlug}, &body)
	if err != nil {
		zap.L().Warn("failed to load progress overview",
			zap.String("courseSlug", courseSlug), zap.Error(err))

		return emptyCourseProgress()
	}

	progress := &courseProgress{
		Enrolled:      body.Enrolled,
		Viewed:        make(map[string]bool, len(body.Viewed)),
		Modules:       make(map[int]moduleProgressData, len(body.Modules)),
		TotalScore:    body.TotalScore,
		PassedModules: make(map[string]bool, len(body.PassedModules)),
		ViewedCount:   body.ViewedCount,
	}

	for _, slug := range body.Viewed {
		progress.Viewed[slug] = true
	}

	for _, row := range body.Modules {
		progress.Modules[row.ModuleIndex] = moduleProgressData{
			BestScore: row.BestScore,
			MaxScore:  row.MaxScore,
			Passed:    row.Passed,
			Attempts:  row.Attempts,
		}
	}

	for _, slug := range body.PassedModules {
		progress.PassedModules[slug] = true
	}

	return progress
}

// courseSummaries fetches the prerequisite-relevant aggregates for every
// course in slugs in one call, keyed by slug. A course the response does
// not mention maps to an empty summary.
func (s *State) courseSummaries(ctx context.Context, userID string, slugs []string) map[string]coursePrereqSummary {
	out := make(map[string]coursePrereqSummary, len(slugs))
	if len(slugs) == 0 {
		return out
	}

	var body courseSummariesBody

	err := s.getInternalJSON(ctx, "/internal/progress/course-summaries",
		map[string]string{userIDJSONKey: userID, "courseSlugs": strings.Join(slugs, ",")}, &body)
	if err != nil {
		zap.L().Warn("failed to load course summaries", zap.Error(err))

		for _, slug := range slugs {
			out[slug] = emptyCoursePrereqSummary()
		}

		return out
	}

	for _, slug := range slugs {
		row, found := body.Summaries[slug]
		if !found {
			out[slug] = emptyCoursePrereqSummary()

			continue
		}

		summary := coursePrereqSummary{
			TotalScore:    row.TotalScore,
			ViewedCount:   row.ViewedCount,
			PassedModules: make(map[string]bool, len(row.PassedModules)),
		}
		for _, moduleSlug := range row.PassedModules {
			summary.PassedModules[moduleSlug] = true
		}

		out[slug] = summary
	}

	return out
}

// pathEnrollmentCheck reports whether userID is enrolled in any of
// pathSlugs.
func (s *State) pathEnrollmentCheck(ctx context.Context, userID string, pathSlugs []string) bool {
	if len(pathSlugs) == 0 {
		return false
	}

	var result pathCheckResponse

	err := s.getInternalJSON(ctx, "/internal/paths/check",
		map[string]string{userIDJSONKey: userID, "pathSlugs": strings.Join(pathSlugs, ",")}, &result)
	if err != nil {
		zap.L().Warn("failed to check path enrollment", zap.Error(err))

		return false
	}

	return result.Enrolled
}

// enrollmentCheck reports whether userID is enrolled in courseSlug. Prefer
// courseProgress when the caller also needs progress: it answers both.
func (s *State) enrollmentCheck(ctx context.Context, courseSlug, userID string) bool {
	var result struct {
		Enrolled bool `json:"enrolled"`
	}

	err := s.getInternalJSON(ctx, "/internal/enrollments/check",
		map[string]string{userIDJSONKey: userID, courseSlugJSONKey: courseSlug}, &result)
	if err != nil {
		zap.L().Warn("failed to check enrollment", zap.String("courseSlug", courseSlug), zap.Error(err))

		return false
	}

	return result.Enrolled
}

// postInternalTimeout bounds a detached write to user-service, so a slow or
// unreachable dependency cannot leak goroutines indefinitely.
const postInternalTimeout = 10 * time.Second

// postInternal sends payload as JSON to a user-service internal endpoint
// and reports whether it was accepted. The response body is drained so the
// connection returns to the keep-alive pool.
func (s *State) postInternal(ctx context.Context, path string, payload any) error {
	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(payload)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.UserServiceURL+path, &buf)
	if err != nil {
		return fmt.Errorf("build POST %s: %w", path, err)
	}

	req.Header.Set("Content-Type", "application/json")
	s.setInternalHeader(req)

	resp, err := httpx.Do(req) //nolint:bodyclose // httpx.Drain closes it
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer httpx.Drain(resp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("user-service %s returned %d", path, resp.StatusCode)
	}

	return nil
}

// postInternalDetached sends payload to a user-service internal endpoint
// from a goroutine detached from the request lifecycle, on its own bounded
// context. Used for writes whose result cannot change the response that has
// already been (or is about to be) sent.
func (s *State) postInternalDetached(path string, payload any, fields ...zap.Field) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), postInternalTimeout)
		defer cancel()

		err := s.postInternal(ctx, path, payload)
		if err != nil {
			zap.L().Warn("detached internal call failed",
				append(fields, zap.String("path", path), zap.Error(err))...)
		}
	}()
}

// skillTotalsFor counts how many courses teach each of a course's skills,
// returning nil when the course teaches none.
func (s *State) skillTotalsFor(ctx context.Context, course *content.Course) map[string]int {
	if len(course.Skills) == 0 {
		return nil
	}

	totals, err := s.Repos.Courses.SkillTotals(ctx, course.Skills)
	if err != nil {
		zap.L().Error("count skill totals failed", zap.String("courseSlug", course.Slug), zap.Error(err))

		return nil
	}

	return totals
}
