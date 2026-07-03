package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "endpoint", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	}, []string{"method", "endpoint"})

	ActiveUsers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "elearning_active_users_total",
		Help: "Total number of registered users",
	})

	ActiveCourses = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "elearning_active_courses_total",
		Help: "Total number of published courses",
	})

	EnrollmentsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "elearning_enrollments_total",
		Help: "Total course enrollments",
	})
)

func Handler() http.HandlerFunc {
	h := promhttp.Handler()

	return func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }
}
