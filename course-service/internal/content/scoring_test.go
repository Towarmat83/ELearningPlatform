package content

import (
	"testing"
)

func boolPtr(b bool) *bool { return &b }
func strPtr(s string) *string { return &s }
func intPtr(i int) *int { return &i }

func makeQuestion(id, typ string, points int) Question {
	return Question{
		ID:     id,
		Type:   typ,
		Points: points,
		Feedback: Feedback{
			Correct: "Correct!",
			Wrong:   "Wrong!",
		},
	}
}

// ─── ScoreSingle ───────────────────────────────────────────────────────────

func TestScoreSingle_NilAnswer(t *testing.T) {
	q := makeQuestion("q1", "single", 5)
	q.Answers = []Answer{{ID: "a", Text: "A", Correct: true}}
	r := ScoreSingle(q, nil)
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for nil answer")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
	if r.PointsMax != 5 {
		t.Errorf("expected max 5, got %d", r.PointsMax)
	}
}

func TestScoreSingle_CorrectAnswer(t *testing.T) {
	q := makeQuestion("q1", "single", 10)
	q.Answers = []Answer{
		{ID: "a", Text: "A", Correct: false},
		{ID: "b", Text: "B", Correct: true},
	}
	r := ScoreSingle(q, strPtr("b"))
	if !r.IsCorrect {
		t.Error("expected IsCorrect=true")
	}
	if r.PointsEarned != 10 {
		t.Errorf("expected 10 points, got %d", r.PointsEarned)
	}
}

