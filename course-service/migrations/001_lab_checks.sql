CREATE TABLE IF NOT EXISTS lab_checks (
    id           BIGSERIAL PRIMARY KEY,
    username     TEXT        NOT NULL,
    course_slug  TEXT        NOT NULL,
    module_index INT         NOT NULL,
    module_name  TEXT        NOT NULL,
    allow        BOOLEAN     NOT NULL,
    violations   TEXT[]      NOT NULL DEFAULT '{}',
    checked_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS lab_checks_username_idx    ON lab_checks (username);
CREATE INDEX IF NOT EXISTS lab_checks_course_slug_idx ON lab_checks (course_slug);
