use axum::{extract::State, Json};
use bcrypt::{hash, verify, DEFAULT_COST};
use chrono::Utc;

use crate::{
    error::{AppError, Result},
    middleware::auth::create_token,
    models::user::{AuthResponse, LoginRequest, RegisterRequest, UserPublic},
    AppState,
};

pub async fn register(
    State(state): State<AppState>,
    Json(req): Json<RegisterRequest>,
) -> Result<Json<AuthResponse>> {
    // Validate
    if req.username.len() < 3 {
        return Err(AppError::BadRequest("Username must be at least 3 characters".to_string()));
    }
    if req.password.len() < 8 {
        return Err(AppError::BadRequest("Password must be at least 8 characters".to_string()));
    }

    // Check existing
    let existing = sqlx::query_scalar::<_, i64>(
        "SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
    )
    .bind(&req.email)
    .bind(&req.username)
    .fetch_one(&state.db)
    .await?;

    if existing > 0 {
        return Err(AppError::Conflict("Email or username already taken".to_string()));
    }

    let password_hash = hash(&req.password, DEFAULT_COST)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("Hash error: {}", e)))?;

    let user = sqlx::query_as::<_, crate::models::user::User>(
        r#"INSERT INTO users (username, email, password_hash, role)
           VALUES ($1, $2, $3, 'student')
           RETURNING *"#,
    )
    .bind(&req.username)
    .bind(&req.email)
    .bind(&password_hash)
    .fetch_one(&state.db)
    .await?;

    let token = create_token(user.id, &user.email, &user.role, &state.config.jwt_secret, state.config.jwt_expiry_hours)
        .map_err(|e| AppError::Internal(e))?;

    // Update metrics
    crate::metrics::ACTIVE_USERS.inc();

    Ok(Json(AuthResponse {
        token,
        user: UserPublic::from(user),
    }))
}

pub async fn login(
    State(state): State<AppState>,
    Json(req): Json<LoginRequest>,
) -> Result<Json<AuthResponse>> {
    let user = sqlx::query_as::<_, crate::models::user::User>(
        "SELECT * FROM users WHERE email = $1 AND is_active = TRUE",
    )
    .bind(&req.email)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::Unauthorized("Invalid email or password".to_string()))?;

    let valid = verify(&req.password, &user.password_hash)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("Verify error: {}", e)))?;

    if !valid {
        return Err(AppError::Unauthorized("Invalid email or password".to_string()));
    }

    let token = create_token(user.id, &user.email, &user.role, &state.config.jwt_secret, state.config.jwt_expiry_hours)
        .map_err(|e| AppError::Internal(e))?;

    Ok(Json(AuthResponse {
        token,
        user: UserPublic::from(user),
    }))
}

pub async fn me(
    State(state): State<AppState>,
    claims: axum::extract::Extension<crate::middleware::auth::Claims>,
) -> Result<Json<UserPublic>> {
    let user = sqlx::query_as::<_, crate::models::user::User>(
        "SELECT * FROM users WHERE id = $1",
    )
    .bind(claims.sub)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("User not found".to_string()))?;

    Ok(Json(UserPublic::from(user)))
}

pub async fn change_password(
    State(state): State<AppState>,
    claims: axum::extract::Extension<crate::middleware::auth::Claims>,
    Json(req): Json<serde_json::Value>,
) -> Result<Json<serde_json::Value>> {
    let old_password = req["old_password"]
        .as_str()
        .ok_or_else(|| AppError::BadRequest("old_password required".to_string()))?;
    let new_password = req["new_password"]
        .as_str()
        .ok_or_else(|| AppError::BadRequest("new_password required".to_string()))?;

    if new_password.len() < 8 {
        return Err(AppError::BadRequest("New password must be at least 8 characters".to_string()));
    }

    let user = sqlx::query_as::<_, crate::models::user::User>(
        "SELECT * FROM users WHERE id = $1",
    )
    .bind(claims.sub)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("User not found".to_string()))?;

    let valid = verify(old_password, &user.password_hash)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("{}", e)))?;

    if !valid {
        return Err(AppError::Unauthorized("Incorrect current password".to_string()));
    }

    let new_hash = hash(new_password, DEFAULT_COST)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("{}", e)))?;

    sqlx::query("UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3")
        .bind(&new_hash)
        .bind(Utc::now())
        .bind(claims.sub)
        .execute(&state.db)
        .await?;

    Ok(Json(serde_json::json!({"message": "Password changed successfully"})))
}
