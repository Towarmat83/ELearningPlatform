-- ── Groups (synced from IdP on each login) ────────────────────────────────────

CREATE TABLE IF NOT EXISTS groups (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    source      VARCHAR(32) NOT NULL DEFAULT 'local', -- 'oidc', 'ldap', 'local'
    createdAt  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updatedAt  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── User → Group membership ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS user_groups (
    userId  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    groupId UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (userId, groupId)
);

-- ── Group → Platform role mapping ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS group_role_mappings (
    groupName    VARCHAR(255) PRIMARY KEY,
    platformRole VARCHAR(16) NOT NULL CHECK (platformRole IN ('admin', 'student'))
);

-- ── Indexes ───────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_user_groups_user  ON user_groups(userId);
CREATE INDEX IF NOT EXISTS idx_user_groups_group ON user_groups(groupId);

-- ── Platform settings: OIDC ───────────────────────────────────────────────────

INSERT INTO platform_settings (key, value, description) VALUES
    ('oidc_enabled',       'false',                      'Activer l''authentification OIDC'),
    ('oidc_provider_url',  '',                           'URL de découverte OIDC (ex: https://auth.example.com/application/o/slug)'),
    ('oidc_client_id',     '',                           'Client ID OIDC'),
    ('oidc_client_secret', '',                           'Client secret OIDC'),
    ('oidc_scopes',        'openid email profile groups','Scopes OIDC demandés'),
    ('oidc_group_claim',   'groups',                     'Nom du claim contenant les groupes dans l''ID token')
ON CONFLICT (key) DO NOTHING;

-- ── Platform settings: LDAP ───────────────────────────────────────────────────

INSERT INTO platform_settings (key, value, description) VALUES
    ('ldap_enabled',       'false',                                     'Activer l''authentification LDAP'),
    ('ldap_server_url',    '',                                          'URL du serveur LDAP (ex: ldap://authentik:389)'),
    ('ldap_bind_dn',       '',                                          'DN du compte de service LDAP'),
    ('ldap_bind_password', '',                                          'Mot de passe du compte de service LDAP'),
    ('ldap_user_base_dn',  '',                                          'Base DN pour la recherche d''utilisateurs'),
    ('ldap_user_filter',   '(mail=%s)',                                 'Filtre LDAP pour trouver l''utilisateur par email'),
    ('ldap_group_base_dn', '',                                          'Base DN pour la recherche de groupes'),
    ('ldap_group_filter',  '(|(member=%s)(uniqueMember=%s)(memberUid=%s))', 'Filtre LDAP groupes (%s = DN utilisateur)')
ON CONFLICT (key) DO NOTHING;
