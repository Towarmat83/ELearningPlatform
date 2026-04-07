use axum::{
    extract::{Path, State},
    Json,
};
use chrono::Utc;
use serde_json::{json, Value};
use sqlx::Row;
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    metrics::LAB_SUBMISSIONS_TOTAL,
    middleware::auth::Claims,
    models::{
        lab::Lab,
        submission::{
            CourseProgress, FlagResult, LabProgressSummary, QuestionResult, StudentProgressEntry,
            SubmissionResult,
        },
    },
    AppState,
};

pub async fn submit_lab(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
    Json(req): Json<crate::models::submission::SubmitLabRequest>,
) -> Result<Json<SubmissionResult>> {
    // Check enrollment
    let enrolled = sqlx::query_scalar!(
        "SELECT COUNT(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
        claims.sub,
        course_id
    )
    .fetch_one(&state.db)
    .await?
    .unwrap_or(0);

    if enrolled == 0 {
        return Err(AppError::Forbidden("You must enroll first".to_string()));
    }

    // Get the lab WITH the flag/correct answers (server-side only)
    let lab = sqlx::query_as::<_, Lab>(
        "SELECT * FROM labs WHERE id = $1 AND course_id = $2 AND is_published = TRUE",
    )
    .bind(lab_id)
    .bind(course_id)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("Lab not found".to_string()))?;

    let result = match lab.lab_type.as_str() {
        "ctf" => grade_ctf(&lab, &req.answer)?,
        "form" => grade_form(&lab, &req.answer)?,
        _ => return Err(AppError::Internal(anyhow::anyhow!("Unknown lab type"))),
    };

    let result_label = if result.is_correct { "correct" } else { "incorrect" };
    LAB_SUBMISSIONS_TOTAL.with_label_values(&[&lab.lab_type, result_label]).inc();

    // Get current attempts
    let current_progress = sqlx::query!(
        "SELECT total_attempts, best_score FROM lab_progress WHERE user_id = $1 AND lab_id = $2",
        claims.sub,
        lab_id
    )
    .fetch_optional(&state.db)
    .await?;

    let total_attempts = current_progress
        .as_ref()
        .map(|p| p.total_attempts + 1)
        .unwrap_or(1);

    let best_score = current_progress
        .as_ref()
        .map(|p| p.best_score.max(result.score))
        .unwrap_or(result.score);

    // Save submission
    sqlx::query!(
        r#"INSERT INTO lab_submissions (lab_id, user_id, answer, is_correct, score, attempts)
           VALUES ($1, $2, $3, $4, $5, $6)"#,
        lab_id,
        claims.sub,
        req.answer,
        result.is_correct,
        result.score,
        total_attempts
    )
    .execute(&state.db)
    .await?;

    // Upsert progress
    let completed_at = if result.is_correct { Some(Utc::now()) } else { None };

    sqlx::query!(
        r#"INSERT INTO lab_progress (user_id, lab_id, course_id, completed, best_score, total_attempts, completed_at, last_attempt_at)
           VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
           ON CONFLICT (user_id, lab_id) DO UPDATE SET
               completed = CASE WHEN lab_progress.completed THEN TRUE ELSE $4 END,
               best_score = $5,
               total_attempts = $6,
               completed_at = CASE WHEN lab_progress.completed_at IS NOT NULL THEN lab_progress.completed_at ELSE $7 END,
               last_attempt_at = NOW()"#,
        claims.sub,
        lab_id,
        course_id,
        result.is_correct,
        best_score,
        total_attempts,
        completed_at
    )
    .execute(&state.db)
    .await?;

    Ok(Json(result))
}

fn grade_ctf(lab: &Lab, answer: &Value) -> crate::error::Result<SubmissionResult> {
    // Multi-flag mode: content has a "flags" array with per-flag metadata
    if let Some(flags_meta) = lab.content.get("flags").and_then(|f| f.as_array()) {
        if !flags_meta.is_empty() {
            return grade_ctf_multi(lab, flags_meta, answer);
        }
    }

    // Single-flag mode (legacy)
    let submitted_flag = answer["flag"]
        .as_str()
        .ok_or_else(|| AppError::BadRequest("Missing 'flag' field in answer".to_string()))?
        .trim();

    let expected_flag = lab
        .flag
        .as_deref()
        .ok_or_else(|| AppError::Internal(anyhow::anyhow!("Lab has no flag configured")))?;

    let is_correct = submitted_flag == expected_flag;

    Ok(SubmissionResult {
        is_correct,
        score: if is_correct { lab.points } else { 0 },
        max_score: lab.points,
        feedback: if is_correct {
            Some("Correct flag! Well done!".to_string())
        } else {
            Some("Incorrect flag. Keep trying!".to_string())
        },
        question_results: None,
        flag_results: None,
    })
}

