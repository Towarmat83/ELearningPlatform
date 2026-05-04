package handlers

import (
	"net/http"

	"github.com/elearning/api-go/internal/content"
)

type lessonSummary struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Order  int    `json:"order"`
	Viewed bool   `json:"viewed"`
}

type lessonDetail struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Content string `json:"content"`
	Viewed  bool   `json:"viewed"`
}

func isEnrolled(s *State, r *http.Request, courseSlug, userID string) bool {
	var cnt int64
	s.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug).Scan(&cnt)
	return cnt > 0
}

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

// GET /api/courses/{slug}/lessons
func (s *State) ListLessons(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && (!c.IsPublished || !isEnrolled(s, r, courseSlug, claims.Subject)) {
		s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
		return
	}

	viewed := viewedLessons(s, r, courseSlug, claims.Subject)
	out := make([]lessonSummary, 0, len(c.Lessons))
	for _, l := range c.Lessons {
		out = append(out, lessonSummary{
			Slug:   l.Slug,
			Title:  l.Title,
			Order:  l.Order,
			Viewed: viewed[l.Slug],
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"lessons": out})
}

// GET /api/courses/{slug}/lessons/{lesson_slug}
func (s *State) GetLesson(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lesson_slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && (!c.IsPublished || !isEnrolled(s, r, courseSlug, claims.Subject)) {
		s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
		return
	}

	var found *content.Lesson
	for i := range c.Lessons {
		if c.Lessons[i].Slug == lessonSlug {
			found = &c.Lessons[i]
			break
		}
	}
	if found == nil {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}

	viewed := viewedLessons(s, r, courseSlug, claims.Subject)
	s.JSON(w, http.StatusOK, map[string]any{"lesson": lessonDetail{
		Slug:    found.Slug,
		Title:   found.Title,
		Order:   found.Order,
		Content: found.Content,
		Viewed:  viewed[found.Slug],
	}})
}

// POST /api/courses/{slug}/lessons/{lesson_slug}/complete
func (s *State) MarkLessonComplete(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lesson_slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil || !c.IsPublished {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if !isEnrolled(s, r, courseSlug, claims.Subject) {
		s.Error(w, http.StatusForbidden, "Not enrolled")
		return
	}

	found := false
	for _, l := range c.Lessons {
		if l.Slug == lessonSlug {
			found = true
			break
		}
	}
	if !found {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}

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
