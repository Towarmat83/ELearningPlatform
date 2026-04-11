use axum::{
    extract::{Path, Query, State, WebSocketUpgrade},
    response::IntoResponse,
    Json,
};
use bollard::{
    container::{Config as ContainerConfig, CreateContainerOptions, LogOutput, RemoveContainerOptions},
    exec::{CreateExecOptions, ResizeExecOptions, StartExecOptions, StartExecResults},
    image::CreateImageOptions,
    models::HostConfig,
};
use futures_util::{SinkExt, StreamExt};
use serde::Deserialize;
use serde_json::{json, Value};
use sqlx::Row;
use tokio::io::AsyncWriteExt;
use uuid::Uuid;

use crate::{
    error::{AppError, Result},
    middleware::auth::{verify_token, Claims},
    AppState,
};

#[derive(Deserialize)]
pub struct TerminalQuery {
    token: Option<String>,
}

#[derive(Deserialize)]
struct ResizeMessage {
    #[serde(rename = "type")]
    msg_type: String,
    rows: u16,
    cols: u16,
}

/// POST /api/courses/:course_id/labs/:lab_id/instance
/// Start (or return existing running) interactive lab container for the authenticated user.
pub async fn start_instance(
    State(state): State<AppState>,
    axum::extract::Extension(claims): axum::extract::Extension<Claims>,
    Path((course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let docker = state
        .docker
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("Interactive labs are not available (Docker not connected)".to_string()))?;

    // Enrollment check (admins bypass)
    if claims.role != "admin" {
        let enrolled: i64 = sqlx::query_scalar(
            "SELECT COUNT(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
        )
        .bind(claims.sub)
        .bind(course_id)
        .fetch_one(&state.db)
        .await?;

        if enrolled == 0 {
            return Err(AppError::Forbidden("You must enroll in this course first".to_string()));
        }
    }

    // Fetch lab + docker_image from content JSONB
    let lab_row = sqlx::query("SELECT id, content FROM labs WHERE id = $1 AND course_id = $2")
        .bind(lab_id)
        .bind(course_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("Lab not found".to_string()))?;

    let content: Option<serde_json::Value> = lab_row.get("content");
    let docker_image = content
        .as_ref()
        .and_then(|c| c.get("docker_image"))
        .and_then(|v| v.as_str())
        .ok_or_else(|| {
            AppError::BadRequest("This lab has no interactive environment configured".to_string())
        })?
        .to_string();

    // Check for an existing instance record
    let existing = sqlx::query(
        "SELECT id, container_id, status, expires_at FROM lab_instances \
         WHERE user_id = $1 AND lab_id = $2",
    )
    .bind(claims.sub)
    .bind(lab_id)
    .fetch_optional(&state.db)
    .await?;

    if let Some(ref inst) = existing {
        let status: String = inst.get("status");
        let container_id: String = inst.get("container_id");

        if status == "running" {
            // Verify the container is actually alive
            if docker.inspect_container(&container_id, None).await.is_ok() {
                let id: Uuid = inst.get("id");
                let expires_at: chrono::DateTime<chrono::Utc> = inst.get("expires_at");
                return Ok(Json(json!({
                    "instance_id": id,
                    "status": "running",
                    "expires_at": expires_at,
                })));
            }
        }

        // Stale record — force-remove the container (best-effort)
        let _ = docker
            .remove_container(
                &container_id,
                Some(RemoveContainerOptions { force: true, v: false, link: false }),
            )
            .await;
    }

    // ── Pull image if not present locally ──────────────────────────────────
    let image_present = docker.inspect_image(&docker_image).await.is_ok();
    if !image_present {
        tracing::info!("Pulling image {} for lab {}", docker_image, lab_id);
        let mut stream = docker.create_image(
            Some(CreateImageOptions {
                from_image: docker_image.as_str(),
                ..Default::default()
            }),
            None,
            None,
        );
        // Drain the stream to completion (pull progress events)
        while let Some(event) = stream.next().await {
            if let Err(e) = event {
                return Err(AppError::Internal(anyhow::anyhow!("Failed to pull image {}: {}", docker_image, e)));
            }
        }
        tracing::info!("Image {} pulled successfully", docker_image);
    }

    // ── Create & start a new container ─────────────────────────────────────
    let container_name = format!("lab-{}-{}", claims.sub.as_simple(), lab_id.as_simple());

    let host_config = HostConfig {
        memory: Some(512 * 1024 * 1024), // 512 MB RAM
        nano_cpus: Some(500_000_000),     // 0.5 vCPU
        pids_limit: Some(50),
        network_mode: Some("none".to_string()), // isolated network
        ..Default::default()
    };

    let container = docker
        .create_container(
            Some(CreateContainerOptions { name: container_name, platform: None }),
            ContainerConfig::<String> {
                image: Some(docker_image),
                tty: Some(true),
                open_stdin: Some(true),
                stdin_once: Some(false),
                host_config: Some(host_config),
                ..Default::default()
            },
        )
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("Failed to create container: {}", e)))?;

    docker
        .start_container(&container.id, None::<bollard::container::StartContainerOptions<String>>)
        .await
        .map_err(|e| AppError::Internal(anyhow::anyhow!("Failed to start container: {}", e)))?;

    // Upsert instance row
    let row = sqlx::query(
        r#"INSERT INTO lab_instances (user_id, lab_id, container_id, status, started_at, expires_at)
           VALUES ($1, $2, $3, 'running', NOW(), NOW() + INTERVAL '30 minutes')
           ON CONFLICT (user_id, lab_id) DO UPDATE
               SET container_id = $3,
                   status       = 'running',
                   started_at   = NOW(),
                   expires_at   = NOW() + INTERVAL '30 minutes'
           RETURNING id, expires_at"#,
    )
    .bind(claims.sub)
    .bind(lab_id)
    .bind(&container.id)
    .fetch_one(&state.db)
    .await?;

    let instance_id: Uuid = row.get("id");
    let expires_at: chrono::DateTime<chrono::Utc> = row.get("expires_at");

    Ok(Json(json!({
        "instance_id": instance_id,
        "status": "running",
        "expires_at": expires_at,
    })))
}

