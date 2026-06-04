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

var patternGVR = schema.GroupVersionResource{
	Group:    "elearning.example.com",
	Version:  "v1",
	Resource: "markdownpatterns",
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

// ListPatterns godoc
// @Summary  List MarkdownPattern CRDs
// @Tags     Patterns
// @Produce  json
// @Param    scope  query  string  false  "course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/patterns [get]
func (s *State) ListPatterns(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	patterns := s.Patterns.List(scope)
	s.JSON(w, http.StatusOK, map[string]any{"patterns": patterns})
}

// CreatePatternCRD godoc
// @Summary  Create a MarkdownPattern CRD (admin)
// @Tags     Admin - Patterns
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Router   /api/admin/patterns [post]
func (s *State) CreatePatternCRD(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		Scope string `json:"scope"`
		HTML  string `json:"html"`
		CSS   string `json:"css"`
		JS    string `json:"js"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" || body.HTML == "" {
		s.Error(w, http.StatusBadRequest, "name and html are required")
		return
	}
	if body.Scope == "" {
		body.Scope = "global"
	}

	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "k8s client error: "+err.Error())
		return
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "elearning.example.com/v1",
			"kind":       "MarkdownPattern",
			"metadata": map[string]any{
				"name":      body.Name,
				"namespace": s.Config.K8sNamespace,
			},
			"spec": map[string]any{
				"name":  body.Name,
				"label": body.Label,
				"scope": body.Scope,
				"html":  body.HTML,
				"css":   body.CSS,
				"js":    body.JS,
			},
		},
	}

	created, err := dyn.Resource(patternGVR).Namespace(s.Config.K8sNamespace).
		Create(context.Background(), obj, metav1.CreateOptions{})
	if err != nil {
		s.Error(w, http.StatusConflict, "Failed to create CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusCreated, map[string]any{"name": created.GetName()})
}

// UpdatePatternCRD godoc
// @Summary  Update a MarkdownPattern CRD (admin)
// @Tags     Admin - Patterns
// @Security BearerAuth
// @Router   /api/admin/patterns/{name} [put]
func (s *State) UpdatePatternCRD(w http.ResponseWriter, r *http.Request) {
	name := param(r, "name")
	var body struct {
		Label string `json:"label"`
		Scope string `json:"scope"`
		HTML  string `json:"html"`
		CSS   string `json:"css"`
		JS    string `json:"js"`
	}
	if err := decode(r, &body); err != nil || body.HTML == "" {
		s.Error(w, http.StatusBadRequest, "html is required")
		return
	}
	if body.Scope == "" {
		body.Scope = "global"
	}

	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "k8s client error: "+err.Error())
		return
	}

	ctx := context.Background()
	existing, err := dyn.Resource(patternGVR).Namespace(s.Config.K8sNamespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		s.Error(w, http.StatusNotFound, "Pattern not found")
		return
	}

	spec := map[string]any{
		"name":  name,
		"label": body.Label,
		"scope": body.Scope,
		"html":  body.HTML,
		"css":   body.CSS,
		"js":    body.JS,
	}
	existing.Object["spec"] = spec

	_, err = dyn.Resource(patternGVR).Namespace(s.Config.K8sNamespace).
		Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to update CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"name": name})
}

// DeletePatternCRD godoc
// @Summary  Delete a MarkdownPattern CRD (admin)
// @Tags     Admin - Patterns
// @Security BearerAuth
// @Router   /api/admin/patterns/{name} [delete]
func (s *State) DeletePatternCRD(w http.ResponseWriter, r *http.Request) {
	name := param(r, "name")
	dyn, err := k8sDynamic(s.Config.Kubeconfig)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "k8s client error: "+err.Error())
		return
	}
	err = dyn.Resource(patternGVR).Namespace(s.Config.K8sNamespace).
		Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		s.Error(w, http.StatusNotFound, "Failed to delete CRD: "+err.Error())
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Pattern deleted"})
}
