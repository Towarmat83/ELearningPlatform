// Package handlers implements the HTTP handlers for the user-service API.
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Repeated literals used across the admin handlers, pulled out to satisfy
// goconst. Role and "message"/"groups" literals are shared with groups.go
// (groupsRoleAdmin, groupsRoleStudent, groupsRespKeyMessage,
// groupsRespKeyGroups) to avoid duplicate constants in the package.
const (
	adminJSONKeyUsers     = "users"
	adminJSONKeyProviders = "providers"
	adminJSONKeyEnrolled  = "enrolled"
)

// AdminStats godoc
// @Summary   Get platform statistics (admin)
// @Tags      Admin - Stats
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/stats [get].
func (s *State) AdminStats(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()

	var totalUsers, totalEnrollments, totalCourses int64

	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE role = 'student'").Scan(&totalUsers)
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments").Scan(&totalEnrollments)
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(DISTINCT courseSlug) FROM enrollments").Scan(&totalCourses)

	s.JSON(writer, http.StatusOK, map[string]any{
		"total_users":       totalUsers,
		"total_enrollments": totalEnrollments,
		"totalCourses":      totalCourses,
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
// @Router    /api/admin/users/providers [get].
func (s *State) ListAuthProviders(writer http.ResponseWriter, request *http.Request) {
	rows, err := s.Pool.Query(request.Context(),
		`SELECT authProvider, COUNT(*)::bigint
		 FROM users
		 GROUP BY authProvider
		 ORDER BY authProvider`)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

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

	s.JSON(writer, http.StatusOK, map[string]any{adminJSONKeyProviders: list})
}

// userAdminRow is a single user row for the admin listing endpoint.
type userAdminRow struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`

	IsActive bool `json:"isActive"`

	AvatarURL *string `json:"avatarUrl"`
	Bio       *string `json:"bio"`

	AuthProvider string `json:"authProvider"`

	CreatedAt string `json:"createdAt"`

	EnrolledCourses int64 `json:"enrolledCourses"`
}

// ListUsers godoc
// @Summary   List all users (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     provider  query  string  false  "Filter by auth provider"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/users [get].
func (s *State) ListUsers(writer http.ResponseWriter, request *http.Request) {
	provider := request.URL.Query().Get("provider")

	users, err := s.queryAdminUsers(request.Context(), provider)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{adminJSONKeyUsers: users, "total": len(users)})
}

// queryAdminUsers runs the admin user listing query, optionally filtered by
// auth provider, and scans the results into userAdminRow values.
func (s *State) queryAdminUsers(ctx context.Context, provider string) ([]userAdminRow, error) {
	query := `
		SELECT u.id::text, u.username, u.email, u.role, u.isActive, u.avatarUrl, u.bio,
		       u.authProvider, u.createdAt::text,
		       COUNT(DISTINCT e.courseSlug)::bigint AS enrolledCourses
		FROM users u
		LEFT JOIN enrollments e ON e.userId = u.id`

	var args []any

	if provider != "" {
		query += ` WHERE u.authProvider = $1`

		args = append(args, provider)
	}

	query += ` GROUP BY u.id ORDER BY u.createdAt DESC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying admin users: %w", err)
	}
	defer rows.Close()

	users := make([]userAdminRow, 0)

	for rows.Next() {
		var user userAdminRow

		err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.IsActive,
			&user.AvatarURL, &user.Bio, &user.AuthProvider, &user.CreatedAt, &user.EnrolledCourses)
		if err != nil {
			return nil, fmt.Errorf("scanning admin user row: %w", err)
		}

		users = append(users, user)
	}

	return users, nil
}

