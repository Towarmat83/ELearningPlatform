CREATE TABLE IF NOT EXISTS path_enrollments (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path_slug   TEXT        NOT NULL,
    enrolled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, path_slug)
);

CREATE INDEX IF NOT EXISTS idx_path_enrollments_user ON path_enrollments(user_id);
CREATE INDEX IF NOT EXISTS idx_path_enrollments_slug ON path_enrollments(path_slug);
