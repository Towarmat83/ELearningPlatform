package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/course-service/internal/content"
)

// courseSessionBody is one scheduled session inside a course definition.
// Sessions are keyed by ID in the request body so that retrying a failed
// write overwrites the same slot rather than appending a duplicate.
type courseSessionBody struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Location string `json:"location,omitempty"`
	Capacity int    `json:"capacity,omitempty"`
}

// courseSpecBody is the wire representation of a course definition, used
// for both the admin read and write endpoints so that the frontend can
// round-trip a course it just fetched.
type courseSpecBody struct {
	Title         string                       `json:"title,omitempty"`
	Description   string                       `json:"description,omitempty"`
	Public        bool                         `json:"public,omitempty"`
	Hidden        bool                         `json:"hidden,omitempty"`
	Category      string                       `json:"category,omitempty"`
	Difficulty    string                       `json:"difficulty,omitempty"`
	Scope         string                       `json:"scope,omitempty"`
	XPRequired    int                          `json:"xpRequired,omitempty"`
	InPerson      bool                         `json:"inPerson,omitempty"`
	Badge         *content.Badge               `json:"badge,omitempty"`
	Modules       []content.Module             `json:"modules,omitempty"`
	Prerequisites []content.CoursePrerequisite `json:"prerequisites,omitempty"`
	Sessions      map[string]courseSessionBody `json:"sessions,omitempty"`
}

// courseMessages is the client-facing wording for course definitions.
//
//nolint:gochecknoglobals // static response wording, read-only
var courseMessages = definitionMessages{
	Kind:         "course",
	InvalidSpec:  "Invalid course spec",
	Conflict:     "Course already exists",
	NotFound:     courseNotFoundMessage,
	CreateFailed: "Failed to create course",
	UpdateFailed: "Failed to update course",
}

// courseFromSpec converts a wire definition into the domain course the
// repository persists.
func courseFromSpec(slug string, spec courseSpecBody) *content.Course {
	course := &content.Course{
		Slug:          slug,
		Title:         spec.Title,
		Description:   spec.Description,
		Category:      spec.Category,
		Difficulty:    spec.Difficulty,
		IsPublic:      spec.Public,
		Hidden:        spec.Hidden,
		Scope:         spec.Scope,
		XPRequired:    spec.XPRequired,
		InPerson:      spec.InPerson,
		Badge:         spec.Badge,
		Modules:       spec.Modules,
		Prerequisites: spec.Prerequisites,
	}

	for sessionID, session := range spec.Sessions {
		course.Sessions = append(course.Sessions, content.Session{
			ID:       sessionID,
			Title:    session.Title,
			Date:     session.Date,
			Location: session.Location,
			Capacity: session.Capacity,
		})
	}

	// Map iteration order is random; sort so a course written twice from
	// the same payload produces the same rows.
	sort.Slice(course.Sessions, func(i, j int) bool {
		return course.Sessions[i].ID < course.Sessions[j].ID
	})

	return course
}

// specFromCourse converts a stored course back into its wire definition.
func specFromCourse(course *content.Course) courseSpecBody {
	spec := courseSpecBody{
		Title:         course.Title,
		Description:   course.Description,
		Public:        course.IsPublic,
		Hidden:        course.Hidden,
		Category:      course.Category,
		Difficulty:    course.Difficulty,
		Scope:         course.Scope,
		XPRequired:    course.XPRequired,
		InPerson:      course.InPerson,
		Badge:         course.Badge,
		Modules:       course.Modules,
		Prerequisites: course.Prerequisites,
	}

	if len(course.Sessions) > 0 {
		spec.Sessions = make(map[string]courseSessionBody, len(course.Sessions))
		for _, session := range course.Sessions {
			spec.Sessions[session.ID] = courseSessionBody{
				Title:    session.Title,
				Date:     session.Date,
				Location: session.Location,
				Capacity: session.Capacity,
			}
		}
	}

	return spec
}

// CreateCourse godoc
// @Summary  Create a course (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses [post].
func (s *State) CreateCourse(writer http.ResponseWriter, req *http.Request) {
	createDefinition(s, writer, req, courseMessages, courseFromSpec, s.Repos.Courses.Create)
}

// GetCourseDefinition godoc
// @Summary  Get a course's full definition (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/definition [get].
func (s *State) GetCourseDefinition(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	course, err := s.Repos.Courses.Get(req.Context(), slug)
	if err != nil {
		s.writeRepoError(writer, err, courseNotFoundMessage, "load course", zap.String("slug", slug))

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		slugJSONKey: slug,
		"spec":      specFromCourse(course),
	})
}

// UpdateCourse godoc
// @Summary  Replace a course definition (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/definition [put].
func (s *State) UpdateCourse(writer http.ResponseWriter, req *http.Request) {
	updateDefinition(s, writer, req, courseMessages, courseFromSpec, s.courseExists, s.Repos.Courses.Upsert)
}

// DeleteCourse godoc
// @Summary  Delete a course (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/definition [delete].
func (s *State) DeleteCourse(writer http.ResponseWriter, req *http.Request) {
	deleteDefinition(s, writer, req, courseMessages, "Course deleted", s.Repos.Courses.Delete)
}

// courseExists reports whether slug names a stored course, returning
// repository.ErrNotFound when it does not.
func (s *State) courseExists(ctx context.Context, slug string) error {
	_, err := s.Repos.Courses.Get(ctx, slug)
	if err != nil {
		return fmt.Errorf("look up course %s: %w", slug, err)
	}

	return nil
}
