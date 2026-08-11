package handlers

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/genesary/pupitre/user-service/internal/config"
	"github.com/genesary/pupitre/user-service/internal/metrics"
	apimiddleware "github.com/genesary/pupitre/user-service/internal/middleware"
)

// remoteIP extracts the IP from r.RemoteAddr (already set to the real client
// IP by chiMiddleware.RealIP) to use as the rate-limit key.
func remoteIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, nil //nolint:nilerr // best-effort: fall back to raw addr
	}

	return ip, nil
}

// corsMaxAgeSeconds bounds CORS preflight cache duration, in seconds.
const corsMaxAgeSeconds = 300

// BuildRouter assembles the HTTP router, wiring public, authenticated, admin,
// pattern, and internal routes with their respective middleware.
func BuildRouter(state *State, cfg *config.Config, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(cfg.JWTSecret)
	adminMW := apimiddleware.Admin(cfg.JWTSecret)
	managerMW := apimiddleware.Manager(cfg.JWTSecret)
	internalMW := apimiddleware.InternalAuth(cfg.InternalSecret)

	router := chi.NewRouter()
	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)

	if withLogger {
		router.Use(chiMiddleware.Logger)
	}

	router.Use(chiMiddleware.Recoverer)
	router.Use(chiMiddleware.RequestSize(maxRequestBodyBytes))
	router.Use(corsHandler(cfg).Handler)

	registerPublicRoutes(router, state, cfg)
	registerAuthenticatedRoutes(router, state, authMW)
	registerAdminRoutes(router, state, adminMW)
	registerManagerRoutes(router, state, managerMW)
	registerPatternRoutes(router, state, authMW, adminMW)
	registerInternalRoutes(router, state, internalMW)

	return router
}

// corsHandler builds the CORS middleware from the configured allowed origins.
func corsHandler(cfg *config.Config) *cors.Cors {
	return cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           corsMaxAgeSeconds,
	})
}

// registerPublicRoutes wires routes that require no authentication.
func registerPublicRoutes(router chi.Router, state *State, cfg *config.Config) {
	router.Get("/health", state.Health)
	router.Get("/metrics", metrics.Handler())

	router.Get("/api/settings/public", state.PublicSettings)
	router.Get("/api/users/{id}", state.PublicUser)
	router.Get("/api/users/{id}/badges", state.UserBadges)
	router.Get("/api/badges/{slug}", state.BadgeStats)
	router.Get("/api/leaderboard", state.BadgeLeaderboard)
	router.Get("/api/auth/oauth/providers", state.ListProviders)
	router.Get("/api/auth/oauth/{provider}/authorize", state.OAuthAuthorize)
	router.Get("/api/auth/oidc/authorize", state.OIDCAuthorize)

	// Rate-limited auth endpoints — set AUTH_RATE_LIMIT_REQUESTS=0 to disable.
	router.Group(func(group chi.Router) {
		if cfg.AuthRateLimitRequests > 0 {
			group.Use(httprate.LimitBy(cfg.AuthRateLimitRequests, time.Duration(cfg.AuthRateLimitWindowSeconds)*time.Second, remoteIP))
		}

		group.Post("/api/auth/register", state.Register)
		group.Post("/api/auth/login", state.Login)
		group.Post("/api/auth/oauth/callback", state.OAuthCallback)
		group.Post("/api/auth/oidc/callback", state.OIDCCallback)
		group.Post("/api/auth/ldap/login", state.LDAPLogin)
	})
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
		group.Get("/api/my/badges", state.MyBadges)
		group.Get("/api/my/skills", state.MySkills)
		group.Get("/api/my/skills/{slug}", state.MySkillModules)
		group.Get("/api/my/groups", state.MyGroups)
		group.Get("/api/my/groups/{groupId}/members", state.MyGroupMembers)

		group.Get("/api/my/session-bookings", state.MySessionBookings)
		group.Post("/api/courses/{slug}/sessions/{sessionId}/book", state.BookSession)
		group.Delete("/api/courses/{slug}/sessions/{sessionId}/book", state.UnbookSession)
		group.Get("/api/courses/{slug}/sessions/{sessionId}/booking-count", state.SessionBookingCount)
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
		group.Get("/api/admin/users/{userId}", state.GetUser)
		group.Put("/api/admin/users/{userId}", state.UpdateUser)
		group.Delete("/api/admin/users/{userId}", state.DeleteUser)

		group.Get("/api/admin/courses/{slug}/sessions/{sessionId}/bookings", state.ListSessionBookings)
		group.Patch("/api/admin/courses/{slug}/sessions/{sessionId}/bookings/{userId}/presence", state.MarkSessionPresence)

		group.Get("/api/admin/courses/{slug}/enrollments", state.ListCourseEnrollments)
		group.Post("/api/admin/courses/{slug}/enrollments", state.AdminEnrollUser)
		group.Get("/api/admin/courses/{slug}/enrollments/groups", state.AdminListGroupEnrollments)
		group.Post("/api/admin/courses/{slug}/enrollments/groups", state.AdminEnrollGroup)
		group.Delete("/api/admin/courses/{slug}/enrollments/groups/{groupId}", state.AdminUnenrollGroup)
		group.Delete("/api/admin/courses/{slug}/enrollments/{userId}", state.AdminUnenrollUser)

		group.Get("/api/admin/paths/{slug}/enrollments", state.AdminListPathEnrollments)
		group.Post("/api/admin/paths/{slug}/enrollments", state.AdminEnrollUserInPath)
		group.Delete("/api/admin/paths/{slug}/enrollments/{userId}", state.AdminUnenrollUserFromPath)

		group.Post("/api/admin/sync-progress", state.SyncProgress)

		group.Get("/api/admin/groups", state.ListGroups)
		group.Post("/api/admin/groups", state.CreateGroup)
		group.Delete("/api/admin/groups/{groupId}", state.DeleteGroup)
		group.Get("/api/admin/groups/{groupId}/members", state.ListGroupMembers)
		group.Post("/api/admin/groups/{groupId}/members", state.AddGroupMember)
		group.Delete("/api/admin/groups/{groupId}/members/{userId}", state.RemoveGroupMember)
		group.Get("/api/admin/groups/mappings", state.ListGroupMappings)
		group.Post("/api/admin/groups/mappings", state.UpsertGroupMapping)
		group.Delete("/api/admin/groups/mappings/{groupName}", state.DeleteGroupMapping)
	})
}

