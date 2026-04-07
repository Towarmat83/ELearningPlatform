<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { coursesApi, labsApi, type CourseWithStats, type Lab, type CourseProgress } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let course: CourseWithStats | null = null;
  let labs: Lab[] = [];
  let progress: CourseProgress | null = null;
  let loading = true;
  let enrolled = false;

  const courseId = $page.params.id!;

  function getToken(): string | null {
    // Lire depuis le store ou directement depuis localStorage (fiable même avec SSR)
    return $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
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
      try {
        const res = await labsApi.list(courseId, token);
        labs = res.labs;
        enrolled = true;
        try {
          const prog = await labsApi.myProgress(courseId, token);
          progress = prog;
        } catch {}
      } catch (e: any) {
        if (e.status && e.status !== 403) {
          toasts.error('Failed to load labs: ' + e.message);
        }
        enrolled = false;
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
      const res = await labsApi.list(courseId, token);
      labs = res.labs;
      enrolled = true;
      try {
        const prog = await labsApi.myProgress(courseId, token);
        progress = prog;
      } catch {}
    } catch (e: any) {
      toasts.error(e.message || 'Failed to enroll');
    }
  }

  function getLabProgress(labId: string) {
    return progress?.lab_progress.find(p => p.lab_id === labId);
  }

  function difficultyColor(d: string | null) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    return 'badge-red';
  }
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
    <!-- Course Header -->
    <div class="card mb-6">
      <div class="flex flex-col md:flex-row gap-6">
        {#if course.thumbnail}
          <img src={course.thumbnail} alt={course.title}
            class="w-full md:w-64 h-44 object-cover rounded-xl" />
        {:else}
          <div class="w-full md:w-64 h-44 bg-gradient-to-br from-primary-100 to-blue-200 rounded-xl flex items-center justify-center text-6xl">
            📚
          </div>
        {/if}
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
            <span>📚 {course.lab_count} labs</span>
            <span>👥 {course.enrollment_count} enrolled</span>
          </div>

          {#if !enrolled}
            <button on:click={enroll} class="btn-primary">
              Enroll Now
            </button>
          {:else if progress}
            <div>
              <div class="flex justify-between text-sm text-gray-500 mb-1">
                <span>Progress: {progress.completed_labs}/{progress.total_labs} labs</span>
                <span>{Math.round(progress.completion_percentage)}%</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-3">
                <div class="bg-primary-500 h-3 rounded-full"
                  style="width: {progress.completion_percentage}%"></div>
              </div>
              <p class="text-sm text-gray-400 mt-1">
                ⭐ {progress.total_points_earned}/{progress.total_points_possible} points
              </p>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Labs List -->
    {#if enrolled}
      <h2 class="text-xl font-semibold mb-4">Labs</h2>
      {#if labs.length === 0}
        <div class="card text-center py-8 text-gray-400">No labs available yet.</div>
      {:else}
        <div class="space-y-3">
          {#each labs as lab}
            {@const lp = getLabProgress(lab.id)}
            <a href="/courses/{courseId}/labs/{lab.id}"
              class="card flex items-center gap-4 hover:shadow-md transition-shadow cursor-pointer group">
              <!-- Status icon -->
              <div class="w-10 h-10 rounded-full flex items-center justify-center shrink-0
                {lp?.completed ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-400'}">
                {lp?.completed ? '✓' : lab.order_index + 1}
              </div>

              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-medium text-gray-900 group-hover:text-primary-600 transition-colors">
                    {lab.title}
                  </span>
                  <span class={lab.lab_type === 'ctf' ? 'badge-ctf' : 'badge-form'}>
                    {lab.lab_type === 'ctf' ? '🚩 CTF' : '📝 Quiz'}
                  </span>
                </div>
                <p class="text-sm text-gray-500 truncate">{lab.description}</p>
              </div>

              <div class="text-right shrink-0">
                <div class="text-sm font-medium text-gray-700">
                  {lp?.best_score ?? 0}/{lab.points} pts
                </div>
                {#if lp?.total_attempts}
                  <div class="text-xs text-gray-400">{lp.total_attempts} attempt{lp.total_attempts !== 1 ? 's' : ''}</div>
                {/if}
              </div>

              <svg class="w-5 h-5 text-gray-300 group-hover:text-primary-400 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
              </svg>
            </a>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="card text-center py-8 text-gray-400">
        Enroll in this course to access labs.
      </div>
    {/if}
  {/if}
</div>
