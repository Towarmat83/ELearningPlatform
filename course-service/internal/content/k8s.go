package content

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coursev1 "github.com/genesary/pupitre/course-service/api/v1"
)

// k8sCooldownDefaultBaseSeconds is the default cooldown base duration (in
// seconds) applied to quiz modules that omit it.
const k8sCooldownDefaultBaseSeconds = 30

// k8sCooldownDefaultMultiplier is the default exponential cooldown
// multiplier applied to quiz modules that omit it.
const k8sCooldownDefaultMultiplier = 1.0

// K8sWatcher watches Course CRDs in the cluster and populates a Store.
// It uses controller-runtime's informer-backed cache, which relists on
// watch errors/reconnects automatically instead of only once at startup.
type K8sWatcher struct {
	store *Store
	cache crcache.Cache
}

// NewK8sWatcher creates a watcher that syncs Course CRDs into the given store.
func NewK8sWatcher(store *Store, kubeconfig, namespace string) (*K8sWatcher, error) {
	cache, err := newCRCache(kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	return &K8sWatcher{store: store, cache: cache}, nil
}

// Start begins watching Course CRDs. Cache sync happens in the background
// so the HTTP server can start accepting requests immediately.
func (w *K8sWatcher) Start(ctx context.Context) error {
	return watchCRD(ctx, w.cache, &coursev1.Course{},
		func(obj any) { w.upsert(obj) },
		func(obj any) { w.remove(obj) },
		"k8s cache stopped", "course cache sync failed", "initial course list synced from K8s")
}

// upsert converts obj into a Course and stores it, ignoring objects of
// the wrong type.
func (w *K8sWatcher) upsert(obj any) {
	cr, ok := obj.(*coursev1.Course)
	if !ok {
		return
	}

	course := courseFromCR(cr)
	w.store.Put(course)
	zap.L().Debug("course upserted from K8s", zap.String("slug", course.Slug))
}

// remove deletes the course backed by obj from the store, ignoring
// objects of the wrong type.
func (w *K8sWatcher) remove(obj any) {
	if final, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = final.Obj
	}

	course, ok := obj.(*coursev1.Course)
	if !ok {
		return
	}

	w.store.DeleteBySource(sourceK8s(course.Name))
	zap.L().Debug("course removed from K8s", zap.String("slug", course.Name))
}

// courseFromCR converts a typed Course custom resource into the
// in-memory Course.
func courseFromCR(cr *coursev1.Course) *Course {
	slug := cr.Name
	spec := cr.Spec

	title := spec.Title
	if title == "" {
		title = slug
	}

	var prerequisites []CoursePrerequisite

	for _, prereq := range spec.Prerequisites {
		if prereq.Course == "" {
			continue
		}

		prerequisites = append(prerequisites, CoursePrerequisite{
			Course:   prereq.Course,
			MinScore: prereq.MinScore,
			Modules:  prereq.Modules,
		})
	}

	modules := make([]Module, 0, len(spec.Modules))
	for i, crModule := range spec.Modules {
		modules = append(modules, moduleFromCR(i, crModule))
	}

	var badge *Badge
	if spec.Badge != nil {
		badge = &Badge{Name: spec.Badge.Name, Icon: spec.Badge.Icon}
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
		Skills:        aggregateSkills(modules),
		Scope:         spec.Scope,
		Badge:         badge,
		Source:        sourceK8s(slug),
	}
}

// moduleFromCR converts a single CRD module entry into the in-memory
// Module, applying name/type/cooldown defaults and parsing inline quiz
// questions when present.
func moduleFromCR(index int, crModule coursev1.Module) Module {
	mod := Module{
		Name:                   crModule.Name,
		Type:                   crModule.Type,
		Src:                    crModule.Src,
		Ref:                    crModule.Ref,
		Path:                   crModule.Path,
		LabURL:                 crModule.LabURL,
		InlineContent:          crModule.InlineContent,
		Replication:            crModule.Replication,
		Hidden:                 crModule.Hidden,
		Inline:                 crModule.Inline,
		Prerequisites:          crModule.Prerequisites,
		PassingScore:           crModule.PassingScore,
		MaxAttemptsPerQuestion: crModule.MaxAttemptsPerQuestion,
		LockOnMaxAttempts:      crModule.LockOnMaxAttempts,
		CheckProvider:          crModule.CheckProvider,
		CheckType:              crModule.CheckType,
		CheckParams:            rawExtensionToMap(crModule.CheckParams),
		Steps:                  stepsFromCR(crModule.Steps),
		Skills:                 crModule.Skills,
	}
	if mod.Name == "" {
		mod.Name = fmt.Sprintf("module-%d", index+1)
	}

	if mod.Type == "" {
		mod.Type = moduleTypeText
	}

	// Parse inline quiz config for quiz-type modules.
	if mod.Type == moduleTypeQuiz {
		applyQuizDefaults(&mod, crModule)
	}

	return mod
}

