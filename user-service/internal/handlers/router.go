package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/db"
	"github.com/elearning/user-service/internal/metrics"
	apimiddleware "github.com/elearning/user-service/internal/middleware"
)

// corsMaxAgeSeconds bounds CORS preflight cache duration, in seconds.
const corsMaxAgeSeconds = 300

// BuildRouter assembles the HTTP router, wiring public, authenticated, admin,
// pattern, and internal routes with their respective middleware.
func BuildRouter(state *State, cfg *config.Config, pool db.Pool, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(pool, cfg.JWTSecret)
	adminMW := apimiddleware.Admin(pool, cfg.JWTSecret)

	router := chi.NewRouter()
	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)

	if withLogger {
		router.Use(chiMiddleware.Logger)
	}

	router.Use(chiMiddleware.Recoverer)
	router.Use(corsHandler(cfg).Handler)

	registerPublicRoutes(router, state)
	registerAuthenticatedRoutes(router, state, authMW)
	registerAdminRoutes(router, state, adminMW)
	registerPatternRoutes(router, state, authMW, adminMW)
	registerInternalRoutes(router, state)

	return router
}

// corsHandler builds the CORS middleware from the configured allowed origins.
func corsHandler(cfg *config.Config) *cors.Cors {
	return cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           corsMaxAgeSeconds,
	})
}

// registerPublicRoutes wires routes that require no authentication.
func registerPublicRoutes(router chi.Router, state *State) {
	router.Get("/health", state.Health)
	router.Get("/metrics", metrics.Handler())

	router.Get("/api/settings/public", state.PublicSettings)

	router.Post("/api/auth/register", state.Register)
	router.Post("/api/auth/login", state.Login)

	router.Get("/api/auth/oauth/providers", state.ListProviders)
	router.Get("/api/auth/oauth/{provider}/authorize", state.OAuthAuthorize)
	router.Post("/api/auth/oauth/callback", state.OAuthCallback)

	router.Get("/api/auth/oidc/authorize", state.OIDCAuthorize)
	router.Post("/api/auth/oidc/callback", state.OIDCCallback)

	router.Post("/api/auth/ldap/login", state.LDAPLogin)
}

// registerAuthenticatedRoutes wires routes that require a valid session.
func registerAuthenticatedRoutes(router chi.Router, state *State, authMW func(http.Handler) http.Handler) {
	router.Group(func(group chi.Router) {
		group.Use(authMW)

		group.Get("/api/auth/me", state.Me)
		group.Put("/api/auth/profile", state.UpdateProfile)
		group.Put("/api/auth/password", state.ChangePassword)

		group.Post("/api/courses/{slug}/enroll", state.Enroll)
		group.Delete("/api/courses/{slug}/unenroll", state.Unenroll)

		group.Post("/api/courses/{slug}/lessons/{lesson_slug}/complete", state.MarkLessonComplete)

		group.Get("/api/my/courses", state.MyCourses)
		group.Get("/api/my/paths", state.MyPaths)
	})
}

// registerAdminRoutes wires routes restricted to admin users.
func registerAdminRoutes(router chi.Router, state *State, adminMW func(http.Handler) http.Handler) {
	router.Group(func(group chi.Router) {
		group.Use(adminMW)

		group.Get("/api/admin/settings", state.GetSettings)
		group.Put("/api/admin/settings", state.UpdateSettings)

		group.Get("/api/admin/stats", state.AdminStats)
		group.Get("/api/admin/leaderboard", state.AdminLeaderboard)

		group.Get("/api/admin/users", state.ListUsers)
		group.Get("/api/admin/users/providers", state.ListAuthProviders)
		group.Get("/api/admin/users/search", state.SearchUsers)
		group.Get("/api/admin/users/{user_id}", state.GetUser)
		group.Put("/api/admin/users/{user_id}", state.UpdateUser)
		group.Delete("/api/admin/users/{user_id}", state.DeleteUser)

		group.Get("/api/admin/courses/{slug}/enrollments", state.ListCourseEnrollments)
		group.Post("/api/admin/courses/{slug}/enrollments", state.AdminEnrollUser)
		group.Get("/api/admin/courses/{slug}/enrollments/groups", state.AdminListGroupEnrollments)
		group.Post("/api/admin/courses/{slug}/enrollments/groups", state.AdminEnrollGroup)
		group.Delete("/api/admin/courses/{slug}/enrollments/groups/{group_id}", state.AdminUnenrollGroup)
		group.Delete("/api/admin/courses/{slug}/enrollments/{user_id}", state.AdminUnenrollUser)

		group.Get("/api/admin/paths/{slug}/enrollments", state.AdminListPathEnrollments)
		group.Post("/api/admin/paths/{slug}/enrollments", state.AdminEnrollUserInPath)
		group.Delete("/api/admin/paths/{slug}/enrollments/{user_id}", state.AdminUnenrollUserFromPath)

		group.Post("/api/admin/sync-progress", state.SyncProgress)

		group.Get("/api/admin/groups", state.ListGroups)
		group.Post("/api/admin/groups", state.CreateGroup)
		group.Delete("/api/admin/groups/{group_id}", state.DeleteGroup)
		group.Get("/api/admin/groups/mappings", state.ListGroupMappings)
		group.Post("/api/admin/groups/mappings", state.UpsertGroupMapping)
		group.Delete("/api/admin/groups/mappings/{group_name}", state.DeleteGroupMapping)
	})
}

// registerPatternRoutes wires the markdown pattern routes, split between
// authenticated read access and admin-only write access.
func registerPatternRoutes(router chi.Router, state *State, authMW, adminMW func(http.Handler) http.Handler) {
	router.Group(func(group chi.Router) {
		group.Use(authMW)
		group.Get("/api/patterns", state.ListPatterns)
		group.Get("/api/patterns/{id}", state.GetPattern)
	})

	router.Group(func(group chi.Router) {
		group.Use(adminMW)
		group.Post("/api/admin/patterns", state.CreatePattern)
		group.Put("/api/admin/patterns/{name}", state.UpdatePattern)
		group.Delete("/api/admin/patterns/{name}", state.DeletePattern)
	})
}

// registerInternalRoutes wires routes reserved for Course Service, which rely
// on network policy rather than authentication for access control.
//
//nolint:godox // tracked security follow-up, see PR #74 review comment
func registerInternalRoutes(router chi.Router, state *State) {
	// TODO(security): these routes accept no service-to-service authentication.
	// Any caller that can reach this port can forge enrollment/progress data for
	// any user_id. Flagged in PR #74 review as requiring a dedicated follow-up
	// PR (e.g. a shared INTERNAL_SERVICE_SECRET header, mirroring the existing
	// JWT_SECRET pattern in helm/templates/secret.yaml).
	router.Post("/internal/enrollments/auto", state.InternalAutoEnroll)
	router.Get("/internal/enrollments/check", state.InternalCheckEnrollment)
	router.Get("/internal/progress/viewed", state.InternalViewedLessons)
	router.Post("/internal/progress/complete", state.InternalMarkComplete)
	router.Post("/internal/progress/course-complete", state.InternalMarkCourseComplete)
	router.Post("/internal/progress/module", state.InternalRecordModuleProgress)
	router.Get("/internal/progress/modules", state.InternalGetModuleProgress)
	router.Get("/internal/progress/course-summary", state.InternalCourseSummary)
}
