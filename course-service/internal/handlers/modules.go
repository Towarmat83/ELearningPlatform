package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/elearning/course-service/internal/content"
	"github.com/elearning/course-service/internal/middleware"
)

// publicAnswer is the client-facing representation of a quiz answer option;
// whether it is correct is only populated for admin requesters.
type publicAnswer struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct bool   `json:"correct,omitempty"`
}

// publicQuestion is the client-facing representation of a quiz question,
// stripped of internal scoring details unless the requester is an admin.
type publicQuestion struct {
	ID             string                  `json:"id"`
	Type           string                  `json:"type"`
	Difficulty     string                  `json:"difficulty,omitempty"`
	Points         int                     `json:"points"`
	Question       string                  `json:"question"`
	Answers        []publicAnswer          `json:"answers,omitempty"`
	Items          []content.OrderItem     `json:"items,omitempty"`
	PartialScoring *content.PartialScoring `json:"partialScoring,omitempty"`
	SourceRefs     []content.SourceRef     `json:"sourceRefs,omitempty"`
}

// sanitizeQuestions converts internal quiz questions into their public,
// client-safe representation, revealing correct answers only when admin
// is true.
func sanitizeQuestions(qq []content.Question, admin bool) []any {
	out := make([]any, len(qq))
	for index, question := range qq {
		pub := publicQuestion{
			ID:             question.ID,
			Type:           question.Type,
			Difficulty:     question.Difficulty,
			Points:         question.Points,
			Question:       question.Question,
			Items:          question.Items,
			PartialScoring: question.PartialScoring,
			SourceRefs:     question.Feedback.SourceRefs,
		}
		for _, a := range question.Answers {
			pa := publicAnswer{ID: a.ID, Text: a.Text}
			if admin {
				pa.Correct = a.Correct
			}

			pub.Answers = append(pub.Answers, pa)
		}

		out[index] = pub
	}

	return out
}

// inlineQuizResponse is the client-facing representation of a quiz module
// embedded inline immediately after the lesson module that precedes it.
type inlineQuizResponse struct {
	Index         int         `json:"index"`
	Name          string      `json:"name"`
	Slug          string      `json:"slug"`
	Questions     []any       `json:"questions,omitempty"`
	QuestionCount int         `json:"questionCount"`
	PassingScore  int         `json:"passingScore"`
	BestScore     int         `json:"bestScore,omitempty"`
	MaxScore      int         `json:"maxScore,omitempty"`
	Passed        bool        `json:"passed"`
	Attempts      int         `json:"attempts,omitempty"`
	QuizConfig    *quizConfig `json:"quizConfig,omitempty"`
}

