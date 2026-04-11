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

  function bool(key: string, def = 'true') {
    return (edits[key] ?? def) === 'true';
  }
  function setbool(key: string, v: boolean) {
    edits[key] = v ? 'true' : 'false';
    edits = { ...edits };
  }

  // Password strength preview
  $: passwordMinLen = parseInt(edits['password_min_length'] ?? '8', 10) || 8;
  $: passwordPreview = [
    `At least ${passwordMinLen} characters`,
    bool('password_require_uppercase') ? 'At least one uppercase letter (A–Z)' : '',
    bool('password_require_number')    ? 'At least one number (0–9)' : '',
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
              {bool('registration_enabled') ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('registration_enabled', !bool('registration_enabled'))}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {bool('registration_enabled') ? 'translate-x-6' : 'translate-x-1'}"></span>
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
              {bool('password_require_uppercase', 'false') ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('password_require_uppercase', !bool('password_require_uppercase', 'false'))}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {bool('password_require_uppercase', 'false') ? 'translate-x-6' : 'translate-x-1'}"></span>
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
              {bool('password_require_number', 'false') ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('password_require_number', !bool('password_require_number', 'false'))}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {bool('password_require_number', 'false') ? 'translate-x-6' : 'translate-x-1'}"></span>
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
              {bool('profile_allow_username_change') ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('profile_allow_username_change', !bool('profile_allow_username_change'))}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {bool('profile_allow_username_change') ? 'translate-x-6' : 'translate-x-1'}"></span>
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
              {bool('sso_local_login_enabled') ? 'bg-emerald-500' : 'bg-gray-300'}"
            on:click={() => setbool('sso_local_login_enabled', !bool('sso_local_login_enabled'))}>
            <span class="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform
              {bool('sso_local_login_enabled') ? 'translate-x-6' : 'translate-x-1'}"></span>
          </button>
        </div>

        {#if !bool('sso_local_login_enabled')}
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

      <!-- Save -->
      <div class="flex justify-end pt-2">
        <button type="submit" class="btn-primary px-8" disabled={saving}>
          {saving ? 'Saving…' : 'Save Settings'}
        </button>
      </div>

    </form>
  {/if}
</div>
