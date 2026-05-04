package handlers

import (
	"net/http"
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
