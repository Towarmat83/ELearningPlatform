package handlers

import (
	"net/http"
	"strconv"

	"github.com/elearning/api-go/internal/metrics"
)

type courseRow struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Thumbnail       *string `json:"thumbnail"`
	Category        *string `json:"category"`
	Difficulty      *string `json:"difficulty"`
	IsPublished     bool    `json:"is_published"`
	CreatedBy       string  `json:"created_by"`
	CreatorUsername *string `json:"creator_username"`
	LabCount        int64   `json:"lab_count"`
	EnrollmentCount int64   `json:"enrollment_count"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// GET /api/courses
func (s *State) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.ParseInt(q.Get("page"), 10, 64)
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.ParseInt(q.Get("per_page"), 10, 64)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage
	category := nullStr(q.Get("category"))
	difficulty := nullStr(q.Get("difficulty"))
	search := nullStr(q.Get("search"))

	rows, err := s.Pool.Query(r.Context(), `
		SELECT c.id::text, c.title, c.description, c.thumbnail, c.category, c.difficulty,
		       c.is_published, c.created_by::text, u.username,
		       COUNT(DISTINCT l.id) AS lab_count, COUNT(DISTINCT e.id) AS enrollment_count,
		       c.created_at::text, c.updated_at::text
		FROM courses c
		LEFT JOIN users u ON u.id = c.created_by
		LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
		LEFT JOIN enrollments e ON e.course_id = c.id
		WHERE c.is_published = TRUE
		  AND ($1::text IS NULL OR c.category = $1)
		  AND ($2::text IS NULL OR c.difficulty = $2)
		  AND ($3::text IS NULL OR c.title ILIKE '%' || $3 || '%' OR c.description ILIKE '%' || $3 || '%')
		GROUP BY c.id, u.username
		ORDER BY c.created_at DESC
		LIMIT $4 OFFSET $5`,
		category, difficulty, search, perPage, offset)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	courses := make([]courseRow, 0)
	for rows.Next() {
		var c courseRow
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Thumbnail, &c.Category, &c.Difficulty,
			&c.IsPublished, &c.CreatedBy, &c.CreatorUsername,
			&c.LabCount, &c.EnrollmentCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		courses = append(courses, c)
	}

	var total int64
	s.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM courses WHERE is_published = TRUE
		AND ($1::text IS NULL OR category = $1)
		AND ($2::text IS NULL OR difficulty = $2)
		AND ($3::text IS NULL OR title ILIKE '%' || $3 || '%')`,
		category, difficulty, search).Scan(&total)

	totalPages := int64(0)
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	s.JSON(w, http.StatusOK, map[string]any{
		"courses": courses, "total": total,
		"page": page, "per_page": perPage, "total_pages": totalPages,
	})
}

// GET /api/courses/{id}
func (s *State) GetCourse(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	var c courseRow
	err := s.Pool.QueryRow(r.Context(), `
		SELECT c.id::text, c.title, c.description, c.thumbnail, c.category, c.difficulty,
		       c.is_published, c.created_by::text, u.username,
		       COUNT(DISTINCT l.id) AS lab_count, COUNT(DISTINCT e.id) AS enrollment_count,
		       c.created_at::text, c.updated_at::text
		FROM courses c
		LEFT JOIN users u ON u.id = c.created_by
		LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
		LEFT JOIN enrollments e ON e.course_id = c.id
		WHERE c.id = $1::uuid AND c.is_published = TRUE
		GROUP BY c.id, u.username`, id).
		Scan(&c.ID, &c.Title, &c.Description, &c.Thumbnail, &c.Category, &c.Difficulty,
			&c.IsPublished, &c.CreatedBy, &c.CreatorUsername,
			&c.LabCount, &c.EnrollmentCount, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	s.JSON(w, http.StatusOK, c)
}

// POST /api/courses
func (s *State) CreateCourse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Thumbnail   *string `json:"thumbnail"`
		Category    *string `json:"category"`
		Difficulty  *string `json:"difficulty"`
		IsPublished *bool   `json:"is_published"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Title == "" {
		s.Error(w, http.StatusBadRequest, "Title is required")
		return
	}
	published := false
	if req.IsPublished != nil {
		published = *req.IsPublished
	}
	c := s.claims(r)

	var row courseRow
	err := s.Pool.QueryRow(r.Context(), `
		INSERT INTO courses (title, description, thumbnail, category, difficulty, is_published, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::uuid)
		RETURNING id::text, title, description, thumbnail, category, difficulty,
		          is_published, created_by::text, NULL::text, 0::bigint, 0::bigint,
		          created_at::text, updated_at::text`,
		req.Title, req.Description, req.Thumbnail, req.Category, req.Difficulty, published, c.Subject).
		Scan(&row.ID, &row.Title, &row.Description, &row.Thumbnail, &row.Category, &row.Difficulty,
			&row.IsPublished, &row.CreatedBy, &row.CreatorUsername,
			&row.LabCount, &row.EnrollmentCount, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.ActiveCourses.Inc()
	s.JSON(w, http.StatusOK, row)
}

// PUT /api/courses/{id}
func (s *State) UpdateCourse(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Thumbnail   *string `json:"thumbnail"`
		Category    *string `json:"category"`
		Difficulty  *string `json:"difficulty"`
		IsPublished *bool   `json:"is_published"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ctx := r.Context()
	c := s.claims(r)

	var createdBy string
	if err := s.Pool.QueryRow(ctx, "SELECT created_by::text FROM courses WHERE id = $1::uuid", id).
		Scan(&createdBy); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if c.Role != "admin" && createdBy != c.Subject {
		s.Error(w, http.StatusForbidden, "You don't own this course")
		return
	}

	var row courseRow
	err := s.Pool.QueryRow(ctx, `
		UPDATE courses SET
			title        = COALESCE($1, title),
			description  = COALESCE($2, description),
			thumbnail    = COALESCE($3, thumbnail),
			category     = COALESCE($4, category),
			difficulty   = COALESCE($5, difficulty),
			is_published = COALESCE($6, is_published),
			updated_at   = NOW()
		WHERE id = $7::uuid
		RETURNING id::text, title, description, thumbnail, category, difficulty,
		          is_published, created_by::text, NULL::text, 0::bigint, 0::bigint,
		          created_at::text, updated_at::text`,
		req.Title, req.Description, req.Thumbnail, req.Category, req.Difficulty, req.IsPublished, id).
		Scan(&row.ID, &row.Title, &row.Description, &row.Thumbnail, &row.Category, &row.Difficulty,
			&row.IsPublished, &row.CreatedBy, &row.CreatorUsername,
			&row.LabCount, &row.EnrollmentCount, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, row)
}

