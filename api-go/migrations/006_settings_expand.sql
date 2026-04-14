-- Migration 006 — Expand platform settings with user/auth configuration
INSERT INTO platform_settings (key, value, description) VALUES
    ('registration_enabled',        'true',  'Allow new users to self-register'),
    ('registration_email_whitelist','',       'Comma-separated allowed email domains (empty = all). e.g. company.com,partner.com'),
    ('password_min_length',         '8',     'Minimum password length'),
    ('password_require_uppercase',  'false', 'Require at least one uppercase letter in passwords'),
    ('password_require_number',     'false', 'Require at least one number in passwords'),
    ('profile_allow_username_change','true', 'Allow users to change their own username'),
    ('sso_local_login_enabled',     'true',  'Allow local email/password login alongside SSO providers')
ON CONFLICT (key) DO NOTHING;
