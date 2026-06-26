use serde::{Deserialize, Serialize};
use std::process::Command;
use tauri::{Listener, Manager};
use tauri_plugin_deep_link::DeepLinkExt;

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

#[derive(Deserialize)]
pub struct DistroboxLab3Params {
    pub container_name: String,
    pub app: String,
}

fn deep_link_to_nav_url(url: &str) -> String {
    let path = url
        .trim_start_matches("pupitre://")
        .replace("modules/", "lessons/");
    format!("http://localhost:3000/{}", path)
}

fn podman_bin() -> &'static str {
    const CANDIDATES: &[&str] = &[
        "/usr/bin/podman",          // Linux
        "/opt/podman/bin/podman",   // Podman Desktop macOS
        "/usr/local/bin/podman",    // Homebrew Intel
        "/opt/homebrew/bin/podman", // Homebrew Apple Silicon
    ];
    for path in CANDIDATES {
        if std::path::Path::new(path).exists() {
            return path;
        }
    }
    "podman"
}

fn run_podman(args: &[&str]) -> Result<String, String> {
    let out = Command::new(podman_bin())
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

#[derive(Deserialize)]
pub struct DistroboxContainerParams {
    pub container_name: String,
}

fn distrobox_container_exists(container_name: &str) -> bool {
    Command::new(podman_bin())
        .args(["container", "exists", container_name])
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

fn distrobox_app_exported(app: &str) -> bool {
    let home = std::env::var("HOME").unwrap_or_default();
    let apps_dir = std::path::Path::new(&home).join(".local/share/applications");
    apps_dir.read_dir().ok().map_or(false, |mut entries| {
        entries.any(|e| {
            e.ok()
                .and_then(|e| e.file_name().into_string().ok())
                .map_or(false, |name| name.contains(app))
        })
    })
}

#[tauri::command]
fn check_distrobox_lab3_container(params: DistroboxContainerParams) -> Result<LocalCheckResult, String> {
    if distrobox_container_exists(&params.container_name) {
        Ok(LocalCheckResult { allow: true, violations: vec![] })
    } else {
        Ok(LocalCheckResult {
            allow: false,
            violations: vec![format!(
                "Distrobox '{}' introuvable. Avez-vous bien exécuté 'distrobox create --name {}' ?",
                params.container_name, params.container_name
            )],
        })
    }
}

#[tauri::command]
fn check_distrobox_lab3_export(params: DistroboxLab3Params) -> Result<LocalCheckResult, String> {
    let mut violations = vec![];
    if !distrobox_container_exists(&params.container_name) {
        violations.push(format!(
            "Distrobox '{}' introuvable. Créez-le d'abord avec 'distrobox create --name {}'.",
            params.container_name, params.container_name
        ));
    }
    if !distrobox_app_exported(&params.app) {
        violations.push(format!(
            "Application '{}' non exportée. Avez-vous bien exécuté 'distrobox-export -a {}' depuis la Distrobox ?",
            params.app, params.app
        ));
    }
    Ok(LocalCheckResult { allow: violations.is_empty(), violations })
}

#[tauri::command]
fn check_distrobox_lab3(params: DistroboxLab3Params) -> Result<LocalCheckResult, String> {
    let mut violations: Vec<String> = vec![];
    if !distrobox_container_exists(&params.container_name) {
        violations.push(format!(
            "Distrobox '{}' introuvable. Avez-vous bien exécuté 'distrobox create --name {}' ?",
            params.container_name, params.container_name
        ));
    }
    if !distrobox_app_exported(&params.app) {
        violations.push(format!(
            "Application '{}' non exportée. Avez-vous bien exécuté 'distrobox-export -a {}' depuis la Distrobox ?",
            params.app, params.app
        ));
    }
    Ok(LocalCheckResult { allow: violations.is_empty(), violations })
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
        "distrobox_lab3" => {
            let p: DistroboxLab3Params = serde_json::from_value(params)
                .map_err(|e| format!("Params invalides : {e}"))?;
            check_distrobox_lab3(p)
        }
        "distrobox_lab3_container" => {
            let p: DistroboxContainerParams = serde_json::from_value(params)
                .map_err(|e| format!("Params invalides : {e}"))?;
            check_distrobox_lab3_container(p)
        }
        "distrobox_lab3_export" => {
            let p: DistroboxLab3Params = serde_json::from_value(params)
                .map_err(|e| format!("Params invalides : {e}"))?;
            check_distrobox_lab3_export(p)
        }
        _ => Err(format!("check_type inconnu : {check_type}")),
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_deep_link::init())
        .invoke_handler(tauri::generate_handler![
            local_check,
            check_podman_images,
            check_podman_lab2,
            check_distrobox_lab3,
            check_distrobox_lab3_container,
            check_distrobox_lab3_export
        ])
        .setup(|app| {
            // Lancement frais via pupitre:// : récupère l'URL qui a ouvert l'app
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                if let Some(url) = urls.first() {
                    let nav_url = deep_link_to_nav_url(&url.to_string());
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.navigate(nav_url.parse().unwrap());
                    }
                }
            }

            // App déjà ouverte : reçoit un nouveau deep link
            let handle = app.handle().clone();
            app.listen("deep-link://new-url", move |event| {
                if let Ok(urls) = serde_json::from_str::<Vec<String>>(event.payload()) {
                    if let Some(url) = urls.first() {
                        let nav_url = deep_link_to_nav_url(url);
                        if let Some(window) = handle.get_webview_window("main") {
                            let _ = window.navigate(nav_url.parse().unwrap());
                        }
                    }
                }
            });
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("Erreur au démarrage de Tauri");
}
