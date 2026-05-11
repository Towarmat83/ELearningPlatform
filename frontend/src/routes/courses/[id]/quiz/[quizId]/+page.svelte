<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { modulesApi, type ModuleDetail, type QuizQuestion, type QuizSubmitResponse, type QuizCooldown } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  const courseId = $page.params.id as string;
  const moduleIndex = parseInt($page.params.quizId ?? '', 10);

  let quiz: ModuleDetail | null = null;
  let loading = true;

  let answers: Record<string, any> = {};
  let submitted = false;
  let submitting = false;
  let result: QuizSubmitResponse | null = null;

  let cooldowns: Record<string, { remaining: number; attempts: number; locked: boolean }> = {};
  let cooldownTimer: ReturnType<typeof setInterval> | null = null;

  let dragSource: { qId: string; idx: number } | null = null;
  let dropTarget: { qId: string; idx: number } | null = null;

  let allAnswered = false;

  function setAnswer(qId: string, value: any) {
    answers = { ...answers, [qId]: value };
    allAnswered = computeAllAnswered();
  }

  function cloneAnswers(patches: Record<string, any>) {
    answers = { ...answers, ...patches };
  }

  function computeAllAnswered(): boolean {
    if (!quiz?.questions) return false;
    for (const qq of quiz.questions) {
      const v = answers[qq.id];
      if (qq.type === 'single' && !v) return false;
      if (qq.type === 'multiple' && (!v || v.length === 0)) return false;
      if (qq.type === 'boolean' && v === null) return false;
      if (qq.type === 'order' && (!v || v.length === 0)) return false;
    }
    return true;
  }

  function initAnswers(q: ModuleDetail) {
    const init: Record<string, any> = {};
    for (const qq of q.questions ?? []) {
      if (qq.type === 'single') init[qq.id] = '';
      else if (qq.type === 'multiple') init[qq.id] = [];
      else if (qq.type === 'boolean') init[qq.id] = null;
      else if (qq.type === 'order') init[qq.id] = qq.items?.map(i => i.id) ?? [];
    }
    answers = init;
    allAnswered = computeAllAnswered();
  }

  onMount(async () => {
    auth.init();
    const token = $auth.token;
    if (!token) {
      goto('/login?redirect=' + encodeURIComponent($page.url.pathname));
      return;
    }
    try {
      const res = await modulesApi.get(courseId, moduleIndex, token);
      quiz = res;
      initAnswers(quiz);
      if (res.cooldowns) startCooldownTimer(res.cooldowns);
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load quiz');
    } finally {
      loading = false;
    }
  });

  onDestroy(() => {
    if (cooldownTimer) clearInterval(cooldownTimer);
  });

  function startCooldownTimer(cds: Record<string, QuizCooldown>) {
    const next: Record<string, any> = {};
    for (const [key, cd] of Object.entries(cds)) {
      next[key] = { remaining: cd.remaining_seconds, attempts: cd.attempts, locked: cd.locked };
    }
    cooldowns = next;
    if (cooldownTimer) clearInterval(cooldownTimer);
    cooldownTimer = setInterval(() => {
      let anyActive = false;
      for (const key of Object.keys(cooldowns)) {
        if (cooldowns[key].remaining > 0) {
          cooldowns[key].remaining -= 1;
          anyActive = true;
        }
      }
      cooldowns = { ...cooldowns };
      if (!anyActive && cooldownTimer) {
        clearInterval(cooldownTimer);
        cooldownTimer = null;
      }
    }, 1000);
  }

  function questionCooldown(qId: string): { remaining: number; attempts: number; locked: boolean } | null {
    return cooldowns[qId] ?? null;
  }

  function canRetry(): boolean {
    if (!result || result.passed) return false;
    for (const cd of Object.values(cooldowns)) {
      if (cd.remaining > 0) return false;
    }
    return true;
  }

  async function submit() {
    const token = $auth.token;
    if (!token || !quiz) return;
    submitting = true;
    try {
      const payload: Record<string, any> = {};
      for (const qq of quiz.questions ?? []) {
        const val = answers[qq.id];
        if (qq.type === 'single') payload[qq.id] = { single: val || null };
        else if (qq.type === 'multiple') payload[qq.id] = { multiple: val };
        else if (qq.type === 'boolean') payload[qq.id] = { boolean: val };
        else if (qq.type === 'order') payload[qq.id] = { order: val };
      }
      const res = await modulesApi.submit(courseId, moduleIndex, payload, token);
      result = res;
      submitted = true;
      if (res.cooldowns) startCooldownTimer(res.cooldowns);
    } catch (e: any) {
      if (e.status === 425 && e.body?.cooldowns) {
        startCooldownTimer(e.body.cooldowns as Record<string, QuizCooldown>);
      } else {
        toasts.error(e.message || 'Submission failed');
      }
    } finally {
      submitting = false;
    }
  }

  function retry() {
    submitted = false;
    result = null;
    if (quiz) initAnswers(quiz);
  }

  // ── Order: drag-and-drop + buttons ──

  function onDragStart(qId: string, idx: number, event: DragEvent) {
    dragSource = { qId, idx };
    dropTarget = null;
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', `${qId}:${idx}`);
    }
  }

  function onDragOver(qId: string, idx: number, event: DragEvent) {
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    if (!dragSource || dragSource.qId !== qId) return;
    const el = event.currentTarget as HTMLElement;
    const rect = el.getBoundingClientRect();
    const midY = rect.top + rect.height / 2;
    const pos = event.clientY < midY ? idx : idx + 1;
    if (!dropTarget || dropTarget.qId !== qId || dropTarget.idx !== pos) {
      dropTarget = { qId, idx: pos };
    }
  }

  function onDragLeave() {
    setTimeout(() => { dropTarget = null; }, 50);
  }

  function onDrop(qId: string, toIdx: number, event: DragEvent) {
    event.preventDefault();
    if (!dragSource || dragSource.qId !== qId) {
      dragSource = null;
      dropTarget = null;
      return;
    }
    let targetIdx = toIdx;
    if (dropTarget && dropTarget.qId === qId) {
      targetIdx = dropTarget.idx;
    }
    const arr = [...answers[qId]];
    const [moved] = arr.splice(dragSource.idx, 1);
    const insertAt = targetIdx > dragSource.idx ? targetIdx - 1 : targetIdx;
    arr.splice(insertAt, 0, moved);
    setAnswer(qId, arr);
    dragSource = null;
    dropTarget = null;
  }

  function onDragEnd() {
    dragSource = null;
    dropTarget = null;
  }

  function moveUp(qId: string, idx: number) {
    if (idx <= 0) return;
    const arr = [...answers[qId]];
    [arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]];
    setAnswer(qId, arr);
  }

  function moveDown(qId: string, idx: number) {
    const arr = [...answers[qId]];
    if (idx >= arr.length - 1) return;
    [arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]];
    setAnswer(qId, arr);
  }

  // ── Multiple choice toggle ──

  function toggleMultiple(qId: string, ansId: string) {
    const arr = [...(answers[qId] ?? [])];
    const i = arr.indexOf(ansId);
    if (i >= 0) arr.splice(i, 1);
    else arr.push(ansId);
    setAnswer(qId, arr);
  }

  function ghostItemText(qq: QuizQuestion): string {
    if (!dragSource || dragSource.qId !== qq.id) return '';
    const arr = answers[qq.id];
    if (!arr || dragSource.idx >= arr.length) return '';
    const id = arr[dragSource.idx];
    return qq.items?.find(i => i.id === id)?.text ?? '';
  }

  function questionTypeIcon(q: QuizQuestion): string {
    if (q.type === 'single') return '◯';
    if (q.type === 'multiple') return '☐';
    if (q.type === 'boolean') return '✓/✗';
    return '↕';
  }

  function difficultyColor(d: string): string {
    if (d === 'easy') return 'badge-green';
    if (d === 'hard') return 'badge-red';
    return 'badge-yellow';
  }
