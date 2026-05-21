INSERT INTO platform_settings (key, value, description) VALUES
    ('oidc_browser_base_url', '', 'Base URL rewrite for OIDC auth redirect (browser-facing). Set when provider_url is cluster-internal (e.g. http://localhost:9000)')
ON CONFLICT (key) DO NOTHING;
