<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { adminApi, coursesApi, labsApi, type CourseWithStats, type Lab, type AdminEnrollment, type AdminUser } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: CourseWithStats[] = [];
  let loading = true;
  let creating = false;
  let expandedCourse: string | null = null;
  let courseLabs: Record<string, Lab[]> = {};
  let courseEnrollments: Record<string, AdminEnrollment[]> = {};
  let allUsers: AdminUser[] = [];
  let selectedUserId: Record<string, string> = {};
  let enrollingFor: string | null = null;

  // New course form
  let newCourse = { title: '', description: '', category: '', difficulty: 'beginner', is_published: false };
  // New lab form
  let newLab: Record<string, any> = {};
  let creatingLabFor: string | null = null;

  onMount(async () => {
    // auth is already loaded from localStorage via stores.ts
    if (!$auth.token) { toasts.error('Not authenticated'); return; }
    loadCourses();
    try {
      const res = await adminApi.users($auth.token!);
      allUsers = res.users;
    } catch { /* non-fatal */ }
  });

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

  async function togglePublish(course: CourseWithStats) {
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

  async function loadLabs(courseId: string) {
    if (expandedCourse === courseId) { expandedCourse = null; return; }
    expandedCourse = courseId;
    if (!selectedUserId[courseId]) selectedUserId[courseId] = '';
    if (!courseLabs[courseId]) {
      try {
        const res = await labsApi.list(courseId, $auth.token!);
        courseLabs[courseId] = res.labs;
        courseLabs = courseLabs;
      } catch { courseLabs[courseId] = []; }
    }
    if (!courseEnrollments[courseId]) {
      try {
        const res = await adminApi.courseEnrollments(courseId, $auth.token!);
        courseEnrollments[courseId] = res.enrollments;
        courseEnrollments = courseEnrollments;
      } catch { courseEnrollments[courseId] = []; }
    }
  }

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

  function startCreateLab(courseId: string) {
    creatingLabFor = courseId;
    newLab = {
      title: '', description: '', lab_type: 'form', points: 100,
      is_published: true, flag: '',  // published by default
      content: { questions: [{ id: 'q1', text: '', type: 'multiple_choice', options: ['A','B','C','D'], correct_answer: 'A', points: 100, explanation: '' }] }
    };
  }

  async function createLab(courseId: string) {
    if (!newLab.title) { toasts.error('Lab title required'); return; }
    const payload = {
      title: newLab.title,
      description: newLab.description || '',
      lab_type: newLab.lab_type,
      content: newLab.lab_type === 'ctf'
        ? { challenge: newLab.ctf_challenge || '', category: newLab.ctf_category || 'misc', hints: [newLab.hint || ''].filter(Boolean) }
        : newLab.content,
      flag: newLab.lab_type === 'ctf' ? newLab.flag : undefined,
      points: Number(newLab.points) || 100,
      is_published: newLab.is_published,
    };
    try {
      await labsApi.create(courseId, payload, $auth.token!);
      toasts.success('Lab created');
      creatingLabFor = null;
      delete courseLabs[courseId]; // Force reload
      await loadLabs(courseId);
    } catch (e: any) { toasts.error(e.message); }
  }

  async function deleteLab(courseId: string, labId: string) {
    if (!confirm('Delete this lab?')) return;
    try {
      await labsApi.delete(courseId, labId, $auth.token!);
      courseLabs[courseId] = courseLabs[courseId].filter(l => l.id !== labId);
      courseLabs = courseLabs;
      toasts.success('Lab deleted');
    } catch (e: any) { toasts.error(e.message); }
  }
</script>

<svelte:head><title>Courses — Admin</title></svelte:head>

<div class="p-8">
  <h1 class="text-2xl font-bold text-gray-800 mb-6">Course & Lab Management</h1>

  <!-- Create Course -->
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

  <!-- Course List -->
  {#if loading}
    <div class="text-gray-400">Loading...</div>
  {:else}
    <div class="space-y-3">
      {#each courses as course}
        <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          <!-- Course row -->
          <div class="flex items-center gap-4 p-4">
            <button on:click={() => loadLabs(course.id)}
              class="text-gray-400 hover:text-gray-700 font-mono">
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
            <div class="flex gap-2">
              <button on:click={() => togglePublish(course)}
                class="text-xs btn-secondary">
                {course.is_published ? 'Unpublish' : 'Publish'}
              </button>
              <button on:click={() => deleteCourse(course.id)}
                class="text-xs btn-danger">Delete</button>
            </div>
          </div>

          <!-- Labs -->
          {#if expandedCourse === course.id}
            <div class="border-t border-gray-100 bg-gray-50 p-4">
              <div class="flex items-center justify-between mb-3">
                <h3 class="text-sm font-medium text-gray-600">Labs</h3>
                <button class="text-xs btn-primary" on:click={() => startCreateLab(course.id)}>
                  + Add Lab
                </button>
              </div>

              {#if courseLabs[course.id]?.length}
                <div class="space-y-2 mb-4">
                  {#each courseLabs[course.id] as lab}
                    <div class="bg-white rounded-lg border border-gray-100 p-3 flex items-center gap-3">
                      <span class={lab.lab_type === 'ctf' ? 'badge-ctf' : 'badge-form'}>
                        {lab.lab_type === 'ctf' ? '🚩 CTF' : '📝 Form'}
                      </span>
                      <span class="flex-1 text-sm font-medium text-gray-700">{lab.title}</span>
                      <span class="text-xs text-gray-400">{lab.points} pts</span>
                      <span class={lab.is_published ? 'badge-green' : 'badge-yellow'}>
                        {lab.is_published ? 'Live' : 'Draft'}
                      </span>
                      <button on:click={() => deleteLab(course.id, lab.id)}
                        class="text-xs text-red-400 hover:text-red-600">Delete</button>
                    </div>
                  {/each}
                </div>
              {:else}
                <p class="text-sm text-gray-400 mb-3">No labs yet.</p>
              {/if}

              <!-- New Lab Form -->
              {#if creatingLabFor === course.id}
                <div class="bg-white border border-primary-200 rounded-xl p-4">
                  <h4 class="font-medium text-gray-700 mb-3">New Lab</h4>
                  <div class="grid md:grid-cols-2 gap-3 mb-3">
                    <div>
                      <label class="label">Title *</label>
                      <input class="input" bind:value={newLab.title} placeholder="Lab title" />
                    </div>
                    <div>
                      <label class="label">Type</label>
                      <select class="input" bind:value={newLab.lab_type}>
                        <option value="form">📝 Form (Quiz)</option>
                        <option value="ctf">🚩 CTF Challenge</option>
                      </select>
                    </div>
                    <div class="md:col-span-2">
                      <label class="label">Description</label>
                      <textarea class="input" rows="2" bind:value={newLab.description}></textarea>
                    </div>
                    <div>
                      <label class="label">Points</label>
                      <input type="number" class="input" bind:value={newLab.points} min="0" />
                    </div>
                    <div class="flex items-center gap-2 pt-5">
                      <input type="checkbox" bind:checked={newLab.is_published} />
                      <label class="text-sm">Publish immediately</label>
                    </div>
                  </div>

                  {#if newLab.lab_type === 'ctf'}
                    <div class="space-y-3 border-t pt-3">
                      <div>
                        <label class="label">Challenge Description</label>
                        <textarea class="input" rows="3" bind:value={newLab.ctf_challenge}
                          placeholder="Describe the challenge..."></textarea>
                      </div>
                      <div>
                        <label class="label">Category</label>
                        <select class="input" bind:value={newLab.ctf_category}>
                          <option value="web">Web</option>
                          <option value="crypto">Crypto</option>
                          <option value="forensics">Forensics</option>
                          <option value="pwn">Pwn</option>
                          <option value="misc">Misc</option>
                          <option value="reverse">Reverse</option>
                        </select>
                      </div>
                      <div>
                        <label class="label">Flag (secret) *</label>
                        <input class="input font-mono" bind:value={newLab.flag}
                          placeholder={"FLAG{your_flag_here}"} />
                      </div>
                      <div>
                        <label class="label">Hint (optional)</label>
                        <input class="input" bind:value={newLab.hint} placeholder="A helpful hint..." />
                      </div>
                    </div>
                  {:else}
                    <div class="border-t pt-3">
                      <p class="text-xs text-gray-400 mb-2">Form labs: edit questions in JSON below</p>
                      <label class="label">Questions JSON</label>
                      <textarea class="input font-mono text-xs" rows="8"
                        value={JSON.stringify(newLab.content, null, 2)}
                        on:input={(e) => {
                          try { newLab.content = JSON.parse(e.currentTarget.value); } catch {}
                        }}
                      ></textarea>
                    </div>
                  {/if}

                  <div class="flex gap-2 mt-4">
                    <button class="btn-primary" on:click={() => createLab(course.id)}>Save Lab</button>
                    <button class="btn-secondary" on:click={() => creatingLabFor = null}>Cancel</button>
                  </div>
                </div>
              {/if}

              <!-- Enrolled Students -->
              <div class="mt-4 border-t border-gray-100 pt-4">
                <h3 class="text-sm font-medium text-gray-600 mb-3">Enrolled Students ({courseEnrollments[course.id]?.length ?? 0})</h3>

                {#if courseEnrollments[course.id]?.length}
                  <div class="space-y-1 mb-3">
                    {#each courseEnrollments[course.id] as enrollment}
                      <div class="bg-white rounded-lg border border-gray-100 p-2 flex items-center gap-3">
                        <span class="flex-1 text-sm font-medium text-gray-700">{enrollment.username}</span>
                        <span class="text-xs text-gray-400">{enrollment.email}</span>
                        <span class="text-xs text-gray-300">{new Date(enrollment.enrolled_at).toLocaleDateString()}</span>
                        <button
                          on:click={() => unenrollUser(course.id, enrollment.user_id)}
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
                  <button
                    class="btn-primary text-xs"
                    disabled={enrollingFor === course.id}
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
