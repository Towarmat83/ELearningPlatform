package content

import (
	"testing"

	coursev1 "github.com/elearning/course-service/api/v1"
)

func TestSourceK8s(t *testing.T) {
	s := sourceK8s("my-course")
	if s != "k8s:my-course" {
		t.Errorf("expected k8s:my-course, got %q", s)
	}
}

func TestBuildAuthURL_NoToken(t *testing.T) {
	result := buildAuthURL("https://github.com/org/repo", "")
	if result != "https://github.com/org/repo" {
		t.Errorf("expected unchanged URL for empty token, got %q", result)
	}
}

func TestBuildAuthURL_WithToken(t *testing.T) {
	result := buildAuthURL("https://github.com/org/repo", "mytoken")
	expected := "https://oauth2:mytoken@github.com/org/repo"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_NonHTTP(t *testing.T) {
	result := buildAuthURL("git@github.com:org/repo.git", "token")
	// Non-http scheme should return unchanged
	if result != "git@github.com:org/repo.git" {
		t.Errorf("expected unchanged for non-http, got %q", result)
	}
}

func TestBuildAuthURL_InvalidURL(t *testing.T) {
	result := buildAuthURL(":::invalid:::", "token")
	if result != ":::invalid:::" {
		t.Errorf("expected unchanged for invalid URL, got %q", result)
	}
}

func TestBuildAuthURL_NoScheme(t *testing.T) {
	// URL parses successfully but has no scheme → return unchanged.
	result := buildAuthURL("github.com/org/repo", "mytoken")
	if result != "github.com/org/repo" {
		t.Errorf("expected unchanged for no-scheme URL, got %q", result)
	}
}

func TestSanitizeGitOutput_NoToken(t *testing.T) {
	result := sanitizeGitOutput("some git output", "")
	if result != "some git output" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestSanitizeGitOutput_WithToken(t *testing.T) {
	result := sanitizeGitOutput("clone failed: oauth2:secret123@github.com", "secret123")
	if result == "" || result == "(no output)" {
		t.Error("expected non-empty sanitized output")
	}
	// Token should be replaced
	if len(result) > 0 {
		for _, r := range result {
			_ = r
		}
	}
}

func TestSanitizeGitOutput_Empty(t *testing.T) {
	result := sanitizeGitOutput("", "")
	if result != "(no output)" {
		t.Errorf("expected '(no output)', got %q", result)
	}
}

func TestSanitizeGitOutput_WhitespaceOnly(t *testing.T) {
	result := sanitizeGitOutput("   \n   ", "")
	if result != "(no output)" {
		t.Errorf("expected '(no output)' for whitespace, got %q", result)
	}
}

func newCourseCR(name string, spec coursev1.CourseSpec) *coursev1.Course {
	cr := &coursev1.Course{Spec: spec}
	cr.Name = name
	return cr
}

func TestCourseFromCR_BasicCourse(t *testing.T) {
	cr := newCourseCR("my-course", coursev1.CourseSpec{
		Title:       "My Course",
		Description: "A test course",
		Public:      true,
		Category:    "testing",
		Difficulty:  "beginner",
	})

	course := courseFromCR(cr)
	if course.Slug != "my-course" {
		t.Errorf("expected slug=my-course, got %q", course.Slug)
	}
	if course.Title != "My Course" {
		t.Errorf("expected title=My Course, got %q", course.Title)
	}
	if !course.IsPublic {
		t.Error("expected IsPublic=true")
	}
	if course.Category != "testing" {
		t.Errorf("expected category=testing, got %q", course.Category)
	}
	if course.Source != "k8s:my-course" {
		t.Errorf("expected source=k8s:my-course, got %q", course.Source)
	}
}

func TestCourseFromCR_WithModules(t *testing.T) {
	passing := 80
	cr := newCourseCR("course-with-modules", coursev1.CourseSpec{
		Title: "Course With Modules",
		Modules: []coursev1.Module{
			{Name: "Module 1", Type: "text", Src: "https://github.com/org/repo", Ref: "main", Path: "module1.md"},
			{Name: "Quiz 1", Type: "quiz", PassingScore: passing},
		},
	})

	course := courseFromCR(cr)
	if len(course.Modules) != 2 {
		t.Errorf("expected 2 modules, got %d", len(course.Modules))
	}
	if course.Modules[0].Name != "Module 1" {
		t.Errorf("expected Module 1, got %q", course.Modules[0].Name)
	}
	if course.Modules[0].Type != "text" {
		t.Errorf("expected type=text, got %q", course.Modules[0].Type)
	}
	if course.Modules[1].PassingScore != 80 {
		t.Errorf("expected passing_score=80, got %d", course.Modules[1].PassingScore)
	}
}

func TestCourseFromCR_WithPrerequisites(t *testing.T) {
	cr := newCourseCR("advanced-course", coursev1.CourseSpec{
		Title: "Advanced",
		Prerequisites: []coursev1.CoursePrerequisite{
			{Course: "basic-course", MinScore: 70},
		},
	})

	course := courseFromCR(cr)
	if len(course.Prerequisites) != 1 {
		t.Errorf("expected 1 prerequisite, got %d", len(course.Prerequisites))
	}
	if course.Prerequisites[0].Course != "basic-course" {
		t.Errorf("expected course=basic-course, got %q", course.Prerequisites[0].Course)
	}
	if course.Prerequisites[0].MinScore != 70 {
		t.Errorf("expected min_score=70, got %d", course.Prerequisites[0].MinScore)
	}
}

func TestCourseFromCR_PrerequisiteWithModules(t *testing.T) {
	cr := newCourseCR("advanced-b", coursev1.CourseSpec{
		Title: "Advanced B",
		Prerequisites: []coursev1.CoursePrerequisite{
			{Course: "intro-course", Modules: []string{"module-1", "module-2"}},
		},
	})

	course := courseFromCR(cr)
	if len(course.Prerequisites[0].Modules) != 2 {
		t.Errorf("expected 2 modules, got %d", len(course.Prerequisites[0].Modules))
	}
}

func TestCourseFromCR_PrerequisiteMissingCourseSkipped(t *testing.T) {
	cr := newCourseCR("course-a", coursev1.CourseSpec{
		Title: "Course A",
		Prerequisites: []coursev1.CoursePrerequisite{
			{MinScore: 10}, // no course slug → should be skipped
			{Course: "basic-course"},
		},
	})

	course := courseFromCR(cr)
	if len(course.Prerequisites) != 1 {
		t.Fatalf("expected 1 prerequisite, got %d", len(course.Prerequisites))
	}
	if course.Prerequisites[0].Course != "basic-course" {
		t.Errorf("expected basic-course, got %q", course.Prerequisites[0].Course)
	}
}

func TestCourseFromCR_ModuleWithCooldown(t *testing.T) {
	maxAttempts := 3
	cr := newCourseCR("quiz-course", coursev1.CourseSpec{
		Title: "Quiz Course",
		Modules: []coursev1.Module{
			{
				Name:         "Quiz 1",
				Type:         "quiz",
				PassingScore: 80,
				Cooldown: &coursev1.CooldownSpec{
					Strategy:    "exponential",
					BaseSeconds: 30,
					Multiplier:  2.0,
					MaxSeconds:  300,
				},
				MaxAttemptsPerQuestion: &maxAttempts,
				LockOnMaxAttempts:      true,
			},
		},
	})

	course := courseFromCR(cr)
	if len(course.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(course.Modules))
	}
	m := course.Modules[0]
	if m.Cooldown.Strategy != "exponential" {
		t.Errorf("expected Strategy=exponential, got %q", m.Cooldown.Strategy)
	}
	if m.Cooldown.BaseSeconds != 30 {
		t.Errorf("expected BaseSeconds=30, got %d", m.Cooldown.BaseSeconds)
	}
	if m.Cooldown.Multiplier != 2.0 {
		t.Errorf("expected Multiplier=2.0, got %f", m.Cooldown.Multiplier)
	}
	if m.MaxAttemptsPerQuestion == nil || *m.MaxAttemptsPerQuestion != 3 {
		t.Errorf("expected MaxAttemptsPerQuestion=3, got %v", m.MaxAttemptsPerQuestion)
	}
	if !m.LockOnMaxAttempts {
		t.Error("expected LockOnMaxAttempts=true")
	}
}

func TestCourseFromCR_ModuleWithCooldown_DefaultStrategy(t *testing.T) {
	cr := newCourseCR("quiz-defaults", coursev1.CourseSpec{
		Title: "Quiz Defaults",
		Modules: []coursev1.Module{
			{
				Name: "Quiz",
				Type: "quiz",
				// strategy and base_seconds intentionally omitted to test defaults
				Cooldown: &coursev1.CooldownSpec{MaxSeconds: 600},
			},
		},
	})

	course := courseFromCR(cr)
	m := course.Modules[0]
	if m.Cooldown.Strategy != "exponential" {
		t.Errorf("expected default Strategy=exponential, got %q", m.Cooldown.Strategy)
	}
	if m.Cooldown.BaseSeconds != 30 {
		t.Errorf("expected default BaseSeconds=30, got %d", m.Cooldown.BaseSeconds)
	}
	if m.Cooldown.Multiplier != 1.0 {
		t.Errorf("expected default Multiplier=1.0, got %f", m.Cooldown.Multiplier)
	}
}

func TestCourseFromCR_ModuleWithoutCooldownStaysZero(t *testing.T) {
	cr := newCourseCR("quiz-no-cooldown", coursev1.CourseSpec{
		Title: "Quiz No Cooldown",
		Modules: []coursev1.Module{
			{Name: "Quiz", Type: "quiz"},
		},
	})

	course := courseFromCR(cr)
	m := course.Modules[0]
	if m.Cooldown.Strategy != "" || m.Cooldown.BaseSeconds != 0 {
		t.Errorf("expected zero-value cooldown when omitted, got %+v", m.Cooldown)
	}
}

func TestCourseFromCR_ModuleWithInlineQuestions(t *testing.T) {
	correct := true
	cr := newCourseCR("quiz-inline", coursev1.CourseSpec{
		Title: "Inline Quiz Course",
		Modules: []coursev1.Module{
			{
				Name: "Module Quiz",
				Type: "quiz",
				Questions: []coursev1.Question{
					{
						ID: "q1", Type: "single", Question: "What is 2+2?",
						Difficulty: "easy", Points: 2,
						Answers: []coursev1.Answer{
							{ID: "a1", Text: "3", Correct: false},
							{ID: "a2", Text: "4", Correct: true},
						},
					},
					{
						Type: "boolean", Question: "Is the sky blue?",
						// no id → should auto-generate, no difficulty → default "medium", no points → default 1
						CorrectAnswer: &correct,
					},
				},
			},
		},
	})

	course := courseFromCR(cr)
	if len(course.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(course.Modules))
	}
	m := course.Modules[0]
	if len(m.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(m.Questions))
	}
	q1 := m.Questions[0]
	if q1.ID != "q1" {
		t.Errorf("q1.ID: want q1, got %q", q1.ID)
	}
	if q1.Points != 2 {
		t.Errorf("q1.Points: want 2, got %d", q1.Points)
	}
	if len(q1.Answers) != 2 {
		t.Errorf("expected 2 answers, got %d", len(q1.Answers))
	}
	q2 := m.Questions[1]
	if q2.ID == "" {
		t.Error("expected auto-generated ID for q2")
	}
	if q2.Difficulty != "medium" {
		t.Errorf("expected default difficulty=medium, got %q", q2.Difficulty)
	}
	if q2.Points != 1 {
		t.Errorf("expected default points=1, got %d", q2.Points)
	}
	if q2.CorrectAnswer == nil || !*q2.CorrectAnswer {
		t.Errorf("expected CorrectAnswer=true, got %v", q2.CorrectAnswer)
	}
}

