package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/elearning/course-service/api/v1"
)

// k8sClient builds a controller-runtime client for the Course CRD, using
// the given kubeconfig path or falling back to in-cluster configuration.
func k8sClient(kubeconfig string) (client.Client, error) { //nolint:ireturn // controller-runtime's constructor is interface-typed
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

	err = coursev1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("build k8s client: %w", err)
	}

	return cl, nil
}

// CreateCourseCRD godoc
// @Summary  Create a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses [post].
func (s *State) CreateCourseCRD(writer http.ResponseWriter, req *http.Request) {
	var body map[string]any

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	slug, _ := body[slugJSONKey].(string)
	if slug == "" {
		s.Error(writer, http.StatusBadRequest, "slug is required")

		return
	}

	spec, err := courseSpecFromBody(body)
	if err != nil {
		slog.Error("invalid course spec", "err", err)
		s.Error(writer, http.StatusBadRequest, "Invalid course spec")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		slog.Error("k8s client init failed", "err", err)
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	courseCR := &coursev1.Course{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
		Spec:       spec,
	}

	err = kubeClient.Create(req.Context(), courseCR)
	if err != nil {
		slog.Error("create course CRD failed", "slug", slug, "err", err)
		s.Error(writer, http.StatusConflict, "Failed to create course")

		return
	}

	s.JSON(writer, http.StatusCreated, map[string]any{slugJSONKey: slug})
}

// GetCourseCRD returns the raw CRD spec for a course.
func (s *State) GetCourseCRD(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		slog.Error("k8s client init failed", "err", err)
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	var courseCR coursev1.Course

	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	err = kubeClient.Get(req.Context(), key, &courseCR)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		slugJSONKey: slug,
		"spec":      courseCR.Spec,
	})
}

// UpdateCourseCRD godoc
// @Summary  Update a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [put].
func (s *State) UpdateCourseCRD(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	var body map[string]any

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	spec, err := courseSpecFromBody(body)
	if err != nil {
		slog.Error("invalid course spec", "err", err)
		s.Error(writer, http.StatusBadRequest, "Invalid course spec")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		slog.Error("k8s client init failed", "err", err)
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	ctx := req.Context()

	var existing coursev1.Course

	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	err = kubeClient.Get(ctx, key, &existing)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	existing.Spec = spec

	err = kubeClient.Update(ctx, &existing)
	if err != nil {
		slog.Error("update course CRD failed", "slug", slug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "Failed to update course")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{slugJSONKey: slug})
}

// DeleteCourseCRD godoc
// @Summary  Delete a Course CRD (admin)
// @Tags     Admin - Courses
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/crd [delete].
func (s *State) DeleteCourseCRD(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		slog.Error("k8s client init failed", "err", err)
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	courseCR := &coursev1.Course{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
	}

	err = kubeClient.Delete(req.Context(), courseCR)
	if err != nil {
		if apierrors.IsNotFound(err) {
			slog.Error("delete course CRD failed", "slug", slug, "err", err)
			s.Error(writer, http.StatusNotFound, "Failed to delete course")

			return
		}

		slog.Error("delete course CRD failed", "slug", slug, "err", err)
		s.Error(writer, http.StatusNotFound, "Failed to delete course")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: "Course deleted"})
}

// buildCourseSpec extracts recognized CourseSpec fields from a flat
// top-level request body map.
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
		return spec, fmt.Errorf("marshal course spec: %w", err)
	}

	err = json.Unmarshal(data, &spec)
	if err != nil {
		return spec, fmt.Errorf("unmarshal course spec: %w", err)
	}

	return spec, nil
}
