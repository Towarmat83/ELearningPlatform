package handlers

import (
	"encoding/json"
	"net/http"
)

type labRow struct {
	ID          string          `json:"id"`
	CourseID    string          `json:"course_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	LabType     string          `json:"lab_type"`
	Content     json.RawMessage `json:"content"`
	Points      int32           `json:"points"`
	OrderIndex  int32           `json:"order_index"`
	IsPublished bool            `json:"is_published"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

// stripAnswers removes correct_answer fields from form lab content for students.
func stripAnswers(labType string, content json.RawMessage) json.RawMessage {
	if labType != "form" {
		return content
	}
	var c map[string]json.RawMessage
	if err := json.Unmarshal(content, &c); err != nil {
		return content
	}
	rawQs, ok := c["questions"]
	if !ok {
		return content
	}
	var questions []map[string]json.RawMessage
	if err := json.Unmarshal(rawQs, &questions); err != nil {
		return content
	}
	for i := range questions {
		delete(questions[i], "correct_answer")
	}
	qs, err := json.Marshal(questions)
	if err != nil {
		return content
	}
	c["questions"] = qs
	out, err := json.Marshal(c)
	if err != nil {
		return content
	}
	return out
}

func isEnrolled(s *State, r *http.Request, courseID, userID string) bool {
	var cnt int64
	s.Pool.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM enrollments WHERE user_id = $1::uuid AND course_id = $2::uuid",
		userID, courseID).Scan(&cnt)
	return cnt > 0
}

// GET /api/courses/{course_id}/labs
func (s *State) ListLabs(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	c := s.claims(r)

	if c.Role != "admin" && !isEnrolled(s, r, courseID, c.Subject) {
		s.Error(w, http.StatusForbidden, "You must enroll in this course first")
		return
	}

	query := "SELECT id::text, course_id::text, title, description, lab_type, content::text, points, order_index, is_published, created_at::text, updated_at::text FROM labs WHERE course_id = $1::uuid"
	if c.Role != "admin" {
		query += " AND is_published = TRUE"
	}
	query += " ORDER BY order_index ASC"

	rows, err := s.Pool.Query(r.Context(), query, courseID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	labs := make([]labRow, 0)
	for rows.Next() {
		var lab labRow
		var contentStr string
		if err := rows.Scan(&lab.ID, &lab.CourseID, &lab.Title, &lab.Description, &lab.LabType,
			&contentStr, &lab.Points, &lab.OrderIndex, &lab.IsPublished, &lab.CreatedAt, &lab.UpdatedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		lab.Content = stripAnswers(lab.LabType, json.RawMessage(contentStr))
		labs = append(labs, lab)
	}
	s.JSON(w, http.StatusOK, map[string]any{"labs": labs})
}

// GET /api/courses/{course_id}/labs/{lab_id}
func (s *State) GetLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")
	c := s.claims(r)

	if c.Role != "admin" && !isEnrolled(s, r, courseID, c.Subject) {
		s.Error(w, http.StatusForbidden, "You must enroll in this course first")
		return
	}

	query := "SELECT id::text, course_id::text, title, description, lab_type, content::text, points, order_index, is_published, created_at::text, updated_at::text FROM labs WHERE id = $1::uuid AND course_id = $2::uuid"
	if c.Role != "admin" {
		query += " AND is_published = TRUE"
	}
	var lab labRow
	var contentStr string
	err := s.Pool.QueryRow(r.Context(), query, labID, courseID).
		Scan(&lab.ID, &lab.CourseID, &lab.Title, &lab.Description, &lab.LabType,
			&contentStr, &lab.Points, &lab.OrderIndex, &lab.IsPublished, &lab.CreatedAt, &lab.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lab not found")
		return
	}
	lab.Content = stripAnswers(lab.LabType, json.RawMessage(contentStr))

	// User progress
	var progress *struct {
		Completed     bool    `json:"completed"`
		BestScore     int32   `json:"best_score"`
		TotalAttempts int32   `json:"total_attempts"`
		CompletedAt   *string `json:"completed_at"`
	}
	var completed bool
	var bestScore, totalAttempts int32
	var completedAt *string
	err = s.Pool.QueryRow(r.Context(),
		"SELECT completed, best_score, total_attempts, completed_at::text FROM lab_progress WHERE user_id = $1::uuid AND lab_id = $2::uuid",
		c.Subject, labID).Scan(&completed, &bestScore, &totalAttempts, &completedAt)
	if err == nil {
		progress = &struct {
			Completed     bool    `json:"completed"`
			BestScore     int32   `json:"best_score"`
			TotalAttempts int32   `json:"total_attempts"`
			CompletedAt   *string `json:"completed_at"`
		}{completed, bestScore, totalAttempts, completedAt}
	}

	s.JSON(w, http.StatusOK, map[string]any{"lab": lab, "progress": progress})
}

// POST /api/courses/{course_id}/labs
func (s *State) CreateLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	var req struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		LabType     string          `json:"lab_type"`
		Content     json.RawMessage `json:"content"`
		Flag        *string         `json:"flag"`
		Points      *int32          `json:"points"`
		OrderIndex  *int32          `json:"order_index"`
		IsPublished *bool           `json:"is_published"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.LabType != "form" && req.LabType != "ctf" && req.LabType != "interactive" {
		s.Error(w, http.StatusBadRequest, "lab_type must be 'form', 'ctf', or 'interactive'")
		return
	}
	if req.LabType == "ctf" && req.Flag == nil {
		s.Error(w, http.StatusBadRequest, "CTF labs require a flag")
		return
	}

	ctx := r.Context()
	c := s.claims(r)
	var createdBy string
	if err := s.Pool.QueryRow(ctx, "SELECT created_by::text FROM courses WHERE id = $1::uuid", courseID).
		Scan(&createdBy); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if c.Role != "admin" && createdBy != c.Subject {
		s.Error(w, http.StatusForbidden, "You don't own this course")
		return
	}

	points := int32(100)
	if req.Points != nil {
		points = *req.Points
	}
	orderIndex := int32(0)
	if req.OrderIndex != nil {
		orderIndex = *req.OrderIndex
	}
	published := false
	if req.IsPublished != nil {
		published = *req.IsPublished
	}
	content := req.Content
	if len(content) == 0 {
		content = json.RawMessage("{}")
	}

	var lab labRow
	var contentStr string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO labs (course_id, title, description, lab_type, content, flag, points, order_index, is_published)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)
		RETURNING id::text, course_id::text, title, description, lab_type, content::text, points, order_index, is_published, created_at::text, updated_at::text`,
		courseID, req.Title, req.Description, req.LabType, string(content),
		req.Flag, points, orderIndex, published).
		Scan(&lab.ID, &lab.CourseID, &lab.Title, &lab.Description, &lab.LabType,
			&contentStr, &lab.Points, &lab.OrderIndex, &lab.IsPublished, &lab.CreatedAt, &lab.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	lab.Content = json.RawMessage(contentStr)
	s.JSON(w, http.StatusOK, lab)
}

