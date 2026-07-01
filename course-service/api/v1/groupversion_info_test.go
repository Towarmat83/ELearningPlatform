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
	if !scheme.Recognizes(GroupVersion.WithKind("Course")) {
		t.Error("scheme does not recognize Course")
	}
	if !scheme.Recognizes(GroupVersion.WithKind("CourseList")) {
		t.Error("scheme does not recognize CourseList")
	}
}

func newFullCourse() *Course {
	maxAttempts := 3
	correctAnswer := true
	return &Course{
		Spec: CourseSpec{
			Title:       "Intro to Go",
			Description: "Learn Go",
			Public:      true,
			Category:    "programming",
			Difficulty:  "beginner",
			Prerequisites: []CoursePrerequisite{
				{Course: "prereq-course", MinScore: 10, Modules: []string{"mod-a", "mod-b"}},
			},
			Modules: []Module{
				{
					Name:          "video-module",
					Type:          "video",
					Src:           "https://example.com/video.mp4",
					Prerequisites: []string{"other-module"},
					Questions: []Question{
						{
							Type:          "single",
							Question:      "2+2?",
							Answers:       []Answer{{ID: "a1", Text: "4", Correct: true}},
							CorrectOrder:  []string{"a1"},
							Items:         []OrderItem{{ID: "i1", Text: "first"}},
							CorrectAnswer: &correctAnswer,
							PartialScoring: &PartialScoring{
								Enabled:       true,
								AllowNegative: false,
							},
							Feedback: Feedback{
								Wrong:   "nope",
								Correct: "yes",
								SourceRefs: []SourceRef{
									{Course: "course", Module: "module", Anchor: "anchor", Priority: 1},
								},
							},
						},
					},
					Cooldown: &CooldownSpec{
						Strategy:    "exponential",
						BaseSeconds: 5,
						Multiplier:  2.0,
						MaxSeconds:  60,
					},
					MaxAttemptsPerQuestion: &maxAttempts,
					CheckParams: &runtime.RawExtension{
						Raw: []byte(`{"key":"value"}`),
					},
					Steps: []CheckStep{
						{
							Title:     "step-1",
							CheckType: "file-exists",
							CheckParams: &runtime.RawExtension{
								Raw: []byte(`{"path":"/tmp/foo"}`),
							},
						},
					},
				},
			},
		},
	}
}

func TestCourseDeepCopy(t *testing.T) {
	original := newFullCourse()
	copied := original.DeepCopy()

	if copied == original {
		t.Fatal("DeepCopy returned the same pointer as the original")
	}

	// Mutate the copy's nested slices/pointers and verify the original is untouched.
	copied.Spec.Modules[0].Prerequisites[0] = "mutated"
	copied.Spec.Modules[0].Questions[0].Answers[0].Text = "mutated"
	*copied.Spec.Modules[0].Questions[0].CorrectAnswer = false
	copied.Spec.Modules[0].Cooldown.Strategy = "fixed"
	*copied.Spec.Modules[0].MaxAttemptsPerQuestion = 99
	copied.Spec.Modules[0].CheckParams.Raw[0] = 'X'
	copied.Spec.Modules[0].Steps[0].Title = "mutated"

	if original.Spec.Modules[0].Prerequisites[0] != "other-module" {
		t.Error("DeepCopy did not deep-copy Module.Prerequisites")
	}
	if original.Spec.Modules[0].Questions[0].Answers[0].Text != "4" {
		t.Error("DeepCopy did not deep-copy Question.Answers")
	}
	if !*original.Spec.Modules[0].Questions[0].CorrectAnswer {
		t.Error("DeepCopy did not deep-copy Question.CorrectAnswer pointer")
	}
	if original.Spec.Modules[0].Cooldown.Strategy != "exponential" {
		t.Error("DeepCopy did not deep-copy Module.Cooldown pointer")
	}
	if *original.Spec.Modules[0].MaxAttemptsPerQuestion != 3 {
		t.Error("DeepCopy did not deep-copy Module.MaxAttemptsPerQuestion pointer")
	}
	if original.Spec.Modules[0].CheckParams.Raw[0] == 'X' {
		t.Error("DeepCopy did not deep-copy Module.CheckParams")
	}
	if original.Spec.Modules[0].Steps[0].Title != "step-1" {
		t.Error("DeepCopy did not deep-copy Module.Steps")
	}
}

func TestCourseDeepCopyObject(t *testing.T) {
	original := newFullCourse()
	obj := original.DeepCopyObject()
	copied, ok := obj.(*Course)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *Course", obj)
	}
	if copied == original {
		t.Fatal("DeepCopyObject returned the same pointer as the original")
	}
}

