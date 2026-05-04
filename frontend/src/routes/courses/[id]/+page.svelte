<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { coursesApi, lessonsApi, type Course, type LessonSummary } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let course: Course | null = null;
  let lessons: LessonSummary[] = [];
  let loading = true;
  let enrolled = false;

  const slug = $page.params.id;

  function getToken(): string | null {
    return $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
  }

  onMount(async () => {
    auth.init();
    try {
      course = await coursesApi.get(slug);
    } catch {
      toasts.error('Course not found');
      loading = false;
      return;
    }

    const token = getToken();
    if (token) {
      try {
        const res = await lessonsApi.list(slug, token);
        lessons = res.lessons;
        enrolled = true;
      } catch (e: any) {
        if (e.status !== 403) toasts.error('Failed to load lessons: ' + e.message);
        enrolled = false;
      }
    }
    loading = false;
  });

  async function enroll() {
    const token = getToken();
    if (!token) {
      goto('/login?redirect=/courses/' + slug);
      return;
    }
    try {
      await coursesApi.enroll(slug, token);
      toasts.success('Enrolled successfully!');
      const res = await lessonsApi.list(slug, token);
      lessons = res.lessons;
      enrolled = true;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to enroll');
    }
  }

  function difficultyColor(d: string) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    return 'badge-red';
  }

  $: viewedCount = lessons.filter(l => l.viewed).length;
  $: progress = lessons.length > 0 ? Math.round((viewedCount / lessons.length) * 100) : 0;
</script>

<svelte:head>
  <title>{course?.title ?? 'Course'} — LearnLab</title>
</svelte:head>

<div class="max-w-5xl mx-auto px-6 py-8">
  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading...</div>
  {:else if !course}
    <div class="text-center py-16 text-gray-400">Course not found.</div>
  {:else}
    <!-- Header -->
    <div class="card mb-6">
      <div class="flex flex-col md:flex-row gap-6">
        <div class="w-full md:w-56 h-40 bg-gradient-to-br from-primary-100 to-blue-200 rounded-xl flex items-center justify-center text-6xl shrink-0">
          📚
        </div>
        <div class="flex-1">
          <div class="flex items-start gap-3 mb-2 flex-wrap">
            <h1 class="text-2xl font-bold text-gray-900">{course.title}</h1>
            {#if course.difficulty}
              <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
            {/if}
            {#if course.category}
              <span class="badge-blue">{course.category}</span>
            {/if}
          </div>
          <p class="text-gray-600 mb-4">{course.description}</p>
          <div class="flex gap-6 text-sm text-gray-400 mb-4">
            <span>📖 {course.lesson_count} lesson{course.lesson_count !== 1 ? 's' : ''}</span>
            {#if course.source && course.source !== 'local'}
              <span title={course.source} class="text-gray-300">⎇ synced from git</span>
            {/if}
          </div>

          {#if !enrolled}
            <button on:click={enroll} class="btn-primary">Enroll Now</button>
          {:else if lessons.length > 0}
            <div>
              <div class="flex justify-between text-sm text-gray-500 mb-1">
                <span>{viewedCount}/{lessons.length} lessons completed</span>
                <span>{progress}%</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-3">
                <div class="bg-primary-500 h-3 rounded-full transition-all" style="width: {progress}%"></div>
              </div>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Lesson list -->
    {#if enrolled}
      {#if lessons.length === 0}
        <div class="card text-center py-8 text-gray-400">No lessons available yet.</div>
      {:else}
        <div class="space-y-2">
          {#each lessons as ls}
            <a href="/courses/{slug}/lessons/{ls.slug}"
              class="card flex items-center gap-4 hover:shadow-md transition-shadow cursor-pointer group p-4">
              <div class="w-10 h-10 rounded-full flex items-center justify-center shrink-0
                {ls.viewed ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-500'}">
                {#if ls.viewed}✓{:else}{ls.order}{/if}
              </div>
              <div class="flex-1 min-w-0">
                <span class="font-medium text-gray-900 group-hover:text-primary-600 transition-colors">
                  {ls.title}
                </span>
              </div>
              {#if ls.viewed}
                <span class="text-xs text-green-600 font-medium shrink-0">Completed</span>
              {/if}
              <svg class="w-5 h-5 text-gray-300 group-hover:text-primary-400 transition-colors shrink-0"
                fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
              </svg>
            </a>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="card text-center py-8 text-gray-400">
        Enroll in this course to access the lessons.
      </div>
    {/if}
  {/if}
</div>
