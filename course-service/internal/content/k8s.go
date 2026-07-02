package content

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"

	coursev1 "github.com/elearning/course-service/api/v1"
)

// K8sWatcher watches Course CRDs in the cluster and populates a Store.
// It uses controller-runtime's informer-backed cache, which relists on
// watch errors/reconnects automatically instead of only once at startup.
type K8sWatcher struct {
	store *Store
	cache crcache.Cache
}

// NewK8sWatcher creates a watcher that syncs Course CRDs into the given store.
func NewK8sWatcher(store *Store, kubeconfig, namespace string) (*K8sWatcher, error) {
	cfg, err := restConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := coursev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	c, err := crcache.New(cfg, crcache.Options{
		Scheme:            scheme,
		DefaultNamespaces: map[string]crcache.Config{namespace: {}},
	})
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	return &K8sWatcher{store: store, cache: c}, nil
}

// Start begins watching Course CRDs and blocks until the initial list is synced.
func (w *K8sWatcher) Start(ctx context.Context) error {
	informer, err := w.cache.GetInformer(ctx, &coursev1.Course{})
	if err != nil {
		return fmt.Errorf("get informer: %w", err)
	}

	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.upsert(obj) },
		UpdateFunc: func(_, obj interface{}) { w.upsert(obj) },
		DeleteFunc: func(obj interface{}) { w.remove(obj) },
	}); err != nil {
		return fmt.Errorf("add event handler: %w", err)
	}

	go func() {
		if err := w.cache.Start(ctx); err != nil {
			slog.Error("k8s cache stopped", "err", err)
		}
	}()

	if !w.cache.WaitForCacheSync(ctx) {
		return fmt.Errorf("cache sync failed")
	}
	slog.Info("initial course list synced from K8s")
	return nil
}

func (w *K8sWatcher) upsert(obj interface{}) {
	cr, ok := obj.(*coursev1.Course)
	if !ok {
		return
	}
	c := courseFromCR(cr)
	w.store.Put(c)
	slog.Debug("course upserted from K8s", "slug", c.Slug)
}

func (w *K8sWatcher) remove(obj interface{}) {
	if final, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = final.Obj
	}
	cr, ok := obj.(*coursev1.Course)
	if !ok {
		return
	}
	w.store.DeleteBySource(sourceK8s(cr.Name))
	slog.Debug("course removed from K8s", "slug", cr.Name)
}

// courseFromCR converts a typed Course custom resource into the in-memory Course.
func courseFromCR(cr *coursev1.Course) *Course {
	slug := cr.Name
	spec := cr.Spec

	title := spec.Title
	if title == "" {
		title = slug
	}

	var prerequisites []CoursePrerequisite
	for _, p := range spec.Prerequisites {
		if p.Course == "" {
			continue
		}
		prerequisites = append(prerequisites, CoursePrerequisite{
			Course:   p.Course,
			MinScore: p.MinScore,
			Modules:  p.Modules,
		})
	}

	modules := make([]Module, 0, len(spec.Modules))
	for i, m := range spec.Modules {
		modules = append(modules, moduleFromCR(i, m))
	}

	return &Course{
		Slug:          slug,
		Title:         title,
		Description:   spec.Description,
		Category:      spec.Category,
		Difficulty:    spec.Difficulty,
		IsPublic:      spec.Public,
		Prerequisites: prerequisites,
		Modules:       modules,
		Source:        sourceK8s(slug),
	}
}

func moduleFromCR(index int, m coursev1.Module) Module {
	mod := Module{
		Name:                   m.Name,
		Type:                   m.Type,
		Src:                    m.Src,
		Ref:                    m.Ref,
		Path:                   m.Path,
		LabURL:                 m.LabURL,
		InlineContent:          m.InlineContent,
		Replication:            m.Replication,
		Hidden:                 m.Hidden,
		Inline:                 m.Inline,
		Prerequisites:          m.Prerequisites,
		PassingScore:           m.PassingScore,
		MaxAttemptsPerQuestion: m.MaxAttemptsPerQuestion,
		LockOnMaxAttempts:      m.LockOnMaxAttempts,
		CheckProvider:          m.CheckProvider,
		CheckType:              m.CheckType,
		CheckParams:            rawExtensionToMap(m.CheckParams),
		Steps:                  stepsFromCR(m.Steps),
	}
	if mod.Name == "" {
		mod.Name = fmt.Sprintf("module-%d", index+1)
	}
	if mod.Type == "" {
		mod.Type = "text"
	}

	// Parse inline quiz config for quiz-type modules.
	if mod.Type == "quiz" {
		if m.Cooldown != nil {
			mod.Cooldown = CooldownSpec{
				Strategy:    m.Cooldown.Strategy,
				BaseSeconds: m.Cooldown.BaseSeconds,
				Multiplier:  m.Cooldown.Multiplier,
				MaxSeconds:  m.Cooldown.MaxSeconds,
			}
			if mod.Cooldown.Strategy == "" {
				mod.Cooldown.Strategy = "exponential"
			}
			if mod.Cooldown.BaseSeconds == 0 {
				mod.Cooldown.BaseSeconds = 30
			}
			if mod.Cooldown.Multiplier == 0 {
				mod.Cooldown.Multiplier = 1.0
			}
		}
		for i, q := range m.Questions {
			mod.Questions = append(mod.Questions, questionFromCR(i, q))
		}
	}

	return mod
}

func questionFromCR(index int, q coursev1.Question) Question {
	question := Question{
		ID:            q.ID,
		Type:          q.Type,
		Difficulty:    q.Difficulty,
		Points:        q.Points,
		Question:      q.Question,
		CorrectAnswer: q.CorrectAnswer,
		CorrectOrder:  q.CorrectOrder,
	}
	for _, a := range q.Answers {
		question.Answers = append(question.Answers, Answer{ID: a.ID, Text: a.Text, Correct: a.Correct})
	}
	for _, it := range q.Items {
		question.Items = append(question.Items, OrderItem{ID: it.ID, Text: it.Text})
	}
	if question.ID == "" {
		question.ID = fmt.Sprintf("q-%d", index+1)
	}
	if question.Difficulty == "" {
		question.Difficulty = "medium"
	}
	if question.Points == 0 {
		question.Points = 1
	}
	if q.PartialScoring != nil {
		question.PartialScoring = &PartialScoring{
			Enabled:       q.PartialScoring.Enabled,
			AllowNegative: q.PartialScoring.AllowNegative,
		}
	}
	question.Feedback = Feedback{
		Wrong:   q.Feedback.Wrong,
		Correct: q.Feedback.Correct,
	}
	for _, sr := range q.Feedback.SourceRefs {
		question.Feedback.SourceRefs = append(question.Feedback.SourceRefs, SourceRef{
			Course:   sr.Course,
			Module:   sr.Module,
			Anchor:   sr.Anchor,
			Priority: sr.Priority,
		})
	}
	return question
}

func stepsFromCR(steps []coursev1.CheckStep) []CheckStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]CheckStep, 0, len(steps))
	for _, s := range steps {
		out = append(out, CheckStep{
			Title:       s.Title,
			CheckType:   s.CheckType,
			CheckParams: rawExtensionToMap(s.CheckParams),
		})
	}
	return out
}

func rawExtensionToMap(re *runtime.RawExtension) map[string]interface{} {
	if re == nil || len(re.Raw) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(re.Raw, &m); err != nil {
		return nil
	}
	return m
}

func sourceK8s(slug string) string {
	return "k8s:" + slug
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
