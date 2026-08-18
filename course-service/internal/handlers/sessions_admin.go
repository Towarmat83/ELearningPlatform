package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
	"github.com/genesary/pupitre/course-service/internal/content"
)

const (
	// sessionIDBytes is the number of random bytes used to generate a session ID.
	// 8 bytes = 16 hex chars; collision probability is negligible even at scale.
	sessionIDBytes = 8
)

// sessionBody is the request payload for creating or updating a session.
type sessionBody struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Location string `json:"location"`
	Capacity int    `json:"capacity"`
}

// generateSessionID returns a random identifier of the form "sess-<hex>".
func generateSessionID() (string, error) {
	buf := make([]byte, sessionIDBytes)

	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return "sess-" + hex.EncodeToString(buf), nil
}

// sessionFromBody converts a sessionBody and ID into a content.Session.
func sessionFromBody(id string, body sessionBody) content.Session {
	return content.Session{
		ID:       id,
		Title:    body.Title,
		Date:     body.Date,
		Location: body.Location,
		Capacity: body.Capacity,
	}
}

// CreateSession godoc
// @Summary  Add a session to a course (admin or manager)
// @Tags     Admin - Sessions
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/sessions [post].
func (s *State) CreateSession(writer http.ResponseWriter, req *http.Request) { //nolint:funlen // multiple guard clauses required
	slug := chi.URLParam(req, "slug")

	var body sessionBody

	decodeErr := decode(req, &body)
	if decodeErr != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.Title == "" || body.Date == "" {
		s.Error(writer, http.StatusBadRequest, "title and date are required")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	sessionID, err := generateSessionID()
	if err != nil {
		zap.L().Error("generate session id failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to generate session ID")

		return
	}

	ctx := req.Context()
	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	var course coursev1.Course

	getErr := kubeClient.Get(ctx, key, &course)
	if getErr != nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if course.Spec.Sessions == nil {
		course.Spec.Sessions = make(map[string]coursev1.CourseSession)
	}

	course.Spec.Sessions[sessionID] = coursev1.CourseSession{
		Title:    body.Title,
		Date:     body.Date,
		Location: body.Location,
		Capacity: body.Capacity,
	}

	rollback := s.Content.AddSession(slug, sessionFromBody(sessionID, body))

	updateErr := kubeClient.Update(ctx, &course)
	if updateErr != nil {
		zap.L().Error("update course sessions failed", zap.String("slug", slug), zap.Error(updateErr))
		rollback()
		s.Error(writer, http.StatusInternalServerError, "Failed to create session")

		return
	}

	s.JSON(writer, http.StatusCreated, map[string]string{"id": sessionID})
}

// UpdateSession godoc
// @Summary  Update a session on a course (admin or manager)
// @Tags     Admin - Sessions
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/sessions/{sessionId} [put].
func (s *State) UpdateSession(writer http.ResponseWriter, req *http.Request) {
	slug := chi.URLParam(req, "slug")
	sessionID := chi.URLParam(req, "sessionId")

	var body sessionBody

	decodeErr := decode(req, &body)
	if decodeErr != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	ctx := req.Context()
	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	var course coursev1.Course

	getErr := kubeClient.Get(ctx, key, &course)
	if getErr != nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if _, ok := course.Spec.Sessions[sessionID]; !ok {
		s.Error(writer, http.StatusNotFound, "Session not found")

		return
	}

	course.Spec.Sessions[sessionID] = coursev1.CourseSession{
		Title:    body.Title,
		Date:     body.Date,
		Location: body.Location,
		Capacity: body.Capacity,
	}

	rollback := s.Content.ReplaceSession(slug, sessionFromBody(sessionID, body))

	updateErr := kubeClient.Update(ctx, &course)
	if updateErr != nil {
		zap.L().Error("update course sessions failed", zap.String("slug", slug), zap.Error(updateErr))
		rollback()
		s.Error(writer, http.StatusInternalServerError, "Failed to update session")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{"id": sessionID})
}

// DeleteSession godoc
// @Summary  Delete a session from a course (admin or manager)
// @Tags     Admin - Sessions
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/sessions/{sessionId} [delete].
func (s *State) DeleteSession(writer http.ResponseWriter, req *http.Request) {
	slug := chi.URLParam(req, "slug")
	sessionID := chi.URLParam(req, "sessionId")

	kubeClient, err := k8sClient(s.Config.Kubeconfig)
	if err != nil {
		zap.L().Error("k8s client init failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Internal error")

		return
	}

	ctx := req.Context()
	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	var course coursev1.Course

	getErr := kubeClient.Get(ctx, key, &course)
	if getErr != nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if _, ok := course.Spec.Sessions[sessionID]; !ok {
		s.Error(writer, http.StatusNotFound, "Session not found")

		return
	}

	delete(course.Spec.Sessions, sessionID)

	rollback := s.Content.RemoveSession(slug, sessionID)

	updateErr := kubeClient.Update(ctx, &course)
	if updateErr != nil {
		zap.L().Error("delete course session failed", zap.String("slug", slug), zap.Error(updateErr))
		rollback()
		s.Error(writer, http.StatusInternalServerError, "Failed to delete session")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: "Session deleted"})
}
