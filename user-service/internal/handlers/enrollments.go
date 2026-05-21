package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/elearning/user-service/internal/metrics"
)

// Enroll godoc
// @Summary   Enroll in a course
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]string
// @Failure   401   {object}  map[string]string
// @Router    /api/courses/{slug}/enroll [post]
func (s *State) Enroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)

	// Check course prerequisites (skip for admins)
	if claims.Role != "admin" {
		if details, err := s.fetchCourseDetails(slug); err == nil && len(details.Prerequisites) > 0 {
			for _, prereq := range details.Prerequisites {
				if err := s.checkPrerequisite(r.Context(), claims.Subject, prereq); err != nil {
					s.Error(w, http.StatusForbidden, err.Error())
					return
				}
			}
		}
	}

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO enrollments (user_id, course_slug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		claims.Subject, slug)
	if err != nil {
		slog.Error("enroll failed", "user_id", claims.Subject, "course_slug", slug, "err", err)
		if strings.Contains(err.Error(), "foreign key constraint") {
			s.Error(w, http.StatusUnauthorized, "Session expired, please log in again")
		} else {
			s.Error(w, http.StatusInternalServerError, "Database error")
		}
		return
	}
	metrics.EnrollmentsTotal.Inc()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Enrolled successfully"})
}

// Unenroll godoc
// @Summary   Unenroll from a course
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]string
// @Router    /api/courses/{slug}/unenroll [delete]
func (s *State) Unenroll(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		claims.Subject, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	metrics.EnrollmentsTotal.Dec()
	s.JSON(w, http.StatusOK, map[string]string{"message": "Unenrolled successfully"})
}

// coursePrerequisiteJSON mirrors content.CoursePrerequisite for JSON decoding.
// Defined locally to avoid importing course-service packages.
type coursePrerequisiteJSON struct {
	Course   string   `json:"course"`
	MinScore int      `json:"min_score,omitempty"`
	Modules  []string `json:"modules,omitempty"`
}

type courseServiceCourse struct {
	Slug            string                   `json:"slug"`
	ID              string                   `json:"id"`
	Title           string                   `json:"title"`
	Description     string                   `json:"description"`
	Category        string                   `json:"category"`
	Difficulty      string                   `json:"difficulty"`
	IsPublished     bool                     `json:"is_published"`
	LabCount        int                      `json:"lab_count"`
	EnrollmentCount int                      `json:"enrollment_count"`
	Prerequisites   []coursePrerequisiteJSON `json:"prerequisites,omitempty"`
	Source          string                   `json:"source,omitempty"`
}

