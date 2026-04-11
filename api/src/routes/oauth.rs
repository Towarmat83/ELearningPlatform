use axum::{extract::State, Json};
use chrono::Utc;
use jsonwebtoken::{decode, encode, DecodingKey, EncodingKey, Header, Validation};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    middleware::auth::create_token,
    models::user::{AuthResponse, User, UserPublic},
    routes::settings::read_setting,
    AppState,
};

// ── State JWT (CSRF protection) ───────────────────────────────────────────────

#[derive(Debug, Serialize, Deserialize)]
struct OAuthStateClaims {
    pub provider: String,
    pub exp: i64,
}

fn make_state(provider: &str, secret: &str) -> anyhow::Result<String> {
    let claims = OAuthStateClaims {
        provider: provider.to_string(),
        exp: Utc::now().timestamp() + 600, // 10 minutes
    };
    encode(
        &Header::default(),
        &claims,
        &EncodingKey::from_secret(secret.as_bytes()),
    )
    .map_err(|e| anyhow::anyhow!("State JWT error: {}", e))
}

fn decode_state(state: &str, secret: &str) -> Option<String> {
    let mut validation = Validation::default();
    validation.validate_exp = true;
    decode::<OAuthStateClaims>(
        state,
        &DecodingKey::from_secret(secret.as_bytes()),
        &validation,
    )
    .ok()
    .map(|d| d.claims.provider)
}

// ── Request / Response types ──────────────────────────────────────────────────

#[derive(Deserialize)]
pub struct CallbackRequest {
    pub code: String,
    pub state: String,
}

// ── Handlers ──────────────────────────────────────────────────────────────────

/// GET /api/auth/oauth/providers
/// Returns the list of enabled OAuth providers.
pub async fn list_providers(State(state): State<AppState>) -> Json<Value> {
    let mut providers: Vec<Value> = vec![];
    if state.config.gitlab_client_id.is_some() {
        providers.push(json!({ "id": "gitlab", "name": "GitLab" }));
    }
    if state.config.github_client_id.is_some() {
        providers.push(json!({ "id": "github", "name": "GitHub" }));
    }
    Json(json!({ "providers": providers }))
}

/// GET /api/auth/oauth/:provider/authorize
/// Returns the provider authorization URL + a signed CSRF state token.
pub async fn authorize(
    State(state): State<AppState>,
    axum::extract::Path(provider): axum::extract::Path<String>,
) -> Result<Json<Value>> {
    let state_token = make_state(&provider, &state.config.jwt_secret)
        .map_err(|e| AppError::Internal(e))?;

    let redirect_uri = format!("{}/auth/callback", state.config.oauth_redirect_base);

    let url = match provider.as_str() {
        "gitlab" => {
            let client_id = state
                .config
                .gitlab_client_id
                .as_ref()
                .ok_or_else(|| AppError::BadRequest("GitLab OAuth not configured".to_string()))?;

            let gitlab_url = read_setting(&state.db, "gitlab_url", &state.config.gitlab_url).await;
            let mut url = reqwest::Url::parse(&format!("{}/oauth/authorize", gitlab_url))
                .map_err(|e| AppError::Internal(anyhow::anyhow!("Invalid GitLab URL: {}", e)))?;
            url.query_pairs_mut()
                .append_pair("client_id", client_id)
                .append_pair("redirect_uri", &redirect_uri)
                .append_pair("response_type", "code")
                .append_pair("scope", "read_user")
                .append_pair("state", &state_token);
            url.to_string()
        }
        "github" => {
            let client_id = state
                .config
                .github_client_id
                .as_ref()
                .ok_or_else(|| AppError::BadRequest("GitHub OAuth not configured".to_string()))?;

            let mut url =
                reqwest::Url::parse("https://github.com/login/oauth/authorize").unwrap();
            url.query_pairs_mut()
                .append_pair("client_id", client_id)
                .append_pair("redirect_uri", &redirect_uri)
                .append_pair("scope", "user:email read:user")
                .append_pair("state", &state_token);
            url.to_string()
        }
        _ => {
            return Err(AppError::BadRequest(format!(
                "Unknown provider: {}",
                provider
            )))
        }
    };

    Ok(Json(json!({ "url": url, "state": state_token })))
}