// GetUser godoc
// @Summary   Get a user by ID (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     userId  path  string  true  "User UUID"
// @Success   200   {object}  map[string]interface{}
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/users/{userId} [get].
func (s *State) GetUser(writer http.ResponseWriter, request *http.Request) {
	userID := param(request, "userId")

	type userDetailRow struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`

		IsActive bool `json:"isActive"`

		AvatarURL *string `json:"avatarUrl"`
		Bio       *string `json:"bio"`

		AuthProvider string `json:"authProvider"`

		CreatedAt string `json:"createdAt"`

		EnrolledCourses int64 `json:"enrolledCourses"`
		ViewedLessons   int64 `json:"viewedLessons"`
	}

	var user userDetailRow

	err := s.Pool.QueryRow(request.Context(), `
		SELECT u.id::text, u.username, u.email, u.role, u.isActive, u.avatarUrl, u.bio,
		       u.authProvider, u.createdAt::text,
		       COUNT(DISTINCT e.courseSlug)::bigint,
		       COUNT(DISTINCT lp.lessonSlug)::bigint
		FROM users u
		LEFT JOIN enrollments e ON e.userId = u.id
		LEFT JOIN lesson_progress lp ON lp.userId = u.id
		WHERE u.id = $1::uuid
		GROUP BY u.id`, userID).
		Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.IsActive, &user.AvatarURL, &user.Bio,
			&user.AuthProvider, &user.CreatedAt, &user.EnrolledCourses, &user.ViewedLessons)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "User not found")

		return
	}

	s.JSON(writer, http.StatusOK, user)
}

// UpdateUser godoc
// @Summary   Update a user (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     userId  path  string  true  "User UUID"
// @Param     body  object  true  "Optional fields to update"
// @Success   200   {object}  userPublicRow
// @Failure   404   {object}  map[string]string
// @Router    /api/admin/users/{userId} [put].
func (s *State) UpdateUser(writer http.ResponseWriter, request *http.Request) {
	userID := param(request, "userId")

	var body struct {
		Username *string `json:"username"`
		Bio      *string `json:"bio"`

		AvatarURL *string `json:"avatarUrl"`

		IsActive *bool   `json:"isActive"`
		Role     *string `json:"role"`
	}

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.Role != nil && *body.Role != groupsRoleAdmin && *body.Role != groupsRoleStudent {
		s.Error(writer, http.StatusBadRequest, "Role must be 'admin' or 'student'")

		return
	}

	user, err := scanUserPublic(s.Pool.QueryRow(request.Context(), `
		UPDATE users SET
			username   = COALESCE($1, username),
			bio        = COALESCE($2, bio),
			avatarUrl = COALESCE($3, avatarUrl),
			isActive  = COALESCE($4, isActive),
			role       = COALESCE($5, role),
			updatedAt = NOW()
		WHERE id = $6::uuid
		RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		body.Username, body.Bio, body.AvatarURL, body.IsActive, body.Role, userID))
	if err != nil {
		s.Error(writer, http.StatusNotFound, "User not found")

		return
	}

	s.JSON(writer, http.StatusOK, user)
}

// DeleteUser godoc
// @Summary   Delete a user (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     userId  path  string  true  "User UUID"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/users/{userId} [delete].
func (s *State) DeleteUser(writer http.ResponseWriter, request *http.Request) {
	userID := param(request, "userId")

	_, err := s.Pool.Exec(request.Context(), "DELETE FROM users WHERE id = $1::uuid", userID)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "User deleted"})
}

// SearchUsers godoc
// @Summary   Search users by username or email (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Produce   json
// @Param     q  query  string  true  "Search query"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/users/search [get].
func (s *State) SearchUsers(writer http.ResponseWriter, request *http.Request) {
	q := "%" + strings.ToLower(request.URL.Query().Get("q")) + "%"

	rows, err := s.Pool.Query(request.Context(), `
		SELECT id::text, username, email FROM users
		WHERE LOWER(username) LIKE $1 OR LOWER(email) LIKE $1
		ORDER BY username LIMIT 10`, q)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

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

	s.JSON(writer, http.StatusOK, map[string]any{adminJSONKeyUsers: users})
}

// ListCourseEnrollments godoc
// @Summary   List enrollments for a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/courses/{slug}/enrollments [get].
func (s *State) ListCourseEnrollments(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	rows, err := s.Pool.Query(request.Context(), `
		SELECT u.id::text, u.username, u.email, e.enrolledAt::text
		FROM enrollments e
		JOIN users u ON u.id = e.userId
		WHERE e.courseSlug = $1
		ORDER BY e.enrolledAt DESC`, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type enrollment struct {
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Email    string `json:"email"`

		EnrolledAt string `json:"enrolledAt"`
	}

	list := make([]enrollment, 0)

	for rows.Next() {
		var e enrollment
		if rows.Scan(&e.UserID, &e.Username, &e.Email, &e.EnrolledAt) == nil {
			list = append(list, e)
		}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"enrollments": list})
}

// AdminEnrollUser godoc
// @Summary   Enroll a user in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Param     body  object  true  "userId (UUID)"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments [post].
func (s *State) AdminEnrollUser(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	var body struct {
		UserID string `json:"userId"`
	}

	err := decode(request, &body)
	if err != nil || body.UserID == "" {
		s.Error(writer, http.StatusBadRequest, "userId is required")

		return
	}

	_, err = s.Pool.Exec(request.Context(),
		`INSERT INTO enrollments (userId, courseSlug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		body.UserID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "User enrolled"})
}

// AdminUnenrollUser godoc
// @Summary   Unenroll a user from a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug     path  string  true  "Course slug"
// @Param     userId  path  string  true  "User UUID"
// @Success   200   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/{userId} [delete].
func (s *State) AdminUnenrollUser(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")
	userID := param(request, "userId")

	_, err := s.Pool.Exec(request.Context(),
		`DELETE FROM enrollments WHERE courseSlug = $1 AND userId = $2::uuid`, slug, userID)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "User unenrolled"})
}

// AdminEnrollGroup godoc
// @Summary   Enroll all members of a group in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Param     body  object  true  "groupId (UUID)"
// @Success   200   {object}  map[string]interface{}
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/groups [post].
func (s *State) AdminEnrollGroup(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	var body struct {
		GroupID string `json:"groupId"`
	}

	err := decode(request, &body)
	if err != nil || body.GroupID == "" {
		s.Error(writer, http.StatusBadRequest, "groupId is required")

		return
	}

	ctx := request.Context()

	// Register the group → course link
	_, err = s.Pool.Exec(ctx,
		`INSERT INTO group_enrollments (groupId, courseSlug)
		 VALUES ($1::uuid, $2)
		 ON CONFLICT DO NOTHING`,
		body.GroupID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	// Backfill current members
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO enrollments (userId, courseSlug)
		 SELECT ug.userId, $1
		 FROM user_groups ug
		 WHERE ug.groupId = $2::uuid
		 ON CONFLICT DO NOTHING`,
		slug, body.GroupID)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		groupsRespKeyMessage: "Group enrolled",
		adminJSONKeyEnrolled: tag.RowsAffected(),
	})
}

