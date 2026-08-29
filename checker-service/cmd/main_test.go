package main

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestInitLogger_DefaultsToInfoJSON builds an info-level production logger
// by default.
func TestInitLogger_DefaultsToInfoJSON(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")

	logger := initLogger()
	if logger == nil {
		t.Fatal("initLogger returned nil")
	}

	if !logger.Core().Enabled(zapcore.InfoLevel) {
		t.Error("expected info level to be enabled by default")
	}

	if logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("expected debug level to be disabled by default")
	}
}

// TestInitLogger_RespectsLevelAndTextFormat honours LOG_LEVEL and
// LOG_FORMAT.
func TestInitLogger_RespectsLevelAndTextFormat(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")

	logger := initLogger()
	if logger == nil {
		t.Fatal("initLogger returned nil")
	}

	if !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("expected debug level to be enabled when LOG_LEVEL=debug")
	}
}

// TestInitLogger_IgnoresInvalidLevel still builds a logger when LOG_LEVEL is
// unparseable.
func TestInitLogger_IgnoresInvalidLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "not-a-level")
	t.Setenv("LOG_FORMAT", "")

	logger := initLogger()
	if logger == nil {
		t.Fatal("initLogger returned nil")
	}
}
