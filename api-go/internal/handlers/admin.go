package handlers

import (
	"net/http"
	"strings"
)

// GET /api/admin/stats
func (s *State) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalUsers, totalEnrollments int64
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE role = 'student'").Scan(&totalUsers)         //nolint:errcheck
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments").Scan(&totalEnrollments)                    //nolint:errcheck

	totalCourses := int64(len(s.Content.All()))

	s.JSON(w, http.StatusOK, map[string]any{
		"total_users":       totalUsers,
		"total_courses":     totalCourses,
		"total_enrollments": totalEnrollments,
	})
}

// GET /api/admin/users
func (s *State) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(), `
		SELECT u.id::text, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio, u.created_at::text,
		       COUNT(DISTINCT e.course_slug)::bigint AS enrolled_courses
		FROM users u
		LEFT JOIN enrollments e ON e.user_id = u.id
		GROUP BY u.id
		ORDER BY u.created_at DESC`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type userAdminRow struct {
		ID              string  `json:"id"`
		Username        string  `json:"username"`
		Email           string  `json:"email"`
		Role            string  `json:"role"`
		IsActive        bool    `json:"is_active"`
		AvatarURL       *string `json:"avatar_url"`
		Bio             *string `json:"bio"`
		CreatedAt       string  `json:"created_at"`
		EnrolledCourses int64   `json:"enrolled_courses"`
	}
	users := make([]userAdminRow, 0)
	for rows.Next() {
		var u userAdminRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive,
			&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.EnrolledCourses); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		users = append(users, u)
	}
	s.JSON(w, http.StatusOK, map[string]any{"users": users, "total": len(users)})
}

// GET /api/admin/users/{user_id}
func (s *State) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := param(r, "user_id")
	type userDetailRow struct {
		ID              string  `json:"id"`
		Username        string  `json:"username"`
		Email           string  `json:"email"`
		Role            string  `json:"role"`
		IsActive        bool    `json:"is_active"`
		AvatarURL       *string `json:"avatar_url"`
		Bio             *string `json:"bio"`
		CreatedAt       string  `json:"created_at"`
		EnrolledCourses int64   `json:"enrolled_courses"`
		ViewedLessons   int64   `json:"viewed_lessons"`
	}
	var u userDetailRow
	err := s.Pool.QueryRow(r.Context(), `
		SELECT u.id::text, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio, u.created_at::text,
		       COUNT(DISTINCT e.course_slug)::bigint,
		       COUNT(DISTINCT lp.lesson_slug)::bigint
		FROM users u
		LEFT JOIN enrollments e ON e.user_id = u.id
		LEFT JOIN lesson_progress lp ON lp.user_id = u.id
		WHERE u.id = $1::uuid
		GROUP BY u.id`, userID).
		Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive, &u.AvatarURL, &u.Bio, &u.CreatedAt,
			&u.EnrolledCourses, &u.ViewedLessons)
	if err != nil {
		s.Error(w, http.StatusNotFound, "User not found")
		return
	}
	s.JSON(w, http.StatusOK, u)
}

// PUT /api/admin/users/{user_id}
func (s *State) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := param(r, "user_id")
	var req struct {
		Username  *string `json:"username"`
		Bio       *string `json:"bio"`
		AvatarURL *string `json:"avatar_url"`
		IsActive  *bool   `json:"is_active"`
		Role      *string `json:"role"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Role != nil && *req.Role != "admin" && *req.Role != "student" {
		s.Error(w, http.StatusBadRequest, "Role must be 'admin' or 'student'")
		return
	}

	u, err := scanUserPublic(s.Pool.QueryRow(r.Context(), `
		UPDATE users SET
			username   = COALESCE($1, username),
			bio        = COALESCE($2, bio),
			avatar_url = COALESCE($3, avatar_url),
			is_active  = COALESCE($4, is_active),
			role       = COALESCE($5, role),
			updated_at = NOW()
		WHERE id = $6::uuid
		RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
		req.Username, req.Bio, req.AvatarURL, req.IsActive, req.Role, userID))
	if err != nil {
		s.Error(w, http.StatusNotFound, "User not found")
		return
	}
	s.JSON(w, http.StatusOK, u)
}

// DELETE /api/admin/users/{user_id}
func (s *State) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := param(r, "user_id")
	if _, err := s.Pool.Exec(r.Context(), "DELETE FROM users WHERE id = $1::uuid", userID); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "User deleted"})
}

// GET /api/admin/users/search?q=...
func (s *State) SearchUsers(w http.ResponseWriter, r *http.Request) {
	q := "%" + strings.ToLower(r.URL.Query().Get("q")) + "%"
	rows, err := s.Pool.Query(r.Context(), `
		SELECT id::text, username, email FROM users
		WHERE LOWER(username) LIKE $1 OR LOWER(email) LIKE $1
		ORDER BY username LIMIT 10`, q)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()
	type result struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	users := make([]result, 0)
	for rows.Next() {
		var u result
		if rows.Scan(&u.ID, &u.Username, &u.Email) == nil {
			users = append(users, u)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"users": users})
}

// PATCH /api/admin/courses/{slug}/settings
func (s *State) UpdateCourseSettings(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	if s.Content.Get(slug) == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	var req struct {
		IsPublished *bool `json:"is_published"`
		AutoEnroll  *bool `json:"auto_enroll"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	_, err := s.Pool.Exec(r.Context(), `
		INSERT INTO course_settings (course_slug, is_published, auto_enroll, source, updated_at)
		VALUES ($1,
		        COALESCE($2, false),
		        COALESCE($3, false),
		        COALESCE((SELECT source FROM course_settings WHERE course_slug = $1), 'local'),
		        NOW())
		ON CONFLICT (course_slug) DO UPDATE SET
		    is_published = COALESCE($2, course_settings.is_published),
		    auto_enroll  = COALESCE($3, course_settings.auto_enroll),
		    updated_at   = NOW()`,
		slug, req.IsPublished, req.AutoEnroll)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	dbSettings := s.loadAllCourseSettings(r.Context())
	c := s.Content.Get(slug)
	s.JSON(w, http.StatusOK, toCourseResponse(c, dbSettings))
}

// GET /api/admin/courses/{slug}/enrollments
func (s *State) ListCourseEnrollments(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	rows, err := s.Pool.Query(r.Context(), `
		SELECT u.id::text, u.username, u.email, e.enrolled_at::text
		FROM enrollments e
		JOIN users u ON u.id = e.user_id
		WHERE e.course_slug = $1
		ORDER BY e.enrolled_at DESC`, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()
	type enrollment struct {
		UserID     string `json:"user_id"`
		Username   string `json:"username"`
		Email      string `json:"email"`
		EnrolledAt string `json:"enrolled_at"`
	}
	list := make([]enrollment, 0)
	for rows.Next() {
		var e enrollment
		if rows.Scan(&e.UserID, &e.Username, &e.Email, &e.EnrolledAt) == nil {
			list = append(list, e)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"enrollments": list})
}

// POST /api/admin/courses/{slug}/enrollments  body: {"user_id": "uuid"}
func (s *State) AdminEnrollUser(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := decode(r, &req); err != nil || req.UserID == "" {
		s.Error(w, http.StatusBadRequest, "user_id is required")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO enrollments (user_id, course_slug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		req.UserID, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "User enrolled"})
}

// DELETE /api/admin/courses/{slug}/enrollments/{user_id}
func (s *State) AdminUnenrollUser(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	userID := param(r, "user_id")
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM enrollments WHERE course_slug = $1 AND user_id = $2::uuid`, slug, userID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "User unenrolled"})
}
