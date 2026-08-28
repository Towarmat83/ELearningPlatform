package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesary/pupitre/course-service/internal/config"
	"github.com/genesary/pupitre/course-service/internal/content"
)

// singleChoiceCourse returns a minimal course with one single-choice quiz
// module for use in finalizeSubmission tests.
func singleChoiceCourse() *content.Course {
	return &content.Course{
		Slug:       "quiz-course",
		Title:      "Quiz Course",
		Difficulty: "beginner",
		IsPublic:   true,
		Modules: []content.Module{
			{
				Name: "Quiz 1",
				Type: "quiz",
				Questions: []content.Question{
					{
						ID:     "q1",
						Type:   "single",
						Points: 1,
						Answers: []content.Answer{
							{ID: "a-correct", Text: "Right answer", Correct: true},
							{ID: "a-wrong", Text: "Wrong answer", Correct: false},
						},
					},
				},
				PassingScore: 1,
			},
		},
	}
}

// encodeSubmit encodes a SubmitRequest as a POST [http.Request].
func encodeSubmit(t *testing.T, req content.SubmitRequest) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(req)
	if err != nil {
		t.Fatalf("encode submission: %v", err)
	}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/submit", &buf)
	r.Header.Set("Content-Type", "application/json")

	return r
}

// TestFinalizeSubmission_InvalidBody verifies malformed JSON returns 400.
func TestFinalizeSubmission_InvalidBody(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	course := singleChoiceCourse()
	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	mod := course.Modules[0]

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/submit", strings.NewReader("not-json"),
	)
	rec := httptest.NewRecorder()

	s.finalizeSubmission(rec, req, course, mod, 0,
		mod.Questions, mod.PassingScore, mod.Cooldown, "user-1", course.Slug, 0)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFinalizeSubmission_Pass verifies a correct submission returns 200
// and passed=true.
func TestFinalizeSubmission_Pass(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	course := singleChoiceCourse()
	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	mod := course.Modules[0]
	answer := "a-correct"
	submission := content.SubmitRequest{
		Answers: map[string]content.UserAnswer{"q1": {Single: &answer}},
	}

	rec := httptest.NewRecorder()

	s.finalizeSubmission(rec, encodeSubmit(t, submission), course, mod, 0,
		mod.Questions, mod.PassingScore, mod.Cooldown, "user-1", course.Slug, 0)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Passed bool `json:"passed"`
	}

	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Passed {
		t.Error("want passed=true for correct answer")
	}
}

// TestFinalizeSubmission_Fail verifies a wrong submission returns 200 and
// passed=false.
func TestFinalizeSubmission_Fail(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	course := singleChoiceCourse()
	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	mod := course.Modules[0]
	answer := "a-wrong"
	submission := content.SubmitRequest{
		Answers: map[string]content.UserAnswer{"q1": {Single: &answer}},
	}

	rec := httptest.NewRecorder()

	s.finalizeSubmission(rec, encodeSubmit(t, submission), course, mod, 0,
		mod.Questions, mod.PassingScore, mod.Cooldown, "user-1", course.Slug, 0)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Passed bool `json:"passed"`
	}

	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Passed {
		t.Error("want passed=false for wrong answer")
	}
}

// cooldownQuizCourse returns a course whose quiz applies a fixed 60-second
// cooldown after a wrong answer.
func cooldownQuizCourse() *content.Course {
	course := singleChoiceCourse()
	course.Modules[0].Cooldown = content.CooldownSpec{Strategy: "fixed", BaseSeconds: 60}

	return course
}

// submitAnswer runs one submission of answerID against the course's quiz
// module and returns the recorder.
func submitAnswer(t *testing.T, s *State, course *content.Course, answerID string) *httptest.ResponseRecorder {
	t.Helper()

	mod := course.Modules[0]
	submission := content.SubmitRequest{
		Answers: map[string]content.UserAnswer{"q1": {Single: &answerID}},
	}

	rec := httptest.NewRecorder()
	s.finalizeSubmission(rec, encodeSubmit(t, submission), course, mod, 0,
		mod.Questions, mod.PassingScore, mod.Cooldown, "user-1", course.Slug, 0)

	return rec
}

// TestFinalizeSubmission_CooldownPersistedAndBlocks verifies that a wrong
// answer records a cooldown and that the next submission is rejected with
// 425 until it expires. The state lives in the quiz-attempt repository
// rather than in the handler, so it survives a restart and holds across
// replicas.
func TestFinalizeSubmission_CooldownPersistedAndBlocks(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	course := cooldownQuizCourse()
	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	rec := submitAnswer(t, s, course, "a-wrong")
	if rec.Code != http.StatusOK {
		t.Fatalf("first submission: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var first submitResponse

	err := json.NewDecoder(rec.Body).Decode(&first)
	if err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	state, ok := first.Cooldowns["q1"]
	if !ok {
		t.Fatalf("want a cooldown reported for q1, got %+v", first.Cooldowns)
	}

	if state.Attempts != 1 || state.RemainingSeconds <= 0 {
		t.Errorf("want 1 attempt with a pending cooldown, got %+v", state)
	}

	// The recorded cooldown must now block a retry.
	rec = submitAnswer(t, s, course, "a-correct")
	if rec.Code != http.StatusTooEarly {
		t.Fatalf("second submission: want 425, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFinalizeSubmission_CorrectAnswerClearsCooldown verifies that answering
// correctly forgets the recorded attempts, so the learner is not left behind
// a cooldown they have already cleared.
func TestFinalizeSubmission_CorrectAnswerClearsCooldown(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	// A zero-second cooldown records the attempt without blocking the retry.
	course := singleChoiceCourse()
	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	if rec := submitAnswer(t, s, course, "a-wrong"); rec.Code != http.StatusOK {
		t.Fatalf("first submission: want 200, got %d", rec.Code)
	}

	key := attemptKey("user-1", course.Slug, 0)

	states, err := s.Repos.QuizAttempts.States(t.Context(), key, []string{"q1"})
	if err != nil {
		t.Fatalf("States after wrong answer: %v", err)
	}

	if states["q1"].Attempts != 1 {
		t.Fatalf("want 1 recorded attempt, got %+v", states["q1"])
	}

	if rec := submitAnswer(t, s, course, "a-correct"); rec.Code != http.StatusOK {
		t.Fatalf("second submission: want 200, got %d", rec.Code)
	}

	states, err = s.Repos.QuizAttempts.States(t.Context(), key, []string{"q1"})
	if err != nil {
		t.Fatalf("States after correct answer: %v", err)
	}

	if _, still := states["q1"]; still {
		t.Errorf("want the attempt forgotten after a correct answer, got %+v", states["q1"])
	}
}

// TestFinalizeSubmission_LocksOnMaxAttempts verifies that a module
// configured to lock reports locked=true once the attempt cap is reached,
// instead of handing out another cooldown.
func TestFinalizeSubmission_LocksOnMaxAttempts(t *testing.T) {
	t.Parallel()

	mock := newUserServiceMock()
	defer mock.Close()

	maxAttempts := 1

	course := singleChoiceCourse()
	course.Modules[0].MaxAttemptsPerQuestion = &maxAttempts
	course.Modules[0].LockOnMaxAttempts = true

	s := newStateWith(&config.Config{JWTSecret: "test-secret", UserServiceURL: mock.URL}, course)

	rec := submitAnswer(t, s, course, "a-wrong")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp submitResponse

	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Cooldowns["q1"].Locked {
		t.Errorf("want q1 locked at the attempt cap, got %+v", resp.Cooldowns["q1"])
	}
}
