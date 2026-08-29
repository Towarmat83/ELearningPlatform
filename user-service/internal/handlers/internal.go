package handlers

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// courseCompleteLessonSlug is the sentinel lesson slug used to mark an
// entire course as complete (for courses with no individual lessons to
// track, e.g. quiz-only courses).
const courseCompleteLessonSlug = "__complete__"

// courseBody is the request body for endpoints keyed by user+course.
type courseBody struct {
	UserID string `json:"userId"`

	CourseSlug string `json:"courseSlug"`
}

// courseCompleteBody is the request body for the course-complete endpoint.
// It extends courseBody with skill and difficulty metadata so user-service
// can update the learner's per-skill expertise level in one call.
type courseCompleteBody struct {
	UserID            string              `json:"userId"`
	CourseSlug        string              `json:"courseSlug"`
	Skills            map[string]struct{} `json:"skills,omitempty"`
	Difficulty        string              `json:"difficulty,omitempty"`
	SkillTotalCourses map[string]int      `json:"skillTotalCourses,omitempty"`
}

// lessonCompleteBody is the request body for marking a single lesson complete.
type lessonCompleteBody struct {
	UserID string `json:"userId"`

	CourseSlug string `json:"courseSlug"`

	LessonSlug string `json:"lessonSlug"`
}

// moduleProgressBody is the request body for recording a quiz module result.
type moduleProgressBody struct {
	UserID string `json:"userId"`

	CourseSlug string `json:"courseSlug"`

	ModuleIndex int `json:"moduleIndex"`

	ModuleSlug string `json:"moduleSlug"`
	ModuleType string `json:"moduleType"`
	Score      int    `json:"score"`

	MaxScore int  `json:"maxScore"`
	Passed   bool `json:"passed"`
}

// moduleProgressRow is a single row returned by InternalGetModuleProgress.
type moduleProgressRow struct {
	ModuleIndex int `json:"moduleIndex"`

	BestScore int `json:"bestScore"`

	MaxScore int  `json:"maxScore"`
	Passed   bool `json:"passed"`
	Attempts int  `json:"attempts"`
}

// internalDecodeOrBadRequest decodes the JSON request body into dst. On
// failure it writes a 400 response and returns false; callers must return
// immediately in that case.
func internalDecodeOrBadRequest(s *State, writer http.ResponseWriter, req *http.Request, dst any) bool {
	err := decode(req, dst)
	if err != nil {
		zap.L().Error("JSON decode failed", zap.Error(err))
		s.Error(writer, http.StatusBadRequest, "Invalid request body")

		return false
	}

	return true
}

// internalRespondExecResult logs execErr (if any) alongside logMsg/fields and
// writes the {"ok": bool} response shared by internal write endpoints.
func internalRespondExecResult(state *State, writer http.ResponseWriter, execErr error, logMsg string, fields ...zap.Field) {
	if execErr != nil {
		zap.L().Error(logMsg, append(fields, zap.Error(execErr))...)
		state.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	state.JSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// InternalAutoEnroll godoc
// @Summary  Auto-enroll a user in a public course (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "userId, courseSlug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/enrollments/auto [post].
func (s *State) InternalAutoEnroll(writer http.ResponseWriter, req *http.Request) {
	var body courseBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	err := s.Repos.Enrollments.Create(req.Context(), body.UserID, body.CourseSlug)
	internalRespondExecResult(s, writer, err, "failed to auto-enroll user",
		zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug))
}

// InternalCheckEnrollment godoc
// @Summary  Check if a user is enrolled (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId      query  string  true  "User UUID"
// @Param    courseSlug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]bool
// @Router   /internal/enrollments/check [get].
func (s *State) InternalCheckEnrollment(writer http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("userId")
	courseSlug := req.URL.Query().Get("courseSlug")

	enrolled, err := s.Repos.Enrollments.Exists(req.Context(), userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to check enrollment", zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{adminJSONKeyEnrolled: enrolled})
}

// InternalCheckPathEnrollment godoc
// @Summary  Check if a user is enrolled in any of the given paths (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId     query  string  true  "User UUID"
// @Param    pathSlugs  query  string  true  "Comma-separated path slugs"
// @Success  200  {object}  map[string]bool
// @Router   /internal/paths/check [get].
func (s *State) InternalCheckPathEnrollment(writer http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("userId")
	raw := req.URL.Query().Get("pathSlugs")

	var slugs []string

	for sl := range strings.SplitSeq(raw, ",") {
		if sl = strings.TrimSpace(sl); sl != "" {
			slugs = append(slugs, sl)
		}
	}

	if userID == "" || len(slugs) == 0 {
		s.JSON(writer, http.StatusOK, map[string]bool{adminJSONKeyEnrolled: false})

		return
	}

	enrolled, err := s.Repos.Paths.EnrolledInAny(req.Context(), userID, slugs)
	if err != nil {
		zap.L().Error("failed to check path enrollment", zap.String("userID", userID), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{adminJSONKeyEnrolled: enrolled})
}

// InternalViewedLessons godoc
// @Summary  Get viewed lessons for a user (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId      query  string  true  "User UUID"
// @Param    courseSlug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/viewed [get].
func (s *State) InternalViewedLessons(writer http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("userId")
	courseSlug := req.URL.Query().Get("courseSlug")

	slugs, err := s.Repos.LessonProgress.ViewedSlugs(req.Context(), userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to query viewed lessons", zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	if slugs == nil {
		slugs = []string{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"viewed": slugs})
}

// InternalMarkComplete godoc
// @Summary  Mark a lesson complete (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "userId, courseSlug, lessonSlug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/complete [post].
func (s *State) InternalMarkComplete(writer http.ResponseWriter, req *http.Request) {
	var body lessonCompleteBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	err := s.Repos.LessonProgress.MarkComplete(req.Context(), body.UserID, body.CourseSlug, body.LessonSlug)
	internalRespondExecResult(s, writer, err, "failed to mark lesson complete",
		zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug), zap.String("lessonSlug", body.LessonSlug))
}

// InternalMarkCourseComplete godoc
// @Summary  Mark a whole course as complete (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "userId, courseSlug, skills, difficulty"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/course-complete [post].
func (s *State) InternalMarkCourseComplete(writer http.ResponseWriter, req *http.Request) {
	var body courseCompleteBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	err := s.Repos.LessonProgress.MarkComplete(req.Context(), body.UserID, body.CourseSlug, courseCompleteLessonSlug)
	if err != nil {
		internalRespondExecResult(s, writer, err, "failed to mark course complete",
			zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug))

		return
	}

	awardErr := s.Repos.Badges.Award(req.Context(), body.UserID, body.CourseSlug)
	if awardErr != nil {
		zap.L().Warn("failed to award badge on course completion", zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug), zap.Error(awardErr))
	}

	_ = s.awardXP(req, body.UserID, repository.XPSourceCourse, body.CourseSlug, repository.XPAmountCourse)

	if body.Difficulty != "" && len(body.Skills) > 0 {
		err := s.Repos.SkillLevels.UpsertAll(req.Context(), body.UserID, body.Skills, body.Difficulty, body.SkillTotalCourses)
		if err != nil {
			zap.L().Error("failed to upsert skill levels",
				zap.String("userID", body.UserID), zap.Error(err))
		}
	}

	internalRespondExecResult(s, writer, nil, "",
		zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug))
}

