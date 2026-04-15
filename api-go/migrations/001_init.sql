-- Users table
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    VARCHAR(64) UNIQUE NOT NULL,
    email       VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role        VARCHAR(16) NOT NULL DEFAULT 'student' CHECK (role IN ('admin', 'student')),
    avatar_url  TEXT,
    bio         TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Courses table
CREATE TABLE IF NOT EXISTS courses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    thumbnail   TEXT,
    category    VARCHAR(64),
    difficulty  VARCHAR(16) CHECK (difficulty IN ('beginner', 'intermediate', 'advanced')),
    is_published BOOLEAN NOT NULL DEFAULT FALSE,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Labs table
CREATE TABLE IF NOT EXISTS labs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    title       VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    lab_type    VARCHAR(16) NOT NULL CHECK (lab_type IN ('form', 'ctf')),
    -- For 'form' labs: JSON array of questions with options and correct answers
    -- For 'ctf' labs: challenge description, hints
    content     JSONB NOT NULL DEFAULT '{}',
    -- Only used for ctf labs: the expected flag
    flag        TEXT,
    points      INTEGER NOT NULL DEFAULT 100,
    order_index INTEGER NOT NULL DEFAULT 0,
    is_published BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Enrollments
CREATE TABLE IF NOT EXISTS enrollments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id   UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    enrolled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, course_id)
);

-- Lab submissions (both form and ctf)
CREATE TABLE IF NOT EXISTS lab_submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lab_id          UUID NOT NULL REFERENCES labs(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- For ctf: the flag submitted
    -- For form: JSON of {question_id: answer}
    answer          JSONB NOT NULL,
    is_correct      BOOLEAN NOT NULL DEFAULT FALSE,
    score           INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 1,
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Progress tracking (best submission per user per lab)
CREATE TABLE IF NOT EXISTS lab_progress (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lab_id          UUID NOT NULL REFERENCES labs(id) ON DELETE CASCADE,
    course_id       UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    completed       BOOLEAN NOT NULL DEFAULT FALSE,
    best_score      INTEGER NOT NULL DEFAULT 0,
    total_attempts  INTEGER NOT NULL DEFAULT 0,
    completed_at    TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, lab_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_courses_created_by ON courses(created_by);
CREATE INDEX IF NOT EXISTS idx_labs_course_id ON labs(course_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_user ON enrollments(user_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_course ON enrollments(course_id);
CREATE INDEX IF NOT EXISTS idx_submissions_lab ON lab_submissions(lab_id);
CREATE INDEX IF NOT EXISTS idx_submissions_user ON lab_submissions(user_id);
CREATE INDEX IF NOT EXISTS idx_progress_user ON lab_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_progress_course ON lab_progress(course_id);

-- Default admin user (password: Admin@1234)
INSERT INTO users (username, email, password_hash, role)
VALUES (
    'admin',
    'admin@elearning.local',
    '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewRO1cGxbEj7RTUG',
    'admin'
) ON CONFLICT DO NOTHING;
