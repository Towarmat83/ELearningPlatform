<script lang="ts">
  import { onMount } from 'svelte';
  import { groupsApi, type Group } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let groups: Group[] = [];
  let loading = true;
  let newGroupName = '';
  let creating = false;

  // Per-row role edits and save state
  let roleEdits: Record<string, string> = {};
  let rowState: Record<string, 'idle' | 'saving' | 'saved' | 'error'> = {};

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
        rowState[g.name] = 'idle';
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

  async function saveMapping(g: Group) {
    const desired = roleEdits[g.name] ?? '';
    if (desired === g.mapped_role) return;

    rowState[g.name] = 'saving';
    rowState = { ...rowState };

    try {
      if (desired === '') {
        await groupsApi.deleteMapping(g.name, $auth.token!);
      } else {
        await groupsApi.upsertMapping({ group_name: g.name, platform_role: desired }, $auth.token!);
      }
      g.mapped_role = desired;
      groups = [...groups];
      rowState[g.name] = 'saved';
      rowState = { ...rowState };
      setTimeout(() => {
        rowState[g.name] = 'idle';
        rowState = { ...rowState };
      }, 2000);
    } catch (e: any) {
      rowState[g.name] = 'error';
      rowState = { ...rowState };
      roleEdits[g.name] = g.mapped_role;
      roleEdits = { ...roleEdits };
      toasts.error(e.message || 'Failed to save mapping');
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

  const roleBadge: Record<string, string> = {
    admin:   'bg-orange-100 text-orange-700',
    student: 'bg-green-100 text-green-700',
  };
</script>

<svelte:head><title>Groups — Admin</title></svelte:head>

<div class="p-8 max-w-4xl space-y-6">

  <div>
    <h1 class="text-2xl font-bold text-gray-900">Groups</h1>
    <p class="text-sm text-gray-500 mt-1">
      Groups are synced from OIDC / LDAP at each login. Role mappings are saved automatically when changed.
    </p>
  </div>

  <!-- Create group -->
  <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-4 flex items-center gap-3">
    <svg class="w-4 h-4 text-gray-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/>
    </svg>
    <input
      type="text"
      class="input flex-1 text-sm"
      placeholder="New local group name…"
      bind:value={newGroupName}
      on:keydown={(e) => e.key === 'Enter' && createGroup()}
      maxlength="64"
    />
    <button
      class="btn-primary text-sm whitespace-nowrap"
      on:click={createGroup}
      disabled={creating || !newGroupName.trim()}>
      {creating ? 'Creating…' : 'Create group'}
    </button>
  </div>

  <!-- Groups table -->
  {#if loading}
    <div class="text-center py-16 text-gray-400 text-sm">Loading…</div>
  {:else if groups.length === 0}
    <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-12 text-center">
      <p class="text-gray-600 font-medium">No groups yet</p>
      <p class="text-sm text-gray-400 mt-1">
        Create a local group above, or groups will appear automatically after users log in via OIDC or LDAP.
      </p>
    </div>
  {:else}
    <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b border-gray-100">
          <tr>
            <th class="text-left px-5 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Group</th>
            <th class="text-left px-5 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Members</th>
            <th class="text-left px-5 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wide">Platform role</th>
            <th class="px-5 py-3 w-10"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          {#each groups as g (g.id)}
            <tr class="hover:bg-gray-50 transition-colors group">

              <!-- Name + source -->
              <td class="px-5 py-3">
                <div class="flex items-center gap-2">
                  <span class="font-medium text-gray-900">{g.name}</span>
                  <span class="text-xs px-1.5 py-0.5 rounded-full font-medium {sourceBadge[g.source] ?? 'bg-gray-100 text-gray-600'}">
                    {sourceLabel[g.source] ?? g.source}
                  </span>
                </div>
              </td>

              <!-- Members -->
              <td class="px-5 py-3">
                <span class="text-gray-500">{g.member_count}</span>
                <span class="text-gray-300 text-xs ml-1">member{g.member_count !== 1 ? 's' : ''}</span>
              </td>

              <!-- Role select — auto-saves on change -->
              <td class="px-5 py-3">
                <div class="flex items-center gap-2">
                  <select
                    class="input py-1 text-sm w-36"
                    bind:value={roleEdits[g.name]}
                    on:change={() => saveMapping(g)}
                    disabled={rowState[g.name] === 'saving'}>
                    <option value="">— No mapping —</option>
                    <option value="student">Student</option>
                    <option value="admin">Admin</option>
                  </select>

                  <!-- Per-row save state indicator -->
                  {#if rowState[g.name] === 'saving'}
                    <svg class="w-4 h-4 text-gray-400 animate-spin shrink-0" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"/>
                    </svg>
                  {:else if rowState[g.name] === 'saved'}
                    <svg class="w-4 h-4 text-green-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5" aria-label="Saved">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
                    </svg>
                  {:else if rowState[g.name] === 'error'}
                    <svg class="w-4 h-4 text-red-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-label="Error">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                  {/if}
                </div>
              </td>

              <!-- Delete (local groups only, except 'everyone') -->
              <td class="px-5 py-3 text-right">
                {#if g.source === 'local' && g.name !== 'everyone'}
                  <button
                    class="text-xs text-gray-300 hover:text-red-500 transition-colors opacity-0 group-hover:opacity-100"
                    on:click={() => deleteGroup(g)}
                    title="Delete group">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                    </svg>
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
