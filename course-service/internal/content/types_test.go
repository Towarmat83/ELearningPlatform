package content

import (
	"testing"
)

func TestModuleSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Introduction to Go", "introduction-to-go"},
		{"Module 1", "module-1"},
		{"What is Kubernetes?", "what-is-kubernetes"},
		{"  Leading  Spaces  ", "leading--spaces"},
		{"ALL CAPS NAME", "all-caps-name"},
		{"with_underscore", "with_underscore"},
		{"with-hyphen", "with-hyphen"},
	}

	for _, tc := range tests {
		m := Module{Name: tc.name}
		got := m.Slug()
		if got != tc.want {
			t.Errorf("Module{Name:%q}.Slug() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestModuleContent(t *testing.T) {
	tests := []struct {
		typ  string
		src  string
		want string
	}{
		{"video", "/uploads/video.mp4", "/uploads/video.mp4"},
		{"image", "/uploads/img.png", "/uploads/img.png"},
		{"text", "some text", ""},
		{"quiz", "q.yaml", ""},
		{"unknown", "/path/to/thing", ""},
	}

	for _, tc := range tests {
		m := Module{Type: tc.typ, Src: tc.src}
		got := m.Content()
		if got != tc.want {
			t.Errorf("Module{Type:%q}.Content() = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestModuleHasQuestions(t *testing.T) {
	// quiz with questions
	m1 := Module{Type: "quiz", Questions: []Question{{ID: "q1"}}}
	if !m1.HasQuestions() {
		t.Error("expected HasQuestions=true for quiz with questions")
	}

	// quiz without questions
	m2 := Module{Type: "quiz", Questions: nil}
	if m2.HasQuestions() {
		t.Error("expected HasQuestions=false for quiz without questions")
	}

	// non-quiz type
	m3 := Module{Type: "text", Questions: []Question{{ID: "q1"}}}
	if m3.HasQuestions() {
		t.Error("expected HasQuestions=false for non-quiz type")
	}
}

func TestModuleHasGitContent(t *testing.T) {
	m1 := Module{Src: "https://github.com/org/repo", Ref: "main", Path: "content.yaml"}
	if !m1.HasGitContent() {
		t.Error("expected HasGitContent=true when Src+Ref+Path set")
	}

	m2 := Module{Src: "https://github.com/org/repo", Ref: "main"}
	if m2.HasGitContent() {
		t.Error("expected HasGitContent=false when Path empty")
	}

	m3 := Module{Src: "https://github.com/org/repo", Path: "content.yaml"}
	if m3.HasGitContent() {
		t.Error("expected HasGitContent=false when Ref empty")
	}

	m4 := Module{Ref: "main", Path: "content.yaml"}
	if m4.HasGitContent() {
		t.Error("expected HasGitContent=false when Src empty")
	}
}

func TestCourseYAML_MergeSpec_NilSpec(t *testing.T) {
	c := &CourseYAML{Title: "My Course"}
	c.MergeSpec()
	if c.Title != "My Course" {
		t.Errorf("MergeSpec with nil Spec changed title to %q", c.Title)
	}
}

func TestCourseYAML_MergeSpec_FromSpec(t *testing.T) {
	c := &CourseYAML{
		Spec: &CourseSpec{
			Title:       "Spec Title",
			Description: "Spec Desc",
			Public:      true,
			Category:    "k8s",
			Difficulty:  "hard",
			Modules:     []ModuleYAML{{Name: "mod1"}},
			Prerequisites: []CoursePrerequisite{{Course: "prereq1"}},
		},
	}
	c.MergeSpec()
	if c.Title != "Spec Title" {
		t.Errorf("expected Title=Spec Title, got %q", c.Title)
	}
	if c.Description != "Spec Desc" {
		t.Errorf("expected Description=Spec Desc, got %q", c.Description)
	}
	if !c.Public {
		t.Error("expected Public=true")
	}
	if c.Category != "k8s" {
		t.Errorf("expected Category=k8s, got %q", c.Category)
	}
	if c.Difficulty != "hard" {
		t.Errorf("expected Difficulty=hard, got %q", c.Difficulty)
	}
	if len(c.Modules) != 1 {
		t.Errorf("expected 1 module from spec, got %d", len(c.Modules))
	}
	if len(c.Prerequisites) != 1 {
		t.Errorf("expected 1 prerequisite from spec, got %d", len(c.Prerequisites))
	}
}

func TestCourseYAML_MergeSpec_TopLevelTakesPrecedence(t *testing.T) {
	c := &CourseYAML{
		Title:       "Top Title",
		Description: "Top Desc",
		Category:    "docker",
		Difficulty:  "easy",
		Modules:     []ModuleYAML{{Name: "top-mod"}},
		Spec: &CourseSpec{
			Title:       "Spec Title",
			Description: "Spec Desc",
			Category:    "k8s",
			Difficulty:  "hard",
			Modules:     []ModuleYAML{{Name: "spec-mod"}},
		},
	}
	c.MergeSpec()
	if c.Title != "Top Title" {
		t.Errorf("expected top-level Title to take precedence, got %q", c.Title)
	}
	if c.Category != "docker" {
		t.Errorf("expected top-level Category, got %q", c.Category)
	}
	if len(c.Modules) != 1 || c.Modules[0].Name != "top-mod" {
		t.Error("expected top-level modules to take precedence")
	}
}

func TestCourseYAML_MergeSpec_PublicFallbackFromSpec(t *testing.T) {
	c := &CourseYAML{
		Spec: &CourseSpec{Public: true},
	}
	c.MergeSpec()
	if !c.Public {
		t.Error("expected Public=true from spec when top-level is false")
	}
}