fn grade_ctf_multi(
    lab: &Lab,
    flags_meta: &[Value],
    answer: &Value,
) -> crate::error::Result<SubmissionResult> {
    // flag column stores a JSON object: {"flag_id": "FLAG{value}", ...}
    let flag_map: serde_json::Map<String, Value> = lab
        .flag
        .as_deref()
        .and_then(|s| serde_json::from_str(s).ok())
        .unwrap_or_default();

    let submitted_flags = answer
        .get("flags")
        .and_then(|f| f.as_object())
        .ok_or_else(|| AppError::BadRequest("Expected {\"flags\": {\"id\": \"FLAG{...}\"}}".to_string()))?;

    let mut flag_results: Vec<FlagResult> = Vec::new();
    let mut total_flag_points = 0i32;
    let mut earned_points = 0i32;

    for flag_meta in flags_meta {
        let flag_id = flag_meta["id"].as_str().unwrap_or("unknown").to_string();
        let flag_name = flag_meta["name"].as_str().unwrap_or(&flag_id).to_string();
        let flag_points = flag_meta["points"].as_i64().unwrap_or(0) as i32;

        let expected = flag_map
            .get(&flag_id)
            .and_then(|v| v.as_str())
            .unwrap_or("");

        let submitted = submitted_flags
            .get(&flag_id)
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim();

        let is_correct = !submitted.is_empty() && submitted == expected;
        let pts = if is_correct { flag_points } else { 0 };

        total_flag_points += flag_points;
        earned_points += pts;

        flag_results.push(FlagResult {
            flag_id,
            name: flag_name,
            is_correct,
            points_earned: pts,
        });
    }

    // Normalize to lab.points scale
    let score = if total_flag_points > 0 {
        (earned_points as f64 / total_flag_points as f64 * lab.points as f64).round() as i32
    } else {
        0
    };

    let found = flag_results.iter().filter(|r| r.is_correct).count();
    let total = flag_results.len();
    let is_correct = found == total && total > 0;

    Ok(SubmissionResult {
        is_correct,
        score,
        max_score: lab.points,
        feedback: Some(format!("{}/{} flags captured", found, total)),
        question_results: None,
        flag_results: Some(flag_results),
    })
}

fn grade_form(lab: &Lab, answer: &Value) -> crate::error::Result<SubmissionResult> {
    let questions = lab.content["questions"]
        .as_array()
        .ok_or_else(|| AppError::Internal(anyhow::anyhow!("Invalid form lab content")))?;

    let submitted_answers = answer["answers"]
        .as_object()
        .ok_or_else(|| AppError::BadRequest("Missing 'answers' object in answer".to_string()))?;

    let mut total_points = 0i32;
    let mut earned_points = 0i32;
    let mut question_results = Vec::new();

    for question in questions {
        let q_id = question["id"].as_str().unwrap_or("unknown");
        let q_points = question["points"].as_i64().unwrap_or(0) as i32;
        let correct_answer = question["correct_answer"].as_str().unwrap_or("");
        let explanation = question["explanation"].as_str().map(|s| s.to_string());

        total_points += q_points;

        let submitted = submitted_answers.get(q_id).and_then(|v| v.as_str()).unwrap_or("");
        let is_correct = submitted.trim().eq_ignore_ascii_case(correct_answer.trim());
        let points_earned = if is_correct { q_points } else { 0 };
        earned_points += points_earned;

        question_results.push(QuestionResult {
            question_id: q_id.to_string(),
            is_correct,
            points_earned,
            correct_answer: Some(correct_answer.to_string()),
            explanation,
        });
    }

    // Normalize to lab.points scale
    let score = if total_points > 0 {
        (earned_points as f64 / total_points as f64 * lab.points as f64).round() as i32
    } else {
        0
    };

    let is_correct = earned_points == total_points && total_points > 0;

    Ok(SubmissionResult {
        is_correct,
        score,
        max_score: lab.points,
        feedback: Some(format!(
            "You got {}/{} points ({:.0}%)",
            earned_points,
            total_points,
            if total_points > 0 {
                earned_points as f64 / total_points as f64 * 100.0
            } else {
                0.0
            }
        )),
        question_results: Some(question_results),
        flag_results: None,
    })
}

