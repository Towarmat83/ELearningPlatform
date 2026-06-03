<script lang="ts">
  import { onMount } from 'svelte';
  import { adminSettingsApi, type PlatformSetting } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let settings: PlatformSetting[] = [];
  let loading = true;
  let saving = false;
  let edits: Record<string, string> = {};

  onMount(async () => {
    try {
      const res = await adminSettingsApi.get($auth.token!);
      settings = res.settings;
      for (const s of settings) edits[s.key] = s.value;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load settings');
    } finally {
      loading = false;
    }
  });

  async function save() {
    saving = true;
    try {
      const res = await adminSettingsApi.update(edits, $auth.token!);
      settings = res.settings;
      for (const s of settings) edits[s.key] = s.value;
      toasts.success('Settings saved');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to save settings');
    } finally {
      saving = false;
    }
  }

  function setbool(key: string, v: boolean) {
    edits[key] = v ? 'true' : 'false';
    edits = { ...edits };
  }

  // Reactive booleans — $: declarations let Svelte track the edits dependency
  // directly, so the DOM updates when setbool() reassigns edits.
  $: regEnabled    = (edits['registration_enabled']          ?? 'true')  === 'true';
  $: localLogin    = (edits['sso_local_login_enabled']        ?? 'true')  === 'true';
  $: oidcEnabled   = (edits['oidc_enabled']                  ?? 'false') === 'true';
  $: ldapEnabled   = (edits['ldap_enabled']                  ?? 'false') === 'true';
  $: pwdUppercase  = (edits['password_require_uppercase']     ?? 'false') === 'true';
  $: pwdNumber     = (edits['password_require_number']        ?? 'false') === 'true';
  $: profileUname  = (edits['profile_allow_username_change']  ?? 'true')  === 'true';

  // Password strength preview
  $: passwordMinLen = parseInt(edits['password_min_length'] ?? '8', 10) || 8;
  $: passwordPreview = [
    `At least ${passwordMinLen} characters`,
    pwdUppercase ? 'At least one uppercase letter (A–Z)' : '',
    pwdNumber    ? 'At least one number (0–9)' : '',
  ].filter(Boolean);
</script>

<svelte:head><title>Settings — Admin</title></svelte:head>

