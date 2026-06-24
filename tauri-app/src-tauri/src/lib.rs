use serde::{Deserialize, Serialize};
use std::process::Command;

#[derive(Serialize)]
pub struct LocalCheckResult {
    pub allow: bool,
    pub violations: Vec<String>,
}

#[derive(Deserialize)]
pub struct PodmanCheckParams {
    pub image: String,
}

#[derive(Deserialize)]
pub struct PodmanLab2Params {
    pub image: String,
    pub container_name: String,
}

fn run_podman(args: &[&str]) -> Result<String, String> {
    let out = Command::new("podman")
        .args(args)
        .output()
        .map_err(|e| format!("Impossible de lancer podman : {e}"))?;
    Ok(String::from_utf8_lossy(&out.stdout).to_string())
}

/// Vérifie que l'image `params.image` est présente dans `podman images`.
#[tauri::command]
fn check_podman_images(params: PodmanCheckParams) -> Result<LocalCheckResult, String> {
    let stdout = run_podman(&["images", "--format", "{{.Repository}}:{{.Tag}}"])?;
    let found = stdout.lines().any(|line| line.contains(&params.image));

    if found {
        Ok(LocalCheckResult { allow: true, violations: vec![] })
    } else {
        Ok(LocalCheckResult {
            allow: false,
            violations: vec![format!(
                "Image '{}' introuvable. Avez-vous bien exécuté 'podman pull {}'?",
                params.image, params.image
            )],
        })
    }
}

/// Vérifie que l'image a été pullée ET qu'un conteneur a été lancé (via podman events).
#[tauri::command]
fn check_podman_lab2(params: PodmanLab2Params) -> Result<LocalCheckResult, String> {
    let mut violations: Vec<String> = vec![];

    // 1. podman images — nginx présent ?
    let images = run_podman(&["images", "--format", "{{.Repository}}:{{.Tag}}"])?;
    if !images.lines().any(|l| l.contains(&params.image)) {
        violations.push(format!(
            "Image '{}' introuvable. Avez-vous bien exécuté 'podman pull docker.io/library/{}:latest' ?",
            params.image, params.image
        ));
    }

    // 2. podman events — un conteneur nginx a-t-il démarré dans les dernières 24h ?
    // --stream=false : lit les événements existants et quitte immédiatement
    let container_filter = format!("container={}", params.container_name);
    let events = run_podman(&[
        "events",
        "--stream=false",
        "--filter", "type=container",
        "--filter", "event=start",
        "--filter", &container_filter,
        "--since", "24h",
        "--no-trunc",
    ])
    .unwrap_or_default();

    if events.trim().is_empty() {
        violations.push(format!(
            "Aucun conteneur '{}' démarré dans les dernières 24h. Avez-vous bien exécuté 'podman run -it --name {} --rm docker.io/library/{}:latest' ?",
            params.container_name, params.container_name, params.image
        ));
    }

    Ok(LocalCheckResult {
        allow: violations.is_empty(),
        violations,
    })
}

/// Point d'entrée générique pour les checks locaux.
#[tauri::command]
fn local_check(
    check_type: String,
    params: serde_json::Value,
) -> Result<LocalCheckResult, String> {
    match check_type.as_str() {
        "podman_images" => {
            let p: PodmanCheckParams = serde_json::from_value(params)
                .map_err(|e| format!("Params invalides : {e}"))?;
            check_podman_images(p)
        }
        "podman_lab2" => {
            let p: PodmanLab2Params = serde_json::from_value(params)
                .map_err(|e| format!("Params invalides : {e}"))?;
            check_podman_lab2(p)
        }
        _ => Err(format!("check_type inconnu : {check_type}")),
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            local_check,
            check_podman_images,
            check_podman_lab2
        ])
        .run(tauri::generate_context!())
        .expect("Erreur au démarrage de Tauri");
}