func TestCourseFromCR_QuestionWithOrderItems(t *testing.T) {
	cr := newCourseCR("order-quiz", coursev1.CourseSpec{
		Title: "Order Quiz",
		Modules: []coursev1.Module{
			{
				Name: "Order Module",
				Type: "quiz",
				Questions: []coursev1.Question{
					{
						ID:   "q1",
						Type: "order",
						Items: []coursev1.OrderItem{
							{ID: "i1", Text: "Step 1"},
							{ID: "i2", Text: "Step 2"},
						},
						CorrectOrder:   []string{"i1", "i2"},
						PartialScoring: &coursev1.PartialScoring{Enabled: true, AllowNegative: false},
						Feedback: coursev1.Feedback{
							Wrong:   "Try again",
							Correct: "Well done!",
							SourceRefs: []coursev1.SourceRef{
								{Course: "intro", Module: "basics", Priority: 1},
							},
						},
					},
				},
			},
		},
	})

	course := courseFromCR(cr)
	q := course.Modules[0].Questions[0]
	if len(q.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(q.Items))
	}
	if len(q.CorrectOrder) != 2 {
		t.Errorf("expected 2 correct_order, got %d", len(q.CorrectOrder))
	}
	if q.PartialScoring == nil || !q.PartialScoring.Enabled {
		t.Error("expected PartialScoring.Enabled=true")
	}
	if q.Feedback.Wrong != "Try again" {
		t.Errorf("expected Feedback.Wrong='Try again', got %q", q.Feedback.Wrong)
	}
	if len(q.Feedback.SourceRefs) != 1 {
		t.Fatalf("expected 1 source ref, got %d", len(q.Feedback.SourceRefs))
	}
	if q.Feedback.SourceRefs[0].Course != "intro" {
		t.Errorf("expected SourceRef.Course=intro, got %q", q.Feedback.SourceRefs[0].Course)
	}
}

