use axum::{
    extract::{Path, State},
    Json,
};
use serde_json::{json, Value};
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    middleware::auth::Claims,
    models::lab::{CreateLabRequest, Lab, LabStudent, UpdateLabRequest},
    AppState,
};

pub async fn list_labs(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    // Check enrollment (unless admin)
    if claims.role != "admin" {
        let enrolled = sqlx::query_scalar!(
            "SELECT COUNT(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
            claims.sub,
            course_id
        )
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

        if enrolled == 0 {
            return Err(AppError::Forbidden("You must enroll in this course first".to_string()));
        }
    }

    let is_admin = claims.role == "admin";

    // Admins see all labs (incl. drafts), students only see published
    let labs = if is_admin {
        sqlx::query_as::<_, Lab>(
            "SELECT * FROM labs WHERE course_id = $1 ORDER BY order_index ASC",
        )
        .bind(course_id)
        .fetch_all(&state.db)
        .await?
    } else {
        sqlx::query_as::<_, Lab>(
            "SELECT * FROM labs WHERE course_id = $1 AND is_published = TRUE ORDER BY order_index ASC",
        )
        .bind(course_id)
        .fetch_all(&state.db)
        .await?
    };

    // Strip correct answers for students (admins keep full data via admin_get_lab)
    let labs: Vec<LabStudent> = labs.into_iter().map(LabStudent::from).collect();

    Ok(Json(json!({"labs": labs})))
}

pub async fn get_lab(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    if claims.role != "admin" {
        let enrolled = sqlx::query_scalar!(
            "SELECT COUNT(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
            claims.sub,
            course_id
        )
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

        if enrolled == 0 {
            return Err(AppError::Forbidden("You must enroll in this course first".to_string()));
        }
    }

    let filter = if claims.role == "admin" {
        "SELECT * FROM labs WHERE id = $1 AND course_id = $2"
    } else {
        "SELECT * FROM labs WHERE id = $1 AND course_id = $2 AND is_published = TRUE"
    };

    let lab = sqlx::query_as::<_, Lab>(filter)
        .bind(lab_id)
        .bind(course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Lab not found".to_string()))?;

    // Get user's progress for this lab
    let progress = sqlx::query!(
        "SELECT completed, best_score, total_attempts, completed_at FROM lab_progress WHERE user_id = $1 AND lab_id = $2",
        claims.sub,
        lab_id
    )
    .fetch_optional(&state.db)
    .await?;

    let lab_student = LabStudent::from(lab);

    Ok(Json(json!({
        "lab": lab_student,
        "progress": progress.map(|p| json!({
            "completed": p.completed,
            "best_score": p.best_score,
            "total_attempts": p.total_attempts,
            "completed_at": p.completed_at,
        }))
    })))
}

pub async fn create_lab(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
    Json(req): Json<CreateLabRequest>,
) -> Result<Json<Value>> {
    // Validate lab type
    if req.lab_type != "form" && req.lab_type != "ctf" {
        return Err(AppError::BadRequest("lab_type must be 'form' or 'ctf'".to_string()));
    }

    // CTF labs must have a flag
    if req.lab_type == "ctf" && req.flag.is_none() {
        return Err(AppError::BadRequest("CTF labs require a flag".to_string()));
    }

    // Check course ownership
    let course = sqlx::query!("SELECT created_by FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    if claims.role != "admin" && course.created_by != claims.sub {
        return Err(AppError::Forbidden("You don't own this course".to_string()));
    }

    let lab = sqlx::query_as::<_, Lab>(
        r#"INSERT INTO labs (course_id, title, description, lab_type, content, flag, points, order_index, is_published)
           VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
           RETURNING *"#,
    )
    .bind(course_id)
    .bind(&req.title)
    .bind(&req.description)
    .bind(&req.lab_type)
    .bind(&req.content)
    .bind(&req.flag)
    .bind(req.points.unwrap_or(100))
    .bind(req.order_index.unwrap_or(0))
    .bind(req.is_published.unwrap_or(false))
    .fetch_one(&state.db)
    .await?;

    Ok(Json(json!(lab)))
}

pub async fn update_lab(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
    Json(req): Json<UpdateLabRequest>,
) -> Result<Json<Value>> {
    // Check course ownership
    let course = sqlx::query!("SELECT created_by FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    if claims.role != "admin" && course.created_by != claims.sub {
        return Err(AppError::Forbidden("You don't own this course".to_string()));
    }

    let lab = sqlx::query_as::<_, Lab>(
        r#"UPDATE labs SET
            title = COALESCE($1, title),
            description = COALESCE($2, description),
            content = COALESCE($3, content),
            flag = COALESCE($4, flag),
            points = COALESCE($5, points),
            order_index = COALESCE($6, order_index),
            is_published = COALESCE($7, is_published),
            updated_at = NOW()
           WHERE id = $8 AND course_id = $9
           RETURNING *"#,
    )
    .bind(req.title)
    .bind(req.description)
    .bind(req.content)
    .bind(req.flag)
    .bind(req.points)
    .bind(req.order_index)
    .bind(req.is_published)
    .bind(lab_id)
    .bind(course_id)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("Lab not found".to_string()))?;

    Ok(Json(json!(lab)))
}

pub async fn delete_lab(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let course = sqlx::query!("SELECT created_by FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    if claims.role != "admin" && course.created_by != claims.sub {
        return Err(AppError::Forbidden("You don't own this course".to_string()));
    }

    sqlx::query!("DELETE FROM labs WHERE id = $1 AND course_id = $2", lab_id, course_id)
        .execute(&state.db)
        .await?;

    Ok(Json(json!({"message": "Lab deleted"})))
}

/// Admin: get lab with flag and correct answers exposed
pub async fn admin_get_lab(
    State(state): State<AppState>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let lab = sqlx::query_as::<_, Lab>(
        "SELECT * FROM labs WHERE id = $1 AND course_id = $2",
    )
    .bind(lab_id)
    .bind(course_id)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("Lab not found".to_string()))?;

    // Admin sees everything including flag
    Ok(Json(json!({
        "id": lab.id,
        "course_id": lab.course_id,
        "title": lab.title,
        "description": lab.description,
        "lab_type": lab.lab_type,
        "content": lab.content,
        "flag": lab.flag,
        "points": lab.points,
        "order_index": lab.order_index,
        "is_published": lab.is_published,
        "created_at": lab.created_at,
        "updated_at": lab.updated_at,
    })))
}
