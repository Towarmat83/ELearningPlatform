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
	// Course is the slug of the course that must be completed first.
	// +kubebuilder:validation:MinLength=1
	Course string `json:"course"`
	// MinScore is the minimum total quiz score (in points) required in Course.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	MinScore int `json:"minScore,omitempty"`
	// Modules restricts the requirement to specific module slugs within Course,
	// instead of the whole course.
	// +kubebuilder:validation:Optional
	Modules []string `json:"modules,omitempty"`
}

// CooldownSpec configures the retry cooldown for a quiz-type module.
type CooldownSpec struct {
	// Strategy is the backoff strategy applied between attempts: "fixed" keeps a
	// constant delay, "linear" grows the delay by BaseSeconds per attempt, and
	// "exponential" grows it by Multiplier per attempt (capped at MaxSeconds).
	// +kubebuilder:validation:Enum=fixed;linear;exponential
	// +kubebuilder:default=exponential
	// +kubebuilder:validation:Optional
	Strategy string `json:"strategy,omitempty"`
	// BaseSeconds is the initial cooldown duration, in seconds, after the first failed attempt.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	BaseSeconds int `json:"baseSeconds,omitempty"`
	// Multiplier scales BaseSeconds after each subsequent failed attempt (exponential strategy only).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	Multiplier float64 `json:"multiplier,omitempty"`
	// MaxSeconds caps the cooldown duration regardless of how many attempts have failed.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	MaxSeconds int `json:"maxSeconds,omitempty"`
}

// Answer is one candidate answer of a single/multiple-choice question.
type Answer struct {
	// ID uniquely identifies this answer within its question.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`
	// Text is the answer text shown to the user.
	// +kubebuilder:validation:MinLength=1
	Text string `json:"text"`
	// Correct marks this answer as a correct choice.
	// +kubebuilder:validation:Optional
	Correct bool `json:"correct,omitempty"`
}

// OrderItem is one item of an ordering question.
type OrderItem struct {
	// ID uniquely identifies this item within its question.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`
	// Text is the item text shown to the user.
	// +kubebuilder:validation:MinLength=1
	Text string `json:"text"`
}

// PartialScoring configures partial credit for a question.
type PartialScoring struct {
	// Enabled turns on partial credit for this question.
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled,omitempty"`
	// AllowNegative allows the partial score to go below zero for this question.
	// +kubebuilder:validation:Optional
	AllowNegative bool `json:"allowNegative,omitempty"`
}

// SourceRef points to the course/module/anchor backing a feedback message.
type SourceRef struct {
	// Course is the slug of the course this reference points to.
	// +kubebuilder:validation:Optional
	Course string `json:"course,omitempty"`
	// Module is the slug of the module this reference points to.
	// +kubebuilder:validation:Optional
	Module string `json:"module,omitempty"`
	// Anchor is an optional in-page anchor within the referenced module.
	// +kubebuilder:validation:Optional
	Anchor string `json:"anchor,omitempty"`
	// Priority orders multiple source refs; lower values are shown first.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	Priority int `json:"priority,omitempty"`
}

// Feedback holds the messages shown for a right/wrong answer.
type Feedback struct {
	// Wrong is the message shown when the answer is incorrect.
	// +kubebuilder:validation:Optional
	Wrong string `json:"wrong,omitempty"`
	// Correct is the message shown when the answer is correct.
	// +kubebuilder:validation:Optional
	Correct string `json:"correct,omitempty"`
	// SourceRefs point back to the course material covering this question.
	// +kubebuilder:validation:Optional
	SourceRefs []SourceRef `json:"sourceRefs,omitempty"`
}

// Question is one inline quiz question of a quiz-type module.
type Question struct {
	// ID uniquely identifies this question within its module; auto-generated
	// from the question's position if omitted.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`
	// Type is the question kind.
	// +kubebuilder:validation:Enum=single;multiple;boolean;order
	Type string `json:"type"`
	// Difficulty labels this question for display/filtering purposes.
	// +kubebuilder:default=medium
	// +kubebuilder:validation:Optional
	Difficulty string `json:"difficulty,omitempty"`
	// Points is how many points a correct answer to this question is worth.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +kubebuilder:validation:Optional
	Points int `json:"points,omitempty"`
	// Question is the question text shown to the user.
	// +kubebuilder:validation:MinLength=1
	Question string `json:"question"`
	// Answers lists the candidate answers for single/multiple-choice questions.
	// +kubebuilder:validation:Optional
	Answers []Answer `json:"answers,omitempty"`
	// CorrectAnswer is the expected answer for boolean questions.
	// +kubebuilder:validation:Optional
	CorrectAnswer *bool `json:"correctAnswer,omitempty"`
	// Items lists the elements to be ordered for order-type questions.
	// +kubebuilder:validation:Optional
	Items []OrderItem `json:"items,omitempty"`
	// CorrectOrder lists the item IDs in their correct order, for order-type questions.
	// +kubebuilder:validation:Optional
	CorrectOrder []string `json:"correctOrder,omitempty"`
	// PartialScoring configures partial credit for this question.
	// +kubebuilder:validation:Optional
	PartialScoring *PartialScoring `json:"partialScoring,omitempty"`
	// Feedback holds the right/wrong messages shown after answering.
	// +kubebuilder:validation:Optional
	Feedback Feedback `json:"feedback,omitempty"`
}

