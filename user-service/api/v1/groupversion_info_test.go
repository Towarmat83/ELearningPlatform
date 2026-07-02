package v1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}
	if !scheme.Recognizes(GroupVersion.WithKind("MarkdownPattern")) {
		t.Error("scheme does not recognize MarkdownPattern")
	}
	if !scheme.Recognizes(GroupVersion.WithKind("MarkdownPatternList")) {
		t.Error("scheme does not recognize MarkdownPatternList")
	}
}

func newFullMarkdownPattern() *MarkdownPattern {
	return &MarkdownPattern{
		Spec: MarkdownPatternSpec{
			Name:        "callout",
			Label:       "Callout",
			Description: "Renders a highlighted callout box",
			HTML:        "<div class=\"callout\">{{content}}</div>",
			CSS:         ".callout { color: red; }",
			JS:          "console.log('rendered')",
			Scope:       "global",
		},
	}
}

func TestMarkdownPatternDeepCopy(t *testing.T) {
	original := newFullMarkdownPattern()
	copied := original.DeepCopy()

	if copied == original {
		t.Fatal("DeepCopy returned the same pointer as the original")
	}

	copied.Spec.Name = "mutated"
	if original.Spec.Name != "callout" {
		t.Error("DeepCopy did not deep-copy MarkdownPatternSpec")
	}
}

func TestMarkdownPatternDeepCopyObject(t *testing.T) {
	original := newFullMarkdownPattern()
	obj := original.DeepCopyObject()
	copied, ok := obj.(*MarkdownPattern)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *MarkdownPattern", obj)
	}
	if copied == original {
		t.Fatal("DeepCopyObject returned the same pointer as the original")
	}
}

func TestMarkdownPatternListDeepCopy(t *testing.T) {
	original := &MarkdownPatternList{Items: []MarkdownPattern{*newFullMarkdownPattern()}}
	copied := original.DeepCopy()

	copied.Items[0].Spec.Name = "mutated"
	if original.Items[0].Spec.Name != "callout" {
		t.Error("DeepCopy did not deep-copy MarkdownPatternList.Items")
	}

	obj := original.DeepCopyObject()
	if _, ok := obj.(*MarkdownPatternList); !ok {
		t.Fatalf("DeepCopyObject returned %T, want *MarkdownPatternList", obj)
	}
}

func TestMarkdownPatternSpecDeepCopy(t *testing.T) {
	spec := &MarkdownPatternSpec{Name: "callout", HTML: "<div>{{content}}</div>"}
	got := spec.DeepCopy()
	if got == spec || *got != *spec {
		t.Error("MarkdownPatternSpec.DeepCopy did not produce an equal, distinct copy")
	}
}

func TestDeepCopyNilReceiver(t *testing.T) {
	var nilSpec *MarkdownPatternSpec
	var nilPattern *MarkdownPattern
	var nilList *MarkdownPatternList
	if nilSpec.DeepCopy() != nil ||
		nilPattern.DeepCopy() != nil ||
		nilList.DeepCopy() != nil {
		t.Error("DeepCopy on a nil receiver should return nil")
	}
	if nilPattern.DeepCopyObject() != nil {
		t.Error("MarkdownPattern.DeepCopyObject on a nil receiver should return nil")
	}
	if nilList.DeepCopyObject() != nil {
		t.Error("MarkdownPatternList.DeepCopyObject on a nil receiver should return nil")
	}
}
