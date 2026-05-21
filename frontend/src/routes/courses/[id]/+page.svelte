<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { coursesApi, modulesApi, type Course, type ModuleSummary } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let course: Course | null = null;
  let modules: ModuleSummary[] = [];
  let loading = true;
  let enrolled = false;

  const courseId = $page.params.id as string;

  function getToken(): string | null {
    return $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
  }

  async function checkEnrollment(token: string): Promise<boolean> {
    try {
      const res: { courses: { slug: string }[] } = await coursesApi.myCourses(token) as any;
      return res.courses?.some((c) => c.slug === courseId) ?? false;
    } catch {
      return false;
    }
  }

  onMount(async () => {
    auth.init();
    try {
      course = await coursesApi.get(courseId);
    } catch {
      toasts.error('Course not found');
      loading = false;
      return;
    }

    const token = getToken();
    if (token) {
      enrolled = await checkEnrollment(token);
      if (enrolled) {
        try {
          const res = await modulesApi.list(courseId, token);
          modules = res.modules;
        } catch (e: any) {
          if (e.status !== 403) toasts.error('Failed to load modules: ' + e.message);
        }
      }
    }
    loading = false;
  });

  async function enroll() {
    const token = getToken();
    if (!token) {
      goto('/login?redirect=/courses/' + courseId);
      return;
    }
    try {
      await coursesApi.enroll(courseId, token);
      toasts.success('Enrolled successfully!');
      const res = await modulesApi.list(courseId, token);
      modules = res.modules;
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

  function moduleIcon(m: ModuleSummary): string {
    if (m.locked) return '🔒';
    if (m.type === 'video') return '🎬';
    if (m.type === 'image') return '🖼️';
    if (m.type === 'quiz') return '🧩';
    return '📝';
  }

  function moduleHref(m: ModuleSummary): string {
    if (m.locked) return '#';
    if (m.type === 'quiz') return `/courses/${courseId}/quiz/${m.index}`;
    return `/courses/${courseId}/labs/${m.index}`;
  }

  function moduleStatus(m: ModuleSummary): { label: string; cls: string } | null {
    if (m.locked) return { label: 'Locked', cls: 'text-gray-400 bg-gray-100' };
    if (m.type === 'quiz') {
      if (m.passed) return { label: `✓ ${m.best_score}/${m.max_score} pts`, cls: 'text-green-700 bg-green-50' };
      if (m.attempts > 0) return { label: `${m.attempts} attempt${m.attempts > 1 ? 's' : ''}`, cls: 'text-yellow-700 bg-yellow-50' };
    } else {
      if (m.viewed) return { label: 'Viewed', cls: 'text-blue-700 bg-blue-50' };
    }
    return null;
  }

  $: completedCount = modules.filter(m => m.type === 'quiz' ? m.passed : m.viewed).length;
  $: totalCount = modules.length;
  $: pct = totalCount > 0 ? Math.round((completedCount / totalCount) * 100) : 0;
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
            <span>📦 {modules.length || course.lab_count} module{(modules.length || course.lab_count) !== 1 ? 's' : ''}</span>
            <span>👥 {course.enrollment_count} enrolled</span>
            {#if course.creator_username}
              <span>👤 {course.creator_username}</span>
            {/if}
          </div>

          {#if course.prerequisites && course.prerequisites.length > 0 && !enrolled}
            <div class="mb-3 flex flex-col gap-1 text-sm text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2">
              {#each course.prerequisites as p}
                <div class="flex items-start gap-1.5">
                  <span>🔒</span>
                  <span>
                    <span class="font-medium">{p.course}</span>
                    {#if p.min_score} — {p.min_score}+ points required{/if}
                    {#if p.modules && p.modules.length > 0} — must pass: {p.modules.join(', ')}{/if}
                  </span>
                </div>
              {/each}
            </div>
          {/if}

          {#if !enrolled}
            <button on:click={enroll} class="btn-primary">Enroll Now</button>
          {:else if totalCount > 0}
            <div>
              <div class="flex justify-between text-sm text-gray-500 mb-1">
                <span>{completedCount}/{totalCount} modules completed</span>
                <span>{pct}%</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-3">
                <div class="bg-primary-500 h-3 rounded-full transition-all" style="width: {pct}%"></div>
              </div>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Module list -->
    {#if enrolled}
      {#if modules.length === 0}
        <div class="card text-center py-8 text-gray-400">No content available yet.</div>
      {:else}
        <div class="space-y-2">
          {#each modules as m}
            {@const status = moduleStatus(m)}
            <a
              href={moduleHref(m)}
              class="card flex items-center gap-4 p-4 transition-shadow group
                {m.locked ? 'opacity-60 cursor-not-allowed' : 'hover:shadow-md cursor-pointer'}
                {m.hidden ? 'opacity-50' : ''}"
              on:click|preventDefault={m.locked ? undefined : () => goto(moduleHref(m))}
            >
              <div class="w-10 h-10 rounded-full flex items-center justify-center shrink-0 text-lg
                {m.locked ? 'bg-gray-200' : m.type === 'quiz' ? 'bg-amber-100' : 'bg-gray-100'}">
                {moduleIcon(m)}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-medium {m.locked ? 'text-gray-400' : 'text-gray-900'} {m.locked ? '' : 'group-hover:text-primary-600'} transition-colors">
                    {m.name}
                  </span>
                  <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">{m.type}</span>
                  {#if m.hidden}
                    <span class="text-xs text-yellow-600 bg-yellow-50 border border-yellow-200 px-2 py-0.5 rounded-full">hidden</span>
                  {/if}
                  {#if status}
                    <span class="text-xs px-2 py-0.5 rounded-full {status.cls}">{status.label}</span>
                  {/if}
                </div>
                {#if m.locked && m.prerequisites && m.prerequisites.length > 0}
                  <p class="text-xs text-gray-400 mt-0.5">Requires: {m.prerequisites.join(', ')}</p>
                {/if}
              </div>
              {#if !m.locked}
                <svg class="w-5 h-5 text-gray-300 group-hover:text-primary-400 transition-colors shrink-0"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              {/if}
            </a>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="card text-center py-8 text-gray-400">
        Enroll in this course to access the modules.
      </div>
    {/if}
  {/if}
</div>
