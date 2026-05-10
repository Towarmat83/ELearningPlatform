package content

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ── Fetch Quiz from Git ─────────────────────────────────────────────────────────────

// FetchQuizContent clones a git repo at src/ref, reads the YAML file at path,
// and returns the parsed in-memory Quiz.
func FetchQuizContent(src, ref, path, token string) (*Quiz, error) {
	data, err := FetchModuleContent(src, ref, path, token)
	if err != nil {
		return nil, fmt.Errorf("fetch quiz content: %w", err)
	}

	var qy QuizYAML
	if err := yaml.Unmarshal(data, &qy); err != nil {
		return nil, fmt.Errorf("parse quiz yaml: %w", err)
	}

	if qy.ID == "" {
		qy.ID = path
	}
	if qy.Title == "" {
		qy.Title = qy.ID
	}

	return qy.ToQuiz(), nil
}

// ── Quiz YAML (deserialization) ──────────────────────────────────────────────────

type QuizYAML struct {
	ID                     string           `yaml:"id"`
	Title                  string           `yaml:"title"`
	Description            string           `yaml:"description"`
	Version                string           `yaml:"version"`
	Covers                 []CoverEntryYAML `yaml:"covers"`
	PassingScore           int              `yaml:"passing_score"`
	Cooldown               CooldownSpecYAML `yaml:"cooldown"`
	MaxAttemptsPerQuestion *int             `yaml:"max_attempts_per_question"`
	LockOnMaxAttempts      bool             `yaml:"lock_on_max_attempts"`
	Questions              []QuestionYAML   `yaml:"questions"`
}

type CoverEntryYAML struct {
	Course  string   `yaml:"course"`
	Modules []string `yaml:"modules"`
}

type CooldownSpecYAML struct {
	Strategy    string  `yaml:"strategy"`
	BaseSeconds int     `yaml:"base_seconds"`
	Multiplier  float64 `yaml:"multiplier"`
	MaxSeconds  int     `yaml:"max_seconds"`
}

type QuestionYAML struct {
	ID             string              `yaml:"id"`
	Type           string              `yaml:"type"`
	Difficulty     string              `yaml:"difficulty"`
	Points         int                 `yaml:"points"`
	Question       string              `yaml:"question"`
	Answers        []AnswerYAML        `yaml:"answers"`
	CorrectAnswer  *bool               `yaml:"correct_answer"`
	Items          []OrderItemYAML     `yaml:"items"`
	CorrectOrder   []string            `yaml:"correct_order"`
	PartialScoring *PartialScoringYAML `yaml:"partial_scoring"`
	Feedback       FeedbackYAML        `yaml:"feedback"`
}

type AnswerYAML struct {
	ID      string `yaml:"id"`
	Text    string `yaml:"text"`
	Correct bool   `yaml:"correct"`
}

type OrderItemYAML struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

type PartialScoringYAML struct {
	Enabled       bool `yaml:"enabled"`
	AllowNegative bool `yaml:"allow_negative"`
}

type FeedbackYAML struct {
	Wrong      string          `yaml:"wrong"`
	Correct    string          `yaml:"correct"`
	SourceRefs []SourceRefYAML `yaml:"source_refs"`
}

type SourceRefYAML struct {
	Course   string `yaml:"course"`
	Module   string `yaml:"module"`
	Anchor   string `yaml:"anchor"`
	Priority int    `yaml:"priority"`
}

// ── In-Memory Quiz ───────────────────────────────────────────────────────────────

type Quiz struct {
	ID                     string       `json:"id"`
	Title                  string       `json:"title"`
	Description            string       `json:"description"`
	Version                string       `json:"version"`
	Covers                 []CoverEntry `json:"covers"`
	PassingScore           int          `json:"passing_score"`
	Cooldown               CooldownSpec `json:"cooldown"`
	MaxAttemptsPerQuestion *int         `json:"max_attempts_per_question,omitempty"`
	LockOnMaxAttempts      bool         `json:"lock_on_max_attempts"`
	Questions              []Question   `json:"questions"`
}

