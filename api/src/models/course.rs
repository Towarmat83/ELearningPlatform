use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct Course {
    pub id: Uuid,
    pub title: String,
    pub description: String,
    pub thumbnail: Option<String>,
    pub category: Option<String>,
    pub difficulty: Option<String>,
    pub is_published: bool,
    pub created_by: Uuid,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CourseWithStats {
    #[serde(flatten)]
    pub course: Course,
    pub lab_count: i64,
    pub enrollment_count: i64,
    pub creator_username: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct CreateCourseRequest {
    pub title: String,
    pub description: String,
    pub thumbnail: Option<String>,
    pub category: Option<String>,
    pub difficulty: Option<String>,
    pub is_published: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct UpdateCourseRequest {
    pub title: Option<String>,
    pub description: Option<String>,
    pub thumbnail: Option<String>,
    pub category: Option<String>,
    pub difficulty: Option<String>,
    pub is_published: Option<bool>,
}

#[derive(Debug, Deserialize, Default)]
pub struct CourseFilter {
    pub category: Option<String>,
    pub difficulty: Option<String>,
    pub search: Option<String>,
    pub page: Option<i64>,
    pub per_page: Option<i64>,
}
