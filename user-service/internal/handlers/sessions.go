package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// BookSession godoc
// @Summary  Reserve a seat for an in-person session
// @Tags     Sessions
// @Produce  json
// @Param    slug       path  string  true  "Course slug"
// @Param    sessionId  path  string  true  "Session ID"
// @Success  200  {object}  map[string]bool
// @Router   /api/courses/{slug}/sessions/{sessionId}/book [post].
func (s *State) BookSession(writer http.ResponseWriter, req *http.Request) { //nolint:dupl // similar structure to UnbookSession; different repository call
	claims := s.claims(req)
	slug := param(req, "slug")
	sessionID := param(req, "sessionId")

	err := s.Repos.SessionBookings.Book(req.Context(), claims.Subject, slug, sessionID)
	if err != nil {
		zap.L().Error("failed to book session",
			zap.String("userID", claims.Subject),
			zap.String("courseSlug", slug),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to book session")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// UnbookSession godoc
// @Summary  Cancel a seat reservation for an in-person session
// @Tags     Sessions
// @Produce  json
// @Param    slug       path  string  true  "Course slug"
// @Param    sessionId  path  string  true  "Session ID"
// @Success  200  {object}  map[string]bool
// @Router   /api/courses/{slug}/sessions/{sessionId}/book [delete].
func (s *State) UnbookSession(writer http.ResponseWriter, req *http.Request) { //nolint:dupl // similar structure to BookSession; different repository call
	claims := s.claims(req)
	slug := param(req, "slug")
	sessionID := param(req, "sessionId")

	err := s.Repos.SessionBookings.Unbook(req.Context(), claims.Subject, slug, sessionID)
	if err != nil {
		zap.L().Error("failed to unbook session",
			zap.String("userID", claims.Subject),
			zap.String("courseSlug", slug),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to unbook session")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// MySessionBookings godoc
// @Summary  List the current user's session bookings
// @Tags     Sessions
// @Produce  json
// @Success  200  {object}  map[string]interface{}
// @Router   /api/my/session-bookings [get].
func (s *State) MySessionBookings(writer http.ResponseWriter, req *http.Request) {
	claims := s.claims(req)

	bookings, err := s.Repos.SessionBookings.ListByUser(req.Context(), claims.Subject)
	if err != nil {
		zap.L().Error("failed to list session bookings",
			zap.String("userID", claims.Subject),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to fetch bookings")

		return
	}

	if bookings == nil {
		bookings = []repository.SessionBookingRow{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"bookings": bookings})
}

// ListSessionBookings godoc
// @Summary  List all participants for a session (admin/manager)
// @Tags     Sessions
// @Produce  json
// @Param    slug       path  string  true  "Course slug"
// @Param    sessionId  path  string  true  "Session ID"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/admin/courses/{slug}/sessions/{sessionId}/bookings [get].
func (s *State) ListSessionBookings(writer http.ResponseWriter, req *http.Request) { //nolint:dupl // similar structure to SessionBookingCount; returns participants not count
	slug := param(req, "slug")
	sessionID := param(req, "sessionId")

	participants, err := s.Repos.SessionBookings.ListBySession(req.Context(), slug, sessionID)
	if err != nil {
		zap.L().Error("failed to list session participants",
			zap.String("courseSlug", slug),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to fetch participants")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{"participants": participants})
}

// MarkSessionPresence godoc
// @Summary  Mark a participant as present or absent (admin/manager)
// @Tags     Sessions
// @Accept   json
// @Produce  json
// @Param    slug       path  string  true  "Course slug"
// @Param    sessionId  path  string  true  "Session ID"
// @Param    userId     path  string  true  "User ID"
// @Success  200  {object}  map[string]bool
// @Router   /api/admin/courses/{slug}/sessions/{sessionId}/bookings
// /{userId}/presence [patch].
func (s *State) MarkSessionPresence(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")
	sessionID := param(req, "sessionId")
	userID := param(req, "userId")

	var body struct {
		Present bool `json:"present"`
	}

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid request body")

		return
	}

	err = s.Repos.SessionBookings.MarkPresent(req.Context(), userID, slug, sessionID, body.Present)
	if err != nil {
		zap.L().Error("failed to mark presence",
			zap.String("userID", userID),
			zap.String("courseSlug", slug),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to update presence")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// SessionBookingCount godoc
// @Summary  Get the number of bookings for a session
// @Tags     Sessions
// @Produce  json
// @Param    slug       path  string  true  "Course slug"
// @Param    sessionId  path  string  true  "Session ID"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/courses/{slug}/sessions/{sessionId}/booking-count [get].
func (s *State) SessionBookingCount(writer http.ResponseWriter, req *http.Request) { //nolint:dupl // similar structure to ListSessionBookings; returns count not participants
	slug := param(req, "slug")
	sessionID := param(req, "sessionId")

	count, err := s.Repos.SessionBookings.CountBySession(req.Context(), slug, sessionID)
	if err != nil {
		zap.L().Error("failed to count session bookings",
			zap.String("courseSlug", slug),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to count bookings")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{"count": count})
}
