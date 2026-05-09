<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminUser } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let users: AdminUser[] = [];
  let loading = true;
  let editingUser: AdminUser | null = null;
  let search = '';

  onMount(() => { if ($auth.token) loadUsers(); });

  async function loadUsers() {
    try {
      const res = await adminApi.users($auth.token!);
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

  $: filtered = users.filter(u =>
    !search || u.username.includes(search) || u.email.includes(search)
  );
</script>

<svelte:head><title>Users — Admin</title></svelte:head>

<div class="p-8">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-2xl font-bold text-gray-800">User Management</h1>
    <span class="text-gray-400 text-sm">{users.length} users</span>
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
            <th class="text-left px-4 py-3 text-gray-500 font-medium">User</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Role</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Status</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Courses</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Labs Done</th>
            <th class="text-left px-4 py-3 text-gray-500 font-medium">Joined</th>
            <th class="text-right px-4 py-3 text-gray-500 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          {#each filtered as user}
            <tr class="hover:bg-gray-50 transition-colors">
              <td class="px-4 py-3">
                <div class="font-medium text-gray-900">{user.username}</div>
                <div class="text-gray-400 text-xs">{user.email}</div>
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
              <td class="px-4 py-3 text-gray-600">{user.completed_labs}</td>
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
        </tbody>
      </table>
    </div>
  {/if}
</div>
