use axum::{
    extract::{Path, Query, State},
    Json,
};
use serde_json::{json, Value};
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    middleware::auth::Claims,
    models::course::{CourseFilter, CreateCourseRequest, UpdateCourseRequest},
    AppState,
};

pub async fn list_courses(
    State(state): State<AppState>,
    Query(filter): Query<CourseFilter>,
) -> Result<Json<Value>> {
    let page = filter.page.unwrap_or(1).max(1);
    let per_page = filter.per_page.unwrap_or(20).min(100);
    let offset = (page - 1) * per_page;

    let rows = sqlx::query!(
        r#"SELECT
            c.id, c.title, c.description, c.thumbnail, c.category,
            c.difficulty, c.is_published, c.created_by, c.created_at, c.updated_at,
            u.username as creator_username,
            COUNT(DISTINCT l.id) as lab_count,
            COUNT(DISTINCT e.id) as enrollment_count
           FROM courses c
           LEFT JOIN users u ON u.id = c.created_by
           LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
           LEFT JOIN enrollments e ON e.course_id = c.id
           WHERE c.is_published = TRUE
             AND ($1::text IS NULL OR c.category = $1)
             AND ($2::text IS NULL OR c.difficulty = $2)
             AND ($3::text IS NULL OR c.title ILIKE '%' || $3 || '%' OR c.description ILIKE '%' || $3 || '%')
           GROUP BY c.id, u.username
           ORDER BY c.created_at DESC
           LIMIT $4 OFFSET $5"#,
        filter.category,
        filter.difficulty,
        filter.search,
        per_page,
        offset
    )
    .fetch_all(&state.db)
    .await?;

    let total: i64 = sqlx::query_scalar!(
        r#"SELECT COUNT(*) FROM courses WHERE is_published = TRUE
           AND ($1::text IS NULL OR category = $1)
           AND ($2::text IS NULL OR difficulty = $2)
           AND ($3::text IS NULL OR title ILIKE '%' || $3 || '%')"#,
        filter.category,
        filter.difficulty,
        filter.search
    )
    .fetch_one(&state.db)
    .await?
    .unwrap_or(0);

    let courses: Vec<Value> = rows
        .iter()
        .map(|r| {
            json!({
                "id": r.id,
                "title": r.title,
                "description": r.description,
                "thumbnail": r.thumbnail,
                "category": r.category,
                "difficulty": r.difficulty,
                "is_published": r.is_published,
                "created_by": r.created_by,
                "creator_username": r.creator_username,
                "lab_count": r.lab_count.unwrap_or(0),
                "enrollment_count": r.enrollment_count.unwrap_or(0),
                "created_at": r.created_at,
                "updated_at": r.updated_at,
            })
        })
        .collect();

    Ok(Json(json!({
        "courses": courses,
        "total": total,
        "page": page,
        "per_page": per_page,
        "total_pages": (total as f64 / per_page as f64).ceil() as i64,
    })))
}

pub async fn get_course(
    State(state): State<AppState>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    let row = sqlx::query!(
        r#"SELECT
            c.id, c.title, c.description, c.thumbnail, c.category,
            c.difficulty, c.is_published, c.created_by, c.created_at, c.updated_at,
            u.username as creator_username,
            COUNT(DISTINCT l.id) as lab_count,
            COUNT(DISTINCT e.id) as enrollment_count
           FROM courses c
           LEFT JOIN users u ON u.id = c.created_by
           LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
           LEFT JOIN enrollments e ON e.course_id = c.id
           WHERE c.id = $1 AND c.is_published = TRUE
           GROUP BY c.id, u.username"#,
        course_id
    )
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    Ok(Json(json!({
        "id": row.id,
        "title": row.title,
        "description": row.description,
        "thumbnail": row.thumbnail,
        "category": row.category,
        "difficulty": row.difficulty,
        "is_published": row.is_published,
        "created_by": row.created_by,
        "creator_username": row.creator_username,
        "lab_count": row.lab_count.unwrap_or(0),
        "enrollment_count": row.enrollment_count.unwrap_or(0),
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    })))
}

