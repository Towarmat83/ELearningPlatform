package handlers

import (
	"net/http"
	"strings"
)

// AdminStats godoc
// @Summary   Get platform statistics (admin)
// @Tags      Admin - Stats
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/stats [get]
func (s *State) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalUsers, totalEnrollments, totalCourses int64
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE role = 'student'").Scan(&totalUsers)
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments").Scan(&totalEnrollments)
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(DISTINCT course_slug) FROM enrollments").Scan(&totalCourses)

	s.JSON(w, http.StatusOK, map[string]any{
		"total_users":       totalUsers,
		"total_enrollments": totalEnrollments,
		"total_courses":     totalCourses,
		"total_labs":        0,
		"total_submissions": 0,
		"success_rate":      "0",
	})
}

// ListAuthProviders godoc
// @Summary   List distinct auth providers in use (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/users/providers [get]
func (s *State) ListAuthProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT auth_provider, COUNT(*)::bigint
		 FROM users
		 GROUP BY auth_provider
		 ORDER BY auth_provider`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()
	type providerCount struct {
		Provider string `json:"provider"`
		Count    int64  `json:"count"`
	}
	list := make([]providerCount, 0)
	for rows.Next() {
		var p providerCount
		if rows.Scan(&p.Provider, &p.Count) == nil {
			list = append(list, p)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"providers": list})
}

// ListUsers godoc
// @Summary   List all users (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     provider  query  string  false  "Filter by auth provider (local, oidc, ldap, github, gitlab, …)"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/users [get]
func (s *State) ListUsers(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")

	query := `
		SELECT u.id::text, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio,
		       u.auth_provider, u.created_at::text,
		       COUNT(DISTINCT e.course_slug)::bigint AS enrolled_courses
		FROM users u
		LEFT JOIN enrollments e ON e.user_id = u.id`
	var args []any
	if provider != "" {
		query += ` WHERE u.auth_provider = $1`
		args = append(args, provider)
	}
	query += ` GROUP BY u.id ORDER BY u.created_at DESC`

	rows, err := s.Pool.Query(r.Context(), query, args...)
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
		AuthProvider    string  `json:"auth_provider"`
		CreatedAt       string  `json:"created_at"`
		EnrolledCourses int64   `json:"enrolled_courses"`
	}
	users := make([]userAdminRow, 0)
	for rows.Next() {
		var u userAdminRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive,
			&u.AvatarURL, &u.Bio, &u.AuthProvider, &u.CreatedAt, &u.EnrolledCourses); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		users = append(users, u)
	}
	s.JSON(w, http.StatusOK, map[string]any{"users": users, "total": len(users)})
}

// GetUser godoc
// @Summary   Get a user by ID (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     user_id  path  string  true  "User UUID"
// @Success   200   {object}  map[string]interface{}
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/users/{user_id} [get]
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
		AuthProvider    string  `json:"auth_provider"`
		CreatedAt       string  `json:"created_at"`
		EnrolledCourses int64   `json:"enrolled_courses"`
		ViewedLessons   int64   `json:"viewed_lessons"`
	}
	var u userDetailRow
	err := s.Pool.QueryRow(r.Context(), `
		SELECT u.id::text, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio,
		       u.auth_provider, u.created_at::text,
		       COUNT(DISTINCT e.course_slug)::bigint,
		       COUNT(DISTINCT lp.lesson_slug)::bigint
		FROM users u
		LEFT JOIN enrollments e ON e.user_id = u.id
		LEFT JOIN lesson_progress lp ON lp.user_id = u.id
		WHERE u.id = $1::uuid
		GROUP BY u.id`, userID).
		Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive, &u.AvatarURL, &u.Bio,
			&u.AuthProvider, &u.CreatedAt, &u.EnrolledCourses, &u.ViewedLessons)
	if err != nil {
		s.Error(w, http.StatusNotFound, "User not found")
		return
	}
	s.JSON(w, http.StatusOK, u)
}

// UpdateUser godoc
// @Summary   Update a user (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     user_id  path  string  true  "User UUID"
// @Param     body     body  object  true  "Fields: username, bio, avatar_url, is_active, role (all optional)"
// @Success   200   {object}  userPublicRow
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/users/{user_id} [put]
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

// DeleteUser godoc
// @Summary   Delete a user (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     user_id  path  string  true  "User UUID"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/users/{user_id} [delete]
func (s *State) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := param(r, "user_id")
	if _, err := s.Pool.Exec(r.Context(), "DELETE FROM users WHERE id = $1::uuid", userID); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "User deleted"})
}

// SearchUsers godoc
// @Summary   Search users by username or email (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     q  query  string  true  "Search query"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/users/search [get]
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

// ListCourseEnrollments godoc
// @Summary   List enrollments for a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/courses/{slug}/enrollments [get]
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

// AdminEnrollUser godoc
// @Summary   Enroll a user in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Param     body  body  object  true  "user_id (UUID)"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments [post]
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

// AdminUnenrollUser godoc
// @Summary   Unenroll a user from a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug     path  string  true  "Course slug"
// @Param     user_id  path  string  true  "User UUID"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/{user_id} [delete]
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

// AdminEnrollGroup godoc
// @Summary   Enroll all members of a group in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Param     body  body  object  true  "group_id (UUID)"
// @Success   200   {object}  map[string]interface{}
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/groups [post]
func (s *State) AdminEnrollGroup(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	var req struct {
		GroupID string `json:"group_id"`
	}
	if err := decode(r, &req); err != nil || req.GroupID == "" {
		s.Error(w, http.StatusBadRequest, "group_id is required")
		return
	}

	ctx := r.Context()

	// Register the group → course link
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO group_enrollments (group_id, course_slug)
		 VALUES ($1::uuid, $2)
		 ON CONFLICT DO NOTHING`,
		req.GroupID, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Backfill current members
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO enrollments (user_id, course_slug)
		 SELECT ug.user_id, $1
		 FROM user_groups ug
		 WHERE ug.group_id = $2::uuid
		 ON CONFLICT DO NOTHING`,
		slug, req.GroupID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	s.JSON(w, http.StatusOK, map[string]any{
		"message":  "Group enrolled",
		"enrolled": tag.RowsAffected(),
	})
}

// AdminListGroupEnrollments godoc
// @Summary   List groups enrolled in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/courses/{slug}/enrollments/groups [get]
func (s *State) AdminListGroupEnrollments(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	rows, err := s.Pool.Query(r.Context(),
		`SELECT g.id::text, g.name, g.source, COUNT(ug.user_id) AS member_count, ge.created_at::text
		 FROM group_enrollments ge
		 JOIN groups g ON g.id = ge.group_id
		 LEFT JOIN user_groups ug ON ug.group_id = ge.group_id
		 WHERE ge.course_slug = $1
		 GROUP BY g.id, g.name, g.source, ge.created_at
		 ORDER BY g.name`, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type row struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		MemberCount int64  `json:"member_count"`
		EnrolledAt  string `json:"enrolled_at"`
	}
	list := make([]row, 0)
	for rows.Next() {
		var e row
		if rows.Scan(&e.ID, &e.Name, &e.Source, &e.MemberCount, &e.EnrolledAt) == nil {
			list = append(list, e)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"groups": list})
}

// AdminUnenrollGroup godoc
// @Summary   Remove a group enrollment from a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug      path  string  true  "Course slug"
// @Param     group_id  path  string  true  "Group UUID"
// @Success   200  {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/groups/{group_id} [delete]
func (s *State) AdminUnenrollGroup(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	groupID := param(r, "group_id")
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM group_enrollments WHERE group_id = $1::uuid AND course_slug = $2`,
		groupID, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Group enrollment removed"})
}

