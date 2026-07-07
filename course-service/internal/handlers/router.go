package handlers

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/metrics"
	apimiddleware "github.com/elearning/course-service/internal/middleware"
)

// routerCORSMaxAgeSeconds is the max-age advertised for CORS preflight
// response caching.
const routerCORSMaxAgeSeconds = 300

// BuildRouter constructs the course-service chi router, wiring public
// routes, admin routes behind the auth middleware, and CORS/logging
// middleware.
func BuildRouter(state *State, cfg *config.Config, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(cfg.JWTSecret)

	corsOptions := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           routerCORSMaxAgeSeconds,
	})

	router := chi.NewRouter()
	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)

	if withLogger {
		router.Use(chiMiddleware.Logger)
	}

	router.Use(chiMiddleware.Recoverer)
	router.Use(corsOptions.Handler)

	registerPublicRoutes(router, state)

	router.Group(func(group chi.Router) {
		group.Use(authMW)

		registerAdminRoutes(group, state)
	})

	return router
}

// registerPublicRoutes wires the unauthenticated course-service endpoints.
func registerPublicRoutes(router chi.Router, state *State) {
	router.Get("/health", state.Health)
	router.Get("/metrics", metrics.Handler())

	router.Get("/api/courses", state.ListCourses)
	router.Get("/api/courses/{slug}", state.GetCourse)

	router.Get("/api/paths", state.ListPaths)
	router.Get("/api/paths/{slug}", state.GetPath)

	router.Get("/uploads/{filename}", state.ServeUpload)
}

// registerAdminRoutes wires the authenticated admin and course-content
// endpoints behind the auth middleware group.
func registerAdminRoutes(router chi.Router, state *State) {
	router.Get("/api/admin/courses", state.ListAdminCourses)
	router.Post("/api/admin/courses", state.CreateCourseCRD)
	router.Get("/api/admin/courses/{slug}/crd", state.GetCourseCRD)
	router.Put("/api/admin/courses/{slug}/crd", state.UpdateCourseCRD)
	router.Delete("/api/admin/courses/{slug}/crd", state.DeleteCourseCRD)
	router.Get("/api/admin/lab-checks", state.GetLabResults)
	router.Post("/api/admin/cache/clear", state.ClearCache)
	router.Post("/api/admin/courses/{slug}/cache/clear", state.ClearCourseCache)
	router.Post("/api/admin/courses/{slug}/modules/{index}/cache/clear", state.ClearModuleCache)

	router.Get("/api/courses/{slug}/modules", state.ListModules)
	router.Get("/api/courses/{slug}/modules/{index}", state.GetModule)
	router.Post("/api/courses/{slug}/modules/{index}/submit", state.SubmitModule)
	router.Post("/api/courses/{slug}/modules/{index}/check", state.CheckModule)
	router.Post("/api/courses/{slug}/modules/{index}/steps/{stepIndex}/check", state.CheckModuleStep)
	router.Post("/api/courses/{slug}/modules/{index}/record", state.RecordLocalCheck)

	router.Get("/api/courses/{slug}/labs", state.ListLabs)
	router.Get("/api/courses/{slug}/labs/{lab_id}", state.GetLab)
	router.Get("/api/courses/{slug}/progress", state.GetCourseProgress)

	router.Get("/api/courses/{slug}/lessons", state.ListLessons)
	router.Get("/api/courses/{slug}/lessons/{lessonSlug}", state.GetLesson)
	router.Post("/api/courses/{slug}/lessons/{lessonSlug}/complete", state.MarkLessonComplete)
}
