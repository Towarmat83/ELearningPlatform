<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type CourseMonitoring, type CourseWithStats } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: CourseWithStats[] = [];
  let selectedCourse: string = '';
  let monitoring: CourseMonitoring | null = null;
  let loading = false;

  onMount(async () => {
    if (!$auth.token) return;
    try {
      const res = await adminApi.courses($auth.token);
      courses = res.courses;
    } catch { toasts.error('Failed to load courses'); }
  });

  async function loadMonitoring() {
    if (!selectedCourse || !$auth.token) return;
    loading = true;
    monitoring = null;
    try {
      monitoring = await adminApi.courseMonitoring(selectedCourse, $auth.token);
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load monitoring');
    } finally {
      loading = false;
    }
  }

  // Sorted copy — never mutate the reactive object directly
  $: sortedStudents = monitoring
    ? [...monitoring.student_progress].sort((a, b) => Number(b.total_points) - Number(a.total_points))
    : [];

  $: totalLabs = monitoring
    ? sortedStudents.length > 0
      ? Math.max(...sortedStudents.map(s => Number(s.completed_labs)), 1)
      : 1
    : 1;

  $: avgScore = sortedStudents.length > 0
    ? Math.round(sortedStudents.reduce((s, p) => s + Number(p.total_points), 0) / sortedStudents.length)
    : 0;
</script>

<svelte:head><title>Monitoring — Admin</title></svelte:head>

<div class="p-8">
  <h1 class="text-2xl font-bold text-gray-800 mb-6">Student Monitoring</h1>

  <!-- Course selector -->
  <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-4 mb-6 flex gap-3">
    <select class="input flex-1 max-w-sm"
      bind:value={selectedCourse}
      on:change={loadMonitoring}>
      <option value="">Select a course...</option>
      {#each courses as course}
        <option value={course.id}>{course.title}</option>
      {/each}
    </select>
  </div>

  {#if loading}
    <div class="text-gray-400">Loading monitoring data...</div>
  {:else if monitoring}
    <!-- Summary cards -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
      {#each [
        { label: 'Enrolled', value: monitoring.total_enrolled, icon: '👥' },
        { label: 'Avg Score', value: avgScore + ' pts', icon: '⭐' },
        { label: 'Course', value: monitoring.course_title, icon: '📚' },
        { label: 'Students', value: sortedStudents.length, icon: '🎓' },
      ] as stat}
        <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-4 text-center">
          <div class="text-2xl mb-1">{stat.icon}</div>
          <div class="font-bold text-gray-800">{stat.value}</div>
          <div class="text-xs text-gray-400">{stat.label}</div>
        </div>
      {/each}
    </div>

    <!-- Student table -->
    {#if sortedStudents.length === 0}
      <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-8 text-center text-gray-400">
        No students enrolled yet.
      </div>
    {:else}
      <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        <div class="px-4 py-3 border-b border-gray-100 bg-gray-50">
          <h2 class="font-semibold text-gray-700 text-sm">Student Progress — {monitoring.course_title}</h2>
        </div>
        <table class="w-full text-sm">
          <thead class="bg-gray-50 border-b border-gray-100">
            <tr>
              <th class="text-left px-4 py-3 text-gray-500 font-medium">Student</th>
              <th class="text-left px-4 py-3 text-gray-500 font-medium">Labs Done</th>
              <th class="text-left px-4 py-3 text-gray-500 font-medium">Total Score</th>
              <th class="text-left px-4 py-3 text-gray-500 font-medium">Last Activity</th>
              <th class="text-left px-4 py-3 text-gray-500 font-medium">Progress</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-50">
            {#each sortedStudents as student, i}
              {@const pct = Math.min(100, Number(student.completed_labs) > 0
                ? Math.round(Number(student.completed_labs) / totalLabs * 100)
                : 0)}
              <tr class="hover:bg-gray-50">
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    {#if i < 3}
                      <span class="text-lg">{['🥇','🥈','🥉'][i]}</span>
                    {/if}
                    <div>
                      <div class="font-medium text-gray-900">{student.username}</div>
                      <div class="text-gray-400 text-xs">{student.email}</div>
                    </div>
                  </div>
                </td>
                <td class="px-4 py-3 text-gray-700">{student.completed_labs}</td>
                <td class="px-4 py-3">
                  <span class="font-semibold text-primary-600">⭐ {student.total_points}</span>
                </td>
                <td class="px-4 py-3 text-gray-400">
                  {student.last_activity
                    ? new Date(student.last_activity).toLocaleString()
                    : 'Never'}
                </td>
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    <div class="w-24 bg-gray-100 rounded-full h-1.5">
                      <div class="bg-primary-500 h-1.5 rounded-full transition-all" style="width: {pct}%"></div>
                    </div>
                    <span class="text-xs text-gray-400">{pct}%</span>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {:else if selectedCourse}
    <div class="text-center py-8 text-gray-400">Chargement...</div>
  {:else}
    <div class="text-center py-8 text-gray-400">
      <div class="text-4xl mb-3">📈</div>
      <p>Select a course to view student progress.</p>
    </div>
  {/if}
</div>
