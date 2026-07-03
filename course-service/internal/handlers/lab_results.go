package handlers

import (
	"net/http"
	"time"
)

type labCheckRow struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	CourseSlug  string    `json:"course_slug"`
	ModuleIndex int       `json:"module_index"`
	ModuleName  string    `json:"module_name"`
	Allow       bool      `json:"allow"`
	Violations  []string  `json:"violations"`
	CheckedAt   time.Time `json:"checked_at"`
}

func (s *State) GetLabResults(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		s.Error(w, http.StatusServiceUnavailable, "database not configured")

		return
	}

	courseSlug := r.URL.Query().Get("course")

	var (
		query string
		args  []any
	)

	if courseSlug != "" {
		query = `SELECT id, username, course_slug, module_index, module_name, allow, violations, checked_at
		          FROM lab_checks WHERE course_slug = $1 ORDER BY checked_at DESC LIMIT 500`
		args = []any{courseSlug}
	} else {
		query = `SELECT id, username, course_slug, module_index, module_name, allow, violations, checked_at
		          FROM lab_checks ORDER BY checked_at DESC LIMIT 500`
	}

	rows, err := s.DB.Query(r.Context(), query, args...)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "db query failed")

		return
	}
	defer rows.Close()

	results := make([]labCheckRow, 0)

	for rows.Next() {
		var row labCheckRow

		err := rows.Scan(&row.ID, &row.Username, &row.CourseSlug, &row.ModuleIndex,
			&row.ModuleName, &row.Allow, &row.Violations, &row.CheckedAt)
		if err != nil {
			continue
		}

		results = append(results, row)
	}

	s.JSON(w, http.StatusOK, results)
}
