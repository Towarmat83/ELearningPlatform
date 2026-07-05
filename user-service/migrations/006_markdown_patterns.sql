CREATE TABLE IF NOT EXISTS markdown_patterns (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL,
    label       VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parameter   TEXT NOT NULL DEFAULT '',
    html        TEXT NOT NULL,
    css         TEXT NOT NULL DEFAULT '',
    js          TEXT NOT NULL DEFAULT '',
    scope       TEXT NOT NULL DEFAULT 'global',
    createdBy  UUID REFERENCES users(id) ON DELETE SET NULL,
    from_config BOOLEAN NOT NULL DEFAULT FALSE,
    createdAt  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updatedAt  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name, scope)
);

CREATE INDEX IF NOT EXISTS idx_markdown_patterns_scope ON markdown_patterns(scope);
