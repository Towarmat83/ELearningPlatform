// Package definition holds the wire representation of a course or learning
// path: the shape the admin API accepts and returns, and the shape the dev
// seed files are written in.
//
// It lives apart from internal/content (the domain types the rest of the
// service works with) so that the admin API and the seeder cannot drift
// from one another: both go through the conversions below.
package definition

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/genesary/pupitre/course-service/internal/content"
)

// Session is one scheduled in-person session inside a course definition.
// Sessions are keyed by ID in the enclosing map so that retrying a failed
// write overwrites the same slot rather than appending a duplicate.
type Session struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Location string `json:"location,omitempty"`
	Capacity int    `json:"capacity,omitempty"`
}

// Course is the wire representation of a course definition, used for both
// the admin read and write endpoints so that a client can round-trip a
// course it just fetched.
type Course struct {
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
	Sessions      map[string]Session           `json:"sessions,omitempty"`
}

// ToCourse converts a wire definition into the domain course the
// repository persists.
func (d Course) ToCourse(slug string) *content.Course {
	course := &content.Course{
		Slug:          slug,
		Title:         d.Title,
		Description:   d.Description,
		Category:      d.Category,
		Difficulty:    d.Difficulty,
		IsPublic:      d.Public,
		Hidden:        d.Hidden,
		Scope:         d.Scope,
		XPRequired:    d.XPRequired,
		InPerson:      d.InPerson,
		Badge:         d.Badge,
		Modules:       d.Modules,
		Prerequisites: d.Prerequisites,
	}

	for sessionID, session := range d.Sessions {
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

// FromCourse converts a stored course back into its wire definition.
func FromCourse(course *content.Course) Course {
	spec := Course{
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
		spec.Sessions = make(map[string]Session, len(course.Sessions))
		for _, session := range course.Sessions {
			spec.Sessions[session.ID] = Session{
				Title:    session.Title,
				Date:     session.Date,
				Location: session.Location,
				Capacity: session.Capacity,
			}
		}
	}

	return spec
}

// Path is the wire representation of a learning path definition.
type Path struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Level       string   `json:"level,omitempty"`
	Courses     []string `json:"courses,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

// ToPath converts a wire definition into the domain path the repository
// persists.
func (d Path) ToPath(slug string) *content.Path {
	return &content.Path{
		Slug:        slug,
		Title:       d.Title,
		Description: d.Description,
		Kind:        d.Kind,
		Level:       d.Level,
		Courses:     d.Courses,
		Skills:      d.Skills,
	}
}

// errMissingSlug is returned for a definition file that names no slug.
var errMissingSlug = errors.New("course definition has no slug")

// File is a definition read from a YAML file: the slug the resource is
// stored under, plus the definition itself.
type File struct {
	Slug string `json:"slug"`
	Spec Course `json:"spec"`
}

// ParseCourseFile decodes a course definition file.
//
// The file is YAML for authoring comfort — course modules carry long
// markdown bodies that are far more readable as block scalars — but it is
// decoded through JSON so that the `json` tags above are the single
// definition of the wire shape, shared with the admin API.
func ParseCourseFile(data []byte) (File, error) {
	var intermediate any

	err := yaml.Unmarshal(data, &intermediate)
	if err != nil {
		return File{}, fmt.Errorf("parse course YAML: %w", err)
	}

	encoded, err := json.Marshal(intermediate)
	if err != nil {
		return File{}, fmt.Errorf("re-encode course definition: %w", err)
	}

	var file File

	err = json.Unmarshal(encoded, &file)
	if err != nil {
		return File{}, fmt.Errorf("decode course definition: %w", err)
	}

	if file.Slug == "" {
		return File{}, errMissingSlug
	}

	return file, nil
}
