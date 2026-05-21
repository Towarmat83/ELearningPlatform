<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type LeaderboardEntry } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let leaderboard: LeaderboardEntry[] = [];
  let loading = true;

  onMount(async () => {
    if (!$auth.token) { loading = false; return; }
    try {
      const res = await adminApi.leaderboard($auth.token);
      leaderboard = res.leaderboard;
    } catch (e: any) {
      toasts.error('Failed to load leaderboard');
    } finally {
      loading = false;
    }
  });

  function rankBadge(i: number): string {
    if (i === 0) return '🥇';
    if (i === 1) return '🥈';
    if (i === 2) return '🥉';
    return `${i + 1}`;
  }
</script>

<svelte:head><title>Leaderboard — LearnLab Admin</title></svelte:head>

<div class="p-8">
  <h1 class="text-2xl font-bold text-gray-800 mb-2">Leaderboard</h1>
  <p class="text-sm text-gray-500 mb-8">Students ranked by total quiz score (best attempts).</p>

  {#if loading}
    <div class="text-gray-400">Loading...</div>
  {:else if leaderboard.length === 0}
    <div class="text-gray-400 text-center py-12">No quiz submissions yet.</div>
  {:else}
    <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-xs text-gray-500 uppercase tracking-wider border-b border-gray-100 bg-gray-50">
            <th class="px-6 py-3 w-12">Rank</th>
            <th class="px-6 py-3">Student</th>
            <th class="px-6 py-3 text-right">Total Score</th>
            <th class="px-6 py-3 text-right">Modules Passed</th>
            <th class="px-6 py-3 text-right">Courses Enrolled</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          {#each leaderboard as entry, i}
            <tr class="hover:bg-gray-50 transition-colors {i < 3 ? 'font-medium' : ''}">
              <td class="px-6 py-4 text-center text-lg">
                {rankBadge(i)}
              </td>
              <td class="px-6 py-4">
                <div class="flex items-center gap-3">
                  {#if entry.avatar_url}
                    <img src={entry.avatar_url} alt="" class="w-8 h-8 rounded-full object-cover" />
                  {:else}
                    <div class="w-8 h-8 rounded-full bg-primary-100 flex items-center justify-center text-primary-700 font-bold text-xs">
                      {entry.username.slice(0, 2).toUpperCase()}
                    </div>
                  {/if}
                  <div>
                    <div class="font-medium text-gray-900">{entry.username}</div>
                    <div class="text-xs text-gray-400">{entry.email}</div>
                  </div>
                </div>
              </td>
              <td class="px-6 py-4 text-right">
                <span class="text-lg font-bold {i === 0 ? 'text-yellow-500' : i === 1 ? 'text-gray-400' : i === 2 ? 'text-amber-600' : 'text-gray-700'}">
                  {entry.total_score}
                </span>
                <span class="text-xs text-gray-400 ml-1">pts</span>
              </td>
              <td class="px-6 py-4 text-right text-gray-600">{entry.passed_modules}</td>
              <td class="px-6 py-4 text-right text-gray-600">{entry.enrolled_courses}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