/// POST /api/auth/oauth/callback
/// Exchange authorization code for a platform JWT. Provider is decoded from the state token.
pub async fn callback(
    State(state): State<AppState>,
    Json(req): Json<CallbackRequest>,
) -> Result<Json<AuthResponse>> {
    // Verify state and extract provider
    let provider = decode_state(&req.state, &state.config.jwt_secret)
        .ok_or_else(|| AppError::Unauthorized("Invalid or expired OAuth state".to_string()))?;

    let redirect_uri = format!("{}/auth/callback", state.config.oauth_redirect_base);
    let http = reqwest::Client::new();

    let (email, display_name, avatar_url, provider_user_id) = match provider.as_str() {
        "gitlab" => fetch_gitlab(&state, &http, &req.code, &redirect_uri).await?,
        "github" => fetch_github(&state, &http, &req.code, &redirect_uri).await?,
        _ => {
            return Err(AppError::BadRequest(format!(
                "Unknown provider: {}",
                provider
            )))
        }
    };

    let user = upsert_sso_user(&state.db, &email, &display_name, avatar_url, &provider, &provider_user_id).await?;
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

// ── Provider helpers ──────────────────────────────────────────────────────────

async fn fetch_gitlab(
    state: &AppState,
    http: &reqwest::Client,
    code: &str,
    redirect_uri: &str,
) -> Result<(String, String, Option<String>, String)> {
    let client_id = state
        .config
        .gitlab_client_id
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("GitLab OAuth not configured".to_string()))?;
    let client_secret = state
        .config
        .gitlab_client_secret
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("GitLab OAuth not configured".to_string()))?;

    let gitlab_url = read_setting(&state.db, "gitlab_url", &state.config.gitlab_url).await;
    let gitlab_url = gitlab_url.trim_end_matches('/').to_string();

    // Exchange code → access token
    let token_res: Value = http
        .post(format!("{}/oauth/token", gitlab_url))
        .form(&[
            ("client_id", client_id.as_str()),
            ("client_secret", client_secret.as_str()),
            ("code", code),
            ("grant_type", "authorization_code"),
            ("redirect_uri", redirect_uri),
        ])
        .send()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitLab token request failed: {}", e)))?
        .json()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitLab token parse failed: {}", e)))?;

    let access_token = token_res["access_token"]
        .as_str()
        .ok_or_else(|| AppError::Unauthorized("GitLab did not return an access token".to_string()))?
        .to_string();

    // Fetch user profile (includes email when scope = read_user)
    let info: Value = http
        .get(format!("{}/api/v4/user", gitlab_url))
        .bearer_auth(&access_token)
        .send()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitLab user request failed: {}", e)))?
        .json()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitLab user parse failed: {}", e)))?;

    let id = info["id"]
        .as_i64()
        .ok_or_else(|| AppError::Internal(anyhow::anyhow!("GitLab user missing 'id'")))?
        .to_string();
    let email = info["email"]
        .as_str()
        .ok_or_else(|| AppError::Internal(anyhow::anyhow!("GitLab user missing 'email'")))?
        .to_string();
    let name = info["name"]
        .as_str()
        .or_else(|| info["username"].as_str())
        .unwrap_or(&email)
        .to_string();
    let avatar = info["avatar_url"].as_str().map(|s| s.to_string());

    Ok((email, name, avatar, id))
}

async fn fetch_github(
    state: &AppState,
    http: &reqwest::Client,
    code: &str,
    redirect_uri: &str,
) -> Result<(String, String, Option<String>, String)> {
    let client_id = state
        .config
        .github_client_id
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("GitHub OAuth not configured".to_string()))?;
    let client_secret = state
        .config
        .github_client_secret
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("GitHub OAuth not configured".to_string()))?;

    // Exchange code → access token (GitHub returns form-encoded or JSON depending on Accept header)
    let token_res: Value = http
        .post("https://github.com/login/oauth/access_token")
        .header("Accept", "application/json")
        .form(&[
            ("client_id", client_id.as_str()),
            ("client_secret", client_secret.as_str()),
            ("code", code),
            ("redirect_uri", redirect_uri),
        ])
        .send()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub token request failed: {}", e)))?
        .json()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub token parse failed: {}", e)))?;

    let access_token = token_res["access_token"]
        .as_str()
        .ok_or_else(|| AppError::Unauthorized("GitHub did not return an access token".to_string()))?
        .to_string();

    // Fetch user profile
    let profile: Value = http
        .get("https://api.github.com/user")
        .bearer_auth(&access_token)
        .header("User-Agent", "LearnLab-SSO/1.0")
        .header("Accept", "application/vnd.github.v3+json")
        .send()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub user request failed: {}", e)))?
        .json()
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub user parse failed: {}", e)))?;

    let github_id = profile["id"]
        .as_i64()
        .ok_or_else(|| AppError::Internal(anyhow::anyhow!("GitHub profile missing 'id'")))?
        .to_string();
    let login = profile["login"].as_str().unwrap_or("user").to_string();
    let name = profile["name"].as_str().unwrap_or(&login).to_string();
    let avatar = profile["avatar_url"].as_str().map(|s| s.to_string());

    // GitHub may not expose email in profile if it's private — fetch emails endpoint
    let email = if let Some(e) = profile["email"].as_str().filter(|e| !e.is_empty()) {
        e.to_string()
    } else {
        let emails: Vec<Value> = http
            .get("https://api.github.com/user/emails")
            .bearer_auth(&access_token)
            .header("User-Agent", "LearnLab-SSO/1.0")
            .header("Accept", "application/vnd.github.v3+json")
            .send()
            .await
            .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub emails failed: {}", e)))?
            .json()
            .await
            .map_err(|e| AppError::Internal(anyhow::anyhow!("GitHub emails parse failed: {}", e)))?;

        emails
            .iter()
            .find(|e| e["primary"].as_bool() == Some(true) && e["verified"].as_bool() == Some(true))
            .or_else(|| emails.first())
            .and_then(|e| e["email"].as_str())
            .map(|s| s.to_string())
            .ok_or_else(|| AppError::BadRequest("Could not retrieve a verified GitHub email address".to_string()))?
    };

    Ok((email, name, avatar, github_id))
}

