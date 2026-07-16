package content

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
)

// ── PathStore ────────────────────────────────────────────────────────────────

// TestPathStore_PutAndGet checks path store put and get.
func TestPathStore_PutAndGet(t *testing.T) {
	t.Parallel()

	s := NewPathStore()
	p := &Path{Slug: "devops-path", Title: "DevOps Path", Courses: []string{"linux-intro", "docker-fundamentals"}}
	s.Put(p)

	got := s.Get("devops-path")
	if got == nil {
		t.Fatal("expected to get path back")
	}

	if got.Title != "DevOps Path" {
		t.Errorf("expected Title=DevOps Path, got %q", got.Title)
	}
}

// TestPathStore_List checks path store list.
func TestPathStore_List(t *testing.T) {
	t.Parallel()

	s := NewPathStore()
	s.Put(&Path{Slug: "path-b", Title: "B Path"})
	s.Put(&Path{Slug: "path-a", Title: "A Path"})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(list))
	}
	// Order is undefined — sorting is the caller's responsibility.
	slugs := map[string]bool{list[0].Slug: true, list[1].Slug: true}
	if !slugs["path-a"] || !slugs["path-b"] {
		t.Errorf("expected both paths in list, got %v", list)
	}
}

// TestPathStore_DeleteBySource checks path store delete by source.
func TestPathStore_DeleteBySource(t *testing.T) {
	t.Parallel()

	s := NewPathStore()
	s.Put(&Path{Slug: "to-delete", Source: "k8s:to-delete"})
	s.Put(&Path{Slug: "to-keep", Source: "k8s:to-keep"})

	s.DeleteBySource("k8s:to-delete")

	if s.Get("to-delete") != nil {
		t.Error("expected path to be deleted")
	}

	if s.Get("to-keep") == nil {
		t.Error("expected path to be kept")
	}
}

// TestPathStore_GetMissing checks path store get missing.
func TestPathStore_GetMissing(t *testing.T) {
	t.Parallel()

	s := NewPathStore()
	if s.Get("nonexistent") != nil {
		t.Error("expected nil for missing path")
	}
}

// ── pathFromCR ───────────────────────────────────────────────────────────────

// makePathCR builds a Path CRD instance with the given name and spec.
func makePathCR(name string, spec coursev1.PathSpec) *coursev1.Path {
	cr := &coursev1.Path{Spec: spec}
	cr.Name = name
	cr.TypeMeta = metav1.TypeMeta{APIVersion: "elearning.pupitre.io/v1", Kind: "Path"}

	return cr
}

// TestPathFromCR_Basic checks path from CR basic.
func TestPathFromCR_Basic(t *testing.T) {
	t.Parallel()

	cr := makePathCR("devops-path", coursev1.PathSpec{
		Title:       "DevOps Path",
		Description: "From Linux to Kubernetes",
		Courses:     []string{"linux-intro", "docker-fundamentals", "kubernetes-basics"},
	})

	p := pathFromCR(cr)
	if p.Slug != "devops-path" {
		t.Errorf("slug: want devops-path, got %q", p.Slug)
	}

	if p.Title != "DevOps Path" {
		t.Errorf("title: want DevOps Path, got %q", p.Title)
	}

	if p.Description != "From Linux to Kubernetes" {
		t.Errorf("description: want From Linux to Kubernetes, got %q", p.Description)
	}

	if len(p.Courses) != 3 {
		t.Fatalf("expected 3 courses, got %d", len(p.Courses))
	}

	if p.Courses[0] != "linux-intro" {
		t.Errorf("courses[0]: want linux-intro, got %q", p.Courses[0])
	}
}

// TestPathFromCR_FallbackTitle checks path from CR fallback title.
func TestPathFromCR_FallbackTitle(t *testing.T) {
	t.Parallel()

	cr := makePathCR("my-path", coursev1.PathSpec{})

	p := pathFromCR(cr)
	if p.Title != "my-path" {
		t.Errorf("expected title to fallback to slug, got %q", p.Title)
	}
}

// TestPathFromCR_Source checks path from CR source.
func TestPathFromCR_Source(t *testing.T) {
	t.Parallel()

	cr := makePathCR("devops-path", coursev1.PathSpec{Title: "DevOps"})

	p := pathFromCR(cr)
	if p.Source != "k8s:devops-path" {
		t.Errorf("source: want k8s:devops-path, got %q", p.Source)
	}
}

// TestPathFromCR_EmptyCourseSlugSkipped checks empty course slug is skipped.
func TestPathFromCR_EmptyCourseSlugSkipped(t *testing.T) {
	t.Parallel()

	cr := makePathCR("test-path", coursev1.PathSpec{
		Courses: []string{"valid-course", "", "another-course"},
	})

	p := pathFromCR(cr)
	if len(p.Courses) != 2 {
		t.Errorf("expected 2 courses (empty slug skipped), got %d", len(p.Courses))
	}
}

// ── PathWatcher event handlers ────────────────────────────────────────────────

// TestPathWatcher_Upsert_HappyPath checks path watcher upsert happy path.
func TestPathWatcher_Upsert_HappyPath(t *testing.T) {
	t.Parallel()

	store := NewPathStore()
	w := &PathWatcher{store: store}

	cr := makePathCR("devops-path", coursev1.PathSpec{Title: "DevOps"})
	w.upsert(cr)

	if store.Get("devops-path") == nil {
		t.Error("expected path to be stored after upsert")
	}
}

// TestPathWatcher_Upsert_TypeAssertFailure checks upsert type assert failure.
func TestPathWatcher_Upsert_TypeAssertFailure(t *testing.T) {
	t.Parallel()

	store := NewPathStore()
	w := &PathWatcher{store: store}

	// Should be a no-op and not panic
	w.upsert("not-a-path-cr")
	w.upsert(42)

	list := store.List()
	if len(list) != 0 {
		t.Errorf("expected empty store after bad upsert, got %d entries", len(list))
	}
}

// TestPathWatcher_Remove_HappyPath checks path watcher remove happy path.
func TestPathWatcher_Remove_HappyPath(t *testing.T) {
	t.Parallel()

	store := NewPathStore()
	store.Put(&Path{Slug: "to-remove", Source: "k8s:to-remove"})
	w := &PathWatcher{store: store}

	cr := makePathCR("to-remove", coursev1.PathSpec{})
	w.remove(cr)

	if store.Get("to-remove") != nil {
		t.Error("expected path to be removed")
	}
}

// TestPathWatcher_Remove_DeletedFinalStateUnknown checks tombstone removal.
func TestPathWatcher_Remove_DeletedFinalStateUnknown(t *testing.T) {
	t.Parallel()

	store := NewPathStore()
	store.Put(&Path{Slug: "wrapped-path", Source: "k8s:wrapped-path"})
	w := &PathWatcher{store: store}

	cr := makePathCR("wrapped-path", coursev1.PathSpec{})
	w.remove(toolscache.DeletedFinalStateUnknown{Key: "default/wrapped-path", Obj: cr})

	if store.Get("wrapped-path") != nil {
		t.Error("expected path to be removed via DeletedFinalStateUnknown")
	}
}

// TestPathWatcher_Remove_TypeAssertFailure checks remove type assert failure.
func TestPathWatcher_Remove_TypeAssertFailure(t *testing.T) {
	t.Parallel()

	store := NewPathStore()
	store.Put(&Path{Slug: "safe", Source: "k8s:safe"})
	w := &PathWatcher{store: store}

	// Should be a no-op and not panic
	w.remove("not-a-cr")

	if store.Get("safe") == nil {
		t.Error("expected path to be untouched after bad remove")
	}
}
