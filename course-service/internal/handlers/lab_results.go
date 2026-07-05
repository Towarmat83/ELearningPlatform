package handlers

import (
	"net/http"
	"time"
)

// labCheckRow is a single row of the admin lab-checks report, returned
// as-is to the frontend admin dashboard.
type labCheckRow struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	CourseSlug  string    `json:"courseSlug"`
	ModuleIndex int       `json:"moduleIndex"`
	ModuleName  string    `json:"moduleName"`
	Allow       bool      `json:"allow"`
	Violations  []string  `json:"violations"`
	CheckedAt   time.Time `json:"checkedAt"`
}

// GetLabResults godoc
// @Summary   List recorded lab checks (admin)
// @Tags      Admin
// @Security  BearerAuth
// @Produce   json
// @Param     course  query  string  false  "Filter by course slug"
// @Success   200  {array}  labCheckRow
// @Failure   503  {object}  map[string]string
// @Router    /api/admin/lab-checks [get].
func (s *State) GetLabResults(writer http.ResponseWriter, req *http.Request) {
	if s.DB == nil {
		s.Error(writer, http.StatusServiceUnavailable, "database not configured")

		return
	}

	courseSlug := req.URL.Query().Get("course")

	var (
		query string
		args  []any
	)

	if courseSlug != "" {
		query = `SELECT id, username, courseSlug, moduleIndex, moduleName, allow, violations, checkedAt
		          FROM lab_checks WHERE courseSlug = $1 ORDER BY checkedAt DESC LIMIT 500`
		args = []any{courseSlug}
	} else {
		query = `SELECT id, username, courseSlug, moduleIndex, moduleName, allow, violations, checkedAt
		          FROM lab_checks ORDER BY checkedAt DESC LIMIT 500`
	}

	rows, err := s.DB.Query(req.Context(), query, args...)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "db query failed")

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

	s.JSON(writer, http.StatusOK, results)
}
