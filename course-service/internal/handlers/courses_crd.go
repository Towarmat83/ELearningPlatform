package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/elearning/course-service/api/v1"
)

func k8sClient(kubeconfig string) (client.Client, error) {
	var (
		cfg *rest.Config
		err error
	)
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := coursev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	return client.New(cfg, client.Options{Scheme: scheme})
}

// CreateCourseCRD godoc
// @Summary  Create a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses [post].
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

	spec, err := courseSpecFromBody(body)
	if err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid spec: "+err.Error())

		return
	}

	c, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())

		return
	}

	cr := &coursev1.Course{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
		Spec:       spec,
	}
	if err := c.Create(context.Background(), cr); err != nil {
		s.Error(w, http.StatusConflict, "Failed to create CRD: "+err.Error())

		return
	}

	s.JSON(w, http.StatusCreated, map[string]any{"slug": slug})
}

// GetCourseCRD returns the raw CRD spec for a course.
func (s *State) GetCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	c, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())

		return
	}

	var cr coursev1.Course

	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}
	if err := c.Get(context.Background(), key, &cr); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")

		return
	}

	s.JSON(w, http.StatusOK, map[string]any{
		"slug": slug,
		"spec": cr.Spec,
	})
}

// UpdateCourseCRD godoc
// @Summary  Update a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [put].
func (s *State) UpdateCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	var body map[string]any
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")

		return
	}

	spec, err := courseSpecFromBody(body)
	if err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid spec: "+err.Error())

		return
	}

	c, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())

		return
	}

	ctx := context.Background()

	var existing coursev1.Course

	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}
	if err := c.Get(ctx, key, &existing); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")

		return
	}

	existing.Spec = spec

	if err := c.Update(ctx, &existing); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to update CRD: "+err.Error())

		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"slug": slug})
}

// DeleteCourseCRD godoc
// @Summary  Delete a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [delete].
func (s *State) DeleteCourseCRD(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	c, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, err.Error())

		return
	}

	cr := &coursev1.Course{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
	}
	if err := c.Delete(context.Background(), cr); err != nil {
		if apierrors.IsNotFound(err) {
			s.Error(w, http.StatusNotFound, "Failed to delete CRD: "+err.Error())

			return
		}

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

// courseSpecFromBody builds a typed CourseSpec from a request body, accepting
// either a nested "spec" object or flat top-level fields (title, modules, ...).
func courseSpecFromBody(body map[string]any) (coursev1.CourseSpec, error) {
	rawSpec, _ := body["spec"].(map[string]any)
	if rawSpec == nil {
		rawSpec = buildCourseSpec(body)
	}

	var spec coursev1.CourseSpec

	data, err := json.Marshal(rawSpec)
	if err != nil {
		return spec, err
	}

	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, err
	}

	return spec, nil
}
