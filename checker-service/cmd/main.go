// Package main runs the checker-service HTTP API, which evaluates lab
// submissions against GitLab MR state using OPA policies.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/checker-service/internal/config"
	"github.com/genesary/pupitre/checker-service/internal/handlers"
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
	// shutdownTimeout bounds how long graceful shutdown waits for in-flight
	// requests to finish before the server is forced closed.
	shutdownTimeout = 15 * time.Second
)

// main starts the checker-service HTTP server and exits non-zero on a fatal
// startup or serving error.
func main() {
	logger := initLogger()
	zap.ReplaceGlobals(logger)

	err := run()

	_ = logger.Sync()

	if err != nil {
		zap.L().Error("fatal", zap.Error(err))
		os.Exit(1)
	}
}

// run loads configuration, builds the router and serves until an
// interrupt/terminate signal arrives, then shuts the server down gracefully.
func run() error {
	zap.L().Info("starting checker-service")

	zap.L().Info("loading configuration")

	cfg := config.Load()

	zap.L().Info("configuration loaded",
		zap.Int("port", cfg.Port),
		zap.String("gitlabBaseURL", cfg.GitLabBaseURL),
		zap.Bool("gitlabTokenConfigured", cfg.GitLabToken != ""),
		zap.Int("rateLimitRequests", cfg.RateLimitRequests),
		zap.Int("rateLimitWindowSeconds", cfg.RateLimitWindowSeconds),
		zap.Int("corsOriginsCount", len(cfg.CORSOrigins)),
	)

	zap.L().Info("building HTTP router")

	handler := handlers.New(cfg)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler.BuildRouter(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	return serve(srv)
}

// serve starts srv in the background, blocks until an interrupt/terminate
// signal is received or the server exits unexpectedly, then gracefully
// shuts srv down.
func serve(srv *http.Server) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)

	go func() {
		zap.L().Info("API listening", zap.String("addr", srv.Addr))

		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("server error: %w", err)

			return
		}

		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-quit:
	}

	zap.L().Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(shutCtx)
	if err != nil {
		return fmt.Errorf("forced shutdown: %w", err)
	}

	zap.L().Info("server stopped")

	return nil
}

// initLogger builds a zap logger driven by two environment variables:
//   - LOG_LEVEL  — debug/info/warn/error (default: info)
//   - LOG_FORMAT — json (default) or text (human-readable, for local dev)
func initLogger() *zap.Logger {
	level := zap.NewAtomicLevel()
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		_ = level.UnmarshalText([]byte(lvl))
	}

	var cfg zap.Config
	if os.Getenv("LOG_FORMAT") == "text" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Level = level

	return zap.Must(cfg.Build())
}
