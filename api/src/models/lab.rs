use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::FromRow;
use uuid::Uuid;

/// Lab type: form (quiz) or ctf (capture the flag)
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum LabType {
    Form,
    Ctf,
}

impl std::fmt::Display for LabType {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            LabType::Form => write!(f, "form"),
            LabType::Ctf => write!(f, "ctf"),
        }
    }
}

/// Form lab content structure:
/// {
///   "questions": [
///     {
///       "id": "q1",
///       "text": "What is ...",
///       "type": "multiple_choice" | "text" | "code",
///       "options": ["A", "B", "C", "D"],   // for multiple_choice
///       "correct_answer": "A",              // hidden from students
///       "points": 25,
///       "explanation": "Because..."
///     }
///   ]
/// }
///
/// CTF lab content structure:
/// {
///   "challenge": "Description of the challenge",
///   "category": "web" | "crypto" | "forensics" | "pwn" | "misc",
///   "hints": ["Hint 1", "Hint 2"],
///   "resources": [{"name": "File", "url": "..."}],
///   "docker_image": "optional docker image for the challenge"
/// }

#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct Lab {
    pub id: Uuid,
    pub course_id: Uuid,
    pub title: String,
    pub description: String,
    pub lab_type: String,
    pub content: Value,
    #[serde(skip_serializing)] // Never expose flag to client
    pub flag: Option<String>,
    pub points: i32,
    pub order_index: i32,
    pub is_published: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Lab as seen by student (flag removed, correct_answers removed from form questions)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LabStudent {
    pub id: Uuid,
    pub course_id: Uuid,
    pub title: String,
    pub description: String,
    pub lab_type: String,
    pub content: Value,
    pub points: i32,
    pub order_index: i32,
    pub is_published: bool,
    pub created_at: DateTime<Utc>,
}

impl From<Lab> for LabStudent {
    fn from(lab: Lab) -> Self {
        let content = strip_answers(lab.lab_type.as_str(), lab.content);
        Self {
            id: lab.id,
            course_id: lab.course_id,
            title: lab.title,
            description: lab.description,
            lab_type: lab.lab_type,
            content,
            points: lab.points,
            order_index: lab.order_index,
            is_published: lab.is_published,
            created_at: lab.created_at,
        }
    }
}

fn strip_answers(lab_type: &str, mut content: Value) -> Value {
    if lab_type == "form" {
        if let Some(questions) = content.get_mut("questions").and_then(|q| q.as_array_mut()) {
            for question in questions.iter_mut() {
                if let Some(obj) = question.as_object_mut() {
                    obj.remove("correct_answer");
                }
            }
        }
    }
    content
}

#[derive(Debug, Deserialize)]
pub struct CreateLabRequest {
    pub title: String,
    pub description: String,
    pub lab_type: String,
    pub content: Value,
    pub flag: Option<String>,
    pub points: Option<i32>,
    pub order_index: Option<i32>,
    pub is_published: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct UpdateLabRequest {
    pub title: Option<String>,
    pub description: Option<String>,
    pub content: Option<Value>,
    pub flag: Option<String>,
    pub points: Option<i32>,
    pub order_index: Option<i32>,
    pub is_published: Option<bool>,
}
