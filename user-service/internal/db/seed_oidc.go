package db

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elearning/user-service/internal/config"
)

// SeedOIDC bootstraps the OIDC platform settings from deploy-time configuration
// (Helm). It is a no-op unless OIDC_ENABLED=true, which keeps the admin-UI-only
// workflow fully backward compatible: deployments that don't provision OIDC via
// Helm are never touched.
//
// When enabled, the Helm configuration is authoritative — the relevant
// platform_settings rows are upserted on every startup so a `helm upgrade`
// reliably reflects the chart (GitOps). Only non-empty bootstrap values are
// written, so individual fields left blank in the chart fall back to whatever
// is already stored (e.g. the migration defaults or a prior admin edit).
//
// The client secret originates from a mounted Kubernetes Secret (file) or env
// var; it is never hardcoded in the chart values. Rotating it requires a pod
// restart so the new value is re-seeded.
func SeedOIDC(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
	b := cfg.OIDC
	if !b.Enabled {
		return nil
	}

	// key/value pairs to seed; empty values are skipped below.
	settings := map[string]string{
		"oidc_enabled":          "true",
		"oidc_provider_url":     b.ProviderURL,
		"oidc_issuer_url":       b.IssuerURL,
		"oidc_client_id":        b.ClientID,
		"oidc_client_secret":    b.ResolveClientSecret(),
		"oidc_scopes":           b.Scopes,
		"oidc_group_claim":      b.GroupClaim,
		"oidc_redirect_base":    b.RedirectBase,
		"oidc_browser_base_url": b.BrowserBaseURL,
	}
	if b.InsecureSkipVerify {
		settings["oidc_insecure_skip_verify"] = "true"
	}

	seeded := make([]string, 0, len(settings))
	for key, val := range settings {
		if val == "" {
			continue
		}

		_, err := pool.Exec(ctx,
			`INSERT INTO platform_settings (key, value, updated_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
			key, val)
		if err != nil {
			return err
		}

		if key != "oidc_client_secret" {
			seeded = append(seeded, key)
		} else {
			seeded = append(seeded, "oidc_client_secret(redacted)")
		}
	}

	slog.Info("OIDC settings bootstrapped from deploy-time config", "keys", seeded)

	return nil
}
