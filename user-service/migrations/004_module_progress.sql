-- ── Module progress ───────────────────────────────────────────────────────────
-- Tracks quiz submission results per user per module.
-- bestScore is kept across attempts; passed flips to true and stays true.

CREATE TABLE IF NOT EXISTS module_progress (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    userId      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    courseSlug  TEXT        NOT NULL,
    moduleIndex INTEGER     NOT NULL,
    bestScore   INTEGER     NOT NULL DEFAULT 0,
    maxScore    INTEGER     NOT NULL DEFAULT 0,
    passed       BOOLEAN     NOT NULL DEFAULT FALSE,
    attempts     INTEGER     NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    updatedAt   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(userId, courseSlug, moduleIndex)
);

CREATE INDEX IF NOT EXISTS idx_module_progress_user   ON module_progress(userId);
CREATE INDEX IF NOT EXISTS idx_module_progress_course ON module_progress(userId, courseSlug);
