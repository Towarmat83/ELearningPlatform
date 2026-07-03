package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PathSpec defines the desired state of a Path.
type PathSpec struct {
	// Title is the human-readable name of this learning path.
	// +kubebuilder:validation:Optional
	Title string `json:"title,omitempty"`
	// Description provides a brief overview of the path's goals and content.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
	// Courses is the ordered list of course slugs that compose this path.
	// Each slug must match an existing Course resource in the same namespace.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:Pattern=`^[a-z0-9][a-z0-9-]*$`
	Courses []string `json:"courses,omitempty"`
}

// Path is the Schema for the paths API.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=path
// +kubebuilder:printcolumn:name="Title",type="string",JSONPath=".spec.title"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Path struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PathSpec `json:"spec,omitempty"`
}

// PathList contains a list of Path.
// +kubebuilder:object:root=true
type PathList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Path `json:"items"`
}
