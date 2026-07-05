package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// Course-status literals shared by buildCourseStatuses and its callers,
// pulled out to satisfy goconst.
const (
	pathStatusLocked    = "locked"
	pathStatusCompleted = "completed"
	pathStatusAvailable = "available"
)

// parsePagination reads optional limit/offset query params. A zero,
// negative, or omitted limit maps to a nil *int, passed through to SQL
// as LIMIT NULL (unbounded); pass limit=-1 to request everything.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func parsePagination(request *http.Request) (*int, int) {
	var limit *int

	limitVal, err := strconv.Atoi(request.URL.Query().Get("limit"))
	if err == nil && limitVal > 0 {
		limit = &limitVal
	}

	offset := 0

	offsetVal, err := strconv.Atoi(request.URL.Query().Get("offset"))
	if err == nil && offsetVal > 0 {
		offset = offsetVal
	}

	return limit, offset
}

// pathDetail is the learning-path metadata fetched from course-service.
type pathDetail struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Courses     []string `json:"courses"`
}

// courseStatus is a single course's completion state within a path.
type courseStatus struct {
	Slug   string `json:"slug"`
	Status string `json:"status"` // "completed", "available", "locked"
}

// myPath is a learning path the current user is enrolled in, with
// per-course completion status.
type myPath struct {
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	EnrolledAt  time.Time      `json:"enrolledAt"`
	Courses     []courseStatus `json:"courses"`
}

// enrollment is a raw path_enrollments row: which path, and when the
// current user enrolled in it.
type enrollment struct {
	slug       string
	enrolledAt time.Time
}

