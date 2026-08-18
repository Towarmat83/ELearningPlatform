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

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/genesary/pupitre/course-service/internal/config"
	"github.com/genesary/pupitre/course-service/internal/content"
	coursedb "github.com/genesary/pupitre/course-service/internal/db"
	"github.com/genesary/pupitre/course-service/internal/handlers"
	"github.com/genesary/pupitre/course-service/internal/repository"
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
	ctrl.SetLogger(zapr.NewLogger(logger))

	zap.L().Info("starting course-service")

	// ── Configuration ────────────────────────────────────────────────────────
	zap.L().Info("loading configuration")

	cfg, warnings := config.Load()

	for _, w := range warnings {
		zap.L().Warn("configuration warning", zap.String("detail", w))
	}

	logConfig(cfg)

	ctx := context.Background()

	// ── Stores & state ───────────────────────────────────────────────────────
	zap.L().Info("initializing in-memory stores")

	store := content.NewStore()
	pathStore := content.NewPathStore()
	state := handlers.NewState(cfg, store, pathStore)

	// ── Database ──────────────────────────────────────────────────────────────
	zap.L().Info("connecting to database")

	connectDatabase(ctx, cfg, state)

	// ── K8s watchers ──────────────────────────────────────────────────────────
	zap.L().Info("starting Kubernetes watchers", zap.String("namespace", cfg.K8sNamespace))

	startWatchers(ctx, cfg, store, pathStore)

	zap.L().Info("Kubernetes watchers started")

	// ── HTTP router ───────────────────────────────────────────────────────────
	zap.L().Info("building HTTP router")

	r := handlers.BuildRouter(state, cfg, true)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	serveAndWait(srv)

	_ = logger.Sync()
}

// serveAndWait starts the HTTP server in a goroutine, blocks until SIGINT or
// SIGTERM is received, then performs a graceful shutdown.
func serveAndWait(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		zap.L().Info("API listening", zap.String("addr", srv.Addr))

		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			zap.L().Error("server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	<-quit
	zap.L().Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(shutCtx)
	if err != nil {
		zap.L().Error("forced shutdown", zap.Error(err))
	}

	zap.L().Info("server stopped")
}

// logConfig logs the effective (post-override) configuration values,
// omitting secrets and connection strings.
func logConfig(cfg *config.Config) {
	kubeMode := "in-cluster"
	if cfg.Kubeconfig != "" {
		kubeMode = cfg.Kubeconfig
	}

	zap.L().Info("configuration loaded",
		zap.Int("port", cfg.Port),
		zap.String("k8sNamespace", cfg.K8sNamespace),
		zap.String("kubeconfig", kubeMode),
		zap.Bool("databaseConfigured", cfg.DatabaseURL != ""),
		zap.String("userServiceURL", cfg.UserServiceURL),
		zap.String("checkerServiceURL", cfg.CheckerServiceURL),
		zap.Int("gitCacheTTLMin", cfg.GitCacheTTL),
		zap.Int("corsOriginsCount", len(cfg.CORSOrigins)),
	)
}

// connectDatabase connects to the database configured via cfg.DatabaseURL,
// runs pending migrations, and wires the resulting pool into state. It logs
// and continues without a database (disabling lab result tracking) if any
// step fails, since the database is optional for course-service.
//
// TODO: this only attempts the connection once, at startup. If Postgres
// isn't ready yet at that moment (e.g. its pod is still starting up
// alongside this one), lab result tracking stays disabled for the rest of
// this pod's lifetime instead of retrying/reconnecting later.
//
//nolint:godox // tracked follow-up, not a lint-worthy code smell: see the TODO body below
func connectDatabase(ctx context.Context, cfg *config.Config, state *handlers.State) {
	if cfg.DatabaseURL == "" {
		return
	}

	pool, err := coursedb.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	if err != nil {
		zap.L().Warn("database unavailable, lab result tracking disabled", zap.Error(err))

		return
	}

	err = coursedb.RunMigrations(ctx, pool)
	if err != nil {
		zap.L().Warn("db migration failed", zap.Error(err))

		return
	}

	state.LabChecks = repository.NewGormLabCheckRepository(pool)

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
