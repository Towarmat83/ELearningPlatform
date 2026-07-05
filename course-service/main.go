// Package main is the entry point for the course-service HTTP API, which
// serves course and module content backed by Kubernetes CRD-managed Git
// repositories.
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

const (
	// readTimeout bounds how long the server waits to read an entire request.
	readTimeout = 30 * time.Second
	// writeTimeout bounds how long the server waits to write a response.
	writeTimeout = 60 * time.Second
	// idleTimeout bounds how long the server keeps idle keep-alive connections.
	idleTimeout = 120 * time.Second
	// shutdownTimeout bounds how long graceful shutdown waits for requests.
	shutdownTimeout = 15 * time.Second
)

// main starts the course-service HTTP API server.
//
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

	state := handlers.NewState(cfg, store, pathStore)

	connectDatabase(ctx, cfg, state)
	startWatchers(ctx, cfg, store, pathStore)

	slog.Info("K8s CRD watchers started", "namespace", cfg.K8sNamespace)

	r := handlers.BuildRouter(state, cfg, true)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
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

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(shutCtx)
	if err != nil {
		slog.Error("forced shutdown", "err", err)
	}

	slog.Info("server stopped")
}

// connectDatabase connects to the database configured via cfg.DatabaseURL,
// runs pending migrations, and wires the resulting pool into state. It logs
// and continues without a database (disabling lab result tracking) if any
// step fails, since the database is optional for course-service.
func connectDatabase(ctx context.Context, cfg *config.Config, state *handlers.State) {
	if cfg.DatabaseURL == "" {
		return
	}

	pool, err := coursedb.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Warn("database unavailable, lab result tracking disabled", "err", err)

		return
	}

	err = coursedb.RunMigrations(ctx, pool, migrations.FS)
	if err != nil {
		slog.Warn("db migration failed", "err", err)

		return
	}

	state.DB = pool

	slog.Info("database connected, lab tracking enabled")
}

// startWatchers starts the Kubernetes CRD watchers that keep store and
// pathStore up to date. It exits the process if the required module watcher
// cannot be created or started; the optional path watcher only logs a
// warning on failure.
func startWatchers(ctx context.Context, cfg *config.Config, store *content.Store, pathStore *content.PathStore) {
	watcher, err := content.NewK8sWatcher(store, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		slog.Error("failed to create K8s watcher", "err", err)
		os.Exit(1)
	}

	err = watcher.Start(ctx)
	if err != nil {
		slog.Error("failed to start K8s watcher", "err", err)
		os.Exit(1)
	}

	pathWatcher, err := content.NewPathWatcher(pathStore, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		slog.Warn("failed to create Path watcher, paths disabled", "err", err)

		return
	}

	err = pathWatcher.Start(ctx)
	if err != nil {
		slog.Warn("failed to start Path watcher", "err", err)
	}
}