func TestLeafTypesDeepCopy(t *testing.T) {
	correctAnswer := true

	answer := &Answer{ID: "a1", Text: "4", Correct: true}
	if got := answer.DeepCopy(); got == answer || *got != *answer {
		t.Error("Answer.DeepCopy did not produce an equal, distinct copy")
	}

	cooldown := &CooldownSpec{Strategy: "fixed", BaseSeconds: 1, Multiplier: 1.5, MaxSeconds: 30}
	if got := cooldown.DeepCopy(); got == cooldown || *got != *cooldown {
		t.Error("CooldownSpec.DeepCopy did not produce an equal, distinct copy")
	}

	prereq := &CoursePrerequisite{Course: "c1", MinScore: 5, Modules: []string{"m1"}}
	if got := prereq.DeepCopy(); got == prereq || got.Modules[0] != "m1" {
		t.Error("CoursePrerequisite.DeepCopy did not produce an equal, distinct copy")
	}
	prereq.DeepCopyInto(&CoursePrerequisite{})

	feedback := &Feedback{Wrong: "no", Correct: "yes", SourceRefs: []SourceRef{{Course: "c1"}}}
	if got := feedback.DeepCopy(); got == feedback || got.SourceRefs[0].Course != "c1" {
		t.Error("Feedback.DeepCopy did not produce an equal, distinct copy")
	}

	orderItem := &OrderItem{ID: "i1", Text: "first"}
	if got := orderItem.DeepCopy(); got == orderItem || *got != *orderItem {
		t.Error("OrderItem.DeepCopy did not produce an equal, distinct copy")
	}

	partial := &PartialScoring{Enabled: true, AllowNegative: true}
	if got := partial.DeepCopy(); got == partial || *got != *partial {
		t.Error("PartialScoring.DeepCopy did not produce an equal, distinct copy")
	}

	sourceRef := &SourceRef{Course: "c1", Module: "m1", Anchor: "a1", Priority: 2}
	if got := sourceRef.DeepCopy(); got == sourceRef || *got != *sourceRef {
		t.Error("SourceRef.DeepCopy did not produce an equal, distinct copy")
	}

	checkStep := &CheckStep{Title: "t1", CheckType: "ct1", CheckParams: &runtime.RawExtension{Raw: []byte(`{}`)}}
	if got := checkStep.DeepCopy(); got == checkStep || got.CheckParams == checkStep.CheckParams {
		t.Error("CheckStep.DeepCopy did not produce an equal, distinct copy")
	}

	question := &Question{Type: "boolean", Question: "q?", CorrectAnswer: &correctAnswer}
	if got := question.DeepCopy(); got == question || got.CorrectAnswer == question.CorrectAnswer {
		t.Error("Question.DeepCopy did not produce an equal, distinct copy")
	}

	module := newFullCourse().Spec.Modules[0]
	if got := module.DeepCopy(); got == &module || got.Cooldown == module.Cooldown {
		t.Error("Module.DeepCopy did not produce an equal, distinct copy")
	}

	spec := newFullCourse().Spec
	if got := spec.DeepCopy(); got == &spec || len(got.Modules) != len(spec.Modules) {
		t.Error("CourseSpec.DeepCopy did not produce an equal, distinct copy")
	}

	var nilAnswer *Answer
	var nilCooldown *CooldownSpec
	var nilPrereq *CoursePrerequisite
	var nilFeedback *Feedback
	var nilOrderItem *OrderItem
	var nilPartial *PartialScoring
	var nilSourceRef *SourceRef
	var nilCheckStep *CheckStep
	var nilQuestion *Question
	var nilModule *Module
	var nilSpec *CourseSpec
	var nilCourse *Course
	var nilCourseList *CourseList
	if nilAnswer.DeepCopy() != nil ||
		nilCooldown.DeepCopy() != nil ||
		nilPrereq.DeepCopy() != nil ||
		nilFeedback.DeepCopy() != nil ||
		nilOrderItem.DeepCopy() != nil ||
		nilPartial.DeepCopy() != nil ||
		nilSourceRef.DeepCopy() != nil ||
		nilCheckStep.DeepCopy() != nil ||
		nilQuestion.DeepCopy() != nil ||
		nilModule.DeepCopy() != nil ||
		nilSpec.DeepCopy() != nil ||
		nilCourse.DeepCopy() != nil ||
		nilCourseList.DeepCopy() != nil {
		t.Error("DeepCopy on a nil receiver should return nil")
	}
	if nilCourse.DeepCopyObject() != nil {
		t.Error("Course.DeepCopyObject on a nil receiver should return nil")
	}
	if nilCourseList.DeepCopyObject() != nil {
		t.Error("CourseList.DeepCopyObject on a nil receiver should return nil")
	}
}

func TestCourseListDeepCopy(t *testing.T) {
	original := &CourseList{Items: []Course{*newFullCourse()}}
	copied := original.DeepCopy()

	copied.Items[0].Spec.Title = "mutated"
	if original.Items[0].Spec.Title != "Intro to Go" {
		t.Error("DeepCopy did not deep-copy CourseList.Items")
	}

	obj := original.DeepCopyObject()
	if _, ok := obj.(*CourseList); !ok {
		t.Fatalf("DeepCopyObject returned %T, want *CourseList", obj)
	}
}
