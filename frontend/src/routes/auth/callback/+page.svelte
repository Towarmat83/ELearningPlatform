<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ssoApi, oidcApi } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  function decodeStateProvider(state: string): string {
    try {
      const payload = JSON.parse(atob(state.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
      return payload.provider || '';
    } catch {
      return '';
    }
  }

  let status: 'loading' | 'error' = 'loading';
  let errorMessage = '';

  onMount(async () => {
    const code = $page.url.searchParams.get('code');
    const state = $page.url.searchParams.get('state');
    const error = $page.url.searchParams.get('error');

    if (error) {
      status = 'error';
      errorMessage = error === 'access_denied'
        ? 'Access denied — you cancelled the sign-in.'
        : `OAuth error: ${error}`;
      return;
    }

    if (!code || !state) {
      status = 'error';
      errorMessage = 'Missing OAuth parameters. Please try signing in again.';
      return;
    }

    try {
      const provider = decodeStateProvider(state);
      const res = provider === 'oidc'
        ? await oidcApi.callback(code, state)
        : await ssoApi.callback(code, state);
      auth.login(res.token, res.user);
      toasts.success(`Welcome, ${res.user.username}!`);
      goto(res.user.role === 'admin' ? '/admin' : '/dashboard');
    } catch (e: any) {
      status = 'error';
      errorMessage = e.message || 'SSO authentication failed. Please try again.';
    }
  });
</script>

<svelte:head><title>Signing in… — LearnLab</title></svelte:head>

<div class="min-h-screen bg-gradient-to-br from-primary-50 to-blue-100 flex items-center justify-center p-4">
  <div class="card w-full max-w-sm text-center py-10">
    {#if status === 'loading'}
      <div class="text-4xl mb-4 animate-pulse">🔐</div>
      <h2 class="text-lg font-semibold text-gray-700">Completing sign-in…</h2>
      <p class="text-sm text-gray-400 mt-2">Please wait while we verify your identity.</p>
    {:else}
      <div class="text-4xl mb-4">❌</div>
      <h2 class="text-lg font-semibold text-red-600">Sign-in failed</h2>
      <p class="text-sm text-gray-500 mt-2">{errorMessage}</p>
      <a href="/login" class="btn-primary mt-6 inline-block">Back to login</a>
    {/if}
  </div>
</div>
