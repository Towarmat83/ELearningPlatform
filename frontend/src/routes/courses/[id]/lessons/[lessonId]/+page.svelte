<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { lessonsApi, type LessonDetail, type LessonSummary } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  const courseId = $page.params.id!;
  const lessonId = $page.params.lessonId!;

  let lesson: LessonDetail | null = null;
  let allLessons: LessonSummary[] = [];
  let loading = true;
  let completing = false;

  // Cache for fetched markdown file contents: url → markdown string
  let markdownFileCache: Record<string, string> = {};

  async function fetchMarkdownFile(url: string): Promise<string> {
    if (markdownFileCache[url] !== undefined) return markdownFileCache[url];
    try {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const text = await res.text();
      markdownFileCache[url] = text;
      markdownFileCache = markdownFileCache; // trigger reactivity
      return text;
    } catch (e: any) {
      const msg = `> ⚠️ Failed to load \`${url}\`: ${e.message}`;
      markdownFileCache[url] = msg;
      markdownFileCache = markdownFileCache;
      return msg;
    }
  }

  // Kick off all markdown_file fetches once the lesson loads
  $: if (lesson) {
    for (const block of lesson.content) {
      if (block.type === 'markdown_file') fetchMarkdownFile(block.url);
    }
  }

  $: currentIndex = allLessons.findIndex(l => l.id === lessonId);
  $: prevLesson = currentIndex > 0 ? allLessons[currentIndex - 1] : null;
  $: nextLesson = currentIndex < allLessons.length - 1 ? allLessons[currentIndex + 1] : null;

  onMount(async () => {
    auth.init();
    const token = $auth.token;
    if (!token) {
      goto('/login?redirect=' + encodeURIComponent($page.url.pathname));
      return;
    }
    try {
      const [lessonRes, listRes] = await Promise.all([
        lessonsApi.get(courseId, lessonId, token),
        lessonsApi.list(courseId, token),
      ]);
      lesson = lessonRes.lesson;
      allLessons = listRes.lessons;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load lesson');
      goto('/courses/' + courseId);
    } finally {
      loading = false;
    }
  });

  async function markComplete() {
    if (!lesson || lesson.viewed || completing) return;
    completing = true;
    try {
      await lessonsApi.complete(courseId, lessonId, $auth.token!);
      lesson = { ...lesson, viewed: true };
      // Update in list too
      allLessons = allLessons.map(l => l.id === lessonId ? { ...l, viewed: true } : l);
      toasts.success('Lesson completed!');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to mark complete');
    } finally {
      completing = false;
    }
  }

  function navigateTo(id: string) {
    goto(`/courses/${courseId}/lessons/${id}`);
  }
</script>

<svelte:head>
  <title>{lesson?.title ?? 'Lesson'} — LearnLab</title>
</svelte:head>