</script>

<svelte:head>
  <title>{quiz?.name ?? 'Quiz'} — LearnLab</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  <div class="flex items-center gap-2 text-sm text-gray-400 mb-6">
    <a href="/courses" class="hover:text-primary-600">Courses</a>
    <span>/</span>
    <a href="/courses/{courseId}" class="hover:text-primary-600">{courseId}</a>
    <span>/</span>
    <span class="text-gray-600">{quiz?.name ?? '...'}</span>
  </div>

  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading quiz...</div>
  {:else if !quiz}
    <div class="text-center py-16 text-gray-400">Quiz not found.</div>
  {:else}
    <div class="card mb-6">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-bold text-gray-900">{quiz.name}</h1>
          <div class="flex gap-4 mt-2 text-sm text-gray-400">
            <span>{quiz.questions?.length ?? 0} questions</span>
            {#if quiz.quiz_config}
              <span>Score requis : {quiz.quiz_config.passing_score}%</span>
              {#if quiz.quiz_config.max_attempts_per_question}
                <span>Max {quiz.quiz_config.max_attempts_per_question} tentatives/question</span>
              {/if}
            {/if}
          </div>
        </div>
      </div>
    </div>

    {#if submitted && result}
      <div class="card mb-6 p-6 {result.passed ? 'bg-green-50 border border-green-200' : 'bg-yellow-50 border border-yellow-200'}">
        <div class="text-center">
          <p class="text-3xl font-bold {result.passed ? 'text-green-600' : 'text-yellow-600'}">
            {result.total_score}/{result.max_score}
          </p>
          <p class="text-sm text-gray-500 mt-1">
            {#if result.passed}
              ✓ Quiz réussi !
            {:else if quiz.quiz_config}
              Score insuffisant ({Math.round(result.total_score / result.max_score * 100)}% &lt; {quiz.quiz_config.passing_score}%)
            {:else}
              Score insuffisant
            {/if}
          </p>
          {#if result.passed}
            <a href="/courses/{courseId}" class="btn-primary text-sm mt-3 inline-block">← Back to course</a>
          {:else if canRetry()}
            <button on:click={retry} class="btn-primary text-sm mt-3">Réessayer</button>
          {:else}
            {#each Object.entries(cooldowns) as [qId, cd]}
              {#if cd.remaining > 0}
                <p class="text-sm text-gray-500 mt-2">
                  Question {qId} : réessayez dans {cd.remaining}s
                  {#if cd.locked}(verrouillée){/if}
                </p>
              {/if}
            {/each}
          {/if}
        </div>
      </div>
    {/if}

    <div class="space-y-6">
      {#each quiz.questions ?? [] as qq, i}
        {@const cd = questionCooldown(qq.id)}
        <div class="card p-6 {cd && cd.remaining > 0 ? 'opacity-50' : ''}" id="q-{qq.id}">
          <div class="flex items-start gap-3 mb-4">
            <span class="text-lg font-mono shrink-0">{questionTypeIcon(qq)}</span>
            <div class="flex-1">
              <div class="flex items-center gap-2 mb-1">
                <span class="font-semibold text-gray-800">Question {i + 1}</span>
                <span class="text-xs text-gray-400">({qq.points} pt{qq.points > 1 ? 's' : ''})</span>
                {#if qq.difficulty}
                  <span class={difficultyColor(qq.difficulty)}>{qq.difficulty}</span>
                {/if}
                {#if qq.source_refs && qq.source_refs.length > 0}
                  <button
                    on:click|stopPropagation
                    class="ml-auto text-xs text-primary-500 hover:text-primary-700 underline underline-offset-2"
                    on:click={() => { const el = document.getElementById('help-' + qq.id); if (el) el.classList.toggle('hidden'); }}>
                    📖 Aide
                  </button>
                {/if}
              </div>
              <div class="prose prose-sm max-w-none text-gray-700">
                <Markdown content={qq.question} />
              </div>
            </div>
          </div>

          {#if submitted && result}
            {@const qr = result.question_results.find(r => r.question_id === qq.id)}
            {#if qr}
              <div class="mb-4 p-3 rounded-lg {qr.is_correct ? 'bg-green-50 border border-green-100' : 'bg-red-50 border border-red-100'}">
                <div class="flex items-center gap-2 text-sm">
                  <span class="font-bold {qr.is_correct ? 'text-green-700' : 'text-red-700'}">
                    {qr.is_correct ? '✓' : '✗'} {qr.points_earned}/{qr.points_max} pts
                  </span>
                </div>
                {#if qr.feedback}
                  <p class="text-sm text-gray-600 mt-1">{qr.feedback}</p>
                {/if}
                {#if qr.source_refs && qr.source_refs.length > 0}
                  <div class="mt-2 text-xs text-gray-500">
                    <p class="font-medium mb-1">Révisions recommandées :</p>
                    {#each qr.source_refs as ref}
                      <a href="/courses/{ref.course}/lessons/{ref.module}" class="block text-primary-600 hover:underline">
                        → {ref.module} #{ref.anchor}
                      </a>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          {/if}

          {#if cd && cd.remaining > 0}
            <div class="mb-3 p-2 rounded bg-orange-50 border border-orange-200 text-sm text-orange-700">
              {#if cd.locked}
                🔒 Question verrouillée (tentatives épuisées)
              {:else}
                ⏳ Réessayez dans {cd.remaining}s (tentative {cd.attempts})
              {/if}
            </div>
          {/if}

          <div class="pl-7">
            {#if qq.type === 'single'}
              <div class="space-y-2">
                {#each qq.answers ?? [] as ans}
                  <button
                    on:click={() => setAnswer(qq.id, ans.id)}
                    class="w-full text-left flex items-center gap-3 p-3 rounded-lg border transition-colors
                      {answers[qq.id] === ans.id ? 'border-primary-400 bg-primary-50 text-primary-800' : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'}"
                    disabled={submitted && !canRetry()}>
                    <span class="w-5 h-5 rounded-full border-2 shrink-0 flex items-center justify-center
                      {answers[qq.id] === ans.id ? 'border-primary-500 bg-primary-500' : 'border-gray-300'}">
                      {#if answers[qq.id] === ans.id}
                        <span class="w-2 h-2 rounded-full bg-white"></span>
                      {/if}
                    </span>
                    <span class="text-sm">{ans.text}</span>
                  </button>
                {/each}
              </div>

            {:else if qq.type === 'multiple'}
              <div class="space-y-2">
                {#each qq.answers ?? [] as ans}
                  {@const checked = answers[qq.id]?.includes(ans.id) ?? false}
                  <button
                    on:click={() => toggleMultiple(qq.id, ans.id)}
                    class="w-full text-left flex items-center gap-3 p-3 rounded-lg border transition-colors
                      {checked ? 'border-primary-400 bg-primary-50 text-primary-800' : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'}"
                    disabled={submitted && !canRetry()}>
                    <span class="w-5 h-5 rounded border-2 shrink-0 flex items-center justify-center
                      {checked ? 'border-primary-500 bg-primary-500' : 'border-gray-300'}">
                      {#if checked}
                        <span class="text-white text-xs font-bold">✓</span>
                      {/if}
                    </span>
                    <span class="text-sm">{ans.text}</span>
                  </button>
                {/each}
              </div>

            {:else if qq.type === 'boolean'}
              <div class="flex gap-3">
                {#each [{ value: true, label: 'Vrai' }, { value: false, label: 'Faux' }] as opt}
                  <button
                    on:click={() => setAnswer(qq.id, opt.value)}
                    class="flex-1 p-3 rounded-lg border text-center text-sm font-medium transition-colors
                      {answers[qq.id] === opt.value ? 'bg-primary-50 border-primary-400 text-primary-700' : 'bg-white border-gray-200 text-gray-600 hover:bg-gray-50'}"
                    disabled={submitted && !canRetry()}>
                    {opt.label}
                  </button>
                {/each}
              </div>

            {:else if qq.type === 'order'}
              <div class="space-y-1">
                {#each answers[qq.id] ?? [] as itemId, idx}
                  {@const showBefore = dropTarget?.qId === qq.id && dropTarget.idx === idx}
                  {#if showBefore}
                    {@const ghostItem = ghostItemText(qq)}
                    <!-- svelte-ignore a11y-no-static-element-interactions -->
                    <div class="flex items-center gap-2 px-3 py-2 rounded-lg border-2 border-dashed border-primary-400 bg-primary-50/50"
                      on:drop={(e) => { e.preventDefault(); onDrop(qq.id, idx, e); }}
                      on:dragover={(e) => { e.preventDefault(); }}>
                      <span class="text-sm font-mono text-primary-400 w-6 shrink-0">{idx + 1}.</span>
                      <div class="flex-1 text-sm text-primary-500 font-medium italic">{ghostItem || ''}</div>
                      <span class="text-xs text-primary-400 shrink-0">↕ déposer ici</span>
                    </div>
                  {/if}
                  {@const item = qq.items?.find(i => i.id === itemId)}
                  <div
                    class="flex items-center gap-2 p-3 rounded-lg border transition-colors
                      {dragSource?.qId === qq.id && dragSource?.idx === idx && !dropTarget
                        ? 'border-primary-400 bg-primary-50 opacity-40'
                        : dragSource?.qId === qq.id && dragSource?.idx === idx
                        ? 'border-gray-200 bg-gray-50 opacity-30'
                        : 'border-gray-200 bg-white hover:border-primary-300'}"
                    draggable={!(submitted && !canRetry())}
                    on:dragstart={(e) => onDragStart(qq.id, idx, e)}
                    on:dragover={(e) => onDragOver(qq.id, idx, e)}
                    on:drop={(e) => onDrop(qq.id, idx, e)}
                    on:dragend={onDragEnd}>
                    <span class="cursor-grab text-gray-400 shrink-0 text-sm">⠿</span>
                    <span class="text-sm font-mono text-gray-400 w-6 shrink-0">{idx + 1}.</span>
                    <div class="flex-1 text-sm">{item?.text ?? itemId}</div>
                    <div class="flex gap-1 shrink-0">
                      <button on:click={() => moveUp(qq.id, idx)} type="button"
                        class="p-1 text-gray-400 hover:text-gray-600 disabled:opacity-30 text-sm"
                        disabled={idx === 0 || (submitted && !canRetry())}>▲</button>
                      <button on:click={() => moveDown(qq.id, idx)} type="button"
                        class="p-1 text-gray-400 hover:text-gray-600 disabled:opacity-30 text-sm"
                        disabled={idx === answers[qq.id].length - 1 || (submitted && !canRetry())}>▼</button>
                    </div>
                  </div>
                  {#if idx === answers[qq.id].length - 1 && dropTarget?.qId === qq.id && dropTarget.idx === idx + 1}
                    {@const ghostItem = ghostItemText(qq)}
                    <!-- svelte-ignore a11y-no-static-element-interactions -->
                    <div class="flex items-center gap-2 px-3 py-2 rounded-lg border-2 border-dashed border-primary-400 bg-primary-50/50"
                      on:drop={(e) => { e.preventDefault(); onDrop(qq.id, idx + 1, e); }}
                      on:dragover={(e) => { e.preventDefault(); }}>
                      <span class="text-sm font-mono text-primary-400 w-6 shrink-0">{idx + 2}.</span>
                      <div class="flex-1 text-sm text-primary-500 font-medium italic">{ghostItem || ''}</div>
                      <span class="text-xs text-primary-400 shrink-0">↕ déposer ici</span>
                    </div>
                  {/if}
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    {#if !submitted || !result?.passed}
      <div class="text-center mt-8">
        {#if submitted}
          {#if canRetry()}
            <button on:click={retry} class="btn-primary">Réessayer le quiz</button>
          {:else}
            <button class="btn-primary opacity-50 cursor-not-allowed" disabled>
              ⏳ Cooldown actif...
            </button>
          {/if}
        {:else}
          <button on:click={submit} class="btn-primary" disabled={submitting || !allAnswered}>
            {submitting ? 'Correction...' : 'Soumettre les réponses'}
          </button>
          {#if !allAnswered}
            <p class="text-xs text-gray-400 mt-2">Répondez à toutes les questions avant de soumettre.</p>
          {/if}
        {/if}
      </div>
    {/if}
  {/if}
</div>
