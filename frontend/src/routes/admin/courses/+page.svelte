<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, coursesApi, labsApi, type CourseWithStats, type Lab, type AdminEnrollment, type AdminUser } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  // ─── State ────────────────────────────────────────────────────────────────

  let courses: CourseWithStats[] = [];
  let loading = true;
  let creating = false;
  let expandedCourse: string | null = null;
  let courseLabs: Record<string, Lab[]> = {};
  let courseEnrollments: Record<string, AdminEnrollment[]> = {};
  let allUsers: AdminUser[] = [];
  let selectedUserId: Record<string, string> = {};
  let enrollingFor: string | null = null;

  let newCourse = { title: '', description: '', category: '', difficulty: 'beginner', is_published: false };

  // ─── Lab form ─────────────────────────────────────────────────────────────

  type LabFormMode =
    | { mode: 'create'; courseId: string }
    | { mode: 'edit'; courseId: string; labId: string }
    | null;

  type FormQuestion = {
    id: string;
    text: string;
    type: 'multiple_choice' | 'text';
    options: string[];
    correct_answer: string;
    points: number;
    explanation: string;
  };

  type MultiFlag = {
    id: string;
    name: string;
    description: string;
    points: number;
    flag: string;
  };

  let labFormMode: LabFormMode = null;
  let labFormSaving = false;

  let labForm = {
    title: '',
    description: '',
    lab_type: 'form' as 'form' | 'ctf',
    points: 100,
    is_published: true,
    order_index: 0,
    // Form
    questions: [] as FormQuestion[],
    // CTF
    ctf_mode: 'single' as 'single' | 'multi',
    ctf_challenge: '',
    ctf_category: 'misc',
    ctf_hints: [''] as string[],
    ctf_flag: '',
    ctf_instructions: '',
    ctf_flags: [] as MultiFlag[],
  };

  function newQuestion(): FormQuestion {
    return {
      id: `q${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      text: '',
      type: 'multiple_choice',
      options: ['', '', '', ''],
      correct_answer: '',
      points: 10,
      explanation: '',
    };
  }

  function newMultiFlag(): MultiFlag {
    return {
      id: `flag${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      name: '',
      description: '',
      points: 50,
      flag: '',
    };
  }

  function resetLabForm() {
    labForm = {
      title: '',
      description: '',
      lab_type: 'form',
      points: 100,
      is_published: true,
      order_index: 0,
      questions: [newQuestion()],
      ctf_mode: 'single',
      ctf_challenge: '',
      ctf_category: 'misc',
      ctf_hints: [''],
      ctf_flag: '',
      ctf_instructions: '',
      ctf_flags: [newMultiFlag()],
    };
  }

  // ─── Lifecycle ────────────────────────────────────────────────────────────

  onMount(async () => {
    if (!$auth.token) { toasts.error('Not authenticated'); return; }
    loadCourses();
    try {
      const res = await adminApi.users($auth.token!);
      allUsers = res.users;
    } catch { /* non-fatal */ }
  });

  // ─── Courses ──────────────────────────────────────────────────────────────

  async function loadCourses() {
    try {
      const res = await adminApi.courses($auth.token!);
      courses = res.courses;
    } catch { toasts.error('Failed to load courses'); }
    finally { loading = false; }
  }

  async function createCourse() {
    if (!newCourse.title) { toasts.error('Title required'); return; }
    creating = true;
    try {
      await coursesApi.create(newCourse, $auth.token!);
      toasts.success('Course created');
      newCourse = { title: '', description: '', category: '', difficulty: 'beginner', is_published: false };
      await loadCourses();
    } catch (e: any) { toasts.error(e.message); }
    finally { creating = false; }
  }

  async function toggleCoursePublish(course: CourseWithStats) {
    try {
      await coursesApi.update(course.id, { is_published: !course.is_published }, $auth.token!);
      course.is_published = !course.is_published;
      courses = courses;
      toasts.success('Updated');
    } catch (e: any) { toasts.error(e.message); }
  }

  async function deleteCourse(id: string) {
    if (!confirm('Delete this course and all its labs?')) return;
    try {
      await coursesApi.delete(id, $auth.token!);
      courses = courses.filter(c => c.id !== id);
      toasts.success('Course deleted');
    } catch (e: any) { toasts.error(e.message); }
  }

  // ─── Labs ─────────────────────────────────────────────────────────────────

  async function loadLabs(courseId: string) {
    if (expandedCourse === courseId) { expandedCourse = null; return; }
    expandedCourse = courseId;
    if (!selectedUserId[courseId]) selectedUserId[courseId] = '';
    await refreshLabs(courseId);
    if (!courseEnrollments[courseId]) {
      try {
        const res = await adminApi.courseEnrollments(courseId, $auth.token!);
        courseEnrollments[courseId] = res.enrollments;
        courseEnrollments = courseEnrollments;
      } catch { courseEnrollments[courseId] = []; }
    }
  }

  async function refreshLabs(courseId: string) {
    try {
      const res = await labsApi.list(courseId, $auth.token!);
      courseLabs[courseId] = res.labs;
      courseLabs = courseLabs;
    } catch { courseLabs[courseId] = []; }
  }

  function startCreateLab(courseId: string) {
    resetLabForm();
    labFormMode = { mode: 'create', courseId };
  }

  async function startEditLab(courseId: string, labId: string) {
    try {
      const lab = await adminApi.adminGetLab(courseId, labId, $auth.token!);
      resetLabForm();
      labForm.title = lab.title;
      labForm.description = lab.description;
      labForm.lab_type = lab.lab_type as 'form' | 'ctf';
      labForm.points = lab.points;
      labForm.is_published = lab.is_published;
      labForm.order_index = lab.order_index;

      if (lab.lab_type === 'form') {
        const qs = (lab.content.questions ?? []) as any[];
        labForm.questions = qs.map(q => ({
          id: q.id ?? newQuestion().id,
          text: q.text ?? q.question ?? '',
          type: q.type === 'text' ? 'text' : 'multiple_choice',
          options: q.options ?? ['', '', '', ''],
          correct_answer: q.correct_answer ?? '',
          points: q.points ?? 10,
          explanation: q.explanation ?? '',
        }));
        if (labForm.questions.length === 0) labForm.questions = [newQuestion()];
      } else {
        const flags = lab.content.flags as any[] | undefined;
        if (flags && flags.length > 0) {
          labForm.ctf_mode = 'multi';
          labForm.ctf_instructions = (lab.content.instructions as string) ?? '';
          let flagValues: Record<string, string> = {};
          if (lab.flag) {
            try { flagValues = JSON.parse(lab.flag); } catch {}
          }
          labForm.ctf_flags = flags.map(f => ({
            id: f.id,
            name: f.name ?? '',
            description: f.description ?? '',
            points: f.points ?? 50,
            flag: flagValues[f.id] ?? '',
          }));
        } else {
          labForm.ctf_mode = 'single';
          labForm.ctf_challenge = (lab.content.challenge as string) ?? (lab.content.instructions as string) ?? '';
          labForm.ctf_category = (lab.content.category as string) ?? 'misc';
          const hints = lab.content.hints as string[] | undefined;
          labForm.ctf_hints = hints && hints.length > 0 ? hints : [''];
          labForm.ctf_flag = lab.flag ?? '';
        }
      }

      labFormMode = { mode: 'edit', courseId, labId };
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load lab');
    }
  }

  function buildLabPayload() {
    let content: unknown;
    let flag: string | undefined;

    if (labForm.lab_type === 'form') {
      content = {
        questions: labForm.questions.map(q => ({
          id: q.id,
          text: q.text,
          type: q.type,
          ...(q.type === 'multiple_choice' ? { options: q.options.filter(o => o.trim()) } : {}),
          correct_answer: q.correct_answer,
          points: q.points,
          ...(q.explanation ? { explanation: q.explanation } : {}),
        })),
      };
    } else if (labForm.ctf_mode === 'single') {
      content = {
        challenge: labForm.ctf_challenge,
        category: labForm.ctf_category,
        hints: labForm.ctf_hints.filter(h => h.trim()),
      };
      flag = labForm.ctf_flag;
    } else {
      content = {
        flags: labForm.ctf_flags.map(f => ({
          id: f.id,
          name: f.name,
          description: f.description,
          points: f.points,
        })),
        instructions: labForm.ctf_instructions,
      };
      const flagMap: Record<string, string> = {};
      for (const f of labForm.ctf_flags) flagMap[f.id] = f.flag;
      flag = JSON.stringify(flagMap);
    }

    return {
      title: labForm.title,
      description: labForm.description,
      lab_type: labForm.lab_type,
      content,
      flag,
      points: labForm.points,
      order_index: labForm.order_index,
      is_published: labForm.is_published,
    };
  }

  async function saveLab() {
    if (!labForm.title.trim()) { toasts.error('Lab title required'); return; }
    if (labForm.lab_type === 'ctf' && labForm.ctf_mode === 'single' && !labForm.ctf_flag.trim()) {
      toasts.error('Flag required for CTF lab');
      return;
    }
    if (labForm.lab_type === 'ctf' && labForm.ctf_mode === 'multi') {
      if (labForm.ctf_flags.length === 0) { toasts.error('Add at least one flag'); return; }
      if (labForm.ctf_flags.some(f => !f.flag.trim())) { toasts.error('All flags must have a value'); return; }
    }
    if (labForm.lab_type === 'form' && labForm.questions.length === 0) {
      toasts.error('Add at least one question');
      return;
    }

    labFormSaving = true;
    const mode = labFormMode!;
    try {
      const payload = buildLabPayload();
      if (mode.mode === 'create') {
        await labsApi.create(mode.courseId, payload, $auth.token!);
        toasts.success('Lab created');
      } else {
        await labsApi.update(mode.courseId, mode.labId, payload, $auth.token!);
        toasts.success('Lab updated');
      }
      labFormMode = null;
      delete courseLabs[mode.courseId];
      await refreshLabs(mode.courseId);
    } catch (e: any) {
      toasts.error(e.message || 'Failed to save lab');
    } finally {
      labFormSaving = false;
    }
  }

  async function deleteLab(courseId: string, labId: string) {
    if (!confirm('Delete this lab?')) return;
    try {
      await labsApi.delete(courseId, labId, $auth.token!);
      courseLabs[courseId] = courseLabs[courseId].filter(l => l.id !== labId);
      courseLabs = courseLabs;
      if (labFormMode && labFormMode.mode === 'edit' && labFormMode.labId === labId) {
        labFormMode = null;
      }
      toasts.success('Lab deleted');
    } catch (e: any) { toasts.error(e.message); }
  }

  async function toggleLabPublish(courseId: string, lab: Lab) {
    try {
      await labsApi.update(courseId, lab.id, { is_published: !lab.is_published }, $auth.token!);
      lab.is_published = !lab.is_published;
      courseLabs = courseLabs;
      toasts.success(lab.is_published ? 'Lab published' : 'Lab unpublished');
    } catch (e: any) { toasts.error(e.message); }
  }

  // ─── Question helpers ─────────────────────────────────────────────────────

  function addQuestion() {
    labForm.questions = [...labForm.questions, newQuestion()];
  }

  function removeQuestion(i: number) {
    labForm.questions = labForm.questions.filter((_, idx) => idx !== i);
  }

  function addOption(qIdx: number) {
    labForm.questions[qIdx].options = [...labForm.questions[qIdx].options, ''];
    labForm.questions = labForm.questions;
  }

  function removeOption(qIdx: number, oIdx: number) {
    labForm.questions[qIdx].options = labForm.questions[qIdx].options.filter((_, i) => i !== oIdx);
    labForm.questions = labForm.questions;
  }

  // ─── CTF helpers ──────────────────────────────────────────────────────────

  function addHint() { labForm.ctf_hints = [...labForm.ctf_hints, '']; }
  function removeHint(i: number) { labForm.ctf_hints = labForm.ctf_hints.filter((_, idx) => idx !== i); }
  function addMultiFlag() { labForm.ctf_flags = [...labForm.ctf_flags, newMultiFlag()]; }
  function removeMultiFlag(i: number) { labForm.ctf_flags = labForm.ctf_flags.filter((_, idx) => idx !== i); }

  // ─── Enrollments ──────────────────────────────────────────────────────────

  async function enrollUser(courseId: string) {
    const userId = selectedUserId[courseId];
    if (!userId) { toasts.error('Select a user'); return; }
    enrollingFor = courseId;
    try {
      await adminApi.enrollUser(courseId, userId, $auth.token!);
      const res = await adminApi.courseEnrollments(courseId, $auth.token!);
      courseEnrollments[courseId] = res.enrollments;
      courseEnrollments = courseEnrollments;
      selectedUserId[courseId] = '';
      const c = courses.find(c => c.id === courseId);
      if (c) { c.enrollment_count += 1; courses = courses; }
      toasts.success('User enrolled');
    } catch (e: any) { toasts.error(e.message); }
    finally { enrollingFor = null; }
  }

  async function unenrollUser(courseId: string, userId: string) {
    if (!confirm('Remove this student from the course?')) return;
    try {
      await adminApi.unenrollUser(courseId, userId, $auth.token!);
      courseEnrollments[courseId] = courseEnrollments[courseId].filter(e => e.user_id !== userId);
      courseEnrollments = courseEnrollments;
      const c = courses.find(c => c.id === courseId);
      if (c) { c.enrollment_count = Math.max(0, c.enrollment_count - 1); courses = courses; }
      toasts.success('User unenrolled');
    } catch (e: any) { toasts.error(e.message); }
  }

  // ─── Helpers ──────────────────────────────────────────────────────────────

  function labTypeLabel(type: string) {
    return type === 'ctf' ? '🚩 CTF' : '📝 Quiz';
  }

  function labFormTitle() {
    if (!labFormMode) return '';
    return labFormMode.mode === 'create' ? 'New Lab' : 'Edit Lab';
  }
</script>

<svelte:head><title>Courses — Admin</title></svelte:head>

<div class="p-8 max-w-5xl mx-auto">
  <h1 class="text-2xl font-bold text-gray-800 mb-6">Course & Lab Management</h1>

  <!-- ══════════════════════════════════════════════════
       CREATE COURSE
  ═══════════════════════════════════════════════════ -->
  <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-6 mb-6">
    <h2 class="font-semibold text-gray-700 mb-4">Create New Course</h2>
    <div class="grid md:grid-cols-2 gap-3">
      <div>
        <label class="label">Title *</label>
        <input class="input" bind:value={newCourse.title} placeholder="Course title" />
      </div>
      <div>
        <label class="label">Category</label>
        <input class="input" bind:value={newCourse.category} placeholder="e.g. Security, Web..." />
      </div>
      <div class="md:col-span-2">
        <label class="label">Description</label>
        <textarea class="input" rows="2" bind:value={newCourse.description} placeholder="Course description"></textarea>
      </div>
      <div>
        <label class="label">Difficulty</label>
        <select class="input" bind:value={newCourse.difficulty}>
          <option value="beginner">Beginner</option>
          <option value="intermediate">Intermediate</option>
          <option value="advanced">Advanced</option>
        </select>
      </div>
      <div class="flex items-center gap-2 pt-5">
        <input type="checkbox" id="pub" bind:checked={newCourse.is_published} />
        <label for="pub" class="text-sm text-gray-700">Publish immediately</label>
      </div>
    </div>
    <button class="btn-primary mt-4" on:click={createCourse} disabled={creating}>
      {creating ? 'Creating...' : '+ Create Course'}
    </button>
  </div>

  <!-- ══════════════════════════════════════════════════
       COURSE LIST
  ═══════════════════════════════════════════════════ -->
  {#if loading}
    <div class="text-gray-400 py-8 text-center">Loading...</div>
  {:else}
    <div class="space-y-3">
      {#each courses as course (course.id)}
        <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">

          <!-- Course row -->
          <div class="flex items-center gap-4 p-4">
            <button on:click={() => loadLabs(course.id)} class="text-gray-400 hover:text-gray-700 w-5 text-center font-mono">
              {expandedCourse === course.id ? '▼' : '▶'}
            </button>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-medium text-gray-900">{course.title}</span>
                <span class={course.is_published ? 'badge-green' : 'badge-yellow'}>
                  {course.is_published ? 'Published' : 'Draft'}
                </span>
                {#if course.difficulty}
                  <span class="badge-blue">{course.difficulty}</span>
                {/if}
              </div>
              <div class="text-xs text-gray-400">
                {course.lab_count} labs · {course.enrollment_count} enrolled
                {#if course.creator_username} · by {course.creator_username}{/if}
              </div>
            </div>
            <div class="flex gap-2 shrink-0">
              <button on:click={() => toggleCoursePublish(course)} class="text-xs btn-secondary">
                {course.is_published ? 'Unpublish' : 'Publish'}
              </button>
              <button on:click={() => deleteCourse(course.id)} class="text-xs btn-danger">Delete</button>
            </div>
          </div>

          <!-- Expanded content -->
          {#if expandedCourse === course.id}
            <div class="border-t border-gray-100 bg-gray-50 p-4 space-y-4">

              <!-- Labs list -->
              <div>
                <div class="flex items-center justify-between mb-3">
                  <h3 class="text-sm font-semibold text-gray-600">Labs ({courseLabs[course.id]?.length ?? 0})</h3>
                  {#if !labFormMode || labFormMode.courseId !== course.id}
                    <button class="text-xs btn-primary" on:click={() => startCreateLab(course.id)}>
                      + Add Lab
                    </button>
                  {/if}
                </div>

                {#if courseLabs[course.id]?.length}
                  <div class="space-y-2">
                    {#each courseLabs[course.id] as lab (lab.id)}
                      {@const isEditing = labFormMode?.mode === 'edit' && labFormMode.labId === lab.id}
                      <div class="bg-white rounded-lg border-2 transition-colors p-3 flex items-center gap-3
                        {isEditing ? 'border-primary-300' : 'border-gray-100'}">
                        <span class={lab.lab_type === 'ctf' ? 'badge-ctf text-xs' : 'badge-form text-xs'}>
                          {labTypeLabel(lab.lab_type)}
                        </span>
                        <span class="flex-1 text-sm font-medium text-gray-700 truncate">{lab.title}</span>
                        <span class="text-xs text-gray-400 shrink-0">{lab.points} pts</span>
                        <button
                          on:click={() => toggleLabPublish(course.id, lab)}
                          class="text-xs shrink-0 {lab.is_published ? 'badge-green' : 'badge-yellow'} cursor-pointer hover:opacity-75 transition-opacity">
                          {lab.is_published ? 'Live' : 'Draft'}
                        </button>
                        <button
                          on:click={() => startEditLab(course.id, lab.id)}
                          class="text-xs btn-secondary shrink-0">
                          Edit
                        </button>
                        <button on:click={() => deleteLab(course.id, lab.id)} class="text-xs text-red-400 hover:text-red-600 shrink-0">
                          Delete
                        </button>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <p class="text-sm text-gray-400">No labs yet.</p>
                {/if}
              </div>

              <!-- ══════════════════════════════════════════
                   LAB FORM (create or edit)
              ══════════════════════════════════════════ -->
              {#if labFormMode && labFormMode.courseId === course.id}
                <div class="bg-white border-2 border-primary-200 rounded-xl p-5">
                  <div class="flex items-center justify-between mb-4">
                    <h4 class="font-semibold text-gray-800 text-base">{labFormTitle()}</h4>
                    <button on:click={() => labFormMode = null} class="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
                  </div>

                  <!-- Base fields -->
                  <div class="grid md:grid-cols-2 gap-3 mb-4">
                    <div>
                      <label class="label">Title *</label>
                      <input class="input" bind:value={labForm.title} placeholder="Lab title" />
                    </div>
                    <div>
                      <label class="label">Type</label>
                      <select class="input" bind:value={labForm.lab_type}
                        on:change={() => { labForm.questions = labForm.questions.length ? labForm.questions : [newQuestion()]; labForm.ctf_flags = labForm.ctf_flags.length ? labForm.ctf_flags : [newMultiFlag()]; }}>
                        <option value="form">📝 Quiz</option>
                        <option value="ctf">🚩 CTF Challenge</option>
                      </select>
                    </div>
                    <div class="md:col-span-2">
                      <label class="label">Description</label>
                      <textarea class="input" rows="2" bind:value={labForm.description} placeholder="Describe this lab..."></textarea>
                    </div>
                    <div>
                      <label class="label">Points</label>
                      <input type="number" class="input" bind:value={labForm.points} min="0" />
                    </div>
                    <div>
                      <label class="label">Order</label>
                      <input type="number" class="input" bind:value={labForm.order_index} min="0" />
                    </div>
                    <div class="flex items-center gap-2 pt-1">
                      <input type="checkbox" id="lab-pub" bind:checked={labForm.is_published} />
                      <label for="lab-pub" class="text-sm text-gray-700">Publish immediately</label>
                    </div>
                  </div>

                  <!-- ─── FORM / QUIZ ─────────────────────────────────────── -->
                  {#if labForm.lab_type === 'form'}
                    <div class="border-t pt-4">
                      <div class="flex items-center justify-between mb-3">
                        <h5 class="text-sm font-semibold text-gray-700">Questions ({labForm.questions.length})</h5>
                        <button class="text-xs btn-secondary" on:click={addQuestion}>+ Add Question</button>
                      </div>

                      <div class="space-y-4">
                        {#each labForm.questions as q, qi (q.id)}
                          <div class="border border-gray-200 rounded-lg p-4 bg-gray-50">
                            <div class="flex items-start justify-between gap-2 mb-3">
                              <span class="text-xs font-mono text-gray-400 mt-1">Q{qi + 1}</span>
                              <div class="flex-1 grid md:grid-cols-3 gap-2">
                                <div class="md:col-span-2">
                                  <label class="label text-xs">Question text *</label>
                                  <input class="input text-sm" bind:value={labForm.questions[qi].text} placeholder="What is...?" />
                                </div>
                                <div>
                                  <label class="label text-xs">Type</label>
                                  <select class="input text-sm" bind:value={labForm.questions[qi].type}
                                    on:change={() => { labForm.questions[qi].correct_answer = ''; labForm.questions = labForm.questions; }}>
                                    <option value="multiple_choice">Multiple choice</option>
                                    <option value="text">Free text</option>
                                  </select>
                                </div>
                              </div>
                              <button class="text-red-400 hover:text-red-600 mt-5 shrink-0 text-lg leading-none"
                                on:click={() => removeQuestion(qi)} title="Remove question">×</button>
                            </div>

                            <!-- Multiple choice options -->
                            {#if q.type === 'multiple_choice'}
                              <div class="mb-3">
                                <label class="label text-xs">Options</label>
                                <div class="space-y-1">
                                  {#each q.options as opt, oi}
                                    <div class="flex items-center gap-2">
                                      <input
                                        type="radio"
                                        name="correct_{q.id}"
                                        value={opt || `__opt${oi}`}
                                        checked={labForm.questions[qi].correct_answer === opt && opt !== ''}
                                        on:change={() => { if (opt.trim()) labForm.questions[qi].correct_answer = opt; labForm.questions = labForm.questions; }}
                                        title="Mark as correct answer"
                                        class="shrink-0 accent-green-600"
                                      />
                                      <input
                                        class="input text-sm flex-1"
                                        bind:value={labForm.questions[qi].options[oi]}
                                        placeholder="Option {oi + 1}"
                                        on:input={() => {
                                          if (labForm.questions[qi].correct_answer === labForm.questions[qi].options[oi]) {
                                            labForm.questions[qi].correct_answer = labForm.questions[qi].options[oi];
                                          }
                                          labForm.questions = labForm.questions;
                                        }}
                                      />
                                      {#if q.options.length > 2}
                                        <button class="text-gray-300 hover:text-red-400 text-lg leading-none"
                                          on:click={() => removeOption(qi, oi)}>×</button>
                                      {/if}
                                    </div>
                                  {/each}
                                </div>
                                <button class="text-xs text-primary-600 hover:underline mt-1" on:click={() => addOption(qi)}>
                                  + Add option
                                </button>
                                {#if labForm.questions[qi].correct_answer}
                                  <p class="text-xs text-green-600 mt-1">✓ Correct: <span class="font-medium">{labForm.questions[qi].correct_answer}</span></p>
                                {:else}
                                  <p class="text-xs text-orange-500 mt-1">Select the correct answer (radio button)</p>
                                {/if}
                              </div>
                            {:else}
                              <div class="mb-3">
                                <label class="label text-xs">Expected answer (exact match, case-insensitive)</label>
                                <input class="input text-sm" bind:value={labForm.questions[qi].correct_answer} placeholder="Correct answer..." />
                              </div>
                            {/if}

                            <div class="grid md:grid-cols-2 gap-2">
                              <div>
                                <label class="label text-xs">Points</label>
                                <input type="number" class="input text-sm" bind:value={labForm.questions[qi].points} min="0" />
                              </div>
                              <div>
                                <label class="label text-xs">Explanation (shown after submit)</label>
                                <input class="input text-sm" bind:value={labForm.questions[qi].explanation} placeholder="Why this answer?" />
                              </div>
                            </div>
                          </div>
                        {/each}
                      </div>

                      {#if labForm.questions.length === 0}
                        <div class="text-center py-4 text-sm text-gray-400 border border-dashed border-gray-200 rounded-lg">
                          No questions yet. Click "+ Add Question".
                        </div>
                      {/if}
                    </div>

                  <!-- ─── CTF ────────────────────────────────────────────── -->
                  {:else}
                    <div class="border-t pt-4 space-y-4">

                      <!-- Mode selector -->
                      <div class="flex gap-3">
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input type="radio" bind:group={labForm.ctf_mode} value="single" class="accent-emerald-600" />
                          <span class="text-sm font-medium text-gray-700">Single flag</span>
                        </label>
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input type="radio" bind:group={labForm.ctf_mode} value="multi" class="accent-emerald-600" />
                          <span class="text-sm font-medium text-gray-700">Multi-flag</span>
                        </label>
                      </div>

                      <!-- Single flag -->
                      {#if labForm.ctf_mode === 'single'}
                        <div>
                          <label class="label">Challenge description *</label>
                          <textarea class="input" rows="4" bind:value={labForm.ctf_challenge}
                            placeholder="Describe the challenge, what the student needs to find..."></textarea>
                        </div>
                        <div class="grid md:grid-cols-2 gap-3">
                          <div>
                            <label class="label">Flag (secret) *</label>
                            <input class="input font-mono" bind:value={labForm.ctf_flag} placeholder="FLAG&#123;your_secret_flag&#125;" />
                          </div>
                          <div>
                            <label class="label">Category</label>
                            <select class="input" bind:value={labForm.ctf_category}>
                              <option value="web">Web</option>
                              <option value="crypto">Crypto</option>
                              <option value="forensics">Forensics</option>
                              <option value="pwn">Pwn</option>
                              <option value="reverse">Reverse</option>
                              <option value="misc">Misc</option>
                            </select>
                          </div>
                        </div>
                        <div>
                          <div class="flex items-center justify-between mb-1">
                            <label class="label mb-0">Hints</label>
                            <button class="text-xs text-primary-600 hover:underline" on:click={addHint}>+ Add hint</button>
                          </div>
                          <div class="space-y-1">
                            {#each labForm.ctf_hints as hint, hi}
                              <div class="flex gap-2">
                                <input class="input text-sm flex-1" bind:value={labForm.ctf_hints[hi]} placeholder="Hint {hi + 1}..." />
                                {#if labForm.ctf_hints.length > 1}
                                  <button class="text-gray-300 hover:text-red-400 text-lg leading-none px-1" on:click={() => removeHint(hi)}>×</button>
                                {/if}
                              </div>
                            {/each}
                          </div>
                        </div>

                      <!-- Multi-flag -->
                      {:else}
                        <div>
                          <label class="label">Instructions</label>
                          <textarea class="input" rows="3" bind:value={labForm.ctf_instructions}
                            placeholder="Overall scenario and instructions for the student..."></textarea>
                        </div>

                        <div>
                          <div class="flex items-center justify-between mb-2">
                            <label class="label mb-0">Flags ({labForm.ctf_flags.length})</label>
                            <button class="text-xs btn-secondary" on:click={addMultiFlag}>+ Add Flag</button>
                          </div>
                          <div class="space-y-3">
                            {#each labForm.ctf_flags as mf, fi (mf.id)}
                              <div class="border border-gray-200 rounded-lg p-3 bg-gray-50">
                                <div class="flex items-start justify-between gap-2 mb-2">
                                  <span class="text-xs font-mono text-gray-400 mt-1">Flag {fi + 1}</span>
                                  {#if labForm.ctf_flags.length > 1}
                                    <button class="text-gray-300 hover:text-red-400 text-lg leading-none ml-auto" on:click={() => removeMultiFlag(fi)}>×</button>
                                  {/if}
                                </div>
                                <div class="grid md:grid-cols-2 gap-2">
                                  <div>
                                    <label class="label text-xs">Name *</label>
                                    <input class="input text-sm" bind:value={labForm.ctf_flags[fi].name} placeholder="e.g. User flag" />
                                  </div>
                                  <div>
                                    <label class="label text-xs">Points</label>
                                    <input type="number" class="input text-sm" bind:value={labForm.ctf_flags[fi].points} min="0" />
                                  </div>
                                  <div>
                                    <label class="label text-xs">Description</label>
                                    <input class="input text-sm" bind:value={labForm.ctf_flags[fi].description} placeholder="What does this flag unlock?" />
                                  </div>
                                  <div>
                                    <label class="label text-xs">Flag value (secret) *</label>
                                    <input class="input text-sm font-mono" bind:value={labForm.ctf_flags[fi].flag} placeholder="FLAG&#123;...&#125;" />
                                  </div>
                                </div>
                              </div>
                            {/each}
                          </div>
                        </div>
                      {/if}
                    </div>
                  {/if}

                  <!-- Save / Cancel -->
                  <div class="flex gap-2 mt-5 pt-4 border-t">
                    <button class="btn-primary" on:click={saveLab} disabled={labFormSaving}>
                      {labFormSaving ? 'Saving...' : labFormMode?.mode === 'edit' ? 'Save Changes' : 'Create Lab'}
                    </button>
                    <button class="btn-secondary" on:click={() => labFormMode = null} disabled={labFormSaving}>
                      Cancel
                    </button>
                  </div>
                </div>
              {/if}

              <!-- ══════════════════════════════════════════
                   ENROLLED STUDENTS
              ══════════════════════════════════════════ -->
              <div class="border-t border-gray-200 pt-4">
                <h3 class="text-sm font-semibold text-gray-600 mb-3">
                  Enrolled Students ({courseEnrollments[course.id]?.length ?? 0})
                </h3>

                {#if courseEnrollments[course.id]?.length}
                  <div class="space-y-1 mb-3">
                    {#each courseEnrollments[course.id] as enrollment}
                      <div class="bg-white rounded-lg border border-gray-100 p-2 flex items-center gap-3">
                        <span class="flex-1 text-sm font-medium text-gray-700">{enrollment.username}</span>
                        <span class="text-xs text-gray-400">{enrollment.email}</span>
                        <span class="text-xs text-gray-300">{new Date(enrollment.enrolled_at).toLocaleDateString()}</span>
                        <button on:click={() => unenrollUser(course.id, enrollment.user_id)}
                          class="text-xs text-red-400 hover:text-red-600">Remove</button>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <p class="text-sm text-gray-400 mb-3">No students enrolled.</p>
                {/if}

                <div class="flex items-center gap-2">
                  <select class="input text-sm flex-1" bind:value={selectedUserId[course.id]}>
                    <option value="">Select a student...</option>
                    {#each allUsers.filter(u => !courseEnrollments[course.id]?.some(e => e.user_id === u.id)) as user}
                      <option value={user.id}>{user.username} ({user.email})</option>
                    {/each}
                  </select>
                  <button class="btn-primary text-xs" disabled={enrollingFor === course.id}
                    on:click={() => enrollUser(course.id)}>
                    {enrollingFor === course.id ? 'Enrolling...' : 'Enroll'}
                  </button>
                </div>
              </div>

            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
