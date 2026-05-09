package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elearning/user-service/internal/config"
	"github.com/elearning/user-service/internal/middleware"
)

type State struct {
	Pool   *pgxpool.Pool
	Config *config.Config
}

func (s *State) Health(w http.ResponseWriter, r *http.Request) {
	s.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "user-service",
		"version": "1.0.0",
	})
}

func (s *State) JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *State) Error(w http.ResponseWriter, status int, msg string) {
	s.JSON(w, status, map[string]string{"error": msg})
}

func (s *State) claims(r *http.Request) *middleware.Claims {
	return middleware.GetClaims(r)
}

func param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
