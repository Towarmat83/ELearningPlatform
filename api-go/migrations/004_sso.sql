-- Migration 004 — SSO: make password nullable, add OAuth provider columns
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider VARCHAR(32) NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_user_id VARCHAR(255);
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_provider_uid_key;
ALTER TABLE users ADD CONSTRAINT users_provider_uid_key UNIQUE (auth_provider, provider_user_id);
