package handlers

import (
	"net/http"

	"github.com/elearning/user-service/internal/middleware"
)

// InternalCheckEnrollment godoc
// @Summary  Check if a user is enrolled (internal)
// @Tags     Internal
// @Produce  json
// @Param    user_id      query  string  true  "User UUID"
// @Param    course_slug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]bool
// @Router   /internal/enrollments/check [get]
func (s *State) InternalCheckEnrollment(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	courseSlug := r.URL.Query().Get("course_slug")
	var enrolled bool
	err := s.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) > 0 FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug).Scan(&enrolled)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"enrolled": enrolled})
}

// InternalViewedLessons godoc
// @Summary  Get viewed lessons for a user (internal)
// @Tags     Internal
// @Produce  json
// @Param    user_id      query  string  true  "User UUID"
// @Param    course_slug  query  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /internal/progress/viewed [get]
func (s *State) InternalViewedLessons(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	courseSlug := r.URL.Query().Get("course_slug")
	rows, err := s.Pool.Query(r.Context(),
		`SELECT lesson_slug FROM lesson_progress WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, courseSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			slugs = append(slugs, slug)
		}
	}
	if slugs == nil {
		slugs = []string{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"viewed": slugs})
}

// InternalMarkComplete godoc
// @Summary  Mark a lesson complete (internal)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "user_id, course_slug, lesson_slug"
// @Success  200   {object}  map[string]bool
// @Router   /internal/progress/complete [post]
func (s *State) InternalMarkComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     string `json:"user_id"`
		CourseSlug string `json:"course_slug"`
		LessonSlug string `json:"lesson_slug"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`		INSERT INTO lesson_progress (user_id, course_slug, lesson_slug, viewed_at)
		 VALUES ($1::uuid, $2, $3, NOW())
		 ON CONFLICT (user_id, course_slug, lesson_slug) DO NOTHING`,
		body.UserID, body.CourseSlug, body.LessonSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "DB error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// InternalSSOLogin godoc
// @Summary  Upsert SSO user and issue JWT (internal — no auth, network-policy protected)
// @Tags     Internal
// @Accept   json
// @Produce  json
// @Param    body  body  object  true  "email, name, avatar_url, provider, provider_user_id, groups, group_source"
// @Success  200   {object}  authResponse
// @Failure  400   {object}  map[string]string
// @Router   /internal/sso-login [post]
func (s *State) InternalSSOLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email          string   `json:"email"`
		Name           string   `json:"name"`
		AvatarURL      *string  `json:"avatar_url"`
		Provider       string   `json:"provider"`
		ProviderUserID string   `json:"provider_user_id"`
		Groups         []string `json:"groups"`
		GroupSource    string   `json:"group_source"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Email == "" || req.Provider == "" || req.ProviderUserID == "" {
		s.Error(w, http.StatusBadRequest, "email, provider, and provider_user_id are required")
		return
	}

	ctx := r.Context()

	user, err := upsertSSOUser(ctx, s.Pool, req.Email, req.Name, req.AvatarURL, req.Provider, req.ProviderUserID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to upsert user: "+err.Error())
		return
	}

	groupSource := req.GroupSource
	if groupSource == "" {
		groupSource = req.Provider
	}

	role, err := syncGroupsAndDeriveRole(ctx, s.Pool, user.ID, req.Groups, groupSource)
	if err != nil {
		role = user.Role
	}

	token, err := middleware.CreateToken(user.ID, user.Email, role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}
	user.Role = role
	s.JSON(w, http.StatusOK, authResponse{Token: token, User: *user})
}
