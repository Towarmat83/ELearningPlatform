-- Migration 005 — Platform settings: runtime-configurable key/value store
CREATE TABLE platform_settings (
    key         VARCHAR(64) PRIMARY KEY,
    value       TEXT        NOT NULL,
    description TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default values (overridden at runtime by admin UI)
INSERT INTO platform_settings (key, value, description) VALUES
    ('gitlab_url', 'https://gitlab.com', 'Base URL of the GitLab instance used for SSO (e.g. https://gitlab.mycompany.com)')
ON CONFLICT (key) DO NOTHING;
