package content

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coursev1 "github.com/elearning/course-service/api/v1"
)

// ── PathStore ────────────────────────────────────────────────────────────────

func TestPathStore_PutAndGet(t *testing.T) {
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

func TestPathStore_List(t *testing.T) {
	s := NewPathStore()
	s.Put(&Path{Slug: "path-b", Title: "B Path"})
	s.Put(&Path{Slug: "path-a", Title: "A Path"})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(list))
	}
	if list[0].Title != "A Path" {
		t.Errorf("expected sorted order, got %q first", list[0].Title)
	}
}

func TestPathStore_DeleteBySource(t *testing.T) {
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

func TestPathStore_GetMissing(t *testing.T) {
	s := NewPathStore()
	if s.Get("nonexistent") != nil {
		t.Error("expected nil for missing path")
	}
}

// ── pathFromCR ───────────────────────────────────────────────────────────────

func makePathCR(name string, spec coursev1.PathSpec) *coursev1.Path {
	cr := &coursev1.Path{Spec: spec}
	cr.Name = name
	cr.TypeMeta = metav1.TypeMeta{APIVersion: "elearning.pupitre.io/v1", Kind: "Path"}
	return cr
}

func TestPathFromCR_Basic(t *testing.T) {
	cr := makePathCR("devops-path", coursev1.PathSpec{
		Title:       "DevOps Path",
		Description: "From Linux to Kubernetes",
		Courses: []coursev1.PathCourseEntry{
			{Slug: "linux-intro"},
			{Slug: "docker-fundamentals"},
			{Slug: "kubernetes-basics"},
		},
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

func TestPathFromCR_FallbackTitle(t *testing.T) {
	cr := makePathCR("my-path", coursev1.PathSpec{})
	p := pathFromCR(cr)
	if p.Title != "my-path" {
		t.Errorf("expected title to fallback to slug, got %q", p.Title)
	}
}

func TestPathFromCR_Source(t *testing.T) {
	cr := makePathCR("devops-path", coursev1.PathSpec{Title: "DevOps"})
	p := pathFromCR(cr)
	if p.Source != "k8s:devops-path" {
		t.Errorf("source: want k8s:devops-path, got %q", p.Source)
	}
}

func TestPathFromCR_EmptyCourseSlugSkipped(t *testing.T) {
	cr := makePathCR("test-path", coursev1.PathSpec{
		Courses: []coursev1.PathCourseEntry{
			{Slug: "valid-course"},
			{Slug: ""},
			{Slug: "another-course"},
		},
	})
	p := pathFromCR(cr)
	if len(p.Courses) != 2 {
		t.Errorf("expected 2 courses (empty slug skipped), got %d", len(p.Courses))
	}
}
