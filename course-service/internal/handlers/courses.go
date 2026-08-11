package handlers

import (
	"net/http"
	"slices"
	"strings"

	"github.com/genesary/pupitre/course-service/internal/content"
	"github.com/genesary/pupitre/course-service/internal/middleware"
)

// prerequisiteResponse describes a single course prerequisite in API
// responses.
type prerequisiteResponse struct {
	Course   string   `json:"course"`
	MinScore int      `json:"minScore,omitempty"`
	Modules  []string `json:"modules,omitempty"`
}

// courseResponse is the public API representation of a course.
type courseResponse struct {
	Slug            string                 `json:"slug"`
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Category        string                 `json:"category"`
	Difficulty      string                 `json:"difficulty"`
	IsPublic        bool                   `json:"isPublic"`
	ModuleCount     int                    `json:"moduleCount"`
	LabCount        int                    `json:"labCount"`
	EnrollmentCount int                    `json:"enrollmentCount"`
	Scope           string                 `json:"scope,omitempty"`
	Source          string                 `json:"source,omitempty"`
	Prerequisites   []prerequisiteResponse `json:"prerequisites,omitempty"`
	Skills          []string               `json:"skills,omitempty"`
	Badge           *content.Badge         `json:"badge,omitempty"`
}

// toCourseResponse converts an internal content.Course into the public
// courseResponse shape returned by the API.
func toCourseResponse(course *content.Course) courseResponse {
	prereqs := make([]prerequisiteResponse, 0, len(course.Prerequisites))

	for _, p := range course.Prerequisites {
		prereqs = append(prereqs, prerequisiteResponse{
			Course:   p.Course,
			MinScore: p.MinScore,
			Modules:  p.Modules,
		})
	}

	return courseResponse{
		Slug:            course.Slug,
		ID:              course.Slug,
		Title:           course.Title,
		Description:     course.Description,
		Category:        course.Category,
		Difficulty:      course.Difficulty,
		IsPublic:        course.IsPublic,
		ModuleCount:     len(course.Modules),
		LabCount:        len(course.Modules),
		EnrollmentCount: 0,
		Scope:           course.Scope,
		Source:          course.Source,
		Prerequisites:   prereqs,
		Skills:          course.Skills,
		Badge:           course.Badge,
	}
}

// ListCourses godoc
// @Summary  List published courses
// @Tags     Courses
// @Produce  json
// @Param    category   query  string  false  "Filter by category"
// @Param    difficulty query  string  false  "Filter by difficulty"
// @Param    search     query  string  false  "Search by title or description"
// @Param    skill      query  string  false  "Filter by skill (slug)"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/courses [get].
func (s *State) ListCourses(writer http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	filters := courseFilters{
		category:   strings.ToLower(q.Get("category")),
		difficulty: strings.ToLower(q.Get("difficulty")),
		search:     strings.ToLower(q.Get("search")),
		skill:      strings.ToLower(q.Get("skill")),
	}

	all := s.Content.List()

	out := make([]courseResponse, 0, len(all))
	for _, course := range all {
		if filters.matches(course) {
			out = append(out, toCourseResponse(course))
		}
	}

	s.JSON(writer, http.StatusOK, map[string]any{coursesJSONKey: out, totalJSONKey: len(out)})
}

// courseFilters holds the parsed query parameters for ListCourses.
type courseFilters struct {
	category, difficulty, search, skill string
}

// matches reports whether course passes all active filters.
func (f courseFilters) matches(course *content.Course) bool {
	if f.category != "" && !strings.EqualFold(course.Category, f.category) {
		return false
	}

	if f.difficulty != "" && !strings.EqualFold(course.Difficulty, f.difficulty) {
		return false
	}

	if f.search != "" {
		title := strings.ToLower(course.Title)
		desc := strings.ToLower(course.Description)

		if !strings.Contains(title, f.search) && !strings.Contains(desc, f.search) {
			return false
		}
	}

	if f.skill != "" && !slices.ContainsFunc(course.Skills, func(s string) bool {
		return strings.EqualFold(s, f.skill)
	}) {
		return false
	}

	return true
}

