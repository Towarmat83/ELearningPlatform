package content

import "math"

// ── Answer Submission ─────────────────────────────────────────────────────────────

// UserAnswer represents a user's answer to a single question.
type UserAnswer struct {
	Single   *string  `json:"single,omitempty"`   // type single: answer ID
	Multiple []string `json:"multiple,omitempty"` // type multiple: answer IDs
	Boolean  *bool    `json:"boolean,omitempty"`  // type boolean
	Order    []string `json:"order,omitempty"`    // type order: item IDs in user order
}

// QuestionResult holds the scoring outcome for one question.
type QuestionResult struct {
	QuestionID    string      `json:"question_id"`
	Type          string      `json:"type"`
	IsCorrect     bool        `json:"is_correct"`
	PointsEarned  int         `json:"points_earned"`
	PointsMax     int         `json:"points_max"`
	CorrectAnswer interface{} `json:"correct_answer,omitempty"`
	Feedback      string      `json:"feedback,omitempty"`
	SourceRefs    []SourceRef `json:"source_refs,omitempty"`
}

// QuizResult holds the overall scoring outcome.
type QuizResult struct {
	TotalScore      int              `json:"total_score"`
	MaxScore        int              `json:"max_score"`
	Passed          bool             `json:"passed"`
	QuestionResults []QuestionResult `json:"question_results"`
}

// SubmitRequest represents a user's answers to a quiz submission.
type SubmitRequest struct {
	Answers map[string]UserAnswer `json:"answers"`
}

// ── Scoring Functions ─────────────────────────────────────────────────────────────

func ScoreSingle(q Question, answer *string) QuestionResult {
	r := QuestionResult{
		QuestionID: q.ID,
		Type:       q.Type,
		PointsMax:  q.Points,
	}
	if answer == nil {
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
		return r
	}
	for _, a := range q.Answers {
		if a.ID == *answer {
			if a.Correct {
				r.IsCorrect = true
				r.PointsEarned = q.Points
				r.CorrectAnswer = a.ID
				r.Feedback = q.Feedback.Correct
			} else {
				r.Feedback = q.Feedback.Wrong
				r.CorrectAnswer = correctAnswerID(q.Answers)
				r.SourceRefs = q.Feedback.SourceRefs
			}
			return r
		}
	}
	r.Feedback = q.Feedback.Wrong
	r.CorrectAnswer = correctAnswerID(q.Answers)
	r.SourceRefs = q.Feedback.SourceRefs
	return r
}

func ScoreMultiple(q Question, answers []string) QuestionResult {
	r := QuestionResult{
		QuestionID: q.ID,
		Type:       q.Type,
		PointsMax:  q.Points,
	}
	if len(answers) == 0 {
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
		return r
	}
	correctIDs := make(map[string]bool)
	for _, a := range q.Answers {
		if a.Correct {
			correctIDs[a.ID] = true
		}
	}
	selected := make(map[string]bool)
	for _, id := range answers {
		selected[id] = true
	}

	correctSelected := 0
	falsePositives := 0
	for id := range selected {
		if correctIDs[id] {
			correctSelected++
		} else {
			falsePositives++
		}
	}
	allCorrect := correctSelected == len(correctIDs) && falsePositives == 0

	if allCorrect {
		r.IsCorrect = true
		r.PointsEarned = q.Points
		r.Feedback = q.Feedback.Correct
		r.CorrectAnswer = correctIDs
		return r
	}

	// Partial scoring
	if q.PartialScoring != nil && q.PartialScoring.Enabled {
		raw := float64(q.Points) * float64(correctSelected-falsePositives) / float64(len(correctIDs))
		if !q.PartialScoring.AllowNegative && raw < 0 {
			raw = 0
		}
		r.PointsEarned = int(math.Round(raw))
	} else {
		r.PointsEarned = 0
	}
	r.Feedback = q.Feedback.Wrong
	r.CorrectAnswer = correctIDs
	r.SourceRefs = q.Feedback.SourceRefs
	return r
}

func ScoreBoolean(q Question, answer *bool) QuestionResult {
	r := QuestionResult{
		QuestionID: q.ID,
		Type:       q.Type,
		PointsMax:  q.Points,
	}
	if answer == nil {
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
		return r
	}
	if q.CorrectAnswer != nil && *answer == *q.CorrectAnswer {
		r.IsCorrect = true
		r.PointsEarned = q.Points
		r.CorrectAnswer = *q.CorrectAnswer
		r.Feedback = q.Feedback.Correct
	} else {
		r.CorrectAnswer = q.CorrectAnswer
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
	}
	return r
}

func ScoreOrder(q Question, order []string) QuestionResult {
	r := QuestionResult{
		QuestionID: q.ID,
		Type:       q.Type,
		PointsMax:  q.Points,
	}
	if len(order) == 0 {
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
		return r
	}
	correct := true
	for i, id := range order {
		if i >= len(q.CorrectOrder) || id != q.CorrectOrder[i] {
			correct = false
			break
		}
	}
	if correct && len(order) == len(q.CorrectOrder) {
		r.IsCorrect = true
		r.PointsEarned = q.Points
		r.CorrectAnswer = q.CorrectOrder
		r.Feedback = q.Feedback.Correct
	} else {
		r.CorrectAnswer = q.CorrectOrder
		r.Feedback = q.Feedback.Wrong
		r.SourceRefs = q.Feedback.SourceRefs
	}
	return r
}

// ── Helpers ───────────────────────────────────────────────────────────────────────

func correctAnswerID(answers []Answer) string {
	for _, a := range answers {
		if a.Correct {
			return a.ID
		}
	}
	return ""
}

// ScoreQuiz evaluates all answers and returns the full result.
func ScoreQuiz(q *Quiz, answers map[string]UserAnswer) QuizResult {
	r := QuizResult{}
	for _, qq := range q.Questions {
		ua := answers[qq.ID]
		var qr QuestionResult
		switch qq.Type {
		case "single":
			qr = ScoreSingle(qq, ua.Single)
		case "multiple":
			qr = ScoreMultiple(qq, ua.Multiple)
		case "boolean":
			qr = ScoreBoolean(qq, ua.Boolean)
		case "order":
			qr = ScoreOrder(qq, ua.Order)
		default:
			qr = QuestionResult{QuestionID: qq.ID, Type: qq.Type, PointsMax: qq.Points}
		}
		r.TotalScore += qr.PointsEarned
		r.MaxScore += qr.PointsMax
		r.QuestionResults = append(r.QuestionResults, qr)
	}
	if r.MaxScore > 0 {
		pct := float64(r.TotalScore) / float64(r.MaxScore) * 100
		r.Passed = pct >= float64(q.PassingScore)
	}
	return r
}
