package content

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// ── crdToPath ────────────────────────────────────────────────────────────────

func makePath(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetUnstructuredContent(map[string]interface{}{
		"metadata": map[string]interface{}{"name": name},
		"spec":     spec,
	})
	return obj
}

func TestCrdToPath_Basic(t *testing.T) {
	obj := makePath("devops-path", map[string]interface{}{
		"title":       "DevOps Path",
		"description": "From Linux to Kubernetes",
		"courses": []interface{}{
			map[string]interface{}{"slug": "linux-intro"},
			map[string]interface{}{"slug": "docker-fundamentals"},
			map[string]interface{}{"slug": "kubernetes-basics"},
		},
	})

	p, err := crdToPath(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestCrdToPath_StringCourses(t *testing.T) {
	obj := makePath("simple-path", map[string]interface{}{
		"title": "Simple Path",
		"courses": []interface{}{
			"linux-intro",
			"python-basics",
		},
	})

	p, err := crdToPath(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Courses) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(p.Courses))
	}
	if p.Courses[1] != "python-basics" {
		t.Errorf("courses[1]: want python-basics, got %q", p.Courses[1])
	}
}

func TestCrdToPath_MissingSpec(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("bad-path")
	obj.SetUnstructuredContent(map[string]interface{}{})

	_, err := crdToPath(obj)
	if err == nil {
		t.Error("expected error for missing spec")
	}
}

func TestCrdToPath_FallbackTitle(t *testing.T) {
	obj := makePath("my-path", map[string]interface{}{})

	p, err := crdToPath(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Title != "my-path" {
		t.Errorf("expected title to fallback to slug, got %q", p.Title)
	}
}

func TestCrdToPath_Source(t *testing.T) {
	obj := makePath("devops-path", map[string]interface{}{"title": "DevOps"})
	p, err := crdToPath(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Source != "k8s:devops-path" {
		t.Errorf("source: want k8s:devops-path, got %q", p.Source)
	}
}
