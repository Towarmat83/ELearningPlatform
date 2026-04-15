package handlers

import (
	"encoding/json"
	"net/http"
)

// ── Row types ─────────────────────────────────────────────────────────────────

// lessonAdminRow is returned to admins (includes content + no viewed field).
type lessonAdminRow struct {
	ID          string          `json:"id"`
	CourseID    string          `json:"course_id"`
	Title       string          `json:"title"`
	OrderIndex  int             `json:"order_index"`
	IsPublished bool            `json:"is_published"`
	Content     json.RawMessage `json:"content"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// lessonStudentRow is returned to students (no content in list, has viewed).
type lessonStudentRow struct {
	ID          string `json:"id"`
	CourseID    string `json:"course_id"`
	Title       string `json:"title"`
	OrderIndex  int    `json:"order_index"`
	IsPublished bool   `json:"is_published"`
	Viewed      bool   `json:"viewed"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// lessonDetailRow is returned for the single-lesson GET.
type lessonDetailRow struct {
	ID          string          `json:"id"`
	CourseID    string          `json:"course_id"`
	Title       string          `json:"title"`
	OrderIndex  int             `json:"order_index"`
	IsPublished bool            `json:"is_published"`
	Viewed      bool            `json:"viewed"`
	Content     json.RawMessage `json:"content"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// ── Student endpoints ──────────────────────────────────────────────────────────

// GET /api/courses/{course_id}/lessons
// Admins: all lessons with full content.
// Students: published lessons only, with viewed flag, no content (for performance).
func (s *State) ListLessons(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	c := s.claims(r)
	ctx := r.Context()

	var exists bool
	if err := s.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM courses WHERE id = $1::uuid)", courseID).Scan(&exists); err != nil || !exists {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}

	if c.Role == "admin" {
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, course_id::text, title, order_index, is_published,
			       content, created_at::text, updated_at::text
			FROM lessons
			WHERE course_id = $1::uuid
			ORDER BY order_index, created_at`, courseID)
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		lessons := make([]lessonAdminRow, 0)
		for rows.Next() {
			var lr lessonAdminRow
			if err := rows.Scan(&lr.ID, &lr.CourseID, &lr.Title, &lr.OrderIndex,
				&lr.IsPublished, &lr.Content, &lr.CreatedAt, &lr.UpdatedAt); err != nil {
				s.Error(w, http.StatusInternalServerError, "Scan error")
				return
			}
			lessons = append(lessons, lr)
		}
		if err := rows.Err(); err != nil {
			s.Error(w, http.StatusInternalServerError, "Query error")
			return
		}
		s.JSON(w, http.StatusOK, map[string]any{"lessons": lessons})
		return
	}

	// Student: must be enrolled
	var enrolled bool
	s.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id = $1::uuid AND user_id = $2::uuid)",
		courseID, c.Subject).Scan(&enrolled) //nolint:errcheck
	if !enrolled {
		s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
		return
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT l.id::text, l.course_id::text, l.title, l.order_index, l.is_published,
		       l.created_at::text, l.updated_at::text,
		       (lp.lesson_id IS NOT NULL) AS viewed
		FROM lessons l
		LEFT JOIN lesson_progress lp ON lp.lesson_id = l.id AND lp.user_id = $2::uuid
		WHERE l.course_id = $1::uuid AND l.is_published = TRUE
		ORDER BY l.order_index, l.created_at`, courseID, c.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	lessons := make([]lessonStudentRow, 0)
	for rows.Next() {
		var lr lessonStudentRow
		if err := rows.Scan(&lr.ID, &lr.CourseID, &lr.Title, &lr.OrderIndex,
			&lr.IsPublished, &lr.CreatedAt, &lr.UpdatedAt, &lr.Viewed); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		lessons = append(lessons, lr)
	}
	if err := rows.Err(); err != nil {
		s.Error(w, http.StatusInternalServerError, "Query error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{"lessons": lessons})
}

// GET /api/courses/{course_id}/lessons/{lesson_id}
func (s *State) GetLesson(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	lessonID := param(r, "lesson_id")
	c := s.claims(r)
	ctx := r.Context()

	if c.Role != "admin" {
		var enrolled bool
		s.Pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id = $1::uuid AND user_id = $2::uuid)",
			courseID, c.Subject).Scan(&enrolled) //nolint:errcheck
		if !enrolled {
			s.Error(w, http.StatusForbidden, "Enroll in this course to access lessons")
			return
		}
	}

	var ld lessonDetailRow
	var err error

	if c.Role == "admin" {
		err = s.Pool.QueryRow(ctx, `
			SELECT id::text, course_id::text, title, order_index, is_published,
			       content, created_at::text, updated_at::text
			FROM lessons
			WHERE id = $1::uuid AND course_id = $2::uuid`,
			lessonID, courseID).Scan(
			&ld.ID, &ld.CourseID, &ld.Title, &ld.OrderIndex, &ld.IsPublished,
			&ld.Content, &ld.CreatedAt, &ld.UpdatedAt)
	} else {
		err = s.Pool.QueryRow(ctx, `
			SELECT l.id::text, l.course_id::text, l.title, l.order_index, l.is_published,
			       l.content, l.created_at::text, l.updated_at::text,
			       (lp.lesson_id IS NOT NULL) AS viewed
			FROM lessons l
			LEFT JOIN lesson_progress lp ON lp.lesson_id = l.id AND lp.user_id = $3::uuid
			WHERE l.id = $1::uuid AND l.course_id = $2::uuid AND l.is_published = TRUE`,
			lessonID, courseID, c.Subject).Scan(
			&ld.ID, &ld.CourseID, &ld.Title, &ld.OrderIndex, &ld.IsPublished,
			&ld.Content, &ld.CreatedAt, &ld.UpdatedAt, &ld.Viewed)
	}
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{"lesson": ld})
}

// POST /api/courses/{course_id}/lessons/{lesson_id}/complete
func (s *State) MarkLessonComplete(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	lessonID := param(r, "lesson_id")
	c := s.claims(r)
	ctx := r.Context()

	var enrolled bool
	s.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id = $1::uuid AND user_id = $2::uuid)",
		courseID, c.Subject).Scan(&enrolled) //nolint:errcheck
	if !enrolled {
		s.Error(w, http.StatusForbidden, "Not enrolled")
		return
	}

	var exists bool
	s.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM lessons WHERE id = $1::uuid AND course_id = $2::uuid AND is_published = TRUE)",
		lessonID, courseID).Scan(&exists) //nolint:errcheck
	if !exists {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO lesson_progress (user_id, lesson_id)
		VALUES ($1::uuid, $2::uuid)
		ON CONFLICT (user_id, lesson_id) DO NOTHING`,
		c.Subject, lessonID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Lesson marked as complete"})
}

