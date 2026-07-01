package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"

	patternv1 "github.com/elearning/user-service/api/v1"
	"github.com/elearning/user-service/internal/db"
)

// PatternWatcher watches MarkdownPattern CRDs and syncs them into the DB.
// It uses controller-runtime's informer-backed cache, which relists on
// watch errors/reconnects automatically instead of only once at startup.
type PatternWatcher struct {
	pool  db.Pool
	cache crcache.Cache
}

func NewPatternWatcher(pool db.Pool, kubeconfig, namespace string) (*PatternWatcher, error) {
	cfg, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := patternv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	c, err := crcache.New(cfg, crcache.Options{
		Scheme:            scheme,
		DefaultNamespaces: map[string]crcache.Config{namespace: {}},
	})
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	return &PatternWatcher{pool: pool, cache: c}, nil
}

func (w *PatternWatcher) Start(ctx context.Context) error {
	informer, err := w.cache.GetInformer(ctx, &patternv1.MarkdownPattern{})
	if err != nil {
		return fmt.Errorf("get informer: %w", err)
	}

	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.upsert(context.Background(), obj) },
		UpdateFunc: func(_, obj interface{}) { w.upsert(context.Background(), obj) },
		DeleteFunc: func(obj interface{}) { w.delete(context.Background(), obj) },
	}); err != nil {
		return fmt.Errorf("add event handler: %w", err)
	}

	go func() {
		if err := w.cache.Start(ctx); err != nil {
			slog.Error("pattern CRD cache stopped", "err", err)
		}
	}()

	if !w.cache.WaitForCacheSync(ctx) {
		return fmt.Errorf("cache sync failed")
	}
	slog.Info("markdown patterns synced from CRDs")
	return nil
}

func (w *PatternWatcher) upsert(ctx context.Context, obj interface{}) {
	cr, ok := obj.(*patternv1.MarkdownPattern)
	if !ok {
		return
	}
	spec := cr.Spec
	name := spec.Name
	if name == "" {
		name = cr.Name
	}
	label := spec.Label
	if label == "" {
		label = name
	}
	scope := spec.Scope
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
		spec.Description,
		spec.HTML,
		spec.CSS,
		spec.JS,
		scope,
	)
	if err != nil {
		slog.Error("failed to upsert pattern from CRD", "name", name, "err", err)
		return
	}
	slog.Info("pattern upserted from CRD", "name", name, "scope", scope)
}

func (w *PatternWatcher) delete(ctx context.Context, obj interface{}) {
	if final, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = final.Obj
	}
	cr, ok := obj.(*patternv1.MarkdownPattern)
	if !ok {
		return
	}
	name := cr.Spec.Name
	if name == "" {
		name = cr.Name
	}
	scope := cr.Spec.Scope
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

func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
