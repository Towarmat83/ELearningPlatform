export interface CheckResult {
  allow: boolean;
  violations: string[];
}

export interface LocalCheckMeta {
  checkProvider?: "local" | "gitlab";
  checkType?: string;
  checkParams?: Record<string, unknown>;
}

function isTauri(): boolean {
  return (
    typeof window !== "undefined" &&
    ("__TAURI__" in window || "__TAURI_INTERNALS__" in window)
  );
}

async function invokeLocalCheck(
  checkType: string,
  params: Record<string, unknown>,
): Promise<CheckResult> {
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<CheckResult>("local_check", { checkType, params });
}

/**
 * Résout le check selon le provider :
 * - provider=local + contexte Tauri → commande Rust locale
 * - sinon → API course-service existante
 */
export async function resolveCheck(
  meta: LocalCheckMeta,
  remoteCheck: () => Promise<CheckResult>,
  recordUrl?: string,
  token?: string,
): Promise<CheckResult> {
  if (meta.checkProvider === "local" && isTauri()) {
    if (!meta.checkType) {
      return {
        allow: false,
        violations: ["checkType manquant dans le module"],
      };
    }
    const result = await invokeLocalCheck(
      meta.checkType,
      meta.checkParams ?? {},
    );
    if (recordUrl) {
      fetch(recordUrl, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify(result),
      }).catch(() => {});
    }
    return result;
  }
  return remoteCheck();
}
