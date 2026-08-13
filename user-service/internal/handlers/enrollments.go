package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/metrics"
	"github.com/genesary/pupitre/user-service/internal/repository"
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
func (s *State) Enroll(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")
	claims := s.claims(req)

	course, err := s.fetchCourseDetails(req.Context(), slug)
	if err != nil {
		zap.L().Warn("could not fetch course details for xp gate", zap.String("courseSlug", slug), zap.Error(err))
	}

	if err == nil && course.XPRequired > 0 {
		total, xpErr := s.Repos.XP.Total(req.Context(), claims.Subject)
		if xpErr != nil {
			zap.L().Error("xp gate check failed", zap.String("userID", claims.Subject), zap.Error(xpErr))
			s.Error(writer, http.StatusInternalServerError, "Database error")

			return
		}

		if total < course.XPRequired {
			s.Error(writer, http.StatusForbidden, "Not enough XP to enroll in this course")

			return
		}
	}

	err = s.Repos.Enrollments.Create(req.Context(), claims.Subject, slug)
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

	err := s.Repos.Enrollments.Delete(r.Context(), claims.Subject, slug)
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
	XPRequired      int    `json:"xpRequired,omitempty"`
}

// fetchCourseDetails fetches a single course's metadata from course-service.
func (s *State) fetchCourseDetails(ctx context.Context, slug string) (*courseServiceCourse, error) {
	courseURL := fmt.Sprintf("%s/api/courses/%s", s.Config.CourseServiceURL, slug)

	//nolint:gosec // courseURL is built from trusted server config (CourseServiceURL) and a validated slug
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, courseURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building course-service request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // see comment above
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
	Completed       bool    `json:"completed"`
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

	slugs := make([]string, len(enrolled))
	for i, row := range enrolled {
		slugs[i] = row.Slug
	}

	completed := s.completedCoursesCtx(req, claims.Subject, slugs)

	courses := s.buildMyCourses(req.Context(), enrolled, completed)

	s.JSON(writer, http.StatusOK, map[string]any{myCoursesRespKeyCourses: courses})
}

// queryMyEnrollments loads the user's enrollments with progress aggregates.
func (s *State) queryMyEnrollments(ctx context.Context, userID string) ([]repository.EnrollmentRow, error) {
	enrolled, err := s.Repos.Enrollments.MyEnrollments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("querying enrollments: %w", err)
	}

	return enrolled, nil
}

// buildMyCourses enriches each enrollment with course-service metadata, falling
// back to the bare slug when course-service is unreachable.
func (s *State) buildMyCourses(ctx context.Context, enrolled []repository.EnrollmentRow, completed map[string]struct{}) []myCourse {
	courses := make([]myCourse, 0, len(enrolled))

	for _, row := range enrolled {
		_, isDone := completed[row.Slug]

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
				Completed:     isDone,
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
			Completed:       isDone,
		})
	}

	return courses
}
