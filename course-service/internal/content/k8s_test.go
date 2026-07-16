package content

import (
	"testing"
)

// TestStore_Operations checks store operations.
func TestStore_Operations(t *testing.T) {
	t.Parallel()

	store := NewStore()

	store.Put(&Course{
		Slug:        "test-course",
		Title:       "Test Course",
		Description: "A test course",
		Category:    "testing",
		Difficulty:  "beginner",
		IsPublic:    true,
		Source:      "local",
		Modules: []Module{
			{Name: "Module 1", Type: "text", Path: "intro.md"},
		},
	})

	c := store.Get("test-course")
	if c == nil {
		t.Fatal("expected course to be stored")
	}

	if c.Title != "Test Course" {
		t.Errorf("expected title 'Test Course', got '%s'", c.Title)
	}

	if len(c.Modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(c.Modules))
	}

	all := store.All()
	if len(all) != 1 {
		t.Errorf("expected 1 course in store, got %d", len(all))
	}

	store.DeleteBySource("local")

	c = store.Get("test-course")
	if c != nil {
		t.Error("expected course to be deleted")
	}
}
