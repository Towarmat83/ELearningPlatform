package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
)

// pathSpecFromBody builds a typed PathSpec from a request body.
func pathSpecFromBody(body map[string]any) (coursev1.PathSpec, error) {
	rawSpec, _ := body["spec"].(map[string]any)
	if rawSpec == nil {
		rawSpec = buildPathSpec(body)
	}

	var spec coursev1.PathSpec

	data, err := json.Marshal(rawSpec)
	if err != nil {
		return spec, fmt.Errorf("marshal path spec: %w", err)
	}

	err = json.Unmarshal(data, &spec)
	if err != nil {
		return spec, fmt.Errorf("unmarshal path spec: %w", err)
	}

	return spec, nil
}

// buildPathSpec extracts recognized PathSpec fields from a flat top-level
// request body map.
func buildPathSpec(body map[string]any) map[string]any {
	spec := map[string]any{}

	for _, k := range []string{specKeyTitle, specKeyDescription, "kind", "level"} {
		if v, ok := body[k]; ok {
			spec[k] = v
		}
	}

	if v, ok := body["courses"]; ok {
		spec["courses"] = v
	}

	if v, ok := body["skills"]; ok {
		spec["skills"] = v
	}

	return spec
}

// CreatePathCRD godoc
// @Summary  Create a Path CRD (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths [post].
func (s *State) CreatePathCRD(writer http.ResponseWriter, req *http.Request) {
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

	spec, err := pathSpecFromBody(body)
	if err != nil {
		zap.L().Error("invalid path spec", zap.Error(err))
		s.Error(writer, http.StatusBadRequest, "Invalid path spec")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error when connecting to kubernetes")

		return
	}

	pathCR := &coursev1.Path{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
		Spec:       spec,
	}

	err = kubeClient.Create(req.Context(), pathCR)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.Error(writer, http.StatusConflict, "Path already exists")

			return
		}

		zap.L().Error("create path CRD failed", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to create path")

		return
	}

	s.JSON(writer, http.StatusCreated, map[string]any{slugJSONKey: slug})
}

// UpdatePathCRD godoc
// @Summary  Update a Path CRD (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths/{slug}/crd [put].
func (s *State) UpdatePathCRD(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	var body map[string]any

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	spec, err := pathSpecFromBody(body)
	if err != nil {
		zap.L().Error("invalid path spec", zap.Error(err))
		s.Error(writer, http.StatusBadRequest, "Invalid path spec")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error when connecting to kubernetes")

		return
	}

	ctx := req.Context()

	var existing coursev1.Path

	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	err = kubeClient.Get(ctx, key, &existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.Error(writer, http.StatusNotFound, "Path not found")

			return
		}

		zap.L().Error("fetch path CRD failed", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to fetch path")

		return
	}

	existing.Spec = spec

	err = kubeClient.Update(ctx, &existing)
	if err != nil {
		zap.L().Error("update path CRD failed", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to update path")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{slugJSONKey: slug})
}

// DeletePathCRD godoc
// @Summary  Delete a Path CRD (admin)
// @Tags     Admin - Paths
// @Security BearerAuth
// @Router   /api/admin/courses/paths/{slug}/crd [delete].
func (s *State) DeletePathCRD(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error when connecting to kubernetes")

		return
	}

	pathCR := &coursev1.Path{
		ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Config.K8sNamespace},
	}

	err = kubeClient.Delete(req.Context(), pathCR)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.Error(writer, http.StatusNotFound, "Path not found")

			return
		}

		zap.L().Error("delete path CRD failed", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to delete path")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: "Path deleted"})
}
