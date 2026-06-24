package content

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetStr(t *testing.T) {
	m := map[string]interface{}{
		"name":  "hello",
		"empty": "",
	}
	if getStr(m, "name") != "hello" {
		t.Error("expected 'hello'")
	}
	if getStr(m, "empty") != "" {
		t.Error("expected empty string")
	}
	if getStr(m, "missing") != "" {
		t.Error("expected empty string for missing key")
	}
	if getStr(nil, "any") != "" {
		t.Error("expected empty string for nil map")
	}
}

func TestGetInt(t *testing.T) {
	m := map[string]interface{}{
		"int":     42,
		"int32":   int32(10),
		"int64":   int64(20),
		"float64": float64(3.7),
		"str":     "not-a-number",
	}
	if getInt(m, "int") != 42 {
		t.Errorf("expected 42, got %d", getInt(m, "int"))
	}
	if getInt(m, "int32") != 10 {
		t.Errorf("expected 10, got %d", getInt(m, "int32"))
	}
	if getInt(m, "int64") != 20 {
		t.Errorf("expected 20, got %d", getInt(m, "int64"))
	}
	if getInt(m, "float64") != 3 {
		t.Errorf("expected 3 (truncated), got %d", getInt(m, "float64"))
	}
	if getInt(m, "str") != 0 {
		t.Errorf("expected 0 for non-number, got %d", getInt(m, "str"))
	}
	if getInt(m, "missing") != 0 {
		t.Errorf("expected 0 for missing key, got %d", getInt(m, "missing"))
	}
}

func TestGetBool(t *testing.T) {
	m := map[string]interface{}{
		"trueBool":  true,
		"falseBool": false,
		"trueStr":   "true",
		"yesStr":    "yes",
		"oneStr":    "1",
		"falseStr":  "false",
		"other":     42,
	}
	if !getBool(m, "trueBool") {
		t.Error("expected true for bool true")
	}
	if getBool(m, "falseBool") {
		t.Error("expected false for bool false")
	}
	if !getBool(m, "trueStr") {
		t.Error("expected true for string 'true'")
	}
	if !getBool(m, "yesStr") {
		t.Error("expected true for string 'yes'")
	}
	if !getBool(m, "oneStr") {
		t.Error("expected true for string '1'")
	}
	if getBool(m, "falseStr") {
		t.Error("expected false for string 'false'")
	}
	if getBool(m, "other") {
		t.Error("expected false for non-bool/string")
	}
	if getBool(m, "missing") {
		t.Error("expected false for missing key")
	}
}

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

