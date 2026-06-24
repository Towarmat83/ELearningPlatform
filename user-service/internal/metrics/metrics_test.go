package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ReturnsMetrics(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "elearning_active_users_total") {
		t.Error("expected elearning_active_users_total in metrics output")
	}
}
