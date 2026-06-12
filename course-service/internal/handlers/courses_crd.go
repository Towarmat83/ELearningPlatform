package handlers

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var courseGVR = schema.GroupVersionResource{
	Group:    "elearning.example.com",
	Version:  "v1",
	Resource: "courses",
}

func k8sDynamic(kubeconfig string) (dynamic.Interface, error) {
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}
	return dynamic.NewForConfig(cfg)
}

// CreateCourseCRD godoc
// @Summary  Create a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses [post]
func (s *State) CreateCourseCRD(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	slug, _ := body["slug"].(string)
	if slug == "" {
		s.Error(w, http.StatusBadRequest, "slug is required")
		return
	}
	spec, _ := body["spec"].(map[string]any)
	if spec == nil {
		spec = buildCourseSpec(body)
	}

	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "elearning.example.com/v1",
		"kind":       "Course",
		"metadata":   map[string]any{"name": slug, "namespace": s.Config.K8sNamespace},
		"spec":       spec,
	}}

	created, err := dyn.Resource(courseGVR).Namespace(s.Config.K8sNamespace).
		Create(context.Background(), obj, metav1.CreateOptions{})
	if err != nil {
		s.Error(w, http.StatusConflict, "Failed to create CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusCreated, map[string]any{"slug": created.GetName()})
}

// GetCourseCRD returns the raw CRD spec for a course.
func (s *State) GetCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	obj, err := dyn.Resource(courseGVR).Namespace(s.Config.K8sNamespace).
		Get(context.Background(), slug, metav1.GetOptions{})
	if err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"slug": slug,
		"spec": obj.Object["spec"],
	})
}

// UpdateCourseCRD godoc
// @Summary  Update a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [put]
func (s *State) UpdateCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	var body map[string]any
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	spec, _ := body["spec"].(map[string]any)
	if spec == nil {
		spec = buildCourseSpec(body)
	}

	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	ctx := context.Background()
	existing, err := dyn.Resource(courseGVR).Namespace(s.Config.K8sNamespace).
		Get(ctx, slug, metav1.GetOptions{})
	if err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	existing.Object["spec"] = spec

	_, err = dyn.Resource(courseGVR).Namespace(s.Config.K8sNamespace).
		Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to update CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"slug": slug})
}

// DeleteCourseCRD godoc
// @Summary  Delete a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [delete]
func (s *State) DeleteCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = dyn.Resource(courseGVR).Namespace(s.Config.K8sNamespace).
		Delete(context.Background(), slug, metav1.DeleteOptions{})
	if err != nil {
		s.Error(w, http.StatusNotFound, "Failed to delete CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Course deleted"})
}

func buildCourseSpec(body map[string]any) map[string]any {
	spec := map[string]any{}
	for _, k := range []string{"title", "description", "category", "difficulty"} {
		if v, ok := body[k]; ok {
			spec[k] = v
		}
	}
	if v, ok := body["public"]; ok {
		spec["public"] = v
	}
	if v, ok := body["modules"]; ok {
		spec["modules"] = v
	}
	if v, ok := body["prerequisites"]; ok {
		spec["prerequisites"] = v
	}
	return spec
}
