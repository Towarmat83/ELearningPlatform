package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/elearning/course-service/internal/content"
)

type publicAnswer struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct bool   `json:"correct,omitempty"`
}

type publicQuestion struct {
	ID             string                  `json:"id"`
	Type           string                  `json:"type"`
	Difficulty     string                  `json:"difficulty,omitempty"`
	Points         int                     `json:"points"`
	Question       string                  `json:"question"`
	Answers        []publicAnswer          `json:"answers,omitempty"`
	Items          []content.OrderItem     `json:"items,omitempty"`
	PartialScoring *content.PartialScoring `json:"partial_scoring,omitempty"`
	SourceRefs     []content.SourceRef     `json:"source_refs,omitempty"`
}

func sanitizeQuestions(qq []content.Question, admin bool) []any {
	out := make([]any, len(qq))
	for i, q := range qq {
		pq := publicQuestion{
			ID:             q.ID,
			Type:           q.Type,
			Difficulty:     q.Difficulty,
			Points:         q.Points,
			Question:       q.Question,
			Items:          q.Items,
			PartialScoring: q.PartialScoring,
			SourceRefs:     q.Feedback.SourceRefs,
		}
		for _, a := range q.Answers {
			pa := publicAnswer{ID: a.ID, Text: a.Text}
			if admin {
				pa.Correct = a.Correct
			}
			pq.Answers = append(pq.Answers, pa)
		}
		out[i] = pq
	}
	return out
}

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
	BestScore     int                      `json:"best_score,omitempty"`
	MaxScore      int                      `json:"max_score,omitempty"`
	Passed        bool                     `json:"passed,omitempty"`
	Attempts      int                      `json:"attempts,omitempty"`
	QuestionCount int                      `json:"question_count,omitempty"`
	Questions     []any                    `json:"questions,omitempty"`
	QuizConfig    *quizConfig              `json:"quiz_config,omitempty"`
	Cooldowns     map[string]cooldownState `json:"cooldowns,omitempty"`
	// Admin-only fields (omitted for regular users)
	Src  string `json:"src,omitempty"`
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`
}

type quizConfig struct {
	PassingScore           int      `json:"passing_score"`
	Cooldown               cooldown `json:"cooldown,omitempty"`
	MaxAttemptsPerQuestion *int     `json:"max_attempts_per_question,omitempty"`
	LockOnMaxAttempts      bool     `json:"lock_on_max_attempts"`
}

type cooldown struct {
	Strategy    string  `json:"strategy"`
	BaseSeconds int     `json:"base_seconds"`
	Multiplier  float64 `json:"multiplier"`
	MaxSeconds  int     `json:"max_seconds"`
}

type submitResponse struct {
	TotalScore      int                      `json:"total_score"`
	MaxScore        int                      `json:"max_score"`
	Passed          bool                     `json:"passed"`
	QuestionResults []questionResultAPI      `json:"question_results"`
	Cooldowns       map[string]cooldownState `json:"cooldowns,omitempty"`
}

type cooldownState struct {
	RemainingSeconds int  `json:"remaining_seconds"`
	Attempts         int  `json:"attempts"`
	Locked           bool `json:"locked"`
}

type questionResultAPI struct {
	QuestionID    string              `json:"question_id"`
	Type          string              `json:"type"`
	IsCorrect     bool                `json:"is_correct"`
	PointsEarned  int                 `json:"points_earned"`
	PointsMax     int                 `json:"points_max"`
	CorrectAnswer interface{}         `json:"correct_answer,omitempty"`
	Feedback      string              `json:"feedback,omitempty"`
	SourceRefs    []content.SourceRef `json:"source_refs,omitempty"`
}

func (s *State) viewedLessons(r *http.Request, courseSlug, userID string) map[string]bool {
	u, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/viewed")
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Set("user_id", userID)
	q.Set("course_slug", courseSlug)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Viewed []string `json:"viewed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	m := make(map[string]bool, len(result.Viewed))
	for _, slug := range result.Viewed {
		m[slug] = true
	}
	return m
}

// moduleProgressData holds per-module quiz progress fetched from user-service.
type moduleProgressData struct {
	BestScore int
	MaxScore  int
	Passed    bool
	Attempts  int
}

