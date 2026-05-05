<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminCourse, type CourseEnrollment, type UserSearchResult } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: AdminCourse[] = [];
  let loading = true;

  // Enrollment management state (one course expanded at a time)
  let expandedSlug: string | null = null;
  let enrollments: CourseEnrollment[] = [];
  let enrollmentsLoading = false;
  let searchQuery = '';
  let searchResults: UserSearchResult[] = [];
  let searching = false;
  let searchTimeout: ReturnType<typeof setTimeout>;

  onMount(async () => {
    auth.init();
    await loadCourses();
  });

  async function loadCourses() {
    loading = true;
    try {
      const res = await adminApi.courses($auth.token!);
      courses = res.courses;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  }

  async function togglePublished(slug: string, value: boolean) {
    try {
      await adminApi.updateCourseSettings(slug, { is_published: value }, $auth.token!);
      courses = courses.map(c => c.slug === slug ? { ...c, is_published: value } : c);
    } catch (e: any) {
      toasts.error(e.message || 'Update failed');
    }
  }

  async function toggleAutoEnroll(slug: string, value: boolean) {
    try {
      await adminApi.updateCourseSettings(slug, { auto_enroll: value }, $auth.token!);
      courses = courses.map(c => c.slug === slug ? { ...c, auto_enroll: value } : c);
    } catch (e: any) {
      toasts.error(e.message || 'Update failed');
    }
  }

  async function openEnrollments(slug: string) {
    if (expandedSlug === slug) {
      expandedSlug = null;
      return;
    }
    expandedSlug = slug;
    searchQuery = '';
    searchResults = [];
    enrollmentsLoading = true;
    try {
      const res = await adminApi.listEnrollments(slug, $auth.token!);
      enrollments = res.enrollments;
    } catch {
      toasts.error('Failed to load enrollments');
    } finally {
      enrollmentsLoading = false;
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimeout);
    if (searchQuery.trim().length < 2) { searchResults = []; return; }
    searchTimeout = setTimeout(async () => {
      searching = true;
      try {
        const res = await adminApi.searchUsers(searchQuery, $auth.token!);
        searchResults = res.users;
      } catch {} finally {
        searching = false;
      }
    }, 300);
  }

  async function enrollUser(userId: string) {
    if (!expandedSlug) return;
    try {
      await adminApi.enrollUser(expandedSlug, userId, $auth.token!);
      const res = await adminApi.listEnrollments(expandedSlug, $auth.token!);
      enrollments = res.enrollments;
      courses = courses.map(c => c.slug === expandedSlug
        ? { ...c, enrollment_count: c.enrollment_count + 1 } : c);
      searchQuery = '';
      searchResults = [];
      toasts.success('User enrolled');
    } catch (e: any) {
      toasts.error(e.message || 'Enroll failed');
    }
  }

  async function unenrollUser(userId: string) {
    if (!expandedSlug) return;
    try {
      await adminApi.unenrollUser(expandedSlug, userId, $auth.token!);
      enrollments = enrollments.filter(e => e.user_id !== userId);
      courses = courses.map(c => c.slug === expandedSlug
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
    <a href="/admin/repos" class="btn-secondary text-sm">⎇ Manage Git Repos</a>
  </div>

  {#if loading}
    <div class="text-center py-10 text-gray-400">Loading…</div>
  {:else if courses.length === 0}
    <div class="bg-white rounded-xl border border-gray-100 p-10 text-center text-gray-400">
      <p class="text-lg mb-2">No courses loaded</p>
      <p class="text-sm">Add files to <code>courses/</code> or sync a git repo.</p>
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
                <code class="text-xs text-gray-400 bg-gray-100 px-1 rounded">{course.slug}</code>
                {#if course.difficulty}
                  <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
                {/if}
                {#if course.category}
                  <span class="badge-blue">{course.category}</span>
                {/if}
                {#if course.source && course.source !== 'local'}
                  <span class="text-xs text-gray-400" title={course.source}>⎇ git</span>
                {:else}
                  <span class="text-xs text-gray-400">📁 local</span>
                {/if}
              </div>
              <p class="text-xs text-gray-400">
                {course.lesson_count} lesson{course.lesson_count !== 1 ? 's' : ''} ·
                {course.enrollment_count} enrolled
              </p>
            </div>

            <!-- Toggles -->
            <div class="flex items-center gap-4 shrink-0">
              <label class="flex items-center gap-2 cursor-pointer" title="Published">
                <span class="text-xs text-gray-500">Published</span>
                <button
                  role="switch"
                  aria-checked={course.is_published}
                  on:click={() => togglePublished(course.slug, !course.is_published)}
                  class="relative inline-flex h-5 w-9 rounded-full transition-colors
                    {course.is_published ? 'bg-green-500' : 'bg-gray-200'}">
                  <span class="inline-block h-4 w-4 rounded-full bg-white shadow transform transition-transform mt-0.5
                    {course.is_published ? 'translate-x-4' : 'translate-x-0.5'}"></span>
                </button>
              </label>
              <label class="flex items-center gap-2 cursor-pointer" title="Auto-enroll: anyone can self-enroll">
                <span class="text-xs text-gray-500">Auto-enroll</span>
                <button
                  role="switch"
                  aria-checked={course.auto_enroll}
                  on:click={() => toggleAutoEnroll(course.slug, !course.auto_enroll)}
                  class="relative inline-flex h-5 w-9 rounded-full transition-colors
                    {course.auto_enroll ? 'bg-primary-500' : 'bg-gray-200'}">
                  <span class="inline-block h-4 w-4 rounded-full bg-white shadow transform transition-transform mt-0.5
                    {course.auto_enroll ? 'translate-x-4' : 'translate-x-0.5'}"></span>
                </button>
              </label>
              <button
                class="btn-secondary text-xs"
                on:click={() => openEnrollments(course.slug)}>
                👥 {expandedSlug === course.slug ? 'Close' : 'Enrollments'}
              </button>
            </div>
          </div>

          <!-- Enrollment panel (expanded) -->
          {#if expandedSlug === course.slug}
            <div class="border-t border-gray-100 bg-gray-50 p-4 space-y-4">

              <!-- Add user -->
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1">Enroll a user</label>
                <div class="relative">
                  <input
                    type="text"
                    class="input w-full"
                    placeholder="Search by username or email…"
                    bind:value={searchQuery}
                    on:input={onSearchInput} />
                  {#if searching}
                    <span class="absolute right-3 top-2.5 text-gray-400 text-xs">Searching…</span>
                  {/if}
                </div>
                {#if searchResults.length > 0}
                  <ul class="mt-1 bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
                    {#each searchResults as u}
                      <li>
                        <button
                          class="w-full flex items-center justify-between px-3 py-2 hover:bg-gray-50 text-sm"
                          on:click={() => enrollUser(u.id)}>
                          <span>
                            <span class="font-medium text-gray-800">{u.username}</span>
                            <span class="text-gray-400 ml-2">{u.email}</span>
                          </span>
                          <span class="text-primary-600 text-xs">+ Enroll</span>
                        </button>
                      </li>
                    {/each}
                  </ul>
                {/if}
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
                            on:click={() => unenrollUser(e.user_id)}>
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
