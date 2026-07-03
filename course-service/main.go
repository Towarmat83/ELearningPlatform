package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/content"
	coursedb "github.com/elearning/course-service/internal/db"
	"github.com/elearning/course-service/internal/handlers"
	"github.com/elearning/course-service/migrations"
)

// @title          Course Service API
// @version        1.0.0
// @description    Stateless micro-service for course/module content delivery.
// @host           localhost:8082
// @BasePath       /
// @securityDefinitions.apikey BearerAuth
// @in             header
// @name           Authorization.
func main() {
	cfg := config.Load()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ctx := context.Background()

	// Content store — populated from K8s CRD watcher
	store := content.NewStore()
	pathStore := content.NewPathStore()

	s := handlers.NewState(cfg, store, pathStore)

	if cfg.DatabaseURL != "" {
		pool, err := coursedb.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Warn("database unavailable, lab result tracking disabled", "err", err)
		} else {
			err := coursedb.RunMigrations(ctx, pool, migrations.FS)
			if err != nil {
				slog.Warn("db migration failed", "err", err)
			} else {
				s.DB = pool

				slog.Info("database connected, lab tracking enabled")
			}
		}
	}

	watcher, err := content.NewK8sWatcher(store, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		slog.Error("failed to create K8s watcher", "err", err)
		os.Exit(1)
	}

	if err := watcher.Start(ctx); err != nil {
		slog.Error("failed to start K8s watcher", "err", err)
		os.Exit(1)
	}

	pathWatcher, err := content.NewPathWatcher(pathStore, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		slog.Warn("failed to create Path watcher, paths disabled", "err", err)
	} else {
		err := pathWatcher.Start(ctx)
		if err != nil {
			slog.Warn("failed to start Path watcher", "err", err)
		}
	}

	slog.Info("K8s CRD watchers started", "namespace", cfg.K8sNamespace)

	r := handlers.BuildRouter(s, cfg, true)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("API listening", "addr", addr)

		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("forced shutdown", "err", err)
	}

	slog.Info("server stopped")
}
