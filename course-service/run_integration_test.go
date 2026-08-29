//go:build integration

// run_integration_test.go boots the whole service through run() against a
// live PostgreSQL instance and shuts it down with SIGTERM, covering the
// startup orchestration (config, connect, migrate, dev-seed, router, serve).
// Behind the `integration` build tag; run with TEST_DATABASE_URL set.

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestRun_BootAndGracefulShutdown starts run() in the background, lets it
// come up (migrating + dev-seeding), then SIGTERMs for a clean nil return.
func TestRun_BootAndGracefulShutdown(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	t.Setenv("DATABASE_URL", url)
	t.Setenv("PORT", "0")
	t.Setenv("JWT_SECRET", "an-adequately-long-test-jwt-secret-value")
	t.Setenv("SEED_DEV_COURSES", "true") // exercise the embedded-catalogue seeder

	done := make(chan error, 1)
	go func() { done <- run() }()

	time.Sleep(1500 * time.Millisecond)

	err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Errorf("run() returned %v, want nil after graceful shutdown", runErr)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("run() did not return after SIGTERM")
	}
}

// TestRun_MissingDatabaseURL fails fast with a clear error.
func TestRun_MissingDatabaseURL(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "an-adequately-long-test-jwt-secret-value")

	err := run()
	if err == nil {
		t.Fatal("run() should fail when DATABASE_URL is unset")
	}
}

// TestMain_GracefulShutdown invokes main() directly. run() returns nil on a
// SIGTERM-triggered graceful shutdown, so main() returns without calling
// [os.Exit] and is safe to run in-process.
func TestMain_GracefulShutdown(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	t.Setenv("DATABASE_URL", url)
	t.Setenv("PORT", "0")
	t.Setenv("JWT_SECRET", "an-adequately-long-test-jwt-secret-value")

	done := make(chan struct{})

	go func() {
		main()
		close(done)
	}()

	time.Sleep(1500 * time.Millisecond)

	err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case <-done:
	case <-time.After(25 * time.Second):
		t.Fatal("main() did not return after SIGTERM")
	}
}
