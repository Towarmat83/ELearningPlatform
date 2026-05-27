<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminUser } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let users: AdminUser[] = [];
  let loading = true;
  let search = '';
  let providerFilter = '';
  let providerCounts: { provider: string; count: number }[] = [];

  onMount(() => { if ($auth.token) loadAll(); });

  async function loadAll() {
    loading = true;
    try {
      const [usersRes, provRes] = await Promise.all([
        adminApi.users($auth.token!),
        adminApi.authProviders($auth.token!),
      ]);
      users = usersRes.users;
      providerCounts = provRes.providers;
    } catch (e: any) {
      toasts.error('Failed to load users');
    } finally {
      loading = false;
    }
  }

  async function applyFilter() {
    loading = true;
    try {
      const res = await adminApi.users($auth.token!, providerFilter || undefined);
      users = res.users;
    } catch (e: any) {
      toasts.error('Failed to load users');
    } finally {
      loading = false;
    }
  }

  async function toggleActive(user: AdminUser) {
    try {
      await adminApi.updateUser(user.id, { is_active: !user.is_active }, $auth.token!);
      user.is_active = !user.is_active;
      users = users;
      toasts.success('User updated');
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  async function changeRole(user: AdminUser, role: string) {
    try {
      await adminApi.updateUser(user.id, { role }, $auth.token!);
      user.role = role as 'admin' | 'student';
      users = users;
      toasts.success('Role changed');
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  async function deleteUser(user: AdminUser) {
    if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return;
    try {
      await adminApi.deleteUser(user.id, $auth.token!);
      users = users.filter(u => u.id !== user.id);
      toasts.success('User deleted');
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  // Auth provider badge styles + labels
  const providerLabel: Record<string, string> = {
    local:  'Local',
    oidc:   'OIDC',
    ldap:   'LDAP',
    github: 'GitHub',
    gitlab: 'GitLab',
  };
  const providerBadge: Record<string, string> = {
    local:  'bg-gray-100 text-gray-500',
    oidc:   'bg-blue-100 text-blue-700',
    ldap:   'bg-purple-100 text-purple-700',
    github: 'bg-gray-900 text-white',
    gitlab: 'bg-orange-100 text-orange-700',
  };
  function badgeClass(p: string) {
    return providerBadge[p] ?? 'bg-indigo-100 text-indigo-700';
  }
  function label(p: string) {
    return providerLabel[p] ?? p;
  }

  $: filtered = users.filter(u =>
    !search || u.username.toLowerCase().includes(search.toLowerCase()) ||
               u.email.toLowerCase().includes(search.toLowerCase())
  );

  // Count SSO users for summary
  $: ssoCount = providerCounts
    .filter(p => p.provider !== 'local')
    .reduce((s, p) => s + p.count, 0);
</script>

<svelte:head><title>Users — Admin</title></svelte:head>

<div class="p-8">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-2xl font-bold text-gray-800">User Management</h1>
      {#if !loading}
        <p class="text-sm text-gray-400 mt-0.5">
          {users.length} users
          {#if ssoCount > 0}
            · <span class="text-indigo-500 font-medium">{ssoCount} via SSO</span>
          {/if}
        </p>
      {/if}
    </div>

    <!-- Provider filter pills -->
    {#if providerCounts.length > 1}
      <div class="flex items-center gap-2 flex-wrap">
        <button
          class="text-xs px-3 py-1.5 rounded-full border transition-colors {providerFilter === '' ? 'bg-gray-800 text-white border-gray-800' : 'border-gray-200 text-gray-500 hover:border-gray-400'}"
          on:click={() => { providerFilter = ''; applyFilter(); }}>
          All
        </button>
        {#each providerCounts as p}
          <button
            class="text-xs px-3 py-1.5 rounded-full border transition-colors flex items-center gap-1.5 {providerFilter === p.provider ? 'bg-gray-800 text-white border-gray-800' : 'border-gray-200 text-gray-500 hover:border-gray-400'}"
            on:click={() => { providerFilter = p.provider; applyFilter(); }}>
            <span class="inline-block w-1.5 h-1.5 rounded-full {badgeClass(p.provider)}"></span>
            {label(p.provider)}
            <span class="opacity-60">({p.count})</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <div class="mb-4">
    <input type="text" class="input max-w-xs" placeholder="Search users..."
      bind:value={search} />
  </div>

  {#if loading}
    <div class="text-gray-400">Loading...</div>
  {:else}
    <div class="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b border-gray-100">
          <tr>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">UID</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">User</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Provider</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Role</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Status</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Courses</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Joined</th>
            <th class="text-right px-4 py-3 text-gray-500 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          {#each filtered as user}
            <tr class="hover:bg-gray-50 transition-colors">
              <td class="px-4 py-3">
                <button on:click={() => navigator.clipboard.writeText(user.id)}
                  class="text-gray-400 hover:text-gray-600 text-xs font-mono flex items-center gap-1"
                  title="Click to copy: {user.id}">
                  <span>{user.id.slice(0, 8)}...</span>
                  <svg class="w-3.5 h-3.5 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                  </svg>
                </button>
              </td>
              <td class="px-4 py-3">
                <div class="font-medium text-gray-900">{user.username}</div>
                <div class="text-gray-400 text-xs">{user.email}</div>
              </td>
              <!-- Auth provider badge -->
              <td class="px-4 py-3">
                <span class="text-xs px-2 py-0.5 rounded-full font-medium {badgeClass(user.auth_provider)}">
                  {label(user.auth_provider)}
                </span>
              </td>
              <td class="px-4 py-3">
                <select class="text-xs border rounded px-2 py-1"
                  value={user.role}
                  on:change={(e) => changeRole(user, e.currentTarget.value)}>
                  <option value="student">Student</option>
                  <option value="admin">Admin</option>
                </select>
              </td>
              <td class="px-4 py-3">
                <button on:click={() => toggleActive(user)}
                  class={user.is_active ? 'badge-green' : 'badge-red'}>
                  {user.is_active ? 'Active' : 'Disabled'}
                </button>
              </td>
              <td class="px-4 py-3 text-gray-600">{user.enrolled_courses}</td>
              <td class="px-4 py-3 text-gray-400">
                {new Date(user.created_at).toLocaleDateString()}
              </td>
              <td class="px-4 py-3 text-right">
                <button on:click={() => deleteUser(user)}
                  class="text-xs text-red-500 hover:text-red-700 hover:underline">
                  Delete
                </button>
              </td>
            </tr>
          {/each}
          {#if filtered.length === 0}
            <tr>
              <td colspan="8" class="px-4 py-8 text-center text-gray-400">No users found.</td>
            </tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}
</div>