// moduleResponse is the client-facing representation of a single course
// module: its content, the caller's progress, and (for admins only) the
// underlying source location.
type moduleResponse struct {
	Index         int                      `json:"index"`
	Name          string                   `json:"name"`
	Slug          string                   `json:"slug"`
	Type          string                   `json:"type"`
	Content       string                   `json:"content,omitempty"`
	Viewed        bool                     `json:"viewed"`
	Hidden        bool                     `json:"hidden"`
	Locked        bool                     `json:"locked"`
	Prerequisites []string                 `json:"prerequisites,omitempty"`
	BestScore     int                      `json:"bestScore,omitempty"`
	MaxScore      int                      `json:"maxScore,omitempty"`
	Passed        bool                     `json:"passed,omitempty"`
	Attempts      int                      `json:"attempts,omitempty"`
	QuestionCount int                      `json:"questionCount,omitempty"`
	Questions     []any                    `json:"questions,omitempty"`
	QuizConfig    *quizConfig              `json:"quizConfig,omitempty"`
	Cooldowns     map[string]cooldownState `json:"cooldowns,omitempty"`
	Inline        bool                     `json:"inline,omitempty"`
	InlineQuiz    *inlineQuizResponse      `json:"inlineQuiz,omitempty"`
	HasCheck      bool                     `json:"hasCheck,omitempty"`
	LabURL        string                   `json:"labUrl,omitempty"`
	CheckProvider string                   `json:"checkProvider,omitempty"`
	CheckType     string                   `json:"checkType,omitempty"`
	CheckParams   map[string]any           `json:"checkParams,omitempty"`
	Steps         []content.CheckStep      `json:"steps,omitempty"`
	// Admin-only fields (omitted for regular users)
	Src  string `json:"src,omitempty"`
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`
}

// quizConfig describes a quiz's grading rules as exposed to the client.
type quizConfig struct {
	PassingScore           int      `json:"passingScore"`
	Cooldown               cooldown `json:"cooldown,omitempty"`
	MaxAttemptsPerQuestion *int     `json:"maxAttemptsPerQuestion,omitempty"`
	LockOnMaxAttempts      bool     `json:"lockOnMaxAttempts"`
}

// cooldown describes the backoff strategy applied between graded quiz
// attempts, as exposed to the client.
type cooldown struct {
	Strategy    string  `json:"strategy"`
	BaseSeconds int     `json:"baseSeconds"`
	Multiplier  float64 `json:"multiplier"`
	MaxSeconds  int     `json:"maxSeconds"`
}

// submitResponse is returned to the client after a quiz submission has been
// scored.
type submitResponse struct {
	TotalScore      int                      `json:"totalScore"`
	MaxScore        int                      `json:"maxScore"`
	Passed          bool                     `json:"passed"`
	QuestionResults []questionResultAPI      `json:"questionResults"`
	Cooldowns       map[string]cooldownState `json:"cooldowns,omitempty"`
}

// cooldownState reports the current cooldown/attempt state for one quiz
// question, as exposed to the client.
type cooldownState struct {
	RemainingSeconds int  `json:"remainingSeconds"`
	Attempts         int  `json:"attempts"`
	Locked           bool `json:"locked"`
}

// questionResultAPI is the client-facing result of scoring a single quiz
// question.
type questionResultAPI struct {
	QuestionID    string              `json:"questionId"`
	Type          string              `json:"type"`
	IsCorrect     bool                `json:"isCorrect"`
	PointsEarned  int                 `json:"pointsEarned"`
	PointsMax     int                 `json:"pointsMax"`
	CorrectAnswer any                 `json:"correctAnswer,omitempty"`
	Feedback      string              `json:"feedback,omitempty"`
	SourceRefs    []content.SourceRef `json:"sourceRefs,omitempty"`
}

// viewedLessons fetches the set of lesson slugs the user has marked as
// viewed for courseSlug, keyed by slug for O(1) lookup.
func (s *State) viewedLessons(httpReq *http.Request, courseSlug, userID string) map[string]bool {
	reqURL, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/viewed")
	if err != nil {
		return nil
	}

	q := reqURL.Query()
	q.Set("userId", userID)
	q.Set("courseSlug", courseSlug)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(httpReq.Context(), http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Viewed []string `json:"viewed"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil
	}

	viewedSet := make(map[string]bool, len(result.Viewed))
	for _, slug := range result.Viewed {
		viewedSet[slug] = true
	}

	return viewedSet
}

// moduleProgressData holds per-module quiz progress fetched from user-service.
type moduleProgressData struct {
	BestScore int
	MaxScore  int
	Passed    bool
	Attempts  int
}

