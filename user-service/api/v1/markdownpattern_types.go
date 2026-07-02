package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MarkdownPatternSpec defines the desired state of a MarkdownPattern.
type MarkdownPatternSpec struct {
	// Name is the identifier used in markdown: |||name content|||
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Label is the human-readable name shown for this pattern in the admin UI;
	// defaults to Name if omitted.
	// +kubebuilder:validation:Optional
	Label string `json:"label,omitempty"`
	// Description explains what this pattern renders, shown in the admin UI.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
	// HTML template; use {{content}} as placeholder.
	// +kubebuilder:validation:MinLength=1
	HTML string `json:"html"`
	// CSS is injected alongside the rendered HTML to style this pattern.
	// +kubebuilder:validation:Optional
	CSS string `json:"css,omitempty"`
	// JS executed after rendering (container is the wrapper element).
	// +kubebuilder:validation:Optional
	JS string `json:"js,omitempty"`
	// Scope limits where this pattern can be used: "global" for all courses,
	// or a specific course slug to restrict it to that course.
	// +kubebuilder:default=global
	// +kubebuilder:validation:Optional
	Scope string `json:"scope,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=mdpat
// +kubebuilder:printcolumn:name="Label",type=string,JSONPath=`.spec.label`
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.spec.scope`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MarkdownPattern is the Schema for the markdownpatterns API.
type MarkdownPattern struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the MarkdownPattern.
	Spec MarkdownPatternSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MarkdownPatternList contains a list of MarkdownPattern.
type MarkdownPatternList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of MarkdownPatterns.
	Items []MarkdownPattern `json:"items"`
}