<div class="max-w-5xl mx-auto px-4 py-6">
  {#if loading}
    <div class="text-center py-20 text-gray-400">Loading...</div>
  {:else if !lesson}
    <div class="text-center py-20 text-gray-400">Lesson not found.</div>
  {:else}
    <div class="flex gap-6">

      <!-- ── Sidebar: lesson list ─────────────────────────────────────────── -->
      {#if allLessons.length > 1}
        <aside class="hidden lg:block w-64 shrink-0">
          <div class="card sticky top-4">
            <a href="/courses/{courseId}" class="flex items-center gap-1 text-xs text-gray-400 hover:text-primary-600 mb-4 transition-colors">
              ← Back to course
            </a>
            <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">Lessons</h3>
            <nav class="space-y-1">
              {#each allLessons as ls}
                <button
                  on:click={() => navigateTo(ls.id)}
                  class="w-full text-left flex items-center gap-2 px-2 py-2 rounded-lg text-sm transition-colors
                    {ls.id === lessonId
                      ? 'bg-primary-50 text-primary-700 font-medium'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-800'}">
                  <span class="shrink-0 w-4 h-4 flex items-center justify-center text-xs">
                    {#if ls.viewed}
                      <span class="text-green-500">✓</span>
                    {:else if ls.id === lessonId}
                      <span class="w-2 h-2 rounded-full bg-primary-500 inline-block"></span>
                    {:else}
                      <span class="w-2 h-2 rounded-full bg-gray-300 inline-block"></span>
                    {/if}
                  </span>
                  <span class="truncate">{ls.title}</span>
                </button>
              {/each}
            </nav>
          </div>
        </aside>
      {/if}

      <!-- ── Main content ─────────────────────────────────────────────────── -->
      <div class="flex-1 min-w-0">

        <!-- Header -->
        <div class="mb-2">
          <a href="/courses/{courseId}" class="text-sm text-gray-400 hover:text-primary-600 transition-colors lg:hidden">
            ← Back to course
          </a>
        </div>
        <div class="flex items-start justify-between gap-4 mb-6">
          <div>
            <h1 class="text-2xl font-bold text-gray-900">{lesson.title}</h1>
            {#if currentIndex >= 0}
              <p class="text-sm text-gray-400 mt-1">
                Lesson {currentIndex + 1} of {allLessons.length}
              </p>
            {/if}
          </div>
          {#if lesson.viewed}
            <span class="inline-flex items-center gap-1 px-3 py-1 bg-green-50 text-green-700 border border-green-200 rounded-full text-sm font-medium shrink-0">
              ✓ Completed
            </span>
          {:else}
            <button
              on:click={markComplete}
              disabled={completing}
              class="btn-primary text-sm shrink-0">
              {completing ? 'Saving...' : 'Mark as complete'}
            </button>
          {/if}
        </div>

        <!-- Content blocks -->
        <div class="space-y-6">
          {#each lesson.content as block (block.id)}
            {#if block.type === 'text'}
              <div class="card prose-container">
                <Markdown content={block.markdown} />
              </div>
            {:else if block.type === 'markdown_file'}
              <div class="card prose-container">
                {#if block.title}
                  <p class="text-xs text-gray-400 font-mono mb-3 flex items-center gap-1">
                    <span>📄</span> {block.title}
                  </p>
                {/if}
                {#if markdownFileCache[block.url] !== undefined}
                  <Markdown content={markdownFileCache[block.url]} />
                {:else}
                  <p class="text-sm text-gray-400 italic">Loading {block.url}…</p>
                {/if}
              </div>

            {:else if block.type === 'video'}
              <div class="card p-0 overflow-hidden">
                {#if block.title}
                  <div class="px-5 py-3 border-b border-gray-100">
                    <h3 class="font-medium text-gray-700">{block.title}</h3>
                  </div>
                {/if}
                <div class="bg-black rounded-b-xl">
                  <!-- svelte-ignore a11y-media-has-caption -->
                  <video
                    class="w-full max-h-[560px] outline-none"
                    controls
                    preload="metadata"
                    src={block.url}
                    on:ended={markComplete}>
                    Your browser does not support video playback.
                  </video>
                </div>
              </div>
            {/if}
          {/each}

          {#if lesson.content.length === 0}
            <div class="card text-center py-12 text-gray-400">
              This lesson has no content yet.
            </div>
          {/if}
        </div>

        <!-- Navigation buttons -->
        <div class="flex items-center justify-between mt-8 pt-6 border-t border-gray-100">
          {#if prevLesson}
            <button on:click={() => navigateTo(prevLesson?.id ?? '')}
              class="flex items-center gap-2 text-sm text-gray-500 hover:text-primary-600 transition-colors group">
              <svg class="w-4 h-4 group-hover:-translate-x-0.5 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
              </svg>
              <span class="truncate max-w-[200px]">{prevLesson.title}</span>
            </button>
          {:else}
            <div></div>
          {/if}

          {#if nextLesson}
            <button on:click={() => navigateTo(nextLesson?.id ?? '')}
              class="flex items-center gap-2 text-sm text-gray-500 hover:text-primary-600 transition-colors group ml-auto">
              <span class="truncate max-w-[200px]">{nextLesson.title}</span>
              <svg class="w-4 h-4 group-hover:translate-x-0.5 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
              </svg>
            </button>
          {:else}
            <a href="/courses/{courseId}"
              class="flex items-center gap-2 text-sm font-medium text-primary-600 hover:text-primary-700 transition-colors ml-auto">
              Back to course →
            </a>
          {/if}
        </div>

      </div>
    </div>
  {/if}
</div>

<style>
  :global(.prose-container .markdown-content) { line-height: 1.7; color: #374151; }
  :global(.prose-container .markdown-content h1) { font-size: 1.5rem; font-weight: 700; color: #111827; margin: 0 0 1rem; }
  :global(.prose-container .markdown-content h2) { font-size: 1.25rem; font-weight: 600; color: #1f2937; margin: 1.25rem 0 0.75rem; }
  :global(.prose-container .markdown-content h3) { font-size: 1.125rem; font-weight: 600; color: #374151; margin: 1rem 0 0.5rem; }
  :global(.prose-container .markdown-content p)  { margin-bottom: 1rem; }
  :global(.prose-container .markdown-content ul) { list-style: disc; padding-left: 1.5rem; margin-bottom: 1rem; }
  :global(.prose-container .markdown-content ol) { list-style: decimal; padding-left: 1.5rem; margin-bottom: 1rem; }
  :global(.prose-container .markdown-content li) { margin-bottom: 0.25rem; }
  :global(.prose-container .markdown-content code) { background: #f3f4f6; color: #1f2937; padding: 0.125rem 0.375rem; border-radius: 0.25rem; font-size: 0.875rem; font-family: monospace; }
  :global(.prose-container .markdown-content pre)  { background: #111827; color: #f9fafb; border-radius: 0.75rem; padding: 1rem; overflow-x: auto; margin-bottom: 1rem; }
  :global(.prose-container .markdown-content pre code) { background: transparent; color: #f9fafb; padding: 0; }
  :global(.prose-container .markdown-content blockquote) { border-left: 4px solid #bfdbfe; padding-left: 1rem; color: #6b7280; font-style: italic; margin: 1rem 0; }
  :global(.prose-container .markdown-content a) { color: #2563eb; text-decoration: underline; }
  :global(.prose-container .markdown-content strong) { font-weight: 600; color: #111827; }
  :global(.prose-container .markdown-content hr) { border-color: #e5e7eb; margin: 1.5rem 0; }
  :global(.prose-container .markdown-content table) { width: 100%; font-size: 0.875rem; border-collapse: collapse; margin-bottom: 1rem; }
  :global(.prose-container .markdown-content th) { border: 1px solid #e5e7eb; padding: 0.5rem 0.75rem; background: #f9fafb; font-weight: 600; text-align: left; }
  :global(.prose-container .markdown-content td) { border: 1px solid #e5e7eb; padding: 0.5rem 0.75rem; }
</style>
