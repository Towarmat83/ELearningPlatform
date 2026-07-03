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

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/db"
	"github.com/elearning/user-service/internal/handlers"
	"github.com/elearning/user-service/migrations"
)

// @title          User Service API
// @version        1.0.0
// @description    User Service — auth, profiles, enrollments, lesson progress, platform settings, and admin user management. Internal API endpoints for the Course Service are under `/internal/`.
// @host           localhost:8081
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

	// Database
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("database connected")

	// Migrations
	if err := db.RunMigrations(ctx, pool, migrations.FS); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}

	slog.Info("migrations applied")

	// Seed default admin (idempotent — safe to run on every startup).
	if err := db.SeedAdmin(ctx, pool, cfg.AdminPassword); err != nil {
		slog.Error("failed to seed admin user", "err", err)
		os.Exit(1)
	}

	// Bootstrap OIDC settings from deploy-time config (no-op unless OIDC_ENABLED).
	if err := db.SeedOIDC(ctx, pool, cfg); err != nil {
		slog.Error("failed to seed OIDC settings", "err", err)
		os.Exit(1)
	}

	// Seed mock users and enrollments (dev/demo only).
	if os.Getenv("SEED_MOCK_DATA") == "true" {
		err := db.SeedMockData(ctx, pool)
		if err != nil {
			slog.Error("failed to seed mock data", "err", err)
		}
	}

	// Watch the admin password file for live rotation (no pod restart needed).
	// Active only when ADMIN_PASSWORD_FILE is set (i.e. K8s Secret volume mount).
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	if cfg.AdminPasswordFile != "" {
		go db.WatchAdminPassword(watchCtx, pool, cfg.AdminPasswordFile)
	}

	// MarkdownPattern CRD watcher — syncs CRDs into the markdown_patterns table.
	// Disabled when K8S_NAMESPACE is empty (local dev without cluster).
	if cfg.K8sNamespace != "" {
		watchCtxPat, watchCancelPat := context.WithCancel(ctx)
		defer watchCancelPat()

		pw, err := handlers.NewPatternWatcher(db.NewAdapter(pool), cfg.Kubeconfig, cfg.K8sNamespace)
		if err != nil {
			slog.Warn("pattern CRD watcher disabled", "reason", err)
		} else {
			err := pw.Start(watchCtxPat)
			if err != nil {
				slog.Warn("pattern CRD watcher failed to start", "err", err)
			} else {
				slog.Info("pattern CRD watcher started", "namespace", cfg.K8sNamespace)
			}
		}
	}

	s := &handlers.State{
		Pool:   db.NewAdapter(pool),
		Config: cfg,
	}

	r := handlers.BuildRouter(s, cfg, db.NewAdapter(pool), true)

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
