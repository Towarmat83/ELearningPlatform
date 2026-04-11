use axum::{extract::State, Json};
use serde_json::{json, Value};

use crate::{error::{AppError, Result}, AppState};

const ALLOWED_KEYS: &[&str] = &[
    "gitlab_url",
    "registration_enabled",
    "registration_email_whitelist",
    "password_min_length",
    "password_require_uppercase",
    "password_require_number",
    "profile_allow_username_change",
    "sso_local_login_enabled",
];

/// GET /api/admin/settings
pub async fn get_settings(State(state): State<AppState>) -> Result<Json<Value>> {
    let rows = sqlx::query("SELECT key, value, description FROM platform_settings ORDER BY key")
        .fetch_all(&state.db)
        .await?;

    let settings: Vec<Value> = rows
        .iter()
        .map(|r| {
            use sqlx::Row;
            json!({
                "key":         r.get::<String, _>("key"),
                "value":       r.get::<String, _>("value"),
                "description": r.get::<Option<String>, _>("description"),
            })
        })
        .collect();

    Ok(Json(json!({ "settings": settings })))
}

/// PUT /api/admin/settings — body: { "key": "value", ... }
pub async fn update_settings(
    State(state): State<AppState>,
    Json(body): Json<Value>,
) -> Result<Json<Value>> {
    let map = body
        .as_object()
        .ok_or_else(|| AppError::BadRequest("Expected a JSON object { key: value }".to_string()))?;

    if map.is_empty() {
        return Err(AppError::BadRequest("No settings provided".to_string()));
    }

    for key in map.keys() {
        if !ALLOWED_KEYS.contains(&key.as_str()) {
            return Err(AppError::BadRequest(format!("Unknown setting key: '{}'", key)));
        }
    }

    for (key, val) in map {
        let value = val
            .as_str()
            .ok_or_else(|| AppError::BadRequest(format!("Setting '{}' must be a string", key)))?
            .trim();

        sqlx::query(
            "INSERT INTO platform_settings (key, value, updated_at)
             VALUES ($1, $2, NOW())
             ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()",
        )
        .bind(key)
        .bind(value)
        .execute(&state.db)
        .await?;
    }

    get_settings(State(state)).await
}

/// GET /api/settings/public — non-sensitive subset for the frontend
pub async fn public_settings(State(state): State<AppState>) -> Json<Value> {
    const PUBLIC_KEYS: &[&str] = &[
        "registration_enabled",
        "sso_local_login_enabled",
        "password_min_length",
        "password_require_uppercase",
        "password_require_number",
    ];

    let mut out = serde_json::Map::new();
    for key in PUBLIC_KEYS {
        let val = read_setting(&state.db, key, "").await;
        out.insert(key.to_string(), Value::String(val));
    }
    Json(Value::Object(out))
}

/// Helper: fetch a single setting value with a fallback default.
pub async fn read_setting(db: &sqlx::PgPool, key: &str, fallback: &str) -> String {
    sqlx::query_scalar("SELECT value FROM platform_settings WHERE key = $1")
        .bind(key)
        .fetch_optional(db)
        .await
        .ok()
        .flatten()
        .unwrap_or_else(|| fallback.to_string())
}
