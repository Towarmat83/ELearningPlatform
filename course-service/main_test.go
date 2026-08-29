package main

import (
	"context"
	"net/http"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/genesary/pupitre/course-service/internal/config"
)

// TestInitLogger_Defaults builds an info-level production logger by default.
func TestInitLogger_Defaults(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")

	logger := initLogger()
	if logger == nil {
		t.Fatal("initLogger returned nil")
	}

	if !logger.Core().Enabled(zapcore.InfoLevel) || logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("default logger should enable info but not debug")
	}
}

// TestInitLogger_DebugText honours LOG_LEVEL and LOG_FORMAT.
func TestInitLogger_DebugText(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")

	logger := initLogger()
	if logger == nil || !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("expected a debug-enabled logger")
	}
}

// TestInitLogger_InvalidLevel still builds a logger.
func TestInitLogger_InvalidLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "not-a-level")

	if initLogger() == nil {
		t.Error("initLogger returned nil for an unparseable level")
	}
}

// TestLogConfig does not panic with a populated config.
func TestLogConfig(t *testing.T) {
	t.Parallel()

	logConfig(&config.Config{
		Port: 8082, DatabaseURL: "postgres://x", DBMaxOpenConns: 10, DBMaxIdleConns: 5,
		UserServiceURL: "http://user", CheckerServiceURL: "http://checker",
		GitCacheTTL: 15, CORSOrigins: []string{"*"},
	})
}

// TestConnectWithRetry_ContextCancelled returns promptly when the context is
// already done rather than sleeping through every attempt.
func TestConnectWithRetry_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()

	_, err := connectWithRetry(ctx, &config.Config{
		DatabaseURL: "postgres://127.0.0.1:1/none?sslmode=disable&connect_timeout=1",
	})
	if err == nil {
		t.Fatal("expected an error connecting to an unreachable database")
	}

	if time.Since(start) > 25*time.Second {
		t.Errorf("connectWithRetry ignored the cancelled context (took %s)", time.Since(start))
	}
}

// TestServe_GracefulShutdown starts the server, sends SIGTERM to this
// process, and expects serve to return nil after a clean shutdown.
func TestServe_GracefulShutdown(t *testing.T) { //nolint:paralleltest // sends a process signal
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
	}

	done := make(chan error, 1)
	go func() { done <- serve(srv) }()

	// Give serve() time to register its signal handler and start listening.
	time.Sleep(200 * time.Millisecond)

	err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case serveErr := <-done:
		if serveErr != nil {
			t.Errorf("serve returned %v, want nil after graceful shutdown", serveErr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("serve did not return after SIGTERM")
	}
}
