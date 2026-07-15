// Package v1 contains the Course API Schema definitions for the
// elearning.pupitre.io/v1 API group.
// +kubebuilder:object:generate=true
// +groupName=elearning.pupitre.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "elearning.pupitre.io", Version: "v1"} //nolint:gochecknoglobals // controller-runtime scheme registration pattern

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes) //nolint:gochecknoglobals // controller-runtime scheme registration pattern

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme //nolint:gochecknoglobals // controller-runtime scheme registration pattern
)

// addKnownTypes registers this package's API types with the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		GroupVersion,
		&Course{},
		&CourseList{},
		&Path{},
		&PathList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)

	return nil
}
