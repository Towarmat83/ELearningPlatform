package content

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStore_PutAndGet checks store put and get.
func TestStore_PutAndGet(t *testing.T) {
	t.Parallel()

	s := NewStore()
	c := &Course{Slug: "test-course", Title: "Test", IsPublic: true}
	s.Put(c)

	got := s.Get("test-course")
	if got == nil {
		t.Fatal("expected to get course back")
	}

	if got.Title != "Test" {
		t.Errorf("expected Title=Test, got %q", got.Title)
	}
}

// TestStore_Get_Missing checks store get missing.
func TestStore_Get_Missing(t *testing.T) {
	t.Parallel()

	s := NewStore()

	got := s.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for missing slug")
	}
}

// TestStore_List_OnlyPublic checks store list only public.
func TestStore_List_OnlyPublic(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.Put(&Course{Slug: "public1", Title: "A Public", IsPublic: true})
	s.Put(&Course{Slug: "private1", Title: "Private", IsPublic: false})
	s.Put(&Course{Slug: "public2", Title: "B Public", IsPublic: true})

	list := s.List()
	if len(list) != 2 {
		t.Errorf("expected 2 public courses, got %d", len(list))
	}
	// Should be sorted by Title
	if list[0].Title != "A Public" {
		t.Errorf("expected A Public first, got %q", list[0].Title)
	}
}

// TestStore_All checks store all.
func TestStore_All(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.Put(&Course{Slug: "c1", Title: "B", IsPublic: true})
	s.Put(&Course{Slug: "c2", Title: "A", IsPublic: false})

	all := s.All()
	if len(all) != 2 {
		t.Errorf("expected 2 courses, got %d", len(all))
	}
	// Sorted by Title
	if all[0].Title != "A" {
		t.Errorf("expected A first, got %q", all[0].Title)
	}
}

// TestStore_DeleteBySource checks store delete by source.
func TestStore_DeleteBySource(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.Put(&Course{Slug: "c1", Title: "C1", Source: "k8s:c1"})
	s.Put(&Course{Slug: "c2", Title: "C2", Source: "k8s:c2"})
	s.Put(&Course{Slug: "c3", Title: "C3", Source: "git:c3"})

	s.DeleteBySource("k8s:c1")

	if s.Get("c1") != nil {
		t.Error("c1 should be deleted")
	}

	if s.Get("c2") == nil {
		t.Error("c2 should still exist")
	}

	if s.Get("c3") == nil {
		t.Error("c3 should still exist (different source)")
	}
}

// TestStore_PutOverwrite checks store put overwrite.
func TestStore_PutOverwrite(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.Put(&Course{Slug: "c1", Title: "Original"})
	s.Put(&Course{Slug: "c1", Title: "Updated"})

	got := s.Get("c1")
	if got.Title != "Updated" {
		t.Errorf("expected Updated, got %q", got.Title)
	}
}

// TestParseMarkdownLesson_WithFrontmatter checks frontmatter parsing.
func TestParseMarkdownLesson_WithFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "01-intro.md")
	content := `---
title: Introduction to Kubernetes
---
# Hello

Some content here.
`
	os.WriteFile(file, []byte(content), 0o644)

	lesson, err := ParseMarkdownLesson(file, 1)
	if err != nil {
		t.Fatalf("ParseMarkdownLesson failed: %v", err)
	}

	if lesson.Title != "Introduction to Kubernetes" {
		t.Errorf("expected title from frontmatter, got %q", lesson.Title)
	}

	if lesson.Slug != "intro" {
		t.Errorf("expected slug=intro (stripped 01-), got %q", lesson.Slug)
	}

	if lesson.Order != 1 {
		t.Errorf("expected order=1, got %d", lesson.Order)
	}

	if lesson.Content == "" {
		t.Error("expected non-empty content")
	}
}

// TestParseMarkdownLesson_NoFrontmatter checks parsing without frontmatter.
func TestParseMarkdownLesson_NoFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "getting-started.md")
	content := `# Getting Started

Just content, no frontmatter.`
	os.WriteFile(file, []byte(content), 0o644)

	lesson, err := ParseMarkdownLesson(file, 2)
	if err != nil {
		t.Fatalf("ParseMarkdownLesson failed: %v", err)
	}
	// Title defaults to slug when no frontmatter
	if lesson.Slug != "getting-started" {
		t.Errorf("expected slug=getting-started, got %q", lesson.Slug)
	}

	if lesson.Title != "getting-started" {
		t.Errorf("expected title=getting-started, got %q", lesson.Title)
	}

	if lesson.Order != 2 {
		t.Errorf("expected order=2, got %d", lesson.Order)
	}
}

// TestParseMarkdownLesson_NonExistentFile checks missing file handling.
func TestParseMarkdownLesson_NonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := ParseMarkdownLesson("/nonexistent/file.md", 0)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestParseMarkdownLesson_NoOrderPrefix checks lesson without order prefix.
func TestParseMarkdownLesson_NoOrderPrefix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "my-lesson.md")
	os.WriteFile(file, []byte("hello world"), 0o644)

	lesson, err := ParseMarkdownLesson(file, 0)
	if err != nil {
		t.Fatal(err)
	}

	if lesson.Slug != "my-lesson" {
		t.Errorf("expected slug=my-lesson, got %q", lesson.Slug)
	}
}

// TestExtractFrontmatter_NoFrontmatter checks extraction with no frontmatter.
func TestExtractFrontmatter_NoFrontmatter(t *testing.T) {
	t.Parallel()

	data := []byte("# Hello\nContent here")

	title, body := extractFrontmatter(data)
	if title != "" {
		t.Errorf("expected no title, got %q", title)
	}

	if body != "# Hello\nContent here" {
		t.Errorf("unexpected body: %q", body)
	}
}

// TestExtractFrontmatter_WithTitle checks extract frontmatter with title.
func TestExtractFrontmatter_WithTitle(t *testing.T) {
	t.Parallel()

	data := []byte("---\ntitle: My Title\n---\nBody content")

	title, body := extractFrontmatter(data)
	if title != "My Title" {
		t.Errorf("expected My Title, got %q", title)
	}

	if body != "Body content" {
		t.Errorf("expected Body content, got %q", body)
	}
}

// TestExtractFrontmatter_MissingClosingDelimiter checks missing delimiter.
func TestExtractFrontmatter_MissingClosingDelimiter(t *testing.T) {
	t.Parallel()

	data := []byte("---\ntitle: Incomplete")

	title, body := extractFrontmatter(data)
	if title != "" {
		t.Errorf("expected no title for incomplete frontmatter, got %q", title)
	}
	// Should return original content
	_ = body
}

// TestExtractFrontmatter_EmptyFrontmatter checks empty frontmatter block.
func TestExtractFrontmatter_EmptyFrontmatter(t *testing.T) {
	t.Parallel()

	data := []byte("---\n---\nBody here")

	_, body := extractFrontmatter(data)
	if body != "Body here" {
		t.Errorf("expected Body here, got %q", body)
	}
}

// TestExtractFrontmatter_InvalidYAML checks extract frontmatter invalid YAML.
func TestExtractFrontmatter_InvalidYAML(t *testing.T) {
	t.Parallel()

	// Tab-indented YAML is invalid and triggers yaml.Unmarshal error.
	data := []byte("---\n\t invalid: yaml:\n---\nBody content")
	title, body := extractFrontmatter(data)
	// On YAML error, title should be empty and body returned.
	if title != "" {
		t.Errorf("expected empty title on YAML error, got %q", title)
	}

	if body == "" {
		t.Error("expected non-empty body even on YAML error")
	}
}
