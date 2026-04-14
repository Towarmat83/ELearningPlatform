-- Migration 002 — Interactive lab instances: tracks Docker containers spawned per user per lab
CREATE TABLE IF NOT EXISTS lab_instances (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lab_id       UUID        NOT NULL REFERENCES labs(id)  ON DELETE CASCADE,
    container_id TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'running', -- running | stopped | error
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 minutes',
    UNIQUE(user_id, lab_id)
);

CREATE INDEX IF NOT EXISTS idx_lab_instances_user_lab ON lab_instances(user_id, lab_id);
CREATE INDEX IF NOT EXISTS idx_lab_instances_expires  ON lab_instances(expires_at);
