package content

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

var patternGVR = schema.GroupVersionResource{
	Group:    "elearning.example.com",
	Version:  "v1",
	Resource: "markdownpatterns",
}

// Pattern holds the definition of a markdown pattern stored as a CRD.
type Pattern struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Scope  string `json:"scope"`
	HTML   string `json:"html"`
	CSS    string `json:"css"`
	JS     string `json:"js"`
	Source string `json:"source"` // always "crd"
}

// PatternStore is an in-memory cache of MarkdownPattern CRs.
type PatternStore struct {
	mu       sync.RWMutex
	patterns map[string]Pattern // key: name+"/"+scope
}

func NewPatternStore() *PatternStore {
	return &PatternStore{patterns: make(map[string]Pattern)}
}

func (s *PatternStore) key(name, scope string) string { return name + "/" + scope }

func (s *PatternStore) Put(p Pattern) {
	s.mu.Lock()
	s.patterns[s.key(p.Name, p.Scope)] = p
	s.mu.Unlock()
}

func (s *PatternStore) Delete(name, scope string) {
	s.mu.Lock()
	delete(s.patterns, s.key(name, scope))
	s.mu.Unlock()
}

// List returns all patterns, optionally filtered by scope.
// When scope is non-empty, returns global patterns + scope-specific ones.
func (s *PatternStore) List(scope string) []Pattern {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Pattern, 0, len(s.patterns))
	for _, p := range s.patterns {
		if scope == "" || p.Scope == "global" || p.Scope == scope {
			out = append(out, p)
		}
	}
	return out
}

// PatternWatcher watches MarkdownPattern CRDs and keeps a PatternStore in sync.
type PatternWatcher struct {
	store     *PatternStore
	client    dynamic.NamespaceableResourceInterface
	namespace string
}

func NewPatternWatcher(store *PatternStore, kubeconfig, namespace string) (*PatternWatcher, error) {
	cfg, err := restConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &PatternWatcher{
		store:     store,
		client:    dyn.Resource(patternGVR),
		namespace: namespace,
	}, nil
}

func (w *PatternWatcher) Start(ctx context.Context) {
	if err := w.syncAll(ctx); err != nil {
		slog.Warn("MarkdownPattern initial sync failed", "err", err)
	}
	go w.watchLoop(ctx)
}

func (w *PatternWatcher) syncAll(ctx context.Context) error {
	list, err := w.client.Namespace(w.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range list.Items {
		w.upsert(&list.Items[i])
	}
	slog.Info("MarkdownPattern CRDs loaded", "count", len(list.Items))
	return nil
}

func (w *PatternWatcher) watchLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		timeout := int64(600)
		watcher, err := w.client.Namespace(w.namespace).Watch(ctx, metav1.ListOptions{TimeoutSeconds: &timeout})
		if err != nil {
			slog.Error("MarkdownPattern watch error, retrying", "err", err, "backoff", backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
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

func (w *PatternWatcher) upsert(obj *unstructured.Unstructured) {
	p := crdToPattern(obj)
	if p == nil {
		return
	}
	w.store.Put(*p)
	slog.Debug("MarkdownPattern upserted", "name", p.Name, "scope", p.Scope)
}

func (w *PatternWatcher) remove(obj *unstructured.Unstructured) {
	p := crdToPattern(obj)
	if p == nil {
		return
	}
	w.store.Delete(p.Name, p.Scope)
	slog.Debug("MarkdownPattern removed", "name", p.Name, "scope", p.Scope)
}

func crdToPattern(obj *unstructured.Unstructured) *Pattern {
	spec, ok := obj.UnstructuredContent()["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	name, _ := spec["name"].(string)
	if name == "" {
		name = obj.GetName()
	}
	html, _ := spec["html"].(string)
	if html == "" {
		return nil
	}
	scope, _ := spec["scope"].(string)
	if scope == "" {
		scope = "global"
	}
	label, _ := spec["label"].(string)
	css, _ := spec["css"].(string)
	js, _ := spec["js"].(string)
	return &Pattern{Name: name, Label: label, Scope: scope, HTML: html, CSS: css, JS: js, Source: "crd"}
}
