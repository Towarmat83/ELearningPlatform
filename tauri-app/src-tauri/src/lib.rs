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

/// Vérifie que l'image `params.image` est présente dans `podman images`.
#[tauri::command]
fn check_podman_images(params: PodmanCheckParams) -> Result<LocalCheckResult, String> {
    let output = Command::new("podman")
        .args(["images", "--format", "{{.Repository}}:{{.Tag}}"])
        .output()
        .map_err(|e| format!("Impossible de lancer podman : {e}"))?;

    let stdout = String::from_utf8_lossy(&output.stdout);
    let found = stdout.lines().any(|line| line.contains(&params.image));

    if found {
        Ok(LocalCheckResult {
            allow: true,
            violations: vec![],
        })
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

/// Point d'entrée générique pour les checks locaux.
/// `check_type` correspond au champ `check_type` du check.yaml.
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
        _ => Err(format!("check_type inconnu : {check_type}")),
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![local_check, check_podman_images])
        .run(tauri::generate_context!())
        .expect("Erreur au démarrage de Tauri");
}
