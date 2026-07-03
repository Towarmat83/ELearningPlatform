package handlers

import (
	"net/http"
)

func viewedLessons(s *State, r *http.Request, courseSlug, userID string) map[string]bool {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT lesson_slug FROM lesson_progress WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug)
	if err != nil {
		return nil
	}
	defer rows.Close()

	m := make(map[string]bool)

	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			m[slug] = true
		}
	}

	return m
}

// MarkLessonComplete godoc
// @Summary   Mark a lesson as complete
// @Tags      Progress
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lesson_slug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lesson_slug}/complete [post].
func (s *State) MarkLessonComplete(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lesson_slug")
	claims := s.claims(r)

	_, err := s.Pool.Exec(r.Context(), `
		INSERT INTO lesson_progress (user_id, course_slug, lesson_slug)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id, course_slug, lesson_slug) DO NOTHING`,
		claims.Subject, courseSlug, lessonSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"message": "Lesson marked as complete"})
}
