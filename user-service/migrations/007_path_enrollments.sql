CREATE TABLE IF NOT EXISTS path_enrollments (
    userId     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path_slug   TEXT        NOT NULL,
    enrolledAt TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (userId, path_slug)
);

CREATE INDEX IF NOT EXISTS idx_path_enrollments_user ON path_enrollments(userId);
CREATE INDEX IF NOT EXISTS idx_path_enrollments_slug ON path_enrollments(path_slug);
