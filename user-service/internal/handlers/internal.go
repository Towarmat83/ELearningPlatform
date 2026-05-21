package handlers

import (
	"net/http"
)

// InternalCheckEnrollment godoc
// @Summary  Check if a user is enrolled (internal)
// @Tags     Internal
// @Produce  json
// @Param    user_id      query  string  true  "User UUID"
// @Param    course_slug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]bool
// @Router   /internal/enrollments/check [get]
func (s *State) InternalCheckEnrollment(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	courseSlug := r.URL.Query().Get("course_slug")
	var enrolled bool
	err := s.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) > 0 FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug).Scan(&enrolled)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"enrolled": enrolled})
}

// InternalViewedLessons godoc
// @Summary  Get viewed lessons for a user (internal)
// @Tags     Internal
// @Produce  json
// @Param    user_id      query  string  true  "User UUID"
// @Param    course_slug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/viewed [get]
func (s *State) InternalViewedLessons(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	courseSlug := r.URL.Query().Get("course_slug")
	rows, err := s.Pool.Query(r.Context(),
		`SELECT lesson_slug FROM lesson_progress WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
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
	s.JSON(w, http.StatusOK, map[string]any{"viewed": slugs})
}

// InternalMarkComplete godoc
// @Summary  Mark a lesson complete (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "user_id, course_slug, lesson_slug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/complete [post]
func (s *State) InternalMarkComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     string `json:"user_id"`
		CourseSlug string `json:"course_slug"`
		LessonSlug string `json:"lesson_slug"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`		INSERT INTO lesson_progress (user_id, course_slug, lesson_slug, viewed_at)
		 VALUES ($1::uuid, $2, $3, NOW())
		 ON CONFLICT (user_id, course_slug, lesson_slug) DO NOTHING`,
		body.UserID, body.CourseSlug, body.LessonSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// InternalRecordModuleProgress godoc
// @Summary  Record quiz result for a module (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "user_id, course_slug, module_index, score, max_score, passed"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/module [post]
func (s *State) InternalRecordModuleProgress(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID      string `json:"user_id"`
		CourseSlug  string `json:"course_slug"`
		ModuleIndex int    `json:"module_index"`
		ModuleSlug  string `json:"module_slug"`
		Score       int    `json:"score"`
		MaxScore    int    `json:"max_score"`
		Passed      bool   `json:"passed"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	_, err := s.Pool.Exec(r.Context(), `
		INSERT INTO module_progress (user_id, course_slug, module_index, module_slug, best_score, max_score, passed, attempts, completed_at)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''), $5, $6, $7, 1, CASE WHEN $7 THEN NOW() ELSE NULL END)
		ON CONFLICT (user_id, course_slug, module_index) DO UPDATE SET
			attempts     = module_progress.attempts + 1,
			best_score   = GREATEST(module_progress.best_score, $5),
			max_score    = $6,
			passed       = module_progress.passed OR $7,
			module_slug  = COALESCE(module_progress.module_slug, NULLIF($4, '')),
			completed_at = CASE WHEN ($7 AND module_progress.completed_at IS NULL) THEN NOW() ELSE module_progress.completed_at END,
			updated_at   = NOW()`,
		body.UserID, body.CourseSlug, body.ModuleIndex, body.ModuleSlug, body.Score, body.MaxScore, body.Passed)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// InternalGetModuleProgress godoc
// @Summary  Get module progress for a user in a course (internal)
// @Tags     Internal
// @Produce  json
// @Param    user_id      query  string  true  "User UUID"
// @Param    course_slug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/modules [get]
func (s *State) InternalGetModuleProgress(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	courseSlug := r.URL.Query().Get("course_slug")
	rows, err := s.Pool.Query(r.Context(),
		`SELECT module_index, best_score, max_score, passed, attempts
		 FROM module_progress
		 WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	type progressRow struct {
		ModuleIndex int  `json:"module_index"`
		BestScore   int  `json:"best_score"`
		MaxScore    int  `json:"max_score"`
		Passed      bool `json:"passed"`
		Attempts    int  `json:"attempts"`
	}
	progress := make([]progressRow, 0)
	for rows.Next() {
		var p progressRow
		if rows.Scan(&p.ModuleIndex, &p.BestScore, &p.MaxScore, &p.Passed, &p.Attempts) == nil {
			progress = append(progress, p)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"progress": progress})
}