// fetchModuleProgress fetches quiz progress for all modules in a course.
func (s *State) fetchModuleProgress(r *http.Request, courseSlug, userID string) map[int]moduleProgressData {
	u, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/modules")
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Set("user_id", userID)
	q.Set("course_slug", courseSlug)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Progress []struct {
			ModuleIndex int  `json:"module_index"`
			BestScore   int  `json:"best_score"`
			MaxScore    int  `json:"max_score"`
			Passed      bool `json:"passed"`
			Attempts    int  `json:"attempts"`
		} `json:"progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	m := make(map[int]moduleProgressData, len(result.Progress))
	for _, p := range result.Progress {
		m[p.ModuleIndex] = moduleProgressData{
			BestScore: p.BestScore,
			MaxScore:  p.MaxScore,
			Passed:    p.Passed,
			Attempts:  p.Attempts,
		}
	}
	return m
}

// recordModuleProgress sends quiz results to user-service asynchronously.
func (s *State) recordModuleProgress(courseSlug, userID, moduleSlug string, idx, score, maxScore int, passed bool) {
	body, _ := json.Marshal(map[string]any{
		"user_id":      userID,
		"course_slug":  courseSlug,
		"module_index": idx,
		"module_slug":  moduleSlug,
		"score":        score,
		"max_score":    maxScore,
		"passed":       passed,
	})
	go func() {
		resp, err := http.Post(s.Config.UserServiceURL+"/internal/progress/module",
			"application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}()
}

// completedSlugs builds a set of module slugs the user has completed (viewed or passed).
// modules is the visible module list; viewedMap from viewedLessons; progressMap from fetchModuleProgress.
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

// fetchCoursePrereqSummary calls the user-service internal API to retrieve the
// total accumulated score and the set of passed module slugs for the given user
// in the given prerequisite course.
func (s *State) fetchCoursePrereqSummary(userID, courseSlug string) coursePrereqSummary {
	u, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/course-summary")
	if err != nil {
		return coursePrereqSummary{PassedModules: map[string]bool{}}
	}
	q := u.Query()
	q.Set("user_id", userID)
	q.Set("course_slug", courseSlug)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return coursePrereqSummary{PassedModules: map[string]bool{}}
	}
	defer resp.Body.Close()

	var result struct {
		TotalScore    int      `json:"total_score"`
		PassedModules []string `json:"passed_modules"`
		ViewedCount   int      `json:"viewed_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return coursePrereqSummary{PassedModules: map[string]bool{}}
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

// checkCoursePrerequisites returns false if any course-level prerequisite declared
// on the course is not yet satisfied by the user:
//   - If MinScore > 0: the sum of the user's best scores in the prereq course must
//     be >= MinScore.
//   - If Modules is non-empty: every listed module slug must be passed in the prereq
//     course.
//   - If neither is set: only the existence of any score is required (any attempt).
func (s *State) checkCoursePrerequisites(prereqs []content.CoursePrerequisite, userID string) bool {
	for _, p := range prereqs {
		summary := s.fetchCoursePrereqSummary(userID, p.Course)
		if p.MinScore > 0 && summary.TotalScore < p.MinScore {
			return false
		}
		for _, modSlug := range p.Modules {
			if !summary.PassedModules[modSlug] {
				return false
			}
		}
		// If neither MinScore nor Modules are set, require at least some progress
		// (either quiz score or viewed lessons — covers text-only courses).
		if p.MinScore == 0 && len(p.Modules) == 0 && summary.TotalScore == 0 && summary.ViewedCount == 0 {
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
// @Router    /api/courses/{slug}/modules [get]
func (s *State) ListModules(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" {
		if c.IsPublic {
			s.autoEnroll(claims.Subject, courseSlug)
		} else if !s.isEnrolled(r, courseSlug, claims.Subject) {
			s.Error(w, http.StatusForbidden, "Enroll in this course to access it")
			return
		}
	}

	// Enforce cross-course prerequisites for non-admins.
	if claims.Role != "admin" && len(c.Prerequisites) > 0 {
		if !s.checkCoursePrerequisites(c.Prerequisites, claims.Subject) {
			s.Error(w, http.StatusForbidden, "Complete prerequisite courses first")
			return
		}
	}

	modules := s.visibleModules(c, r)
	userID := claims.Subject

	viewed := s.viewedLessons(r, courseSlug, userID)
	progress := s.fetchModuleProgress(r, courseSlug, userID)
	done := completedSlugs(modules, viewed, progress)

	out := make([]moduleResponse, 0, len(modules))
	for i, m := range modules {
		locked := claims.Role != "admin" && isLocked(m.Prerequisites, done)
		p := progress[i]
		resp := moduleResponse{
			Index:         i,
			Name:          m.Name,
			Slug:          m.Slug(),
			Type:          m.Type,
			Viewed:        viewed[m.Slug()],
			Hidden:        m.Hidden,
			Locked:        locked,
			Prerequisites: m.Prerequisites,
			BestScore:     p.BestScore,
			MaxScore:      p.MaxScore,
			Passed:        p.Passed,
			Attempts:      p.Attempts,
		}
		if claims != nil && claims.Role == "admin" {
			resp.Src = m.Src
			resp.Ref = m.Ref
			resp.Path = m.Path
		}
		if m.Type == "quiz" {
			if m.HasQuestions() {
				resp.QuestionCount = len(m.Questions)
			} else if m.HasGitContent() {
				resp.QuestionCount = 0 // unknown until fetched
			}
		}
		out = append(out, resp)
	}
	s.JSON(w, http.StatusOK, map[string]any{"modules": out})
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
// @Router    /api/courses/{slug}/modules/{index} [get]
func (s *State) GetModule(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	indexStr := param(r, "index")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" {
		if c.IsPublic {
			s.autoEnroll(claims.Subject, courseSlug)
		} else if !s.isEnrolled(r, courseSlug, claims.Subject) {
			s.Error(w, http.StatusForbidden, "Enroll in this course to access it")
			return
		}
	}

	// Enforce cross-course prerequisites for non-admins.
	if claims.Role != "admin" && len(c.Prerequisites) > 0 {
		if !s.checkCoursePrerequisites(c.Prerequisites, claims.Subject) {
			s.Error(w, http.StatusForbidden, "Complete prerequisite courses first")
			return
		}
	}

	modules := s.visibleModules(c, r)

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(w, http.StatusNotFound, "Module not found")
		return
	}

	m := modules[idx]
	resp := moduleResponse{
		Index:  idx,
		Name:   m.Name,
		Slug:   m.Slug(),
		Type:   m.Type,
		Hidden: m.Hidden,
	}
	if m.Type == "quiz" && m.HasQuestions() {
		resp.QuestionCount = len(m.Questions)
	}

	viewed := s.viewedLessons(r, courseSlug, claims.Subject)
	resp.Viewed = viewed[m.Slug()]

	switch m.Type {
	case "video", "image":
		resp.Content = content.ReplicatedPath(m, s.Config.UploadsDir)
	case "text":
		if m.HasGitContent() {
			data, err := content.FetchModuleContent(m.Src, m.Ref, m.Path, s.tokenForRepo(m.Src))
			if err != nil {
				s.Error(w, http.StatusInternalServerError, "Failed to fetch module content")
				return
			}
			resp.Content = string(data)
		} else {
			resp.Content = m.Content()
		}
	case "quiz":
		var quizQuestions []content.Question
		if m.HasGitContent() {
			quiz, err := content.FetchQuizContent(m.Src, m.Ref, m.Path, s.tokenForRepo(m.Src))
			if err != nil {
				s.Error(w, http.StatusInternalServerError, "Failed to fetch quiz content")
				return
			}
			quizQuestions = quiz.Questions
			isAdmin := claims != nil && claims.Role == "admin"
			resp.Questions = sanitizeQuestions(quiz.Questions, isAdmin)
			resp.QuizConfig = &quizConfig{
				PassingScore:           quiz.PassingScore,
				Cooldown:               cooldown(quiz.Cooldown),
				MaxAttemptsPerQuestion: quiz.MaxAttemptsPerQuestion,
				LockOnMaxAttempts:      quiz.LockOnMaxAttempts,
			}
		} else if m.HasQuestions() {
			quizQuestions = m.Questions
			isAdmin := claims != nil && claims.Role == "admin"
			resp.Questions = sanitizeQuestions(m.Questions, isAdmin)
			resp.QuizConfig = &quizConfig{
				PassingScore:           m.PassingScore,
				Cooldown:               cooldown(m.Cooldown),
				MaxAttemptsPerQuestion: m.MaxAttemptsPerQuestion,
				LockOnMaxAttempts:      m.LockOnMaxAttempts,
			}
		}
		resp.Content = ""

		// Include current cooldown state so frontend persists it across refreshes
		if len(quizQuestions) > 0 && claims != nil {
			cooldowns := make(map[string]cooldownState)
			for _, qq := range quizQuestions {
				remaining, attempts := s.CooldownTracker.CheckModule(claims.Subject, courseSlug, idx, qq.ID)
				if remaining > 0 || attempts > 0 {
					cooldowns[qq.ID] = cooldownState{
						RemainingSeconds: int(remaining.Seconds()),
						Attempts:         attempts,
						Locked:           false,
					}
				}
			}
			if len(cooldowns) > 0 {
				resp.Cooldowns = cooldowns
			}
		}
	}

	s.JSON(w, http.StatusOK, resp)
}

// SubmitModule godoc
// @Summary   Submit quiz answers for a module
// @Tags      Modules
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug   path  string                true  "Course slug"
// @Param     index  path  int                   true  "Module index (0-based)"
// @Param     body   body  content.SubmitRequest true  "Quiz answers"
// @Success   200    {object}  submitResponse
// @Failure   400    {object}  map[string]string
// @Failure   404    {object}  map[string]string
// @Router    /api/courses/{slug}/modules/{index}/submit [post]
func (s *State) SubmitModule(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)
	if claims == nil {
		s.Error(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID := claims.Subject

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" {
		if c.IsPublic {
			s.autoEnroll(userID, courseSlug)
		} else if !s.isEnrolled(r, courseSlug, userID) {
			s.Error(w, http.StatusForbidden, "Enroll in this course to access it")
			return
		}
	}

	// Enforce cross-course prerequisites for non-admins.
	if claims.Role != "admin" && len(c.Prerequisites) > 0 {
		if !s.checkCoursePrerequisites(c.Prerequisites, userID) {
			s.Error(w, http.StatusForbidden, "Complete prerequisite courses first")
			return
		}
	}

	modules := s.visibleModules(c, r)

	indexStr := param(r, "index")
	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(w, http.StatusNotFound, "Module not found")
		return
	}

	m := modules[idx]
	if m.Type != "quiz" {
		s.Error(w, http.StatusBadRequest, "Module is not a quiz")
		return
	}

	// Resolve questions: fetch from git repo first, then fallback to inline
	var questions []content.Question
	var passingScore int
	var cooldownSpec content.CooldownSpec
	if m.HasGitContent() {
		quiz, err := content.FetchQuizContent(m.Src, m.Ref, m.Path, s.tokenForRepo(m.Src))
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Failed to fetch quiz content")
			return
		}
		questions = quiz.Questions
		passingScore = quiz.PassingScore
		cooldownSpec = content.CooldownSpec(quiz.Cooldown)
	} else if m.HasQuestions() {
		questions = m.Questions
		passingScore = m.PassingScore
		cooldownSpec = m.Cooldown
	} else {
		s.Error(w, http.StatusNotFound, "No questions found for this quiz")
		return
	}

	var req content.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Check cooldowns for each question before scoring
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
	if len(cooldowns) > 0 {
		s.JSON(w, http.StatusTooEarly, map[string]any{
			"error":     "Cooldown active for some questions",
			"cooldowns": cooldowns,
		})
		return
	}

	// Build effective quiz for scoring
	effectiveQuiz := &content.Quiz{
		Questions:    questions,
		PassingScore: passingScore,
	}

	result := content.ScoreQuiz(effectiveQuiz, req.Answers)

	// Record cooldowns for wrong answers
	respCooldowns := make(map[string]cooldownState)
	for _, qr := range result.QuestionResults {
		if !qr.IsCorrect {
			remaining, locked := s.CooldownTracker.RecordModule(userID, courseSlug, idx, qr.QuestionID, cooldownSpec, m.MaxAttemptsPerQuestion, m.LockOnMaxAttempts)
			_, attempts := s.CooldownTracker.CheckModule(userID, courseSlug, idx, qr.QuestionID)
			respCooldowns[qr.QuestionID] = cooldownState{
				RemainingSeconds: int(remaining.Seconds()),
				Attempts:         attempts,
				Locked:           locked,
			}
		} else {
			s.CooldownTracker.ClearModule(userID, courseSlug, idx, qr.QuestionID)
		}
	}

	apiResults := make([]questionResultAPI, len(result.QuestionResults))
	for i, qr := range result.QuestionResults {
		apiResults[i] = questionResultAPI{
			QuestionID:    qr.QuestionID,
			Type:          qr.Type,
			IsCorrect:     qr.IsCorrect,
			PointsEarned:  qr.PointsEarned,
			PointsMax:     qr.PointsMax,
			CorrectAnswer: qr.CorrectAnswer,
			Feedback:      qr.Feedback,
			SourceRefs:    qr.SourceRefs,
		}
	}

	s.JSON(w, http.StatusOK, submitResponse{
		TotalScore:      result.TotalScore,
		MaxScore:        result.MaxScore,
		Passed:          result.Passed,
		QuestionResults: apiResults,
		Cooldowns:       respCooldowns,
	})
}