// PUT /api/courses/{course_id}/labs/{lab_id}
func (s *State) UpdateLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")
	var req struct {
		Title       *string          `json:"title"`
		Description *string          `json:"description"`
		Content     *json.RawMessage `json:"content"`
		Flag        *string          `json:"flag"`
		Points      *int32           `json:"points"`
		OrderIndex  *int32           `json:"order_index"`
		IsPublished *bool            `json:"is_published"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ctx := r.Context()
	c := s.claims(r)

	var createdBy string
	if err := s.Pool.QueryRow(ctx, "SELECT created_by::text FROM courses WHERE id = $1::uuid", courseID).
		Scan(&createdBy); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if c.Role != "admin" && createdBy != c.Subject {
		s.Error(w, http.StatusForbidden, "You don't own this course")
		return
	}

	var contentParam *string
	if req.Content != nil {
		cs := string(*req.Content)
		contentParam = &cs
	}

	var lab labRow
	var contentStr string
	err := s.Pool.QueryRow(ctx, `
		UPDATE labs SET
			title        = COALESCE($1, title),
			description  = COALESCE($2, description),
			content      = COALESCE($3::jsonb, content),
			flag         = COALESCE($4, flag),
			points       = COALESCE($5, points),
			order_index  = COALESCE($6, order_index),
			is_published = COALESCE($7, is_published),
			updated_at   = NOW()
		WHERE id = $8::uuid AND course_id = $9::uuid
		RETURNING id::text, course_id::text, title, description, lab_type, content::text, points, order_index, is_published, created_at::text, updated_at::text`,
		req.Title, req.Description, contentParam, req.Flag,
		req.Points, req.OrderIndex, req.IsPublished, labID, courseID).
		Scan(&lab.ID, &lab.CourseID, &lab.Title, &lab.Description, &lab.LabType,
			&contentStr, &lab.Points, &lab.OrderIndex, &lab.IsPublished, &lab.CreatedAt, &lab.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lab not found")
		return
	}
	lab.Content = json.RawMessage(contentStr)
	s.JSON(w, http.StatusOK, lab)
}

// DELETE /api/courses/{course_id}/labs/{lab_id}
func (s *State) DeleteLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")
	ctx := r.Context()
	c := s.claims(r)

	var createdBy string
	if err := s.Pool.QueryRow(ctx, "SELECT created_by::text FROM courses WHERE id = $1::uuid", courseID).
		Scan(&createdBy); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if c.Role != "admin" && createdBy != c.Subject {
		s.Error(w, http.StatusForbidden, "You don't own this course")
		return
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM labs WHERE id = $1::uuid AND course_id = $2::uuid", labID, courseID); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Lab deleted"})
}

// GET /api/admin/courses/{course_id}/labs/{lab_id}  (exposes flag + correct_answers)
func (s *State) AdminGetLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")

	type adminLabRow struct {
		labRow
		Flag *string `json:"flag"`
	}
	var lab adminLabRow
	var contentStr string
	err := s.Pool.QueryRow(r.Context(), `
		SELECT id::text, course_id::text, title, description, lab_type, content::text,
		       flag, points, order_index, is_published, created_at::text, updated_at::text
		FROM labs WHERE id = $1::uuid AND course_id = $2::uuid`, labID, courseID).
		Scan(&lab.ID, &lab.CourseID, &lab.Title, &lab.Description, &lab.LabType,
			&contentStr, &lab.Flag, &lab.Points, &lab.OrderIndex, &lab.IsPublished,
			&lab.CreatedAt, &lab.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lab not found")
		return
	}
	lab.Content = json.RawMessage(contentStr)
	s.JSON(w, http.StatusOK, lab)
}
