<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let username = '';
  let email = '';
  let password = '';
  let confirm = '';
  let loading = false;

  async function handleRegister() {
    if (!username || !email || !password) {
      toasts.error('Please fill in all fields');
      return;
    }
    if (password !== confirm) {
      toasts.error('Passwords do not match');
      return;
    }
    if (password.length < 8) {
      toasts.error('Password must be at least 8 characters');
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
            placeholder="Min. 8 characters" minlength="8" required />
        </div>
        <div>
          <label for="confirm" class="label">Confirm Password</label>
          <input id="confirm" type="password" class="input" bind:value={confirm}
            placeholder="••••••••" required />
        </div>
        <button type="submit" class="btn-primary w-full" disabled={loading}>
          {loading ? 'Creating account...' : 'Create Account'}
        </button>
      </form>

      <div class="mt-4 text-center text-sm">
        Already have an account?
        <a href="/login" class="text-primary-600 font-medium hover:underline">Sign in</a>
      </div>
    </div>
  </div>
</div>