// AdminListGroupEnrollments godoc
// @Summary   List groups enrolled in a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/courses/{slug}/enrollments/groups [get].
func (s *State) AdminListGroupEnrollments(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	rows, err := s.Pool.Query(request.Context(),
		`SELECT g.id::text, g.name, g.source, COUNT(ug.userId) AS memberCount, ge.createdAt::text
		 FROM group_enrollments ge
		 JOIN groups g ON g.id = ge.groupId
		 LEFT JOIN user_groups ug ON ug.groupId = ge.groupId
		 WHERE ge.courseSlug = $1
		 GROUP BY g.id, g.name, g.source, ge.createdAt
		 ORDER BY g.name`, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type row struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Source string `json:"source"`

		MemberCount int64 `json:"memberCount"`

		EnrolledAt string `json:"enrolledAt"`
	}

	list := make([]row, 0)

	for rows.Next() {
		var e row
		if rows.Scan(&e.ID, &e.Name, &e.Source, &e.MemberCount, &e.EnrolledAt) == nil {
			list = append(list, e)
		}
	}

	s.JSON(writer, http.StatusOK, map[string]any{groupsRespKeyGroups: list})
}

// AdminUnenrollGroup godoc
// @Summary   Remove a group enrollment from a course (admin)
// @Tags      Admin - Courses
// @Security  BearerAuth
// @Produce   json
// @Param     slug      path  string  true  "Course slug"
// @Param     groupId  path  string  true  "Group UUID"
// @Success   200  {object}  map[string]string
// @Router    /api/admin/courses/{slug}/enrollments/groups/{groupId} [delete].
func (s *State) AdminUnenrollGroup(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")
	groupID := param(request, "groupId")

	_, err := s.Pool.Exec(request.Context(),
		`DELETE FROM group_enrollments WHERE groupId = $1::uuid AND courseSlug = $2`,
		groupID, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Group enrollment removed"})
}

// SyncProgress godoc
// @Summary   Sync lesson progress from Course Service (admin)
// @Tags      Admin - Users
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  object  true  "userId, courseSlug, lessonSlug"
// @Success   200   {object}  map[string]bool
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/sync-progress [post].
func (s *State) SyncProgress(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		UserID string `json:"userId"`

		CourseSlug string `json:"courseSlug"`

		LessonSlug string `json:"lessonSlug"`
	}

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.UserID == "" || body.CourseSlug == "" || body.LessonSlug == "" {
		s.Error(writer, http.StatusBadRequest, "userId, courseSlug, and lessonSlug are required")

		return
	}

	_, err = s.Pool.Exec(request.Context(),
		`INSERT INTO lesson_progress (userId, courseSlug, lessonSlug)
		 VALUES ($1::uuid, $2, $3)
		 ON CONFLICT (userId, courseSlug, lessonSlug) DO NOTHING`,
		body.UserID, body.CourseSlug, body.LessonSlug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// AdminLeaderboard godoc
// @Summary   Get users ranked by total quiz score (admin)
// @Tags      Admin - Stats
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/admin/leaderboard [get].
func (s *State) AdminLeaderboard(writer http.ResponseWriter, request *http.Request) {
	rows, err := s.Pool.Query(request.Context(), `
		SELECT u.id::text, u.username, u.email, u.avatarUrl,
		       COALESCE(SUM(mp.bestScore), 0)::bigint AS totalScore,
		       COUNT(DISTINCT CASE WHEN mp.passed THEN mp.courseSlug || ':' || mp.moduleIndex::text END)::bigint AS passedModules,
		       COUNT(DISTINCT e.courseSlug)::bigint AS enrolledCourses
		FROM users u
		LEFT JOIN module_progress mp ON mp.userId = u.id
		LEFT JOIN enrollments e ON e.userId = u.id
		WHERE u.role = 'student' AND u.isActive = TRUE
		GROUP BY u.id, u.username, u.email, u.avatarUrl
		ORDER BY totalScore DESC, passedModules DESC`)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	type leaderboardRow struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`

		AvatarURL *string `json:"avatarUrl"`

		TotalScore int64 `json:"totalScore"`

		PassedModules int64 `json:"passedModules"`

		EnrolledCourses int64 `json:"enrolledCourses"`
	}

	leaderboard := make([]leaderboardRow, 0)

	for rows.Next() {
		var row leaderboardRow

		err := rows.Scan(&row.ID, &row.Username, &row.Email, &row.AvatarURL,
			&row.TotalScore, &row.PassedModules, &row.EnrolledCourses)
		if err == nil {
			leaderboard = append(leaderboard, row)
		}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"leaderboard": leaderboard})
}
