// Package main is the entry point for the course-service HTTP API, which
// serves course and module content backed by Kubernetes CRD-managed Git
// repositories.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/course-service/internal/config"
	"github.com/genesary/pupitre/course-service/internal/content"
	coursedb "github.com/genesary/pupitre/course-service/internal/db"
	"github.com/genesary/pupitre/course-service/internal/handlers"
	"github.com/genesary/pupitre/course-service/migrations"
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
	logger := initLogger()
	zap.ReplaceGlobals(logger)

	cfg := config.Load()

	ctx := context.Background()

	// Content store — populated from K8s CRD watcher
	store := content.NewStore()
	pathStore := content.NewPathStore()

	state := handlers.NewState(cfg, store, pathStore)

	connectDatabase(ctx, cfg, state)
	startWatchers(ctx, cfg, store, pathStore)

	zap.L().Info("K8s CRD watchers started", zap.String("namespace", cfg.K8sNamespace))

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
		zap.L().Info("API listening", zap.String("addr", addr))

		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			zap.L().Error("server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	<-quit
	zap.L().Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(shutCtx)
	if err != nil {
		zap.L().Error("forced shutdown", zap.Error(err))
	}

	zap.L().Info("server stopped")

	_ = logger.Sync()
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
		zap.L().Warn("database unavailable, lab result tracking disabled", zap.Error(err))

		return
	}

	err = coursedb.RunMigrations(ctx, pool, migrations.FS)
	if err != nil {
		zap.L().Warn("db migration failed", zap.Error(err))

		return
	}

	state.DB = pool

	zap.L().Info("database connected, lab tracking enabled")
}

// startWatchers starts the Kubernetes CRD watchers that keep store and
// pathStore up to date. It exits the process if the required module watcher
// cannot be created or started; the optional path watcher only logs a
// warning on failure.
func startWatchers(ctx context.Context, cfg *config.Config, store *content.Store, pathStore *content.PathStore) {
	watcher, err := content.NewK8sWatcher(store, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		zap.L().Error("failed to create K8s watcher", zap.Error(err))
		os.Exit(1)
	}

	err = watcher.Start(ctx)
	if err != nil {
		zap.L().Error("failed to start K8s watcher", zap.Error(err))
		os.Exit(1)
	}

	pathWatcher, err := content.NewPathWatcher(pathStore, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		zap.L().Warn("failed to create Path watcher, paths disabled", zap.Error(err))

		return
	}

	err = pathWatcher.Start(ctx)
	if err != nil {
		zap.L().Warn("failed to start Path watcher", zap.Error(err))
	}
}

// initLogger builds a zap logger driven by two environment variables:
//   - LOG_LEVEL  — debug/info/warn/error (default: info)
//   - LOG_FORMAT — json (default) or text (human-readable, for local dev)
func initLogger() *zap.Logger {
	level := zap.NewAtomicLevel()
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		_ = level.UnmarshalText([]byte(lvl))
	}

	var cfg zap.Config
	if os.Getenv("LOG_FORMAT") == "text" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Level = level

	return zap.Must(cfg.Build())
}
