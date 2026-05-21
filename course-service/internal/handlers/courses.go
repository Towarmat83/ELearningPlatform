package handlers

import (
	"net/http"
	"strings"

	"github.com/elearning/course-service/internal/content"
)

type courseResponse struct {
	Slug            string                      `json:"slug"`
	ID              string                      `json:"id"`
	Title           string                      `json:"title"`
	Description     string                      `json:"description"`
	Category        string                      `json:"category"`
	Difficulty      string                      `json:"difficulty"`
	IsPublished     bool                        `json:"is_published"`
	Prerequisites   []content.CoursePrerequisite `json:"prerequisites,omitempty"`
	ModuleCount     int                         `json:"module_count"`
	LabCount        int                         `json:"lab_count"`
	EnrollmentCount int                         `json:"enrollment_count"`
	Source          string                      `json:"source,omitempty"`
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
		Prerequisites:   c.Prerequisites,
		ModuleCount:     len(c.Modules),
		LabCount:        len(c.Modules),
		EnrollmentCount: 0,
		Source:          c.Source,
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
// @Router   /api/courses [get]
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

// ListAdminCourses godoc
// @Summary   List all courses including hidden (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/courses [get]
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
// @Router    /api/courses/{slug}/labs/{lab_id} [get]
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
// @Router    /api/courses/{slug}/labs [get]
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
// @Router    /api/courses/{slug}/progress [get]
func (s *State) GetCourseProgress(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}

	modules := s.visibleModules(c, r)
	total := len(modules)

	viewedMap := s.viewedLessons(r, courseSlug, claims.Subject)
	progressMap := s.fetchModuleProgress(r, courseSlug, claims.Subject)

	type labProgressEntry struct {
		LabID     string `json:"lab_id"`
		LabTitle  string `json:"lab_title"`
		Type      string `json:"type"`
		Completed bool   `json:"completed"`
		BestScore int    `json:"best_score"`
		MaxScore  int    `json:"max_score"`
		Attempts  int    `json:"attempts"`
	}

	completedCount := 0
	totalPoints := 0
	earnedPoints := 0
	labProgress := make([]labProgressEntry, 0, len(modules))

	for i, m := range modules {
		entry := labProgressEntry{
			LabID:    m.Slug(),
			LabTitle: m.Name,
			Type:     m.Type,
		}
		if m.Type == "quiz" {
			if p, ok := progressMap[i]; ok {
				entry.Completed = p.Passed
				entry.BestScore = p.BestScore
				entry.MaxScore = p.MaxScore
				entry.Attempts = p.Attempts
				totalPoints += p.MaxScore
				earnedPoints += p.BestScore
				if p.Passed {
					completedCount++
				}
			}
		} else {
			if viewedMap[m.Slug()] {
				entry.Completed = true
				completedCount++
			}
		}
		labProgress = append(labProgress, entry)
	}

	pct := 0
	if total > 0 {
		pct = completedCount * 100 / total
	}

	s.JSON(w, http.StatusOK, map[string]any{
		"course_id":             courseSlug,
		"user_id":               claims.Subject,
		"total_labs":            total,
		"completed_labs":        completedCount,
		"total_points_possible": totalPoints,
		"total_points_earned":   earnedPoints,
		"completion_percentage": pct,
		"lab_progress":          labProgress,
	})
}

// GetCourse godoc
// @Summary  Get a course by slug
// @Tags     Courses
// @Produce  json
// @Param    slug  path  string  true  "Course slug"
// @Success  200   {object}  courseResponse
// @Failure  404   {object}  map[string]string
// @Router   /api/courses/{slug} [get]
func (s *State) GetCourse(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	c := s.Content.Get(slug)
	if c == nil || !c.IsPublished {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	s.JSON(w, http.StatusOK, toCourseResponse(c))
}
