package handlers

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

	"github.com/elearning/user-service/internal/db"
)

var patternGVR = schema.GroupVersionResource{
	Group:    "elearning.example.com",
	Version:  "v1",
	Resource: "markdownpatterns",
}

// PatternWatcher watches MarkdownPattern CRDs and syncs them into the DB.
type PatternWatcher struct {
	pool      db.Pool
	client    dynamic.NamespaceableResourceInterface
	namespace string
}

func NewPatternWatcher(pool db.Pool, kubeconfig, namespace string) (*PatternWatcher, error) {
	cfg, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &PatternWatcher{
		pool:      pool,
		client:    dyn.Resource(patternGVR),
		namespace: namespace,
	}, nil
}

func (w *PatternWatcher) Start(ctx context.Context) error {
	if err := w.syncAll(ctx); err != nil {
		return fmt.Errorf("initial sync: %w", err)
	}
	go w.loop(ctx)
	return nil
}

func (w *PatternWatcher) syncAll(ctx context.Context) error {
	list, err := w.client.Namespace(w.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range list.Items {
		w.upsert(ctx, &list.Items[i])
	}
	slog.Info("markdown patterns synced from CRDs", "count", len(list.Items))
	return nil
}

func (w *PatternWatcher) loop(ctx context.Context) {
	backoff := 2 * time.Second
	const maxBackoff = 60 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		timeout := int64(600)
		watcher, err := w.client.Namespace(w.namespace).Watch(ctx, metav1.ListOptions{TimeoutSeconds: &timeout})
		if err != nil {
			slog.Error("pattern CRD watch error, retrying", "err", err, "backoff", backoff)
			time.Sleep(backoff)
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = 2 * time.Second
		for event := range watcher.ResultChan() {
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				w.upsert(ctx, obj)
			case watch.Deleted:
				w.delete(ctx, obj)
			}
		}
	}
}

func (w *PatternWatcher) upsert(ctx context.Context, obj *unstructured.Unstructured) {
	spec, ok := obj.UnstructuredContent()["spec"].(map[string]interface{})
	if !ok {
		return
	}
	name := getString(spec, "name")
	if name == "" {
		name = obj.GetName()
	}
	label := getString(spec, "label")
	if label == "" {
		label = name
	}
	scope := getString(spec, "scope")
	if scope == "" {
		scope = "global"
	}

	_, err := w.pool.Exec(ctx, `
		INSERT INTO markdown_patterns (name, label, description, html, css, js, scope, from_config)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
		ON CONFLICT (name, scope) DO UPDATE SET
			label       = EXCLUDED.label,
			description = EXCLUDED.description,
			html        = EXCLUDED.html,
			css         = EXCLUDED.css,
			js          = EXCLUDED.js,
			from_config = TRUE,
			updated_at  = NOW()
	`,
		name,
		label,
		getString(spec, "description"),
		getString(spec, "html"),
		getString(spec, "css"),
		getString(spec, "js"),
		scope,
	)
	if err != nil {
		slog.Error("failed to upsert pattern from CRD", "name", name, "err", err)
		return
	}
	slog.Info("pattern upserted from CRD", "name", name, "scope", scope)
}

func (w *PatternWatcher) delete(ctx context.Context, obj *unstructured.Unstructured) {
	spec, _ := obj.UnstructuredContent()["spec"].(map[string]interface{})
	name := getString(spec, "name")
	if name == "" {
		name = obj.GetName()
	}
	scope := getString(spec, "scope")
	if scope == "" {
		scope = "global"
	}
	_, err := w.pool.Exec(ctx, `DELETE FROM markdown_patterns WHERE name = $1 AND scope = $2 AND from_config = TRUE`, name, scope)
	if err != nil {
		slog.Error("failed to delete pattern from CRD", "name", name, "err", err)
		return
	}
	slog.Info("pattern deleted from CRD", "name", name)
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