pub async fn create_course(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Json(req): Json<CreateCourseRequest>,
) -> Result<Json<Value>> {
    if req.title.is_empty() {
        return Err(AppError::BadRequest("Title is required".to_string()));
    }

    let row = sqlx::query!(
        r#"INSERT INTO courses (title, description, thumbnail, category, difficulty, is_published, created_by)
           VALUES ($1, $2, $3, $4, $5, $6, $7)
           RETURNING id, title, description, thumbnail, category, difficulty, is_published, created_by, created_at, updated_at"#,
        req.title,
        req.description,
        req.thumbnail,
        req.category,
        req.difficulty,
        req.is_published.unwrap_or(false),
        claims.sub
    )
    .fetch_one(&state.db)
    .await?;

    crate::metrics::ACTIVE_COURSES.inc();

    Ok(Json(json!({
        "id": row.id,
        "title": row.title,
        "description": row.description,
        "thumbnail": row.thumbnail,
        "category": row.category,
        "difficulty": row.difficulty,
        "is_published": row.is_published,
        "created_by": row.created_by,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    })))
}

pub async fn update_course(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
    Json(req): Json<UpdateCourseRequest>,
) -> Result<Json<Value>> {
    // Check ownership or admin
    let course = sqlx::query!("SELECT created_by FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    if claims.role != "admin" && course.created_by != claims.sub {
        return Err(AppError::Forbidden("You don't own this course".to_string()));
    }

    let row = sqlx::query!(
        r#"UPDATE courses SET
            title = COALESCE($1, title),
            description = COALESCE($2, description),
            thumbnail = COALESCE($3, thumbnail),
            category = COALESCE($4, category),
            difficulty = COALESCE($5, difficulty),
            is_published = COALESCE($6, is_published),
            updated_at = NOW()
           WHERE id = $7
           RETURNING id, title, description, thumbnail, category, difficulty, is_published, created_by, created_at, updated_at"#,
        req.title,
        req.description,
        req.thumbnail,
        req.category,
        req.difficulty,
        req.is_published,
        course_id
    )
    .fetch_one(&state.db)
    .await?;

    Ok(Json(json!({
        "id": row.id,
        "title": row.title,
        "description": row.description,
        "thumbnail": row.thumbnail,
        "category": row.category,
        "difficulty": row.difficulty,
        "is_published": row.is_published,
        "created_by": row.created_by,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    })))
}

pub async fn delete_course(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    let course = sqlx::query!("SELECT created_by FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    if claims.role != "admin" && course.created_by != claims.sub {
        return Err(AppError::Forbidden("You don't own this course".to_string()));
    }

    sqlx::query!("DELETE FROM courses WHERE id = $1", course_id)
        .execute(&state.db)
        .await?;

    crate::metrics::ACTIVE_COURSES.dec();

    Ok(Json(json!({"message": "Course deleted"})))
}

pub async fn enroll(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    // Check course exists and is published
    let exists = sqlx::query_scalar!(
        "SELECT COUNT(*) FROM courses WHERE id = $1 AND is_published = TRUE",
        course_id
    )
    .fetch_one(&state.db)
    .await?
    .unwrap_or(0);

    if exists == 0 {
        return Err(AppError::NotFound("Course not found".to_string()));
    }

    sqlx::query!(
        "INSERT INTO enrollments (user_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
        claims.sub,
        course_id
    )
    .execute(&state.db)
    .await?;

    crate::metrics::ENROLLMENTS_TOTAL.inc();

    Ok(Json(json!({"message": "Enrolled successfully"})))
}

