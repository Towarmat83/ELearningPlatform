package main

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
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
	t.Setenv("LOG_LEVEL", "nonsense")

	if initLogger() == nil {
		t.Error("initLogger returned nil for an unparseable level")
	}
}

// TestLoadConfig returns a populated config and logs its warnings.
func TestLoadConfig(t *testing.T) {
	t.Setenv("PORT", "9091")
	t.Setenv("JWT_SECRET", "a-sufficiently-long-test-secret-value")

	cfg := loadConfig()
	if cfg == nil {
		t.Fatal("loadConfig returned nil")
	}

	if cfg.Port != 9091 {
		t.Errorf("Port = %d, want 9091", cfg.Port)
	}
}

// TestServe_GracefulShutdown starts the server, signals this process with
// SIGTERM, and expects serve to return nil after a clean shutdown.
func TestServe_GracefulShutdown(t *testing.T) { //nolint:paralleltest // sends a process signal
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
	}

	done := make(chan error, 1)
	go func() { done <- serve(srv) }()

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