// fetchModuleProgress fetches quiz progress for all modules in a course.
func (s *State) fetchModuleProgress(httpReq *http.Request, courseSlug, userID string) map[int]moduleProgressData {
	reqURL, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/modules")
	if err != nil {
		return nil
	}

	q := reqURL.Query()
	q.Set("userId", userID)
	q.Set("courseSlug", courseSlug)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(httpReq.Context(), http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Progress []struct {
			ModuleIndex int  `json:"moduleIndex"`
			BestScore   int  `json:"bestScore"`
			MaxScore    int  `json:"maxScore"`
			Passed      bool `json:"passed"`
			Attempts    int  `json:"attempts"`
		} `json:"progress"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil
	}

	progressByIndex := make(map[int]moduleProgressData, len(result.Progress))
	for _, p := range result.Progress {
		progressByIndex[p.ModuleIndex] = moduleProgressData{
			BestScore: p.BestScore,
			MaxScore:  p.MaxScore,
			Passed:    p.Passed,
			Attempts:  p.Attempts,
		}
	}

	return progressByIndex
}

// moduleProgressRequest is the payload sent to user-service to record a quiz
// module's score. Field names mirror user-service's internal API contract
// (user-service/internal/handlers/internal.go).
type moduleProgressRequest struct {
	UserID      string `json:"userId"`
	CourseSlug  string `json:"courseSlug"`
	ModuleIndex int    `json:"moduleIndex"`
	ModuleSlug  string `json:"moduleSlug"`
	Score       int    `json:"score"`
	MaxScore    int    `json:"maxScore"`
	Passed      bool   `json:"passed"`
}

// recordModuleProgressTimeout bounds the async POST to user-service so a
// slow or unreachable dependency cannot leak goroutines indefinitely.
const recordModuleProgressTimeout = 10 * time.Second

// recordModuleProgress sends quiz results to user-service asynchronously.
func (s *State) recordModuleProgress(courseSlug, userID, moduleSlug string, idx, score, maxScore int, passed bool) {
	body, err := json.Marshal(moduleProgressRequest{
		UserID:      userID,
		CourseSlug:  courseSlug,
		ModuleIndex: idx,
		ModuleSlug:  moduleSlug,
		Score:       score,
		MaxScore:    maxScore,
		Passed:      passed,
	})
	if err != nil {
		return
	}

	go func() {
		// Detached from the request lifecycle on purpose: the HTTP response
		// has already been (or is about to be) sent, so this uses a fresh,
		// independently-timed context rather than the request's context,
		// which may already be canceled by the time this goroutine runs.
		ctx, cancel := context.WithTimeout(context.Background(), recordModuleProgressTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			s.Config.UserServiceURL+"/internal/progress/module", bytes.NewReader(body))
		if err != nil {
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
}

// completedSlugs builds a set of module slugs the user has completed
// (viewed or passed). modules is the visible module list; viewedMap comes
// from viewedLessons; progressMap comes from fetchModuleProgress.
func completedSlugs(modules []content.Module, viewedMap map[string]bool, progressMap map[int]moduleProgressData) map[string]bool {
	out := make(map[string]bool)
	for slug := range viewedMap {
		out[slug] = true
	}

	for idx, p := range progressMap {
		if p.Passed && idx >= 0 && idx < len(modules) {
			out[modules[idx].Slug()] = true
		}
	}

	return out
}

// isLocked returns true if any prerequisite module slug has not been completed.
func isLocked(prereqs []string, done map[string]bool) bool {
	for _, slug := range prereqs {
		if !done[slug] {
			return true
		}
	}

	return false
}

// coursePrereqSummary holds the total score, passed module slugs, and viewed
// lesson count for a user in a specific course — used to evaluate cross-course
// prerequisites.
type coursePrereqSummary struct {
	TotalScore    int
	PassedModules map[string]bool
	ViewedCount   int // number of lessons marked complete (text/video/image)
}

// emptyCoursePrereqSummary is returned by fetchCoursePrereqSummary whenever
// the user-service call fails, so callers can treat it as "no progress"
// without special-casing errors.
func emptyCoursePrereqSummary() coursePrereqSummary {
	return coursePrereqSummary{PassedModules: map[string]bool{}}
}

// fetchCoursePrereqSummary calls the user-service internal API to retrieve the
// total accumulated score and the set of passed module slugs for the given user
// in the given prerequisite course.
func (s *State) fetchCoursePrereqSummary(ctx context.Context, userID, courseSlug string) coursePrereqSummary {
	reqURL, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/course-summary")
	if err != nil {
		return emptyCoursePrereqSummary()
	}

	q := reqURL.Query()
	q.Set("userId", userID)
	q.Set("courseSlug", courseSlug)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return emptyCoursePrereqSummary()
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return emptyCoursePrereqSummary()
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		TotalScore    int      `json:"totalScore"`
		PassedModules []string `json:"passedModules"`
		ViewedCount   int      `json:"viewedCount"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return emptyCoursePrereqSummary()
	}

	summary := coursePrereqSummary{
		TotalScore:    result.TotalScore,
		ViewedCount:   result.ViewedCount,
		PassedModules: make(map[string]bool, len(result.PassedModules)),
	}
	for _, slug := range result.PassedModules {
		summary.PassedModules[slug] = true
	}

	return summary
}

