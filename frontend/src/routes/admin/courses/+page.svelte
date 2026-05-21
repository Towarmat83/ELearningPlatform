<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, groupsApi, type Course, type CourseEnrollment, type Group } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: Course[] = [];
  let groups: Group[] = [];
  let loading = true;

  // Enrollment panel state
  let expandedId: string | null = null;
  let enrollments: CourseEnrollment[] = [];
  let groupEnrollments: { id: string; name: string; source: string; member_count: number; enrolled_at: string }[] = [];
  let enrollmentsLoading = false;
  let enrollUserId = '';
  let enrollGroupId = '';
  let enrollingGroup = false;

  onMount(async () => {
    auth.init();
    await Promise.all([loadCourses(), loadGroups()]);
  });

  async function loadCourses() {
    loading = true;
    try {
      const res = await adminApi.adminCourses($auth.token!);
      courses = res.courses;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  }

  async function loadGroups() {
    try {
      const res = await groupsApi.list($auth.token!);
      groups = res.groups;
    } catch {}
  }

  async function openEnrollments(id: string) {
    if (expandedId === id) {
      expandedId = null;
      return;
    }
    expandedId = id;
    enrollUserId = '';
    enrollGroupId = '';
    enrollmentsLoading = true;
    try {
      const [userRes, groupRes] = await Promise.all([
        adminApi.listEnrollments(id, $auth.token!),
        adminApi.listGroupEnrollments(id, $auth.token!),
      ]);
      enrollments = userRes.enrollments;
      groupEnrollments = groupRes.groups;
    } catch {
      toasts.error('Failed to load enrollments');
    } finally {
      enrollmentsLoading = false;
    }
  }

  async function enrollUser(courseId: string) {
    if (!enrollUserId.trim()) return;
    try {
      await adminApi.enrollUser(courseId, enrollUserId.trim(), $auth.token!);
      const res = await adminApi.listEnrollments(courseId, $auth.token!);
      enrollments = res.enrollments;
      enrollUserId = '';
      toasts.success('User enrolled');
    } catch (e: any) {
      toasts.error(e.message || 'Enroll failed');
    }
  }

  async function enrollGroup(courseId: string) {
    if (!enrollGroupId) return;
    enrollingGroup = true;
    try {
      const res = await adminApi.enrollGroup(courseId, enrollGroupId, $auth.token!);
      const [userRes, groupRes] = await Promise.all([
        adminApi.listEnrollments(courseId, $auth.token!),
        adminApi.listGroupEnrollments(courseId, $auth.token!),
      ]);
      enrollments = userRes.enrollments;
      groupEnrollments = groupRes.groups;
      toasts.success(`Group enrolled — ${res.enrolled} current member(s) added`);
      enrollGroupId = '';
    } catch (e: any) {
      toasts.error(e.message || 'Group enroll failed');
    } finally {
      enrollingGroup = false;
    }
  }

  async function unenrollGroup(courseId: string, groupId: string) {
    try {
      await adminApi.unenrollGroup(courseId, groupId, $auth.token!);
      groupEnrollments = groupEnrollments.filter(g => g.id !== groupId);
      toasts.success('Group enrollment removed');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to remove group enrollment');
    }
  }

  async function unenrollUser(courseId: string, userId: string) {
    try {
      await adminApi.unenrollUser(courseId, userId, $auth.token!);
      enrollments = enrollments.filter(e => e.user_id !== userId);
    } catch (e: any) {
      toasts.error(e.message || 'Unenroll failed');
    }
  }

  function difficultyColor(d: string) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }

  // Groups not yet enrolled in this course
  $: availableGroups = groups.filter(g => !groupEnrollments.some(ge => ge.id === g.id));
</script>

<svelte:head><title>Courses — Admin</title></svelte:head>