// enrolledUser represents a user enrolled in a learning path,
// along with their progress across the path's courses.
type enrolledUser struct {
	// UserID is the UUID of the enrolled user.
	UserID string `json:"userId"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Role is the user's platform role (e.g. "student", "admin").
	Role string `json:"role"`
	// EnrolledAt is when the user was enrolled in this path.
	EnrolledAt time.Time `json:"enrolledAt"`
	// CompletedCourses is the count of courses this user finished in the path.
	CompletedCourses int `json:"completedCourses"`
	// TotalCourses is the total number of courses in the path.
	TotalCourses int `json:"totalCourses"`
	// Courses holds per-course completion, populated when detail is available.
	Courses []courseStatus `json:"courses,omitempty"`
}

// buildCourseStatuses derives each course's completion status in an
// ordered path, given the set of courses the user has completed. Status
// is "completed" if done, "available" if the previous course is done
// (or it is the first course), and "locked" otherwise.
func buildCourseStatuses(courses []string, completed map[string]bool) []courseStatus {
	out := make([]courseStatus, len(courses))
	for idx, slug := range courses {
		status := pathStatusLocked
		if completed[slug] {
			status = pathStatusCompleted
		} else if idx == 0 || completed[courses[idx-1]] {
			status = pathStatusAvailable
		}

		out[idx] = courseStatus{Slug: slug, Status: status}
	}

	return out
}

// slugRE matches the allowed shape of a course/path slug.
var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// fetchPathDetail fetches a learning path's metadata from course-service.
func (s *State) fetchPathDetail(request *http.Request, slug string) (*pathDetail, error) {
	if !slugRE.MatchString(slug) {
		return nil, fmt.Errorf("invalid path slug: %q", slug)
	}

	rawURL := fmt.Sprintf("%s/api/paths/%s", s.Config.CourseServiceURL, slug)

	//nolint:gosec // slug is validated against slugRE above; CourseServiceURL is trusted server config, not user input
	req, err := http.NewRequestWithContext(request.Context(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build path detail request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // rawURL is built from a validated slug and trusted server config
	if err != nil {
		return nil, fmt.Errorf("fetch path detail: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("course-service returned %d for path %s: %s", resp.StatusCode, slug, body)
	}

	var detail pathDetail

	err = json.NewDecoder(resp.Body).Decode(&detail)
	if err != nil {
		return nil, fmt.Errorf("decode path detail: %w", err)
	}

	return &detail, nil
}

// MyPaths returns learning paths the authenticated user is enrolled in, with
// per-course completion status derived from quiz and lesson progress.
// Supports optional limit/offset pagination; without them, all paths are
// returned.
// @Summary   List learning paths the current user is enrolled in
// @Tags      Paths
// @Security  BearerAuth
// @Produce   json
// @Param     limit   query  int  false  "Max number of paths to return"
// @Param     offset  query  int  false  "Number of paths to skip"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/my/paths [get].
func (s *State) MyPaths(writer http.ResponseWriter, request *http.Request) {
	claims := s.claims(request)
	limit, offset := parsePagination(request)

	rows, err := s.Pool.Query(request.Context(),
		`SELECT path_slug, enrolledAt FROM path_enrollments
		 WHERE userId = $1::uuid ORDER BY enrolledAt DESC
		 LIMIT $2 OFFSET $3`,
		claims.Subject, limit, offset)
	if err != nil {
		slog.Error("failed to query path enrollments", "err", err)
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var enrollments []enrollment

	for rows.Next() {
		var enr enrollment

		err := rows.Scan(&enr.slug, &enr.enrolledAt)
		if err == nil {
			enrollments = append(enrollments, enr)
		}
	}

	result := make([]myPath, 0, len(enrollments))
	for _, enr := range enrollments {
		detail, err := s.fetchPathDetail(request, enr.slug)
		if err != nil {
			slog.Warn("failed to fetch path detail", "slug", enr.slug, "err", err)
			result = append(result, myPath{
				Slug:       enr.slug,
				Title:      enr.slug,
				EnrolledAt: enr.enrolledAt,
				Courses:    []courseStatus{},
			})

			continue
		}

		completed := s.completedCoursesCtx(request, claims.Subject, detail.Courses)
		courses := buildCourseStatuses(detail.Courses, completed)

		result = append(result, myPath{
			Slug:        detail.Slug,
			Title:       detail.Title,
			Description: detail.Description,
			EnrolledAt:  enr.enrolledAt,
			Courses:     courses,
		})
	}

	s.JSON(writer, http.StatusOK, map[string][]myPath{"paths": result})
}

// completedCoursesCtx returns the set of course slugs (from slugs) that
// userID has completed. Completion is not path-scoped: a course finished
// in any path counts. Two sources are checked — passed quiz modules
// (module_progress) and the __complete__ sentinel in lesson_progress.
func (s *State) completedCoursesCtx(request *http.Request, userID string, slugs []string) map[string]bool {
	if len(slugs) == 0 {
		return nil
	}

	rows, err := s.Pool.Query(request.Context(),
		`SELECT courseSlug FROM (
		     SELECT DISTINCT courseSlug FROM module_progress
		     WHERE userId = $1::uuid AND passed = true AND courseSlug = ANY($2)
		     UNION
		     SELECT DISTINCT courseSlug FROM lesson_progress
		     WHERE userId = $1::uuid AND lessonSlug = '__complete__' AND courseSlug = ANY($2)
		 ) sub`,
		userID, slugs)
	if err != nil {
		slog.Error("failed to query completed courses", "userID", userID, "err", err)

		return nil
	}
	defer rows.Close()

	result := make(map[string]bool)

	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			result[slug] = true
		}
	}

	return result
}

// AdminListPathEnrollments lists all users enrolled in a learning path,
// including each user's per-course completion status when the path
// detail is reachable.
// @Summary   List users enrolled in a path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Path slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/paths/{slug}/enrollments [get].
func (s *State) AdminListPathEnrollments(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	detail, err := s.fetchPathDetail(request, slug)
	if err != nil {
		slog.Warn("failed to fetch path detail for enrollments", "slug", slug, "err", err)
	}

	rows, err := s.Pool.Query(request.Context(), `
		SELECT u.id, u.email, u.role, pe.enrolledAt
		FROM path_enrollments pe
		JOIN users u ON u.id = pe.userId
		WHERE pe.path_slug = $1
		ORDER BY pe.enrolledAt DESC`, slug)
	if err != nil {
		slog.Error("failed to query path enrollments", "slug", slug, "err", err)
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var users []enrolledUser

	for rows.Next() {
		var member enrolledUser

		err := rows.Scan(&member.UserID, &member.Email, &member.Role, &member.EnrolledAt)
		if err != nil {
			continue
		}

		if detail != nil {
			completed := s.completedCoursesCtx(request, member.UserID, detail.Courses)
			member.TotalCourses = len(detail.Courses)

			member.Courses = buildCourseStatuses(detail.Courses, completed)
			for _, cs := range member.Courses {
				if cs.Status == pathStatusCompleted {
					member.CompletedCourses++
				}
			}
		}

		users = append(users, member)
	}

	if users == nil {
		users = []enrolledUser{}
	}

	s.JSON(writer, http.StatusOK, map[string][]enrolledUser{"users": users})
}

// AdminEnrollUserInPath enrolls a user in a learning path. Idempotent:
// enrolling an already-enrolled user is a no-op.
// @Summary   Enroll a user in a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string                   true  "Path slug"
// @Param     body  map[string]string  true  "userId"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments [post].
func (s *State) AdminEnrollUserInPath(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")

	var body struct {
		UserID string `json:"userId"`
	}

	err := decode(request, &body)
	if err != nil || body.UserID == "" {
		s.Error(writer, http.StatusBadRequest, "userId required")

		return
	}

	_, err = s.Pool.Exec(request.Context(),
		`INSERT INTO path_enrollments (userId, path_slug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		body.UserID, slug)
	if err != nil {
		slog.Error("failed to enroll user in path", "slug", slug, "userID", body.UserID, "err", err)
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Enrolled in path"})
}

// AdminUnenrollUserFromPath removes a user from a learning path.
// @Summary   Remove a user from a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug     path  string  true  "Path slug"
// @Param     userId  path  string  true  "User UUID"
// @Success   200      {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments/{userId} [delete].
func (s *State) AdminUnenrollUserFromPath(writer http.ResponseWriter, request *http.Request) {
	slug := param(request, "slug")
	userID := param(request, "userId")

	_, err := s.Pool.Exec(request.Context(),
		`DELETE FROM path_enrollments WHERE userId = $1::uuid AND path_slug = $2`,
		userID, slug)
	if err != nil {
		slog.Error("failed to unenroll user from path", "slug", slug, "userID", userID, "err", err)
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Unenrolled from path"})
}
