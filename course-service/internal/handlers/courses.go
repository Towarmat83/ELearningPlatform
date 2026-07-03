package handlers

import (
	"net/http"
	"strings"

	"github.com/elearning/course-service/internal/content"
	"github.com/elearning/course-service/internal/middleware"
)

type prerequisiteResponse struct {
	Course   string   `json:"course"`
	MinScore int      `json:"min_score,omitempty"`
	Modules  []string `json:"modules,omitempty"`
}

type courseResponse struct {
	Slug            string                 `json:"slug"`
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Category        string                 `json:"category"`
	Difficulty      string                 `json:"difficulty"`
	IsPublic        bool                   `json:"is_public"`
	ModuleCount     int                    `json:"module_count"`
	LabCount        int                    `json:"lab_count"`
	EnrollmentCount int                    `json:"enrollment_count"`
	Source          string                 `json:"source,omitempty"`
	Prerequisites   []prerequisiteResponse `json:"prerequisites,omitempty"`
}

func toCourseResponse(c *content.Course) courseResponse {
	var prereqs []prerequisiteResponse
	for _, p := range c.Prerequisites {
		prereqs = append(prereqs, prerequisiteResponse{
			Course:   p.Course,
			MinScore: p.MinScore,
			Modules:  p.Modules,
		})
	}

	return courseResponse{
		Slug:            c.Slug,
		ID:              c.Slug,
		Title:           c.Title,
		Description:     c.Description,
		Category:        c.Category,
		Difficulty:      c.Difficulty,
		IsPublic:        c.IsPublic,
		ModuleCount:     len(c.Modules),
		LabCount:        len(c.Modules),
		EnrollmentCount: 0,
		Source:          c.Source,
		Prerequisites:   prereqs,
	}
}

// ListCourses godoc
// @Summary  List published courses
// @Tags     Courses
// @Produce  json
// @Param    category   query  string  false  "Filter by category"
// @Param    difficulty query  string  false  "Filter by difficulty"
// @Param    search     query  string  false  "Search by title or description"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/courses [get].
func (s *State) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := strings.ToLower(q.Get("category"))
	difficulty := strings.ToLower(q.Get("difficulty"))
	search := strings.ToLower(q.Get("search"))

	all := s.Content.List()

	out := make([]courseResponse, 0, len(all))
	for _, c := range all {
		if category != "" && !strings.EqualFold(c.Category, category) {
			continue
		}

		if difficulty != "" && !strings.EqualFold(c.Difficulty, difficulty) {
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

// ListAdminCourses godoc
// @Summary   List all courses including private (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/courses [get].
func (s *State) ListAdminCourses(w http.ResponseWriter, r *http.Request) {
	all := s.Content.All()

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
	ModuleType  string `json:"module_type"`
	Points      int    `json:"points"`
	OrderIndex  int    `json:"order_index"`
	IsPublished bool   `json:"is_published"`
	Hidden      bool   `json:"hidden"`
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

// GetLab godoc
// @Summary   Get a single lab
// @Tags      Labs
// @Security  BearerAuth
// @Produce   json
// @Param     slug    path  string  true  "Course slug"
// @Param     lab_id  path  string  true  "Lab ID"
// @Success   200  {object}  map[string]interface{}
// @Failure   404  {object}  map[string]string
// @Router    /api/courses/{slug}/labs/{lab_id} [get].
func (s *State) GetLab(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	labID := param(r, "lab_id")

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")

		return
	}

	for _, m := range s.visibleModules(c, r) {
		if m.Slug() == labID {
			s.JSON(w, http.StatusOK, map[string]any{
				"lab": labResponse{
					ID:          m.Slug(),
					CourseID:    c.Slug,
					Title:       m.Name,
					Description: "",
					LabType:     moduleTypeToLabType(m.Type),
					ModuleType:  m.Type,
					Points:      0,
					OrderIndex:  0,
					IsPublished: true,
					Hidden:      m.Hidden,
				},
				"progress": nil,
			})

			return
		}
	}

	s.Error(w, http.StatusNotFound, "Lab not found")
}

// ListLabs godoc
// @Summary   List labs for a course
// @Tags      Labs
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200  {object}  map[string]interface{}
// @Failure   404  {object}  map[string]string
// @Router    /api/courses/{slug}/labs [get].
func (s *State) ListLabs(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")

		return
	}

	modules := s.visibleModules(c, r)

	labs := make([]labResponse, 0, len(modules))
	for i, m := range modules {
		labs = append(labs, labResponse{
			ID:          m.Slug(),
			CourseID:    c.Slug,
			Title:       m.Name,
			Description: "",
			LabType:     moduleTypeToLabType(m.Type),
			ModuleType:  m.Type,
			Points:      0,
			OrderIndex:  i + 1,
			IsPublished: true,
			Hidden:      m.Hidden,
		})
	}

	s.JSON(w, http.StatusOK, map[string]any{"labs": labs})
}

// GetCourseProgress godoc
// @Summary   Get course progress summary
// @Tags      Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/courses/{slug}/progress [get].
func (s *State) GetCourseProgress(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	c := s.Content.Get(courseSlug)

	total := 0
	if c != nil {
		total = len(s.visibleModules(c, r))
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

// GetCourse godoc
// @Summary  Get a course by slug
// @Tags     Courses
// @Produce  json
// @Param    slug  path  string  true  "Course slug"
// @Success  200   {object}  courseResponse
// @Failure  404   {object}  map[string]string
// @Router   /api/courses/{slug} [get].
func (s *State) GetCourse(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	c := s.Content.Get(slug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")

		return
	}

	if !c.IsPublic {
		// Try to extract claims from Authorization header (route has no auth middleware).
		claims := s.claims(r)
		if claims == nil {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				parsed, err := middleware.VerifyToken(strings.TrimPrefix(auth, "Bearer "), s.Config.JWTSecret)
				if err == nil {
					claims = parsed
				}
			}
		}

		if claims == nil || (claims.Role != "admin" && !s.isEnrolled(r, slug, claims.Subject)) {
			s.Error(w, http.StatusNotFound, "Course not found")

			return
		}
	}

	s.JSON(w, http.StatusOK, toCourseResponse(c))
}
