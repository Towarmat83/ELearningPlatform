export interface CheckResult {
  allow: boolean;
  violations: string[];
}

export interface LocalCheckMeta {
  check_provider?: "local" | "gitlab";
  check_type?: string;
  check_params?: Record<string, unknown>;
}

function isTauri(): boolean {
  return (
    typeof window !== "undefined" &&
    ("__TAURI__" in window || "__TAURI_INTERNALS__" in window)
  );
}

async function invokeLocalCheck(
  checkType: string,
  params: Record<string, unknown>
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
  if (meta.check_provider === "local" && isTauri()) {
    if (!meta.check_type) {
      return { allow: false, violations: ["check_type manquant dans le module"] };
    }
    const result = await invokeLocalCheck(meta.check_type, meta.check_params ?? {});
    if (recordUrl) {
      fetch(recordUrl, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: JSON.stringify(result),
      }).catch(() => {});
    }
    return result;
  }
  return remoteCheck();
}