// checkCoursePrerequisites returns false if any course-level prerequisite
// declared on the course is not yet satisfied by the user:
//   - If MinScore > 0: the sum of the user's best scores in the prereq
//     course must be >= MinScore.
//   - If Modules is non-empty: every listed module slug must be passed
//     in the prereq course.
//   - If neither is set: only the existence of any score is required
//     (any attempt).
func (s *State) checkCoursePrerequisites(ctx context.Context, prereqs []content.CoursePrerequisite, userID string) bool {
	for _, prereq := range prereqs {
		summary := s.fetchCoursePrereqSummary(ctx, userID, prereq.Course)
		if prereq.MinScore > 0 && summary.TotalScore < prereq.MinScore {
			return false
		}

		for _, modSlug := range prereq.Modules {
			if !summary.PassedModules[modSlug] {
				return false
			}
		}
		// If neither MinScore nor Modules are set, require at least some progress
		// (either quiz score or viewed lessons — covers text-only courses).
		if prereq.MinScore == 0 && len(prereq.Modules) == 0 && summary.TotalScore == 0 && summary.ViewedCount == 0 {
			return false
		}
	}

	return true
}

// ListModules godoc
// @Summary   List modules for a course
// @Tags      Modules
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200   {object}  map[string]interface{}
// @Failure   404   {object}  map[string]string
// @Router    /api/courses/{slug}/modules [get].
func (s *State) ListModules(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")

	claims := s.claims(req)
	if claims == nil {
		s.Error(writer, http.StatusUnauthorized, "Unauthorized")

		return
	}

	isAdmin := claims.Role == roleAdmin

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureModuleAccess(writer, req, course, courseSlug, claims, isAdmin) {
		return
	}

	modules := s.visibleModules(course, req) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param
	userID := claims.Subject

	viewed := s.viewedLessons(req, courseSlug, userID)
	progress := s.fetchModuleProgress(req, courseSlug, userID)
	done := completedSlugs(modules, viewed, progress)

	out := make([]moduleResponse, 0, len(modules))
	for idx, mod := range modules {
		locked := !isAdmin && isLocked(mod.Prerequisites, done)
		out = append(out, buildModuleListEntry(mod, idx, isAdmin, locked, viewed, progress))
	}

	s.JSON(writer, http.StatusOK, map[string]any{"modules": out})
}

// ensureModuleAccess auto-enrolls the user in public courses, verifies
// enrollment for private ones, and checks cross-course prerequisites. It
// writes an error response and returns false if access should be denied.
// Admins bypass all of these checks.
func (s *State) ensureModuleAccess(writer http.ResponseWriter, req *http.Request, course *content.Course, courseSlug string, claims *middleware.Claims, isAdmin bool) bool {
	if isAdmin {
		return true
	}

	if course.IsPublic {
		s.autoEnroll(claims.Subject, courseSlug) //nolint:contextcheck // content.GitCache/user-service fetch helpers don't accept a context param
	} else if !s.isEnrolled(req, courseSlug, claims.Subject) {
		s.Error(writer, http.StatusForbidden, "Enroll in this course to access it")

		return false
	}

	if len(course.Prerequisites) > 0 && !s.checkCoursePrerequisites(req.Context(), course.Prerequisites, claims.Subject) {
		s.Error(writer, http.StatusForbidden, "Complete prerequisite courses first")

		return false
	}

	return true
}

// resolveModuleIndex parses and bounds-checks a module index path param
// against the number of visible modules in the course.
func resolveModuleIndex(indexStr string, moduleCount int) (int, bool) {
	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= moduleCount {
		return 0, false
	}

	return idx, true
}

// buildModuleListEntry assembles the per-module summary returned by
// ListModules, applying visibility rules for admin-only fields and lab URLs.
func buildModuleListEntry(mod content.Module, idx int, isAdmin, locked bool, viewed map[string]bool, progress map[int]moduleProgressData) moduleResponse {
	prog := progress[idx]

	resp := moduleResponse{
		Index:         idx,
		Name:          mod.Name,
		Slug:          mod.Slug(),
		Type:          mod.Type,
		Viewed:        viewed[mod.Slug()],
		Hidden:        mod.Hidden,
		Inline:        mod.Inline,
		Locked:        locked,
		Prerequisites: mod.Prerequisites,
		BestScore:     prog.BestScore,
		MaxScore:      prog.MaxScore,
		Passed:        prog.Passed,
		Attempts:      prog.Attempts,
	}
	if isAdmin {
		resp.Src = mod.Src
		resp.Ref = mod.Ref
		resp.Path = mod.Path
	}
	// Lab modules expose their URLs to everyone — the frontend
	// opens labUrl in a new tab (student-facing GitLab link).
	if mod.Type == moduleTypeLab && !locked {
		resp.Src = mod.Src
		resp.Ref = mod.Ref
		resp.Path = mod.Path
		resp.LabURL = mod.LabURL
	}

	if mod.Type == moduleTypeQuiz {
		if mod.HasQuestions() {
			resp.QuestionCount = len(mod.Questions)
		} else if mod.HasGitContent() {
			resp.QuestionCount = 0 // unknown until fetched
		}
	}

	return resp
}

