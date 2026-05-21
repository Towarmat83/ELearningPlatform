<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { authApi, ssoApi, oidcApi, ldapApi, type OAuthProvider } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  export let data: { settings: Record<string, string> };

  let email = '';
  let password = '';
  let loading = false;
  let ssoLoading = '';
  let providers: OAuthProvider[] = [];

  let localLoginEnabled = data.settings.sso_local_login_enabled !== 'false';
  let oidcEnabled = data.settings.oidc_enabled === 'true';
  let ldapEnabled = data.settings.ldap_enabled === 'true';

  // True when at least one SSO method is available — form is collapsed by default.
  $: ssoActive = oidcEnabled || providers.length > 0;
  let showLocalForm = !oidcEnabled;

  onMount(async () => {
    const ssoRes = await Promise.allSettled([ssoApi.providers()]);
    if (ssoRes[0].status === 'fulfilled') {
      providers = ssoRes[0].value.providers;
      if (providers.length > 0) showLocalForm = false;
    }
  });

  async function handleLogin() {
    if (!email || !password) {
      toasts.error('Please fill in all fields');
      return;
    }
    loading = true;
    const redirect = $page.url.searchParams.get('redirect');
    try {
      const res = await authApi.login(email, password);
      auth.login(res.token, res.user);
      toasts.success(`Welcome back, ${res.user.username}!`);
      goto(redirect ?? (res.user.role === 'admin' ? '/admin' : '/dashboard'));
      return;
    } catch (e: any) {
      // Fallback to LDAP only for "user not found / wrong password" errors.
      // Don't fallback for SSO-only accounts or other errors.
      if (ldapEnabled && e.message === 'Invalid email or password') {
        try {
          const res = await ldapApi.login(email, password);
          auth.login(res.token, res.user);
          toasts.success(`Welcome back, ${res.user.username}!`);
          goto(redirect ?? (res.user.role === 'admin' ? '/admin' : '/dashboard'));
          return;
        } catch (ldapErr: any) {
          toasts.error(ldapErr.message || 'Invalid email or password');
        }
      } else {
        toasts.error(e.message || 'Login failed');
      }
    } finally {
      loading = false;
    }
  }

  async function loginWithProvider(providerId: string) {
    ssoLoading = providerId;
    try {
      const { url } = await ssoApi.authorize(providerId);
      window.location.href = url;
    } catch (e: any) {
      toasts.error(e.message || `Failed to start ${providerId} sign-in`);
      ssoLoading = '';
    }
  }

  async function loginWithOIDC() {
    ssoLoading = 'oidc';
    try {
      const { url } = await oidcApi.authorize();
      window.location.href = url;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to start SSO sign-in');
      ssoLoading = '';
    }
  }

  const providerIcon: Record<string, string> = {
    gitlab: `<svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor">
      <path d="M22.65 14.39L12 22.13 1.35 14.39a.84.84 0 0 1-.3-.94l1.22-3.78 2.44-7.51A.42.42 0 0 1 4.82 2a.43.43 0 0 1 .58 0 .42.42 0 0 1 .11.18l2.44 7.49h8.1l2.44-7.49a.42.42 0 0 1 .11-.18.43.43 0 0 1 .58 0 .42.42 0 0 1 .11.18l2.44 7.51L23 13.45a.84.84 0 0 1-.35.94z" fill="#FC6D26"/>
    </svg>`,
    github: `<svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
    </svg>`,
  };
</script>

<svelte:head><title>Login — LearnLab</title></svelte:head>

<div class="min-h-screen bg-gradient-to-br from-primary-50 to-blue-100 flex items-center justify-center p-4">
  <div class="w-full max-w-md">
    <div class="text-center mb-8">
      <a href="/" class="text-3xl font-bold text-primary-600">LearnLab</a>
      <p class="text-gray-500 mt-2">Sign in to your account</p>
    </div>

    <div class="card">
      <!-- SSO / OIDC / LDAP Buttons -->
      {#if providers.length > 0 || oidcEnabled || ldapEnabled}
        <div class="space-y-2 mb-5">
          {#each providers as provider}
            <button
              type="button"
              class="w-full flex items-center justify-center gap-3 px-4 py-2.5 rounded-lg border border-gray-200 bg-white hover:bg-gray-50 text-gray-700 font-medium text-sm transition-colors disabled:opacity-60"
              on:click={() => loginWithProvider(provider.id)}
              disabled={!!ssoLoading}
            >
              {#if ssoLoading === provider.id}
                <span class="animate-spin text-base">↻</span>
              {:else}
                {@html providerIcon[provider.id] ?? '🔑'}
              {/if}
              Continue with {provider.name}
            </button>
          {/each}

          {#if oidcEnabled}
            <button
              type="button"
              class="w-full flex items-center justify-center gap-3 px-4 py-2.5 rounded-lg border border-blue-200 bg-blue-50 hover:bg-blue-100 text-blue-700 font-medium text-sm transition-colors disabled:opacity-60"
              on:click={loginWithOIDC}
              disabled={!!ssoLoading}
            >
              {#if ssoLoading === 'oidc'}
                <span class="animate-spin text-base">↻</span>
              {:else}
                <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10"/><path d="M12 8v4l3 3"/>
                </svg>
              {/if}
              Continue with SSO (OIDC)
            </button>
          {/if}

        </div>

        <!-- Toggle to show email/password form -->
        <button
          type="button"
          class="w-full flex items-center justify-center gap-2 text-sm text-gray-400 hover:text-gray-600 transition-colors mb-1"
          on:click={() => showLocalForm = !showLocalForm}
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4 transition-transform {showLocalForm ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 9l-7 7-7-7"/>
          </svg>
          Sign in with email{ldapEnabled ? ' / LDAP' : ''}
        </button>
      {/if}

      {#if !ssoActive || showLocalForm}
        {#if !localLoginEnabled}
          <div class="bg-amber-50 border border-amber-200 rounded-lg p-3 text-sm text-amber-700 text-center mb-4">
            Local login is disabled. Please use an SSO provider above.
          </div>
        {/if}
        <form on:submit|preventDefault={handleLogin} class="space-y-4 {ssoActive ? 'mt-3' : ''}" class:opacity-40={!localLoginEnabled} class:pointer-events-none={!localLoginEnabled}>
          <div>
            <label for="email" class="label">Email</label>
            <input id="email" type="email" class="input" bind:value={email}
              placeholder="your@email.com" required />
          </div>
          <div>
            <label for="password" class="label">Password</label>
            <input id="password" type="password" class="input" bind:value={password}
              placeholder="••••••••" required />
          </div>
          <button type="submit" class="btn-primary w-full" disabled={loading || !!ssoLoading}>
            {loading ? 'Signing in…' : 'Sign In'}
          </button>
          {#if ldapEnabled}
            <p class="text-xs text-center text-gray-400">
              Also works with LDAP / Active Directory accounts
            </p>
          {/if}
        </form>
      {/if}

      <div class="mt-4 text-center text-sm">
        Don't have an account?
        <a href="/register" class="text-primary-600 font-medium hover:underline">Sign up</a>
      </div>
    </div>
  </div>
</div>