func TestScoreSingle_WrongAnswer(t *testing.T) {
	q := makeQuestion("q1", "single", 10)
	q.Answers = []Answer{
		{ID: "a", Text: "A", Correct: false},
		{ID: "b", Text: "B", Correct: true},
	}
	r := ScoreSingle(q, strPtr("a"))
	if r.IsCorrect {
		t.Error("expected IsCorrect=false")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
	if r.CorrectAnswer != "b" {
		t.Errorf("expected correct answer=b, got %v", r.CorrectAnswer)
	}
}

func TestScoreSingle_UnknownAnswer(t *testing.T) {
	q := makeQuestion("q1", "single", 5)
	q.Answers = []Answer{{ID: "a", Text: "A", Correct: true}}
	r := ScoreSingle(q, strPtr("z"))
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for unknown answer")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
}

// ─── ScoreMultiple ─────────────────────────────────────────────────────────

func TestScoreMultiple_EmptyAnswers(t *testing.T) {
	q := makeQuestion("q1", "multiple", 6)
	q.Answers = []Answer{{ID: "a", Correct: true}, {ID: "b", Correct: true}}
	r := ScoreMultiple(q, nil)
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for nil answers")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
}

func TestScoreMultiple_AllCorrect(t *testing.T) {
	q := makeQuestion("q1", "multiple", 6)
	q.Answers = []Answer{
		{ID: "a", Correct: true},
		{ID: "b", Correct: true},
		{ID: "c", Correct: false},
	}
	r := ScoreMultiple(q, []string{"a", "b"})
	if !r.IsCorrect {
		t.Error("expected IsCorrect=true")
	}
	if r.PointsEarned != 6 {
		t.Errorf("expected 6 points, got %d", r.PointsEarned)
	}
}

func TestScoreMultiple_PartialWithFalsePositive(t *testing.T) {
	q := makeQuestion("q1", "multiple", 10)
	q.Answers = []Answer{
		{ID: "a", Correct: true},
		{ID: "b", Correct: true},
		{ID: "c", Correct: false},
	}
	q.PartialScoring = &PartialScoring{Enabled: true}
	// Selected a (correct) + c (wrong) → 1 correct, 1 false positive → score = 10*(1-1)/2 = 0
	r := ScoreMultiple(q, []string{"a", "c"})
	if r.IsCorrect {
		t.Error("expected IsCorrect=false")
	}
}

func TestScoreMultiple_PartialNoNegative(t *testing.T) {
	q := makeQuestion("q1", "multiple", 10)
	q.Answers = []Answer{
		{ID: "a", Correct: true},
		{ID: "b", Correct: true},
		{ID: "c", Correct: false},
	}
	q.PartialScoring = &PartialScoring{Enabled: true, AllowNegative: false}
	// All wrong selected → score cannot be negative
	r := ScoreMultiple(q, []string{"c"})
	if r.PointsEarned < 0 {
		t.Errorf("partial scoring should not go negative, got %d", r.PointsEarned)
	}
}

func TestScoreMultiple_WrongAll_NoPartial(t *testing.T) {
	q := makeQuestion("q1", "multiple", 6)
	q.Answers = []Answer{
		{ID: "a", Correct: true},
		{ID: "b", Correct: false},
	}
	r := ScoreMultiple(q, []string{"b"})
	if r.IsCorrect {
		t.Error("expected IsCorrect=false")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points without partial scoring, got %d", r.PointsEarned)
	}
}

// ─── ScoreBoolean ──────────────────────────────────────────────────────────

func TestScoreBoolean_NilAnswer(t *testing.T) {
	q := makeQuestion("q1", "boolean", 4)
	q.CorrectAnswer = boolPtr(true)
	r := ScoreBoolean(q, nil)
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for nil")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
}

func TestScoreBoolean_Correct(t *testing.T) {
	q := makeQuestion("q1", "boolean", 4)
	q.CorrectAnswer = boolPtr(true)
	r := ScoreBoolean(q, boolPtr(true))
	if !r.IsCorrect {
		t.Error("expected IsCorrect=true")
	}
	if r.PointsEarned != 4 {
		t.Errorf("expected 4 points, got %d", r.PointsEarned)
	}
}

func TestScoreBoolean_Wrong(t *testing.T) {
	q := makeQuestion("q1", "boolean", 4)
	q.CorrectAnswer = boolPtr(true)
	r := ScoreBoolean(q, boolPtr(false))
	if r.IsCorrect {
		t.Error("expected IsCorrect=false")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
}

func TestScoreBoolean_NilCorrectAnswer(t *testing.T) {
	q := makeQuestion("q1", "boolean", 4)
	q.CorrectAnswer = nil
	r := ScoreBoolean(q, boolPtr(true))
	if r.IsCorrect {
		t.Error("expected IsCorrect=false when CorrectAnswer is nil")
	}
}

func TestScoreSingle_NoCorrectAnswerDefined(t *testing.T) {
	q := makeQuestion("q1", "single", 5)
	// No answer has Correct: true → correctAnswerID returns ""
	q.Answers = []Answer{
		{ID: "a", Text: "A", Correct: false},
		{ID: "b", Text: "B", Correct: false},
	}
	r := ScoreSingle(q, strPtr("a"))
	if r.IsCorrect {
		t.Error("expected IsCorrect=false when no correct answer defined")
	}
}

// ─── ScoreOrder ────────────────────────────────────────────────────────────

func TestScoreOrder_EmptyAnswer(t *testing.T) {
	q := makeQuestion("q1", "order", 5)
	q.CorrectOrder = []string{"a", "b", "c"}
	r := ScoreOrder(q, nil)
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for nil order")
	}
}

func TestScoreOrder_Correct(t *testing.T) {
	q := makeQuestion("q1", "order", 5)
	q.CorrectOrder = []string{"a", "b", "c"}
	r := ScoreOrder(q, []string{"a", "b", "c"})
	if !r.IsCorrect {
		t.Error("expected IsCorrect=true")
	}
	if r.PointsEarned != 5 {
		t.Errorf("expected 5 points, got %d", r.PointsEarned)
	}
}

func TestScoreOrder_WrongOrder(t *testing.T) {
	q := makeQuestion("q1", "order", 5)
	q.CorrectOrder = []string{"a", "b", "c"}
	r := ScoreOrder(q, []string{"b", "a", "c"})
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for wrong order")
	}
	if r.PointsEarned != 0 {
		t.Errorf("expected 0 points, got %d", r.PointsEarned)
	}
}

