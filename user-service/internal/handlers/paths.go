package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// Course-status literals shared by buildCourseStatuses and its callers,
// pulled out to satisfy goconst.
const (
	pathStatusLocked    = "locked"
	pathStatusCompleted = "completed"
	pathStatusAvailable = "available"
	// pathKindSkill identifies a skill-kind learning path.
	pathKindSkill = "skill"
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
	Kind        string   `json:"kind,omitempty"`
	Courses     []string `json:"courses,omitempty"`
	Skills      []string `json:"skills,omitempty"`
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
	Kind        string         `json:"kind,omitempty"`
	EnrolledAt  time.Time      `json:"enrolledAt"`
	Courses     []courseStatus `json:"courses,omitempty"`
	Skills      []string       `json:"skills,omitempty"`
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
// ordered path, given the set of courses the user has completed.
//
// Rules:
//   - "completed" if the user finished the course (cross-path counts).
//   - "available" if this is the first course or the immediately preceding
//     course is "completed".
//   - "locked" otherwise.
func buildCourseStatuses(courses []string, completed map[string]struct{}) []courseStatus {
	out := make([]courseStatus, len(courses))

	for idx, slug := range courses {
		_, done := completed[slug]

		switch {
		case done:
			out[idx] = courseStatus{Slug: slug, Status: pathStatusCompleted}
		case idx == 0 || out[idx-1].Status == pathStatusCompleted:
			out[idx] = courseStatus{Slug: slug, Status: pathStatusAvailable}
		default:
			out[idx] = courseStatus{Slug: slug, Status: pathStatusLocked}
		}
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

	var detail pathDetail

	err := s.fetchCourseServiceJSON(request, "/api/paths/"+slug, &detail)
	if err != nil {
		return nil, fmt.Errorf("fetch path detail: %w", err)
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

	rows, err := s.Repos.Paths.MyEnrollments(request.Context(), claims.Subject, limit, offset)
	if err != nil {
		zap.L().Error("failed to query path enrollments", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	result := make([]myPath, 0, len(rows))
	for _, row := range rows {
		result = append(result, s.resolveEnrollment(request, claims.Subject, row.Slug, row.EnrolledAt))
	}

	s.JSON(writer, http.StatusOK, map[string][]myPath{"paths": result})
}

// resolveEnrollment fetches path detail for a single enrollment and returns the
// myPath struct with per-course (or per-skill) completion statuses filled in.
func (s *State) resolveEnrollment(req *http.Request, userID, pathSlug string, enrolledAt time.Time) myPath {
	detail, err := s.fetchPathDetail(req, pathSlug)
	if err != nil {
		zap.L().Warn("failed to fetch path detail", zap.String("slug", pathSlug), zap.Error(err))

		return myPath{
			Slug:       pathSlug,
			Title:      pathSlug,
			EnrolledAt: enrolledAt,
			Courses:    []courseStatus{},
		}
	}

	var courses []courseStatus

	if detail.Kind == pathKindSkill {
		courses = s.buildSkillStatuses(req, userID, detail.Skills)
	} else {
		completed := s.completedCoursesCtx(req, userID, detail.Courses)
		courses = buildCourseStatuses(detail.Courses, completed)
	}

	return myPath{
		Slug:        detail.Slug,
		Title:       detail.Title,
		Description: detail.Description,
		Kind:        detail.Kind,
		EnrolledAt:  enrolledAt,
		Courses:     courses,
		Skills:      detail.Skills,
	}
}

// buildSkillStatuses computes the ordered completion status for each skill in a
// skill-kind learning path. A skill is "completed" when all its assessable
// modules (quiz/lab) are passed; otherwise the first incomplete skill is
// "available" and subsequent ones are "locked" (sequential ordering).
func (s *State) buildSkillStatuses(req *http.Request, userID string, skills []string) []courseStatus {
	passed := s.passedModulesCtx(req, userID)
	viewed := s.viewedLessonsCtx(req, userID)
	out := make([]courseStatus, 0, len(skills))
	prevCompleted := true // first skill has no prerequisite

	for _, skill := range skills {
		modules, err := s.fetchSkillModules(req, skill)
		if err != nil {
			zap.L().Warn("failed to fetch skill modules for path", zap.String("skill", skill), zap.Error(err))
			out = append(out, courseStatus{Slug: skill, Status: pathStatusLocked})
			prevCompleted = false

			continue
		}

		done := skillIsCompleted(modules, passed, viewed)

		var status string

		switch {
		case done:
			status = pathStatusCompleted
		case prevCompleted:
			status = pathStatusAvailable
		default:
			status = pathStatusLocked
		}

		out = append(out, courseStatus{Slug: skill, Status: status})
		prevCompleted = done
	}

	return out
}

// completedCoursesCtx returns the set of course slugs (from slugs) that
// userID has completed. Completion is not path-scoped: a course finished
// in any path counts. Two sources are checked — passed quiz modules
// (module_progress) and the __complete__ sentinel in lesson_progress.
func (s *State) completedCoursesCtx(request *http.Request, userID string, slugs []string) map[string]struct{} {
	if len(slugs) == 0 {
		return nil
	}

	result := make(map[string]struct{})

	moduleCompleted, err := s.Repos.ModuleProgress.CompletedCourseSlugs(request.Context(), userID, slugs)
	if err != nil {
		zap.L().Error("failed to query completed courses via module progress", zap.String("userID", userID), zap.Error(err))

		return nil
	}

	for _, slug := range moduleCompleted {
		result[slug] = struct{}{}
	}

	lessonCompleted, err := s.Repos.LessonProgress.CompletedCourseSlugs(request.Context(), userID, slugs)
	if err != nil {
		zap.L().Error("failed to query completed courses via lesson progress", zap.String("userID", userID), zap.Error(err))

		return nil
	}

	for _, slug := range lessonCompleted {
		result[slug] = struct{}{}
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
		zap.L().Warn("failed to fetch path detail for enrollments", zap.String("slug", slug), zap.Error(err))
	}

	rows, err := s.Repos.Paths.ListBySlug(request.Context(), slug)
	if err != nil {
		zap.L().Error("failed to query path enrollments", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	users := make([]enrolledUser, 0, len(rows))

	for _, row := range rows {
		member := enrolledUser{UserID: row.UserID, Email: row.Email, Role: row.Role, EnrolledAt: row.EnrolledAt}

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

	s.JSON(writer, http.StatusOK, map[string][]enrolledUser{adminJSONKeyUsers: users})
}

// ManagerListPathEnrollments lists users enrolled in a learning path, filtered
// to those within the manager's group scope.
// @Summary   List users enrolled in a path (manager, scope-restricted)
// @Tags      Manager
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Path slug"
// @Success   200   {object}  map[string]interface{}
// @Router    /api/manager/paths/{slug}/enrollments [get].
func (s *State) ManagerListPathEnrollments(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	slug := param(request, "slug")
	claims := s.claims(request)

	groups, err := s.Repos.Groups.GetGroupsByUserID(ctx, claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	groupIDs := managerScopeIDs(groups)

	detail, err := s.fetchPathDetail(request, slug)
	if err != nil {
		zap.L().Warn("failed to fetch path detail for manager enrollment list", zap.String("slug", slug), zap.Error(err))
	}

	rows, err := s.Repos.Paths.ListBySlug(ctx, slug)
	if err != nil {
		zap.L().Error("failed to query path enrollments", zap.String("slug", slug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	users := make([]enrolledUser, 0, len(rows))

	for _, row := range rows {
		inScope, scopeErr := s.Repos.Groups.UserInAnyGroup(ctx, row.UserID, groupIDs)
		if scopeErr != nil || !inScope {
			continue
		}

		member := enrolledUser{UserID: row.UserID, Email: row.Email, Role: row.Role, EnrolledAt: row.EnrolledAt}

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

	s.JSON(writer, http.StatusOK, map[string][]enrolledUser{adminJSONKeyUsers: users})
}

// enrollPathCourses auto-enrolls userID in each course belonging to pathSlug.
// Best-effort: failures are only logged so the caller's HTTP response is
// not affected.
func (s *State) enrollPathCourses(req *http.Request, userID, pathSlug string) {
	detail, err := s.fetchPathDetail(req, pathSlug)
	if err != nil {
		zap.L().Warn("could not fetch path detail for course auto-enroll",
			zap.String("pathSlug", pathSlug), zap.Error(err))

		return
	}

	for _, courseSlug := range detail.Courses {
		cerr := s.Repos.Enrollments.Create(req.Context(), userID, courseSlug)
		if cerr != nil {
			zap.L().Warn("failed to auto-enroll user in path course",
				zap.String("userID", userID), zap.String("courseSlug", courseSlug), zap.Error(cerr))
		}
	}
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

	err = s.Repos.Paths.Enroll(request.Context(), body.UserID, slug)
	if err != nil {
		zap.L().Error("failed to enroll user in path", zap.String("slug", slug), zap.String("userID", body.UserID), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.enrollPathCourses(request, body.UserID, slug)

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

	err := s.Repos.Paths.Unenroll(request.Context(), userID, slug)
	if err != nil {
		zap.L().Error("failed to unenroll user from path", zap.String("slug", slug), zap.String("userID", userID), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Unenrolled from path"})
}
