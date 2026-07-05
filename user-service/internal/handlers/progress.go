package handlers

import (
	"net/http"
)

// viewedLessons returns the set of lesson slugs the user has completed in a
// course, keyed by lesson slug with value true.
func viewedLessons(s *State, r *http.Request, courseSlug, userID string) map[string]bool {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT lessonSlug FROM lesson_progress WHERE userId = $1::uuid AND courseSlug = $2`,
		userID, courseSlug)
	if err != nil {
		return nil
	}
	defer rows.Close()

	lessonsViewed := make(map[string]bool)

	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			lessonsViewed[slug] = true
		}
	}

	return lessonsViewed
}

// MarkLessonComplete godoc
// @Summary   Mark a lesson as complete
// @Tags      Progress
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lessonSlug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lessonSlug}/complete [post].
func (s *State) MarkLessonComplete(writer http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lessonSlug")
	claims := s.claims(r)

	_, err := s.Pool.Exec(r.Context(), `
		INSERT INTO lesson_progress (userId, courseSlug, lessonSlug)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (userId, courseSlug, lessonSlug) DO NOTHING`,
		claims.Subject, courseSlug, lessonSlug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{"message": "Lesson marked as complete"})
}
