CREATE TABLE IF NOT EXISTS group_enrollments (
    groupId    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    courseSlug TEXT NOT NULL,
    createdAt  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (groupId, courseSlug)
);

CREATE INDEX IF NOT EXISTS idx_group_enrollments_group ON group_enrollments(groupId);
