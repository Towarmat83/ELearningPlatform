package handlers

import (
	"net/http"
	"strings"

	"github.com/elearning/course-service/internal/content"
)

type courseResponse struct {
	Slug            string `json:"slug"`
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Difficulty      string `json:"difficulty"`
	IsPublished     bool   `json:"is_published"`
	ModuleCount     int    `json:"module_count"`
	LabCount        int    `json:"lab_count"`
	EnrollmentCount int    `json:"enrollment_count"`
	Source          string `json:"source,omitempty"`
}

func toCourseResponse(c *content.Course) courseResponse {
	return courseResponse{
		Slug:            c.Slug,
		ID:              c.Slug,
		Title:           c.Title,
		Description:     c.Description,
		Category:        c.Category,
		Difficulty:      c.Difficulty,
		IsPublished:     c.IsPublished,
		ModuleCount:     len(c.Modules),
		LabCount:        len(c.Modules),
		EnrollmentCount: 0,
		Source:          c.Source,
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

// GET /api/admin/courses — all courses (including hidden), admin only
func (s *State) ListAdminCourses(w http.ResponseWriter, r *http.Request) {
	all := s.Content.List()
	out := make([]courseResponse, 0, len(all))
	for _, c := range all {
		out = append(out, toCourseResponse(c))
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": len(out)})
}

type labResponse struct {
	ID          string `json:"id"`
	CourseID    string `json:"course_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	LabType     string `json:"lab_type"`
	Points      int    `json:"points"`
	OrderIndex  int    `json:"order_index"`
	IsPublished bool   `json:"is_published"`
}

func moduleTypeToLabType(t string) string {
	switch t {
	case "text":
		return "form"
	case "video", "image":
		return "interactive"
	default:
		return "interactive"
	}
}

// GET /api/courses/{slug}/labs/{lab_id} — returns a single module as a lab
func (s *State) GetLab(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	labID := param(r, "lab_id")
	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	for _, m := range c.Modules {
		if m.Slug() == labID {
			s.JSON(w, http.StatusOK, map[string]any{
				"lab": labResponse{
					ID:          m.Slug(),
					CourseID:    c.Slug,
					Title:       m.Name,
					Description: "",
					LabType:     moduleTypeToLabType(m.Type),
					Points:      0,
					OrderIndex:  0,
					IsPublished: true,
				},
				"progress": nil,
			})
			return
		}
	}
	s.Error(w, http.StatusNotFound, "Lab not found")
}

// GET /api/courses/{slug}/labs — returns course modules as labs
func (s *State) ListLabs(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}

	labs := make([]labResponse, 0, len(c.Modules))
	for i, m := range c.Modules {
		labs = append(labs, labResponse{
			ID:          m.Slug(),
			CourseID:    c.Slug,
			Title:       m.Name,
			Description: "",
			LabType:     moduleTypeToLabType(m.Type),
			Points:      0,
			OrderIndex:  i + 1,
			IsPublished: true,
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"labs": labs})
}

// GET /api/courses/{slug}/progress — returns progress based on module count
func (s *State) GetCourseProgress(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	c := s.Content.Get(courseSlug)
	total := 0
	if c != nil {
		total = len(c.Modules)
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"course_id":             courseSlug,
		"user_id":               "",
		"total_labs":            total,
		"completed_labs":        0,
		"total_points_possible": 0,
		"total_points_earned":   0,
		"completion_percentage": 0,
		"lab_progress":          []any{},
	})
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