<div class="p-8 max-w-3xl">
  <h1 class="text-2xl font-bold text-gray-900 mb-1">Platform Settings</h1>
  <p class="text-gray-500 text-sm mb-8">Runtime configuration — changes take effect immediately without restarting.</p>

  {#if loading}
    <div class="text-gray-400 py-12 text-center">Loading…</div>
  {:else}
    <form on:submit|preventDefault={save} class="space-y-6">

      <!-- ── Registration ───────────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          📝 Registration
        </h2>

        <!-- registration_enabled -->
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Allow public registration</p>
            <p class="text-xs text-gray-400">When off, only admins can create accounts.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {regEnabled ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('registration_enabled', !regEnabled)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {regEnabled ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        <!-- registration_email_whitelist -->
        <div>
          <label class="label" for="email_whitelist">Email domain whitelist</label>
          <p class="text-xs text-gray-400 mb-1">
            Comma-separated domains. Leave empty to allow all.
            Example: <code class="bg-gray-100 px-1 rounded">company.com, partner.org</code>
          </p>
          <input
            id="email_whitelist"
            type="text"
            class="input font-mono text-sm"
            bind:value={edits['registration_email_whitelist']}
            placeholder="company.com, partner.org"
          />
          {#if edits['registration_email_whitelist']?.trim()}
            <div class="mt-1 flex flex-wrap gap-1">
              {#each edits['registration_email_whitelist'].split(',').map(s => s.trim()).filter(Boolean) as domain}
                <span class="text-xs bg-emerald-100 text-emerald-700 px-2 py-0.5 rounded-full font-mono">@{domain}</span>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <!-- ── Password Policy ───────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          🔑 Password Policy
        </h2>

        <!-- password_min_length -->
        <div class="flex items-center justify-between gap-4">
          <div class="flex-1">
            <label class="label mb-0" for="pwd_min_len">Minimum length</label>
            <p class="text-xs text-gray-400">Applies to registration and password changes.</p>
          </div>
          <input
            id="pwd_min_len"
            type="number"
            min="4" max="128"
            class="input w-24 text-center font-mono"
            bind:value={edits['password_min_length']}
          />
        </div>

        <!-- password_require_uppercase -->
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Require uppercase letter</p>
            <p class="text-xs text-gray-400">At least one A–Z character.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {pwdUppercase ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('password_require_uppercase', !pwdUppercase)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {pwdUppercase ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        <!-- password_require_number -->
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Require number</p>
            <p class="text-xs text-gray-400">At least one digit 0–9.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {pwdNumber ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('password_require_number', !pwdNumber)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {pwdNumber ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        <!-- Live preview -->
        <div class="bg-gray-50 rounded-lg p-3 border border-gray-100">
          <p class="text-xs font-medium text-gray-500 mb-1">Current password requirements:</p>
          <ul class="space-y-0.5">
            {#each passwordPreview as rule}
              <li class="text-xs text-gray-600 flex items-center gap-1.5">
                <span class="text-emerald-500">✓</span> {rule}
              </li>
            {/each}
          </ul>
        </div>
      </div>

      <!-- ── User Profiles ─────────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          👤 User Profiles
        </h2>

        <!-- profile_allow_username_change -->
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Allow username change</p>
            <p class="text-xs text-gray-400">Let users update their own username from their profile.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {profileUname ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('profile_allow_username_change', !profileUname)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {profileUname ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>
      </div>

      <!-- ── SSO ───────────────────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          🔐 SSO & Authentication
        </h2>

        <!-- sso_local_login_enabled -->
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Allow local login</p>
            <p class="text-xs text-gray-400">
              Email + password login. Disable to force SSO-only authentication.
            </p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {localLogin ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('sso_local_login_enabled', !localLogin)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {localLogin ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        {#if !localLogin}
          <div class="bg-amber-50 border border-amber-200 rounded-lg p-3 text-xs text-amber-700">
            ⚠️ Local login is disabled. Make sure at least one SSO provider is configured before saving, otherwise no one will be able to sign in.
          </div>
        {/if}

        <!-- gitlab_url -->
        <div>
          <label class="label" for="gitlab_url">GitLab Instance URL</label>
          <p class="text-xs text-gray-400 mb-1">
            Override for self-hosted GitLab. OAuth credentials (Client ID / Secret) are set via environment variables.
          </p>
          <input
            id="gitlab_url"
            type="url"
            class="input font-mono text-sm"
            bind:value={edits['gitlab_url']}
            placeholder="https://gitlab.com"
          />
        </div>
      </div>

      <!-- ── OIDC ─────────────────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          🌐 OIDC (OpenID Connect)
        </h2>
        <p class="text-xs text-gray-400 -mt-2">SSO with Authentik, Keycloak, Azure AD, Google Workspace…</p>

        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Enable OIDC authentication</p>
            <p class="text-xs text-gray-400">Show OIDC button on login page and activate the flow.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {oidcEnabled ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('oidc_enabled', !oidcEnabled)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {oidcEnabled ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        {#if oidcEnabled && !edits['oidc_provider_url']?.trim()}
          <div class="bg-amber-50 border border-amber-200 rounded-lg p-3 text-xs text-amber-700">
            ⚠️ OIDC is enabled but no Provider URL is set. Users won't be able to sign in until configured.
          </div>
        {/if}

        <div>
          <label class="label" for="oidc_provider_url">Provider Discovery URL</label>
          <p class="text-xs text-gray-400 mb-1">
            Authentik example: <code class="bg-gray-100 px-1 rounded">https://auth.example.com/application/o/&lt;slug&gt;</code>.
            Use the cluster-internal URL (e.g. <code class="bg-gray-100 px-1 rounded">http://authentik-server.authentik.svc.cluster.local/application/o/elearning-oidc/</code>) when running in Kubernetes.
          </p>
          <input id="oidc_provider_url" type="text" class="input font-mono text-sm"
            bind:value={edits['oidc_provider_url']}
            placeholder="https://auth.example.com/application/o/elearning" />
        </div>

        <div>
          <label class="label" for="oidc_redirect_base">Redirect base URL <span class="text-gray-400 font-normal">(optional)</span></label>
          <p class="text-xs text-gray-400 mb-1">
            Public URL of this app sent to the OIDC provider as <code class="bg-gray-100 px-1 rounded">redirect_uri</code> (e.g. <code class="bg-gray-100 px-1 rounded">https://elearning.example.com</code>). Defaults to the value set in Helm (<code class="bg-gray-100 px-1 rounded">sso.redirectBase</code>).
          </p>
          <input id="oidc_redirect_base" type="text" class="input font-mono text-sm"
            bind:value={edits['oidc_redirect_base']}
            placeholder="https://elearning.example.com" />
        </div>

        <div>
          <label class="label" for="oidc_issuer_url">Issuer URL override <span class="text-gray-400 font-normal">(optional)</span></label>
          <p class="text-xs text-gray-400 mb-1">
            Split-horizon only: public issuer URL embedded in tokens when Provider URL is a cluster-internal address (e.g. <code class="bg-gray-100 px-1 rounded">http://localhost:9090</code>). Leave empty in production.
          </p>
          <input id="oidc_issuer_url" type="text" class="input font-mono text-sm"
            bind:value={edits['oidc_issuer_url']}
            placeholder="http://localhost:9090" />
        </div>

        <div>
          <label class="label" for="oidc_browser_base_url">Browser base URL <span class="text-gray-400 font-normal">(optional)</span></label>
          <p class="text-xs text-gray-400 mb-1">
            Only needed when Provider URL is cluster-internal. The auth redirect will use this base instead (e.g. <code class="bg-gray-100 px-1 rounded">http://localhost:9000</code>).
          </p>
          <input id="oidc_browser_base_url" type="text" class="input font-mono text-sm"
            bind:value={edits['oidc_browser_base_url']}
            placeholder="http://localhost:9000" />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label" for="oidc_client_id">Client ID</label>
            <input id="oidc_client_id" type="text" class="input font-mono text-sm"
              bind:value={edits['oidc_client_id']} placeholder="client-id" />
          </div>
          <div>
            <label class="label" for="oidc_client_secret">Client Secret</label>
            <input id="oidc_client_secret" type="password" class="input font-mono text-sm"
              bind:value={edits['oidc_client_secret']} placeholder="••••••••" />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label" for="oidc_scopes">Scopes</label>
            <p class="text-xs text-gray-400 mb-1">Space-separated. Include <code class="bg-gray-100 px-1 rounded">groups</code> for group sync.</p>
            <input id="oidc_scopes" type="text" class="input font-mono text-sm"
              bind:value={edits['oidc_scopes']} placeholder="openid email profile groups" />
          </div>
          <div>
            <label class="label" for="oidc_group_claim">Group claim name</label>
            <p class="text-xs text-gray-400 mb-1">JWT claim containing group list.</p>
            <input id="oidc_group_claim" type="text" class="input font-mono text-sm"
              bind:value={edits['oidc_group_claim']} placeholder="groups" />
          </div>
        </div>
      </div>

      <!-- ── LDAP ─────────────────────────────────────────────────────── -->
      <div class="card space-y-5">
        <h2 class="font-semibold text-gray-800 flex items-center gap-2 text-base">
          🗂️ LDAP / Active Directory
        </h2>
        <p class="text-xs text-gray-400 -mt-2">Enterprise directory authentication via Authentik LDAP, OpenLDAP, Active Directory…</p>

        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-700">Enable LDAP authentication</p>
            <p class="text-xs text-gray-400">Show LDAP login option on the login page.</p>
          </div>
          <button type="button"
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
              {ldapEnabled ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('ldap_enabled', !ldapEnabled)}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {ldapEnabled ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        <div>
          <label class="label" for="ldap_server_url">Server URL</label>
          <input id="ldap_server_url" type="text" class="input font-mono text-sm"
            bind:value={edits['ldap_server_url']}
            placeholder="ldap://authentik.example.com:389" />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label" for="ldap_bind_dn">Service account DN</label>
            <p class="text-xs text-gray-400 mb-1">Used to search the directory.</p>
            <input id="ldap_bind_dn" type="text" class="input font-mono text-xs"
              bind:value={edits['ldap_bind_dn']}
              placeholder="cn=service,ou=accounts,dc=example,dc=com" />
          </div>
          <div>
            <label class="label" for="ldap_bind_password">Service account password</label>
            <input id="ldap_bind_password" type="password" class="input font-mono text-sm"
              bind:value={edits['ldap_bind_password']} placeholder="••••••••" />
          </div>
        </div>

        <div>
          <label class="label" for="ldap_user_base_dn">User base DN</label>
          <input id="ldap_user_base_dn" type="text" class="input font-mono text-xs"
            bind:value={edits['ldap_user_base_dn']}
            placeholder="ou=users,dc=ldap,dc=goauthentik,dc=io" />
        </div>

        <div>
          <label class="label" for="ldap_user_filter">User search filter</label>
          <p class="text-xs text-gray-400 mb-1"><code class="bg-gray-100 px-1 rounded">%s</code> is replaced with the user's email.</p>
          <input id="ldap_user_filter" type="text" class="input font-mono text-sm"
            bind:value={edits['ldap_user_filter']} placeholder="(mail=%s)" />
        </div>

        <div>
          <label class="label" for="ldap_group_base_dn">Group base DN <span class="text-gray-400 font-normal">(optional)</span></label>
          <input id="ldap_group_base_dn" type="text" class="input font-mono text-xs"
            bind:value={edits['ldap_group_base_dn']}
            placeholder="ou=groups,dc=ldap,dc=goauthentik,dc=io" />
        </div>

        <div>
          <label class="label" for="ldap_group_filter">Group search filter</label>
          <p class="text-xs text-gray-400 mb-1"><code class="bg-gray-100 px-1 rounded">%s</code> is replaced with the user's DN (up to 3 times).</p>
          <input id="ldap_group_filter" type="text" class="input font-mono text-xs"
            bind:value={edits['ldap_group_filter']}
            placeholder="(|(member=%s)(uniqueMember=%s)(memberUid=%s))" />
        </div>
      </div>

      <!-- Save -->
      <div class="flex justify-end pt-2">
        <button type="submit" class="btn-primary px-8" disabled={saving}>
          {saving ? 'Saving…' : 'Save Settings'}
        </button>
      </div>

    </form>
  {/if}
</div>