// DELETE /api/courses/{id}
func (s *State) DeleteCourse(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	ctx := r.Context()
	c := s.claims(r)

	var createdBy string
	if err := s.Pool.QueryRow(ctx, "SELECT created_by::text FROM courses WHERE id = $1::uuid", id).
		Scan(&createdBy); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if c.Role != "admin" && createdBy != c.Subject {
		s.Error(w, http.StatusForbidden, "You don't own this course")
		return
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM courses WHERE id = $1::uuid", id); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.ActiveCourses.Dec()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Course deleted"})
}

// POST /api/courses/{id}/enroll
func (s *State) Enroll(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	c := s.claims(r)
	var exists int64
	s.Pool.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM courses WHERE id = $1::uuid AND is_published = TRUE", id).Scan(&exists)
	if exists == 0 {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		"INSERT INTO enrollments (user_id, course_id) VALUES ($1::uuid, $2::uuid) ON CONFLICT DO NOTHING",
		c.Subject, id)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Inc()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Enrolled successfully"})
}

// DELETE /api/courses/{id}/unenroll
func (s *State) Unenroll(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	c := s.claims(r)
	_, err := s.Pool.Exec(r.Context(),
		"DELETE FROM enrollments WHERE user_id = $1::uuid AND course_id = $2::uuid",
		c.Subject, id)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Dec()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Unenrolled successfully"})
}