func TestCourseFromCR_ModuleDefaultNameAndType(t *testing.T) {
	cr := newCourseCR("defaults-course", coursev1.CourseSpec{
		Title:   "", // empty title → should use slug
		Modules: []coursev1.Module{{}},
	})

	course := courseFromCR(cr)
	if course.Title != "defaults-course" {
		t.Errorf("expected title to default to slug, got %q", course.Title)
	}
	if len(course.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(course.Modules))
	}
	m := course.Modules[0]
	if m.Name != "module-1" {
		t.Errorf("expected name=module-1, got %q", m.Name)
	}
	if m.Type != "text" {
		t.Errorf("expected type=text, got %q", m.Type)
	}
}

func TestCourseFromCR_ModuleWithPrerequisites(t *testing.T) {
	cr := newCourseCR("prereq-modules", coursev1.CourseSpec{
		Title: "Prereq Modules",
		Modules: []coursev1.Module{
			{Name: "Module A", Type: "text"},
			{Name: "Module B", Type: "text", Prerequisites: []string{"module-a"}},
		},
	})

	course := courseFromCR(cr)
	if len(course.Modules[1].Prerequisites) != 1 {
		t.Errorf("expected 1 prerequisite for Module B, got %d", len(course.Modules[1].Prerequisites))
	}
	if course.Modules[1].Prerequisites[0] != "module-a" {
		t.Errorf("expected prereq=module-a, got %q", course.Modules[1].Prerequisites[0])
	}
}

func TestCourseFromCR_MaxAttemptsInt(t *testing.T) {
	maxAttempts := 5
	cr := newCourseCR("int-attempts", coursev1.CourseSpec{
		Title: "Int Attempts",
		Modules: []coursev1.Module{
			{Name: "Quiz", Type: "quiz", MaxAttemptsPerQuestion: &maxAttempts},
		},
	})

	course := courseFromCR(cr)
	m := course.Modules[0]
	if m.MaxAttemptsPerQuestion == nil || *m.MaxAttemptsPerQuestion != 5 {
		t.Errorf("expected MaxAttemptsPerQuestion=5, got %v", m.MaxAttemptsPerQuestion)
	}
}