// GetModule godoc
// @Summary   Get a module by index
// @Tags      Modules
// @Security  BearerAuth
// @Produce   json
// @Param     slug   path  string  true  "Course slug"
// @Param     index  path  int     true  "Module index (0-based)"
// @Success   200    {object}  moduleResponse
// @Failure   404    {object}  map[string]string
// @Router    /api/courses/{slug}/modules/{index} [get].
func (s *State) GetModule(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	indexStr := param(req, "index")

	claims := s.claims(req)
	if claims == nil {
		s.Error(writer, http.StatusUnauthorized, "Unauthorized")

		return
	}

	isAdmin := claims.Role == roleAdmin

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureModuleAccess(writer, req, course, courseSlug, claims, isAdmin) {
		return
	}

	modules := s.visibleModules(course, req) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param

	idx, ok := resolveModuleIndex(indexStr, len(modules))
	if !ok {
		s.Error(writer, http.StatusNotFound, "Module not found")

		return
	}

	mod := modules[idx]
	resp := s.buildModuleDetailResponse(req, mod, idx, courseSlug, claims.Subject)

	if !s.populateModuleContent(writer, &resp, mod, isAdmin, claims.Subject, courseSlug, idx) { //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param
		return
	}

	// If the next module is an inline quiz, embed it in the response.
	resp.InlineQuiz = s.buildInlineQuizResponse(req, modules, idx, courseSlug, claims.Subject, isAdmin) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param

	s.JSON(writer, http.StatusOK, resp)
}

// buildModuleDetailResponse assembles the base module detail fields — quiz
// question count, viewed state, and check metadata — shared by GetModule
// before type-specific content is resolved.
func (s *State) buildModuleDetailResponse(req *http.Request, mod content.Module, idx int, courseSlug, userID string) moduleResponse {
	resp := moduleResponse{
		Index:  idx,
		Name:   mod.Name,
		Slug:   mod.Slug(),
		Type:   mod.Type,
		Hidden: mod.Hidden,
		Inline: mod.Inline,
	}
	if mod.Type == moduleTypeQuiz && mod.HasQuestions() {
		resp.QuestionCount = len(mod.Questions)
	}

	viewed := s.viewedLessons(req, courseSlug, userID)
	resp.Viewed = viewed[mod.Slug()]

	// Lab modules with git content support the /check endpoint.
	if mod.Type == moduleTypeLab && mod.HasGitContent() {
		resp.HasCheck = true
		resp.LabURL = mod.LabURL
	}
	// Local check modules (Tauri) expose check meta to the frontend.
	if mod.CheckProvider == "local" {
		resp.HasCheck = true
		resp.CheckProvider = mod.CheckProvider
		resp.CheckType = mod.CheckType
		resp.CheckParams = mod.CheckParams
		resp.Steps = mod.Steps
	}

	return resp
}

// populateModuleContent resolves and attaches a module's body content
// (replicated media path, lab/text markdown, or quiz questions) onto resp.
// It writes an error response and returns false if content resolution fails.
func (s *State) populateModuleContent(writer http.ResponseWriter, resp *moduleResponse, mod content.Module, isAdmin bool, userID, courseSlug string, idx int) bool {
	switch mod.Type {
	case moduleTypeVideo, moduleTypeImage:
		resp.Content = content.ReplicatedPath(mod, s.Config.UploadsDir)
	case moduleTypeLab:
		text, err := s.moduleGitOrInlineContent(mod)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Failed to fetch lab content")

			return false
		}

		resp.Content = text
	case moduleTypeText:
		text, err := s.moduleGitOrInlineContent(mod)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Failed to fetch module content")

			return false
		}

		resp.Content = text
	case moduleTypeQuiz:
		if !s.populateQuizContent(writer, resp, mod, isAdmin, userID, courseSlug, idx) {
			return false
		}
	}

	return true
}

