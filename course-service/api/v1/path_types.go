package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PathCourseEntry references one course within a learning path.
type PathCourseEntry struct {
	// +kubebuilder:validation:MinLength=1
	Slug string `json:"slug"`
}

// PathSpec defines the desired state of a Path.
type PathSpec struct {
	// +kubebuilder:validation:Optional
	Title string `json:"title,omitempty"`
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
	// +kubebuilder:validation:Optional
	Courses []PathCourseEntry `json:"courses,omitempty"`
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
