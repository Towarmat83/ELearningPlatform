package content

import (
	"strings"
	"unicode"
)

// CoursePrerequisite declares that the user must have completed another course
// before accessing modules in this one.
type CoursePrerequisite struct {
	// Course is the slug of the course that must be completed first.
	Course string `json:"course" yaml:"course"`
	// MinScore is the minimum total score (sum of best_score across modules) the
	// user must have accumulated in the prerequisite course. 0 means no score
	// requirement (any progress suffices when combined with Modules or alone).
	MinScore int `json:"min_score,omitempty" yaml:"min_score,omitempty"`
	// Modules lists specific module slugs in the prerequisite course that must
	// have been passed (quiz passed = true). Empty means no specific modules are
	// required.
	Modules []string `json:"modules,omitempty" yaml:"modules,omitempty"`
}

// Course is the in-memory representation of a course loaded from disk.
type Course struct {
	Slug          string               `json:"slug"`
	Title         string               `json:"title"`
	Description   string               `json:"description"`
	Category      string               `json:"category"`
	Difficulty    string               `json:"difficulty"`
	IsPublic      bool                 `json:"is_public"`
	Prerequisites []CoursePrerequisite `json:"prerequisites,omitempty"`
	Lessons       []Lesson             `json:"lessons"`
	Modules       []Module             `json:"modules,omitempty"`
	Source        string               `json:"source,omitempty"`
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
	Name                   string       `json:"name"`
	Type                   string       `json:"type"` // video, text, image, quiz
	Src                    string       `json:"src,omitempty"`
	Ref                    string       `json:"ref,omitempty"`
	Path                   string       `json:"path,omitempty"`
	Replication            bool         `json:"replication,omitempty"`
	Hidden                 bool         `json:"hidden,omitempty"`
	Prerequisites          []string     `json:"prerequisites,omitempty"` // module slugs that must be completed first
	Questions              []Question   `json:"questions,omitempty"`
	PassingScore           int          `json:"passing_score,omitempty"`
	Cooldown               CooldownSpec `json:"cooldown,omitempty"`
	MaxAttemptsPerQuestion *int         `json:"max_attempts_per_question,omitempty"`
	LockOnMaxAttempts      bool         `json:"lock_on_max_attempts,omitempty"`
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
// For text/quiz with git: needs FetchModuleContent (returns empty, caller must fetch).
func (m Module) Content() string {
	switch m.Type {
	case "video", "image":
		return m.Src
	default:
		return ""
	}
}

// HasQuestions returns true if this module has inline quiz questions.
func (m Module) HasQuestions() bool {
	return m.Type == "quiz" && len(m.Questions) > 0
}

// HasGitContent returns true if this module references a git repo for content.
func (m Module) HasGitContent() bool {
	return m.Src != "" && m.Ref != "" && m.Path != ""
}

// CourseYAML represents the course.yaml file (CRD or flat format).
type CourseYAML struct {
	APIVersion string      `yaml:"apiVersion,omitempty"`
	Kind       string      `yaml:"kind,omitempty"`
	Spec       *CourseSpec `yaml:"spec,omitempty"`

	Title         string               `yaml:"title,omitempty"`
	Description   string               `yaml:"description,omitempty"`
	Category      string               `yaml:"category,omitempty"`
	Difficulty    string               `yaml:"difficulty,omitempty"`
	Public        bool                 `yaml:"public,omitempty"`
	Prerequisites []CoursePrerequisite `yaml:"prerequisites,omitempty"`
	Modules       []ModuleYAML         `yaml:"modules,omitempty"`
}

// CourseSpec holds the nested spec content in the CRD format.
type CourseSpec struct {
	Title         string               `yaml:"title"`
	Description   string               `yaml:"description"`
	Public        bool                 `yaml:"public"`
	Category      string               `yaml:"category,omitempty"`
	Difficulty    string               `yaml:"difficulty,omitempty"`
	Prerequisites []CoursePrerequisite `yaml:"prerequisites,omitempty"`
	Modules       []ModuleYAML         `yaml:"modules"`
}

// ModuleYAML is a module entry in the CRD spec.modules[].
type ModuleYAML struct {
	Name                   string           `yaml:"name"`
	Type                   string           `yaml:"type"`
	Src                    string           `yaml:"src,omitempty"`
	Ref                    string           `yaml:"ref,omitempty"`
	Path                   string           `yaml:"path,omitempty"`
	Replication            bool             `yaml:"replication,omitempty"`
	Hidden                 bool             `yaml:"hidden,omitempty"`
	Questions              []QuestionYAML   `yaml:"questions,omitempty"`
	PassingScore           int              `yaml:"passing_score"`
	Cooldown               CooldownSpecYAML `yaml:"cooldown"`
	MaxAttemptsPerQuestion *int             `yaml:"max_attempts_per_question"`
	LockOnMaxAttempts      bool             `yaml:"lock_on_max_attempts"`
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
	if !c.Public {
		c.Public = c.Spec.Public
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
	if len(c.Prerequisites) == 0 && len(c.Spec.Prerequisites) > 0 {
		c.Prerequisites = c.Spec.Prerequisites
	}
}
