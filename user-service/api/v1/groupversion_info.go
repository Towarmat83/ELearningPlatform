// Package v1 contains the MarkdownPattern API Schema definitions for the
// elearning.pupitre.io/v1 API group.
// +kubebuilder:object:generate=true
// +groupName=elearning.pupitre.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//nolint:gochecknoglobals // controller-runtime scheme registration pattern
var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "elearning.pupitre.io", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes registers the MarkdownPattern types with the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &MarkdownPattern{}, &MarkdownPatternList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)

	return nil
}
