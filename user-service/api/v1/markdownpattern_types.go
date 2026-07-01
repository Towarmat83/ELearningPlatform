package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MarkdownPatternSpec defines the desired state of a MarkdownPattern.
type MarkdownPatternSpec struct {
	// Name is the identifier used in markdown: |||name content|||
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	// HTML template; use {{content}} as placeholder.
	HTML string `json:"html"`
	CSS  string `json:"css,omitempty"`
	// JS executed after rendering (container is the wrapper element).
	JS string `json:"js,omitempty"`
	// +kubebuilder:default=global
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

	Spec MarkdownPatternSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MarkdownPatternList contains a list of MarkdownPattern.
type MarkdownPatternList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MarkdownPattern `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MarkdownPattern{}, &MarkdownPatternList{})
}
