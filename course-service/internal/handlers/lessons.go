package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/elearning/course-service/internal/content"
	"github.com/elearning/course-service/internal/middleware"
)

// courseCompleteBody is the request body sent to the user-service when a
// course is completed.
type courseCompleteBody struct {
	UserID     string `json:"userId"`
	CourseSlug string `json:"courseSlug"`
}

// lessonSummary is the public API representation of a lesson within a
// lesson list.
type lessonSummary struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Order  int    `json:"order"`
	Viewed bool   `json:"viewed"`
}

// lessonDetail is the public API representation of a single lesson,
// including its full content.
type lessonDetail struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Viewed  bool   `json:"viewed"`
}

// autoEnroll silently enrolls the user in a public course via the user-service
// internal API. Errors are ignored — the course is accessible regardless.
func (s *State) autoEnroll(userID, courseSlug string) {
	body, err := json.Marshal(map[string]string{
		userIDJSONKey:     userID,
		courseSlugJSONKey: courseSlug,
	})
	if err != nil {
		zap.S().Warnw("failed to build auto-enroll request", "err", err)

		return
	}

	httpReq, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, s.Config.UserServiceURL+"/internal/enrollments/auto", bytes.NewReader(body),
	)
	if err != nil {
		zap.S().Warnw("failed to build auto-enroll request", "err", err)

		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	s.setInternalHeader(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
	}
}

