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

	dockerclient "github.com/docker/docker/client"

	"github.com/elearning/api-go/internal/config"
	"github.com/elearning/api-go/internal/content"
	"github.com/elearning/api-go/internal/db"
	"github.com/elearning/api-go/internal/handlers"
	"github.com/elearning/api-go/migrations"
)

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

	// Docker (optional — needed for interactive labs)
	var docker *dockerclient.Client
	if d, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	); err != nil {
		slog.Warn("Docker not available — interactive labs disabled", "err", err)
	} else if _, err := d.Ping(ctx); err != nil {
		slog.Warn("Docker daemon unreachable — interactive labs disabled", "err", err)
		d.Close()
	} else {
		docker = d
		defer docker.Close()
		slog.Info("Docker daemon connected — interactive labs enabled")
	}

	// Content store — load local courses on startup
	store := content.NewStore()
	if err := store.LoadDir(cfg.CoursesDir, "local"); err != nil {
		slog.Warn("could not load courses directory", "dir", cfg.CoursesDir, "err", err)
	}

	// Seed course_settings for local courses (respects is_published from YAML; never overwrites)
	for _, c := range store.All() {
		if c.Source == "local" {
			pool.Exec(ctx, `
				INSERT INTO course_settings (course_slug, is_published, auto_enroll, source)
				VALUES ($1, $2, false, 'local') ON CONFLICT (course_slug) DO NOTHING`,
				c.Slug, c.IsPublished)
		}
	}

	s := &handlers.State{
		Pool:    pool,
		Config:  cfg,
		Docker:  docker,
		Content: store,
	}

	r := handlers.BuildRouter(s, cfg, pool, true)

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
