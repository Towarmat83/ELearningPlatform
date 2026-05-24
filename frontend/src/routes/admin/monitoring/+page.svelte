<script lang="ts">
  import { adminApi, type Course } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import { onMount } from 'svelte';

  let courses: Course[] = [];
  let loading = true;

  onMount(async () => {
    if (!$auth.token) return;
    try {
      const res = await adminApi.adminCourses($auth.token);
      courses = res.courses;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head><title>Monitoring — Admin</title></svelte:head>

<div class="space-y-6">
  <h2 class="text-xl font-semibold text-gray-800">Monitoring</h2>

  {#if loading}
    <div class="text-center py-10 text-gray-400">Loading…</div>
  {:else}
    <div class="grid md:grid-cols-3 gap-4">
      <div class="card text-center">
        <div class="text-2xl mb-1">📚</div>
        <div class="text-2xl font-bold text-blue-600">{courses.length}</div>
        <div class="text-xs text-gray-500 mt-1">Total Courses</div>
      </div>
      <div class="card text-center">
        <div class="text-2xl mb-1">✅</div>
        <div class="text-2xl font-bold text-green-600">{courses.filter(c => c.is_public).length}</div>
        <div class="text-xs text-gray-500 mt-1">Public</div>
      </div>
      <div class="card text-center">
        <div class="text-2xl mb-1">📝</div>
        <div class="text-2xl font-bold text-purple-600">{courses.reduce((s, c) => s + c.lab_count, 0)}</div>
        <div class="text-xs text-gray-500 mt-1">Total Labs</div>
      </div>
    </div>

    <div class="card">
      <h3 class="font-semibold text-gray-700 mb-4">Courses overview</h3>
      {#if courses.length === 0}
        <p class="text-sm text-gray-400 text-center py-4">No courses loaded.</p>
      {:else}
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-gray-100">
              <th class="text-left px-3 py-2 text-gray-500 font-medium">Course</th>
              <th class="text-left px-3 py-2 text-gray-500 font-medium">Labs</th>
              <th class="text-left px-3 py-2 text-gray-500 font-medium">Enrolled</th>
              <th class="text-left px-3 py-2 text-gray-500 font-medium">Status</th>
            </tr>
          </thead>
          <tbody>
            {#each courses as course}
              <tr class="border-b border-gray-50 hover:bg-gray-50">
                <td class="px-3 py-2 font-medium text-gray-800">{course.title}</td>
                <td class="px-3 py-2 text-gray-600">{course.lab_count}</td>
                <td class="px-3 py-2 text-gray-600">{course.enrollment_count}</td>
                <td class="px-3 py-2">
                  {#if course.is_public}
                    <span class="badge-green">public</span>
                  {:else}
                    <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">private</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}
</div>
