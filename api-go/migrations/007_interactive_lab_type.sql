-- Migration 007 — Allow 'interactive' as a valid lab_type
ALTER TABLE labs
    DROP CONSTRAINT IF EXISTS labs_lab_type_check;

ALTER TABLE labs
    ADD CONSTRAINT labs_lab_type_check
    CHECK (lab_type IN ('form', 'ctf', 'interactive'));
