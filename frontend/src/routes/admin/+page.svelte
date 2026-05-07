<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminStats } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let stats: AdminStats | null = null;
  let loading = true;

  onMount(async () => {
    if (!$auth.token) { loading = false; return; }
    try {
      stats = await adminApi.stats($auth.token);
    } catch (e: any) {
      toasts.error('Failed to load stats');
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head><title>Admin Dashboard — LearnLab</title></svelte:head>

<div class="p-8">
  <h1 class="text-2xl font-bold text-gray-800 mb-8">Admin Dashboard</h1>

  {#if loading}
    <div class="text-gray-400">Loading stats...</div>
  {:else if stats}
    <div class="grid grid-cols-2 md:grid-cols-3 gap-6 mb-8">
      {#each [
        { label: 'Total Users', value: stats.total_users, icon: '👥', color: 'text-blue-600' },
        { label: 'Total Courses', value: stats.total_courses, icon: '📚', color: 'text-purple-600' },
        { label: 'Enrollments', value: stats.total_enrollments, icon: '🎓', color: 'text-cyan-600' },
      ] as s}
        <div class="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
          <div class="text-3xl mb-2">{s.icon}</div>
          <div class="text-3xl font-bold {s.color}">{s.value}</div>
          <div class="text-sm text-gray-400 mt-1">{s.label}</div>
        </div>
      {/each}
    </div>

    <div class="grid md:grid-cols-2 gap-6">
      <div class="bg-white rounded-xl border border-gray-100 p-6 shadow-sm">
        <h2 class="font-semibold text-gray-700 mb-4">Quick Actions</h2>
        <div class="space-y-2">
          <a href="/admin/courses" class="block p-3 bg-gray-50 rounded-lg hover:bg-gray-100 text-sm">
            📚 Manage Courses →
          </a>
          <a href="/admin/repos" class="block p-3 bg-gray-50 rounded-lg hover:bg-gray-100 text-sm">
            ⎇ Git Repositories →
          </a>
          <a href="/admin/users" class="block p-3 bg-gray-50 rounded-lg hover:bg-gray-100 text-sm">
            👥 Manage Users →
          </a>
          <a href="/admin/monitoring" class="block p-3 bg-gray-50 rounded-lg hover:bg-gray-100 text-sm">
            📈 View Monitoring →
          </a>
        </div>
      </div>
      <div class="bg-white rounded-xl border border-gray-100 p-6 shadow-sm">
        <h2 class="font-semibold text-gray-700 mb-4">Platform Info</h2>
        <div class="space-y-3 text-sm text-gray-600">
          <p>📊 Metrics: <a href="/metrics" target="_blank" class="text-primary-600 hover:underline">/metrics</a> (Prometheus)</p>
          <p>❤️ Health: <a href="/health" target="_blank" class="text-primary-600 hover:underline">/health</a></p>
          <p>🔌 API: <a href="/api" target="_blank" class="text-primary-600 hover:underline">REST API</a></p>
        </div>
      </div>
    </div>
  {/if}
</div>