// InternalRecordModuleProgress godoc
// @Summary  Record quiz result for a module (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "see moduleProgressBody"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/module [post].
func (s *State) InternalRecordModuleProgress(writer http.ResponseWriter, req *http.Request) {
	var body moduleProgressBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	err := s.Repos.ModuleProgress.RecordProgress(req.Context(), body.UserID, body.CourseSlug,
		body.ModuleIndex, body.ModuleSlug, body.ModuleType, body.Score, body.MaxScore, body.Passed)

	if err == nil && body.Passed {
		_ = s.awardXP(req, body.UserID, repository.XPSourceModule, body.CourseSlug+"/"+body.ModuleSlug, repository.XPAmountModule)
	}

	internalRespondExecResult(s, writer, err, "failed to record module progress",
		zap.String("userID", body.UserID), zap.String("courseSlug", body.CourseSlug))
}

// InternalCourseSummary godoc
// @Summary  Get total score and passed modules for a user in a course
// @Tags     Internal
// @Produce  json
// @Param    userId      query  string  true  "User UUID"
// @Param    courseSlug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/course-summary [get].
func (s *State) InternalCourseSummary(writer http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("userId")
	courseSlug := req.URL.Query().Get("courseSlug")

	totalScore, err := s.Repos.ModuleProgress.TotalScore(req.Context(), userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to query course score", zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	passedModules, err := s.Repos.ModuleProgress.PassedModuleSlugs(req.Context(), userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to query passed modules", zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	// Also count viewed lessons (text/video/image modules marked complete).
	// This allows text-only courses to satisfy "any progress" prerequisites.
	viewedCount, _ := s.Repos.LessonProgress.CountViewed(req.Context(), userID, courseSlug)

	s.JSON(writer, http.StatusOK, map[string]any{
		"totalScore":    totalScore,
		"passedModules": passedModules,
		"viewedCount":   viewedCount,
	})
}

// InternalGetModuleProgress godoc
// @Summary  Get module progress for a user in a course (internal)
// @Tags     Internal
// @Produce  json
// @Param    userId      query  string  true  "User UUID"
// @Param    courseSlug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/modules [get].
func (s *State) InternalGetModuleProgress(writer http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("userId")
	courseSlug := req.URL.Query().Get("courseSlug")

	list, err := s.Repos.ModuleProgress.ListByUserCourse(req.Context(), userID, courseSlug)
	if err != nil {
		zap.L().Error("failed to query module progress", zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	progress := make([]moduleProgressRow, 0, len(list))
	for _, p := range list {
		progress = append(progress, moduleProgressRow{
			ModuleIndex: p.ModuleIndex, BestScore: p.BestScore, MaxScore: p.MaxScore, Passed: p.Passed, Attempts: p.Attempts,
		})
	}

	s.JSON(writer, http.StatusOK, map[string]any{"progress": progress})
}