// ── Admin lesson endpoints ─────────────────────────────────────────────────────

type createLessonRequest struct {
	Title       string           `json:"title"`
	OrderIndex  int              `json:"order_index"`
	IsPublished bool             `json:"is_published"`
	// Content is optional on update: nil means "do not change existing content"
	Content *json.RawMessage `json:"content"`
}

// POST /api/admin/courses/{course_id}/lessons
func (s *State) AdminCreateLesson(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	ctx := r.Context()

	var req createLessonRequest
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Title == "" {
		s.Error(w, http.StatusBadRequest, "title required")
		return
	}
	content := json.RawMessage("[]")
	if req.Content != nil {
		content = *req.Content
	}

	var ld lessonAdminRow
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO lessons (course_id, title, order_index, is_published, content)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text, course_id::text, title, order_index, is_published,
		          content, created_at::text, updated_at::text`,
		courseID, req.Title, req.OrderIndex, req.IsPublished, content).
		Scan(&ld.ID, &ld.CourseID, &ld.Title, &ld.OrderIndex, &ld.IsPublished,
			&ld.Content, &ld.CreatedAt, &ld.UpdatedAt)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	s.JSON(w, http.StatusCreated, map[string]any{"lesson": ld})
}

// PUT /api/admin/courses/{course_id}/lessons/{lesson_id}
// If "content" field is absent (null pointer), the existing content is preserved.
func (s *State) AdminUpdateLesson(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	lessonID := param(r, "lesson_id")
	ctx := r.Context()

	var req createLessonRequest
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Title == "" {
		s.Error(w, http.StatusBadRequest, "title required")
		return
	}

	var ld lessonAdminRow
	var err error

	if req.Content == nil {
		// Partial update: preserve existing content
		err = s.Pool.QueryRow(ctx, `
			UPDATE lessons SET
				title       = $3,
				order_index = $4,
				is_published = $5,
				updated_at  = NOW()
			WHERE id = $1::uuid AND course_id = $2::uuid
			RETURNING id::text, course_id::text, title, order_index, is_published,
			          content, created_at::text, updated_at::text`,
			lessonID, courseID, req.Title, req.OrderIndex, req.IsPublished).
			Scan(&ld.ID, &ld.CourseID, &ld.Title, &ld.OrderIndex, &ld.IsPublished,
				&ld.Content, &ld.CreatedAt, &ld.UpdatedAt)
	} else {
		err = s.Pool.QueryRow(ctx, `
			UPDATE lessons SET
				title       = $3,
				order_index = $4,
				is_published = $5,
				content     = $6,
				updated_at  = NOW()
			WHERE id = $1::uuid AND course_id = $2::uuid
			RETURNING id::text, course_id::text, title, order_index, is_published,
			          content, created_at::text, updated_at::text`,
			lessonID, courseID, req.Title, req.OrderIndex, req.IsPublished, *req.Content).
			Scan(&ld.ID, &ld.CourseID, &ld.Title, &ld.OrderIndex, &ld.IsPublished,
				&ld.Content, &ld.CreatedAt, &ld.UpdatedAt)
	}
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{"lesson": ld})
}

// DELETE /api/admin/courses/{course_id}/lessons/{lesson_id}
func (s *State) AdminDeleteLesson(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	lessonID := param(r, "lesson_id")
	ctx := r.Context()

	tag, err := s.Pool.Exec(ctx,
		"DELETE FROM lessons WHERE id = $1::uuid AND course_id = $2::uuid",
		lessonID, courseID)
	if err != nil || tag.RowsAffected() == 0 {
		s.Error(w, http.StatusNotFound, "Lesson not found")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Lesson deleted"})
}
