package content

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	crdGroup    = "elearning.example.com"
	crdVersion  = "v1"
	crdResource = "courses"
)

// CRD GVR used to watch Course resources.
var courseGVR = schema.GroupVersionResource{
	Group:    crdGroup,
	Version:  crdVersion,
	Resource: crdResource,
}

// K8sWatcher watches Course CRDs in the cluster and populates a Store.
type K8sWatcher struct {
	store     *Store
	client    dynamic.NamespaceableResourceInterface
	namespace string
}

// NewK8sWatcher creates a watcher that syncs Course CRDs into the given store.
func NewK8sWatcher(store *Store, kubeconfig, namespace string) (*K8sWatcher, error) {
	config, err := restConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}

	return &K8sWatcher{
		store:     store,
		client:    dyn.Resource(courseGVR),
		namespace: namespace,
	}, nil
}

// Start begins watching Course CRDs and blocks until ctx is cancelled.
func (w *K8sWatcher) Start(ctx context.Context) error {
	if err := w.listAll(ctx); err != nil {
		return fmt.Errorf("initial list: %w", err)
	}
	go w.watchLoop(ctx)
	return nil
}

func (w *K8sWatcher) listAll(ctx context.Context) error {
	list, err := w.client.Namespace(w.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list courses: %w", err)
	}
	for i := range list.Items {
		w.upsert(&list.Items[i])
	}
	slog.Info("initial course list synced from K8s", "count", len(list.Items))
	return nil
}

func (w *K8sWatcher) watchLoop(ctx context.Context) {
	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		timeout := int64(600)
		watcher, err := w.client.Namespace(w.namespace).Watch(ctx, metav1.ListOptions{
			TimeoutSeconds: &timeout,
		})
		if err != nil {
			slog.Error("k8s watch failed, retrying", "err", err, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = 1 * time.Second

		for event := range watcher.ResultChan() {
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				w.upsert(obj)
			case watch.Deleted:
				w.remove(obj)
			}
		}
	}
}

func (w *K8sWatcher) upsert(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	c, err := crdToCourse(obj)
	if err != nil {
		slog.Warn("invalid Course CRD, skipping", "name", obj.GetName(), "err", err)
		return
	}

	w.store.Put(c)
	slog.Debug("course upserted from K8s", "slug", c.Slug)
}

func (w *K8sWatcher) remove(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	slug := obj.GetName()
	if slug != "" {
		w.store.DeleteBySource(sourceK8s(slug))
		slog.Debug("course removed from K8s", "slug", slug)
	}
}

