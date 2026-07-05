package handlers

import (
	"log/slog"
	"net/http"
)

// courseBody is the request body for endpoints keyed by user+course.
type courseBody struct {
	UserID string `json:"userId"`

	CourseSlug string `json:"courseSlug"`
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
		s.Error(writer, http.StatusBadRequest, "error when decoding JSON body: "+err.Error())

		return false
	}

	return true
}

// internalRespondExecResult logs execErr (if any) alongside logMsg/logArgs and
// writes the {"ok": bool} response shared by internal write endpoints.
func internalRespondExecResult(state *State, writer http.ResponseWriter, execErr error, logMsg string, logArgs ...any) {
	if execErr != nil {
		slog.Error(logMsg, append(logArgs, "err", execErr)...)
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
// @Param    body  object  true  "userId, courseSlug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/enrollments/auto [post].
func (s *State) InternalAutoEnroll(writer http.ResponseWriter, req *http.Request) {
	var body courseBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	_, err := s.Pool.Exec(req.Context(),
		`INSERT INTO enrollments (userId, courseSlug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		body.UserID, body.CourseSlug)
	internalRespondExecResult(s, writer, err, "failed to auto-enroll user",
		"userID", body.UserID, "courseSlug", body.CourseSlug)
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

	var enrolled bool

	err := s.Pool.QueryRow(req.Context(),
		`SELECT COUNT(*) > 0 FROM enrollments WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug).Scan(&enrolled)
	if err != nil {
		slog.Error("failed to check enrollment", "userID", userID, "courseSlug", courseSlug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{"enrolled": enrolled})
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

	rows, err := s.Pool.Query(req.Context(),
		`SELECT lessonSlug FROM lesson_progress WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug)
	if err != nil {
		slog.Error("failed to query viewed lessons", "userID", userID, "courseSlug", courseSlug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}
	defer rows.Close()

	var slugs []string

	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			slugs = append(slugs, slug)
		}
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
// @Param    body  object  true  "userId, courseSlug, lessonSlug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/complete [post].
func (s *State) InternalMarkComplete(writer http.ResponseWriter, req *http.Request) {
	var body lessonCompleteBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	_, err := s.Pool.Exec(req.Context(),
		`INSERT INTO lesson_progress (userId, courseSlug, lessonSlug, viewed_at)
		 VALUES ($1::uuid, $2, $3, NOW())
		 ON CONFLICT (userId, courseSlug, lessonSlug) DO NOTHING`,
		body.UserID, body.CourseSlug, body.LessonSlug)
	internalRespondExecResult(s, writer, err, "failed to mark lesson complete",
		"userID", body.UserID, "courseSlug", body.CourseSlug, "lessonSlug", body.LessonSlug)
}

// InternalMarkCourseComplete godoc
// @Summary  Mark a whole course as complete (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  object  true  "userId, courseSlug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/course-complete [post].
func (s *State) InternalMarkCourseComplete(writer http.ResponseWriter, req *http.Request) {
	var body courseBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	_, err := s.Pool.Exec(req.Context(),
		`INSERT INTO lesson_progress (userId, courseSlug, lessonSlug, viewed_at)
		 VALUES ($1::uuid, $2, '__complete__', NOW())
		 ON CONFLICT (userId, courseSlug, lessonSlug) DO NOTHING`,
		body.UserID, body.CourseSlug)
	internalRespondExecResult(s, writer, err, "failed to mark course complete",
		"userID", body.UserID, "courseSlug", body.CourseSlug)
}

// InternalRecordModuleProgress godoc
// @Summary  Record quiz result for a module (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  object  true  "see moduleProgressBody"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/module [post].
func (s *State) InternalRecordModuleProgress(writer http.ResponseWriter, req *http.Request) {
	var body moduleProgressBody
	if !internalDecodeOrBadRequest(s, writer, req, &body) {
		return
	}

	_, err := s.Pool.Exec(req.Context(), `
		INSERT INTO module_progress (userId, courseSlug, moduleIndex, moduleSlug, bestScore, maxScore, passed, attempts, completed_at)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''), $5, $6, $7, 1, CASE WHEN $7 THEN NOW() ELSE NULL END)
		ON CONFLICT (userId, courseSlug, moduleIndex) DO UPDATE SET
			attempts     = module_progress.attempts + 1,
			bestScore   = GREATEST(module_progress.bestScore, $5),
			maxScore    = $6,
			passed       = module_progress.passed OR $7,
			moduleSlug  = COALESCE(module_progress.moduleSlug, NULLIF($4, '')),
			completed_at = CASE WHEN ($7 AND module_progress.completed_at IS NULL) THEN NOW() ELSE module_progress.completed_at END,
			updatedAt   = NOW()`,
		body.UserID, body.CourseSlug, body.ModuleIndex, body.ModuleSlug, body.Score, body.MaxScore, body.Passed)
	internalRespondExecResult(s, writer, err, "failed to record module progress",
		"userID", body.UserID, "courseSlug", body.CourseSlug)
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

	var totalScore int

	err := s.Pool.QueryRow(req.Context(),
		`SELECT COALESCE(SUM(bestScore), 0) FROM module_progress
		 WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug).Scan(&totalScore)
	if err != nil {
		slog.Error("failed to query course score", "userID", userID, "courseSlug", courseSlug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}

	rows, err := s.Pool.Query(req.Context(),
		`SELECT moduleSlug FROM module_progress
		 WHERE userId = $1::uuid AND courseSlug = $2
		   AND passed = true AND moduleSlug IS NOT NULL`,
		userID, courseSlug)
	if err != nil {
		slog.Error("failed to query passed modules", "userID", userID, "courseSlug", courseSlug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}
	defer rows.Close()

	passedModules := []string{}

	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			passedModules = append(passedModules, slug)
		}
	}

	// Also count viewed lessons (text/video/image modules marked complete).
	// This allows text-only courses to satisfy "any progress" prerequisites.
	var viewedCount int

	_ = s.Pool.QueryRow(req.Context(),
		`SELECT COUNT(*) FROM lesson_progress WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug).Scan(&viewedCount)

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

	rows, err := s.Pool.Query(req.Context(),
		`SELECT moduleIndex, bestScore, maxScore, passed, attempts
		 FROM module_progress
		 WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug)
	if err != nil {
		slog.Error("failed to query module progress", "userID", userID, "courseSlug", courseSlug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "DB error")

		return
	}
	defer rows.Close()

	progress := make([]moduleProgressRow, 0)

	for rows.Next() {
		var row moduleProgressRow
		if rows.Scan(&row.ModuleIndex, &row.BestScore, &row.MaxScore, &row.Passed, &row.Attempts) == nil {
			progress = append(progress, row)
		}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"progress": progress})
}