pub async fn unenroll(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    sqlx::query!(
        "DELETE FROM enrollments WHERE user_id = $1 AND course_id = $2",
        claims.sub,
        course_id
    )
    .execute(&state.db)
    .await?;

    crate::metrics::ENROLLMENTS_TOTAL.dec();

    Ok(Json(json!({"message": "Unenrolled successfully"})))
}

pub async fn my_courses(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
) -> Result<Json<Value>> {
    let rows = sqlx::query!(
        r#"SELECT
            c.id, c.title, c.description, c.thumbnail, c.category, c.difficulty,
            c.is_published, c.created_by, c.created_at, c.updated_at,
            COUNT(DISTINCT l.id) as lab_count,
            COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE) as completed_labs,
            COALESCE(SUM(lp.best_score), 0) as total_score
           FROM enrollments e
           JOIN courses c ON c.id = e.course_id
           LEFT JOIN labs l ON l.course_id = c.id AND l.is_published = TRUE
           LEFT JOIN lab_progress lp ON lp.course_id = c.id AND lp.user_id = $1
           WHERE e.user_id = $1
           GROUP BY c.id
           ORDER BY MAX(e.enrolled_at) DESC"#,
        claims.sub
    )
    .fetch_all(&state.db)
    .await?;

    let courses: Vec<Value> = rows
        .iter()
        .map(|r| {
            json!({
                "id": r.id,
                "title": r.title,
                "description": r.description,
                "thumbnail": r.thumbnail,
                "category": r.category,
                "difficulty": r.difficulty,
                "is_published": r.is_published,
                "created_by": r.created_by,
                "created_at": r.created_at,
                "updated_at": r.updated_at,
                "lab_count": r.lab_count.unwrap_or(0),
                "completed_labs": r.completed_labs.unwrap_or(0),
                "total_score": r.total_score.unwrap_or(0),
            })
        })
        .collect();

    Ok(Json(json!({"courses": courses})))
}

/// Admin: list all courses (published + unpublished)
pub async fn admin_list_courses(
    State(state): State<AppState>,
    Query(filter): Query<CourseFilter>,
) -> Result<Json<Value>> {
    let page = filter.page.unwrap_or(1).max(1);
    let per_page = filter.per_page.unwrap_or(50).min(100);
    let offset = (page - 1) * per_page;

    let rows = sqlx::query!(
        r#"SELECT
            c.id, c.title, c.description, c.thumbnail, c.category,
            c.difficulty, c.is_published, c.created_by, c.created_at, c.updated_at,
            u.username as creator_username,
            COUNT(DISTINCT l.id) as lab_count,
            COUNT(DISTINCT e.id) as enrollment_count
           FROM courses c
           LEFT JOIN users u ON u.id = c.created_by
           LEFT JOIN labs l ON l.course_id = c.id
           LEFT JOIN enrollments e ON e.course_id = c.id
           WHERE ($1::text IS NULL OR c.title ILIKE '%' || $1 || '%')
           GROUP BY c.id, u.username
           ORDER BY c.created_at DESC
           LIMIT $2 OFFSET $3"#,
        filter.search,
        per_page,
        offset
    )
    .fetch_all(&state.db)
    .await?;

    let total: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM courses")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let courses: Vec<Value> = rows
        .iter()
        .map(|r| {
            json!({
                "id": r.id,
                "title": r.title,
                "description": r.description,
                "thumbnail": r.thumbnail,
                "category": r.category,
                "difficulty": r.difficulty,
                "is_published": r.is_published,
                "created_by": r.created_by,
                "creator_username": r.creator_username,
                "lab_count": r.lab_count.unwrap_or(0),
                "enrollment_count": r.enrollment_count.unwrap_or(0),
                "created_at": r.created_at,
                "updated_at": r.updated_at,
            })
        })
        .collect();

    Ok(Json(json!({
        "courses": courses,
        "total": total,
        "page": page,
        "per_page": per_page,
    })))
}

#[derive(serde::Deserialize)]
pub struct AdminEnrollRequest {
    pub user_id: Uuid,
}

