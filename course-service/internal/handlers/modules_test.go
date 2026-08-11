package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	s := newTestState(t, mock)

	course := singleChoiceCourse()
	s.Content.Put(course)

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

	s := newTestState(t, mock)

	course := singleChoiceCourse()
	s.Content.Put(course)

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

	s := newTestState(t, mock)

	course := singleChoiceCourse()
	s.Content.Put(course)

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
