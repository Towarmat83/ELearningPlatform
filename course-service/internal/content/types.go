package content

import (
	"strings"
	"unicode"
)

// Module type identifiers used across course content, quiz, and Kubernetes
// CRD handling.
const (
	moduleTypeVideo = "video"
	moduleTypeImage = "image"
	moduleTypeLab   = "lab"
	moduleTypeText  = "text"
	moduleTypeQuiz  = "quiz"
)

// difficultyMedium is the default question/course difficulty.
const difficultyMedium = "medium"

// Path is a sequential learning path composed of course slugs or skill tags.
type Path struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`              // "course" | "skill"
	Level       string   `json:"level,omitempty"`   // "beginner" | "intermediate" | "advanced"
	Courses     []string `json:"courses,omitempty"` // ordered — index N+1 unlocks after N completed
	Skills      []string `json:"skills,omitempty"`  // ordered skill tags (kind=skill paths)
	Source      string   `json:"source,omitempty"`
}

// CoursePrerequisite describes one condition to be met before enrolling.
// If only Course is set, enrollment in that course is required.
// If MinScore > 0, the user must have earned at least that many points.
// If Modules is non-empty, every listed module slug must be passed.
type CoursePrerequisite struct {
	Course string `json:"course" yaml:"course"`

	MinScore int      `json:"minScore,omitempty" yaml:"minScore,omitempty"`
	Modules  []string `json:"modules,omitempty"  yaml:"modules,omitempty"`
}

// Badge is the reward earned when a learner completes a course.
type Badge struct {
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
}

// Course is the in-memory representation of a course loaded from disk.
type Course struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`

	IsPublic      bool                 `json:"isPublic"`
	Prerequisites []CoursePrerequisite `json:"prerequisites,omitempty"`
	Lessons       []Lesson             `json:"lessons"`
	Modules       []Module             `json:"modules,omitempty"`
	Skills        []string             `json:"skills,omitempty"` // aggregated from all modules
	Scope         string               `json:"scope,omitempty"`
	Badge         *Badge               `json:"badge,omitempty"`
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
	Name string `json:"name"`
	Type string `json:"type"` // video, text, image, quiz, lab
	Src  string `json:"src,omitempty"`
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`

	LabURL        string     `json:"labUrl,omitempty"`
	InlineContent string     `json:"content,omitempty"`
	Replication   bool       `json:"replication,omitempty"`
	Hidden        bool       `json:"hidden,omitempty"`
	Inline        bool       `json:"inline,omitempty"`        // quiz rendered inside the previous module
	Prerequisites []string   `json:"prerequisites,omitempty"` // module slugs that must be completed first
	Questions     []Question `json:"questions,omitempty"`

	PassingScore int          `json:"passingScore,omitempty"`
	Cooldown     CooldownSpec `json:"cooldown,omitempty"`

	MaxAttemptsPerQuestion *int `json:"maxAttemptsPerQuestion,omitempty"`

	LockOnMaxAttempts bool `json:"lockOnMaxAttempts,omitempty"`

	CheckProvider string `json:"checkProvider,omitempty"` // "local" | "gitlab"

	CheckType string `json:"checkType,omitempty"`

	CheckParams map[string]any `json:"checkParams,omitempty"`
	Steps       []CheckStep    `json:"steps,omitempty"`
	Skills      []string       `json:"skills,omitempty"` // competency tags this module teaches
}

// CheckStep is one verifiable step inside a lab module.
type CheckStep struct {
	Title string `json:"title" yaml:"title"`

	CheckType string `json:"checkType" yaml:"checkType"`

	CheckParams map[string]any `json:"checkParams,omitempty" yaml:"checkParams,omitempty"`
}

// Slug returns a DNS-compliant slug derived from the module name.
func (m Module) Slug() string {
	var builder strings.Builder

	for _, c := range m.Name {
		switch {
		case unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_':
			builder.WriteRune(c)
		default:
			builder.WriteRune('-')
		}
	}

	return strings.ToLower(strings.Trim(builder.String(), "-"))
}

// Content resolves and returns the module content.
// video/image: returns Src, a direct URL.
// lab without inline content: returns Src, an external lab URL.
// text/quiz with git: needs FetchModuleContent (returns empty here, caller
// must fetch it separately).
func (m Module) Content() string {
	switch m.Type {
	case moduleTypeVideo, moduleTypeImage:
		return m.Src
	case moduleTypeLab:
		if m.InlineContent != "" {
			return m.InlineContent
		}

		return m.Src
	default:
		return m.InlineContent
	}
}

// HasQuestions returns true if the module has inline quiz questions.
func (m Module) HasQuestions() bool {
	return m.Type == moduleTypeQuiz && len(m.Questions) > 0
}

// HasGitContent returns true if the module references git repo content.
func (m Module) HasGitContent() bool {
	return m.Src != "" && m.Ref != "" && m.Path != ""
}

// BadgeYAML is the badge definition in the flat course.yaml format.
type BadgeYAML struct {
	Name string `yaml:"name"`
	Icon string `yaml:"icon,omitempty"`
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
	Badge         *BadgeYAML           `yaml:"badge,omitempty"`
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
	Badge         *BadgeYAML           `yaml:"badge,omitempty"`
}

// ModuleYAML is a module entry in the CRD spec.modules[].
type ModuleYAML struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Src  string `yaml:"src,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
	Path string `yaml:"path,omitempty"`

	LabURL        string         `yaml:"labUrl,omitempty"`
	InlineContent string         `yaml:"content,omitempty"`
	Replication   bool           `yaml:"replication,omitempty"`
	Hidden        bool           `yaml:"hidden,omitempty"`
	Inline        bool           `yaml:"inline,omitempty"`
	Prerequisites []string       `yaml:"prerequisites,omitempty"`
	Questions     []QuestionYAML `yaml:"questions,omitempty"`

	PassingScore int              `yaml:"passingScore"`
	Cooldown     CooldownSpecYAML `yaml:"cooldown"`

	MaxAttemptsPerQuestion *int `yaml:"maxAttemptsPerQuestion"`

	LockOnMaxAttempts bool `yaml:"lockOnMaxAttempts"`

	CheckProvider string `yaml:"checkProvider,omitempty"`

	CheckType string `yaml:"checkType,omitempty"`

	CheckParams map[string]any `yaml:"checkParams,omitempty"`
	Steps       []CheckStep    `yaml:"steps,omitempty"`
	Skills      []string       `yaml:"skills,omitempty"`
}