// applyQuizDefaults populates mod's Cooldown and Questions fields from
// crModule, defaulting Cooldown's Strategy/BaseSeconds/Multiplier when
// omitted. It is only called for quiz-type modules.
func applyQuizDefaults(mod *Module, crModule coursev1.Module) {
	if crModule.Cooldown != nil {
		mod.Cooldown = CooldownSpec{
			Strategy:    crModule.Cooldown.Strategy,
			BaseSeconds: crModule.Cooldown.BaseSeconds,
			Multiplier:  crModule.Cooldown.Multiplier,
			MaxSeconds:  crModule.Cooldown.MaxSeconds,
		}
		if mod.Cooldown.Strategy == "" {
			mod.Cooldown.Strategy = cooldownStrategyExponential
		}

		if mod.Cooldown.BaseSeconds == 0 {
			mod.Cooldown.BaseSeconds = k8sCooldownDefaultBaseSeconds
		}

		if mod.Cooldown.Multiplier == 0 {
			mod.Cooldown.Multiplier = k8sCooldownDefaultMultiplier
		}
	}

	for i, q := range crModule.Questions {
		mod.Questions = append(mod.Questions, questionFromCR(i, q))
	}
}

// questionFromCR converts a single CRD question entry into the in-memory
// Question, applying ID/Difficulty/Points defaults.
func questionFromCR(index int, crQuestion coursev1.Question) Question {
	question := Question{
		ID:            crQuestion.ID,
		Type:          crQuestion.Type,
		Difficulty:    crQuestion.Difficulty,
		Points:        crQuestion.Points,
		Question:      crQuestion.Question,
		CorrectAnswer: crQuestion.CorrectAnswer,
		CorrectOrder:  crQuestion.CorrectOrder,
	}
	for _, a := range crQuestion.Answers {
		question.Answers = append(question.Answers, Answer{ID: a.ID, Text: a.Text, Correct: a.Correct})
	}

	for _, it := range crQuestion.Items {
		question.Items = append(question.Items, OrderItem{ID: it.ID, Text: it.Text})
	}

	if question.ID == "" {
		question.ID = fmt.Sprintf("q-%d", index+1)
	}

	if question.Difficulty == "" {
		question.Difficulty = difficultyMedium
	}

	if question.Points == 0 {
		question.Points = 1
	}

	if crQuestion.PartialScoring != nil {
		question.PartialScoring = &PartialScoring{
			Enabled:       crQuestion.PartialScoring.Enabled,
			AllowNegative: crQuestion.PartialScoring.AllowNegative,
		}
	}

	question.Feedback = Feedback{
		Wrong:   crQuestion.Feedback.Wrong,
		Correct: crQuestion.Feedback.Correct,
	}
	for _, sr := range crQuestion.Feedback.SourceRefs {
		question.Feedback.SourceRefs = append(question.Feedback.SourceRefs, SourceRef{
			Course:   sr.Course,
			Module:   sr.Module,
			Anchor:   sr.Anchor,
			Priority: sr.Priority,
		})
	}

	return question
}

// stepsFromCR converts CRD check steps into their in-memory form,
// returning nil when steps is empty.
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

// rawExtensionToMap decodes a Kubernetes RawExtension into a generic map,
// returning nil when raw is empty or invalid JSON.
func rawExtensionToMap(raw *runtime.RawExtension) map[string]any {
	if raw == nil || len(raw.Raw) == 0 {
		return nil
	}

	var decoded map[string]any

	err := json.Unmarshal(raw.Raw, &decoded)
	if err != nil {
		return nil
	}

	return decoded
}

// sourceK8s builds the Source value recorded for content synced from K8s.
func sourceK8s(slug string) string {
	return "k8s:" + slug
}

// PathWatcher watches Path CRDs in the cluster and populates a PathStore.
// It mirrors the K8sWatcher pattern using controller-runtime cache.
type PathWatcher struct {
	store *PathStore
	cache crcache.Cache
}