pub async fn my_progress(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<CourseProgress>> {
    let rows = sqlx::query_as::<_, LabProgressSummary>(
        r#"SELECT
            l.id as lab_id, l.title as lab_title, l.lab_type, l.points,
            COALESCE(lp.completed, FALSE) as completed,
            COALESCE(lp.best_score, 0) as best_score,
            COALESCE(lp.total_attempts, 0) as total_attempts,
            lp.completed_at
           FROM labs l
           LEFT JOIN lab_progress lp ON lp.lab_id = l.id AND lp.user_id = $1
           WHERE l.course_id = $2 AND l.is_published = TRUE
           ORDER BY l.order_index ASC"#,
    )
    .bind(claims.sub)
    .bind(course_id)
    .fetch_all(&state.db)
    .await?;

    let total_labs = rows.len() as i64;
    let completed_labs = rows.iter().filter(|r| r.completed).count() as i64;
    let total_points_possible: i64 = rows.iter().map(|r| r.points as i64).sum();
    let total_points_earned: i64 = rows.iter().map(|r| r.best_score as i64).sum();

    let completion_percentage = if total_labs > 0 {
        completed_labs as f64 / total_labs as f64 * 100.0
    } else {
        0.0
    };

    Ok(Json(CourseProgress {
        course_id,
        user_id: claims.sub,
        total_labs,
        completed_labs,
        total_points_possible,
        total_points_earned,
        completion_percentage,
        lab_progress: rows,
    }))
}

pub async fn my_submissions(
    State(state): State<AppState>,
    claims: axum::extract::Extension<Claims>,
    Path((_course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let submissions = sqlx::query(
        r#"SELECT id, answer, is_correct, score, attempts, submitted_at
           FROM lab_submissions
           WHERE user_id = $1 AND lab_id = $2
           ORDER BY submitted_at DESC
           LIMIT 20"#,
    )
    .bind(claims.sub)
    .bind(lab_id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(json!({
        "submissions": submissions.iter().map(|s| {
            let id: Uuid = s.get("id");
            let answer: Value = s.get("answer");
            let is_correct: bool = s.get("is_correct");
            let score: i32 = s.get("score");
            let attempts: i32 = s.get("attempts");
            let submitted_at: chrono::DateTime<Utc> = s.get("submitted_at");
            json!({
                "id": id,
                "answer": answer,
                "is_correct": is_correct,
                "score": score,
                "attempts": attempts,
                "submitted_at": submitted_at,
            })
        }).collect::<Vec<_>>()
    })))
}

/// Admin: monitoring all students for a course
pub async fn admin_course_monitoring(
    State(state): State<AppState>,
    Path(course_id): Path<Uuid>,
) -> Result<Json<Value>> {
    let course = sqlx::query!("SELECT title FROM courses WHERE id = $1", course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Course not found".to_string()))?;

    let total_enrolled: i64 = sqlx::query_scalar!(
        "SELECT COUNT(*) FROM enrollments WHERE course_id = $1",
        course_id
    )
    .fetch_one(&state.db)
    .await?
    .unwrap_or(0);

    let students = sqlx::query_as::<_, StudentProgressEntry>(
        r#"SELECT
            u.id as user_id, u.username, u.email,
            COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE) as completed_labs,
            COALESCE(SUM(lp.best_score), 0) as total_points,
            MAX(lp.last_attempt_at) as last_activity
           FROM enrollments e
           JOIN users u ON u.id = e.user_id
           LEFT JOIN lab_progress lp ON lp.user_id = u.id AND lp.course_id = $1
           WHERE e.course_id = $1
           GROUP BY u.id, u.username, u.email
           ORDER BY total_points DESC"#,
    )
    .bind(course_id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(json!({
        "course_id": course_id,
        "course_title": course.title,
        "total_enrolled": total_enrolled,
        "student_progress": students,
    })))
}

/// Admin: all submissions for a specific lab
pub async fn admin_lab_submissions(
    State(state): State<AppState>,
    Path((_course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let rows = sqlx::query!(
        r#"SELECT
            ls.id, ls.user_id, u.username, ls.is_correct, ls.score, ls.attempts, ls.submitted_at
           FROM lab_submissions ls
           JOIN users u ON u.id = ls.user_id
           WHERE ls.lab_id = $1
           ORDER BY ls.submitted_at DESC"#,
        lab_id
    )
    .fetch_all(&state.db)
    .await?;

    Ok(Json(json!({
        "submissions": rows.iter().map(|r| json!({
            "id": r.id,
            "user_id": r.user_id,
            "username": r.username,
            "is_correct": r.is_correct,
            "score": r.score,
            "attempts": r.attempts,
            "submitted_at": r.submitted_at,
        })).collect::<Vec<_>>()
    })))
}

/// Global stats for admin dashboard
pub async fn admin_stats(State(state): State<AppState>) -> Result<Json<Value>> {
    let total_users: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM users")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let total_courses: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM courses")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let total_labs: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM labs")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let total_submissions: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM lab_submissions")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let total_enrollments: i64 = sqlx::query_scalar!("SELECT COUNT(*) FROM enrollments")
        .fetch_one(&state.db)
        .await?
        .unwrap_or(0);

    let success_rate: f64 = sqlx::query_scalar!(
        r#"SELECT COALESCE(AVG(CASE WHEN is_correct THEN 1.0 ELSE 0.0 END) * 100, 0)::FLOAT8 FROM lab_submissions"#
    )
    .fetch_one(&state.db)
    .await?
    .unwrap_or(0.0);

    // Update prometheus gauges
    crate::metrics::ACTIVE_USERS.set(total_users as f64);
    crate::metrics::ACTIVE_COURSES.set(total_courses as f64);
    crate::metrics::ENROLLMENTS_TOTAL.set(total_enrollments as f64);

    Ok(Json(json!({
        "total_users": total_users,
        "total_courses": total_courses,
        "total_labs": total_labs,
        "total_submissions": total_submissions,
        "total_enrollments": total_enrollments,
        "success_rate": format!("{:.1}", success_rate),
    })))
}