// registerManagerRoutes wires routes restricted to manager users.
func registerManagerRoutes(router chi.Router, state *State, managerMW func(http.Handler) http.Handler) {
	router.Group(func(group chi.Router) {
		group.Use(managerMW)

		group.Get("/api/manager/users", state.ManagerListUsers)
		group.Get("/api/manager/users/{userId}/enrollments", state.ManagerGetUserEnrollments)
		group.Get("/api/manager/users/{userId}/path-enrollments", state.ManagerGetUserPathEnrollments)
		group.Post("/api/manager/courses/{slug}/enrollments", state.ManagerEnrollUser)
		group.Delete("/api/manager/courses/{slug}/enrollments/{userId}", state.ManagerUnenrollUser)
		group.Post("/api/manager/paths/{slug}/enrollments", state.ManagerEnrollUserPath)
		group.Delete("/api/manager/paths/{slug}/enrollments/{userId}", state.ManagerUnenrollUserPath)
		group.Get("/api/manager/courses/{slug}/sessions/{sessionId}/bookings", state.ListSessionBookings)
		group.Patch("/api/manager/courses/{slug}/sessions/{sessionId}/bookings/{userId}/presence", state.MarkSessionPresence)
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

// registerInternalRoutes wires routes reserved for Course Service behind the
// shared-secret middleware (X-Internal-Secret header).
func registerInternalRoutes(router chi.Router, state *State, internalMW func(http.Handler) http.Handler) {
	router.Group(func(group chi.Router) {
		group.Use(internalMW)

		group.Post("/internal/enrollments/auto", state.InternalAutoEnroll)
		group.Get("/internal/enrollments/check", state.InternalCheckEnrollment)
		group.Get("/internal/progress/viewed", state.InternalViewedLessons)
		group.Post("/internal/progress/complete", state.InternalMarkComplete)
		group.Post("/internal/progress/course-complete", state.InternalMarkCourseComplete)
		group.Post("/internal/progress/module", state.InternalRecordModuleProgress)
		group.Get("/internal/progress/modules", state.InternalGetModuleProgress)
		group.Get("/internal/progress/course-summary", state.InternalCourseSummary)
	})
}
