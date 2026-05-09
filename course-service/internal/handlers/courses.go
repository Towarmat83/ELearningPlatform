package handlers

import (
	"net/http"
	"strings"

	"github.com/elearning/course-service/internal/content"
)

type courseResponse struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`
	IsPublished bool   `json:"is_published"`
	ModuleCount int    `json:"module_count"`
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
		ModuleCount: len(c.Modules),
		Source:      c.Source,
	}
}

// GET /api/courses — only published courses, public endpoint
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