// SyncProgress godoc
// @Summary   Sync lesson progress from Course Service (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  body  object  true  "user_id, course_slug, lesson_slug"
// @Success   200   {object}  map[string]bool
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/sync-progress [post]
func (s *State) SyncProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     string `json:"user_id"`
		CourseSlug string `json:"course_slug"`
		LessonSlug string `json:"lesson_slug"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.UserID == "" || req.CourseSlug == "" || req.LessonSlug == "" {
		s.Error(w, http.StatusBadRequest, "user_id, course_slug, and lesson_slug are required")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO lesson_progress (user_id, course_slug, lesson_slug)
		 VALUES ($1::uuid, $2, $3)
		 ON CONFLICT (user_id, course_slug, lesson_slug) DO NOTHING`,
		req.UserID, req.CourseSlug, req.LessonSlug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AdminLeaderboard godoc
// @Summary   Get users ranked by total quiz score (admin)
// @Tags      Admin - Stats
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/leaderboard [get]
func (s *State) AdminLeaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(), `
		SELECT u.id::text, u.username, u.email, u.avatar_url,
		       COALESCE(SUM(mp.best_score), 0)::bigint AS total_score,
		       COUNT(DISTINCT CASE WHEN mp.passed THEN mp.course_slug || ':' || mp.module_index::text END)::bigint AS passed_modules,
		       COUNT(DISTINCT e.course_slug)::bigint AS enrolled_courses
		FROM users u
		LEFT JOIN module_progress mp ON mp.user_id = u.id
		LEFT JOIN enrollments e ON e.user_id = u.id
		WHERE u.role = 'student' AND u.is_active = TRUE
		GROUP BY u.id, u.username, u.email, u.avatar_url
		ORDER BY total_score DESC, passed_modules DESC`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()
	type leaderboardRow struct {
		ID              string  `json:"id"`
		Username        string  `json:"username"`
		Email           string  `json:"email"`
		AvatarURL       *string `json:"avatar_url"`
		TotalScore      int64   `json:"total_score"`
		PassedModules   int64   `json:"passed_modules"`
		EnrolledCourses int64   `json:"enrolled_courses"`
	}
	leaderboard := make([]leaderboardRow, 0)
	for rows.Next() {
		var row leaderboardRow
		if err := rows.Scan(&row.ID, &row.Username, &row.Email, &row.AvatarURL,
			&row.TotalScore, &row.PassedModules, &row.EnrolledCourses); err == nil {
			leaderboard = append(leaderboard, row)
		}
	}
	s.JSON(w, http.StatusOK, map[string]any{"leaderboard": leaderboard})
}
