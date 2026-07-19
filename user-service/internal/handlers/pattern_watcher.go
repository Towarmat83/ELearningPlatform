package handlers

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"

	patternv1 "github.com/genesary/pupitre/user-service/api/v1"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// PatternWatcher watches MarkdownPattern CRDs and syncs them into the DB.
// It uses controller-runtime's informer-backed cache, which relists on
// watch errors/reconnects automatically instead of only once at startup.
type PatternWatcher struct {
	patterns repository.PatternRepository
	cache    crcache.Cache
}

// NewPatternWatcher builds a PatternWatcher for the given Kubernetes cluster.
func NewPatternWatcher(patterns repository.PatternRepository, kubeconfig, namespace string) (*PatternWatcher, error) {
	cfg, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	scheme := runtime.NewScheme()

	err = patternv1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("register scheme: %w", err)
	}

	patternCache, err := crcache.New(cfg, crcache.Options{
		Scheme:            scheme,
		DefaultNamespaces: map[string]crcache.Config{namespace: {}},
	})
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	return &PatternWatcher{patterns: patterns, cache: patternCache}, nil
}

// Start begins watching MarkdownPattern CRDs until ctx is cancelled.
func (w *PatternWatcher) Start(ctx context.Context) error {
	informer, err := w.cache.GetInformer(ctx, &patternv1.MarkdownPattern{})
	if err != nil {
		return fmt.Errorf("get informer: %w", err)
	}

	_, err = informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { w.upsert(ctx, obj) },
		UpdateFunc: func(_, obj any) { w.upsert(ctx, obj) },
		DeleteFunc: func(obj any) { w.delete(ctx, obj) },
	})
	if err != nil {
		return fmt.Errorf("add event handler: %w", err)
	}

	go func() {
		err := w.cache.Start(ctx)
		if err != nil {
			zap.L().Error("pattern CRD cache stopped", zap.Error(err))
		}
	}()

	if !w.cache.WaitForCacheSync(ctx) {
		return errors.New("cache sync failed")
	}

	zap.L().Info("markdown patterns synced from CRDs")

	return nil
}

// upsert writes a MarkdownPattern CRD's spec into the markdown_patterns table.
func (w *PatternWatcher) upsert(ctx context.Context, obj any) {
	pattern, ok := obj.(*patternv1.MarkdownPattern)
	if !ok {
		return
	}

	spec := pattern.Spec

	name := spec.Name
	if name == "" {
		name = pattern.Name
	}

	label := spec.Label
	if label == "" {
		label = name
	}

	scope := spec.Scope
	if scope == "" {
		scope = patternsGlobalScope
	}

	err := w.patterns.UpsertFromCRD(ctx, name, label, spec.Description, spec.HTML, spec.CSS, spec.JS, scope)
	if err != nil {
		zap.L().Error("failed to upsert pattern from CRD", zap.String("name", name), zap.Error(err))

		return
	}

	zap.L().Info("pattern upserted from CRD", zap.String("name", name), zap.String("scope", scope))
}

// delete removes the markdown_patterns row for a deleted MarkdownPattern CRD.
func (w *PatternWatcher) delete(ctx context.Context, obj any) {
	if final, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = final.Obj
	}

	pattern, ok := obj.(*patternv1.MarkdownPattern)
	if !ok {
		return
	}

	name := pattern.Spec.Name
	if name == "" {
		name = pattern.Name
	}

	scope := pattern.Spec.Scope
	if scope == "" {
		scope = patternsGlobalScope
	}

	err := w.patterns.DeleteFromCRD(ctx, name, scope)
	if err != nil {
		zap.L().Error("failed to delete pattern from CRD", zap.String("name", name), zap.Error(err))

		return
	}

	zap.L().Info("pattern deleted from CRD", zap.String("name", name))
}

// buildRestConfig loads a Kubernetes REST config from kubeconfig, or from the
// in-cluster service account when kubeconfig is empty.
func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("building rest config from kubeconfig: %w", err)
		}

		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("building in-cluster rest config: %w", err)
	}

	return cfg, nil
}
