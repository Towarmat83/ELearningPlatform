use axum::{
    extract::{Path, State},
    Json,
};
use serde_json::{json, Value};
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    models::user::UpdateUserRequest,
    AppState,
};

pub async fn list_users(State(state): State<AppState>) -> Result<Json<Value>> {
    let users = sqlx::query!(
        r#"SELECT
            u.id, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio, u.created_at,
            COUNT(DISTINCT e.course_id) as enrolled_courses,
            COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE) as completed_labs
           FROM users u
           LEFT JOIN enrollments e ON e.user_id = u.id
           LEFT JOIN lab_progress lp ON lp.user_id = u.id
           GROUP BY u.id
           ORDER BY u.created_at DESC"#
    )
    .fetch_all(&state.db)
    .await?;

    let result: Vec<Value> = users
        .iter()
        .map(|u| {
            json!({
                "id": u.id,
                "username": u.username,
                "email": u.email,
                "role": u.role,
                "is_active": u.is_active,
                "avatar_url": u.avatar_url,
                "bio": u.bio,
                "created_at": u.created_at,
                "enrolled_courses": u.enrolled_courses.unwrap_or(0),
                "completed_labs": u.completed_labs.unwrap_or(0),
            })
        })
        .collect();

    Ok(Json(json!({"users": result, "total": result.len()})))
}

pub async fn get_user(
    State(state): State<AppState>,
    Path(user_id): Path<Uuid>,
) -> Result<Json<Value>> {
    let user = sqlx::query!(
        r#"SELECT
            u.id, u.username, u.email, u.role, u.is_active, u.avatar_url, u.bio, u.created_at,
            COUNT(DISTINCT e.course_id) as enrolled_courses,
            COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE) as completed_labs,
            COALESCE(SUM(lp.best_score), 0) as total_score
           FROM users u
           LEFT JOIN enrollments e ON e.user_id = u.id
           LEFT JOIN lab_progress lp ON lp.user_id = u.id
           WHERE u.id = $1
           GROUP BY u.id"#,
        user_id
    )
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("User not found".to_string()))?;

    Ok(Json(json!({
        "id": user.id,
        "username": user.username,
        "email": user.email,
        "role": user.role,
        "is_active": user.is_active,
        "avatar_url": user.avatar_url,
        "bio": user.bio,
        "created_at": user.created_at,
        "enrolled_courses": user.enrolled_courses.unwrap_or(0),
        "completed_labs": user.completed_labs.unwrap_or(0),
        "total_score": user.total_score.unwrap_or(0),
    })))
}

pub async fn update_user(
    State(state): State<AppState>,
    Path(user_id): Path<Uuid>,
    Json(req): Json<UpdateUserRequest>,
) -> Result<Json<Value>> {
    // Validate role if provided
    if let Some(ref role) = req.role {
        if role != "admin" && role != "student" {
            return Err(AppError::BadRequest("Role must be 'admin' or 'student'".to_string()));
        }
    }

    let user = sqlx::query!(
        r#"UPDATE users SET
            username = COALESCE($1, username),
            bio = COALESCE($2, bio),
            avatar_url = COALESCE($3, avatar_url),
            is_active = COALESCE($4, is_active),
            role = COALESCE($5, role),
            updated_at = NOW()
           WHERE id = $6
           RETURNING id, username, email, role, is_active, avatar_url, bio, created_at"#,
        req.username,
        req.bio,
        req.avatar_url,
        req.is_active,
        req.role,
        user_id
    )
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("User not found".to_string()))?;

    Ok(Json(json!({
        "id": user.id,
        "username": user.username,
        "email": user.email,
        "role": user.role,
        "is_active": user.is_active,
        "avatar_url": user.avatar_url,
        "bio": user.bio,
        "created_at": user.created_at,
    })))
}

pub async fn delete_user(
    State(state): State<AppState>,
    Path(user_id): Path<Uuid>,
) -> Result<Json<Value>> {
    // Prevent deleting yourself — handled in route with claims check
    sqlx::query!("DELETE FROM users WHERE id = $1", user_id)
        .execute(&state.db)
        .await?;

    Ok(Json(json!({"message": "User deleted"})))
}
