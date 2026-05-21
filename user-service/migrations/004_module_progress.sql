-- ── Module progress ───────────────────────────────────────────────────────────
-- Tracks quiz submission results per user per module.
-- best_score is kept across attempts; passed flips to true and stays true.

CREATE TABLE IF NOT EXISTS module_progress (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_slug  TEXT        NOT NULL,
    module_index INTEGER     NOT NULL,
    best_score   INTEGER     NOT NULL DEFAULT 0,
    max_score    INTEGER     NOT NULL DEFAULT 0,
    passed       BOOLEAN     NOT NULL DEFAULT FALSE,
    attempts     INTEGER     NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, course_slug, module_index)
);

CREATE INDEX IF NOT EXISTS idx_module_progress_user   ON module_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_module_progress_course ON module_progress(user_id, course_slug);
