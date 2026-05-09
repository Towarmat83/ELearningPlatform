package handlers

import (
	"net/http"

	"github.com/elearning/user-service/internal/metrics"
)

// POST /api/courses/{slug}/enroll
func (s *State) Enroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO enrollments (user_id, course_slug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		claims.Subject, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Inc()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Enrolled successfully"})
}

// DELETE /api/courses/{slug}/unenroll
func (s *State) Unenroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		claims.Subject, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Dec()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Unenrolled successfully"})
}

// GET /api/my/courses
func (s *State) MyCourses(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	rows, err := s.Pool.Query(r.Context(), `
		SELECT e.course_slug,
		       COUNT(lp.lesson_slug) AS viewed_lessons,
		       MAX(lp.viewed_at)::text AS last_activity
		FROM enrollments e
		LEFT JOIN lesson_progress lp ON lp.user_id = e.user_id AND lp.course_slug = e.course_slug
		WHERE e.user_id = $1::uuid
		GROUP BY e.course_slug
		ORDER BY MAX(e.enrolled_at) DESC`, claims.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type myCourse struct {
		Slug          string  `json:"slug"`
		ViewedLessons int64   `json:"viewed_lessons"`
		LastActivity  *string `json:"last_activity"`
	}

	courses := make([]myCourse, 0)
	for rows.Next() {
		var c myCourse
		if err := rows.Scan(&c.Slug, &c.ViewedLessons, &c.LastActivity); err != nil {
			continue
		}
		courses = append(courses, c)
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": courses})
}
