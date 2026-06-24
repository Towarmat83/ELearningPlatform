package handlers

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/db"
	"github.com/elearning/user-service/internal/metrics"
	apimiddleware "github.com/elearning/user-service/internal/middleware"
)

func BuildRouter(s *State, cfg *config.Config, pool db.Pool, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(pool, cfg.JWTSecret)
	adminMW := apimiddleware.Admin(pool, cfg.JWTSecret)

	corsOptions := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
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

	r.Get("/api/auth/oidc/authorize", s.OIDCAuthorize)
	r.Post("/api/auth/oidc/callback", s.OIDCCallback)

	r.Post("/api/auth/ldap/login", s.LDAPLogin)

	// ── Authenticated ────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(authMW)

		r.Get("/api/auth/me", s.Me)
		r.Put("/api/auth/profile", s.UpdateProfile)
		r.Put("/api/auth/password", s.ChangePassword)

		r.Post("/api/courses/{slug}/enroll", s.Enroll)
		r.Delete("/api/courses/{slug}/unenroll", s.Unenroll)

		r.Post("/api/courses/{slug}/lessons/{lesson_slug}/complete", s.MarkLessonComplete)

		r.Get("/api/my/courses", s.MyCourses)
	})

	// ── Admin ─────────────────────────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(adminMW)

		r.Get("/api/admin/settings", s.GetSettings)
		r.Put("/api/admin/settings", s.UpdateSettings)

		r.Get("/api/admin/stats", s.AdminStats)
		r.Get("/api/admin/leaderboard", s.AdminLeaderboard)

		r.Get("/api/admin/users", s.ListUsers)
		r.Get("/api/admin/users/providers", s.ListAuthProviders)
		r.Get("/api/admin/users/search", s.SearchUsers)
		r.Get("/api/admin/users/{user_id}", s.GetUser)
		r.Put("/api/admin/users/{user_id}", s.UpdateUser)
		r.Delete("/api/admin/users/{user_id}", s.DeleteUser)

		r.Get("/api/admin/courses/{slug}/enrollments", s.ListCourseEnrollments)
		r.Post("/api/admin/courses/{slug}/enrollments", s.AdminEnrollUser)
		r.Get("/api/admin/courses/{slug}/enrollments/groups", s.AdminListGroupEnrollments)
		r.Post("/api/admin/courses/{slug}/enrollments/groups", s.AdminEnrollGroup)
		r.Delete("/api/admin/courses/{slug}/enrollments/groups/{group_id}", s.AdminUnenrollGroup)
		r.Delete("/api/admin/courses/{slug}/enrollments/{user_id}", s.AdminUnenrollUser)

		r.Post("/api/admin/sync-progress", s.SyncProgress)

		r.Get("/api/admin/groups", s.ListGroups)
		r.Post("/api/admin/groups", s.CreateGroup)
		r.Delete("/api/admin/groups/{group_id}", s.DeleteGroup)
		r.Get("/api/admin/groups/mappings", s.ListGroupMappings)
		r.Post("/api/admin/groups/mappings", s.UpsertGroupMapping)
		r.Delete("/api/admin/groups/mappings/{group_name}", s.DeleteGroupMapping)
	})

	// ── Patterns (authenticated) ─────────────────────────────────────────────────

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/api/patterns", s.ListPatterns)
		r.Get("/api/patterns/{id}", s.GetPattern)
	})

	r.Group(func(r chi.Router) {
		r.Use(adminMW)
		r.Post("/api/admin/patterns", s.CreatePattern)
		r.Put("/api/admin/patterns/{name}", s.UpdatePattern)
		r.Delete("/api/admin/patterns/{name}", s.DeletePattern)
	})

	// ── Internal API (for Course Service, no auth — rely on network policy) ──────

	r.Post("/internal/enrollments/auto", s.InternalAutoEnroll)
	r.Get("/internal/enrollments/check", s.InternalCheckEnrollment)
	r.Get("/internal/progress/viewed", s.InternalViewedLessons)
	r.Post("/internal/progress/complete", s.InternalMarkComplete)
	r.Post("/internal/progress/module", s.InternalRecordModuleProgress)
	r.Get("/internal/progress/modules", s.InternalGetModuleProgress)
	r.Get("/internal/progress/course-summary", s.InternalCourseSummary)

	return r
}