// NewPathWatcher creates a watcher that syncs Path CRDs into the given store.
func NewPathWatcher(store *PathStore, kubeconfig, namespace string) (*PathWatcher, error) {
	cache, err := newCRCache(kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	return &PathWatcher{store: store, cache: cache}, nil
}

// Start begins watching Path CRDs. Cache sync happens in the background
// so the HTTP server can start accepting requests immediately.
func (w *PathWatcher) Start(ctx context.Context) error {
	return watchCRD(ctx, w.cache, &coursev1.Path{},
		func(obj any) { w.upsert(obj) },
		func(obj any) { w.remove(obj) },
		"k8s path cache stopped", "path cache sync failed", "initial path list synced from K8s")
}

// upsert converts obj into a Path and stores it, ignoring objects of the
// wrong type.
func (w *PathWatcher) upsert(obj any) {
	cr, ok := obj.(*coursev1.Path)
	if !ok {
		return
	}

	learningPath := pathFromCR(cr)
	w.store.Put(learningPath)
	zap.L().Debug("path upserted from K8s", zap.String("slug", learningPath.Slug))
}

// remove deletes the path backed by obj from the store, ignoring objects
// of the wrong type.
func (w *PathWatcher) remove(obj any) {
	if final, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = final.Obj
	}

	pathCR, ok := obj.(*coursev1.Path)
	if !ok {
		return
	}

	w.store.DeleteBySource(sourceK8s(pathCR.Name))
	zap.L().Debug("path removed from K8s", zap.String("slug", pathCR.Name))
}

// pathFromCR converts a typed Path custom resource into the in-memory Path.
func pathFromCR(pathCR *coursev1.Path) *Path {
	slug := pathCR.Name

	title := pathCR.Spec.Title
	if title == "" {
		title = slug
	}

	courses := make([]string, 0, len(pathCR.Spec.Courses))
	for _, courseSlug := range pathCR.Spec.Courses {
		if courseSlug != "" {
			courses = append(courses, courseSlug)
		}
	}

	skillSet := make(map[string]struct{}, len(pathCR.Spec.Skills))
	for _, skill := range pathCR.Spec.Skills {
		if skill != "" {
			skillSet[skill] = struct{}{}
		}
	}

	skills := make([]string, 0, len(skillSet))
	for skill := range skillSet {
		skills = append(skills, skill)
	}

	kind := pathCR.Spec.Kind
	if kind == "" {
		kind = "course"
	}

	return &Path{
		Slug:        slug,
		Title:       title,
		Description: pathCR.Spec.Description,
		Kind:        kind,
		Level:       pathCR.Spec.Level,
		Courses:     courses,
		Skills:      skills,
		Source:      sourceK8s(slug),
	}
}

// restConfig builds a Kubernetes REST config from kubeconfig, falling
// back to the in-cluster config when kubeconfig is empty.
func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build config from kubeconfig: %w", err)
		}

		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("build in-cluster config: %w", err)
	}

	return cfg, nil
}

// newCRCache builds a controller-runtime cache scoped to namespace,
// backed by kubeconfig (or the in-cluster config when kubeconfig is
// empty).
//
// crcache.Cache is controller-runtime's own interface; wrapping it in
// a concrete type here would add no value.
func newCRCache(kubeconfig, namespace string) (crcache.Cache, error) { //nolint:ireturn // third-party interface, no useful concrete wrapper
	cfg, err := restConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	scheme := runtime.NewScheme()

	err = coursev1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	cache, err := crcache.New(cfg, crcache.Options{
		Scheme:            scheme,
		DefaultNamespaces: map[string]crcache.Config{namespace: {}},
	})
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	return cache, nil
}

// watchCRD registers onUpsert/onRemove informer event handlers for
// objects of the same type as example, starts cache in the background,
// and logs the outcome using stoppedMsg/syncFailedMsg/syncedMsg.
func watchCRD(
	ctx context.Context,
	cache crcache.Cache,
	example client.Object,
	onUpsert, onRemove func(obj any),
	stoppedMsg, syncFailedMsg, syncedMsg string,
) error {
	informer, err := cache.GetInformer(ctx, example)
	if err != nil {
		return fmt.Errorf("get informer: %w", err)
	}

	_, err = informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { onUpsert(obj) },
		UpdateFunc: func(_, obj any) { onUpsert(obj) },
		DeleteFunc: func(obj any) { onRemove(obj) },
	})
	if err != nil {
		return fmt.Errorf("add event handler: %w", err)
	}

	go func() {
		err := cache.Start(ctx)
		if err != nil {
			zap.L().Error(stoppedMsg, zap.Error(err))
		}
	}()

	go func() {
		if !cache.WaitForCacheSync(ctx) {
			zap.L().Error(syncFailedMsg)

			return
		}

		zap.L().Info(syncedMsg)
	}()

	return nil
}

// aggregateSkills returns a deduplicated list of all skill tags declared
// across the given modules.
func aggregateSkills(modules []Module) []string {
	seen := make(map[string]struct{})

	for _, m := range modules {
		for _, s := range m.Skills {
			seen[s] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	out := make([]string, 0, len(seen))

	for s := range seen {
		out = append(out, s)
	}

	return out
}
