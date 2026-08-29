package main

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"
)

// TestRun_BootAndGracefulShutdown starts run() in the background, lets it
// bind, then SIGTERMs this process for a clean nil return.
func TestRun_BootAndGracefulShutdown(t *testing.T) {
	t.Setenv("PORT", "0")

	done := make(chan error, 1)
	go func() { done <- run() }()

	time.Sleep(300 * time.Millisecond)

	err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Errorf("run() returned %v, want nil after graceful shutdown", runErr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("run() did not return after SIGTERM")
	}
}

// TestMain_GracefulShutdown calls main() directly: because run() returns nil
// on a SIGTERM-triggered graceful shutdown, main() returns without calling
// [os.Exit], so it is safe to invoke in-process.
func TestMain_GracefulShutdown(t *testing.T) {
	t.Setenv("PORT", "0")

	done := make(chan struct{})

	go func() {
		main()
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)

	err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("main() did not return after SIGTERM")
	}
}

// TestServe_ListenError returns the ListenAndServe error when the requested
// address is already in use.
//
//nolint:paralleltest // serve() blocks on the port; keep this test serial
func TestServe_ListenError(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer func() { _ = ln.Close() }()

	srv := &http.Server{Addr: ln.Addr().String(), ReadHeaderTimeout: readHeaderTimeout}

	serveErr := serve(srv)
	if serveErr == nil {
		t.Error("serve() should return an error when the address is in use")
	}
}
