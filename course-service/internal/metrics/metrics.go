// Package metrics defines and exposes the Prometheus metrics collected by
// course-service.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// labelMethod is the Prometheus label name for the HTTP method.
	labelMethod = "method"
	// labelEndpoint is the Prometheus label name for the request endpoint.
	labelEndpoint = "endpoint"
	// labelStatus is the Prometheus label name for the HTTP status code.
	labelStatus = "status"
)

var (
	// HTTPRequestsTotal counts HTTP requests handled by course-service,
	// labeled by method, endpoint, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{ //nolint:gochecknoglobals // promauto collectors must be package-level
		Name: "http_requests_total",
		Help: "Total HTTP requests",
	}, []string{labelMethod, labelEndpoint, labelStatus})

	// HTTPRequestDuration observes HTTP request durations, labeled by
	// method and endpoint.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{ //nolint:gochecknoglobals // promauto collectors must be package-level
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	}, []string{labelMethod, labelEndpoint})

	// ActiveUsers reports the current number of registered users.
	ActiveUsers = promauto.NewGauge(prometheus.GaugeOpts{ //nolint:gochecknoglobals,promlinter // promauto collectors must be package-level; deployed metric name, see dashboards
		Name: "elearning_active_users_total",
		Help: "Total number of registered users",
	})

	// ActiveCourses reports the current number of published courses.
	ActiveCourses = promauto.NewGauge(prometheus.GaugeOpts{ //nolint:gochecknoglobals,promlinter // promauto collectors must be package-level; deployed metric name, see dashboards
		Name: "elearning_active_courses_total",
		Help: "Total number of published courses",
	})

	// EnrollmentsTotal reports the current number of course enrollments.
	EnrollmentsTotal = promauto.NewGauge(prometheus.GaugeOpts{ //nolint:gochecknoglobals,promlinter // promauto collectors must be package-level; deployed metric name, see dashboards
		Name: "elearning_enrollments_total",
		Help: "Total course enrollments",
	})
)

// Handler returns the HTTP handler that serves Prometheus metrics.
func Handler() http.HandlerFunc {
	h := promhttp.Handler()

	return func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }
}