/// GET /api/courses/:course_id/labs/:lab_id/instance
pub async fn get_instance(
    State(state): State<AppState>,
    axum::extract::Extension(claims): axum::extract::Extension<Claims>,
    Path((_course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let row = sqlx::query(
        "SELECT id, status, started_at, expires_at \
         FROM lab_instances WHERE user_id = $1 AND lab_id = $2",
    )
    .bind(claims.sub)
    .bind(lab_id)
    .fetch_optional(&state.db)
    .await?;

    match row {
        None => Ok(Json(json!({ "status": "none" }))),
        Some(r) => {
            let id: Uuid = r.get("id");
            let status: String = r.get("status");
            let started_at: chrono::DateTime<chrono::Utc> = r.get("started_at");
            let expires_at: chrono::DateTime<chrono::Utc> = r.get("expires_at");
            Ok(Json(json!({
                "instance_id": id,
                "status": status,
                "started_at": started_at,
                "expires_at": expires_at,
            })))
        }
    }
}

/// DELETE /api/courses/:course_id/labs/:lab_id/instance
pub async fn stop_instance(
    State(state): State<AppState>,
    axum::extract::Extension(claims): axum::extract::Extension<Claims>,
    Path((_course_id, lab_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>> {
    let docker = state
        .docker
        .as_ref()
        .ok_or_else(|| AppError::BadRequest("Docker not available".to_string()))?;

    let row = sqlx::query(
        "SELECT id, container_id FROM lab_instances \
         WHERE user_id = $1 AND lab_id = $2 AND status = 'running'",
    )
    .bind(claims.sub)
    .bind(lab_id)
    .fetch_optional(&state.db)
    .await?
    .ok_or_else(|| AppError::NotFound("No running instance found".to_string()))?;

    let instance_id: Uuid = row.get("id");
    let container_id: String = row.get("container_id");

    // Stop then remove (best-effort — container may have died already)
    let _ = docker.stop_container(&container_id, None).await;
    let _ = docker
        .remove_container(
            &container_id,
            Some(RemoveContainerOptions { force: true, v: false, link: false }),
        )
        .await;

    sqlx::query("UPDATE lab_instances SET status = 'stopped' WHERE id = $1")
        .bind(instance_id)
        .execute(&state.db)
        .await?;

    Ok(Json(json!({ "message": "Instance stopped" })))
}

/// GET /ws/courses/:course_id/labs/:lab_id/terminal?token=<JWT>
///
/// Upgrades to a WebSocket that proxies stdin/stdout of the running container's
/// shell.  Authentication is done via `?token=` because the browser WebSocket API
/// cannot send custom headers.
pub async fn terminal_ws(
    ws: WebSocketUpgrade,
    State(state): State<AppState>,
    Path((_course_id, lab_id)): Path<(Uuid, Uuid)>,
    Query(params): Query<TerminalQuery>,
) -> impl IntoResponse {
    // Perform auth + DB lookup before upgrading so we can return HTTP errors.
    let setup = async {
        let token = params
            .token
            .ok_or_else(|| AppError::Unauthorized("Missing token query parameter".to_string()))?;
        let claims = verify_token(&token, &state.config.jwt_secret)?;

        let docker = state
            .docker
            .clone()
            .ok_or_else(|| AppError::BadRequest("Docker not available".to_string()))?;

        let row = sqlx::query(
            "SELECT container_id FROM lab_instances \
             WHERE user_id = $1 AND lab_id = $2 AND status = 'running'",
        )
        .bind(claims.sub)
        .bind(lab_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or_else(|| AppError::NotFound("No running instance — start one first".to_string()))?;

        let container_id: String = row.get("container_id");
        Ok::<_, AppError>((docker, container_id))
    };

    match setup.await {
        Err(e) => e.into_response(),
        Ok((docker, container_id)) => ws
            .on_upgrade(move |socket| async move {
                if let Err(e) = run_terminal(socket, docker, container_id).await {
                    tracing::error!("Terminal WS error: {}", e);
                }
            })
            .into_response(),
    }
}

/// Bidirectional bridge: WebSocket ↔ Docker exec PTY
async fn run_terminal(
    socket: axum::extract::ws::WebSocket,
    docker: bollard::Docker,
    container_id: String,
) -> anyhow::Result<()> {
    use axum::extract::ws::Message;

    // Create a TTY exec inside the container
    let exec = docker
        .create_exec(
            &container_id,
            CreateExecOptions::<String> {
                attach_stdin: Some(true),
                attach_stdout: Some(true),
                attach_stderr: Some(true),
                tty: Some(true),
                cmd: Some(vec!["/bin/sh".to_string()]),
                ..Default::default()
            },
        )
        .await?;

    let StartExecResults::Attached {
        output: mut docker_out,
        input: mut docker_in,
    } = docker
        .start_exec(&exec.id, Some(StartExecOptions { detach: false, tty: true, ..Default::default() }))
        .await?
    else {
        anyhow::bail!("Expected attached exec result");
    };

    let (mut ws_tx, mut ws_rx) = socket.split();

    // Task: Docker stdout/stderr → WebSocket client
    let forward_out = tokio::spawn(async move {
        while let Some(Ok(log)) = docker_out.next().await {
            let data: bytes::Bytes = match log {
                LogOutput::StdOut { message } => message,
                LogOutput::StdErr { message } => message,
                LogOutput::Console { message } => message,
                _ => continue,
            };
            if ws_tx.send(Message::Binary(data.to_vec())).await.is_err() {
                break;
            }
        }
    });

    // Main loop: WebSocket → Docker stdin  (resize messages handled inline)
    while let Some(Ok(msg)) = ws_rx.next().await {
        match msg {
            Message::Binary(data) => {
                if docker_in.write_all(&data).await.is_err() {
                    break;
                }
            }
            Message::Text(text) => {
                // A JSON resize event sent by xterm.js FitAddon
                if let Ok(r) = serde_json::from_str::<ResizeMessage>(&text) {
                    if r.msg_type == "resize" {
                        let _ = docker
                            .resize_exec(&exec.id, ResizeExecOptions { height: r.rows, width: r.cols })
                            .await;
                        continue;
                    }
                }
                // Otherwise treat as raw input
                if docker_in.write_all(text.as_bytes()).await.is_err() {
                    break;
                }
            }
            Message::Close(_) => break,
            _ => {}
        }
    }

    forward_out.abort();
    Ok(())
}
