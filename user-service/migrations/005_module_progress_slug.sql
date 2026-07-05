-- ── Add moduleSlug to module_progress ─────────────────────────────────────
-- Stores the derived slug of the module so user-service can check
-- prerequisite modules by slug without calling course-service.
-- Nullable: existing rows keep NULL; new submissions populate it going forward.

ALTER TABLE module_progress
    ADD COLUMN IF NOT EXISTS moduleSlug VARCHAR(255);

CREATE INDEX IF NOT EXISTS idx_module_progress_slug
    ON module_progress(userId, courseSlug, moduleSlug)
    WHERE moduleSlug IS NOT NULL;
