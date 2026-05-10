package content

import (
	"strings"
	"unicode"
)

// Course is the in-memory representation of a course loaded from disk.
type Course struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Difficulty  string   `json:"difficulty"`
	IsPublished bool     `json:"is_published"`
	AutoEnroll  bool     `json:"auto_enroll"`
	Lessons     []Lesson `json:"lessons"`
	Modules     []Module `json:"modules,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// Lesson is a single lesson inside a Course (loaded from NN-slug.md files).
type Lesson struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Content string `json:"content"`
}

// Module is a course element as defined in the CRD spec.modules[].
type Module struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // video, text, image
	Src         string `json:"src,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Path        string `json:"path,omitempty"`
	Replication bool   `json:"replication,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// Slug returns a DNS-compliant slug derived from the module name.
func (m Module) Slug() string {
	var b strings.Builder
	for _, c := range m.Name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' {
			b.WriteRune(c)
		} else if unicode.IsSpace(c) {
			b.WriteRune('-')
		} else {
			b.WriteRune('-')
		}
	}
	return strings.ToLower(strings.Trim(b.String(), "-"))
}

// Content resolves and returns the module content.
// For video/image: returns the Src as a direct URL.
// For text with git: needs FetchModuleContent (returns empty, caller must fetch).
func (m Module) Content() string {
	switch m.Type {
	case "video", "image":
		return m.Src
	default:
		return ""
	}
}

// HasGitContent returns true if this module references a git repo for content.
func (m Module) HasGitContent() bool {
	return m.Type == "text" && m.Src != "" && m.Ref != "" && m.Path != ""
}

// CourseYAML represents the course.yaml file (CRD or flat format).
type CourseYAML struct {
	APIVersion string      `yaml:"apiVersion,omitempty"`
	Kind       string      `yaml:"kind,omitempty"`
	Spec       *CourseSpec `yaml:"spec,omitempty"`

	Title       string       `yaml:"title,omitempty"`
	Description string       `yaml:"description,omitempty"`
	Category    string       `yaml:"category,omitempty"`
	Difficulty  string       `yaml:"difficulty,omitempty"`
	IsPublished bool         `yaml:"is_published,omitempty"`
	AutoEnroll  bool         `yaml:"auto_enroll,omitempty"`
	Hidden      bool         `yaml:"hidden,omitempty"`
	Modules     []ModuleYAML `yaml:"modules,omitempty"`
}

// CourseSpec holds the nested spec content in the CRD format.
type CourseSpec struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	Hidden      bool         `yaml:"hidden"`
	Category    string       `yaml:"category,omitempty"`
	Difficulty  string       `yaml:"difficulty,omitempty"`
	Modules     []ModuleYAML `yaml:"modules"`
}

// ModuleYAML is a module entry in the CRD spec.modules[].
type ModuleYAML struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Src         string `yaml:"src,omitempty"`
	Ref         string `yaml:"ref,omitempty"`
	Path        string `yaml:"path,omitempty"`
	Replication bool   `yaml:"replication,omitempty"`
	Hidden      bool   `yaml:"hidden,omitempty"`
}

// lessonFrontmatter is the YAML front matter in NN-slug.md files.
type lessonFrontmatter struct {
	Title string `yaml:"title"`
}

// MergeSpec merges the CRD format (spec nesting) into the flat fields.
// If Spec is present, flat fields from Spec take priority over top-level.
func (c *CourseYAML) MergeSpec() {
	if c.Spec == nil {
		return
	}
	if c.Title == "" {
		c.Title = c.Spec.Title
	}
	if c.Description == "" {
		c.Description = c.Spec.Description
	}
	if !c.Hidden {
		c.Hidden = c.Spec.Hidden
	}
	if c.Category == "" {
		c.Category = c.Spec.Category
	}
	if c.Difficulty == "" {
		c.Difficulty = c.Spec.Difficulty
	}
	if len(c.Modules) == 0 && len(c.Spec.Modules) > 0 {
		c.Modules = c.Spec.Modules
	}
}
