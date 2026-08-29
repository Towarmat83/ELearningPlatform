package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// summariesJSONKey is the JSON key carrying the per-course summary map.
const summariesJSONKey = "summaries"

// maxSummaryCourseSlugs caps how many courses one course-summaries request
// may ask about. A course's prerequisite list is a handful of entries, so
// this only ever rejects a malformed or hostile caller.
const maxSummaryCourseSlugs = 200

// courseProgressSummary is one course's aggregate progress for a user.
type courseProgressSummary struct {
	TotalScore    int64    `json:"totalScore"`
	PassedModules []string `json:"passedModules"`
	ViewedCount   int      `json:"viewedCount"`
}

// progressOverviewResponse is everything course-service needs to render one
// course for one learner, in a single response.
//
// It replaces four separate internal round-trips (enrollment check, viewed
// lessons, module progress, course summary) that every module and lesson
// request used to make in sequence — each with its own connection, JSON
// round-trip and set of database queries.
type progressOverviewResponse struct {
	Enrolled      bool                `json:"enrolled"`
	Viewed        []string            `json:"viewed"`
	Modules       []moduleProgressRow `json:"modules"`
	TotalScore    int64               `json:"totalScore"`
	PassedModules []string            `json:"passedModules"`
	ViewedCount   int                 `json:"viewedCount"`
}

// InternalProgressOverview godoc
// @Summary  Get a learner's full progress for one course (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId      query  string  true  "User UUID"
// @Param    courseSlug  query  string  true  "Course slug"
// @Success  200  {object}  progressOverviewResponse
// @Router   /internal/progress/overview [get].
func (s *State) InternalProgressOverview(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	userID := req.URL.Query().Get("userId")
	courseSlug := req.URL.Query().Get("courseSlug")

	if userID == "" || courseSlug == "" {
		s.Error(writer, http.StatusBadRequest, "userId and courseSlug are required")

		return
	}

	slugs := []string{courseSlug}

	viewedByCourse, moduleRows, err := s.loadCourseProgress(ctx, userID, slugs)
	if err != nil {
		zap.L().Error("failed to load progress overview",
			zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	enrolled, err := s.Repos.Enrollments.Exists(ctx, userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to check enrollment",
			zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	viewed := viewedByCourse[courseSlug]
	if viewed == nil {
		viewed = []string{}
	}

	rows := moduleRows[courseSlug]
	summary := summarizeCourseProgress(rows, viewed)

	s.JSON(writer, http.StatusOK, progressOverviewResponse{
		Enrolled:      enrolled,
		Viewed:        viewed,
		Modules:       toModuleProgressRows(rows),
		TotalScore:    summary.TotalScore,
		PassedModules: summary.PassedModules,
		ViewedCount:   summary.ViewedCount,
	})
}

// InternalCourseSummaries godoc
// @Summary  Get aggregate progress for several courses at once (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId       query  string  true  "User UUID"
// @Param    courseSlugs  query  string  true  "Comma-separated course slugs"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/course-summaries [get].
func (s *State) InternalCourseSummaries(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	userID := req.URL.Query().Get("userId")
	slugs := splitCommaList(req.URL.Query().Get("courseSlugs"))

	if userID == "" {
		s.Error(writer, http.StatusBadRequest, "userId is required")

		return
	}

	if len(slugs) > maxSummaryCourseSlugs {
		s.Error(writer, http.StatusBadRequest,
			fmt.Sprintf("at most %d course slugs may be requested at once", maxSummaryCourseSlugs))

		return
	}

	summaries := make(map[string]courseProgressSummary, len(slugs))

	if len(slugs) == 0 {
		s.JSON(writer, http.StatusOK, map[string]any{summariesJSONKey: summaries})

		return
	}

	viewedByCourse, moduleRows, err := s.loadCourseProgress(ctx, userID, slugs)
	if err != nil {
		zap.L().Error("failed to load course summaries", zap.String("userID", userID), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	for _, slug := range slugs {
		summaries[slug] = summarizeCourseProgress(moduleRows[slug], viewedByCourse[slug])
	}

	s.JSON(writer, http.StatusOK, map[string]any{summariesJSONKey: summaries})
}

// loadCourseProgress reads the lesson and module progress of every course
// in slugs with exactly two queries, whatever the length of slugs. It
// returns the viewed lesson slugs and the module progress rows, both keyed
// by course slug.
//
//nolint:gocritic // named results here would trip nonamedreturns instead
func (s *State) loadCourseProgress(
	ctx context.Context, userID string, slugs []string,
) (map[string][]string, map[string][]models.ModuleProgress, error) {
	viewedByCourse, err := s.Repos.LessonProgress.ViewedSlugsByCourses(ctx, userID, slugs)
	if err != nil {
		return nil, nil, fmt.Errorf("load viewed lessons: %w", err)
	}

	moduleRows, err := s.Repos.ModuleProgress.ListByUserCourses(ctx, userID, slugs)
	if err != nil {
		return nil, nil, fmt.Errorf("load module progress: %w", err)
	}

	return viewedByCourse, moduleRows, nil
}

// summarizeCourseProgress derives one course's aggregate progress from the
// rows already in memory, rather than asking the database to sum and filter
// them again.
func summarizeCourseProgress(rows []models.ModuleProgress, viewed []string) courseProgressSummary {
	summary := courseProgressSummary{
		PassedModules: make([]string, 0, len(rows)),
		ViewedCount:   len(viewed),
	}

	for _, row := range rows {
		summary.TotalScore += int64(row.BestScore)

		if row.Passed && row.ModuleSlug != nil {
			summary.PassedModules = append(summary.PassedModules, *row.ModuleSlug)
		}
	}

	return summary
}

// toModuleProgressRows converts persisted module progress into the wire
// shape shared by the internal progress endpoints.
func toModuleProgressRows(rows []models.ModuleProgress) []moduleProgressRow {
	out := make([]moduleProgressRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, moduleProgressRow{
			ModuleIndex: row.ModuleIndex,
			BestScore:   row.BestScore,
			MaxScore:    row.MaxScore,
			Passed:      row.Passed,
			Attempts:    row.Attempts,
		})
	}

	return out
}

// splitCommaList splits a comma-separated query parameter into its
// non-empty, trimmed, deduplicated entries, preserving first-seen order.
func splitCommaList(raw string) []string {
	if raw == "" {
		return nil
	}

	seen := make(map[string]struct{})

	var out []string

	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if _, dup := seen[entry]; dup {
			continue
		}

		seen[entry] = struct{}{}

		out = append(out, entry)
	}

	return out
}
