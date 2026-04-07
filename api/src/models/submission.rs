use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::FromRow;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct LabSubmission {
    pub id: Uuid,
    pub lab_id: Uuid,
    pub user_id: Uuid,
    pub answer: Value,
    pub is_correct: bool,
    pub score: i32,
    pub attempts: i32,
    pub submitted_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct LabProgress {
    pub id: Uuid,
    pub user_id: Uuid,
    pub lab_id: Uuid,
    pub course_id: Uuid,
    pub completed: bool,
    pub best_score: i32,
    pub total_attempts: i32,
    pub completed_at: Option<DateTime<Utc>>,
    pub last_attempt_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize)]
pub struct SubmitLabRequest {
    /// For CTF: {"flag": "FLAG{...}"}
    /// For Form: {"answers": {"q1": "A", "q2": "Paris"}}
    pub answer: Value,
}

#[derive(Debug, Serialize)]
pub struct SubmissionResult {
    pub is_correct: bool,
    pub score: i32,
    pub max_score: i32,
    pub feedback: Option<String>,
    /// For form labs: per-question results
    pub question_results: Option<Vec<QuestionResult>>,
    /// For multi-flag CTF labs: per-flag results
    pub flag_results: Option<Vec<FlagResult>>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct FlagResult {
    pub flag_id: String,
    pub name: String,
    pub is_correct: bool,
    pub points_earned: i32,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct QuestionResult {
    pub question_id: String,
    pub is_correct: bool,
    pub points_earned: i32,
    pub correct_answer: Option<String>,
    pub explanation: Option<String>,
}

/// Student progress summary for a course
#[derive(Debug, Serialize, Deserialize)]
pub struct CourseProgress {
    pub course_id: Uuid,
    pub user_id: Uuid,
    pub total_labs: i64,
    pub completed_labs: i64,
    pub total_points_possible: i64,
    pub total_points_earned: i64,
    pub completion_percentage: f64,
    pub lab_progress: Vec<LabProgressSummary>,
}

#[derive(Debug, Serialize, Deserialize, FromRow)]
pub struct LabProgressSummary {
    pub lab_id: Uuid,
    pub lab_title: String,
    pub lab_type: String,
    pub points: i32,
    pub completed: bool,
    pub best_score: i32,
    pub total_attempts: i32,
    pub completed_at: Option<DateTime<Utc>>,
}

/// Admin monitoring: all users' progress for a course
#[derive(Debug, Serialize, Deserialize)]
pub struct AdminCourseMonitoring {
    pub course_id: Uuid,
    pub course_title: String,
    pub total_enrolled: i64,
    pub student_progress: Vec<StudentProgressEntry>,
}

#[derive(Debug, Serialize, Deserialize, FromRow)]
pub struct StudentProgressEntry {
    pub user_id: Uuid,
    pub username: String,
    pub email: String,
    pub completed_labs: i64,
    pub total_points: i64,
    pub last_activity: Option<DateTime<Utc>>,
}