// ModuleIndexEntry is one entry in a module index YAML file (type: modules).
type ModuleIndexEntry struct {
	Name          string   `yaml:"name"`
	Type          string   `yaml:"type,omitempty"`
	Src           string   `yaml:"src,omitempty"`
	Ref           string   `yaml:"ref,omitempty"`
	Path          string   `yaml:"path"`
	Hidden        bool     `yaml:"hidden,omitempty"`
	Prerequisites []string `yaml:"prerequisites,omitempty"`
}

// lessonFrontmatter is the YAML front matter in NN-slug.md files.
type lessonFrontmatter struct {
	Title string `yaml:"title"`
}

// mergeIfZero sets *dst to src when *dst is the zero value for T.
func mergeIfZero[T comparable](dst *T, src T) {
	var zero T
	if *dst == zero {
		*dst = src
	}
}

// mergeIfEmpty sets *dst to src when *dst is empty and src is not.
func mergeIfEmpty[T any](dst *[]T, src []T) {
	if len(*dst) == 0 && len(src) > 0 {
		*dst = src
	}
}

// MergeSpec merges the CRD format (spec nesting) into the flat fields.
// If Spec is present, flat fields from Spec take priority over top-level.
func (c *CourseYAML) MergeSpec() {
	if c.Spec == nil {
		return
	}

	mergeIfZero(&c.Title, c.Spec.Title)
	mergeIfZero(&c.Description, c.Spec.Description)

	if !c.Public {
		c.Public = c.Spec.Public
	}

	mergeIfZero(&c.Category, c.Spec.Category)
	mergeIfZero(&c.Difficulty, c.Spec.Difficulty)
	mergeIfEmpty(&c.Prerequisites, c.Spec.Prerequisites)
	mergeIfEmpty(&c.Modules, c.Spec.Modules)

	if c.Badge == nil && c.Spec.Badge != nil {
		c.Badge = c.Spec.Badge
	}
}