// moduleGitOrInlineContent returns a module's markdown body, fetching it from
// the git source when the module references external content.
func (s *State) moduleGitOrInlineContent(mod content.Module) (string, error) {
	if !mod.HasGitContent() {
		return mod.Content(), nil
	}

	data, err := s.GitCache.FetchModuleContent(mod.Src, mod.Ref, mod.Path, s.tokenForRepo(mod.Src))
	if err != nil {
		return "", fmt.Errorf("fetch module content: %w", err)
	}

	return string(data), nil
}

// populateQuizContent resolves quiz questions (from git or inline), attaches
// the sanitized questions/config to resp, and reports current per-question
// cooldowns. It writes an error response and returns false on fetch failure.
func (s *State) populateQuizContent(writer http.ResponseWriter, resp *moduleResponse, mod content.Module, isAdmin bool, userID, courseSlug string, idx int) bool {
	var quizQuestions []content.Question

	switch {
	case mod.HasGitContent():
		quiz, err := content.FetchQuizContent(s.GitCache, mod.Src, mod.Ref, mod.Path, s.tokenForRepo(mod.Src))
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Failed to fetch quiz content")

			return false
		}

		quizQuestions = quiz.Questions
		resp.Questions = sanitizeQuestions(quiz.Questions, isAdmin)
		resp.QuizConfig = &quizConfig{
			PassingScore:           quiz.PassingScore,
			Cooldown:               cooldown(quiz.Cooldown),
			MaxAttemptsPerQuestion: quiz.MaxAttemptsPerQuestion,
			LockOnMaxAttempts:      quiz.LockOnMaxAttempts,
		}
	case mod.HasQuestions():
		quizQuestions = mod.Questions
		resp.Questions = sanitizeQuestions(mod.Questions, isAdmin)
		resp.QuizConfig = &quizConfig{
			PassingScore:           mod.PassingScore,
			Cooldown:               cooldown(mod.Cooldown),
			MaxAttemptsPerQuestion: mod.MaxAttemptsPerQuestion,
			LockOnMaxAttempts:      mod.LockOnMaxAttempts,
		}
	}

	resp.Content = ""

	// Include current cooldown state so frontend persists it across refreshes.
	if len(quizQuestions) > 0 {
		if cooldowns := s.moduleCooldowns(quizQuestions, userID, courseSlug, idx); len(cooldowns) > 0 {
			resp.Cooldowns = cooldowns
		}
	}

	return true
}

// moduleCooldowns returns the current cooldown/attempt state for each
// question in quizQuestions that has an active cooldown or recorded attempts.
func (s *State) moduleCooldowns(quizQuestions []content.Question, userID, courseSlug string, idx int) map[string]cooldownState {
	cooldowns := make(map[string]cooldownState)

	for _, qq := range quizQuestions {
		remaining, attempts := s.CooldownTracker.CheckModule(userID, courseSlug, idx, qq.ID)
		if remaining > 0 || attempts > 0 {
			cooldowns[qq.ID] = cooldownState{
				RemainingSeconds: int(remaining.Seconds()),
				Attempts:         attempts,
				Locked:           false,
			}
		}
	}

	return cooldowns
}

// buildInlineQuizResponse embeds the following module's quiz inline in the
// response when that module is marked Inline — this lets the frontend render
// a lesson immediately followed by its check-for-understanding quiz.
func (s *State) buildInlineQuizResponse(req *http.Request, modules []content.Module, idx int, courseSlug, userID string, isAdmin bool) *inlineQuizResponse {
	if idx+1 >= len(modules) {
		return nil
	}

	next := modules[idx+1]
	if next.Type != moduleTypeQuiz || !next.Inline {
		return nil
	}

	inlineQuiz := &inlineQuizResponse{
		Index:        idx + 1,
		Name:         next.Name,
		Slug:         next.Slug(),
		PassingScore: next.PassingScore,
	}

	quizQuestions := s.populateInlineQuizQuestions(inlineQuiz, next, isAdmin)
	if len(quizQuestions) == 0 {
		return nil
	}

	inlineQuiz.QuestionCount = len(quizQuestions)

	maxScore := 0
	for _, q := range quizQuestions {
		maxScore += q.Points
	}

	inlineQuiz.MaxScore = maxScore

	prog := s.fetchModuleProgress(req, courseSlug, userID)
	if p, ok := prog[idx+1]; ok {
		inlineQuiz.BestScore = p.BestScore
		inlineQuiz.Attempts = p.Attempts
		inlineQuiz.Passed = p.Passed
	}

	return inlineQuiz
}