// ListAdminCourses godoc
// @Summary   List all courses including private (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/courses [get].
func (s *State) ListAdminCourses(writer http.ResponseWriter, req *http.Request) {
	all := s.Content.All()

	out := make([]courseResponse, 0, len(all))
	for _, course := range all {
		out = append(out, toCourseResponse(course))
	}

	s.JSON(writer, http.StatusOK, map[string]any{coursesJSONKey: out, totalJSONKey: len(out)})
}

// labResponse is the public API representation of a lab (a lab-type
// module) within a course.
type labResponse struct {
	ID          string `json:"id"`
	CourseID    string `json:"courseId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	LabType     string `json:"labType"`
	ModuleType  string `json:"moduleType"`
	Points      int    `json:"points"`
	OrderIndex  int    `json:"orderIndex"`
	IsPublished bool   `json:"isPublished"`
	Hidden      bool   `json:"hidden"`
}

// moduleTypeToLabType maps an internal module type to the legacy labType
// value expected by the frontend.
func moduleTypeToLabType(moduleType string) string {
	switch moduleType {
	case moduleTypeText:
		return labTypeForm
	case moduleTypeVideo, moduleTypeImage:
		return labTypeInteractive
	default:
		return labTypeInteractive
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
func (s *State) GetLab(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	labID := param(req, "lab_id")

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	for _, mod := range s.visibleModules(course, req) {
		if mod.Slug() == labID {
			s.JSON(writer, http.StatusOK, map[string]any{
				moduleTypeLab: labResponse{
					ID:          mod.Slug(),
					CourseID:    course.Slug,
					Title:       mod.Name,
					Description: "",
					LabType:     moduleTypeToLabType(mod.Type),
					ModuleType:  mod.Type,
					Points:      0,
					OrderIndex:  0,
					IsPublished: true,
					Hidden:      mod.Hidden,
				},
				progressJSONKey: nil,
			})

			return
		}
	}

	s.Error(writer, http.StatusNotFound, "Lab not found")
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
func (s *State) ListLabs(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	modules := s.visibleModules(course, req)

	labs := make([]labResponse, 0, len(modules))
	for pos, mod := range modules {
		labs = append(labs, labResponse{
			ID:          mod.Slug(),
			CourseID:    course.Slug,
			Title:       mod.Name,
			Description: "",
			LabType:     moduleTypeToLabType(mod.Type),
			ModuleType:  mod.Type,
			Points:      0,
			OrderIndex:  pos + 1,
			IsPublished: true,
			Hidden:      mod.Hidden,
		})
	}

	s.JSON(writer, http.StatusOK, map[string]any{"labs": labs})
}

// GetCourseProgress godoc
// @Summary   Get course progress summary
// @Tags      Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/courses/{slug}/progress [get].
func (s *State) GetCourseProgress(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	course := s.Content.Get(courseSlug)

	total := 0
	if course != nil {
		total = len(s.visibleModules(course, req))
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		"courseId":              courseSlug,
		userIDJSONKey:           "",
		"total_labs":            total,
		"completedLabs":         0,
		"total_points_possible": 0,
		"total_points_earned":   0,
		"completion_percentage": 0,
		"lab_progress":          []any{},
	})
}

// canViewPrivateCourse reports whether the requester is authorized to view
// a course that is not public: an admin, or a user enrolled in it.
func (s *State) canViewPrivateCourse(req *http.Request, slug string) bool {
	claims := s.claims(req)
	if claims == nil {
		auth := req.Header.Get("Authorization")

		rest, hasBearer := strings.CutPrefix(auth, "Bearer ")
		if hasBearer {
			parsed, err := middleware.VerifyToken(rest, s.Config.JWTSecret)
			if err == nil {
				claims = parsed
			}
		}
	}

	return claims != nil && (claims.Role == roleAdmin || s.isEnrolled(req, slug, claims.Subject))
}

// GetCourse godoc
// @Summary  Get a course by slug
// @Tags     Courses
// @Produce  json
// @Param    slug  path  string  true  "Course slug"
// @Success  200   {object}  courseResponse
// @Failure  404   {object}  map[string]string
// @Router   /api/courses/{slug} [get].
func (s *State) GetCourse(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	course := s.Content.Get(slug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !course.IsPublic && !s.canViewPrivateCourse(req, slug) {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	s.JSON(writer, http.StatusOK, toCourseResponse(course))
}
