mod config;
mod error;
mod metrics;
mod middleware;
mod models;
mod routes;

use axum::{
    middleware as axum_middleware,
    routing::{delete, get, post, put},
    Router,
};
use sqlx::postgres::PgPoolOptions;
use std::net::SocketAddr;
use tower_http::{
    cors::{Any, CorsLayer},
    trace::TraceLayer,
};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

use crate::config::Config;

#[derive(Clone)]
pub struct AppState {
    pub db: sqlx::PgPool,
    pub config: Config,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Load .env if present
    dotenvy::dotenv().ok();

    // Tracing
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info,elearning_api=debug".to_string()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    let config = Config::from_env()?;
    tracing::info!("Starting eLearning API on port {}", config.port);

    // Database
    let db = PgPoolOptions::new()
        .max_connections(20)
        .connect(&config.database_url)
        .await?;

    // Run migrations
    sqlx::migrate!("./migrations").run(&db).await?;
    tracing::info!("Database migrations applied");

    let state = AppState { db, config: config.clone() };

    // CORS
    let cors = CorsLayer::new()
        .allow_origin(Any)
        .allow_methods(Any)
        .allow_headers(Any);

    // Auth middleware
    let auth = axum_middleware::from_fn_with_state(
        state.clone(),
        middleware::auth::auth_middleware,
    );
    let admin_auth = axum_middleware::from_fn_with_state(
        state.clone(),
        middleware::auth::admin_middleware,
    );

    // Build router
    let app = Router::new()
        // Health
        .route("/health", get(health_handler))
        // Prometheus metrics
        .route("/metrics", get(metrics::metrics_handler))

        // Auth (public)
        .route("/api/auth/register", post(routes::auth::register))
        .route("/api/auth/login", post(routes::auth::login))

        // Auth (protected)
        .route(
            "/api/auth/me",
            get(routes::auth::me).layer(auth.clone()),
        )
        .route(
            "/api/auth/password",
            put(routes::auth::change_password).layer(auth.clone()),
        )

        // Courses (public read)
        .route("/api/courses", get(routes::courses::list_courses))
        .route("/api/courses/:id", get(routes::courses::get_course))

        // Courses (protected)
        .route(
            "/api/courses",
            post(routes::courses::create_course).layer(auth.clone()),
        )
        .route(
            "/api/courses/:id",
            put(routes::courses::update_course)
                .delete(routes::courses::delete_course)
                .layer(auth.clone()),
        )
        .route(
            "/api/courses/:id/enroll",
            post(routes::courses::enroll).layer(auth.clone()),
        )
        .route(
            "/api/courses/:id/unenroll",
            delete(routes::courses::unenroll).layer(auth.clone()),
        )
        .route(
            "/api/my/courses",
            get(routes::courses::my_courses).layer(auth.clone()),
        )

        // Labs (protected)
        .route(
            "/api/courses/:course_id/labs",
            get(routes::labs::list_labs)
                .post(routes::labs::create_lab)
                .layer(auth.clone()),
        )
        .route(
            "/api/courses/:course_id/labs/:lab_id",
            get(routes::labs::get_lab)
                .put(routes::labs::update_lab)
                .delete(routes::labs::delete_lab)
                .layer(auth.clone()),
        )

        // Submissions (protected)
        .route(
            "/api/courses/:course_id/labs/:lab_id/submit",
            post(routes::submissions::submit_lab).layer(auth.clone()),
        )
        .route(
            "/api/courses/:course_id/labs/:lab_id/submissions",
            get(routes::submissions::my_submissions).layer(auth.clone()),
        )
        .route(
            "/api/courses/:course_id/progress",
            get(routes::submissions::my_progress).layer(auth.clone()),
        )

        // Admin routes
        .route(
            "/api/admin/stats",
            get(routes::submissions::admin_stats).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/users",
            get(routes::admin::list_users).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/users/:user_id",
            get(routes::admin::get_user)
                .put(routes::admin::update_user)
                .delete(routes::admin::delete_user)
                .layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses",
            get(routes::courses::admin_list_courses).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/monitoring",
            get(routes::submissions::admin_course_monitoring).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/labs/:lab_id",
            get(routes::labs::admin_get_lab).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/labs/:lab_id/submissions",
            get(routes::submissions::admin_lab_submissions).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/labs/:lab_id/stats",
            get(routes::submissions::admin_lab_stats).layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/enrollments",
            get(routes::courses::admin_list_enrollments)
                .post(routes::courses::admin_enroll_user)
                .layer(admin_auth.clone()),
        )
        .route(
            "/api/admin/courses/:course_id/enrollments/:user_id",
            delete(routes::courses::admin_unenroll_user).layer(admin_auth.clone()),
        )

        .layer(cors)
        .layer(TraceLayer::new_for_http())
        .with_state(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], config.port));
    tracing::info!("Listening on {}", addr);

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}

async fn health_handler() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "ok",
        "service": "elearning-api",
        "version": env!("CARGO_PKG_VERSION"),
    }))
}