// populateInlineQuizQuestions resolves next's quiz questions (from git or
// inline) into inlineQuiz and returns them for scoring/metadata purposes.
func (s *State) populateInlineQuizQuestions(inlineQuiz *inlineQuizResponse, next content.Module, isAdmin bool) []content.Question {
	switch {
	case next.HasGitContent():
		quiz, err := content.FetchQuizContent(s.GitCache, next.Src, next.Ref, next.Path, s.tokenForRepo(next.Src))
		if err != nil {
			return nil
		}

		inlineQuiz.Questions = sanitizeQuestions(quiz.Questions, isAdmin)
		inlineQuiz.PassingScore = quiz.PassingScore
		inlineQuiz.QuizConfig = &quizConfig{
			PassingScore:           quiz.PassingScore,
			Cooldown:               cooldown(quiz.Cooldown),
			MaxAttemptsPerQuestion: quiz.MaxAttemptsPerQuestion,
			LockOnMaxAttempts:      quiz.LockOnMaxAttempts,
		}

		return quiz.Questions
	case next.HasQuestions():
		inlineQuiz.Questions = sanitizeQuestions(next.Questions, isAdmin)
		inlineQuiz.QuizConfig = &quizConfig{
			PassingScore:           next.PassingScore,
			Cooldown:               cooldown(next.Cooldown),
			MaxAttemptsPerQuestion: next.MaxAttemptsPerQuestion,
			LockOnMaxAttempts:      next.LockOnMaxAttempts,
		}

		return next.Questions
	default:
		return nil
	}
}

// SubmitModule godoc
// @Summary   Submit quiz answers for a module
// @Tags      Modules
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug   path  string                true  "Course slug"
// @Param     index  path  int                   true  "Module index (0-based)"
// @Param     body   content.SubmitRequest true  "Quiz answers"
// @Success   200    {object}  submitResponse
// @Failure   400    {object}  map[string]string
// @Failure   404    {object}  map[string]string
// @Router    /api/courses/{slug}/modules/{index}/submit [post].
func (s *State) SubmitModule(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")

	claims := s.claims(req)
	if claims == nil {
		s.Error(writer, http.StatusUnauthorized, "Authentication required")

		return
	}

	userID := claims.Subject
	isAdmin := claims.Role == roleAdmin

	course := s.Content.Get(courseSlug)
	if course == nil {
		s.Error(writer, http.StatusNotFound, "Course not found")

		return
	}

	if !s.ensureModuleAccess(writer, req, course, courseSlug, claims, isAdmin) {
		return
	}

	modules := s.visibleModules(course, req) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param

	indexStr := param(req, "index")

	idx, indexValid := resolveModuleIndex(indexStr, len(modules))
	if !indexValid {
		s.Error(writer, http.StatusNotFound, "Module not found")

		return
	}

	mod := modules[idx]
	if mod.Type != moduleTypeQuiz {
		s.Error(writer, http.StatusBadRequest, "Module is not a quiz")

		return
	}

	// Resolve questions: fetch from git repo first, then fallback to inline.
	questions, passingScore, cooldownSpec, quizResolved := s.resolveSubmitQuiz(writer, mod) //nolint:contextcheck // content.GitCache fetch helpers don't accept a context param
	if !quizResolved {
		return
	}

	s.finalizeSubmission(writer, req, mod, questions, passingScore, cooldownSpec, userID, courseSlug, idx) //nolint:contextcheck // fire-and-forget async POST intentionally detached from the request context (see recordModuleProgress)
}

