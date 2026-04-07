<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { authApi } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let email = '';
  let password = '';
  let loading = false;

  async function handleLogin() {
    if (!email || !password) {
      toasts.error('Please fill in all fields');
      return;
    }
    loading = true;
    try {
      const res = await authApi.login(email, password);
      auth.login(res.token, res.user);
      toasts.success(`Welcome back, ${res.user.username}!`);
      const redirect = $page.url.searchParams.get('redirect');
      goto(redirect ?? (res.user.role === 'admin' ? '/admin' : '/dashboard'));
    } catch (e: any) {
      toasts.error(e.message || 'Login failed');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Login — LearnLab</title></svelte:head>

<div class="min-h-screen bg-gradient-to-br from-primary-50 to-blue-100 flex items-center justify-center p-4">
  <div class="w-full max-w-md">
    <div class="text-center mb-8">
      <a href="/" class="text-3xl font-bold text-primary-600">LearnLab</a>
      <p class="text-gray-500 mt-2">Sign in to your account</p>
    </div>

    <div class="card">
      <form on:submit|preventDefault={handleLogin} class="space-y-5">
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
        <button type="submit" class="btn-primary w-full" disabled={loading}>
          {loading ? 'Signing in...' : 'Sign In'}
        </button>
      </form>

      <div class="mt-6 text-center text-sm text-gray-500">
        <p>Default admin: <code class="bg-gray-100 px-1 rounded">admin@elearning.local</code> / <code class="bg-gray-100 px-1 rounded">Admin@1234</code></p>
      </div>

      <div class="mt-4 text-center text-sm">
        Don't have an account?
        <a href="/register" class="text-primary-600 font-medium hover:underline">Sign up</a>
      </div>
    </div>
  </div>
</div>