// CheckStep is one verifiable step inside a lab module.
type CheckStep struct {
	// Title is the human-readable name of this step.
	// +kubebuilder:validation:MinLength=1
	Title string `json:"title"`
	// CheckType selects how this step is verified (provider-specific).
	// +kubebuilder:validation:MinLength=1
	CheckType string `json:"checkType"`
	// CheckParams holds provider-specific parameters for verifying this step.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Optional
	CheckParams *runtime.RawExtension `json:"checkParams,omitempty"`
}

// Module is a course element as defined in spec.modules[].
type Module struct {
	// Name is the module's display name; also used to derive its slug.
	// Auto-generated from the module's position if omitted.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// Type selects how this module is rendered and consumed.
	// +kubebuilder:validation:Enum=video;text;image;quiz;lab;modules
	// +kubebuilder:default=text
	// +kubebuilder:validation:Optional
	Type string `json:"type,omitempty"`
	// Src is the git repository URL (or direct URL for video/image modules) this module's content is fetched from.
	// +kubebuilder:validation:Optional
	Src string `json:"src,omitempty"`
	// Ref is the git branch, tag, or commit to fetch content from.
	// +kubebuilder:validation:Optional
	Ref string `json:"ref,omitempty"`
	// Path is the file path within the git repository holding this module's content.
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`
	// LabURL is the URL of the lab environment for lab-type modules.
	// +kubebuilder:validation:Optional
	LabURL string `json:"labUrl,omitempty"`
	// InlineContent is the module content provided directly in the spec instead of fetched from git.
	// +kubebuilder:validation:Optional
	InlineContent string `json:"content,omitempty"`
	// Replication marks the content as safe to serve from a per-user replicated copy.
	// +kubebuilder:validation:Optional
	Replication bool `json:"replication,omitempty"`
	// Hidden hides this module from the course's navigation while still allowing direct access.
	// +kubebuilder:validation:Optional
	Hidden bool `json:"hidden,omitempty"`
	// Inline: when true (quiz type only), rendered inline at the bottom of the previous module.
	// +kubebuilder:validation:Optional
	Inline bool `json:"inline,omitempty"`
	// Prerequisites lists module slugs that must be completed before this module unlocks.
	// +kubebuilder:validation:Optional
	Prerequisites []string `json:"prerequisites,omitempty"`
	// Questions lists the inline quiz questions for quiz-type modules.
	// +kubebuilder:validation:Optional
	Questions []Question `json:"questions,omitempty"`
	// PassingScore is the minimum percentage score required to pass a quiz-type module.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Optional
	PassingScore int `json:"passingScore,omitempty"`
	// Cooldown configures the retry delay after a failed quiz attempt.
	// +kubebuilder:validation:Optional
	Cooldown *CooldownSpec `json:"cooldown,omitempty"`
	// MaxAttemptsPerQuestion caps how many times a single question can be retried.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Optional
	MaxAttemptsPerQuestion *int `json:"maxAttemptsPerQuestion,omitempty"`
	// LockOnMaxAttempts locks the quiz once MaxAttemptsPerQuestion is reached on any question.
	// +kubebuilder:validation:Optional
	LockOnMaxAttempts bool `json:"lockOnMaxAttempts,omitempty"`
	// CheckProvider selects the backend used to verify lab-type module completion.
	// +kubebuilder:validation:Enum=local;gitlab
	// +kubebuilder:validation:Optional
	CheckProvider string `json:"checkProvider,omitempty"`
	// CheckType selects how the module as a whole is verified (provider-specific).
	// +kubebuilder:validation:Optional
	CheckType string `json:"checkType,omitempty"`
	// CheckParams holds provider-specific parameters for verifying this module.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Optional
	CheckParams *runtime.RawExtension `json:"checkParams,omitempty"`
	// Steps lists the individually verifiable steps of a lab-type module.
	// +kubebuilder:validation:Optional
	Steps []CheckStep `json:"steps,omitempty"`
	// QuizRef references an external quiz definition instead of inline Questions.
	// +kubebuilder:validation:Optional
	QuizRef string `json:"quizRef,omitempty"`
}

// CourseSpec defines the desired state of a Course.
type CourseSpec struct {
	// Title is the course's display name; defaults to the resource name if omitted.
	// +kubebuilder:validation:Optional
	Title string `json:"title,omitempty"`
	// Description is a short summary of the course shown in the catalog.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
	// Public: when true, the course is publicly visible and users are auto-enrolled on first access.
	// +kubebuilder:validation:Optional
	Public bool `json:"public,omitempty"`
	// Hidden: when true, the course is hidden from the catalog.
	// +kubebuilder:validation:Optional
	Hidden bool `json:"hidden,omitempty"`
	// Category groups the course for browsing/filtering purposes.
	// +kubebuilder:validation:Optional
	Category string `json:"category,omitempty"`
	// Difficulty labels the course's expected skill level.
	// +kubebuilder:validation:Enum=beginner;intermediate;advanced
	// +kubebuilder:default=beginner
	// +kubebuilder:validation:Optional
	Difficulty string `json:"difficulty,omitempty"`
	// Modules lists the course's content in display order.
	// +kubebuilder:validation:Optional
	Modules []Module `json:"modules,omitempty"`
	// Prerequisites lists the conditions that must be met before a user can enroll.
	// +kubebuilder:validation:Optional
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

	// Spec defines the desired state of the Course.
	Spec CourseSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// CourseList contains a list of Course.
type CourseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Courses.
	Items []Course `json:"items"`
}
