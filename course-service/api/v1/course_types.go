package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CoursePrerequisite describes one condition that must be met before enrolling.
// If only Course is set, enrollment in that course is required.
// If MinScore > 0, the user must have earned at least that many total points.
// If Modules is non-empty, every listed module slug must have been passed.
type CoursePrerequisite struct {
	Course   string   `json:"course"`
	MinScore int      `json:"min_score,omitempty"`
	Modules  []string `json:"modules,omitempty"`
}

// CooldownSpec configures the retry cooldown for a quiz-type module.
type CooldownSpec struct {
	Strategy    string  `json:"strategy,omitempty"`
	BaseSeconds int     `json:"base_seconds,omitempty"`
	Multiplier  float64 `json:"multiplier,omitempty"`
	MaxSeconds  int     `json:"max_seconds,omitempty"`
}

// Answer is one candidate answer of a single/multiple-choice question.
type Answer struct {
	ID      string `json:"id,omitempty"`
	Text    string `json:"text,omitempty"`
	Correct bool   `json:"correct,omitempty"`
}

// OrderItem is one item of an ordering question.
type OrderItem struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

// PartialScoring configures partial credit for a question.
type PartialScoring struct {
	Enabled       bool `json:"enabled,omitempty"`
	AllowNegative bool `json:"allow_negative,omitempty"`
}

// SourceRef points to the course/module/anchor backing a feedback message.
type SourceRef struct {
	Course   string `json:"course,omitempty"`
	Module   string `json:"module,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// Feedback holds the messages shown for a right/wrong answer.
type Feedback struct {
	Wrong      string      `json:"wrong,omitempty"`
	Correct    string      `json:"correct,omitempty"`
	SourceRefs []SourceRef `json:"source_refs,omitempty"`
}

// Question is one inline quiz question of a quiz-type module.
type Question struct {
	ID             string          `json:"id,omitempty"`
	Type           string          `json:"type,omitempty"`
	Difficulty     string          `json:"difficulty,omitempty"`
	Points         int             `json:"points,omitempty"`
	Question       string          `json:"question,omitempty"`
	Answers        []Answer        `json:"answers,omitempty"`
	CorrectAnswer  *bool           `json:"correct_answer,omitempty"`
	Items          []OrderItem     `json:"items,omitempty"`
	CorrectOrder   []string        `json:"correct_order,omitempty"`
	PartialScoring *PartialScoring `json:"partial_scoring,omitempty"`
	Feedback       Feedback        `json:"feedback,omitempty"`
}

// CheckStep is one verifiable step inside a lab module.
type CheckStep struct {
	Title     string `json:"title,omitempty"`
	CheckType string `json:"check_type,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	CheckParams *runtime.RawExtension `json:"check_params,omitempty"`
}

// Module is a course element as defined in spec.modules[].
type Module struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Enum=video;text;image;quiz;lab;modules
	Type          string   `json:"type,omitempty"`
	Src           string   `json:"src,omitempty"`
	Ref           string   `json:"ref,omitempty"`
	Path          string   `json:"path,omitempty"`
	LabURL        string   `json:"lab_url,omitempty"`
	InlineContent string   `json:"content,omitempty"`
	Replication   bool     `json:"replication,omitempty"`
	Hidden        bool     `json:"hidden,omitempty"`
	// Inline: when true (quiz type only), rendered inline at the bottom of the previous module.
	Inline                 bool         `json:"inline,omitempty"`
	Prerequisites          []string     `json:"prerequisites,omitempty"`
	Questions              []Question   `json:"questions,omitempty"`
	PassingScore           int           `json:"passing_score,omitempty"`
	Cooldown               *CooldownSpec `json:"cooldown,omitempty"`
	MaxAttemptsPerQuestion *int          `json:"max_attempts_per_question,omitempty"`
	LockOnMaxAttempts      bool         `json:"lock_on_max_attempts,omitempty"`
	CheckProvider          string       `json:"check_provider,omitempty"`
	CheckType              string       `json:"check_type,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	CheckParams *runtime.RawExtension `json:"check_params,omitempty"`
	Steps       []CheckStep           `json:"steps,omitempty"`
	QuizRef     string                `json:"quiz_ref,omitempty"`
}

// CourseSpec defines the desired state of a Course.
type CourseSpec struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Public: when true, the course is publicly visible and users are auto-enrolled on first access.
	Public bool `json:"public,omitempty"`
	// Hidden: when true, the course is hidden from the catalog.
	Hidden        bool                 `json:"hidden,omitempty"`
	Category      string               `json:"category,omitempty"`
	Difficulty    string               `json:"difficulty,omitempty"`
	Modules       []Module             `json:"modules,omitempty"`
	Prerequisites []CoursePrerequisite `json:"prerequisites,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=course
// +kubebuilder:printcolumn:name="Title",type=string,JSONPath=`.spec.title`
// +kubebuilder:printcolumn:name="Category",type=string,JSONPath=`.spec.category`
// +kubebuilder:printcolumn:name="Public",type=boolean,JSONPath=`.spec.public`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Course is the Schema for the courses API.
type Course struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CourseSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// CourseList contains a list of Course.
type CourseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Course `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Course{}, &CourseList{})
}