// GET /api/my/courses
func (s *State) MyCourses(w http.ResponseWriter, r *http.Request) {
	c := s.claims(r)
	rows, err := s.Pool.Query(r.Context(), `
		SELECT c.id::text, c.title, c.description, c.thumbnail, c.category, c.difficulty,
		       c.is_published, c.created_by::text,
		       COUNT(DISTINCT l.id) AS lab_count,
		       COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE) AS completed_labs,
		       COALESCE(SUM(lp.best_score), 0) AS total_score,
		       c.created_at::text, c.updated_at::text
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
		LEFT JOIN lab_progress lp ON lp.course_id = c.id AND lp.user_id = $1::uuid
		WHERE e.user_id = $1::uuid
		GROUP BY c.id
		ORDER BY MAX(e.enrolled_at) DESC`, c.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type myCourse struct {
		courseRow
		CompletedLabs int64 `json:"completed_labs"`
		TotalScore    int64 `json:"total_score"`
	}
	courses := make([]myCourse, 0)
	for rows.Next() {
		var mc myCourse
		if err := rows.Scan(
			&mc.ID, &mc.Title, &mc.Description, &mc.Thumbnail, &mc.Category, &mc.Difficulty,
			&mc.IsPublished, &mc.CreatedBy,
			&mc.LabCount, &mc.CompletedLabs, &mc.TotalScore,
			&mc.CreatedAt, &mc.UpdatedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		courses = append(courses, mc)
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// GET /api/courses/{id}/leaderboard
func (s *State) CourseLeaderboard(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	ctx := r.Context()
	c := s.claims(r)

	if c.Role != "admin" {
		var enrolled int64
		s.Pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM enrollments WHERE user_id = $1::uuid AND course_id = $2::uuid",
			c.Subject, id).Scan(&enrolled)
		if enrolled == 0 {
			s.Error(w, http.StatusForbidden, "Enroll to see the leaderboard")
			return
		}
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT u.id::text AS user_id, u.username,
		       COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE)::bigint AS completed_labs,
		       COALESCE(SUM(lp.best_score), 0)::bigint AS total_points,
		       MAX(lp.last_attempt_at)::text AS last_activity
		FROM enrollments e
		JOIN users u ON u.id = e.user_id
		LEFT JOIN lab_progress lp ON lp.user_id = u.id AND lp.course_id = $1::uuid
		WHERE e.course_id = $1::uuid AND u.is_active = TRUE
		GROUP BY u.id, u.username
		ORDER BY total_points DESC, completed_labs DESC
		LIMIT 20`, id)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type entry struct {
		Rank          int     `json:"rank"`
		UserID        string  `json:"user_id"`
		IsMe          bool    `json:"is_me"`
		Username      string  `json:"username"`
		CompletedLabs int64   `json:"completed_labs"`
		TotalPoints   int64   `json:"total_points"`
		LastActivity  *string `json:"last_activity"`
	}
	leaderboard := make([]entry, 0)
	rank := 1
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.UserID, &e.Username, &e.CompletedLabs, &e.TotalPoints, &e.LastActivity); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		e.Rank = rank
		e.IsMe = e.UserID == c.Subject
		leaderboard = append(leaderboard, e)
		rank++
	}
	s.JSON(w, http.StatusOK, map[string]any{"leaderboard": leaderboard})
}

// GET /api/admin/courses
func (s *State) AdminListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.ParseInt(q.Get("page"), 10, 64)
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.ParseInt(q.Get("per_page"), 10, 64)
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage
	search := nullStr(q.Get("search"))

	rows, err := s.Pool.Query(r.Context(), `
		SELECT c.id::text, c.title, c.description, c.thumbnail, c.category, c.difficulty,
		       c.is_published, c.created_by::text, u.username,
		       COUNT(DISTINCT l.id) AS lab_count, COUNT(DISTINCT e.id) AS enrollment_count,
		       c.created_at::text, c.updated_at::text
		FROM courses c
		LEFT JOIN users u ON u.id = c.created_by
		LEFT JOIN labs l ON l.course_id = c.id
		LEFT JOIN enrollments e ON e.course_id = c.id
		WHERE ($1::text IS NULL OR c.title ILIKE '%' || $1 || '%')
		GROUP BY c.id, u.username
		ORDER BY c.created_at DESC
		LIMIT $2 OFFSET $3`, search, perPage, offset)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	courses := make([]courseRow, 0)
	for rows.Next() {
		var c courseRow
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Thumbnail, &c.Category, &c.Difficulty,
			&c.IsPublished, &c.CreatedBy, &c.CreatorUsername,
			&c.LabCount, &c.EnrollmentCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		courses = append(courses, c)
	}
	var total int64
	s.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM courses").Scan(&total)
	s.JSON(w, http.StatusOK, map[string]any{
		"courses": courses, "total": total, "page": page, "per_page": perPage,
	})
}

// GET /api/admin/courses/{course_id}/enrollments
func (s *State) AdminListEnrollments(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	rows, err := s.Pool.Query(r.Context(), `
		SELECT u.id::text, u.username, u.email, e.enrolled_at::text
		FROM enrollments e JOIN users u ON u.id = e.user_id
		WHERE e.course_id = $1::uuid ORDER BY e.enrolled_at DESC`, courseID)
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
	enrollments := make([]enrollment, 0)
	for rows.Next() {
		var e enrollment
		if err := rows.Scan(&e.UserID, &e.Username, &e.Email, &e.EnrolledAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		enrollments = append(enrollments, e)
	}
	s.JSON(w, http.StatusOK, map[string]any{"enrollments": enrollments})
}

// POST /api/admin/courses/{course_id}/enrollments
func (s *State) AdminEnrollUser(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		"INSERT INTO enrollments (user_id, course_id) VALUES ($1::uuid, $2::uuid) ON CONFLICT DO NOTHING",
		body.UserID, courseID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Inc()
	s.JSON(w, http.StatusOK, map[string]string{"message": "User enrolled"})
}

// DELETE /api/admin/courses/{course_id}/enrollments/{user_id}
func (s *State) AdminUnenrollUser(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	userID := param(r, "user_id")
	_, err := s.Pool.Exec(r.Context(),
		"DELETE FROM enrollments WHERE course_id = $1::uuid AND user_id = $2::uuid",
		courseID, userID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Dec()
	s.JSON(w, http.StatusOK, map[string]string{"message": "User unenrolled"})
}
