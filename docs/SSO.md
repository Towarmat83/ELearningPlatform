# SSO / OAuth — Configuration Guide

The platform supports three authentication mechanisms in addition to local email/password:

- **`sso.providers`** (Helm) — OAuth2/OIDC providers configured at deploy time (GitHub, GitLab, Google, etc.)
- **OIDC via admin UI** — a single OIDC provider configured at runtime in `/admin/settings`, no Helm change required
- **LDAP / Active Directory** — authenticate against any LDAP directory, configured at runtime in `/admin/settings`

All three can be active simultaneously.

---

## Profile sync

On every SSO login the platform automatically syncs the user's profile from the identity provider:

| Field | Behaviour |
|---|---|
| **Avatar** | Always updated from the provider (`picture` claim / GitHub `avatar_url`). Displayed on the profile page with a graceful fallback to initials if the URL is unreachable. |
| **Bio** | Populated from the provider on first login only (`bio` / `description` / `about` claims). Once a user writes their own bio it is **never overwritten** by a subsequent login. |
| **Display name** | Used only when creating a new account (`name` → `preferred_username` → email). Not updated on subsequent logins to preserve user edits. |

For OIDC providers, the platform fetches claims from **both** the ID token and the [UserInfo endpoint](https://openid.net/specs/openid-connect-core-1_0.html#UserInfo) — the UserInfo endpoint takes priority, which gives access to richer or more up-to-date attributes.

**Profile page** shows an SSO provider badge (GitHub, GitLab, Google, etc.) and hides the password-change section for SSO-managed accounts.

---

## How it works

```
Browser → GET /api/auth/oauth/{id}/authorize  → redirect to provider
       ← provider redirects to /auth/callback?code=...
       → POST /api/auth/oauth/callback {code, state}
       ← JWT token
```

The backend reads `sso.providers` from the Helm configmap. For each provider with a non-empty `client_id`, a button appears on the login page.

**Two internal flows:**

| Provider `id` | Flow |
|---|---|
| `github` | OAuth2 direct (GitHub has no OIDC discovery) |
| anything else | OIDC discovery via `issuer_url` |

The `id` value is stored in the database as `auth_provider` for each user — choose a stable, meaningful value and don't change it once users have signed in.

---

## Helm configuration

```yaml
sso:
  enabled: false          # set to true to enable
  redirectBase: ""        # public URL of the frontend — defaults to ingress host
  providers: []
```

`redirectBase` must match the **Callback URL** registered with each provider.  
The actual redirect URI sent to providers is `{redirectBase}/auth/callback`.

---

## Supported providers

### GitHub

GitHub does not support OIDC. The backend uses the GitHub OAuth2 API directly.

```yaml
sso:
  enabled: true
  redirectBase: "https://your-app.com"
  providers:
    - id: github
      name: GitHub
      client_id: "Iv23li..."
      client_secret: "abc123..."
```

**GitHub App setup** (`github.com/settings/apps` → New GitHub App):

| Field | Value |
|---|---|
| Homepage URL | your frontend URL |
| Callback URL | `{redirectBase}/auth/callback` |
| Webhook | disabled |
| Account permissions → Email addresses | Read-only |
| Where can it be installed | Any account |

After creation: copy **Client ID** and generate a **Client Secret**.

---

### GitLab (SaaS or self-hosted)

Uses OIDC discovery. `issuer_url` defaults to `https://gitlab.com` when omitted —
only set it for self-hosted instances.

```yaml
providers:
  # GitLab SaaS — issuer_url optional
  - id: gitlab
    name: GitLab
    client_id: "xxx"
    client_secret: "yyy"

  # GitLab self-hosted — set issuer_url to your instance
  - id: gitlab
    name: GitLab
    client_id: "xxx"
    client_secret: "yyy"
    issuer_url: "https://gitlab.company.com"
```

**GitLab setup** (User Settings → Applications):

| Field | Value |
|---|---|
| Redirect URI | `{redirectBase}/auth/callback` |
| Scopes | `openid`, `email`, `profile` |

---

### Google

```yaml
providers:
  # issuer_url optional for Google, defaults to https://accounts.google.com
  - id: google
    name: Google
    client_id: "xxx.apps.googleusercontent.com"
    client_secret: "yyy"
```

**Google Cloud Console** (APIs & Services → Credentials → OAuth 2.0 Client ID):

| Field | Value |
|---|---|
| Application type | Web application |
| Authorised redirect URIs | `{redirectBase}/auth/callback` |

---

### Microsoft / Azure AD

```yaml
providers:
  - id: microsoft
    name: Microsoft
    client_id: "xxx"
    client_secret: "yyy"
    # Organisation accounts only:
    issuer_url: "https://login.microsoftonline.com/{tenant-id}/v2.0"
    # Personal + org accounts:
    # issuer_url: "https://login.microsoftonline.com/common/v2.0"
```

**Azure Portal** (App registrations → New registration):

| Field | Value |
|---|---|
| Redirect URI | `{redirectBase}/auth/callback` |
| Supported account types | as needed |

Add a client secret in **Certificates & secrets**.

---

### Authentik

```yaml
providers:
  - id: authentik
    name: "Company SSO"
    client_id: "xxx"
    client_secret: "yyy"
    issuer_url: "https://authentik.company.com/application/o/{app-slug}/"
```

The `issuer_url` is shown in the Authentik provider detail page.  
Required scopes: `openid`, `email`, `profile`.

---

### Keycloak

```yaml
providers:
  - id: keycloak
    name: "Company SSO"
    client_id: "elearning"
    client_secret: "yyy"
    issuer_url: "https://keycloak.company.com/realms/{realm-name}"
```

**Keycloak admin** (Clients → Create):

| Field | Value |
|---|---|
| Client type | OpenID Connect |
| Valid redirect URIs | `{redirectBase}/auth/callback` |
| Client authentication | On (to get a secret) |

---

### Okta

```yaml
providers:
  - id: okta
    name: Okta
    client_id: "xxx"
    client_secret: "yyy"
    issuer_url: "https://your-domain.okta.com"
```

---

### Auth0

```yaml
providers:
  - id: auth0
    name: Auth0
    client_id: "xxx"
    client_secret: "yyy"
    issuer_url: "https://your-domain.auth0.com"
```

---

### Any OIDC-compatible provider

Any provider that exposes a standard OIDC discovery endpoint at  
`{issuer_url}/.well-known/openid-configuration` works out of the box:

```yaml
providers:
  - id: my-sso
    name: "My SSO"
    client_id: "xxx"
    client_secret: "yyy"
    issuer_url: "https://sso.company.com"
```

---

## Multiple providers

You can list as many providers as needed — each gets its own button on the login page:

```yaml
sso:
  enabled: true
  redirectBase: "https://your-app.com"
  providers:
    - id: github
      name: GitHub
      client_id: "..."
      client_secret: "..."
    - id: gitlab
      name: GitLab
      client_id: "..."
      client_secret: "..."
      issuer_url: "https://gitlab.com"
    - id: google
      name: Google
      client_id: "..."
      client_secret: "..."
      issuer_url: "https://accounts.google.com"
```

---

## Adding a provider icon (frontend)

The login page renders an SVG icon for each provider. Icons for `github` and `gitlab`
are built in. For any other provider, the fallback is `🔑`.

To add an icon, edit `frontend/src/routes/login/+page.svelte` and add an entry to the
`providerIcon` map (around line 88):

```typescript
const providerIcon: Record<string, string> = {
  gitlab: `<svg ...>...</svg>`,
  github: `<svg ...>...</svg>`,

  // Add your provider here — inline SVG, class="w-5 h-5"
  google: `<svg viewBox="0 0 24 24" class="w-5 h-5">
    <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
    <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
    <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z"/>
    <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
  </svg>`,

  microsoft: `<svg viewBox="0 0 24 24" class="w-5 h-5">
    <path fill="#F25022" d="M1 1h10v10H1z"/>
    <path fill="#00A4EF" d="M13 1h10v10H13z"/>
    <path fill="#7FBA00" d="M1 13h10v10H1z"/>
    <path fill="#FFB900" d="M13 13h10v10H13z"/>
  </svg>`,

  okta: `<svg viewBox="0 0 24 24" class="w-5 h-5" fill="#007DC1">
    <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm0 18a6 6 0 110-12 6 6 0 010 12z"/>
  </svg>`,
};
```

**Rules:**
- Use `class="w-5 h-5"` on the `<svg>` to match the button size
- The key must exactly match the `id` used in `sso.providers`
- Inline SVG only — no external images (CSP)
- If no entry exists for a provider `id`, the fallback `🔑` is shown automatically

---

## Admin UI OIDC (runtime, no Helm)

One additional OIDC provider can be configured at runtime in `/admin/settings`:

| Setting key | Description |
|---|---|
| `oidc_enabled` | `true` / `false` |
| `oidc_provider_url` | OIDC issuer URL |
| `oidc_client_id` | Client ID |
| `oidc_client_secret` | Client Secret |
| `oidc_scopes` | Space-separated scopes (default: `openid email profile groups`) |
| `oidc_group_claim` | JWT claim containing group names (default: `groups`) |
| `oidc_browser_base_url` | Override issuer base URL for browser redirects (split-horizon DNS) |

When enabled, a **"Continue with SSO (OIDC)"** button appears on the login page.  
This is useful for an enterprise IdP (Authentik, Keycloak, Azure AD) managed by an admin
without needing a Helm upgrade.

---

## LDAP / Active Directory (runtime, no Helm)

LDAP authentication is built into user-service — no separate microservice required.
Configure it at runtime in `/admin/settings`:

| Setting key | Description | Example |
|---|---|---|
| `ldap_enabled` | Enable LDAP login | `true` |
| `ldap_server_url` | LDAP server URL | `ldap://openldap:389` or `ldaps://ad.company.com:636` |
| `ldap_bind_dn` | Service account DN (leave empty for anonymous bind) | `cn=svc-elearning,ou=service,dc=company,dc=com` |
| `ldap_bind_password` | Service account password | `secret` |
| `ldap_user_base_dn` | Base DN for user search | `ou=users,dc=company,dc=com` |
| `ldap_user_filter` | Search filter — `%s` is replaced by the user's email | `(mail=%s)` |
| `ldap_group_base_dn` | Base DN for group search (leave empty to skip group sync) | `ou=groups,dc=company,dc=com` |
| `ldap_group_filter` | Group membership filter — `%s` is the user's DN | `(&#124;(member=%s)(uniqueMember=%s)(memberUid=%s))` |

When enabled, a **LDAP** login option is shown on the login page alongside the email/password form.

### How it works

1. User enters their email and password
2. The service binds with the service account (if configured), then searches for the user by email using `ldap_user_filter`
3. The service re-binds with the found user's DN and the provided password to authenticate
4. If `ldap_group_base_dn` is set, the user's groups are fetched using `ldap_group_filter` and synced to the platform (group → role mappings apply)
5. A JWT is issued and the user is redirected

### OpenLDAP example

```
ldap_server_url       = ldap://openldap.infra.svc.cluster.local:389
ldap_bind_dn          = cn=admin,dc=company,dc=com
ldap_bind_password    = adminpassword
ldap_user_base_dn     = ou=users,dc=company,dc=com
ldap_user_filter      = (mail=%s)
ldap_group_base_dn    = ou=groups,dc=company,dc=com
ldap_group_filter     = (|(member=%s)(uniqueMember=%s)(memberUid=%s))
```

### Active Directory example

```
ldap_server_url       = ldaps://ad.company.com:636
ldap_bind_dn          = CN=svc-elearning,OU=Service Accounts,DC=company,DC=com
ldap_bind_password    = secret
ldap_user_base_dn     = OU=Users,DC=company,DC=com
ldap_user_filter      = (userPrincipalName=%s)
ldap_group_base_dn    = OU=Groups,DC=company,DC=com
ldap_group_filter     = (member=%s)
```

### Authentik LDAP outpost

Authentik can expose an LDAP outpost that proxies your Authentik users and groups:

```
ldap_server_url       = ldap://authentik-ldap.authentik.svc.cluster.local:389
ldap_bind_dn          = cn=ldapservice,ou=serviceaccounts,dc=ldap,dc=goauthentik,dc=io
ldap_bind_password    = <token from Authentik outpost>
ldap_user_base_dn     = dc=ldap,dc=goauthentik,dc=io
ldap_user_filter      = (mail=%s)
ldap_group_base_dn    = ou=groups,dc=ldap,dc=goauthentik,dc=io
ldap_group_filter     = (|(member=%s)(uniqueMember=%s))
```

Create the outpost in Authentik: **Directory → Federation & Social login → LDAP outpost** (or via the Outposts menu). The bind DN and password come from the outpost's service account token.

### Group → role mapping

LDAP groups are synced as platform groups on every login. Use `/admin/groups` to assign a role to a group:

| LDAP group CN | Platform role |
|---|---|
| `elearning-admins` | `admin` |
| `elearning-students` | `student` |

If a user belongs to multiple groups, the highest role wins (`admin` > `student`).
