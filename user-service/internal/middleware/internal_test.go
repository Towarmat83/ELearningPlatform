package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// iaRun drives the InternalAuth middleware with the given secret header value
// (empty means the header is omitted) and returns the recorded status code
// plus whether the wrapped handler ran.
func iaRun(t *testing.T, configured, header string, sendHeader bool) (int, bool) {
	t.Helper()

	var called bool

	mw := InternalAuth(configured)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/internal/x", http.NoBody)
	if sendHeader {
		req.Header.Set("X-Internal-Secret", header)
	}

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	return rec.Code, called
}

// TestInternalAuth_Match lets a request with the correct secret through.
func TestInternalAuth_Match(t *testing.T) {
	t.Parallel()

	code, called := iaRun(t, "s3cret", "s3cret", true)
	if code != http.StatusOK || !called {
		t.Errorf("want 200 and handler called, got %d called=%v", code, called)
	}
}

// TestInternalAuth_Mismatch rejects a wrong secret with 401.
func TestInternalAuth_Mismatch(t *testing.T) {
	t.Parallel()

	code, called := iaRun(t, "s3cret", "nope", true)
	if code != http.StatusUnauthorized || called {
		t.Errorf("want 401 and handler skipped, got %d called=%v", code, called)
	}
}

// TestInternalAuth_MissingHeader rejects a request with no secret header.
func TestInternalAuth_MissingHeader(t *testing.T) {
	t.Parallel()

	code, called := iaRun(t, "s3cret", "", false)
	if code != http.StatusUnauthorized || called {
		t.Errorf("want 401 and handler skipped, got %d called=%v", code, called)
	}
}
