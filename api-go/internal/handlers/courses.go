package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/elearning/api-go/internal/content"
	"github.com/elearning/api-go/internal/metrics"
)

type courseResponse struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`
	IsPublished bool   `json:"is_published"`
	AutoEnroll  bool   `json:"auto_enroll"`
	LessonCount int    `json:"lesson_count"`
	Source      string `json:"source,omitempty"`
}

type dbCourseSettings struct {
	IsPublished bool
	AutoEnroll  bool
	Source      string
}

// loadAllCourseSettings fetches all rows from course_settings.
func (s *State) loadAllCourseSettings(ctx context.Context) map[string]dbCourseSettings {
	rows, err := s.Pool.Query(ctx, "SELECT course_slug, is_published, auto_enroll, source FROM course_settings")
	if err != nil {
		return nil
	}
	defer rows.Close()
	m := map[string]dbCourseSettings{}
	for rows.Next() {
		var slug string
		var cs dbCourseSettings
		if err := rows.Scan(&slug, &cs.IsPublished, &cs.AutoEnroll, &cs.Source); err == nil {
			m[slug] = cs
		}
	}
	return m
}

// effectiveSettings returns (isPublished, autoEnroll) for a course,
// using DB override when present, falling back to YAML defaults.
func effectiveSettings(c *content.Course, dbSettings map[string]dbCourseSettings) (bool, bool) {
	if cs, ok := dbSettings[c.Slug]; ok {
		return cs.IsPublished, cs.AutoEnroll
	}
	// No DB row: local courses use YAML value, git courses default to unpublished
	if c.Source == "local" {
		return c.IsPublished, false
	}
	return false, false
}

func toCourseResponse(c *content.Course, dbSettings map[string]dbCourseSettings) courseResponse {
	pub, autoEnroll := effectiveSettings(c, dbSettings)
	return courseResponse{
		Slug:        c.Slug,
		Title:       c.Title,
		Description: c.Description,
		Category:    c.Category,
		Difficulty:  c.Difficulty,
		IsPublished: pub,
		AutoEnroll:  autoEnroll,
		LessonCount: len(c.Lessons),
		Source:      c.Source,
	}
}

// GET /api/courses — only published courses, public endpoint
func (s *State) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := strings.ToLower(q.Get("category"))
	difficulty := strings.ToLower(q.Get("difficulty"))
	search := strings.ToLower(q.Get("search"))

	dbSettings := s.loadAllCourseSettings(r.Context())

	all := s.Content.List()
	out := make([]courseResponse, 0, len(all))
	for _, c := range all {
		pub, _ := effectiveSettings(c, dbSettings)
		if !pub {
			continue
		}
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
		out = append(out, toCourseResponse(c, dbSettings))
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": len(out)})
}

// GET /api/courses/{slug}
func (s *State) GetCourse(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	dbSettings := s.loadAllCourseSettings(r.Context())
	pub, _ := effectiveSettings(c, dbSettings)
	if !pub {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	s.JSON(w, http.StatusOK, toCourseResponse(c, dbSettings))
}

// POST /api/courses/{slug}/enroll — requires auto_enroll enabled
func (s *State) Enroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	dbSettings := s.loadAllCourseSettings(r.Context())
	pub, autoEnroll := effectiveSettings(c, dbSettings)
	if !pub {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if !autoEnroll {
		s.Error(w, http.StatusForbidden, "Enrollment for this course requires admin approval")
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

	dbSettings := s.loadAllCourseSettings(r.Context())
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
			courses = append(courses, myCourse{
				courseResponse: courseResponse{Slug: slug, Title: slug},
				ViewedLessons:  viewed,
				LastActivity:   lastActivity,
			})
			continue
		}
		courses = append(courses, myCourse{
			courseResponse: toCourseResponse(c, dbSettings),
			ViewedLessons:  viewed,
			LastActivity:   lastActivity,
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// GET /api/admin/courses — all courses with settings, for admins
func (s *State) AdminListCourses(w http.ResponseWriter, r *http.Request) {
	dbSettings := s.loadAllCourseSettings(r.Context())

	// Enrollment counts per course
	enrollCounts := map[string]int64{}
	ecRows, err := s.Pool.Query(r.Context(),
		`SELECT course_slug, COUNT(*)::bigint FROM enrollments GROUP BY course_slug`)
	if err == nil {
		defer ecRows.Close()
		for ecRows.Next() {
			var slug string
			var cnt int64
			if ecRows.Scan(&slug, &cnt) == nil {
				enrollCounts[slug] = cnt
			}
		}
	}

	type adminCourseResponse struct {
		courseResponse
		EnrollmentCount int64 `json:"enrollment_count"`
	}

	all := s.Content.All()
	out := make([]adminCourseResponse, 0, len(all))
	for _, c := range all {
		out = append(out, adminCourseResponse{
			courseResponse:  toCourseResponse(c, dbSettings),
			EnrollmentCount: enrollCounts[c.Slug],
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": len(out)})
}
