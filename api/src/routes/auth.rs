use axum::{extract::State, Json};
use bcrypt::{hash, verify, DEFAULT_COST};
use chrono::Utc;

use crate::{
    error::{AppError, Result},
    middleware::auth::create_token,
    models::user::{AuthResponse, LoginRequest, RegisterRequest, UserPublic},
    routes::settings::read_setting,
    AppState,
};

// ── Password policy helper ────────────────────────────────────────────────────

async fn validate_password_policy(db: &sqlx::PgPool, password: &str) -> Result<()> {
    let min_len: usize = read_setting(db, "password_min_length", "8").await
        .parse()
        .unwrap_or(8);

    if password.len() < min_len {
        return Err(AppError::BadRequest(format!(
            "Password must be at least {} characters",
            min_len
        )));
    }

    if read_setting(db, "password_require_uppercase", "false").await == "true"
        && !password.chars().any(|c| c.is_uppercase())
    {
        return Err(AppError::BadRequest(
            "Password must contain at least one uppercase letter".to_string(),
        ));
    }

    if read_setting(db, "password_require_number", "false").await == "true"
        && !password.chars().any(|c| c.is_numeric())
    {
        return Err(AppError::BadRequest(
            "Password must contain at least one number".to_string(),
        ));
    }

    Ok(())
}

// ── Handlers ──────────────────────────────────────────────────────────────────

pub async fn register(
    State(state): State<AppState>,
    Json(req): Json<RegisterRequest>,
) -> Result<Json<AuthResponse>> {
    // ── Registration gate ──────────────────────────────────────────────────
    if read_setting(&state.db, "registration_enabled", "true").await != "true" {
        return Err(AppError::Forbidden(
            "New user registration is currently disabled.".to_string(),
        ));
    }

    // ── Email domain whitelist ─────────────────────────────────────────────
    let whitelist = read_setting(&state.db, "registration_email_whitelist", "").await;
    if !whitelist.trim().is_empty() {
        let allowed: Vec<&str> = whitelist
            .split(',')
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .collect();
        let domain = req.email.split('@').nth(1).unwrap_or("");
        if !allowed.iter().any(|d| d.eq_ignore_ascii_case(domain)) {
            return Err(AppError::Forbidden(
                "Registration is restricted to specific email domains.".to_string(),
            ));
        }
    }

    // ── Username length ────────────────────────────────────────────────────
    if req.username.len() < 3 {
        return Err(AppError::BadRequest(
            "Username must be at least 3 characters".to_string(),
        ));
    }

    // ── Password policy ────────────────────────────────────────────────────
    validate_password_policy(&state.db, &req.password).await?;

    // ── Uniqueness check ───────────────────────────────────────────────────
    let existing = sqlx::query_scalar::<_, i64>(
        "SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
    )
    .bind(&req.email)
    .bind(&req.username)
    .fetch_one(&state.db)
    .await?;

    if existing > 0 {
        return Err(AppError::Conflict(
            "Email or username already taken".to_string(),
        ));
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

    let token = create_token(
        user.id,
        &user.email,
        &user.role,
        &state.config.jwt_secret,
        state.config.jwt_expiry_hours,
    )
    .map_err(|e| AppError::Internal(e))?;

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
    // ── Local login gate ───────────────────────────────────────────────────
    if read_setting(&state.db, "sso_local_login_enabled", "true").await != "true" {
        return Err(AppError::Forbidden(
            "Local login is disabled. Please sign in using an SSO provider.".to_string(),
        ));
    }

    let user = sqlx::query_as::<_, crate::models::user::User>(
        "SELECT * FROM users WHERE email = $1 AND is_active = TRUE",
    )
    .bind(&req.email)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::Unauthorized("Invalid email or password".to_string()))?;

    let password_hash = user.password_hash.as_deref().ok_or_else(|| {
        AppError::Unauthorized(format!(
            "This account uses {} SSO login. Please sign in with your OAuth provider.",
            user.auth_provider
        ))
    })?;

    let valid = verify(&req.password, password_hash)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("Verify error: {}", e)))?;

    if !valid {
        return Err(AppError::Unauthorized(
            "Invalid email or password".to_string(),
        ));
    }

    let token = create_token(
        user.id,
        &user.email,
        &user.role,
        &state.config.jwt_secret,
        state.config.jwt_expiry_hours,
    )
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

pub async fn update_profile(
    State(state): State<AppState>,
    claims: axum::extract::Extension<crate::middleware::auth::Claims>,
    Json(req): Json<serde_json::Value>,
) -> Result<Json<UserPublic>> {
    let bio = req["bio"].as_str();
    let avatar_url = req["avatar_url"].as_str();
    let new_username = req["username"].as_str().map(str::trim).filter(|s| !s.is_empty());

    // ── Username change gate ───────────────────────────────────────────────
    if let Some(uname) = new_username {
        if read_setting(&state.db, "profile_allow_username_change", "true").await != "true" {
            return Err(AppError::Forbidden(
                "Username changes are not allowed.".to_string(),
            ));
        }
        if uname.len() < 3 {
            return Err(AppError::BadRequest(
                "Username must be at least 3 characters".to_string(),
            ));
        }
        let taken: i64 = sqlx::query_scalar(
            "SELECT COUNT(*) FROM users WHERE username = $1 AND id != $2",
        )
        .bind(uname)
        .bind(claims.sub)
        .fetch_one(&state.db)
        .await?;
        if taken > 0 {
            return Err(AppError::Conflict("Username already taken".to_string()));
        }
    }

    let user = sqlx::query_as::<_, crate::models::user::User>(
        r#"UPDATE users SET
            username   = COALESCE($1, username),
            bio        = COALESCE($2, bio),
            avatar_url = COALESCE($3, avatar_url),
            updated_at = NOW()
           WHERE id = $4
           RETURNING *"#,
    )
    .bind(new_username)
    .bind(bio)
    .bind(avatar_url)
    .bind(claims.sub)
    .fetch_one(&state.db)
    .await?;

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

    // ── Password policy ────────────────────────────────────────────────────
    validate_password_policy(&state.db, new_password).await?;

    let user = sqlx::query_as::<_, crate::models::user::User>(
        "SELECT * FROM users WHERE id = $1",
    )
    .bind(claims.sub)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("User not found".to_string()))?;

    let password_hash = user.password_hash.as_deref().ok_or_else(|| {
        AppError::BadRequest(
            "This account uses SSO login and has no local password.".to_string(),
        )
    })?;

    let valid = verify(old_password, password_hash)
        .map_err(|e| AppError::Internal(anyhow::anyhow!("{}", e)))?;

    if !valid {
        return Err(AppError::Unauthorized(
            "Incorrect current password".to_string(),
        ));
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
