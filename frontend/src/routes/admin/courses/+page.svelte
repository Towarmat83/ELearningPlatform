<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type Course, type CourseEnrollment } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: Course[] = [];
  let loading = true;

  // Enrollment management state
  let expandedId: string | null = null;
  let enrollments: CourseEnrollment[] = [];
  let enrollmentsLoading = false;
  let enrollUserId = '';

  onMount(async () => {
    auth.init();
    await loadCourses();
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

  async function openEnrollments(id: string) {
    if (expandedId === id) {
      expandedId = null;
      return;
    }
    expandedId = id;
    enrollUserId = '';
    enrollmentsLoading = true;
    try {
      const res = await adminApi.listEnrollments(id, $auth.token!);
      enrollments = res.enrollments;
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
      courses = courses.map(c => c.id === courseId
        ? { ...c, enrollment_count: c.enrollment_count + 1 } : c);
      enrollUserId = '';
      toasts.success('User enrolled');
    } catch (e: any) {
      toasts.error(e.message || 'Enroll failed');
    }
  }

  async function unenrollUser(courseId: string, userId: string) {
    try {
      await adminApi.unenrollUser(courseId, userId, $auth.token!);
      enrollments = enrollments.filter(e => e.user_id !== userId);
      courses = courses.map(c => c.id === courseId
        ? { ...c, enrollment_count: Math.max(0, c.enrollment_count - 1) } : c);
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
</script>

<svelte:head><title>Courses — Admin</title></svelte:head>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold text-gray-800">Courses</h2>
  </div>

  {#if loading}
    <div class="text-center py-10 text-gray-400">Loading…</div>
  {:else if courses.length === 0}
    <div class="bg-white rounded-xl border border-gray-100 p-10 text-center text-gray-400">
      <p class="text-lg mb2">No courses loaded</p>
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
              </div>
              <p class="text-xs text-gray-400">
                {course.lab_count} lab{course.lab_count !== 1 ? 's' : ''} ·
                {course.enrollment_count} enrolled
              </p>
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-4 shrink-0">
              {#if course.is_published}
                <span class="badge-green">Published</span>
              {:else}
                <span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">Draft</span>
              {/if}
              <button
                class="btn-secondary text-xs"
                on:click={() => openEnrollments(course.id)}>
                👥 {expandedId === course.id ? 'Close' : 'Enrollments'}
              </button>
            </div>
          </div>

          <!-- Enrollment panel (expanded) -->
          {#if expandedId === course.id}
            <div class="border-t border-gray-100 bg-gray-50 p-4 space-y-4">

              <!-- Add user by ID -->
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1">Enroll a user by ID</label>
                <div class="flex gap-2">
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
              </div>

              <!-- Enrolled users -->
              {#if enrollmentsLoading}
                <p class="text-sm text-gray-400">Loading…</p>
              {:else if enrollments.length === 0}
                <p class="text-sm text-gray-400">No users enrolled in this course yet.</p>
              {:else}
                <div>
                  <p class="text-sm font-medium text-gray-700 mb-2">
                    Enrolled users ({enrollments.length})
                  </p>
                  <div class="space-y-1">
                    {#each enrollments as e}
                      <div class="flex items-center justify-between bg-white px-3 py-2 rounded-lg border border-gray-100">
                        <span class="text-sm">
                          <span class="font-medium text-gray-800">{e.username}</span>
                          <span class="text-gray-400 ml-2 text-xs">{e.email}</span>
                        </span>
                        <div class="flex items-center gap-3">
                          <span class="text-xs text-gray-400">
                            {new Date(e.enrolled_at).toLocaleDateString()}
                          </span>
                          <button
                            class="text-xs text-red-500 hover:text-red-700"
                            on:click={() => unenrollUser(course.id, e.user_id)}>
                            Remove
                          </button>
                        </div>
                      </div>
                    {/each}
                  </div>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
