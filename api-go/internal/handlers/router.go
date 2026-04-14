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
// Set withLogger=true in production to enable the request log middleware.
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

	// Public course reads
	r.Get("/api/courses", s.ListCourses)
	r.Get("/api/courses/{id}", s.GetCourse)

	// WebSocket terminal — auth via ?token= query param, no JWT middleware
	r.Get("/ws/courses/{course_id}/labs/{lab_id}/terminal", s.TerminalWS)

	// ── Authenticated ────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(authMW)

		r.Get("/api/auth/me", s.Me)
		r.Put("/api/auth/profile", s.UpdateProfile)
		r.Put("/api/auth/password", s.ChangePassword)

		r.Post("/api/courses", s.CreateCourse)
		r.Put("/api/courses/{id}", s.UpdateCourse)
		r.Delete("/api/courses/{id}", s.DeleteCourse)
		r.Post("/api/courses/{id}/enroll", s.Enroll)
		r.Delete("/api/courses/{id}/unenroll", s.Unenroll)
		r.Get("/api/my/courses", s.MyCourses)
		r.Get("/api/courses/{id}/leaderboard", s.CourseLeaderboard)

		r.Get("/api/courses/{course_id}/labs", s.ListLabs)
		r.Post("/api/courses/{course_id}/labs", s.CreateLab)
		r.Get("/api/courses/{course_id}/labs/{lab_id}", s.GetLab)
		r.Put("/api/courses/{course_id}/labs/{lab_id}", s.UpdateLab)
		r.Delete("/api/courses/{course_id}/labs/{lab_id}", s.DeleteLab)

		r.Post("/api/courses/{course_id}/labs/{lab_id}/submit", s.SubmitLab)
		r.Get("/api/courses/{course_id}/labs/{lab_id}/submissions", s.MySubmissions)
		r.Get("/api/courses/{course_id}/progress", s.MyProgress)

		r.Post("/api/courses/{course_id}/labs/{lab_id}/instance", s.StartInstance)
		r.Get("/api/courses/{course_id}/labs/{lab_id}/instance", s.GetInstance)
		r.Delete("/api/courses/{course_id}/labs/{lab_id}/instance", s.StopInstance)
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
		r.Get("/api/admin/courses/{course_id}/monitoring", s.AdminCourseMonitoring)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}", s.AdminGetLab)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}/submissions", s.AdminLabSubmissions)
		r.Get("/api/admin/courses/{course_id}/labs/{lab_id}/stats", s.AdminLabStats)

		r.Get("/api/admin/courses/{course_id}/enrollments", s.AdminListEnrollments)
		r.Post("/api/admin/courses/{course_id}/enrollments", s.AdminEnrollUser)
		r.Delete("/api/admin/courses/{course_id}/enrollments/{user_id}", s.AdminUnenrollUser)
	})

	return r
}