type CoverEntry struct {
	Course  string   `json:"course"`
	Modules []string `json:"modules"`
}

type CooldownSpec struct {
	Strategy    string  `json:"strategy"`
	BaseSeconds int     `json:"base_seconds"`
	Multiplier  float64 `json:"multiplier"`
	MaxSeconds  int     `json:"max_seconds"`
}

type Question struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Difficulty     string          `json:"difficulty,omitempty"`
	Points         int             `json:"points"`
	Question       string          `json:"question"`
	Answers        []Answer        `json:"answers,omitempty"`
	CorrectAnswer  *bool           `json:"correct_answer,omitempty"`
	Items          []OrderItem     `json:"items,omitempty"`
	CorrectOrder   []string        `json:"correct_order,omitempty"`
	PartialScoring *PartialScoring `json:"partial_scoring,omitempty"`
	Feedback       Feedback        `json:"feedback,omitempty"`
}

type Answer struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

type OrderItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type PartialScoring struct {
	Enabled       bool `json:"enabled"`
	AllowNegative bool `json:"allow_negative"`
}

type Feedback struct {
	Wrong      string      `json:"wrong,omitempty"`
	Correct    string      `json:"correct,omitempty"`
	SourceRefs []SourceRef `json:"source_refs,omitempty"`
}

type SourceRef struct {
	Course   string `json:"course"`
	Module   string `json:"module"`
	Anchor   string `json:"anchor"`
	Priority int    `json:"priority"`
}

// ── Quiz → In-Memory ─────────────────────────────────────────────────────────────

func (qy QuizYAML) ToQuiz() *Quiz {
	q := &Quiz{
		ID:                     qy.ID,
		Title:                  qy.Title,
		Description:            qy.Description,
		Version:                qy.Version,
		PassingScore:           qy.PassingScore,
		Cooldown:               CooldownSpec(qy.Cooldown),
		MaxAttemptsPerQuestion: qy.MaxAttemptsPerQuestion,
		LockOnMaxAttempts:      qy.LockOnMaxAttempts,
	}
	for _, c := range qy.Covers {
		q.Covers = append(q.Covers, CoverEntry{
			Course:  c.Course,
			Modules: c.Modules,
		})
	}
	for _, qa := range qy.Questions {
		qq := Question{
			ID:            qa.ID,
			Type:          qa.Type,
			Difficulty:    qa.Difficulty,
			Points:        qa.Points,
			Question:      qa.Question,
			CorrectAnswer: qa.CorrectAnswer,
			CorrectOrder:  qa.CorrectOrder,
			Answers:       make([]Answer, len(qa.Answers)),
			Items:         make([]OrderItem, len(qa.Items)),
		}
		for i, a := range qa.Answers {
			qq.Answers[i] = Answer{ID: a.ID, Text: a.Text, Correct: a.Correct}
		}
		for i, it := range qa.Items {
			qq.Items[i] = OrderItem{ID: it.ID, Text: it.Text}
		}
		if qa.PartialScoring != nil {
			qq.PartialScoring = &PartialScoring{
				Enabled:       qa.PartialScoring.Enabled,
				AllowNegative: qa.PartialScoring.AllowNegative,
			}
		}
		qq.Feedback = Feedback{
			Wrong:   qa.Feedback.Wrong,
			Correct: qa.Feedback.Correct,
		}
		for _, sr := range qa.Feedback.SourceRefs {
			qq.Feedback.SourceRefs = append(qq.Feedback.SourceRefs, SourceRef{
				Course:   sr.Course,
				Module:   sr.Module,
				Anchor:   sr.Anchor,
				Priority: sr.Priority,
			})
		}
		if qq.Points == 0 {
			qq.Points = 1
		}
		if qq.Difficulty == "" {
			qq.Difficulty = "medium"
		}
		q.Questions = append(q.Questions, qq)
	}
	return q
}
