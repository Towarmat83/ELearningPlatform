<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { authApi, publicSettingsApi } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let username = '';
  let email = '';
  let password = '';
  let confirm = '';
  let loading = false;

  // Settings-driven rules
  let registrationEnabled = true;
  let passwordMinLen = 8;
  let requireUppercase = false;
  let requireNumber = false;

  onMount(async () => {
    try {
      const s = await publicSettingsApi.get();
      registrationEnabled = s.registration_enabled !== 'false';
      passwordMinLen = parseInt(s.password_min_length, 10) || 8;
      requireUppercase = s.password_require_uppercase === 'true';
      requireNumber    = s.password_require_number === 'true';
    } catch { /* use defaults */ }
  });

  $: passwordRules = [
    { label: `At least ${passwordMinLen} characters`, ok: password.length >= passwordMinLen },
    ...(requireUppercase ? [{ label: 'At least one uppercase letter', ok: /[A-Z]/.test(password) }] : []),
    ...(requireNumber    ? [{ label: 'At least one number', ok: /[0-9]/.test(password) }] : []),
  ];
  $: passwordValid = password.length > 0 && passwordRules.every(r => r.ok);

  async function handleRegister() {
    if (!username || !email || !password) {
      toasts.error('Please fill in all fields');
      return;
    }
    if (password !== confirm) {
      toasts.error('Passwords do not match');
      return;
    }
    if (!passwordValid) {
      toasts.error('Password does not meet the requirements');
      return;
    }
    loading = true;
    try {
      const res = await authApi.register(username, email, password);
      auth.login(res.token, res.user);
      toasts.success('Account created! Welcome!');
      goto('/dashboard');
    } catch (e: any) {
      toasts.error(e.message || 'Registration failed');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Register — LearnLab</title></svelte:head>

<div class="min-h-screen bg-gradient-to-br from-primary-50 to-blue-100 flex items-center justify-center p-4">
  <div class="w-full max-w-md">
    <div class="text-center mb-8">
      <a href="/" class="text-3xl font-bold text-primary-600">LearnLab</a>
      <p class="text-gray-500 mt-2">Create your account</p>
    </div>

    <div class="card">
      {#if !registrationEnabled}
        <div class="text-center py-8">
          <div class="text-4xl mb-3">🔒</div>
          <h2 class="font-semibold text-gray-700 mb-1">Registration is closed</h2>
          <p class="text-sm text-gray-400">New registrations are currently disabled by the administrator.</p>
          <a href="/login" class="btn-primary mt-4 inline-block text-sm">Back to login</a>
        </div>
      {:else}
        <form on:submit|preventDefault={handleRegister} class="space-y-4">
          <div>
            <label for="username" class="label">Username</label>
            <input id="username" type="text" class="input" bind:value={username}
              placeholder="johndoe" minlength="3" required />
          </div>
          <div>
            <label for="email" class="label">Email</label>
            <input id="email" type="email" class="input" bind:value={email}
              placeholder="your@email.com" required />
          </div>
          <div>
            <label for="password" class="label">Password</label>
            <input id="password" type="password" class="input" bind:value={password}
              placeholder="Min. {passwordMinLen} characters" required />

            <!-- Live password strength indicator -->
            {#if password.length > 0}
              <ul class="mt-2 space-y-0.5">
                {#each passwordRules as rule}
                  <li class="text-xs flex items-center gap-1.5
                    {rule.ok ? 'text-emerald-600' : 'text-gray-400'}">
                    <span>{rule.ok ? '✓' : '○'}</span>
                    {rule.label}
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
          <div>
            <label for="confirm" class="label">Confirm Password</label>
            <input id="confirm" type="password" class="input" bind:value={confirm}
              placeholder="••••••••" required />
            {#if confirm && confirm !== password}
              <p class="text-xs text-red-500 mt-1">Passwords do not match</p>
            {/if}
          </div>
          <button type="submit" class="btn-primary w-full" disabled={loading}>
            {loading ? 'Creating account...' : 'Create Account'}
          </button>
        </form>

        <div class="mt-4 text-center text-sm">
          Already have an account?
          <a href="/login" class="text-primary-600 font-medium hover:underline">Sign in</a>
        </div>
      {/if}
    </div>
  </div>
</div>
