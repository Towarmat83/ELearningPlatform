<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type Course } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: Course[] = [];
  let loading = true;

  onMount(async () => {
    auth.init();
    try {
      const res = await adminApi.courses($auth.token!);
      courses = res.courses;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  });

  function difficultyColor(d: string) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }
</script>

<svelte:head><title>Courses — Admin</title></svelte:head>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold text-gray-800">Courses</h2>
    <span class="text-sm text-gray-400">
      Courses are managed via files in <code class="bg-gray-100 px-1 rounded">courses/</code>
      or synced from git repos.
    </span>
  </div>

  {#if loading}
    <div class="text-center py-10 text-gray-400">Loading…</div>
  {:else if courses.length === 0}
    <div class="card text-center py-10 text-gray-400">
      <p class="text-lg mb-2">No courses loaded</p>
      <p class="text-sm">Add a course directory under <code>courses/</code> or sync a git repository.</p>
    </div>
  {:else}
    <div class="space-y-3">
      {#each courses as course}
        <div class="card flex items-start gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 mb-1 flex-wrap">
              <span class="font-semibold text-gray-900">{course.title}</span>
              <code class="text-xs text-gray-400 bg-gray-100 px-1 rounded">{course.slug}</code>
              {#if course.difficulty}
                <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
              {/if}
              {#if course.category}
                <span class="badge-blue">{course.category}</span>
              {/if}
              {#if !course.is_published}
                <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">draft</span>
              {/if}
            </div>
            <p class="text-sm text-gray-500 truncate">{course.description}</p>
            <div class="flex gap-4 text-xs text-gray-400 mt-1">
              <span>📖 {course.lesson_count} lesson{course.lesson_count !== 1 ? 's' : ''}</span>
              {#if course.source && course.source !== 'local'}
                <span title={course.source}>⎇ git</span>
              {:else}
                <span>📁 local</span>
              {/if}
            </div>
          </div>
          <a href="/courses/{course.slug}" target="_blank"
            class="btn-secondary text-sm shrink-0">View →</a>
        </div>
      {/each}
    </div>
  {/if}
</div>
