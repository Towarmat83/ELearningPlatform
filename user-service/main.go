// Package main is the entry point for user-service, the authentication,
// profile, enrollment, lesson-progress, and platform-settings HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/config"
	"github.com/genesary/pupitre/user-service/internal/db"
	"github.com/genesary/pupitre/user-service/internal/handlers"
	"github.com/genesary/pupitre/user-service/internal/repository"
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
	// seedMockDataValue is the SEED_MOCK_DATA env var value that enables
	// seeding mock users and enrollments (dev/demo only).
	seedMockDataValue = "true"
)

// main is the entry point: it delegates to run and, on failure, logs the
// error and exits with a non-zero status.
//
// @title          User Service API
// @version        1.0.0
// @description    User Service — auth, profiles, enrollments, lesson
// @description    progress, platform settings, and admin user management.
// @description    Internal API endpoints for the Course Service are under
// @description    `/internal/`.
// @host           localhost:8081
// @BasePath       /
// @securityDefinitions.apikey BearerAuth
// @in             header
// @name           Authorization.
func main() {
	logger := initLogger()
	zap.ReplaceGlobals(logger)
	ctrl.SetLogger(zapr.NewLogger(logger))

	err := run()
	if err != nil {
		zap.L().Error("fatal startup error", zap.Error(err))

		_ = logger.Sync()

		os.Exit(1)
	}

	_ = logger.Sync()
}

// run wires up the database, background watchers, and HTTP server, then
// blocks until a shutdown signal is received. Keeping this logic separate
// from main lets deferred cleanup (e.g. closing the database pool) run to
// completion instead of being skipped by a direct [os.Exit] call.
func run() error { //nolint:funlen // multiple boot phases each require their own guard clauses
	zap.L().Info("starting user-service")

	// ── Configuration ────────────────────────────────────────────────────────
	zap.L().Info("loading configuration")

	cfg, warnings := config.Load()

	for _, w := range warnings {
		zap.L().Warn("configuration warning", zap.String("detail", w))
	}

	kubeMode := "in-cluster"
	if cfg.Kubeconfig != "" {
		kubeMode = cfg.Kubeconfig
	}

	zap.L().Info("configuration loaded",
		zap.Int("port", cfg.Port),
		zap.String("k8sNamespace", cfg.K8sNamespace),
		zap.String("kubeconfig", kubeMode),
		zap.Bool("databaseConfigured", cfg.DatabaseURL != ""),
		zap.String("courseServiceURL", cfg.CourseServiceURL),
		zap.Bool("oidcEnabled", cfg.OIDC.Enabled),
		zap.Int("providersCount", len(cfg.Providers)),
		zap.Int("corsOriginsCount", len(cfg.CORSOrigins)),
	)

	ctx := context.Background()

	// ── Database ──────────────────────────────────────────────────────────────
	zap.L().Info("connecting to database")

	gdb, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("unwrap sql.DB: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	zap.L().Info("database connected")

	// ── Migrations & seed ─────────────────────────────────────────────────────
	zap.L().Info("running migrations and seeding")

	repos := repository.NewGormRepositories(gdb)

	err = seedDatabase(ctx, gdb, repos, cfg)
	if err != nil {
		return err
	}

	// ── Background watchers ───────────────────────────────────────────────────
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	if cfg.AdminPasswordFile != "" {
		zap.L().Info("watching admin password file", zap.String("path", cfg.AdminPasswordFile))
		go db.WatchAdminPassword(watchCtx, repos.Users, cfg.AdminPasswordFile)
	}

	// MarkdownPattern CRD watcher — syncs CRDs into the markdown_patterns
	// table. Disabled when K8sNamespace is empty (local dev without cluster).
	zap.L().Info("starting pattern CRD watcher")

	defer startPatternWatcher(ctx, cfg, repos.Patterns)()

	// ── HTTP router ───────────────────────────────────────────────────────────
	zap.L().Info("building HTTP router")

	state := &handlers.State{
		Repos:  repos,
		Config: cfg,
	}

	r := handlers.BuildRouter(state, cfg, true)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	return serve(srv)
}

// seedDatabase runs migrations and seeds the default admin user, OIDC
// settings, and (if enabled) mock demo data.
func seedDatabase(ctx context.Context, gdb *gorm.DB, repos *repository.Repositories, cfg *config.Config) error {
	err := db.RunMigrations(ctx, gdb)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	zap.L().Info("migrations applied")

	// Seed default admin (idempotent — safe to run on every startup).
	err = db.SeedAdmin(ctx, repos.Users, cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	// Bootstrap OIDC settings from deploy-time config (no-op unless OIDC_ENABLED).
	err = db.SeedOIDC(ctx, repos.Settings, cfg)
	if err != nil {
		return fmt.Errorf("seed OIDC settings: %w", err)
	}

	// Seed mock users and enrollments (dev/demo only).
	if os.Getenv("SEED_MOCK_DATA") == seedMockDataValue {
		err := db.SeedMockData(ctx, repos.Users, repos.Enrollments)
		if err != nil {
			zap.L().Error("failed to seed mock data", zap.Error(err))
		}
	}

	return nil
}

// startPatternWatcher starts the MarkdownPattern CRD watcher when
// cfg.K8sNamespace is configured. It returns a cancel function that the
// caller must defer to stop the watcher on shutdown; when the watcher is
// disabled or fails to start, the returned function is still safe to call.
func startPatternWatcher(ctx context.Context, cfg *config.Config, patterns repository.PatternRepository) context.CancelFunc {
	if cfg.K8sNamespace == "" {
		return func() {}
	}

	watchCtx, cancel := context.WithCancel(ctx)

	patternWatcher, err := handlers.NewPatternWatcher(patterns, cfg.Kubeconfig, cfg.K8sNamespace)
	if err != nil {
		zap.L().Warn("pattern CRD watcher disabled", zap.NamedError("reason", err))

		return cancel
	}

	err = patternWatcher.Start(watchCtx)
	if err != nil {
		zap.L().Warn("pattern CRD watcher failed to start", zap.Error(err))

		return cancel
	}

	zap.L().Info("pattern CRD watcher started", zap.String("namespace", cfg.K8sNamespace))

	return cancel
}

// serve starts srv in the background, blocks until an interrupt/terminate
// signal is received or the server exits unexpectedly, then gracefully
// shuts srv down.
func serve(srv *http.Server) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)

	go func() {
		zap.L().Info("API listening", zap.String("addr", srv.Addr))

		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("server error: %w", err)

			return
		}

		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-quit:
	}

	zap.L().Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(shutCtx)
	if err != nil {
		return fmt.Errorf("forced shutdown: %w", err)
	}

	zap.L().Info("server stopped")

	return nil
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
