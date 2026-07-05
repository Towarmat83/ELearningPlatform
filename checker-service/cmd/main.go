// Package main runs the checker-service HTTP API, which evaluates lab
// submissions against GitLab MR state using OPA policies.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/elearning/checker-service/internal/config"
	"github.com/elearning/checker-service/internal/handlers"
)

// readHeaderTimeout bounds how long the server waits to read request headers.
const readHeaderTimeout = 10 * time.Second

// main starts the checker-service HTTP server.
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()
	handler := handlers.New(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("checker-service starting", "addr", addr, "gitlab_base_url", cfg.GitLabBaseURL)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler.BuildRouter(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
