use axum::{http::StatusCode, response::IntoResponse};
use lazy_static::lazy_static;
use prometheus::{
    register_counter_vec, register_gauge, register_histogram_vec, CounterVec, Encoder, Gauge,
    HistogramVec, TextEncoder,
};

lazy_static! {
    pub static ref HTTP_REQUESTS_TOTAL: CounterVec = register_counter_vec!(
        "http_requests_total",
        "Total HTTP requests",
        &["method", "endpoint", "status"]
    )
    .unwrap();

    pub static ref HTTP_REQUEST_DURATION: HistogramVec = register_histogram_vec!(
        "http_request_duration_seconds",
        "HTTP request duration in seconds",
        &["method", "endpoint"],
        vec![0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0]
    )
    .unwrap();

    pub static ref ACTIVE_USERS: Gauge = register_gauge!(
        "elearning_active_users_total",
        "Total number of registered users"
    )
    .unwrap();

    pub static ref ACTIVE_COURSES: Gauge = register_gauge!(
        "elearning_active_courses_total",
        "Total number of published courses"
    )
    .unwrap();

    pub static ref LAB_SUBMISSIONS_TOTAL: CounterVec = register_counter_vec!(
        "elearning_lab_submissions_total",
        "Total lab submissions",
        &["lab_type", "result"]
    )
    .unwrap();

    pub static ref ENROLLMENTS_TOTAL: Gauge = register_gauge!(
        "elearning_enrollments_total",
        "Total course enrollments"
    )
    .unwrap();
}

pub async fn metrics_handler() -> impl IntoResponse {
    let encoder = TextEncoder::new();
    let metric_families = prometheus::gather();
    let mut buffer = Vec::new();

    if let Err(e) = encoder.encode(&metric_families, &mut buffer) {
        tracing::error!("Failed to encode metrics: {}", e);
        return (StatusCode::INTERNAL_SERVER_ERROR, "Failed to encode metrics".to_string());
    }

    match String::from_utf8(buffer) {
        Ok(s) => (StatusCode::OK, s),
        Err(e) => {
            tracing::error!("Metrics UTF-8 error: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Encoding error".to_string())
        }
    }
}
