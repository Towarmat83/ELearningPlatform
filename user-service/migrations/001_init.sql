-- Schema initial — content-as-code / GitOps
-- Single source of truth: courses live in files, only user data in DB.

-- ── Users ────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username         VARCHAR(64) UNIQUE NOT NULL,
    email            VARCHAR(255) UNIQUE NOT NULL,
    password_hash    TEXT,                          -- nullable: SSO users have no password
    role             VARCHAR(16) NOT NULL DEFAULT 'student' CHECK (role IN ('admin', 'student')),
    avatar_url       TEXT,
    bio              TEXT,
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    auth_provider    VARCHAR(32) NOT NULL DEFAULT 'local',
    provider_user_id VARCHAR(255),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_provider_uid_key UNIQUE (auth_provider, provider_user_id)
);

-- ── Platform settings ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS platform_settings (
    key        VARCHAR(64) PRIMARY KEY,
    value      TEXT        NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO platform_settings (key, value, description) VALUES
    ('gitlab_url',                   'https://gitlab.com', 'Base URL of the GitLab instance for SSO'),
    ('registration_enabled',         'true',  'Allow new users to self-register'),
    ('registration_email_whitelist', '',      'Comma-separated allowed email domains (empty = all)'),
    ('password_min_length',          '8',     'Minimum password length'),
    ('password_require_uppercase',   'false', 'Require at least one uppercase letter'),
    ('password_require_number',      'false', 'Require at least one number'),
    ('profile_allow_username_change','true',  'Allow users to change their own username'),
    ('sso_local_login_enabled',      'true',  'Allow local email/password login alongside SSO')
ON CONFLICT (key) DO NOTHING;

-- ── Enrollments ───────────────────────────────────────────────────────────────
-- Courses are identified by slug (filesystem), not a UUID.

CREATE TABLE IF NOT EXISTS enrollments (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_slug TEXT        NOT NULL,
    enrolled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, course_slug)
);

-- ── Lesson progress ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS lesson_progress (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_slug TEXT        NOT NULL,
    lesson_slug TEXT        NOT NULL,
    viewed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, course_slug, lesson_slug)
);

-- ── Git repositories ──────────────────────────────────────────────────────────
-- Users can connect any git host (GitHub, GitLab, Gitea, self-hosted…).

CREATE TABLE IF NOT EXISTS git_repos (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url            TEXT        NOT NULL,
    branch         TEXT        NOT NULL DEFAULT 'main',
    token_enc      TEXT,                              -- AES-256-GCM encrypted PAT, nullable
    status         TEXT        NOT NULL DEFAULT 'pending', -- pending|syncing|synced|error
    error_message  TEXT,
    last_synced_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, url)
);

-- ── Indexes ───────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_enrollments_user     ON enrollments(user_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_slug     ON enrollments(course_slug);
CREATE INDEX IF NOT EXISTS idx_lesson_progress_user ON lesson_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_lesson_progress_key  ON lesson_progress(course_slug, lesson_slug);
CREATE INDEX IF NOT EXISTS idx_git_repos_user       ON git_repos(user_id);

-- ── Default admin ─────────────────────────────────────────────────────────────
-- Seeded at startup by the application using the ADMIN_PASSWORD env var.
-- See db/seed.go — SeedAdmin().
