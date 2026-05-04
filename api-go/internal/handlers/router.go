package handlers

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elearning/api-go/internal/config"
	"github.com/elearning/api-go/internal/metrics"
	apimiddleware "github.com/elearning/api-go/internal/middleware"
)

// BuildRouter assembles the chi router with all routes and middleware.
func BuildRouter(s *State, cfg *config.Config, pool *pgxpool.Pool, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(pool, cfg.JWTSecret)
	adminMW := apimiddleware.Admin(pool, cfg.JWTSecret)

	corsOptions := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	if withLogger {
		r.Use(chiMiddleware.Logger)
	}
	r.Use(chiMiddleware.Recoverer)
	r.Use(corsOptions.Handler)

	// ── Public ──────────────────────────────────────────────────────────────────

	r.Get("/health", s.Health)
	r.Get("/metrics", metrics.Handler())

	r.Get("/api/settings/public", s.PublicSettings)

	r.Post("/api/auth/register", s.Register)
	r.Post("/api/auth/login", s.Login)

	r.Get("/api/auth/oauth/providers", s.ListProviders)
	r.Get("/api/auth/oauth/{provider}/authorize", s.OAuthAuthorize)
	r.Post("/api/auth/oauth/callback", s.OAuthCallback)

	// Public course reads (slug-based)
	r.Get("/api/courses", s.ListCourses)
	r.Get("/api/courses/{slug}", s.GetCourse)

	// Static uploads
	r.Get("/uploads/{filename}", s.ServeUpload)

	// ── Authenticated ────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(authMW)

		r.Get("/api/auth/me", s.Me)
		r.Put("/api/auth/profile", s.UpdateProfile)
		r.Put("/api/auth/password", s.ChangePassword)

		// Course enrollment (slug-based)
		r.Post("/api/courses/{slug}/enroll", s.Enroll)
		r.Delete("/api/courses/{slug}/unenroll", s.Unenroll)

		// Lessons (slug-based)
		r.Get("/api/courses/{slug}/lessons", s.ListLessons)
		r.Get("/api/courses/{slug}/lessons/{lesson_slug}", s.GetLesson)
		r.Post("/api/courses/{slug}/lessons/{lesson_slug}/complete", s.MarkLessonComplete)

		// My courses + git repos
		r.Get("/api/my/courses", s.MyCourses)
		r.Get("/api/my/repos", s.ListRepos)
		r.Post("/api/my/repos", s.AddRepo)
		r.Delete("/api/my/repos/{id}", s.DeleteRepo)
		r.Post("/api/my/repos/{id}/sync", s.SyncRepo)
	})

	// ── Admin ─────────────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(adminMW)

		r.Get("/api/admin/settings", s.GetSettings)
		r.Put("/api/admin/settings", s.UpdateSettings)

		r.Get("/api/admin/stats", s.AdminStats)

		r.Get("/api/admin/users", s.ListUsers)
		r.Get("/api/admin/users/{user_id}", s.GetUser)
		r.Put("/api/admin/users/{user_id}", s.UpdateUser)
		r.Delete("/api/admin/users/{user_id}", s.DeleteUser)

		r.Get("/api/admin/courses", s.AdminListCourses)

		r.Post("/api/admin/uploads/video", s.UploadVideo)
	})

	return r
}