pub async fn admin_list_enrollments(
    State(state): State<AppState>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    let rows = sqlx::query!(
        r#"SELECT u.id as "user_id", u.username, u.email, e.enrolled_at
           FROM enrollments e JOIN users u ON u.id = e.user_id
           WHERE e.course_id = $1 ORDER BY e.enrolled_at DESC"#,
        course_id
    )
    .fetch_all(&state.db)
    .await?;

    let enrollments: Vec<Value> = rows
        .iter()
        .map(|r| {
            json!({
                "user_id": r.user_id,
                "username": r.username,
                "email": r.email,
                "enrolled_at": r.enrolled_at,
            })
        })
        .collect();

    Ok(Json(json!({ "enrollments": enrollments })))
}

pub async fn admin_enroll_user(
    State(state): State<AppState>,
    Path(course_id): Path<Uuid>,
    Json(req): Json<AdminEnrollRequest>,
) -> Result<Json<Value>> {
    sqlx::query!(
        "INSERT INTO enrollments (user_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
        req.user_id,
        course_id
    )
    .execute(&state.db)
    .await?;

    crate::metrics::ENROLLMENTS_TOTAL.inc();
    Ok(Json(json!({ "message": "User enrolled" })))
}

pub async fn admin_unenroll_user(
    State(state): State<AppState>,
    Path((course_id, user_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    sqlx::query!(
        "DELETE FROM enrollments WHERE course_id = $1 AND user_id = $2",
        course_id,
        user_id
    )
    .execute(&state.db)
    .await?;

    crate::metrics::ENROLLMENTS_TOTAL.dec();
    Ok(Json(json!({ "message": "User unenrolled" })))
}

pub async fn course_leaderboard(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    // Must be enrolled or admin
    if claims.role != "admin" {
        let enrolled: i64 = sqlx::query_scalar(
            "SELECT COUNT(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
        )
        .bind(claims.sub)
        .bind(course_id)
        .fetch_one(&state.db)
        .await?;

        if enrolled == 0 {
            return Err(AppError::Forbidden("Enroll to see the leaderboard".to_string()));
        }
    }

    use sqlx::Row as _;

    let rows = sqlx::query(
        r#"SELECT
            u.id::TEXT        AS user_id,
            u.username,
            COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE)::BIGINT AS completed_labs,
            COALESCE(SUM(lp.best_score), 0)::BIGINT                               AS total_points,
            MAX(lp.last_attempt_at)                                                AS last_activity
           FROM enrollments e
           JOIN users u ON u.id = e.user_id
           LEFT JOIN lab_progress lp ON lp.user_id = u.id AND lp.course_id = $1
           WHERE e.course_id = $1 AND u.is_active = TRUE
           GROUP BY u.id, u.username
           ORDER BY total_points DESC, completed_labs DESC
           LIMIT 20"#,
    )
    .bind(course_id)
    .fetch_all(&state.db)
    .await?;

    let my_id = claims.sub.to_string();

    let leaderboard: Vec<Value> = rows
        .iter()
        .enumerate()
        .map(|(i, r)| {
            let user_id: String = r.try_get("user_id").unwrap_or_default();
            let username: String = r.try_get("username").unwrap_or_default();
            let completed_labs: i64 = r.try_get("completed_labs").unwrap_or(0);
            let total_points: i64 = r.try_get("total_points").unwrap_or(0);
            let last_activity: Option<chrono::DateTime<chrono::Utc>> =
                r.try_get("last_activity").unwrap_or(None);
            json!({
                "rank": (i + 1) as i64,
                "user_id": user_id,
                "is_me": user_id == my_id,
                "username": username,
                "completed_labs": completed_labs,
                "total_points": total_points,
                "last_activity": last_activity,
            })
        })
        .collect();

    Ok(Json(json!({ "leaderboard": leaderboard })))
}
