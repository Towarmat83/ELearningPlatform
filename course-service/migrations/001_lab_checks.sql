CREATE TABLE IF NOT EXISTS lab_checks (
    id           BIGSERIAL PRIMARY KEY,
    username     TEXT        NOT NULL,
    courseSlug  TEXT        NOT NULL,
    moduleIndex INT         NOT NULL,
    moduleName  TEXT        NOT NULL,
    allow        BOOLEAN     NOT NULL,
    violations   TEXT[]      NOT NULL DEFAULT '{}',
    checkedAt   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS lab_checks_username_idx    ON lab_checks (username);
CREATE INDEX IF NOT EXISTS lab_checks_course_slug_idx ON lab_checks (courseSlug);
