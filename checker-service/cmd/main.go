// Package main runs the checker-service HTTP API, which evaluates lab
// submissions against GitLab MR state using OPA policies.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
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
)

// main starts the checker-service HTTP server.
func main() {
	logger := initLogger()
	zap.ReplaceGlobals(logger)

	cfg := config.Load()
	handler := handlers.New(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	zap.L().Info("checker-service starting", zap.String("addr", addr), zap.String("gitlab_base_url", cfg.GitLabBaseURL))

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
		zap.L().Error("server error", zap.Error(err))

		_ = logger.Sync()

		os.Exit(1)
	}

	_ = logger.Sync()
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
