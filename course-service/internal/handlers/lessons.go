package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/elearning/course-service/internal/content"
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
	Type    string `json:"type"`
	Content string `json:"content"`
	Viewed  bool   `json:"viewed"`
}

func (s *State) isEnrolled(r *http.Request, courseSlug, userID string) bool {
	u, err := url.Parse(s.Config.UserServiceURL + "/internal/enrollments/check")
	if err != nil {
		return false
	}
	q := u.Query()
	q.Set("user_id", userID)
	q.Set("course_slug", courseSlug)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Enrolled bool `json:"enrolled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Enrolled
}

// ListLessons godoc
// @Summary   List lessons for a course
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons [get]
func (s *State) ListLessons(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && (!c.IsPublished || !s.isEnrolled(r, courseSlug, claims.Subject)) {
		s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
		return
	}

	viewed := s.viewedLessons(r, courseSlug, claims.Subject)

	modules := s.visibleModules(c, r)

	if len(c.Lessons) > 0 {
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
		return
	}

	out := make([]lessonSummary, 0, len(modules))
	for i, m := range modules {
		out = append(out, lessonSummary{
			Slug:   m.Slug(),
			Title:  m.Name,
			Order:  i + 1,
			Viewed: viewed[m.Slug()],
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"lessons": out})
}

// GetLesson godoc
// @Summary   Get a lesson by slug
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lesson_slug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lesson_slug} [get]
func (s *State) GetLesson(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lesson_slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && (!c.IsPublished || !s.isEnrolled(r, courseSlug, claims.Subject)) {
		s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
		return
	}

	viewed := s.viewedLessons(r, courseSlug, claims.Subject)
	modules := s.visibleModules(c, r)

	for i := range c.Lessons {
		if c.Lessons[i].Slug == lessonSlug {
			s.JSON(w, http.StatusOK, map[string]any{"lesson": lessonDetail{
				Slug:    c.Lessons[i].Slug,
				Title:   c.Lessons[i].Title,
				Order:   c.Lessons[i].Order,
				Content: c.Lessons[i].Content,
				Viewed:  viewed[c.Lessons[i].Slug],
			}})
			return
		}
	}

	for i, m := range modules {
		if m.Slug() == lessonSlug {
			if m.Type == "quiz" {
				s.Error(w, http.StatusNotFound, "Quiz modules use a separate endpoint")
				return
			}
			body := m.Content()
			if m.Type != "text" {
				body = content.ReplicatedPath(m, s.Config.UploadsDir)
			}
			if m.HasGitContent() {
				data, err := content.FetchModuleContent(m.Src, m.Ref, m.Path, s.tokenForRepo(m.Src))
				if err != nil {
					body = "⚠️ This content is not yet available. Please check back later.\n\n_Failed to load from remote repository._"
				} else {
					body = string(data)
				}
			}
			s.JSON(w, http.StatusOK, map[string]any{"lesson": lessonDetail{
				Slug:    m.Slug(),
				Title:   m.Name,
				Order:   i + 1,
				Type:    m.Type,
				Content: body,
				Viewed:  viewed[m.Slug()],
			}})
			return
		}
	}

	s.Error(w, http.StatusNotFound, "Lesson not found")
}

// MarkLessonComplete godoc
// @Summary   Mark a lesson as complete
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lesson_slug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]string
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lesson_slug}/complete [post]
func (s *State) MarkLessonComplete(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	lessonSlug := param(r, "lesson_slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil || !c.IsPublished {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if !s.isEnrolled(r, courseSlug, claims.Subject) {
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
		for _, m := range c.Modules {
			if m.Slug() == lessonSlug {
				found = true
				break
			}
		}
	}
	if !found {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}

	body := map[string]string{
		"user_id":     claims.Subject,
		"course_slug": courseSlug,
		"lesson_slug": lessonSlug,
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	resp, err := http.Post(s.Config.UserServiceURL+"/internal/progress/complete", "application/json", &buf)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to mark lesson as complete")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		s.Error(w, http.StatusInternalServerError, "Failed to mark lesson as complete")
		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"message": "Lesson marked as complete"})
}