<div class="p-8 max-w-5xl space-y-4">
  <h2 class="text-2xl font-bold text-gray-900">Courses</h2>

  {#if loading}
    <div class="text-center py-10 text-gray-400">Loading…</div>
  {:else if courses.length === 0}
    <div class="bg-white rounded-xl border border-gray-100 p-10 text-center text-gray-400">
      <p class="text-lg mb-2">No courses loaded</p>
      <p class="text-sm">Courses are managed through Kubernetes CRDs.</p>
    </div>
  {:else}
    <div class="space-y-2">
      {#each courses as course}
        <div class="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          <!-- Course row -->
          <div class="flex items-center gap-4 p-4">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-0.5">
                <span class="font-semibold text-gray-900">{course.title}</span>
                <code class="text-xs text-gray-400 bg-gray-100 px-1 rounded">{course.id}</code>
                {#if course.difficulty}
                  <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
                {/if}
                {#if course.category}
                  <span class="badge-blue">{course.category}</span>
                {/if}
                {#if course.enrollment_restricted}
                  <span class="text-xs bg-red-100 text-red-700 px-2 py-0.5 rounded-full font-medium">🔒 Restricted</span>
                {/if}
              </div>
              <p class="text-xs text-gray-400">
                {course.lab_count} lab{course.lab_count !== 1 ? 's' : ''} ·
                {course.enrollment_count} enrolled
              </p>
            </div>
            <div class="flex items-center gap-3 shrink-0">
              {#if course.is_published}
                <span class="badge-green">Published</span>
              {:else}
                <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">Draft</span>
              {/if}
              <button class="btn-secondary text-xs" on:click={() => openEnrollments(course.id)}>
                👥 {expandedId === course.id ? 'Close' : 'Enrollments'}
              </button>
            </div>
          </div>

          <!-- Enrollment panel -->
          {#if expandedId === course.id}
            <div class="border-t border-gray-100 bg-gray-50 p-4 space-y-5">

              <!-- Group enrollments -->
              <div>
                <p class="text-sm font-medium text-gray-700 mb-2">Group access</p>

                {#if enrollmentsLoading}
                  <p class="text-sm text-gray-400">Loading…</p>
                {:else}
                  {#if groupEnrollments.length > 0}
                    <div class="space-y-1 mb-3">
                      {#each groupEnrollments as ge}
                        <div class="flex items-center justify-between bg-white px-3 py-2 rounded-lg border border-gray-100">
                          <span class="text-sm">
                            <span class="font-medium text-gray-800">{ge.name}</span>
                            <span class="text-gray-400 ml-2 text-xs">{ge.member_count} member{ge.member_count !== 1 ? 's' : ''}</span>
                          </span>
                          <button
                            class="text-xs text-red-500 hover:text-red-700"
                            on:click={() => unenrollGroup(course.id, ge.id)}>
                            Remove
                          </button>
                        </div>
                      {/each}
                    </div>
                  {/if}

                  <!-- Add group -->
                  {#if availableGroups.length > 0}
                    <div class="flex gap-2">
                      <select class="input flex-1 text-sm" bind:value={enrollGroupId}>
                        <option value="">— Add a group —</option>
                        {#each availableGroups as g}
                          <option value={g.id}>{g.name} ({g.member_count} members)</option>
                        {/each}
                      </select>
                      <button
                        class="btn-primary text-sm"
                        on:click={() => enrollGroup(course.id)}
                        disabled={!enrollGroupId || enrollingGroup}>
                        {enrollingGroup ? 'Adding…' : 'Add group'}
                      </button>
                    </div>
                  {:else if groups.length > 0}
                    <p class="text-xs text-gray-400">All groups are already enrolled.</p>
                  {:else}
                    <p class="text-xs text-gray-400">No groups available.</p>
                  {/if}
                {/if}
              </div>

              <hr class="border-gray-100" />

              <!-- Individual user enrollment -->
              <div>
                <p class="text-sm font-medium text-gray-700 mb-2">Individual users</p>
                <div class="flex gap-2 mb-3">
                  <input
                    type="text"
                    class="input flex-1 font-mono text-sm"
                    placeholder="User UUID…"
                    bind:value={enrollUserId} />
                  <button
                    class="btn-primary text-sm"
                    on:click={() => enrollUser(course.id)}
                    disabled={!enrollUserId.trim()}>
                    Enroll
                  </button>
                </div>

                {#if !enrollmentsLoading}
                  {#if enrollments.length === 0}
                    <p class="text-sm text-gray-400">No individual enrollments.</p>
                  {:else}
                    <div class="space-y-1">
                      {#each enrollments as e}
                        <div class="flex items-center justify-between bg-white px-3 py-2 rounded-lg border border-gray-100">
                          <span class="text-sm">
                            <span class="font-medium text-gray-800">{e.username}</span>
                            <span class="text-gray-400 ml-2 text-xs">{e.email}</span>
                          </span>
                          <div class="flex items-center gap-3">
                            <span class="text-xs text-gray-400">{new Date(e.enrolled_at).toLocaleDateString()}</span>
                            <button
                              class="text-xs text-red-500 hover:text-red-700"
                              on:click={() => unenrollUser(course.id, e.user_id)}>
                              Remove
                            </button>
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                {/if}
              </div>

            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