func crdToCourse(obj *unstructured.Unstructured) (*Course, error) {
	spec, ok := obj.UnstructuredContent()["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing spec")
	}

	slug := obj.GetName()
	title, _ := spec["title"].(string)
	description, _ := spec["description"].(string)
	category, _ := spec["category"].(string)
	difficulty, _ := spec["difficulty"].(string)
	hidden, _ := spec["hidden"].(bool)

	if title == "" {
		title = slug
	}

	var coursePrereqs []string
	if rawPrereqs, ok := spec["prerequisites"].([]interface{}); ok {
		for _, rp := range rawPrereqs {
			if s, ok := rp.(string); ok {
				coursePrereqs = append(coursePrereqs, s)
			}
		}
	}

	var modules []Module
	if rawModules, ok := spec["modules"].([]interface{}); ok {
		for i, raw := range rawModules {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			var modPrereqs []string
			if rawPrereqs, ok := m["prerequisites"].([]interface{}); ok {
				for _, rp := range rawPrereqs {
					if s, ok := rp.(string); ok {
						modPrereqs = append(modPrereqs, s)
					}
				}
			}

			mod := Module{
				Name:          getStr(m, "name"),
				Type:          getStr(m, "type"),
				Src:           getStr(m, "src"),
				Ref:           getStr(m, "ref"),
				Path:          getStr(m, "path"),
				Replication:   getBool(m, "replication"),
				Hidden:        getBool(m, "hidden"),
				Prerequisites: modPrereqs,
				PassingScore:  getInt(m, "passing_score"),
				MaxAttemptsPerQuestion: func() *int {
					if v, ok := m["max_attempts_per_question"]; ok {
						switch n := v.(type) {
						case int:
							return &n
						case float64:
							i := int(n)
							return &i
						}
					}
					return nil
				}(),
				LockOnMaxAttempts: getBool(m, "lock_on_max_attempts"),
			}
			if mod.Name == "" {
				mod.Name = fmt.Sprintf("module-%d", i+1)
			}
			if mod.Type == "" {
				mod.Type = "text"
			}

			// Parse inline quiz config for quiz-type modules
			if mod.Type == "quiz" {
				if rawCD, ok := m["cooldown"].(map[string]interface{}); ok {
					mod.Cooldown = CooldownSpec{
						Strategy:    getStr(rawCD, "strategy"),
						BaseSeconds: getInt(rawCD, "base_seconds"),
						Multiplier: func() float64 {
							if v, ok := rawCD["multiplier"]; ok {
								switch n := v.(type) {
								case float64:
									return n
								case int:
									return float64(n)
								}
							}
							return 1.0
						}(),
						MaxSeconds: getInt(rawCD, "max_seconds"),
					}
					if mod.Cooldown.Strategy == "" {
						mod.Cooldown.Strategy = "exponential"
					}
					if mod.Cooldown.BaseSeconds == 0 {
						mod.Cooldown.BaseSeconds = 30
					}
				}
				// Parse inline questions for quiz-type modules
				if rawQuestions, ok := m["questions"].([]interface{}); ok {
					for _, rq := range rawQuestions {
						q, ok := rq.(map[string]interface{})
						if !ok {
							continue
						}
						question := Question{
							ID:         getStr(q, "id"),
							Type:       getStr(q, "type"),
							Difficulty: getStr(q, "difficulty"),
							Points:     getInt(q, "points"),
							Question:   getStr(q, "question"),
							CorrectAnswer: func() *bool {
								if v, ok := q["correct_answer"]; ok {
									if b, ok := v.(bool); ok {
										return &b
									}
								}
								return nil
							}(),
						}
						if question.ID == "" {
							question.ID = fmt.Sprintf("q-%d", i+1)
						}
						if question.Difficulty == "" {
							question.Difficulty = "medium"
						}
						if question.Points == 0 {
							question.Points = 1
						}

						if rawAnswers, ok := q["answers"].([]interface{}); ok {
							for _, ra := range rawAnswers {
								a, ok := ra.(map[string]interface{})
								if !ok {
									continue
								}
								question.Answers = append(question.Answers, Answer{
									ID:      getStr(a, "id"),
									Text:    getStr(a, "text"),
									Correct: getBool(a, "correct"),
								})
							}
						}

						if rawItems, ok := q["items"].([]interface{}); ok {
							for _, rit := range rawItems {
								it, ok := rit.(map[string]interface{})
								if !ok {
									continue
								}
								question.Items = append(question.Items, OrderItem{
									ID:   getStr(it, "id"),
									Text: getStr(it, "text"),
								})
							}
						}

						if rawOrder, ok := q["correct_order"].([]interface{}); ok {
							for _, ro := range rawOrder {
								if s, ok := ro.(string); ok {
									question.CorrectOrder = append(question.CorrectOrder, s)
								}
							}
						}

						if rawPS, ok := q["partial_scoring"].(map[string]interface{}); ok {
							question.PartialScoring = &PartialScoring{
								Enabled:       getBool(rawPS, "enabled"),
								AllowNegative: getBool(rawPS, "allow_negative"),
							}
						}

						if rawFb, ok := q["feedback"].(map[string]interface{}); ok {
							question.Feedback = Feedback{
								Wrong:   getStr(rawFb, "wrong"),
								Correct: getStr(rawFb, "correct"),
							}
							if rawRefs, ok := rawFb["source_refs"].([]interface{}); ok {
								for _, rawRef := range rawRefs {
									sr, ok := rawRef.(map[string]interface{})
									if !ok {
										continue
									}
									question.Feedback.SourceRefs = append(question.Feedback.SourceRefs, SourceRef{
										Course:   getStr(sr, "course"),
										Module:   getStr(sr, "module"),
										Anchor:   getStr(sr, "anchor"),
										Priority: getInt(sr, "priority"),
									})
								}
							}
						}

						mod.Questions = append(mod.Questions, question)
					}
				}
			}

			modules = append(modules, mod)
		}
	}

	return &Course{
		Slug:          slug,
		Title:         title,
		Description:   description,
		Category:      category,
		Difficulty:    difficulty,
		IsPublished:   !hidden,
		Prerequisites: coursePrereqs,
		Modules:       modules,
		Source:        sourceK8s(slug),
	}, nil
}

func getStr(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func getInt(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

func getBool(m map[string]interface{}, key string) bool {
	v, ok := m[key].(bool)
	if ok {
		return v
	}
	s, ok := m[key].(string)
	if ok {
		return s == "true" || s == "yes" || s == "1"
	}
	return false
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