func TestScoreOrder_WrongLength(t *testing.T) {
	q := makeQuestion("q1", "order", 5)
	q.CorrectOrder = []string{"a", "b", "c"}
	r := ScoreOrder(q, []string{"a", "b"})
	if r.IsCorrect {
		t.Error("expected IsCorrect=false for wrong length")
	}
}

// ─── ScoreQuiz ─────────────────────────────────────────────────────────────

func TestScoreQuiz_Mixed(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 60,
		Questions: []Question{
			{
				ID:     "q1",
				Type:   "single",
				Points: 10,
				Answers: []Answer{
					{ID: "a", Correct: true},
					{ID: "b", Correct: false},
				},
				Feedback: Feedback{Correct: "yes", Wrong: "no"},
			},
			{
				ID:            "q2",
				Type:          "boolean",
				Points:        10,
				CorrectAnswer: boolPtr(false),
				Feedback:      Feedback{Correct: "yes", Wrong: "no"},
			},
		},
	}

	answers := map[string]UserAnswer{
		"q1": {Single: strPtr("a")},
		"q2": {Boolean: boolPtr(false)},
	}

	result := ScoreQuiz(quiz, answers)
	if result.TotalScore != 20 {
		t.Errorf("expected total score=20, got %d", result.TotalScore)
	}
	if result.MaxScore != 20 {
		t.Errorf("expected max=20, got %d", result.MaxScore)
	}
	if !result.Passed {
		t.Error("expected Passed=true (100%% >= 60%%)")
	}
}

func TestScoreQuiz_NotPassed(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 80,
		Questions: []Question{
			{
				ID:     "q1",
				Type:   "single",
				Points: 10,
				Answers: []Answer{
					{ID: "a", Correct: true},
				},
				Feedback: Feedback{Wrong: "no"},
			},
		},
	}
	answers := map[string]UserAnswer{
		"q1": {Single: nil}, // wrong
	}
	result := ScoreQuiz(quiz, answers)
	if result.Passed {
		t.Error("expected Passed=false")
	}
}

func TestScoreQuiz_UnknownType(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 50,
		Questions: []Question{
			{
				ID:     "q1",
				Type:   "unknown-type",
				Points: 5,
			},
		},
	}
	answers := map[string]UserAnswer{"q1": {}}
	result := ScoreQuiz(quiz, answers)
	if result.TotalScore != 0 {
		t.Errorf("expected 0 for unknown type, got %d", result.TotalScore)
	}
}

func TestScoreQuiz_EmptyAnswers(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 50,
		Questions:    []Question{},
	}
	result := ScoreQuiz(quiz, map[string]UserAnswer{})
	if result.MaxScore != 0 {
		t.Errorf("expected MaxScore=0, got %d", result.MaxScore)
	}
	if result.Passed {
		t.Error("expected Passed=false when MaxScore=0")
	}
}

func TestScoreQuiz_OrderQuestion(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 50,
		Questions: []Question{
			{
				ID:           "q1",
				Type:         "order",
				Points:       10,
				CorrectOrder: []string{"x", "y", "z"},
				Feedback:     Feedback{Correct: "good", Wrong: "bad"},
			},
		},
	}
	answers := map[string]UserAnswer{
		"q1": {Order: []string{"x", "y", "z"}},
	}
	result := ScoreQuiz(quiz, answers)
	if result.TotalScore != 10 {
		t.Errorf("expected 10, got %d", result.TotalScore)
	}
}

func TestScoreQuiz_MultipleQuestion(t *testing.T) {
	quiz := &Quiz{
		PassingScore: 50,
		Questions: []Question{
			{
				ID:     "q1",
				Type:   "multiple",
				Points: 6,
				Answers: []Answer{
					{ID: "a", Correct: true},
					{ID: "b", Correct: true},
					{ID: "c", Correct: false},
				},
				Feedback: Feedback{Correct: "yes", Wrong: "no"},
			},
		},
	}
	answers := map[string]UserAnswer{
		"q1": {Multiple: []string{"a", "b"}},
	}
	result := ScoreQuiz(quiz, answers)
	if result.TotalScore != 6 {
		t.Errorf("expected 6, got %d", result.TotalScore)
	}
	if !result.Passed {
		t.Error("expected Passed=true")
	}
}
