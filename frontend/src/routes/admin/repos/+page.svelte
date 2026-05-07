<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type GitRepo } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let repos: GitRepo[] = [];
  let loading = true;
  let adding = false;
  let syncing: Record<string, boolean> = {};

  let newUrl = '';
  let newBranch = 'main';
  let newToken = '';
  let showTokenField = false;
  let formError = '';

  onMount(async () => {
    auth.init();
    await loadRepos();
  });

  async function loadRepos() {
    loading = true;
    try {
      const res = await adminApi.repos.list($auth.token!);
      repos = res.repos;
    } catch {
      toasts.error('Failed to load repositories');
    } finally {
      loading = false;
    }
  }

  async function addRepo() {
    formError = '';
    if (!newUrl.trim()) { formError = 'URL is required'; return; }
    if (!newUrl.startsWith('http://') && !newUrl.startsWith('https://')) {
      formError = 'Only http/https URLs are supported';
      return;
    }
    adding = true;
    try {
      const repo = await adminApi.repos.add(
        { url: newUrl.trim(), branch: newBranch || 'main', token: newToken || undefined },
        $auth.token!
      );
      repos = [repo, ...repos];
      newUrl = ''; newBranch = 'main'; newToken = ''; showTokenField = false;
      toasts.success('Repository added');
      await triggerSync(repo.id);
    } catch (e: any) {
      formError = e.message || 'Failed to add repository';
    } finally {
      adding = false;
    }
  }

  async function triggerSync(id: string) {
    syncing = { ...syncing, [id]: true };
    try {
      const res = await adminApi.repos.sync(id, $auth.token!);
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
    if (!confirm(`Remove ${url} and all its courses?`)) return;
    try {
      await adminApi.repos.remove(id, $auth.token!);
      repos = repos.filter(r => r.id !== id);
      toasts.success('Repository removed');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to remove repository');
    }
  }

  function statusBadge(status: GitRepo['status']) {
    if (status === 'synced') return 'badge-green';
    if (status === 'syncing') return 'badge-yellow';
    if (status === 'error') return 'badge-red';
    return 'badge-blue';
  }

  function statusLabel(status: GitRepo['status']) {
    if (status === 'synced') return '✓ Synced';
    if (status === 'syncing') return '⟳ Syncing…';
    if (status === 'error') return '✗ Error';
    return '⏳ Pending';
  }

  function formatDate(d: string | null) {
    if (!d) return 'Never';
    return new Date(d).toLocaleString();
  }

  function hostLabel(url: string) {
    try { return new URL(url).hostname; } catch { return url; }
  }
</script>

<svelte:head><title>Git Repos — Admin</title></svelte:head>

<div class="space-y-6">
  <div>
    <h2 class="text-xl font-semibold text-gray-800">Git Repositories</h2>
    <p class="text-sm text-gray-500 mt-1">
      Connect repositories containing a <code class="bg-gray-100 px-1 rounded">courses/</code> folder.
      Synced courses are unpublished by default — enable them in the Courses tab.
    </p>
  </div>

  <!-- Add repo form -->
  <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
    <h3 class="font-semibold text-gray-700 mb-4">Add a repository</h3>
    <div class="space-y-3">
      <div>
        <label class="block text-sm text-gray-600 mb-1">Git URL <span class="text-red-400">*</span></label>
        <input type="url" class="input w-full"
          placeholder="https://github.com/you/my-courses"
          bind:value={newUrl} />
        <p class="text-xs text-gray-400 mt-1">
          URL racine du repo — le serveur normalise automatiquement les URLs GitHub copiées-collées.
        </p>
      </div>
      <div class="flex gap-3">
        <div class="flex-1">
          <label class="block text-sm text-gray-600 mb-1">Branch</label>
          <input type="text" class="input w-full" placeholder="main" bind:value={newBranch} />
        </div>
        <div class="flex-1">
          <label class="block text-sm text-gray-600 mb-1">
            Token <span class="text-gray-400 font-normal">(optional, private repos)</span>
          </label>
          {#if showTokenField}
            <input type="password" class="input w-full" placeholder="Personal Access Token"
              bind:value={newToken} />
          {:else}
            <button type="button" class="btn-secondary text-sm w-full"
              on:click={() => showTokenField = true}>+ Add token</button>
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

  <!-- Repo list -->
  {#if loading}
    <div class="text-center py-8 text-gray-400">Loading…</div>
  {:else if repos.length === 0}
    <div class="bg-white rounded-xl border border-gray-100 p-10 text-center text-gray-400">
      <p class="text-3xl mb-3">⎇</p>
      <p class="font-medium">No repositories yet</p>
      <p class="text-sm mt-1">Add a git repository above to start syncing courses.</p>
    </div>
  {:else}
    <div class="space-y-3">
      {#each repos as repo}
        <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
          <div class="flex items-start gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class={statusBadge(repo.status)}>{statusLabel(repo.status)}</span>
                <span class="badge-blue text-xs">⎇ {repo.branch}</span>
                <span class="text-xs text-gray-400">🌐 {hostLabel(repo.url)}</span>
                {#if repo.has_token}
                  <span class="text-xs text-gray-400">🔑 token</span>
                {/if}
              </div>
              <p class="text-sm text-gray-600 truncate" title={repo.url}>{repo.url}</p>
              <p class="text-xs text-gray-400 mt-1">Last synced: {formatDate(repo.last_synced_at)}</p>
              {#if repo.error_message}
                <p class="text-xs text-red-500 mt-1 font-mono bg-red-50 p-2 rounded">{repo.error_message}</p>
              {/if}
            </div>
            <div class="flex gap-2 shrink-0">
              <button
                class="btn-secondary text-sm"
                on:click={() => triggerSync(repo.id)}
                disabled={syncing[repo.id]}>
                {syncing[repo.id] ? '⟳ Syncing…' : '⟳ Sync'}
              </button>
              <button class="btn-secondary text-sm text-red-600 hover:bg-red-50"
                on:click={() => removeRepo(repo.id, repo.url)}>
                Remove
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Expected structure -->
  <div class="bg-gray-50 rounded-xl border border-gray-200 p-4 text-sm text-gray-600">
    <p class="font-medium mb-2">Expected repository structure</p>
    <pre class="text-xs font-mono text-gray-500 leading-relaxed">your-repo/
└── courses/
    └── my-course/
        ├── course.yaml       # title, description, category, difficulty
        ├── 01-intro.md       # ---\ntitle: "..."\n---\ncontent
        └── 02-next.md</pre>
  </div>
</div>
