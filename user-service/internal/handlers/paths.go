package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

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
	UserID           string         `json:"user_id"`
	Email            string         `json:"email"`
	Role             string         `json:"role"`
	EnrolledAt       time.Time      `json:"enrolled_at"`
	CompletedCourses int            `json:"completed_courses"`
	TotalCourses     int            `json:"total_courses"`
	Courses          []courseStatus `json:"courses,omitempty"`
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (s *State) fetchPathDetail(r *http.Request, slug string) (*pathDetail, error) {
	if !slugRE.MatchString(slug) {
		return nil, fmt.Errorf("invalid path slug: %q", slug)
	}
	url := fmt.Sprintf("%s/api/paths/%s", s.Config.CourseServiceURL, slug)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
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

// MyPaths godoc
// @Summary   List learning paths the current user is enrolled in
// @Tags      Paths
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/my/paths [get]
func (s *State) MyPaths(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT path_slug, enrolled_at FROM path_enrollments
		 WHERE user_id = $1::uuid ORDER BY enrolled_at DESC`,
		claims.Subject)
	if err != nil {
		slog.Error("failed to query path enrollments", "err", err)
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var enrollments []enrollment
	for rows.Next() {
		var e enrollment
		if err := rows.Scan(&e.slug, &e.enrolledAt); err == nil {
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

		courses := make([]courseStatus, len(detail.Courses))
		for i, slug := range detail.Courses {
			status := "locked"
			if completed[slug] {
				status = "completed"
			} else if i == 0 || completed[detail.Courses[i-1]] {
				status = "available"
			}
			courses[i] = courseStatus{Slug: slug, Status: status}
		}

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

// AdminListPathEnrollments godoc
// @Summary   List users enrolled in a path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Path slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/admin/paths/{slug}/enrollments [get]
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
		if err := rows.Scan(&u.UserID, &u.Email, &u.Role, &u.EnrolledAt); err != nil {
			continue
		}
		if detail != nil {
			completed := s.completedCoursesCtx(r, u.UserID, detail.Courses)
			u.TotalCourses = len(detail.Courses)
			courses := make([]courseStatus, len(detail.Courses))
			for i, cs := range detail.Courses {
				status := "locked"
				if completed[cs] {
					status = "completed"
					u.CompletedCourses++
				} else if i == 0 || completed[detail.Courses[i-1]] {
					status = "available"
				}
				courses[i] = courseStatus{Slug: cs, Status: status}
			}
			u.Courses = courses
		}
		users = append(users, u)
	}
	if users == nil {
		users = []enrolledUser{}
	}
	s.JSON(w, http.StatusOK, map[string][]enrolledUser{"users": users})
}

// AdminEnrollUserInPath godoc
// @Summary   Enroll a user in a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string                   true  "Path slug"
// @Param     body  body  map[string]string  true  "user_id"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments [post]
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

// AdminUnenrollUserFromPath godoc
// @Summary   Remove a user from a learning path (admin)
// @Tags      Admin - Paths
// @Security  BearerAuth
// @Produce   json
// @Param     slug     path  string  true  "Path slug"
// @Param     user_id  path  string  true  "User UUID"
// @Success   200      {object}  map[string]string
// @Router    /api/admin/paths/{slug}/enrollments/{user_id} [delete]
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
