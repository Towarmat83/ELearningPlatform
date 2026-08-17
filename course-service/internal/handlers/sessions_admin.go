package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
)

const (
	// sessionIDBytes is the number of random bytes used to generate a session ID.
	// 8 bytes = 16 hex chars; collision probability is negligible even at scale.
	sessionIDBytes = 8

	// sessionMaxRetry caps both unique-ID regeneration attempts and k8s
	// optimistic-lock retries (conflict on concurrent writes).
	sessionMaxRetry = 5
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

// generateUniqueSessionID returns a session ID that does not collide with any
// ID already present in sessions. It retries up to sessionMaxRetry times.
func generateUniqueSessionID(sessions []coursev1.CourseSession) (string, error) {
	for range sessionMaxRetry {
		sessionID, err := generateSessionID()
		if err != nil {
			return "", err
		}

		collision := false

		for _, s := range sessions {
			if s.ID == sessionID {
				collision = true

				break
			}
		}

		if !collision {
			return sessionID, nil
		}
	}

	return "", fmt.Errorf("generate session id: no unique id found in %d attempts", sessionMaxRetry)
}

// CreateSession godoc
// @Summary  Add a session to a course (admin or manager)
// @Tags     Admin - Sessions
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/sessions [post].
func (s *State) CreateSession(writer http.ResponseWriter, req *http.Request) { //nolint:funlen,gocyclo,cyclop // retry loop requires multiple guard clauses
	slug := chi.URLParam(req, "slug")

	var body sessionBody

	err := decode(req, &body)
	if err != nil {
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

	ctx := req.Context()
	key := client.ObjectKey{Name: slug, Namespace: s.Config.K8sNamespace}

	var (
		newSessionID string
		updateErr    error
	)

	for range sessionMaxRetry {
		var course coursev1.Course

		getErr := kubeClient.Get(ctx, key, &course)
		if getErr != nil {
			s.Error(writer, http.StatusNotFound, "Course not found")

			return
		}

		generatedID, genErr := generateUniqueSessionID(course.Spec.Sessions)
		if genErr != nil {
			s.Error(writer, http.StatusInternalServerError, "Failed to generate session ID")

			return
		}

		course.Spec.Sessions = append(course.Spec.Sessions, coursev1.CourseSession{
			ID:       generatedID,
			Title:    body.Title,
			Date:     body.Date,
			Location: body.Location,
			Capacity: body.Capacity,
		})

		updateErr = kubeClient.Update(ctx, &course)
		if updateErr == nil {
			newSessionID = generatedID

			break
		}

		if !apierrors.IsConflict(updateErr) {
			zap.L().Error("update course sessions failed", zap.String("slug", slug), zap.Error(updateErr))
			s.Error(writer, http.StatusInternalServerError, "Failed to create session")

			return
		}
	}

	if updateErr != nil {
		zap.L().Error("update course sessions exhausted retries", zap.String("slug", slug), zap.Error(updateErr))
		s.Error(writer, http.StatusInternalServerError, "Failed to create session")

		return
	}

	s.JSON(writer, http.StatusCreated, map[string]string{"id": newSessionID})
}

// applySessionUpdate replaces the session matching sessionID with body fields.
// Returns the updated slice and false if no matching session was found.
func applySessionUpdate(sessions []coursev1.CourseSession, sessionID string, body sessionBody) ([]coursev1.CourseSession, bool) {
	for idx := range sessions {
		if sessions[idx].ID != sessionID {
			continue
		}

		sessions[idx].Title = body.Title
		sessions[idx].Date = body.Date
		sessions[idx].Location = body.Location
		sessions[idx].Capacity = body.Capacity

		return sessions, true
	}

	return sessions, false
}

// UpdateSession godoc
// @Summary  Update a session on a course (admin or manager)
// @Tags     Admin - Sessions
// @Security BearerAuth
// @Router   /api/admin/courses/{slug}/sessions/{sessionId} [put].
func (s *State) UpdateSession(writer http.ResponseWriter, req *http.Request) { //nolint:funlen // retry loop requires multiple guard clauses
	slug := chi.URLParam(req, "slug")
	sessionID := chi.URLParam(req, "sessionId")

	var body sessionBody

	err := decode(req, &body)
	if err != nil {
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

	var updateErr error

	for range sessionMaxRetry {
		var course coursev1.Course

		getErr := kubeClient.Get(ctx, key, &course)
		if getErr != nil {
			s.Error(writer, http.StatusNotFound, "Course not found")

			return
		}

		updated, found := applySessionUpdate(course.Spec.Sessions, sessionID, body)
		if !found {
			s.Error(writer, http.StatusNotFound, "Session not found")

			return
		}

		course.Spec.Sessions = updated

		updateErr = kubeClient.Update(ctx, &course)
		if updateErr == nil {
			break
		}

		if !apierrors.IsConflict(updateErr) {
			zap.L().Error("update course sessions failed", zap.String("slug", slug), zap.Error(updateErr))
			s.Error(writer, http.StatusInternalServerError, "Failed to update session")

			return
		}
	}

	if updateErr != nil {
		zap.L().Error("update course sessions exhausted retries", zap.String("slug", slug), zap.Error(updateErr))
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
func (s *State) DeleteSession(writer http.ResponseWriter, req *http.Request) { //nolint:funlen // retry loop requires multiple guard clauses
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

	var updateErr error

	for range sessionMaxRetry {
		var course coursev1.Course

		getErr := kubeClient.Get(ctx, key, &course)
		if getErr != nil {
			s.Error(writer, http.StatusNotFound, "Course not found")

			return
		}

		filtered := make([]coursev1.CourseSession, 0, len(course.Spec.Sessions))

		for _, sess := range course.Spec.Sessions {
			if sess.ID != sessionID {
				filtered = append(filtered, sess)
			}
		}

		if len(filtered) == len(course.Spec.Sessions) {
			s.Error(writer, http.StatusNotFound, "Session not found")

			return
		}

		course.Spec.Sessions = filtered

		updateErr = kubeClient.Update(ctx, &course)
		if updateErr == nil {
			break
		}

		if !apierrors.IsConflict(updateErr) {
			zap.L().Error("delete course session failed", zap.String("slug", slug), zap.Error(updateErr))
			s.Error(writer, http.StatusInternalServerError, "Failed to delete session")

			return
		}
	}

	if updateErr != nil {
		zap.L().Error("delete course session exhausted retries", zap.String("slug", slug), zap.Error(updateErr))
		s.Error(writer, http.StatusInternalServerError, "Failed to delete session")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: "Session deleted"})
}