// ── User upsert ───────────────────────────────────────────────────────────────

/// Find or create a user for an SSO login.
/// Strategy:
///   1. Match by (auth_provider, provider_user_id) → most reliable, update avatar
///   2. Match by email (existing local account) → link provider to that account
///   3. No match → create a new account
async fn upsert_sso_user(
    db: &sqlx::PgPool,
    email: &str,
    display_name: &str,
    avatar_url: Option<String>,
    provider: &str,
    provider_user_id: &str,
) -> Result<User> {
    // 1. Exact match on (provider, provider_user_id)
    let existing: Option<User> = sqlx::query_as::<_, User>(
        "SELECT * FROM users WHERE auth_provider = $1 AND provider_user_id = $2",
    )
    .bind(provider)
    .bind(provider_user_id)
    .fetch_optional(db)
    .await?;

    if let Some(user) = existing {
        // Update avatar in case it changed
        let updated = sqlx::query_as::<_, User>(
            "UPDATE users SET avatar_url = COALESCE($1, avatar_url), updated_at = NOW()
             WHERE id = $2 RETURNING *",
        )
        .bind(avatar_url.as_deref())
        .bind(user.id)
        .fetch_one(db)
        .await?;
        return Ok(updated);
    }

    // 2. Match by email (link existing local account)
    let by_email: Option<User> =
        sqlx::query_as::<_, User>("SELECT * FROM users WHERE email = $1")
            .bind(email)
            .fetch_optional(db)
            .await?;

    if let Some(user) = by_email {
        let linked = sqlx::query_as::<_, User>(
            "UPDATE users
             SET auth_provider = $1, provider_user_id = $2,
                 avatar_url = COALESCE($3, avatar_url), updated_at = NOW()
             WHERE id = $4 RETURNING *",
        )
        .bind(provider)
        .bind(provider_user_id)
        .bind(avatar_url.as_deref())
        .bind(user.id)
        .fetch_one(db)
        .await?;
        return Ok(linked);
    }

    // 3. Create new user
    let username = sanitize_username(display_name);
    let username = ensure_unique_username(db, &username).await?;

    let user = sqlx::query_as::<_, User>(
        r#"INSERT INTO users (username, email, auth_provider, provider_user_id, role, avatar_url)
           VALUES ($1, $2, $3, $4, 'student', $5)
           RETURNING *"#,
    )
    .bind(&username)
    .bind(email)
    .bind(provider)
    .bind(provider_user_id)
    .bind(avatar_url.as_deref())
    .fetch_one(db)
    .await?;

    crate::metrics::ACTIVE_USERS.inc();
    Ok(user)
}

fn sanitize_username(name: &str) -> String {
    let slug: String = name
        .chars()
        .filter(|c| c.is_alphanumeric() || *c == '_' || *c == '-')
        .take(32)
        .collect();
    if slug.is_empty() { "user".to_string() } else { slug }
}

async fn ensure_unique_username(db: &sqlx::PgPool, base: &str) -> Result<String> {
    let count: i64 =
        sqlx::query_scalar("SELECT COUNT(*) FROM users WHERE username = $1")
            .bind(base)
            .fetch_one(db)
            .await?;
    if count == 0 {
        return Ok(base.to_string());
    }
    // Append a short UUID suffix
    Ok(format!("{}_{}", base, &Uuid::new_v4().simple().to_string()[..6]))
}
