package handlers

import (
	"net/http"
	"strings"

	"github.com/elearning/api-go/internal/content"
	"github.com/elearning/api-go/internal/metrics"
)

// courseResponse is the public course representation sent to clients.
type courseResponse struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`
	IsPublished bool   `json:"is_published"`
	LessonCount int    `json:"lesson_count"`
	Source      string `json:"source,omitempty"`
}

func toCourseResponse(c *content.Course) courseResponse {
	return courseResponse{
		Slug:        c.Slug,
		Title:       c.Title,
		Description: c.Description,
		Category:    c.Category,
		Difficulty:  c.Difficulty,
		IsPublished: c.IsPublished,
		LessonCount: len(c.Lessons),
		Source:      c.Source,
	}
}

// GET /api/courses
func (s *State) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := strings.ToLower(q.Get("category"))
	difficulty := strings.ToLower(q.Get("difficulty"))
	search := strings.ToLower(q.Get("search"))

	all := s.Content.List()
	out := make([]courseResponse, 0, len(all))
	for _, c := range all {
		if category != "" && strings.ToLower(c.Category) != category {
			continue
		}
		if difficulty != "" && strings.ToLower(c.Difficulty) != difficulty {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(c.Title), search) &&
				!strings.Contains(strings.ToLower(c.Description), search) {
				continue
			}
		}
		out = append(out, toCourseResponse(c))
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": len(out)})
}

// GET /api/courses/{slug}
func (s *State) GetCourse(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil || !c.IsPublished {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	s.JSON(w, http.StatusOK, toCourseResponse(c))
}

// POST /api/courses/{slug}/enroll
func (s *State) Enroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil || !c.IsPublished {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
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
		courseResponse
		ViewedLessons int64   `json:"viewed_lessons"`
		LastActivity  *string `json:"last_activity"`
	}

	courses := make([]myCourse, 0)
	for rows.Next() {
		var slug string
		var viewed int64
		var lastActivity *string
		if err := rows.Scan(&slug, &viewed, &lastActivity); err != nil {
			continue
		}
		c := s.Content.Get(slug)
		if c == nil {
			// Course was removed from disk but enrollment still exists — include minimal info
			courses = append(courses, myCourse{
				courseResponse: courseResponse{Slug: slug, Title: slug},
				ViewedLessons:  viewed,
				LastActivity:   lastActivity,
			})
			continue
		}
		courses = append(courses, myCourse{
			courseResponse: toCourseResponse(c),
			ViewedLessons:  viewed,
			LastActivity:   lastActivity,
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// GET /api/admin/courses  (admin: sees all, including unpublished)
func (s *State) AdminListCourses(w http.ResponseWriter, r *http.Request) {
	all := s.Content.All()
	out := make([]courseResponse, 0, len(all))
	for _, c := range all {
		out = append(out, toCourseResponse(c))
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": len(out)})
}
