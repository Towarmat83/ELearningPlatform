package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/elearning/user-service/internal/metrics"
)

// myCoursesRespKeyCourses is the JSON response field holding the course list.
const myCoursesRespKeyCourses = "courses"

// Enroll godoc
// @Summary   Enroll in a course
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]string
// @Failure   403   {object}  map[string]string
// @Failure   401   {object}  map[string]string
// @Router    /api/courses/{slug}/enroll [post].
func (s *State) Enroll(writer http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO enrollments (userId, courseSlug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		claims.Subject, slug)
	if err != nil {
		zap.L().Error("enroll failed", zap.String("userId", claims.Subject), zap.String("courseSlug", slug), zap.Error(err))

		if strings.Contains(err.Error(), "foreign key constraint") {
			s.Error(writer, http.StatusUnauthorized, "Session expired, please log in again")
		} else {
			s.Error(writer, http.StatusInternalServerError, "Database error")
		}

		return
	}

	metrics.EnrollmentsTotal.Inc()
	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Enrolled successfully"})
}

// Unenroll godoc
// @Summary   Unenroll from a course
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]string
// @Router    /api/courses/{slug}/unenroll [delete].
func (s *State) Unenroll(writer http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	claims := s.claims(r)

	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM enrollments WHERE userId = $1::uuid AND courseSlug = $2`,
		claims.Subject, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	metrics.EnrollmentsTotal.Dec()
	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Unenrolled successfully"})
}

// courseServiceCourse is the course payload returned by course-service's
// GET /api/courses/{slug} endpoint.
type courseServiceCourse struct {
	Slug            string `json:"slug"`
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Difficulty      string `json:"difficulty"`
	IsPublic        bool   `json:"isPublic"`
	LabCount        int    `json:"labCount"`
	EnrollmentCount int    `json:"enrollmentCount"`
	Source          string `json:"source,omitempty"`
}

// fetchCourseDetails fetches a single course's metadata from course-service.
func (s *State) fetchCourseDetails(ctx context.Context, slug string) (*courseServiceCourse, error) {
	courseURL := fmt.Sprintf("%s/api/courses/%s", s.Config.CourseServiceURL, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, courseURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building course-service request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling course-service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("course-service returned %d", resp.StatusCode)
	}

	var course courseServiceCourse

	err = json.NewDecoder(resp.Body).Decode(&course)
	if err != nil {
		return nil, fmt.Errorf("decoding course-service response: %w", err)
	}

	return &course, nil
}

// myCourse is a course enriched with the current user's progress.
type myCourse struct {
	Slug            string  `json:"slug"`
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Category        string  `json:"category"`
	Difficulty      string  `json:"difficulty"`
	IsPublic        bool    `json:"isPublic"`
	LabCount        int     `json:"labCount"`
	EnrollmentCount int     `json:"enrollmentCount"`
	CompletedLabs   int64   `json:"completedLabs"`
	TotalScore      int64   `json:"totalScore"`
	LastActivity    *string `json:"lastActivity"`
}

// MyCourses godoc
// @Summary   List courses the current user is enrolled in
// @Tags      Enrollments
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/my/courses [get].
func (s *State) MyCourses(writer http.ResponseWriter, req *http.Request) {
	claims := s.claims(req)

	enrolled, err := s.queryMyEnrollments(req.Context(), claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	courses := s.buildMyCourses(req.Context(), enrolled)

	s.JSON(writer, http.StatusOK, map[string]any{myCoursesRespKeyCourses: courses})
}

// enrollmentRow is a single enrollment joined with its progress aggregates.
type enrollmentRow struct {
	Slug          string
	CompletedLabs int64
	TotalScore    int64
	LastActivity  *string
}

// queryMyEnrollments loads the user's enrollments with progress aggregates.
func (s *State) queryMyEnrollments(ctx context.Context, userID string) ([]enrollmentRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT e.courseSlug,
		       COUNT(DISTINCT lp.lessonSlug) + COALESCE(mp.passedModules, 0) AS completedLabs,
		       COALESCE(mp.totalScore, 0) AS totalScore,
		       GREATEST(MAX(lp.viewed_at), mp.lastActivity)::text AS lastActivity
		FROM enrollments e
		LEFT JOIN lesson_progress lp ON lp.userId = e.userId AND lp.courseSlug = e.courseSlug
		LEFT JOIN (
			SELECT courseSlug,
			       SUM(bestScore)                         AS totalScore,
			       COUNT(*) FILTER (WHERE passed)          AS passedModules,
			       MAX(updatedAt)                         AS lastActivity
			FROM module_progress
			WHERE userId = $1::uuid
			GROUP BY courseSlug
		) mp ON mp.courseSlug = e.courseSlug
		WHERE e.userId = $1::uuid
		GROUP BY e.courseSlug, mp.totalScore, mp.passedModules, mp.lastActivity, e.enrolledAt
		ORDER BY e.enrolledAt DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying enrollments: %w", err)
	}
	defer rows.Close()

	var enrolled []enrollmentRow

	for rows.Next() {
		var enrollment enrollmentRow

		err := rows.Scan(&enrollment.Slug, &enrollment.CompletedLabs, &enrollment.TotalScore, &enrollment.LastActivity)
		if err != nil {
			continue
		}

		enrolled = append(enrolled, enrollment)
	}

	return enrolled, nil
}

// buildMyCourses enriches each enrollment with course-service metadata, falling
// back to the bare slug when course-service is unreachable.
func (s *State) buildMyCourses(ctx context.Context, enrolled []enrollmentRow) []myCourse {
	courses := make([]myCourse, 0, len(enrolled))

	for _, row := range enrolled {
		details, err := s.fetchCourseDetails(ctx, row.Slug)
		if err != nil {
			zap.L().Warn("failed to fetch course details", zap.String("slug", row.Slug), zap.Error(err))
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
			IsPublic:        details.IsPublic,
			LabCount:        details.LabCount,
			EnrollmentCount: details.EnrollmentCount,
			CompletedLabs:   row.CompletedLabs,
			TotalScore:      row.TotalScore,
			LastActivity:    row.LastActivity,
		})
	}

	return courses
}
