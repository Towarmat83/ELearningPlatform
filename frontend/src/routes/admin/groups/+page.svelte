<script lang="ts">
  import { onMount } from 'svelte';
  import { groupsApi, type Group } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let groups: Group[] = [];
  let loading = true;
  let saving = false;

  // Local state: group_name → chosen role ('admin' | 'student' | '')
  let roleEdits: Record<string, string> = {};

  // Create-group form
  let newGroupName = '';
  let creating = false;

  onMount(async () => {
    await loadGroups();
  });

  async function loadGroups() {
    loading = true;
    try {
      const res = await groupsApi.list($auth.token!);
      groups = res.groups;
      for (const g of groups) {
        roleEdits[g.name] = g.mapped_role;
      }
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load groups');
    } finally {
      loading = false;
    }
  }

  async function createGroup() {
    const name = newGroupName.trim();
    if (!name) return;
    creating = true;
    try {
      await groupsApi.create(name, $auth.token!);
      newGroupName = '';
      toasts.success(`Group "${name}" created`);
      await loadGroups();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to create group');
    } finally {
      creating = false;
    }
  }

  async function deleteGroup(g: Group) {
    if (!confirm(`Delete group "${g.name}"? This cannot be undone.`)) return;
    try {
      await groupsApi.delete(g.id, $auth.token!);
      toasts.success(`Group "${g.name}" deleted`);
      await loadGroups();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to delete group');
    }
  }

  async function saveAll() {
    saving = true;
    const token = $auth.token!;
    try {
      for (const g of groups) {
        const desired = roleEdits[g.name] ?? '';
        const current = g.mapped_role;
        if (desired === current) continue;

        if (desired === '') {
          await groupsApi.deleteMapping(g.name, token);
        } else {
          await groupsApi.upsertMapping({ group_name: g.name, platform_role: desired }, token);
        }
      }
      await loadGroups();
      toasts.success('Group mappings saved');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to save mappings');
    } finally {
      saving = false;
    }
  }

  const sourceLabel: Record<string, string> = {
    oidc:  'OIDC',
    ldap:  'LDAP',
    local: 'Local',
  };

  const sourceBadge: Record<string, string> = {
    oidc:  'bg-blue-100 text-blue-700',
    ldap:  'bg-purple-100 text-purple-700',
    local: 'bg-gray-100 text-gray-600',
  };
</script>

<svelte:head><title>Groups — Admin</title></svelte:head>

<div class="p-8 max-w-4xl">
  <div class="flex items-start justify-between mb-1">
    <div>
      <h1 class="text-2xl font-bold text-gray-900">Groups</h1>
      <p class="text-gray-500 text-sm mt-1">
        Groups are synced from OIDC / LDAP at each login. You can also create local groups manually.
      </p>
    </div>
    {#if !loading && groups.length > 0}
      <button class="btn-primary px-6" on:click={saveAll} disabled={saving}>
        {saving ? 'Saving…' : 'Save Mappings'}
      </button>
    {/if}
  </div>

  <!-- Create group form -->
  <div class="card mt-6 flex items-center gap-3">
    <svg class="w-5 h-5 text-gray-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/>
    </svg>
    <input
      type="text"
      class="input flex-1"
      placeholder="New group name…"
      bind:value={newGroupName}
      on:keydown={(e) => e.key === 'Enter' && createGroup()}
      maxlength="64"
    />
    <button
      class="btn-primary px-5 whitespace-nowrap"
      on:click={createGroup}
      disabled={creating || !newGroupName.trim()}
    >
      {creating ? 'Creating…' : 'Create group'}
    </button>
  </div>

  {#if loading}
    <div class="text-gray-400 py-16 text-center">Loading…</div>
  {:else if groups.length === 0}
    <div class="card mt-6 text-center py-16">
      <p class="text-4xl mb-3">🏷️</p>
      <p class="text-gray-600 font-medium">No groups yet.</p>
      <p class="text-sm text-gray-400 mt-1">
        Create one manually above, or groups will appear automatically after users log in via OIDC or LDAP.
      </p>
    </div>
  {:else}
    <div class="card mt-6 overflow-hidden p-0">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b border-gray-100">
          <tr>
            <th class="text-left px-4 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Group</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Members</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Platform role</th>
            <th class="px-4 py-3"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          {#each groups as g}
            <tr class="hover:bg-gray-50 transition-colors">
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span class="font-medium text-gray-900">{g.name}</span>
                  <span class="text-xs px-1.5 py-0.5 rounded-full font-medium {sourceBadge[g.source] ?? 'bg-gray-100 text-gray-600'}">
                    {sourceLabel[g.source] ?? g.source}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3 text-gray-500">{g.member_count}</td>
              <td class="px-4 py-3">
                <select
                  class="input py-1.5 text-sm w-36"
                  bind:value={roleEdits[g.name]}
                >
                  <option value="">— No mapping —</option>
                  <option value="student">Student</option>
                  <option value="admin">Admin</option>
                </select>
              </td>
              <td class="px-4 py-3 text-right">
                {#if g.source === 'local' && g.name !== 'everyone'}
                  <button
                    class="text-xs text-red-400 hover:text-red-600 hover:underline"
                    on:click={() => deleteGroup(g)}
                  >
                    Delete
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex justify-end mt-4">
      <button class="btn-primary px-8" on:click={saveAll} disabled={saving}>
        {saving ? 'Saving…' : 'Save Mappings'}
      </button>
    </div>
  {/if}
</div>
