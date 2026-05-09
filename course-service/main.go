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
	"github.com/elearning/course-service/internal/handlers"
)

func main() {
	cfg := config.Load()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ctx := context.Background()

	// Content store — populated from K8s CRD watcher
	store := content.NewStore()

	watcher, err := content.NewK8sWatcher(store, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		slog.Error("failed to create K8s watcher", "err", err)
		os.Exit(1)
	}

	if err := watcher.Start(ctx); err != nil {
		slog.Error("failed to start K8s watcher", "err", err)
		os.Exit(1)
	}
	slog.Info("K8s CRD watcher started", "namespace", cfg.K8sNamespace)

	s := &handlers.State{
		Config:         cfg,
		Content:        store,
		UserServiceURL: cfg.UserServiceURL,
	}

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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
