package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	dockerclient "github.com/docker/docker/client"

	"github.com/elearning/api-go/internal/config"
	"github.com/elearning/api-go/internal/db"
	"github.com/elearning/api-go/internal/handlers"
	apimiddleware "github.com/elearning/api-go/internal/middleware"
	"github.com/elearning/api-go/internal/metrics"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	cfg := config.Load()

	// Structured logger
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Database
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")

	// Migrations
	if err := db.RunMigrations(ctx, pool, migrationsFS); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	// Docker (optional — needed for interactive labs)
	var docker *dockerclient.Client
	if d, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation()); err != nil {
		slog.Warn("Docker not available — interactive labs disabled", "err", err)
	} else if _, err := d.Ping(ctx); err != nil {
		slog.Warn("Docker daemon unreachable — interactive labs disabled", "err", err)
		d.Close()
	} else {
		docker = d
		slog.Info("Docker daemon connected — interactive labs enabled")
		defer docker.Close()
	}

	// Handler state
	s := &handlers.State{
		Pool:   pool,
		Config: cfg,
		Docker: docker,
	}

	// Auth middleware factories
	authMiddleware := apimiddleware.Auth(pool, cfg.JWTSecret)
	adminMiddleware := apimiddleware.Admin(pool, cfg.JWTSecret)

	// CORS
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware.Handler)

	// ── Public ──────────────────────────────────────────────────────────────────

	r.Get("/health", s.Health)
	r.Get("/metrics", metrics.Handler())

	r.Get("/api/settings/public", s.PublicSettings)

	// Auth (unauthenticated)
	r.Post("/api/auth/register", s.Register)
	r.Post("/api/auth/login", s.Login)

	// OAuth SSO (unauthenticated)
	r.Get("/api/auth/oauth/providers", s.ListProviders)
	r.Get("/api/auth/oauth/{provider}/authorize", s.OAuthAuthorize)
	r.Post("/api/auth/oauth/callback", s.OAuthCallback)

	// Public course reads
	r.Get("/api/courses", s.ListCourses)
	r.Get("/api/courses/{id}", s.GetCourse)

	// WebSocket terminal — auth via ?token= query param
	r.Get("/ws/courses/{course_id}/labs/{lab_id}/terminal", s.TerminalWS)

	// ── Authenticated ────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		// Profile
		r.Get("/api/auth/me", s.Me)
		r.Put("/api/auth/profile", s.UpdateProfile)
		r.Put("/api/auth/password", s.ChangePassword)

		// Course mutations
		r.Post("/api/courses", s.CreateCourse)
		r.Put("/api/courses/{id}", s.UpdateCourse)
		r.Delete("/api/courses/{id}", s.DeleteCourse)
		r.Post("/api/courses/{id}/enroll", s.Enroll)
		r.Delete("/api/courses/{id}/unenroll", s.Unenroll)
		r.Get("/api/my/courses", s.MyCourses)
		r.Get("/api/courses/{id}/leaderboard", s.CourseLeaderboard)

		// Labs
		r.Get("/api/courses/{course_id}/labs", s.ListLabs)
		r.Post("/api/courses/{course_id}/labs", s.CreateLab)
		r.Get("/api/courses/{course_id}/labs/{lab_id}", s.GetLab)
		r.Put("/api/courses/{course_id}/labs/{lab_id}", s.UpdateLab)
		r.Delete("/api/courses/{course_id}/labs/{lab_id}", s.DeleteLab)

		// Submissions & progress
		r.Post("/api/courses/{course_id}/labs/{lab_id}/submit", s.SubmitLab)
		r.Get("/api/courses/{course_id}/labs/{lab_id}/submissions", s.MySubmissions)
		r.Get("/api/courses/{course_id}/progress", s.MyProgress)

		// Interactive lab instances
		r.Post("/api/courses/{course_id}/labs/{lab_id}/instance", s.StartInstance)
		r.Get("/api/courses/{course_id}/labs/{lab_id}/instance", s.GetInstance)
		r.Delete("/api/courses/{course_id}/labs/{lab_id}/instance", s.StopInstance)
	})

	// ── Admin ─────────────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(adminMiddleware)

		// Settings
		r.Get("/api/admin/settings", s.GetSettings)
		r.Put("/api/admin/settings", s.UpdateSettings)

		// Global stats
		r.Get("/api/admin/stats", s.AdminStats)

		// User management
		r.Get("/api/admin/users", s.ListUsers)
		r.Get("/api/admin/users/{user_id}", s.GetUser)
		r.Put("/api/admin/users/{user_id}", s.UpdateUser)
		r.Delete("/api/admin/users/{user_id}", s.DeleteUser)

		// Course administration
		r.Get("/api/admin/courses", s.AdminListCourses)
		r.Get("/api/admin/courses/{course_id}/monitoring", s.AdminCourseMonitoring)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}", s.AdminGetLab)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}/submissions", s.AdminLabSubmissions)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}/stats", s.AdminLabStats)

		// Enrollment management
		r.Get("/api/admin/courses/{course_id}/enrollments", s.AdminListEnrollments)
		r.Post("/api/admin/courses/{course_id}/enrollments", s.AdminEnrollUser)
		r.Delete("/api/admin/courses/{course_id}/enrollments/{user_id}", s.AdminUnenrollUser)
	})

	// ── Server ─────────────────────────────────────────────────────────────────────

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
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
