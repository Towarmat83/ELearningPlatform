<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { labsApi, instanceApi, type Lab, type LabProgress, type SubmissionResult, type FlagResult, type Submission, type LabInstance } from '$lib/api';
  import { auth, isLoggedIn, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';
  import Terminal from '$lib/Terminal.svelte';

  const courseId = $page.params.id!;
  const labId = $page.params.labId!;

  let lab: Lab | null = null;
  let progress: LabProgress | null = null;
  let loading = true;
  let submitting = false;
  let result: SubmissionResult | null = null;
  let showHint = false;
  let showHistory = false;
  let history: Submission[] = [];
  let loadingHistory = false;

  // Interactive instance state
  let instance: LabInstance | null = null;
  let instanceLoading = false;
  let showTerminal = false;

  // Terminal ref for sendCommand
  let terminalRef: Terminal;

  // Track which commands have been executed (step index -> Set of cmd strings)
  let executedCmds: Set<string> = new Set();

  async function loadHistory() {
    if (history.length > 0) { showHistory = !showHistory; return; }
    loadingHistory = true;
    try {
      const res = await labsApi.mySubmissions(courseId, labId, $auth.token!);
      history = res.submissions;
      showHistory = true;
    } catch { /* non-fatal */ }
    finally { loadingHistory = false; }
  }

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

      // Check for existing running instance
      if (res.lab.content?.docker_image) {
        try {
          instance = await instanceApi.get(courseId, labId, $auth.token!);
          if (instance?.status === 'running') showTerminal = true;
        } catch { /* no instance yet */ }
      }

      // Auto-start instance for interactive labs
      if (res.lab.lab_type === 'interactive' && (!instance || instance.status !== 'running')) {
        await startInstance();
      }

      // For multi-flag CTF: restore confirmed flags from last submission
      if (lab?.lab_type === 'ctf' && lab.content.flags?.length) {
        try {
          const subs = await labsApi.mySubmissions(courseId, labId, $auth.token!);
          if (subs.submissions.length > 0) {
            const best = subs.submissions.reduce((a, b) => a.score >= b.score ? a : b);
            if (best.answer?.flags && typeof best.answer.flags === 'object') {
              const submittedFlags = best.answer.flags as Record<string, string>;
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

  async function startInstance() {
    instanceLoading = true;
    try {
      instance = await instanceApi.start(courseId, labId, $auth.token!);
      showTerminal = true;
      toasts.success('Lab environment started!');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to start lab environment');
    } finally {
      instanceLoading = false;
    }
  }

  async function stopInstance() {
    instanceLoading = true;
    try {
      await instanceApi.stop(courseId, labId, $auth.token!);
      instance = null;
      showTerminal = false;
      toasts.success('Lab environment stopped.');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to stop lab environment');
    } finally {
      instanceLoading = false;
    }
  }

  function formatExpiry(iso: string): string {
    const d = new Date(iso);
    const diff = Math.max(0, d.getTime() - Date.now());
    const mins = Math.floor(diff / 60000);
    return mins > 0 ? `${mins} min remaining` : 'expiring soon';
  }

  function getFlagResult(flagId: string): FlagResult | undefined {
    if (!result || !result.flag_results) return undefined;
    return result.flag_results.find(r => r.flag_id === flagId);
  }

  function getQuestionText(question: any, index: number): string {
    return question.text || question.question || ('Question ' + (index + 1));
  }

  function runCommand(cmd: string) {
    if (!terminalRef) return;
    terminalRef.sendCommand(cmd);
    executedCmds = new Set([...executedCmds, cmd]);
  }
</script>

<svelte:head>
  <title>{lab?.title ?? 'Lab'} — LearnLab</title>
</svelte:head>

<!-- ════════════════════════════════════════════════════════
     INTERACTIVE LAB — full-width split layout
════════════════════════════════════════════════════════ -->
{#if !loading && lab?.lab_type === 'interactive'}
  <div class="interactive-layout">
    <!-- LEFT: Terminal panel -->
    <div class="terminal-panel">
      <!-- Header -->
      <div class="terminal-panel-header">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-lg">🖥️</span>
          <div class="min-w-0">
            <div class="font-semibold text-white text-sm truncate">{lab.title}</div>
            <div class="text-xs text-gray-400 font-mono truncate">{lab.content.docker_image ?? ''}</div>
          </div>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          {#if instance?.status === 'running'}
            <span class="text-xs text-green-400">{formatExpiry(instance.expires_at ?? '')}</span>
            <button
              class="text-xs px-2 py-1 rounded border border-red-700 text-red-400 hover:bg-red-900/30 transition-colors"
              on:click={stopInstance}
              disabled={instanceLoading}
            >
              ■ Stop
            </button>
          {:else if instanceLoading}
            <span class="text-xs text-gray-400 animate-pulse">Starting environment…</span>
          {:else}
            <button class="text-xs px-3 py-1 rounded bg-primary-600 text-white hover:bg-primary-700 transition-colors"
              on:click={startInstance}>
              ▶ Start
            </button>
          {/if}
          <a href="/courses/{courseId}" class="text-xs text-gray-500 hover:text-gray-300 transition-colors">← Course</a>
        </div>
      </div>

      <!-- Terminal -->
      <div class="terminal-body">
        {#if showTerminal && instance?.status === 'running'}
          <Terminal bind:this={terminalRef} {courseId} {labId} token={$auth.token ?? ''} />
        {:else}
          <div class="terminal-placeholder">
            {#if instanceLoading}
              <div class="animate-pulse text-green-400 font-mono text-sm">Initializing container…</div>
            {:else}
              <div class="text-gray-500 font-mono text-sm">Terminal will appear here once the environment is running.</div>
            {/if}
          </div>
        {/if}
      </div>
    </div>

    <!-- RIGHT: Instructions panel -->
    <div class="instructions-panel">
      <!-- Lab header -->
      <div class="instructions-header">
        <div class="flex items-center gap-2 flex-wrap mb-1">
          <span class="badge-interactive">⚡ Interactive Lab</span>
          <span class="text-xs text-gray-400">{lab.points} points</span>
          {#if progress?.completed}
            <span class="badge-green text-xs px-2 py-0.5">✓ Completed</span>
          {/if}
        </div>
        <h1 class="text-xl font-bold text-gray-900 leading-snug">{lab.title}</h1>
        <div class="text-sm text-gray-500 mt-1"><Markdown content={lab.description} /></div>
      </div>

      <!-- Click-to-run hint -->
      <div class="hint-banner">
        <span class="text-blue-700 font-medium text-xs">💡 Clique sur une commande</span>
        <span class="text-blue-600 text-xs"> pour l'exécuter automatiquement dans le terminal.</span>
      </div>

      <!-- Steps -->
      <div class="steps-list">
        {#each lab.content.steps ?? [] as step, si}
          <div class="step-card">
            <h2 class="step-title">{step.title}</h2>
            <div class="step-description prose-sm text-gray-600">
              <Markdown content={step.description} />
            </div>

            {#if step.commands?.length}
              <div class="commands-list">
                {#each step.commands as { cmd, explanation }}
                  <button
                    class="command-btn"
                    class:executed={executedCmds.has(cmd)}
                    on:click={() => runCommand(cmd)}
                    title={explanation ?? 'Cliquer pour exécuter'}
                  >
                    <span class="command-prompt">$</span>
                    <span class="command-text">{cmd}</span>
                    {#if executedCmds.has(cmd)}
                      <span class="command-done">✓</span>
                    {:else}
                      <span class="command-run-icon">▶</span>
                    {/if}
                  </button>
                  {#if explanation}
                    <p class="command-explanation">{explanation}</p>
                  {/if}
                {/each}
              </div>
            {/if}
          </div>
        {/each}

        {#if !(lab.content.steps?.length)}
          <p class="text-gray-400 text-sm italic px-4">No steps defined for this lab.</p>
        {/if}
      </div>
    </div>
  </div>

<!-- ════════════════════════════════════════════════════════
     STANDARD LABS (CTF / Form) — existing layout
════════════════════════════════════════════════════════ -->
{:else}
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
            <div class="text-gray-600 mt-2"><Markdown content={lab.description} /></div>
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
           INTERACTIVE ENVIRONMENT (Docker terminal)
      ════════════════════════════════════════════════════ -->
      {#if lab.content?.docker_image}
        <div class="card mb-6 border-2 border-gray-200">
          <div class="flex items-center justify-between gap-3 flex-wrap">
            <div class="flex items-center gap-3">
              <span class="text-2xl">🖥️</span>
              <div>
                <h2 class="font-semibold text-gray-800">Interactive Environment</h2>
                <p class="text-xs text-gray-400 font-mono">{lab.content.docker_image}</p>
              </div>
            </div>

            <div class="flex items-center gap-2">
              {#if instance?.status === 'running'}
                <span class="text-xs text-gray-400">{formatExpiry(instance.expires_at ?? '')}</span>
                <button
                  class="text-sm px-3 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
                  on:click={() => showTerminal = !showTerminal}
                >
                  {showTerminal ? '▲ Hide terminal' : '▼ Show terminal'}
                </button>
                <button
                  class="text-sm px-3 py-1.5 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 transition-colors"
                  on:click={stopInstance}
                  disabled={instanceLoading}
                >
                  ■ Stop
                </button>
              {:else}
                <button
                  class="btn-primary text-sm"
                  on:click={startInstance}
                  disabled={instanceLoading}
                >
                  {instanceLoading ? 'Starting...' : '▶ Launch Lab'}
                </button>
              {/if}
            </div>
          </div>

          {#if showTerminal && instance?.status === 'running'}
            <div class="mt-4">
              <Terminal {courseId} {labId} token={$auth.token ?? ''} />
            </div>
          {/if}
        </div>
      {/if}

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

          <div class="mb-4">
            <Markdown content={lab.content.instructions ?? lab.content.challenge ?? ''} />
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
          <div class="mb-6">
            <Markdown content={lab.content.challenge ?? lab.content.instructions ?? 'No challenge description.'} />
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
                <h3 class="font-medium text-gray-900 flex-1">
                  <span class="text-gray-400 text-sm mr-2">Q{i + 1}.</span>
                  <Markdown content={questionText} inline={true} />
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

    <!-- ═══════════════════════════════════════════════════
         HISTORIQUE DES SOUMISSIONS
    ════════════════════════════════════════════════════ -->
    {#if progress && progress.total_attempts > 0}
      <div class="mt-6 border-t border-gray-100 pt-4">
        <button
          on:click={loadHistory}
          class="text-sm text-gray-400 hover:text-gray-600 flex items-center gap-1"
          disabled={loadingHistory}
        >
          {#if loadingHistory}
            <span class="animate-spin">↻</span> Loading...
          {:else}
            <span>{showHistory ? '▲' : '▼'}</span>
            Submission history ({progress.total_attempts} attempt{progress.total_attempts !== 1 ? 's' : ''})
          {/if}
        </button>

        {#if showHistory && history.length > 0}
          <div class="mt-3 space-y-1">
            {#each history as sub}
              <div class="flex items-center gap-3 bg-gray-50 rounded-lg px-3 py-2 text-sm">
                <span class={sub.is_correct ? 'text-green-600 font-bold' : 'text-red-400'}>
                  {sub.is_correct ? '✓' : '✗'}
                </span>
                <span class="font-medium text-gray-700">{sub.score} pts</span>
                <span class="text-gray-400">· attempt #{sub.attempts}</span>
                <span class="ml-auto text-xs text-gray-300">
                  {new Date(sub.submitted_at).toLocaleString()}
                </span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  /* ── Interactive split layout ────────────────────────────────── */
  .interactive-layout {
    display: grid;
    grid-template-columns: 1fr 1fr;
    height: calc(100vh - 64px); /* subtract navbar height */
    overflow: hidden;
  }

  /* Terminal (left) */
  .terminal-panel {
    display: flex;
    flex-direction: column;
    background: #0d1117;
    border-right: 1px solid #30363d;
    overflow: hidden;
  }

  .terminal-panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 14px;
    background: #161b22;
    border-bottom: 1px solid #30363d;
    flex-shrink: 0;
  }

  .terminal-body {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }

  .terminal-body :global(.terminal-wrapper) {
    flex: 1;
    border-radius: 0;
    border: none;
  }

  .terminal-body :global(.terminal-container) {
    flex: 1;
    height: 100%;
  }

  .terminal-placeholder {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 32px;
  }

  /* Instructions (right) */
  .instructions-panel {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: #fff;
  }

  .instructions-header {
    padding: 16px 20px 12px;
    border-bottom: 1px solid #e5e7eb;
    flex-shrink: 0;
  }

  .hint-banner {
    padding: 8px 20px;
    background: #eff6ff;
    border-bottom: 1px solid #bfdbfe;
    flex-shrink: 0;
  }

  .steps-list {
    flex: 1;
    overflow-y: auto;
    padding: 12px 0 24px;
  }

  /* Step card */
  .step-card {
    padding: 16px 20px;
    border-bottom: 1px solid #f3f4f6;
  }

  .step-card:last-child {
    border-bottom: none;
  }

  .step-title {
    font-size: 0.9rem;
    font-weight: 700;
    color: #111827;
    margin-bottom: 6px;
  }

  .step-description {
    font-size: 0.85rem;
    line-height: 1.55;
    margin-bottom: 10px;
  }

  /* Commands */
  .commands-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .command-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    background: #0d1117;
    border: 1px solid #30363d;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.15s, border-color 0.15s;
    text-align: left;
    width: 100%;
  }

  .command-btn:hover {
    background: #161b22;
    border-color: #58a6ff;
  }

  .command-btn.executed {
    background: #0d2a0d;
    border-color: #3fb950;
  }

  .command-prompt {
    color: #3fb950;
    font-family: monospace;
    font-size: 0.8rem;
    font-weight: bold;
    flex-shrink: 0;
  }

  .command-text {
    color: #c9d1d9;
    font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 0.8rem;
    flex: 1;
    white-space: pre;
  }

  .command-run-icon {
    color: #58a6ff;
    font-size: 0.65rem;
    opacity: 0.6;
    flex-shrink: 0;
  }

  .command-done {
    color: #3fb950;
    font-size: 0.75rem;
    font-weight: bold;
    flex-shrink: 0;
  }

  .command-explanation {
    font-size: 0.75rem;
    color: #6b7280;
    margin: 0 0 4px 12px;
    font-style: italic;
  }

  /* Badge for interactive type */
  :global(.badge-interactive) {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 9999px;
    font-size: 0.7rem;
    font-weight: 600;
    background: #ede9fe;
    color: #6d28d9;
    border: 1px solid #c4b5fd;
  }
</style>
