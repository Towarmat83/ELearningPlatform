-- Course publishing settings stored in DB (overlays YAML defaults).
-- isPublished: admins control visibility; git-imported courses start unpublished.
-- auto_enroll: if true, any authenticated user can self-enroll.

CREATE TABLE IF NOT EXISTS course_settings (
    courseSlug  TEXT        PRIMARY KEY,
    isPublished BOOLEAN     NOT NULL DEFAULT false,
    auto_enroll  BOOLEAN     NOT NULL DEFAULT false,
    source       TEXT        NOT NULL DEFAULT 'local',
    updatedAt   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_course_settings_source ON course_settings(source);
