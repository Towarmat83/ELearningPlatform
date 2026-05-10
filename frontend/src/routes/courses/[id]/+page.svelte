<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { coursesApi, labsApi, type Course, type Lab } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let course: Course | null = null;
  let labs: Lab[] = [];
  let loading = true;
  let enrolled = false;
  let progress: { completed_labs: number; total_labs: number } | null = null;

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
          const [labsRes, progRes] = await Promise.all([
            labsApi.list(courseId, token),
            labsApi.progress(courseId, token),
          ]);
          labs = labsRes.labs;
          progress = { completed_labs: progRes.completed_labs, total_labs: progRes.total_labs };
        } catch (e: any) {
          if (e.status !== 403) toasts.error('Failed to load labs: ' + e.message);
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
      const [labsRes, progRes] = await Promise.all([
        labsApi.list(courseId, token),
        labsApi.progress(courseId, token),
      ]);
      labs = labsRes.labs;
      progress = { completed_labs: progRes.completed_labs, total_labs: progRes.total_labs };
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

  function labTypeIcon(lab: Lab) {
    const mt = lab.module_type;
    if (mt === 'video') return '🎬';
    if (mt === 'image') return '🖼️';
    if (mt === 'text') return '📝';
    if (lab.lab_type === 'form') return '📝';
    if (lab.lab_type === 'ctf') return '🏴';
    return '💻';
  }

  function displayType(lab: Lab): string {
    return lab.module_type ?? lab.lab_type;
  }

  $: completedCount = progress?.completed_labs ?? 0;
  $: totalCount = progress?.total_labs ?? labs.length;
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
            <span>📝 {course.lab_count} lab{course.lab_count !== 1 ? 's' : ''}</span>
            <span>👥 {course.enrollment_count} enrolled</span>
            {#if course.creator_username}
              <span>👤 {course.creator_username}</span>
            {/if}
          </div>

          {#if !enrolled}
            <button on:click={enroll} class="btn-primary">Enroll Now</button>
          {:else if totalCount > 0}
            <div>
              <div class="flex justify-between text-sm text-gray-500 mb-1">
                <span>{completedCount}/{totalCount} labs completed</span>
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

    <!-- Lab list -->
    {#if enrolled}
      {#if labs.length === 0}
        <div class="card text-center py-8 text-gray-400">No labs available yet.</div>
      {:else}
        <div class="space-y-2">
          {#each labs as lab}
            <a href="/courses/{courseId}/labs/{lab.id}"
              class="card flex items-center gap-4 hover:shadow-md transition-shadow cursor-pointer group p-4 {lab.hidden ? 'opacity-50' : ''}">
              <div class="w-10 h-10 rounded-full flex items-center justify-center shrink-0 text-lg {lab.hidden ? 'bg-gray-200' : 'bg-gray-100'}">
                {labTypeIcon(lab)}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-medium {lab.hidden ? 'text-gray-400' : 'text-gray-900'} group-hover:text-primary-600 transition-colors">
                    {lab.title}
                  </span>
                  <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">{displayType(lab)}</span>
                  {#if lab.hidden}
                    <span class="text-xs text-yellow-600 bg-yellow-50 border border-yellow-200 px-2 py-0.5 rounded-full">hidden</span>
                  {/if}
                </div>
                {#if lab.description}
                  <p class="text-xs text-gray-500 mt-0.5">{lab.description}</p>
                {/if}
              </div>
              <div class="text-right shrink-0">
                <div class="text-sm font-medium text-gray-700">{lab.points} pts</div>
              </div>
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
        Enroll in this course to access the labs.
      </div>
    {/if}
  {/if}
</div>
