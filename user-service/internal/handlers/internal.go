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
