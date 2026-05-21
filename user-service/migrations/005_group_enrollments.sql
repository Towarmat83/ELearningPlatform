CREATE TABLE IF NOT EXISTS group_enrollments (
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    course_slug TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, course_slug)
);

CREATE INDEX IF NOT EXISTS idx_group_enrollments_group ON group_enrollments(group_id);
