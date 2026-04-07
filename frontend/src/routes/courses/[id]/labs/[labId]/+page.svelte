<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { labsApi, type Lab, type LabProgress, type SubmissionResult, type FlagResult } from '$lib/api';
  import { auth, isLoggedIn, toasts } from '$lib/stores';

  const courseId = $page.params.id!;
  const labId = $page.params.labId!;

  let lab: Lab | null = null;
  let progress: LabProgress | null = null;
  let loading = true;
  let submitting = false;
  let result: SubmissionResult | null = null;
  let showHint = false;

  // Single-flag CTF state
  let flagInput = '';

  // Multi-flag CTF state: flagId -> input value
  let multiFlags: Record<string, string> = {};
  // Track which flags are confirmed correct (from flag_results)
  let confirmedFlags: Record<string, boolean> = {};

  // Form state: {questionId: answer}
  let formAnswers: Record<string, string> = {};

  onMount(async () => {
    auth.init();
    if (!$isLoggedIn) { goto('/login'); return; }
    try {
      const res = await labsApi.get(courseId, labId, $auth.token!);
      lab = res.lab;
      progress = res.progress;

      // For multi-flag CTF: restore confirmed flags from last submission
      if (lab?.lab_type === 'ctf' && lab.content.flags?.length) {
        try {
          const subs = await labsApi.mySubmissions(courseId, labId, $auth.token!);
          if (subs.submissions.length > 0) {
            // Find best submission (highest score)
            const best = subs.submissions.reduce((a, b) => a.score >= b.score ? a : b);
            if (best.answer?.flags && typeof best.answer.flags === 'object') {
              const submittedFlags = best.answer.flags as Record<string, string>;
              // Pre-fill inputs with last best submission
              multiFlags = { ...submittedFlags };
            }
          }
        } catch {}
      }
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load lab');
      goto(`/courses/${courseId}`);
    } finally {
      loading = false;
    }
  });

  async function submitCTF() {
    if (!flagInput.trim()) { toasts.error('Please enter a flag'); return; }
    submitting = true;
    result = null;
    try {
      result = await labsApi.submit(courseId, labId, { flag: flagInput.trim() }, $auth.token!);
      if (result.is_correct) {
        toasts.success('🎉 Correct flag!');
      } else {
        toasts.error('Wrong flag. Keep trying!');
      }
      const updated = await labsApi.get(courseId, labId, $auth.token!);
      progress = updated.progress;
    } catch (e: any) {
      toasts.error(e.message || 'Submission failed');
    } finally {
      submitting = false;
    }
  }

  async function submitMultiFlag() {
    const flags = lab?.content.flags ?? [];
    const anyFilled = flags.some(f => multiFlags[f.id]?.trim());
    if (!anyFilled) { toasts.error('Enter at least one flag'); return; }

    submitting = true;
    result = null;
    try {
      const payload: Record<string, string> = {};
      for (const f of flags) {
        if (multiFlags[f.id]?.trim()) payload[f.id] = multiFlags[f.id].trim();
      }
      result = await labsApi.submit(courseId, labId, { flags: payload }, $auth.token!);

      // Update confirmed flags from results
      for (const fr of result.flag_results ?? []) {
        if (fr.is_correct) confirmedFlags[fr.flag_id] = true;
      }
      confirmedFlags = { ...confirmedFlags };

      const found = result.flag_results?.filter(r => r.is_correct).length ?? 0;
      const total = result.flag_results?.length ?? 0;
      if (result.is_correct) {
        toasts.success(`🎉 All ${total} flags captured!`);
      } else {
        toasts.info(`${found}/${total} flags correct`);
      }
      const updated = await labsApi.get(courseId, labId, $auth.token!);
      progress = updated.progress;
    } catch (e: any) {
      toasts.error(e.message || 'Submission failed');
    } finally {
      submitting = false;
    }
  }

  async function submitForm() {
    const questions = lab?.content?.questions ?? [];
    for (const q of questions) {
      if (!formAnswers[q.id]) {
        const text = q.text ?? (q as any).question ?? `Question ${q.id}`;
        toasts.error(`Please answer: ${text.substring(0, 40)}...`);
        return;
      }
    }
    submitting = true;
    result = null;
    try {
      result = await labsApi.submit(courseId, labId, { answers: formAnswers }, $auth.token!);
      if (result.is_correct) {
        toasts.success('🎉 Perfect score!');
      } else {
        toasts.info(result.feedback ?? 'Submitted!');
      }
      const updated = await labsApi.get(courseId, labId, $auth.token!);
      progress = updated.progress;
    } catch (e: any) {
      toasts.error(e.message || 'Submission failed');
    } finally {
      submitting = false;
    }
  }

  function getFlagResult(flagId: string): FlagResult | undefined {
    if (!result || !result.flag_results) return undefined;
    return result.flag_results.find(r => r.flag_id === flagId);
  }

  function getQuestionText(question: any, index: number): string {
    return question.text || question.question || ('Question ' + (index + 1));
  }