// finalizeSubmission decodes a quiz submission body, rejects it if any of
// the resolved questions is still under a cooldown, then scores it, records
// per-question cooldowns and overall module progress, and writes the
// resulting submitResponse to the client.
func (s *State) finalizeSubmission(writer http.ResponseWriter, req *http.Request, mod content.Module, questions []content.Question, passingScore int, cooldownSpec content.CooldownSpec, userID, courseSlug string, idx int) {
	var submission content.SubmitRequest

	err := json.NewDecoder(req.Body).Decode(&submission)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid request body")

		return
	}

	// Check cooldowns for each question before scoring.
	if cooldowns := s.pendingCooldowns(userID, courseSlug, idx, questions); len(cooldowns) > 0 {
		s.JSON(writer, http.StatusTooEarly, map[string]any{
			"error":     "Cooldown active for some questions",
			"cooldowns": cooldowns,
		})

		return
	}

	effectiveQuiz := &content.Quiz{
		Questions:    questions,
		PassingScore: passingScore,
	}

	result := content.ScoreQuiz(effectiveQuiz, submission.Answers)

	// Record cooldowns for wrong answers.
	respCooldowns := s.recordQuestionCooldowns(userID, courseSlug, idx, result.QuestionResults, cooldownSpec, mod.MaxAttemptsPerQuestion, mod.LockOnMaxAttempts)

	s.recordModuleProgress(courseSlug, userID, mod.Slug(), idx, result.TotalScore, result.MaxScore, result.Passed)

	apiResults := make([]questionResultAPI, len(result.QuestionResults))
	for i, questionResult := range result.QuestionResults {
		apiResults[i] = questionResultAPI{
			QuestionID:    questionResult.QuestionID,
			Type:          questionResult.Type,
			IsCorrect:     questionResult.IsCorrect,
			PointsEarned:  questionResult.PointsEarned,
			PointsMax:     questionResult.PointsMax,
			CorrectAnswer: questionResult.CorrectAnswer,
			Feedback:      questionResult.Feedback,
			SourceRefs:    questionResult.SourceRefs,
		}
	}

	s.JSON(writer, http.StatusOK, submitResponse{
		TotalScore:      result.TotalScore,
		MaxScore:        result.MaxScore,
		Passed:          result.Passed,
		QuestionResults: apiResults,
		Cooldowns:       respCooldowns,
	})
}

// resolveSubmitQuiz resolves the questions, passing score, and cooldown spec
// for a quiz module, fetching from git when the module references external
// content and falling back to inline questions otherwise. It writes an error
// response and returns ok=false when no questions can be resolved.
func (s *State) resolveSubmitQuiz(writer http.ResponseWriter, mod content.Module) ([]content.Question, int, content.CooldownSpec, bool) {
	switch {
	case mod.HasGitContent():
		quiz, err := content.FetchQuizContent(s.GitCache, mod.Src, mod.Ref, mod.Path, s.tokenForRepo(mod.Src))
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Failed to fetch quiz content")

			return nil, 0, content.CooldownSpec{}, false
		}

		return quiz.Questions, quiz.PassingScore, quiz.Cooldown, true
	case mod.HasQuestions():
		return mod.Questions, mod.PassingScore, mod.Cooldown, true
	default:
		s.Error(writer, http.StatusNotFound, "No questions found for this quiz")

		return nil, 0, content.CooldownSpec{}, false
	}
}

// pendingCooldowns returns any active per-question cooldowns for userID on
// this quiz module, which should block a resubmission attempt.
func (s *State) pendingCooldowns(userID, courseSlug string, idx int, questions []content.Question) map[string]cooldownState {
	cooldowns := make(map[string]cooldownState)

	for _, qq := range questions {
		remaining, attempts := s.CooldownTracker.CheckModule(userID, courseSlug, idx, qq.ID)
		if remaining > 0 {
			cooldowns[qq.ID] = cooldownState{
				RemainingSeconds: int(remaining.Seconds()),
				Attempts:         attempts,
			}
		}
	}

	return cooldowns
}

// recordQuestionCooldowns records a cooldown for every incorrectly answered
// question (clearing any existing cooldown for correct ones) and returns the
// resulting per-question cooldown state to report back to the client.
func (s *State) recordQuestionCooldowns(userID, courseSlug string, idx int, results []content.QuestionResult, spec content.CooldownSpec, maxAttempts *int, lockOnMax bool) map[string]cooldownState {
	respCooldowns := make(map[string]cooldownState)

	for _, questionResult := range results {
		if questionResult.IsCorrect {
			s.CooldownTracker.ClearModule(userID, courseSlug, idx, questionResult.QuestionID)

			continue
		}

		remaining, locked := s.CooldownTracker.RecordModule(userID, courseSlug, idx, questionResult.QuestionID, spec, maxAttempts, lockOnMax)
		_, attempts := s.CooldownTracker.CheckModule(userID, courseSlug, idx, questionResult.QuestionID)
		respCooldowns[questionResult.QuestionID] = cooldownState{
			RemainingSeconds: int(remaining.Seconds()),
			Attempts:         attempts,
			Locked:           locked,
		}
	}

	return respCooldowns
}
