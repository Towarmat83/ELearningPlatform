<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { modulesApi, lessonsApi, type ModuleDetail } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  const courseId = $page.params.id as string;
  const moduleIndex = parseInt($page.params.labId ?? '', 10);

  let mod: ModuleDetail | null = null;
  let loading = true;
  let marking = false;

  onMount(async () => {
    auth.init();
    const token = $auth.token;
    if (!token) {
      goto('/login?redirect=' + encodeURIComponent($page.url.pathname));
      return;
    }
    try {
      mod = await modulesApi.get(courseId, moduleIndex, token);
      // Redirect quiz modules to the quiz page
      if (mod.type === 'quiz') {
        goto(`/courses/${courseId}/quiz/${moduleIndex}`, { replaceState: true });
        return;
      }
    } catch (e: any) {
      if (e.status === 403) {
        toasts.error('Complete prerequisites before accessing this module');
        goto('/courses/' + courseId);
        return;
      }
      toasts.error('Failed to load module');
    } finally {
      loading = false;
    }
  });

  async function markComplete() {
    const token = $auth.token;
    if (!token || !mod || mod.viewed) return;
    marking = true;
    try {
      await lessonsApi.markComplete(courseId, mod.slug, token);
      mod = { ...mod, viewed: true };
      toasts.success('Module marked as complete!');
    } catch {
      toasts.error('Failed to mark as complete');
    } finally {
      marking = false;
    }
  }
</script>

<svelte:head>
  <title>{mod?.name ?? 'Module'} — LearnLab</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <!-- Breadcrumb -->
  <div class="flex items-center gap-2 text-sm text-gray-400 mb-6">
    <a href="/courses" class="hover:text-primary-600">Courses</a>
    <span>/</span>
    <a href="/courses/{courseId}" class="hover:text-primary-600">{courseId}</a>
    <span>/</span>
    <span class="text-gray-600">{mod?.name ?? '...'}</span>
  </div>

  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading...</div>
  {:else if !mod}
    <div class="text-center py-16 text-gray-400">Module not found.</div>
  {:else}
    <!-- Header -->
    <div class="flex items-start justify-between gap-4 mb-6">
      <div>
        <div class="flex items-center gap-2 mb-1">
          <span class="text-xl">
            {#if mod.type === 'video'}🎬{:else if mod.type === 'image'}🖼️{:else}📝{/if}
          </span>
          <h1 class="text-2xl font-bold text-gray-900">{mod.name}</h1>
          <span class="badge-blue text-xs">{mod.type}</span>
        </div>
      </div>
      <div class="shrink-0">
        {#if mod.viewed}
          <span class="badge-green">✓ Viewed</span>
        {/if}
      </div>
    </div>

    <!-- Content -->
    <div class="card mb-6">
      {#if mod.type === 'video' && mod.content}
        <div class="flex justify-center">
          <video controls class="max-w-full rounded-lg">
            <source src={mod.content} type="video/mp4" />
          </video>
        </div>

      {:else if mod.type === 'image' && mod.content}
        <div class="flex justify-center">
          <img src={mod.content} alt={mod.name} class="max-w-full rounded-lg" />
        </div>

      {:else if mod.content}
        <div class="prose max-w-none">
          <Markdown content={mod.content} />
        </div>

      {:else}
        <div class="text-center py-8 text-gray-400">No content available.</div>
      {/if}
    </div>

    <div class="flex justify-between items-center">
      <a href="/courses/{courseId}" class="btn-secondary text-sm">← Back to course</a>
      {#if !mod.viewed}
        <button
          on:click={markComplete}
          disabled={marking}
          class="btn-primary text-sm"
        >
          {marking ? 'Saving...' : '✓ Mark as complete'}
        </button>
      {/if}
    </div>
  {/if}
</div>
