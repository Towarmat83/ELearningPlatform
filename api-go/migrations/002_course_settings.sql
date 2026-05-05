-- Course publishing settings stored in DB (overlays YAML defaults).
-- is_published: admins control visibility; git-imported courses start unpublished.
-- auto_enroll: if true, any authenticated user can self-enroll.

CREATE TABLE IF NOT EXISTS course_settings (
    course_slug  TEXT        PRIMARY KEY,
    is_published BOOLEAN     NOT NULL DEFAULT false,
    auto_enroll  BOOLEAN     NOT NULL DEFAULT false,
    source       TEXT        NOT NULL DEFAULT 'local',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_course_settings_source ON course_settings(source);
