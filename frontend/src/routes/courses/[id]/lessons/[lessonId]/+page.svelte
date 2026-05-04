<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { lessonsApi, type LessonDetail, type LessonSummary } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  let lesson: LessonDetail | null = null;
  let allLessons: LessonSummary[] = [];
  let loading = true;
  let completing = false;

  $: courseSlug = $page.params.id;
  $: lessonSlug = $page.params.lessonId;
  $: currentIndex = allLessons.findIndex(l => l.slug === lessonSlug);
  $: prevLesson = currentIndex > 0 ? allLessons[currentIndex - 1] : null;
  $: nextLesson = currentIndex < allLessons.length - 1 ? allLessons[currentIndex + 1] : null;

  afterNavigate(async () => {
    auth.init();
    const token = $auth.token;
    if (!token) {
      goto('/login?redirect=' + encodeURIComponent($page.url.pathname));
      return;
    }
    const cSlug = $page.params.id;
    const lSlug = $page.params.lessonId;

    loading = true;
    lesson = null;

    try {
      const [listRes, lessonRes] = await Promise.all([
        lessonsApi.list(cSlug, token),
        lessonsApi.get(cSlug, lSlug, token),
      ]);
      allLessons = listRes.lessons;
      lesson = lessonRes.lesson;
    } catch (e: any) {
      if (e.status === 403) {
        goto('/courses/' + cSlug);
        return;
      }
      toasts.error('Failed to load lesson');
    } finally {
      loading = false;
    }
  });

  async function markComplete() {
    if (!lesson || lesson.viewed || completing) return;
    const token = $auth.token;
    if (!token) return;
    completing = true;
    try {
      await lessonsApi.complete(courseSlug, lessonSlug, token);
      lesson = { ...lesson, viewed: true };
      allLessons = allLessons.map(l => l.slug === lessonSlug ? { ...l, viewed: true } : l);
      toasts.success('Lesson marked as complete!');
    } catch {
      toasts.error('Could not mark lesson as complete');
    } finally {
      completing = false;
    }
  }
</script>

<svelte:head>
  <title>{lesson?.title ?? 'Lesson'} — LearnLab</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <!-- Breadcrumb -->
  <div class="flex items-center gap-2 text-sm text-gray-400 mb-6">
    <a href="/courses" class="hover:text-primary-600">Courses</a>
    <span>/</span>
    <a href="/courses/{courseSlug}" class="hover:text-primary-600">{courseSlug}</a>
    <span>/</span>
    <span class="text-gray-600">{lesson?.title ?? '...'}</span>
  </div>

  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading lesson...</div>
  {:else if !lesson}
    <div class="text-center py-16 text-gray-400">Lesson not found.</div>
  {:else}
    <!-- Header -->
    <div class="flex items-start justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">{lesson.title}</h1>
        <p class="text-sm text-gray-400 mt-1">Lesson {lesson.order}</p>
      </div>
      {#if lesson.viewed}
        <span class="badge-green shrink-0">✓ Completed</span>
      {:else}
        <button on:click={markComplete} disabled={completing} class="btn-primary text-sm shrink-0">
          {completing ? 'Saving...' : 'Mark as complete'}
        </button>
      {/if}
    </div>

    <!-- Content -->
    <div class="card prose prose-gray max-w-none">
      <Markdown content={lesson.content} />
    </div>

    <!-- Navigation -->
    <div class="flex justify-between mt-8 gap-4">
      {#if prevLesson}
        <a href="/courses/{courseSlug}/lessons/{prevLesson.slug}"
          class="btn-secondary flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
          {prevLesson.title}
        </a>
      {:else}
        <a href="/courses/{courseSlug}" class="btn-secondary flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
          Back to course
        </a>
      {/if}

      {#if nextLesson}
        <a href="/courses/{courseSlug}/lessons/{nextLesson.slug}"
          class="btn-primary flex items-center gap-2 ml-auto">
          {nextLesson.title}
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </a>
      {:else}
        <a href="/courses/{courseSlug}" class="btn-secondary flex items-center gap-2 ml-auto">
          Back to course
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </a>
      {/if}
    </div>
  {/if}
</div>