// isEnrolled reports whether userID is enrolled in courseSlug, according to
// the user-service internal enrollment-check API.
func (s *State) isEnrolled(req *http.Request, courseSlug, userID string) bool {
	target, err := url.Parse(s.Config.UserServiceURL + "/internal/enrollments/check")
	if err != nil {
		return false
	}

	q := target.Query()
	q.Set(userIDJSONKey, userID)
	q.Set(courseSlugJSONKey, courseSlug)
	target.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return false
	}

	s.setInternalHeader(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Enrolled bool `json:"enrolled"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return false
	}

	return result.Enrolled
}

// ensureLessonAccess verifies that the requester (identified by claims) may
// access lessons of course: admins always pass, public courses trigger an
// auto-enroll, and private courses require an existing enrollment. On
// denial it writes deniedMsg as a 403 response and returns false.
func (s *State) ensureLessonAccess(
	writer http.ResponseWriter, req *http.Request, course *content.Course, courseSlug string,
	claims *middleware.Claims, deniedMsg string,
) bool {
	if claims.Role == roleAdmin {
		return true
	}

	if course.IsPublic {
		s.autoEnroll(claims.Subject, courseSlug)

		return true
	}

	if !s.isEnrolled(req, courseSlug, claims.Subject) {
		s.Error(writer, http.StatusForbidden, deniedMsg)

		return false
	}

	return true
}

// ListLessons godoc
// @Summary   List lessons for a course
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons [get].
func (s *State) ListLessons(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureLessonAccess(writer, req, course, courseSlug, claims, "Enroll in this course to access lessons") { //nolint:contextcheck // autoEnroll and friends do not accept context (pre-existing architecture)
		return
	}

	viewed := s.viewedLessons(req, courseSlug, claims.Subject)

	modules := s.visibleModules(course, req)

	if len(course.Lessons) > 0 {
		out := make([]lessonSummary, 0, len(course.Lessons))
		for _, lesson := range course.Lessons {
			out = append(out, lessonSummary{
				Slug:   lesson.Slug,
				Title:  lesson.Title,
				Order:  lesson.Order,
				Viewed: viewed[lesson.Slug],
			})
		}

		s.JSON(writer, http.StatusOK, map[string]any{lessonsJSONKey: out})

		return
	}

	out := make([]lessonSummary, 0, len(modules))
	for pos, mod := range modules {
		out = append(out, lessonSummary{
			Slug:   mod.Slug(),
			Title:  mod.Name,
			Order:  pos + 1,
			Viewed: viewed[mod.Slug()],
		})
	}

	s.JSON(writer, http.StatusOK, map[string]any{lessonsJSONKey: out})
}

// findStoredLesson looks up a course-defined (non-module) lesson by slug.
func findStoredLesson(course *content.Course, lessonSlug string, viewed map[string]bool) (lessonDetail, bool) {
	for idx := range course.Lessons {
		if course.Lessons[idx].Slug == lessonSlug {
			return lessonDetail{
				Slug:    course.Lessons[idx].Slug,
				Title:   course.Lessons[idx].Title,
				Order:   course.Lessons[idx].Order,
				Content: course.Lessons[idx].Content,
				Viewed:  viewed[course.Lessons[idx].Slug],
			}, true
		}
	}

	return lessonDetail{}, false
}

// moduleLessonBody resolves the display body for a module-backed lesson,
// fetching remote git content when configured.
func (s *State) moduleLessonBody(ctx context.Context, mod content.Module) string {
	body := mod.Content()
	if mod.Type != moduleTypeText {
		body = content.ReplicatedPath(ctx, mod, s.Config.UploadsDir)
	}

	if mod.HasGitContent() {
		data, err := s.GitCache.FetchModuleContent(ctx, mod.Src, mod.Ref, mod.Path, s.tokenForRepo(mod.Src))
		if err != nil {
			body = "⚠️ This content is not yet available. Please check back later.\n\n_Failed to load from remote repository._"
		} else {
			body = string(data)
		}
	}

	return body
}

// findModuleLesson looks up a module-backed lesson by slug among modules,
// returning its zero-based position when found.
func findModuleLesson(modules []content.Module, lessonSlug string) (content.Module, int, bool) {
	for pos, mod := range modules {
		if mod.Slug() == lessonSlug {
			return mod, pos, true
		}
	}

	return content.Module{}, 0, false
}

// GetLesson godoc
// @Summary   Get a lesson by slug
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lessonSlug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lessonSlug} [get].
func (s *State) GetLesson(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	lessonSlug := param(req, "lessonSlug")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureLessonAccess(writer, req, course, courseSlug, claims, "Enroll in this course to access lessons") { //nolint:contextcheck // autoEnroll and friends do not accept context (pre-existing architecture)
		return
	}

	viewed := s.viewedLessons(req, courseSlug, claims.Subject)
	modules := s.visibleModules(course, req)

	if detail, found := findStoredLesson(course, lessonSlug, viewed); found {
		s.JSON(writer, http.StatusOK, map[string]any{lessonJSONKey: detail})

		return
	}

	mod, pos, found := findModuleLesson(modules, lessonSlug)
	if !found {
		s.Error(writer, http.StatusNotFound, "Lesson not found")

		return
	}

	if mod.Type == moduleTypeQuiz {
		s.Error(writer, http.StatusNotFound, "Quiz modules use a separate endpoint")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{lessonJSONKey: lessonDetail{
		Slug:    mod.Slug(),
		Title:   mod.Name,
		Order:   pos + 1,
		Type:    mod.Type,
		Content: s.moduleLessonBody(req.Context(), mod),
		Viewed:  viewed[mod.Slug()],
	}})
}

// lessonModuleIndex reports whether lessonSlug identifies a course-defined
// lesson (moduleIndex -1) or a module (its index), and whether it was
// found at all.
func lessonModuleIndex(course *content.Course, lessonSlug string) (int, bool) {
	for _, lesson := range course.Lessons {
		if lesson.Slug == lessonSlug {
			return -1, true
		}
	}

	for idx, mod := range course.Modules {
		if mod.Slug() == lessonSlug {
			return idx, true
		}
	}

	return -1, false
}

// isLastMeaningfulModule reports whether moduleIndex is the last module of
// course, ignoring trailing inline quiz modules.
func isLastMeaningfulModule(course *content.Course, moduleIndex int) bool {
	lastMeaningful := len(course.Modules) - 1
	for lastMeaningful > 0 && course.Modules[lastMeaningful].Inline && course.Modules[lastMeaningful].Type == moduleTypeQuiz {
		lastMeaningful--
	}

	return moduleIndex >= 0 && moduleIndex == lastMeaningful
}

// postLessonComplete notifies the user-service that lessonSlug was
// completed by userID, writing an HTTP error and returning false on any
// failure.
func (s *State) postLessonComplete(
	writer http.ResponseWriter, req *http.Request, userID, courseSlug, lessonSlug string,
) bool {
	body := map[string]string{
		userIDJSONKey:     userID,
		courseSlugJSONKey: courseSlug,
		"lessonSlug":      lessonSlug,
	}

	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(body)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to mark lesson as complete")

		return false
	}

	httpReq, err := http.NewRequestWithContext(
		req.Context(), http.MethodPost, s.Config.UserServiceURL+"/internal/progress/complete", &buf,
	)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to mark lesson as complete")

		return false
	}

	httpReq.Header.Set("Content-Type", "application/json")
	s.setInternalHeader(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Failed to mark lesson as complete")

		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		s.Error(writer, http.StatusInternalServerError, "Failed to mark lesson as complete")

		return false
	}

	return true
}

// notifyCourseComplete notifies the user-service that userID completed
// courseSlug. Failures are only logged: the lesson itself was already
// recorded as complete, so they must not affect the HTTP response.
func (s *State) notifyCourseComplete(req *http.Request, userID, courseSlug string) {
	payload := courseCompleteBody{UserID: userID, CourseSlug: courseSlug}

	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(payload)
	if err != nil {
		zap.S().Errorw("failed to build course-complete request", "userID", userID, "courseSlug", courseSlug, "err", err)

		return
	}

	httpReq, err := http.NewRequestWithContext(
		req.Context(), http.MethodPost, s.Config.UserServiceURL+"/internal/progress/course-complete", &buf,
	)
	if err != nil {
		zap.S().Errorw("failed to build course-complete request", "userID", userID, "courseSlug", courseSlug, "err", err)

		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	s.setInternalHeader(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		zap.S().Errorw("failed to mark course complete", "userID", userID, "courseSlug", courseSlug, "err", err)

		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		zap.S().Errorw("user-service rejected course-complete request",
			"userID", userID, "courseSlug", courseSlug, "status", resp.StatusCode)
	}
}

// MarkLessonComplete godoc
// @Summary   Mark a lesson as complete
// @Tags      Lessons
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lessonSlug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]string
// @Failure   403   {object}  map[string]string
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lessonSlug}/complete [post].
func (s *State) MarkLessonComplete(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	lessonSlug := param(req, "lessonSlug")
	claims := s.claims(req)

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureLessonAccess(writer, req, course, courseSlug, claims, "Not enrolled") { //nolint:contextcheck // autoEnroll and friends do not accept context (pre-existing architecture)
		return
	}

	moduleIndex, found := lessonModuleIndex(course, lessonSlug)
	if !found {
		s.Error(writer, http.StatusNotFound, "Lesson not found")

		return
	}

	if !s.postLessonComplete(writer, req, claims.Subject, courseSlug, lessonSlug) {
		return
	}

	if isLastMeaningfulModule(course, moduleIndex) {
		s.notifyCourseComplete(req, claims.Subject, courseSlug)
	}

	s.JSON(writer, http.StatusOK, map[string]string{messageJSONKey: lessonCompleteMessage})
}
