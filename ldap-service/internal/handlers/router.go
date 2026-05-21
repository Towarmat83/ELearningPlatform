package handlers

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/elearning/ldap-service/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func BuildRouter(s *State, cfg *config.Config, pool *pgxpool.Pool, withLogger bool) *chi.Mux {
	corsOptions := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
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

	r.Post("/api/auth/ldap/login", s.LDAPLogin)

	return r
}