</script>

<svelte:head>
  <title>{lab?.title ?? 'Lab'} — LearnLab</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <!-- Breadcrumb -->
  <nav class="text-sm text-gray-400 mb-6 flex gap-2">
    <a href="/courses/{courseId}" class="hover:text-primary-500">← Back to Course</a>
  </nav>

  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading lab...</div>
  {:else if !lab}
    <div class="text-center py-16 text-gray-400">Lab not found.</div>
  {:else}
    <!-- Lab Header -->
    <div class="card mb-6">
      <div class="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <div class="flex items-center gap-2 mb-2 flex-wrap">
            <span class={lab.lab_type === 'ctf' ? 'badge-ctf' : 'badge-form'}>
              {lab.lab_type === 'ctf' ? '🚩 CTF Challenge' : '📝 Quiz'}
            </span>
            <span class="text-sm text-gray-400">{lab.points} points</span>
            {#if lab.lab_type === 'ctf' && lab.content.flags?.length}
              <span class="text-sm text-gray-400">· {lab.content.flags.length} flags</span>
            {/if}
          </div>
          <h1 class="text-2xl font-bold text-gray-900">{lab.title}</h1>
          <p class="text-gray-600 mt-2">{lab.description}</p>
        </div>

        {#if progress}
          <div class="text-right shrink-0">
            {#if progress.completed}
              <div class="badge-green text-sm px-3 py-1">✓ Completed</div>
            {/if}
            <div class="text-sm text-gray-400 mt-1">Best: {progress.best_score}/{lab.points} pts</div>
            <div class="text-xs text-gray-300">{progress.total_attempts} attempt{progress.total_attempts !== 1 ? 's' : ''}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- ═══════════════════════════════════════════════════
         MULTI-FLAG CTF
    ════════════════════════════════════════════════════ -->
    {#if lab.lab_type === 'ctf' && lab.content.flags?.length}
      <!-- Instructions -->
      <div class="card mb-6">
        <h2 class="text-lg font-semibold mb-4 text-emerald-700">🚩 Mission</h2>

        {#if lab.content.category}
          <div class="mb-3">
            <span class="badge-ctf">{lab.content.category}</span>
          </div>
        {/if}

        <div class="prose max-w-none text-gray-700 mb-4 whitespace-pre-wrap leading-relaxed">
          {lab.content.instructions ?? lab.content.challenge ?? ''}
        </div>

        {#if lab.content.resources?.length}
          <div class="mb-4">
            <h3 class="text-sm font-medium text-gray-500 mb-2">Resources:</h3>
            <div class="flex gap-2 flex-wrap">
              {#each lab.content.resources as r}
                <a href={r.url} target="_blank" rel="noopener"
                  class="text-sm text-primary-600 hover:underline flex items-center gap-1">
                  📎 {r.name}
                </a>
              {/each}
            </div>
          </div>
        {/if}

        {#if lab.content.hints?.length}
          <button on:click={() => showHint = !showHint}
            class="text-sm text-yellow-600 hover:underline">
            {showHint ? '🔒 Hide hints' : '💡 Show hints'}
          </button>
          {#if showHint}
            <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4 mt-3">
              <ul class="list-disc list-inside space-y-1">
                {#each lab.content.hints as hint}
                  <li class="text-sm text-yellow-800">{hint}</li>
                {/each}
              </ul>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Flags -->
      <div class="space-y-4 mb-6">
        {#each lab.content.flags as flag, i}
          {@const fr = getFlagResult(flag.id)}
          {@const alreadyFound = !!confirmedFlags[flag.id]}
          {@const frCorrect = !!(fr && fr.is_correct)}
          {@const frWrong = !!(fr && !fr.is_correct)}
          {@const isFound = alreadyFound || frCorrect}
          <div class="card border-2 transition-colors
            {isFound ? 'border-green-300 bg-green-50' : frWrong ? 'border-red-200' : 'border-gray-100'}">

            <div class="flex items-start justify-between gap-3 mb-3">
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-mono text-xs bg-gray-100 px-2 py-0.5 rounded text-gray-500">
                    Flag {i + 1}
                  </span>
                  {#if isFound}
                    <span class="text-green-600 text-sm font-medium">✓ Captured!</span>
                  {/if}
                </div>
                <h3 class="font-semibold text-gray-900 mt-1">{flag.name}</h3>
                <p class="text-sm text-gray-500 mt-0.5">{flag.description}</p>
              </div>
              <span class="text-sm font-medium text-gray-500 shrink-0">{flag.points} pts</span>
            </div>

            <div class="flex gap-2">
              <input
                type="text"
                class="input font-mono text-sm flex-1 {isFound ? 'bg-green-50 border-green-300' : frWrong ? 'border-red-300' : ''}"
                placeholder="FLAG&#123;...&#125;"
                bind:value={multiFlags[flag.id]}
                disabled={isFound || submitting}
                on:keydown={(e) => { if (e.key === 'Enter') submitMultiFlag(); }}
              />
              {#if isFound}
                <div class="flex items-center px-3 text-green-600 font-bold">✓</div>
              {:else if frWrong}
                <div class="flex items-center px-3 text-red-400 text-sm">✗</div>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- Global result banner -->
      {#if result}
        <div class="card border-2 text-center mb-6
          {result.is_correct ? 'border-green-400 bg-green-50' : 'border-orange-200 bg-orange-50'}">
          <div class="text-2xl mb-1">{result.is_correct ? '🎉' : '🏴'}</div>
          <div class="font-bold text-lg {result.is_correct ? 'text-green-700' : 'text-orange-700'}">
            {result.feedback}
          </div>
          <div class="text-sm text-gray-500 mt-1">{result.score}/{result.max_score} points</div>
        </div>
      {/if}

      <!-- Submit button -->
      {#if !progress?.completed}
        <button class="btn-ctf w-full py-3 text-base" on:click={submitMultiFlag} disabled={submitting}>
          {submitting ? 'Checking flags...' : '🚩 Submit Flags'}
        </button>
      {:else}
        <div class="text-center">
          <div class="text-green-600 font-semibold text-lg mb-3">🎉 All flags captured!</div>
          <button class="btn-secondary" on:click={submitMultiFlag} disabled={submitting}>
            Try again
          </button>
        </div>
      {/if}

    <!-- ═══════════════════════════════════════════════════
         SINGLE-FLAG CTF (legacy)
    ════════════════════════════════════════════════════ -->
    {:else if lab.lab_type === 'ctf'}
      <div class="card mb-6">
        <h2 class="text-lg font-semibold mb-4 text-emerald-700">🚩 Challenge</h2>
        <div class="prose max-w-none text-gray-700 mb-6 whitespace-pre-wrap leading-relaxed">
          {lab.content.challenge ?? lab.content.instructions ?? 'No challenge description.'}
        </div>

        {#if lab.content.category}
          <div class="mb-4">
            <span class="text-sm font-medium text-gray-500">Category: </span>
            <span class="badge-ctf">{lab.content.category}</span>
          </div>
        {/if}

        {#if lab.content.resources?.length}
          <div class="mb-4">
            <h3 class="text-sm font-medium text-gray-500 mb-2">Resources:</h3>
            <div class="flex gap-2 flex-wrap">
              {#each lab.content.resources as r}
                <a href={r.url} target="_blank" rel="noopener"
                  class="text-sm text-primary-600 hover:underline flex items-center gap-1">
                  📎 {r.name}
                </a>
              {/each}
            </div>
          </div>
        {/if}

        {#if lab.content.hints?.length}
          <button on:click={() => showHint = !showHint}
            class="text-sm text-yellow-600 hover:underline mb-4">
            {showHint ? '🔒 Hide hints' : '💡 Show hints'}
          </button>
          {#if showHint}
            <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4 mb-4">
              <ul class="list-disc list-inside space-y-1">
                {#each lab.content.hints as hint}
                  <li class="text-sm text-yellow-800">{hint}</li>
                {/each}
              </ul>
            </div>
          {/if}
        {/if}

        {#if !progress?.completed}
          <div class="border-t pt-4 mt-2">
            <label class="label">Submit Flag</label>
            <div class="flex gap-2">
              <input type="text" class="input font-mono" bind:value={flagInput}
                placeholder={"FLAG{...}"} on:keydown={(e) => { if (e.key === 'Enter') submitCTF(); }} />
              <button class="btn-ctf" on:click={submitCTF} disabled={submitting}>
                {submitting ? '...' : 'Submit'}
              </button>
            </div>
          </div>
        {:else}
          <div class="border-t pt-4 mt-2 text-center">
            <div class="text-green-600 font-semibold text-lg">🎉 Challenge Completed!</div>
            <p class="text-gray-400 text-sm mt-1">You can still try again to improve your score.</p>
            <div class="flex gap-2 justify-center mt-3">
              <input type="text" class="input font-mono max-w-xs" bind:value={flagInput}
                placeholder="Try again..." />
              <button class="btn-ctf" on:click={submitCTF} disabled={submitting}>Submit</button>
            </div>
          </div>
        {/if}
      </div>

    <!-- ═══════════════════════════════════════════════════
         FORM / QUIZ
    ════════════════════════════════════════════════════ -->
    {:else if lab.lab_type === 'form'}
      <div class="space-y-6">
        {#each lab.content.questions ?? [] as question, i}
          {@const qResults = result && result.question_results ? result.question_results : []}
          {@const qr = qResults.find(r => r.question_id === question.id)}
          {@const questionText = getQuestionText(question, i)}
          <div class="card" class:border-green-300={!!(qr && qr.is_correct)} class:border-red-300={!!(qr && !qr.is_correct)}>
            <div class="flex items-start justify-between gap-2 mb-3">
              <h3 class="font-medium text-gray-900">
                <span class="text-gray-400 text-sm mr-2">Q{i + 1}.</span>
                {questionText}
              </h3>
              <span class="text-sm font-medium text-gray-400 shrink-0">{question.points} pts</span>
            </div>

            {#if question.type === 'multiple_choice' && question.options}
              <div class="space-y-2">
                {#each question.options as option}
                  <label class="flex items-center gap-3 p-3 rounded-lg border cursor-pointer hover:bg-gray-50
                    {formAnswers[question.id] === option ? 'border-primary-400 bg-primary-50' : 'border-gray-200'}
                    {qr && option === qr.correct_answer ? 'border-green-400 bg-green-50' : ''}
                    {qr && formAnswers[question.id] === option && !qr.is_correct ? 'border-red-300 bg-red-50' : ''}
                  ">
                    <input type="radio" name={question.id} value={option}
                      bind:group={formAnswers[question.id]}
                      disabled={!!result} />
                    <span class="text-sm">{option}</span>
                    {#if qr && option === qr.correct_answer}
                      <span class="ml-auto text-green-600 text-xs">✓ Correct</span>
                    {/if}
                  </label>
                {/each}
              </div>
            {:else}
              <textarea class="input" rows="3"
                placeholder="Your answer..."
                bind:value={formAnswers[question.id]}
                disabled={!!result}></textarea>
            {/if}

            {#if qr}
              <div class="mt-3 p-3 rounded-lg text-sm
                {qr.is_correct ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'}">
                {qr.is_correct ? '✓ Correct!' : `✗ Wrong. Correct: ${qr.correct_answer}`}
                {#if qr.explanation}
                  <p class="mt-1 text-gray-600">{qr.explanation}</p>
                {/if}
              </div>
            {/if}
          </div>
        {/each}

        {#if !result}
          <button class="btn-primary w-full py-3 text-base" on:click={submitForm} disabled={submitting}>
            {submitting ? 'Submitting...' : 'Submit Answers'}
          </button>
        {:else}
          <div class="card text-center border-2
            {result.is_correct ? 'border-green-400' : 'border-orange-300'}">
            <div class="text-3xl mb-2">{result.is_correct ? '🎉' : '📊'}</div>
            <div class="text-xl font-bold {result.is_correct ? 'text-green-600' : 'text-orange-600'}">
              {result.score}/{result.max_score} points
            </div>
            <p class="text-gray-500 mt-1">{result.feedback}</p>
            <button class="btn-secondary mt-4" on:click={() => { result = null; formAnswers = {}; }}>
              Try Again
            </button>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>
