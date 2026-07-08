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

const (
	// readHeaderTimeout bounds how long the server waits to read request headers.
	readHeaderTimeout = 10 * time.Second
	// readTimeout bounds the total time to read the request (headers + body).
	readTimeout = 30 * time.Second
	// writeTimeout bounds the time to write the response.
	writeTimeout = 30 * time.Second
	// idleTimeout bounds how long an idle keep-alive connection is kept open.
	idleTimeout = 60 * time.Second
)

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
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