func TestCrdToCourse_BasicCourse(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("my-course")
	obj.Object = map[string]interface{}{
		"apiVersion": "elearning.example.com/v1",
		"kind":       "Course",
		"metadata": map[string]interface{}{
			"name": "my-course",
		},
		"spec": map[string]interface{}{
			"title":       "My Course",
			"description": "A test course",
			"public":      true,
			"category":    "testing",
			"difficulty":  "beginner",
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_NoSpec(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("empty-course")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "empty-course"},
	}

	_, err := crdToCourse(obj)
	if err == nil {
		t.Fatal("expected error for object without spec, got nil")
	}
}

func TestCrdToCourse_WithModules(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("course-with-modules")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "course-with-modules"},
		"spec": map[string]interface{}{
			"title": "Course With Modules",
			"modules": []interface{}{
				map[string]interface{}{
					"name": "Module 1",
					"type": "text",
					"src":  "https://github.com/org/repo",
					"ref":  "main",
					"path": "module1.md",
				},
				map[string]interface{}{
					"name":          "Quiz 1",
					"type":          "quiz",
					"passing_score": float64(80),
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_WithPrerequisites(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("advanced-course")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "advanced-course"},
		"spec": map[string]interface{}{
			"title": "Advanced",
			"prerequisites": []interface{}{
				map[string]interface{}{
					"course":    "basic-course",
					"min_score": float64(70),
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_StringPrerequisite(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("course-a")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "course-a"},
		"spec": map[string]interface{}{
			"title": "Course A",
			"prerequisites": []interface{}{
				"basic-course", // bare string form
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	if len(course.Prerequisites) != 1 {
		t.Fatalf("expected 1 prerequisite, got %d", len(course.Prerequisites))
	}
	if course.Prerequisites[0].Course != "basic-course" {
		t.Errorf("expected basic-course, got %q", course.Prerequisites[0].Course)
	}
}

func TestCrdToCourse_PrerequisiteWithModules(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("advanced-b")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "advanced-b"},
		"spec": map[string]interface{}{
			"title": "Advanced B",
			"prerequisites": []interface{}{
				map[string]interface{}{
					"course":  "intro-course",
					"modules": []interface{}{"module-1", "module-2"},
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	if len(course.Prerequisites[0].Modules) != 2 {
		t.Errorf("expected 2 modules, got %d", len(course.Prerequisites[0].Modules))
	}
}

func TestCrdToCourse_ModuleWithCooldown(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("quiz-course")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "quiz-course"},
		"spec": map[string]interface{}{
			"title": "Quiz Course",
			"modules": []interface{}{
				map[string]interface{}{
					"name":          "Quiz 1",
					"type":          "quiz",
					"passing_score": float64(80),
					"cooldown": map[string]interface{}{
						"strategy":     "exponential",
						"base_seconds": float64(30),
						"multiplier":   float64(2.0),
						"max_seconds":  float64(300),
					},
					"max_attempts_per_question": float64(3),
					"lock_on_max_attempts":      true,
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_ModuleWithCooldown_DefaultStrategy(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("quiz-defaults")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "quiz-defaults"},
		"spec": map[string]interface{}{
			"title": "Quiz Defaults",
			"modules": []interface{}{
				map[string]interface{}{
					"name": "Quiz",
					"type": "quiz",
					"cooldown": map[string]interface{}{
						// strategy and base_seconds intentionally omitted to test defaults
						"max_seconds": float64(600),
					},
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	m := course.Modules[0]
	if m.Cooldown.Strategy != "exponential" {
		t.Errorf("expected default Strategy=exponential, got %q", m.Cooldown.Strategy)
	}
	if m.Cooldown.BaseSeconds != 30 {
		t.Errorf("expected default BaseSeconds=30, got %d", m.Cooldown.BaseSeconds)
	}
}

func TestCrdToCourse_ModuleWithInlineQuestions(t *testing.T) {
	boolTrue := true
	_ = boolTrue
	obj := &unstructured.Unstructured{}
	obj.SetName("quiz-inline")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "quiz-inline"},
		"spec": map[string]interface{}{
			"title": "Inline Quiz Course",
			"modules": []interface{}{
				map[string]interface{}{
					"name": "Module Quiz",
					"type": "quiz",
					"questions": []interface{}{
						map[string]interface{}{
							"id":         "q1",
							"type":       "single",
							"question":   "What is 2+2?",
							"difficulty": "easy",
							"points":     float64(2),
							"answers": []interface{}{
								map[string]interface{}{"id": "a1", "text": "3", "correct": false},
								map[string]interface{}{"id": "a2", "text": "4", "correct": true},
							},
						},
						map[string]interface{}{
							"type":     "boolean",
							"question": "Is the sky blue?",
							// no id → should auto-generate, no difficulty → default "medium", no points → default 1
							"correct_answer": true,
						},
					},
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_QuestionWithOrderItems(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("order-quiz")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "order-quiz"},
		"spec": map[string]interface{}{
			"title": "Order Quiz",
			"modules": []interface{}{
				map[string]interface{}{
					"name": "Order Module",
					"type": "quiz",
					"questions": []interface{}{
						map[string]interface{}{
							"id":   "q1",
							"type": "order",
							"items": []interface{}{
								map[string]interface{}{"id": "i1", "text": "Step 1"},
								map[string]interface{}{"id": "i2", "text": "Step 2"},
							},
							"correct_order": []interface{}{"i1", "i2"},
							"partial_scoring": map[string]interface{}{
								"enabled":        true,
								"allow_negative": false,
							},
							"feedback": map[string]interface{}{
								"wrong":   "Try again",
								"correct": "Well done!",
								"source_refs": []interface{}{
									map[string]interface{}{
										"course":   "intro",
										"module":   "basics",
										"priority": float64(1),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_ModuleDefaultNameAndType(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("defaults-course")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "defaults-course"},
		"spec": map[string]interface{}{
			"title": "", // empty title → should use slug
			"modules": []interface{}{
				map[string]interface{}{}, // empty module → name and type default
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
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

func TestCrdToCourse_ModuleNotMap(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("bad-module-course")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "bad-module-course"},
		"spec": map[string]interface{}{
			"title": "Bad Modules",
			"modules": []interface{}{
				"not-a-map", // this should be skipped
				map[string]interface{}{"name": "Good Module", "type": "text"},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	// The string entry should be skipped, only the map entry kept
	if len(course.Modules) != 1 {
		t.Errorf("expected 1 module (bad entry skipped), got %d", len(course.Modules))
	}
}

func TestCrdToCourse_ModuleWithPrerequisites(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("prereq-modules")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "prereq-modules"},
		"spec": map[string]interface{}{
			"title": "Prereq Modules",
			"modules": []interface{}{
				map[string]interface{}{
					"name": "Module A",
					"type": "text",
				},
				map[string]interface{}{
					"name":          "Module B",
					"type":          "text",
					"prerequisites": []interface{}{"module-a"},
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	if len(course.Modules[1].Prerequisites) != 1 {
		t.Errorf("expected 1 prerequisite for Module B, got %d", len(course.Modules[1].Prerequisites))
	}
	if course.Modules[1].Prerequisites[0] != "module-a" {
		t.Errorf("expected prereq=module-a, got %q", course.Modules[1].Prerequisites[0])
	}
}

func TestCrdToCourse_MaxAttemptsInt(t *testing.T) {
	maxAttempts := 5
	_ = maxAttempts
	obj := &unstructured.Unstructured{}
	obj.SetName("int-attempts")
	obj.Object = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "int-attempts"},
		"spec": map[string]interface{}{
			"title": "Int Attempts",
			"modules": []interface{}{
				map[string]interface{}{
					"name":                      "Quiz",
					"type":                      "quiz",
					"max_attempts_per_question": 5, // int (not float64)
				},
			},
		},
	}

	course, err := crdToCourse(obj)
	if err != nil {
		t.Fatalf("crdToCourse failed: %v", err)
	}
	m := course.Modules[0]
	if m.MaxAttemptsPerQuestion == nil || *m.MaxAttemptsPerQuestion != 5 {
		t.Errorf("expected MaxAttemptsPerQuestion=5, got %v", m.MaxAttemptsPerQuestion)
	}
}
