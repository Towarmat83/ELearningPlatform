package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/elearning/course-service/internal/content"
)

type publicAnswer struct {
	ID   string `json:"id"`
	Text string `json:"text"`
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
	if admin {
		out := make([]any, len(qq))
		for i := range qq {
			out[i] = qq[i]
		}
		return out
	}
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
			pq.Answers = append(pq.Answers, publicAnswer{ID: a.ID, Text: a.Text})
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
	QuestionCount int                      `json:"question_count,omitempty"`
	Questions     []any                    `json:"questions,omitempty"`
	QuizConfig    *quizConfig              `json:"quiz_config,omitempty"`
	Cooldowns     map[string]cooldownState `json:"cooldowns,omitempty"`
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

type cooldownState struct {
	RemainingSeconds int  `json:"remaining_seconds"`
	Attempts         int  `json:"attempts"`
	Locked           bool `json:"locked"`
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

// GET /api/courses/{slug}/modules
func (s *State) ListModules(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && !c.IsPublished {
		s.Error(w, http.StatusForbidden, "Course not available")
		return
	}

	modules := s.visibleModules(c, r)
	viewed := s.viewedLessons(r, courseSlug, claims.Subject)
	out := make([]moduleResponse, 0, len(modules))
	for i, m := range modules {
		resp := moduleResponse{
			Index:  i,
			Name:   m.Name,
			Slug:   m.Slug(),
			Type:   m.Type,
			Viewed: viewed[m.Slug()],
			Hidden: m.Hidden,
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

// GET /api/courses/{slug}/modules/{index}
func (s *State) GetModule(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	indexStr := param(r, "index")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && !c.IsPublished {
		s.Error(w, http.StatusForbidden, "Course not available")
		return
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
						Locked:           false, // GetModule doesn't know if locked; submit will enforce
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

// POST /api/courses/{slug}/modules/{index}/submit
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
	if claims.Role != "admin" && !c.IsPublished {
		s.Error(w, http.StatusForbidden, "Course not available")
		return
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
