<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { reposApi, type GitRepo } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let repos: GitRepo[] = [];
  let loading = true;
  let adding = false;
  let syncing: Record<string, boolean> = {};

  // Add repo form
  let newUrl = '';
  let newBranch = 'main';
  let newToken = '';
  let showTokenField = false;
  let formError = '';

  function getToken(): string | null {
    return $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
  }

  onMount(async () => {
    auth.init();
    const token = getToken();
    if (!token) { goto('/login?redirect=/dashboard/repos'); return; }
    await loadRepos(token);
  });

  async function loadRepos(token: string) {
    loading = true;
    try {
      const res = await reposApi.list(token);
      repos = res.repos;
    } catch {
      toasts.error('Failed to load repositories');
    } finally {
      loading = false;
    }
  }

  async function addRepo() {
    formError = '';
    const token = getToken();
    if (!token) return;
    if (!newUrl.trim()) { formError = 'URL is required'; return; }
    if (!newUrl.startsWith('http://') && !newUrl.startsWith('https://')) {
      formError = 'Only http/https URLs are supported';
      return;
    }

    adding = true;
    try {
      const repo = await reposApi.add(
        { url: newUrl.trim(), branch: newBranch || 'main', token: newToken || undefined },
        token
      );
      repos = [repo, ...repos];
      newUrl = ''; newBranch = 'main'; newToken = ''; showTokenField = false;
      toasts.success('Repository added');
      // Trigger initial sync
      await triggerSync(repo.id);
    } catch (e: any) {
      formError = e.message || 'Failed to add repository';
    } finally {
      adding = false;
    }
  }

  async function triggerSync(id: string) {
    const token = getToken();
    if (!token) return;
    syncing = { ...syncing, [id]: true };
    try {
      const res = await reposApi.sync(id, token);
      repos = repos.map(r => r.id === id
        ? { ...r, status: 'synced', last_synced_at: res.last_synced_at, error_message: null }
        : r);
      toasts.success('Sync successful');
    } catch (e: any) {
      repos = repos.map(r => r.id === id
        ? { ...r, status: 'error', error_message: e.message }
        : r);
      toasts.error('Sync failed: ' + e.message);
    } finally {
      syncing = { ...syncing, [id]: false };
    }
  }

  async function removeRepo(id: string, url: string) {
    const token = getToken();
    if (!token) return;
    if (!confirm(`Remove ${url} and all its courses?`)) return;
    try {
      await reposApi.remove(id, token);
      repos = repos.filter(r => r.id !== id);
      toasts.success('Repository removed');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to remove repository');
    }
  }

  function statusBadge(status: GitRepo['status']) {
    switch (status) {
      case 'synced':   return 'badge-green';
      case 'syncing':  return 'badge-yellow';
      case 'error':    return 'badge-red';
      default:         return 'badge-blue';
    }
  }

  function statusLabel(status: GitRepo['status']) {
    switch (status) {
      case 'synced':   return '✓ Synced';
      case 'syncing':  return '⟳ Syncing…';
      case 'error':    return '✗ Error';
      default:         return '⏳ Pending';
    }
  }

  function formatDate(d: string | null) {
    if (!d) return 'Never';
    return new Date(d).toLocaleString();
  }

  function hostLabel(url: string) {
    try { return new URL(url).hostname; } catch { return url; }
  }
</script>

<svelte:head><title>Git Repositories — LearnLab</title></svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <div class="mb-8">
    <h1 class="text-2xl font-bold text-gray-900">Git Repositories</h1>
    <p class="text-gray-500 mt-1">
      Connect a git repository containing a <code class="text-sm bg-gray-100 px-1 rounded">courses/</code> folder.
      Works with GitHub, GitLab, Gitea, or any self-hosted instance.
    </p>
  </div>

  <!-- Add repo form -->
  <div class="card mb-8">
    <h2 class="font-semibold text-gray-800 mb-4">Add a repository</h2>

    <div class="space-y-3">
      <div>
        <label class="block text-sm text-gray-600 mb-1">Git URL <span class="text-red-400">*</span></label>
        <input
          type="url"
          class="input w-full"
          placeholder="https://github.com/you/my-courses"
          bind:value={newUrl} />
      </div>

      <div class="flex gap-3">
        <div class="flex-1">
          <label class="block text-sm text-gray-600 mb-1">Branch</label>
          <input type="text" class="input w-full" placeholder="main" bind:value={newBranch} />
        </div>
        <div class="flex-1">
          <label class="block text-sm text-gray-600 mb-1">
            Token
            <span class="text-gray-400 font-normal">(optional, for private repos)</span>
          </label>
          {#if showTokenField}
            <input type="password" class="input w-full" placeholder="Personal Access Token"
              bind:value={newToken} />
          {:else}
            <button type="button" class="btn-secondary text-sm w-full"
              on:click={() => showTokenField = true}>
              + Add token
            </button>
          {/if}
        </div>
      </div>

      {#if formError}
        <p class="text-sm text-red-500">{formError}</p>
      {/if}

      <button class="btn-primary" on:click={addRepo} disabled={adding}>
        {adding ? 'Adding…' : 'Add repository'}
      </button>
    </div>
  </div>

  <!-- Repository list -->
  {#if loading}
    <div class="text-center py-8 text-gray-400">Loading repositories…</div>
  {:else if repos.length === 0}
    <div class="card text-center py-10 text-gray-400">
      <p class="text-3xl mb-3">⎇</p>
      <p class="font-medium mb-1">No repositories yet</p>
      <p class="text-sm">Add your first git repository above to start syncing courses.</p>
    </div>
  {:else}
    <div class="space-y-4">
      {#each repos as repo}
        <div class="card">
          <div class="flex items-start justify-between gap-4 flex-wrap">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1 flex-wrap">
                <span class="font-mono text-sm font-medium text-gray-900 truncate">{repo.url}</span>
                <span class={statusBadge(repo.status)}>{statusLabel(repo.status)}</span>
                {#if repo.has_token}
                  <span class="text-xs text-gray-400">🔒 private</span>
                {/if}
              </div>
              <div class="flex gap-4 text-xs text-gray-400">
                <span>⎇ {repo.branch}</span>
                <span>🌐 {hostLabel(repo.url)}</span>
                <span>Last synced: {formatDate(repo.last_synced_at)}</span>
              </div>
              {#if repo.error_message}
                <div class="mt-2 text-xs text-red-500 bg-red-50 border border-red-100 rounded p-2 font-mono">
                  {repo.error_message}
                </div>
              {/if}
            </div>

            <div class="flex gap-2 shrink-0">
              <button
                class="btn-secondary text-sm"
                disabled={syncing[repo.id]}
                on:click={() => triggerSync(repo.id)}>
                {syncing[repo.id] ? 'Syncing…' : '⟳ Sync'}
              </button>
              <button
                class="text-sm px-3 py-1.5 rounded-lg border border-red-200 text-red-500 hover:bg-red-50 transition-colors"
                on:click={() => removeRepo(repo.id, repo.url)}>
                Remove
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Help -->
  <div class="mt-8 card bg-gray-50 border-gray-200">
    <h3 class="font-semibold text-gray-700 mb-2 text-sm">Expected repository structure</h3>
    <pre class="text-xs text-gray-500 font-mono leading-relaxed">my-courses/
  courses/
    linux-intro/
      course.yaml        ← title, description, category, difficulty
      01-intro.md        ← lesson (order = numeric prefix)
      02-navigation.md
    another-course/
      course.yaml
      01-first-lesson.md</pre>
  </div>
</div>
