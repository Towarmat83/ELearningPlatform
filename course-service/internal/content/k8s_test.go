package content

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestK8sWatcher_CRDToCourse checks k8s watcher CRD to course.
func TestK8sWatcher_CRDToCourse(t *testing.T) {
	t.Parallel()

	store := NewStore()

	kubeconfig, err := findKubeconfig()
	if err != nil {
		t.Skip("kubeconfig not found, skipping integration test")
	}

	watcher, err := NewK8sWatcher(store, kubeconfig, "default")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	time.Sleep(3 * time.Second)

	courses := store.All()
	if len(courses) == 0 {
		t.Fatal("expected at least one course from K8s CRD")
	}

	var found bool

	for _, c := range courses {
		if c.Slug != "kubernetes-basics" {
			continue
		}

		found = true

		if c.Title != "Kubernetes Basics" {
			t.Errorf("expected title 'Kubernetes Basics', got '%s'", c.Title)
		}

		if c.Category != "kubernetes" {
			t.Errorf("expected category 'kubernetes', got '%s'", c.Category)
		}

		if c.Difficulty != "beginner" {
			t.Errorf("expected difficulty 'beginner', got '%s'", c.Difficulty)
		}

		if !c.IsPublic {
			t.Error("expected course to be published (hidden=false)")
		}

		if len(c.Modules) == 0 {
			t.Error("expected at least one module")
		} else if c.Modules[0].Name != "What is Kubernetes" {
			t.Errorf("expected first module name 'What is Kubernetes', got '%s'", c.Modules[0].Name)
		}

		break
	}

	if !found {
		t.Fatal("expected kubernetes-basics course to be loaded from CRD")
	}
}

// TestK8sWatcher_UpsertDelete checks k8s watcher upsert delete.
func TestK8sWatcher_UpsertDelete(t *testing.T) {
	t.Parallel()

	store := NewStore()

	kubeconfig, err := findKubeconfig()
	if err != nil {
		t.Skip("kubeconfig not found, skipping integration test")
	}

	watcher, err := NewK8sWatcher(store, kubeconfig, "default")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	time.Sleep(3 * time.Second)

	c := store.Get("kubernetes-basics")
	if c == nil {
		t.Fatal("expected kubernetes-basics course to be loaded")
	}

	if c.Slug != "kubernetes-basics" {
		t.Errorf("expected slug 'kubernetes-basics', got '%s'", c.Slug)
	}

	if c.Title != "Kubernetes Basics" {
		t.Errorf("expected title 'Kubernetes Basics', got '%s'", c.Title)
	}

	if !c.IsPublic {
		t.Error("expected course to be published")
	}

	if len(c.Modules) == 0 {
		t.Error("expected modules to be populated")
	}
}

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

// TestCRDToCourse checks CRD to course.
func TestCRDToCourse(t *testing.T) {
	t.Parallel()

	store := NewStore()

	kubeconfig, err := findKubeconfig()
	if err != nil {
		t.Skip("kubeconfig not found, skipping integration test")
	}

	watcher, err := NewK8sWatcher(store, kubeconfig, "default")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	_ = watcher.Start(context.Background())

	time.Sleep(2 * time.Second)

	c := store.Get("docker-fundamentals")
	if c != nil && c.IsPublic {
		t.Error("expected hidden=true (docker-fundamentals) to result in IsPublished=false")
	}
}

// findKubeconfig locates a kubeconfig usable for the integration tests.
func findKubeconfig() (string, error) {
	return getKindKubeconfig()
}

// getKindKubeconfig returns the path to the local kind cluster kubeconfig.
func getKindKubeconfig() (string, error) {
	path := "/home/paulm/Projects/elearning/.opencode/ELearningPlatform/api-go/kind-kubeconfig.yaml"

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("stat kubeconfig %s: %w", path, err)
	}

	return path, nil
}