func (s *State) fetchCourseDetails(slug string) (*courseServiceCourse, error) {
	url := fmt.Sprintf("%s/api/courses/%s", s.Config.CourseServiceURL, slug)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("course-service returned %d", resp.StatusCode)
	}
	var c courseServiceCourse
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// MyCourses godoc
// @Summary   List courses the current user is enrolled in
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/my/courses [get]
func (s *State) MyCourses(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	rows, err := s.Pool.Query(r.Context(), `
		SELECT e.course_slug,
		       COUNT(DISTINCT lp.lesson_slug) + COALESCE(mp.passed_modules, 0) AS completed_labs,
		       COALESCE(mp.total_score, 0) AS total_score,
		       GREATEST(MAX(lp.viewed_at), mp.last_activity)::text AS last_activity
		FROM enrollments e
		LEFT JOIN lesson_progress lp ON lp.user_id = e.user_id AND lp.course_slug = e.course_slug
		LEFT JOIN (
			SELECT course_slug,
			       SUM(best_score)                         AS total_score,
			       COUNT(*) FILTER (WHERE passed)          AS passed_modules,
			       MAX(updated_at)                         AS last_activity
			FROM module_progress
			WHERE user_id = $1::uuid
			GROUP BY course_slug
		) mp ON mp.course_slug = e.course_slug
		WHERE e.user_id = $1::uuid
		GROUP BY e.course_slug, mp.total_score, mp.passed_modules, mp.last_activity, e.enrolled_at
		ORDER BY e.enrolled_at DESC`, claims.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type enrollmentRow struct {
		Slug          string
		CompletedLabs int64
		TotalScore    int64
		LastActivity  *string
	}

	var rowsData []enrollmentRow
	for rows.Next() {
		var r enrollmentRow
		if err := rows.Scan(&r.Slug, &r.CompletedLabs, &r.TotalScore, &r.LastActivity); err != nil {
			continue
		}
		rowsData = append(rowsData, r)
	}

	type myCourse struct {
		Slug            string  `json:"slug"`
		ID              string  `json:"id"`
		Title           string  `json:"title"`
		Description     string  `json:"description"`
		Category        string  `json:"category"`
		Difficulty      string  `json:"difficulty"`
		IsPublished     bool    `json:"is_published"`
		LabCount        int     `json:"lab_count"`
		EnrollmentCount int     `json:"enrollment_count"`
		CompletedLabs   int64   `json:"completed_labs"`
		TotalScore      int64   `json:"total_score"`
		LastActivity    *string `json:"last_activity"`
	}

	courses := make([]myCourse, 0, len(rowsData))
	for _, row := range rowsData {
		details, err := s.fetchCourseDetails(row.Slug)
		if err != nil {
			slog.Warn("failed to fetch course details", "slug", row.Slug, "err", err)
			courses = append(courses, myCourse{
				Slug:          row.Slug,
				ID:            row.Slug,
				Title:         row.Slug,
				CompletedLabs: row.CompletedLabs,
				TotalScore:    row.TotalScore,
				LastActivity:  row.LastActivity,
			})
			continue
		}
		courses = append(courses, myCourse{
			Slug:            details.Slug,
			ID:              details.ID,
			Title:           details.Title,
			Description:     details.Description,
			Category:        details.Category,
			Difficulty:      details.Difficulty,
			IsPublished:     details.IsPublished,
			LabCount:        details.LabCount,
			EnrollmentCount: details.EnrollmentCount,
			CompletedLabs:   row.CompletedLabs,
			TotalScore:      row.TotalScore,
			LastActivity:    row.LastActivity,
		})
	}
	if courses == nil {
		courses = make([]myCourse, 0)
	}
	s.JSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// checkPrerequisite validates one prerequisite condition for the given user.
// Returns a descriptive error if the condition is not met, nil otherwise.
func (s *State) checkPrerequisite(ctx context.Context, userID string, p coursePrerequisiteJSON) error {
	// 1. Enrollment check (required for all modes)
	var enrolled bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) > 0 FROM enrollments WHERE user_id = $1::uuid AND course_slug = $2`,
		userID, p.Course).Scan(&enrolled); err != nil {
		return fmt.Errorf("prerequisite check failed for '%s'", p.Course)
	}
	if !enrolled {
		return fmt.Errorf("you must be enrolled in '%s' before enrolling in this course", p.Course)
	}

	// 2. Minimum score check
	if p.MinScore > 0 {
		var totalScore int
		if err := s.Pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(best_score), 0) FROM module_progress
			 WHERE user_id = $1::uuid AND course_slug = $2`,
			userID, p.Course).Scan(&totalScore); err != nil {
			return fmt.Errorf("prerequisite score check failed for '%s'", p.Course)
		}
		if totalScore < p.MinScore {
			return fmt.Errorf("you need at least %d points in '%s' (you currently have %d)", p.MinScore, p.Course, totalScore)
		}
	}

	// 3. Required modules check
	if len(p.Modules) > 0 {
		rows, err := s.Pool.Query(ctx,
			`SELECT module_slug FROM module_progress
			 WHERE user_id = $1::uuid AND course_slug = $2
			   AND passed = true AND module_slug IS NOT NULL`,
			userID, p.Course)
		if err != nil {
			return fmt.Errorf("prerequisite module check failed for '%s'", p.Course)
		}
		defer rows.Close()
		passed := make(map[string]bool)
		for rows.Next() {
			var slug string
			if rows.Scan(&slug) == nil {
				passed[slug] = true
			}
		}
		for _, required := range p.Modules {
			if !passed[required] {
				return fmt.Errorf("you must pass module '%s' in course '%s' before enrolling", required, p.Course)
			}
		}
	}

	return nil
}
