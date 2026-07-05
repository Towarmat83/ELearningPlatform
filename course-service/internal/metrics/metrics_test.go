package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandler_ReturnsMetrics verifies that Handler serves a 200 response
// containing the expected Prometheus metric names.
func TestHandler_ReturnsMetrics(t *testing.T) {
	t.Parallel()

	h := Handler()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "elearning_active_courses_total") {
		t.Error("expected elearning_active_courses_total in metrics output")
	}
}
