<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { api, labsApi, type Lab, type LabProgress, type SubmissionResult } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  const courseId = $page.params.id as string;
  const labId = $page.params.labId as string;

  let lab: Lab | null = null;
  let progress: LabProgress | null = null;
  let loading = true;

  // Module/lesson content
  let lessonContent: string | null = null;
  let lessonTitle = '';

  // Form submission
  let submitting = false;
  let result: SubmissionResult | null = null;

  // CTF submission
  let flagInput = '';
  // Form answers
  let answers: Record<string, string> = {};
  let questions: { id: string; question: string; type?: string; options?: string[] }[] = [];

  onMount(async () => {
    auth.init();
    const token = $auth.token;
    if (!token) {
      goto('/login?redirect=' + encodeURIComponent($page.url.pathname));
      return;
    }
    try {
      const res = await labsApi.get(courseId, labId, token);
      lab = res.lab;
      progress = res.progress;
      // If no interactive content, fetch and show lesson content instead
      if (lab && lab.lab_type !== 'ctf' && !lab.content?.questions) {
        try {
          const lessonRes: any = await api.get(`/courses/${courseId}/lessons/${labId}`, token);
          lessonContent = lessonRes.lesson?.content ?? null;
          lessonTitle = lessonRes.lesson?.title ?? lab.title;
        } catch {
          // fallback: show lab without lesson content
        }
        loading = false;
        return;
      }
      // Initialize form answers
      if (lab.lab_type === 'form' && lab.content?.questions) {
        questions = lab.content.questions as any[];
        for (const q of questions) {
          answers[q.id] = '';
        }
      }
    } catch (e: any) {
      if (e.status === 403) {
        goto('/courses/' + courseId);
        return;
      }
      toasts.error('Failed to load lab');
    } finally {
      loading = false;
    }
  });

  async function submit() {
    const token = $auth.token;
    if (!token || !lab) return;
    submitting = true;
    result = null;
    try {
      let answer: unknown;
      if (lab.lab_type === 'ctf') {
        answer = { flag: flagInput };
      } else if (lab.lab_type === 'form') {
        answer = { answers };
      } else {
        answer = {};
      }
      const res = await labsApi.submit(courseId, labId, answer, token);
      result = res;
      // Refresh progress
      const progRes = await labsApi.progress(courseId, token);
      const myProg = progRes.lab_progress.find(p => p.lab_id === labId);
      if (myProg) {
        progress = {
          completed: myProg.completed,
          best_score: myProg.best_score,
          total_attempts: myProg.total_attempts,
          completed_at: myProg.completed_at,
        };
      }
    } catch (e: any) {
      toasts.error(e.message || 'Submission failed');
    } finally {
      submitting = false;
    }
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

  function retry() {
    result = null;
    flagInput = '';
  }
</script>

<svelte:head>
  <title>{lab?.title ?? 'Lab'} — LearnLab</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <!-- Breadcrumb -->
  <div class="flex items-center gap-2 text-sm text-gray-400 mb-6">
    <a href="/courses" class="hover:text-primary-600">Courses</a>
    <span>/</span>
    <a href="/courses/{courseId}" class="hover:text-primary-600">{courseId}</a>
    <span>/</span>
    <span class="text-gray-600">{lab?.title ?? '...'}</span>
  </div>

  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading lab...</div>
  {:else if !lab}
    <div class="text-center py-16 text-gray-400">Lab not found.</div>
  {:else}
    <!-- Header -->
    <div class="flex items-start justify-between gap-4 mb-6">
      <div>
        <div class="flex items-center gap-2 mb-1">
          <span class="text-xl">{labTypeIcon(lab)}</span>
          <h1 class="text-2xl font-bold text-gray-900">{lab.title}</h1>
          <span class="badge-blue text-xs">{displayType(lab)}</span>
        </div>
        {#if lab.description}
          <p class="text-gray-500 mt-1">{lab.description}</p>
        {/if}
        <p class="text-sm text-gray-400 mt-1">{lab.points} points</p>
      </div>
      <div class="text-right shrink-0">
        {#if progress}
          {#if progress.completed}
            <span class="badge-green">✓ Completed ({progress.best_score}/{lab.points})</span>
          {:else}
            <span class="text-sm text-gray-400">Attempts: {progress.total_attempts}</span>
          {/if}
        {/if}
      </div>
    </div>

    <!-- Content -->
    <div class="card mb-6">
      {#if lab?.module_type === 'video' && lessonContent}
        <div class="flex justify-center">
          <video controls class="max-w-full rounded-lg">
            <source src={lessonContent} type="video/mp4" />
          </video>
        </div>

      {:else if lab?.module_type === 'image' && lessonContent}
        <div class="flex justify-center">
          <img src={lessonContent} alt={lab.title} class="max-w-full rounded-lg" />
        </div>

      {:else if lessonContent !== null}
        <div class="prose max-w-none">
          <Markdown content={lessonContent} />
        </div>

      {:else if lab?.lab_type === 'ctf'}
        <div class="space-y-4">
          <p class="text-gray-700">Enter the flag to complete this challenge.</p>

          {#if result}
            <div class="p-4 rounded-lg {result.is_correct ? 'bg-green-50 border border-green-200' : 'bg-red-50 border border-red-200'}">
              <p class="font-semibold {result.is_correct ? 'text-green-700' : 'text-red-700'}">
                {result.is_correct ? '✓ Correct!' : '✗ Incorrect'}
              </p>
              <p class="text-sm mt-1 text-gray-600">Score: {result.score}/{result.max_score}</p>
              {#if result.feedback}
                <p class="text-sm mt-1 text-gray-600">{result.feedback}</p>
              {/if}
              {#if result.flag_results}
                <div class="mt-2 space-y-1">
                  {#each result.flag_results as fr}
                    <p class="text-sm {fr.is_correct ? 'text-green-600' : 'text-red-600'}">
                      {fr.name}: {fr.is_correct ? '✓' : '✗'} ({fr.points_earned} pts)
                    </p>
                  {/each}
                </div>
              {/if}
            </div>

            {#if !result.is_correct}
              <button on:click={retry} class="btn-primary text-sm">Try Again</button>
            {:else}
              <a href="/courses/{courseId}" class="btn-secondary text-sm">← Back to course</a>
            {/if}

          {:else}
            <div class="flex gap-2">
              <input type="text" class="input font-mono flex-1" placeholder={'FLAG{...}'}
                bind:value={flagInput} />
              <button on:click={submit} class="btn-primary" disabled={submitting || !flagInput}>
                {submitting ? 'Checking...' : 'Submit Flag'}
              </button>
            </div>
          {/if}
        </div>

      {:else if lab?.lab_type === 'form'}
        <div class="space-y-6">
          {#if result}
            <div class="p-4 rounded-lg {result.is_correct ? 'bg-green-50 border border-green-200' : 'bg-yellow-50 border border-yellow-200'}">
              <p class="font-semibold {result.is_correct ? 'text-green-700' : 'text-yellow-700'}">
                Score: {result.score}/{result.max_score}
              </p>
              {#if result.feedback}
                <p class="text-sm mt-1 text-gray-600">{result.feedback}</p>
              {/if}
              {#if result.question_results}
                <div class="mt-3 space-y-2">
                  {#each result.question_results as qr}
                    <div class="p-3 rounded-lg {qr.is_correct ? 'bg-green-50' : 'bg-red-50'}">
                      <div class="flex justify-between text-sm">
                        <span class="{qr.is_correct ? 'text-green-700' : 'text-red-700'} font-medium">
                          {qr.is_correct ? '✓' : '✗'} Question {qr.question_id}
                        </span>
                        <span class="text-gray-500">{qr.points_earned} pts</span>
                      </div>
                      {#if qr.correct_answer}
                        <p class="text-xs text-gray-500 mt-1">Correct answer: {qr.correct_answer}</p>
                      {/if}
                      {#if qr.explanation}
                        <p class="text-xs text-gray-400 mt-0.5">{qr.explanation}</p>
                      {/if}
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
            <a href="/courses/{courseId}" class="btn-secondary text-sm">← Back to course</a>

          {:else}
            {#if questions.length > 0}
              {#each questions as q, i}
                <div>
                  <label class="block text-sm font-medium text-gray-700 mb-1">
                    {i + 1}. {q.question}
                  </label>
                  {#if q.type === 'choice' && q.options}
                    <div class="space-y-1">
                      {#each q.options as opt}
                        <label class="flex items-center gap-2 text-sm p-2 rounded hover:bg-gray-50 cursor-pointer">
                          <input type="radio" name="q-{q.id}" value={opt}
                            bind:group={answers[q.id]} />
                          {opt}
                        </label>
                      {/each}
                    </div>
                  {:else}
                    <textarea class="input w-full" rows="3" placeholder="Your answer..."
                      bind:value={answers[q.id]}></textarea>
                  {/if}
                </div>
              {/each}

              <button on:click={submit} class="btn-primary" disabled={submitting}>
                {submitting ? 'Submitting...' : 'Submit Answers'}
              </button>
            {:else}
              <p class="text-gray-400 text-center py-4">No questions configured for this lab.</p>
            {/if}
          {/if}
        </div>

      {:else}
        <div class="text-center py-8 text-gray-400">
          <p class="text-4xl mb-4">💻</p>
          <p class="font-medium mb-2">Interactive Lab</p>
          <p class="text-sm">Interactive labs are not yet available in this view.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>
