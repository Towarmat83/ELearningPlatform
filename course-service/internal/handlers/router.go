package handlers

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/elearning/course-service/internal/config"
	"github.com/elearning/course-service/internal/metrics"
	apimiddleware "github.com/elearning/course-service/internal/middleware"
)

func BuildRouter(s *State, cfg *config.Config, withLogger bool) *chi.Mux {
	authMW := apimiddleware.Auth(cfg.JWTSecret)

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

	r.Get("/health", s.Health)
	r.Get("/metrics", metrics.Handler())

	r.Get("/api/courses", s.ListCourses)
	r.Get("/api/courses/{slug}", s.GetCourse)

	r.Get("/uploads/{filename}", s.ServeUpload)

	r.Group(func(r chi.Router) {
		r.Use(authMW)

		r.Get("/api/admin/courses", s.ListAdminCourses)

		r.Get("/api/courses/{slug}/modules", s.ListModules)
		r.Get("/api/courses/{slug}/modules/{index}", s.GetModule)
		r.Post("/api/courses/{slug}/modules/{index}/submit", s.SubmitModule)

		r.Get("/api/courses/{slug}/labs", s.ListLabs)
		r.Get("/api/courses/{slug}/labs/{lab_id}", s.GetLab)
		r.Get("/api/courses/{slug}/progress", s.GetCourseProgress)

		r.Get("/api/courses/{slug}/lessons", s.ListLessons)
		r.Get("/api/courses/{slug}/lessons/{lesson_slug}", s.GetLesson)
		r.Post("/api/courses/{slug}/lessons/{lesson_slug}/complete", s.MarkLessonComplete)
	})

	return r
}
