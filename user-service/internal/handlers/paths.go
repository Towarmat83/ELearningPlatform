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

// parsePagination reads optional limit/offset query params. A zero, negative,
// or omitted limit maps to a nil *int, passed through to SQL as LIMIT NULL
// (unbounded) — callers can pass e.g. limit=-1 to explicitly request everything.
func parsePagination(r *http.Request) (limit *int, offset int) {
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = &v
	}

	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}

	return
}

type pathDetail struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Courses     []string `json:"courses"`
}

type courseStatus struct {
	Slug   string `json:"slug"`
	Status string `json:"status"` // "completed", "available", "locked"
}

type myPath struct {
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	EnrolledAt  time.Time      `json:"enrolled_at"`
	Courses     []courseStatus `json:"courses"`
}

type enrollment struct {
	slug       string
	enrolledAt time.Time
}

// enrolledUser represents a user enrolled in a learning path,
// along with their progress across the path's courses.
type enrolledUser struct {
	// UserID is the UUID of the enrolled user.
	UserID string `json:"user_id"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Role is the user's platform role (e.g. "student", "admin").
	Role string `json:"role"`
	// EnrolledAt is the timestamp when the user was enrolled in this path.
	EnrolledAt time.Time `json:"enrolled_at"`
	// CompletedCourses is the count of courses this user has completed in the path.
	CompletedCourses int `json:"completed_courses"`
	// TotalCourses is the total number of courses in the path.
	TotalCourses int `json:"total_courses"`
	// Courses holds per-course completion status, populated when path detail is available.
	Courses []courseStatus `json:"courses,omitempty"`
}

// buildCourseStatuses derives the completion status of each course in an ordered
// path, given the set of courses the user has already completed.
// Status is "completed" if done, "available" if the previous course is done (or it
// is the first course), and "locked" otherwise.
func buildCourseStatuses(courses []string, completed map[string]bool) []courseStatus {
	out := make([]courseStatus, len(courses))
	for i, slug := range courses {
		status := "locked"
		if completed[slug] {
			status = "completed"
		} else if i == 0 || completed[courses[i-1]] {
			status = "available"
		}

		out[i] = courseStatus{Slug: slug, Status: status}
	}

	return out
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (s *State) fetchPathDetail(r *http.Request, slug string) (*pathDetail, error) {
	if !slugRE.MatchString(slug) {
		return nil, fmt.Errorf("invalid path slug: %q", slug)
	}

	url := fmt.Sprintf("%s/api/paths/%s", s.Config.CourseServiceURL, slug)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("course-service returned %d for path %s: %s", resp.StatusCode, slug, body)
	}

	var p pathDetail
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}

	return &p, nil
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
func (s *State) MyPaths(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	limit, offset := parsePagination(r)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT path_slug, enrolled_at FROM path_enrollments
		 WHERE user_id = $1::uuid ORDER BY enrolled_at DESC
		 LIMIT $2 OFFSET $3`,
		claims.Subject, limit, offset)
	if err != nil {
		slog.Error("failed to query path enrollments", "err", err)
		s.Error(w, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var enrollments []enrollment

	for rows.Next() {
		var e enrollment

		err := rows.Scan(&e.slug, &e.enrolledAt)
		if err == nil {
			enrollments = append(enrollments, e)
		}
	}

	result := make([]myPath, 0, len(enrollments))
	for _, e := range enrollments {
		detail, err := s.fetchPathDetail(r, e.slug)
		if err != nil {
			slog.Warn("failed to fetch path detail", "slug", e.slug, "err", err)
			result = append(result, myPath{
				Slug:       e.slug,
				Title:      e.slug,
				EnrolledAt: e.enrolledAt,
				Courses:    []courseStatus{},
			})

			continue
		}

		completed := s.completedCoursesCtx(r, claims.Subject, detail.Courses)
		courses := buildCourseStatuses(detail.Courses, completed)

		result = append(result, myPath{
			Slug:        detail.Slug,
			Title:       detail.Title,
			Description: detail.Description,
			EnrolledAt:  e.enrolledAt,
			Courses:     courses,
		})
	}

	s.JSON(w, http.StatusOK, map[string][]myPath{"paths": result})
}

// completedCoursesCtx returns the set of course slugs (from the provided list)
// that userID has completed. Completion is not path-scoped: a course finished in
// any path counts. Two sources are checked — passed quiz modules (module_progress)
// and the __complete__ sentinel in lesson_progress.
func (s *State) completedCoursesCtx(r *http.Request, userID string, slugs []string) map[string]bool {
	if len(slugs) == 0 {
		return nil
	}

	rows, err := s.Pool.Query(r.Context(),
		`SELECT course_slug FROM (
		     SELECT DISTINCT course_slug FROM module_progress
		     WHERE user_id = $1::uuid AND passed = true AND course_slug = ANY($2)
		     UNION
		     SELECT DISTINCT course_slug FROM lesson_progress
		     WHERE user_id = $1::uuid AND lesson_slug = '__complete__' AND course_slug = ANY($2)
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

// AdminListPathEnrollments lists all users enrolled in a learning path, including
// each user's per-course completion status when the path detail is reachable.
// @Summary   List users enrolled in a path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Path slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/paths/{slug}/enrollments [get].
func (s *State) AdminListPathEnrollments(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	detail, err := s.fetchPathDetail(r, slug)
	if err != nil {
		slog.Warn("failed to fetch path detail for enrollments", "slug", slug, "err", err)
	}

	rows, err := s.Pool.Query(r.Context(), `
		SELECT u.id, u.email, u.role, pe.enrolled_at
		FROM path_enrollments pe
		JOIN users u ON u.id = pe.user_id
		WHERE pe.path_slug = $1
		ORDER BY pe.enrolled_at DESC`, slug)
	if err != nil {
		slog.Error("failed to query path enrollments", "slug", slug, "err", err)
		s.Error(w, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var users []enrolledUser

	for rows.Next() {
		var u enrolledUser

		err := rows.Scan(&u.UserID, &u.Email, &u.Role, &u.EnrolledAt)
		if err != nil {
			continue
		}

		if detail != nil {
			completed := s.completedCoursesCtx(r, u.UserID, detail.Courses)
			u.TotalCourses = len(detail.Courses)

			u.Courses = buildCourseStatuses(detail.Courses, completed)
			for _, cs := range u.Courses {
				if cs.Status == "completed" {
					u.CompletedCourses++
				}
			}
		}

		users = append(users, u)
	}

	if users == nil {
		users = []enrolledUser{}
	}

	s.JSON(w, http.StatusOK, map[string][]enrolledUser{"users": users})
}

// AdminEnrollUserInPath enrolls a user in a learning path. Idempotent: enrolling
// an already-enrolled user is a no-op.
// @Summary   Enroll a user in a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string                   true  "Path slug"
// @Param     body  map[string]string  true  "user_id"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments [post].
func (s *State) AdminEnrollUserInPath(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := decode(r, &body); err != nil || body.UserID == "" {
		s.Error(w, http.StatusBadRequest, "user_id required")

		return
	}

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO path_enrollments (user_id, path_slug) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`,
		body.UserID, slug)
	if err != nil {
		slog.Error("failed to enroll user in path", "slug", slug, "userID", body.UserID, "err", err)
		s.Error(w, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"message": "Enrolled in path"})
}

// AdminUnenrollUserFromPath removes a user from a learning path.
// @Summary   Remove a user from a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug     path  string  true  "Path slug"
// @Param     user_id  path  string  true  "User UUID"
// @Success   200      {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments/{user_id} [delete].
func (s *State) AdminUnenrollUserFromPath(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	userID := param(r, "user_id")

	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM path_enrollments WHERE user_id = $1::uuid AND path_slug = $2`,
		userID, slug)
	if err != nil {
		slog.Error("failed to unenroll user from path", "slug", slug, "userID", userID, "err", err)
		s.Error(w, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(w, http.StatusOK, map[string]string{"message": "Unenrolled from path"})
}
