<script lang="ts">
  import { onMount } from 'svelte';
  import { slide } from 'svelte/transition';
  import { adminApi, modulesApi, groupsApi, type Course, type CourseEnrollment, type Group, type ModuleSummary } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: Course[] = [];
  let groups: Group[] = [];
  let loading = true;

  let openId: string | null = null;
  let activeTab: 'modules' | 'enrollments' = 'modules';

  // Modules panel state
  let courseModules: ModuleSummary[] = [];
  let modulesLoading = false;
  let clearingCourseCache = false;
  let clearingModuleIdx: number | null = null;
  let expandedModuleCount: Record<string, number> = {};

  // Enrollments panel state
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

  function toggle(id: string) {
    if (openId === id) { openId = null; return; }
    openId = id;
    activeTab = 'modules';
    enrollUserId = '';
    enrollGroupId = '';
    loadModules(id);
    loadEnrollments(id);
  }

  function switchTab(tab: 'modules' | 'enrollments') {
    activeTab = tab;
  }

  async function loadModules(id: string) {
    modulesLoading = true;
    courseModules = [];
    try {
      const res = await modulesApi.list(id, $auth.token!);
      courseModules = res.modules;
      expandedModuleCount[id] = res.modules.length;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load modules');
    } finally {
      modulesLoading = false;
    }
  }

  async function loadEnrollments(id: string) {
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

  async function clearCourseCache(courseId: string) {
    clearingCourseCache = true;
    try {
      const res = await adminApi.clearCourseCache(courseId, $auth.token!);
      toasts.success(`Cache cleared — ${res.repos_cleared} repo(s) invalidated`);
      await loadModules(courseId);
    } catch (e: any) {
      toasts.error(e.message || 'Failed to clear cache');
    } finally {
      clearingCourseCache = false;
    }
  }

  async function clearModuleCache(courseId: string, index: number) {
    clearingModuleIdx = index;
    try {
      await adminApi.clearModuleCache(courseId, index, $auth.token!);
      toasts.success('Module cache cleared');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to clear module cache');
    } finally {
      clearingModuleIdx = null;
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

  async function unenrollGroup(courseId: string, groupId: string, groupName: string) {
    if (!confirm(`Remove group "${groupName}" from this course?`)) return;
    try {
      await adminApi.unenrollGroup(courseId, groupId, $auth.token!);
      groupEnrollments = groupEnrollments.filter(g => g.id !== groupId);
      toasts.success('Group enrollment removed');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to remove group enrollment');
    }
  }

  async function unenrollUser(courseId: string, userId: string, username: string) {
    if (!confirm(`Remove "${username}" from this course?`)) return;
    try {
      await adminApi.unenrollUser(courseId, userId, $auth.token!);
      enrollments = enrollments.filter(e => e.user_id !== userId);
      toasts.success('User removed');
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

  function moduleTypeBadge(type: string) {
    switch (type) {
      case 'text':  return 'bg-blue-100 text-blue-700';
      case 'quiz':  return 'bg-purple-100 text-purple-700';
      case 'video': return 'bg-green-100 text-green-700';
      case 'image': return 'bg-orange-100 text-orange-700';
      default:      return 'bg-gray-100 text-gray-600';
    }
  }

  function commonRepo(modules: ModuleSummary[]): { src: string; ref: string } | null {
    const withGit = modules.filter(m => m.src);
    if (withGit.length === 0) return null;
    const { src, ref } = withGit[0];
    return withGit.every(m => m.src === src && m.ref === ref) ? { src: src!, ref: ref! } : null;
  }

  $: availableGroups = groups.filter(g => !groupEnrollments.some(ge => ge.id === g.id));
  $: panelCommonRepo = commonRepo(courseModules);
  $: enrollmentCount = enrollments.length + groupEnrollments.length;
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

          <!-- Course row — accordion trigger -->
          <button
            class="w-full flex items-center gap-4 p-4 text-left hover:bg-gray-50 transition-colors"
            on:click={() => toggle(course.id)}
            aria-expanded={openId === course.id}>
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
              <p class="text-xs">
                {#if course.is_public}
                  <span class="text-green-600">Public</span>
                {:else}
                  <span class="text-gray-400">Private</span>
                {/if}
              </p>
              {#if course.prerequisites && course.prerequisites.length > 0}
                <div class="flex items-center gap-1.5 mt-1 flex-wrap">
                  <span class="text-xs text-amber-600 font-medium">Requires:</span>
                  {#each course.prerequisites as p}
                    <span class="text-xs bg-amber-50 border border-amber-200 text-amber-700 px-2 py-0.5 rounded-full font-mono">
                      {p.course}{p.min_score ? ` ≥${p.min_score}%` : ''}
                    </span>
                  {/each}
                </div>
              {/if}
            </div>
            <svg
              class="w-5 h-5 text-gray-400 shrink-0 transition-transform duration-200 {openId === course.id ? 'rotate-180' : ''}"
              viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>

          <!-- Expanded panel -->
          {#if openId === course.id}
            <div transition:slide={{ duration: 200 }} class="border-t border-gray-100">

              <!-- Tab bar -->
              <div class="flex border-b border-gray-200 bg-white px-4">
                <button
                  class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px {activeTab === 'modules'
                    ? 'border-blue-500 text-blue-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
                  on:click|stopPropagation={() => switchTab('modules')}>
                  Modules
                  {#if !modulesLoading && openId === course.id}
                    <span class="ml-1.5 text-xs px-1.5 py-0.5 rounded-full {activeTab === 'modules' ? 'bg-blue-100 text-blue-600' : 'bg-gray-100 text-gray-500'}">
                      {courseModules.length}
                    </span>
                  {/if}
                </button>
                <button
                  class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px {activeTab === 'enrollments'
                    ? 'border-blue-500 text-blue-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
                  on:click|stopPropagation={() => switchTab('enrollments')}>
                  Enrollments
                  {#if !enrollmentsLoading && openId === course.id}
                    <span class="ml-1.5 text-xs px-1.5 py-0.5 rounded-full {activeTab === 'enrollments' ? 'bg-blue-100 text-blue-600' : 'bg-gray-100 text-gray-500'}">
                      {enrollmentCount}
                    </span>
                  {/if}
                </button>
              </div>

              <!-- ── Modules tab ─────────────────────────────────────── -->
              {#if activeTab === 'modules'}
                <div class="bg-gray-50 p-5 space-y-4">
                  <div class="flex items-start justify-between gap-4 flex-wrap">
                    <div>
                      {#if panelCommonRepo}
                        <div class="flex items-center gap-2">
                          <svg class="w-4 h-4 text-gray-400 shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                            <path d="M23.546 10.93L13.067.452c-.604-.603-1.582-.603-2.188 0L8.708 2.627l2.76 2.76c.645-.215 1.379-.07 1.889.441.516.515.658 1.258.438 1.9l2.658 2.66c.645-.223 1.387-.078 1.9.435.721.72.721 1.884 0 2.604-.719.719-1.881.719-2.6 0-.539-.541-.674-1.337-.404-1.996L12.86 8.955v6.525c.176.086.342.203.488.348.713.721.713 1.883 0 2.6-.719.721-1.889.721-2.609 0-.719-.719-.719-1.879 0-2.598.182-.18.387-.316.605-.406V8.835c-.217-.091-.424-.222-.604-.401-.545-.545-.676-1.342-.396-2.009L7.636 3.7.45 10.881c-.6.605-.6 1.584 0 2.189l10.48 10.477c.604.604 1.582.604 2.186 0l10.43-10.43c.605-.603.605-1.582 0-2.187"/>
                          </svg>
                          <a
                            href={panelCommonRepo.src}
                            target="_blank"
                            rel="noopener"
                            class="text-sm font-mono text-gray-600 hover:text-blue-600 hover:underline truncate"
                            on:click|stopPropagation>
                            {panelCommonRepo.src}
                          </a>
                          <span class="text-gray-300">@</span>
                          <span class="text-sm font-mono font-semibold text-gray-800 bg-white border border-gray-200 px-2 py-0.5 rounded shrink-0">
                            {panelCommonRepo.ref}
                          </span>
                        </div>
                      {/if}
                    </div>
                    <button
                      class="btn-secondary text-xs shrink-0"
                      on:click|stopPropagation={() => clearCourseCache(course.id)}
                      disabled={clearingCourseCache}
                      title="Invalidate all git caches used by this course">
                      {clearingCourseCache ? 'Clearing…' : '🗑 Clear all caches'}
                    </button>
                  </div>

                  {#if modulesLoading}
                    <p class="text-sm text-gray-400">Loading modules…</p>
                  {:else if courseModules.length === 0}
                    <p class="text-sm text-gray-400">No modules found.</p>
                  {:else}
                    <div class="space-y-1.5">
                      {#each courseModules as mod}
                        <div class="bg-white rounded-lg border border-gray-100 px-3 py-2.5">
                          <div class="flex items-start gap-3">
                            <span class="text-xs text-gray-400 font-mono mt-1 w-5 shrink-0 text-right">{mod.index}</span>
                            <div class="flex-1 min-w-0 space-y-1">
                              <div class="flex items-center gap-2 flex-wrap">
                                <span class="text-sm font-medium text-gray-800">{mod.name}</span>
                                <span class="text-xs px-1.5 py-0.5 rounded font-medium {moduleTypeBadge(mod.type)}">{mod.type}</span>
                                {#if mod.hidden}
                                  <span class="text-xs bg-red-100 text-red-600 px-1.5 py-0.5 rounded">hidden</span>
                                {/if}
                              </div>
                              <code class="text-xs text-gray-400">{mod.slug}</code>
                              {#if mod.prerequisites && mod.prerequisites.length > 0}
                                <div class="flex items-center gap-1 flex-wrap">
                                  <span class="text-xs text-gray-400">requires:</span>
                                  {#each mod.prerequisites as prereq}
                                    <code class="text-xs bg-amber-50 text-amber-700 px-1 rounded">{prereq}</code>
                                  {/each}
                                </div>
                              {/if}
                              {#if mod.path}
                                <p class="text-sm text-gray-500 font-mono">
                                  {mod.path}
                                  {#if mod.src && (!panelCommonRepo || mod.src !== panelCommonRepo.src || mod.ref !== panelCommonRepo.ref)}
                                    <span class="text-gray-300 mx-1">·</span>
                                    <span class="text-gray-400">{mod.src} @ {mod.ref}</span>
                                  {/if}
                                </p>
                              {/if}
                            </div>
                            {#if mod.src}
                              <button
                                class="shrink-0 text-xs text-gray-400 hover:text-red-500 border border-gray-200 hover:border-red-200 rounded px-2 py-1 transition-colors whitespace-nowrap"
                                aria-label="Clear git cache for {mod.name}"
                                title="Clears the git cache for this module's repo — other modules sharing the same repo will also be refreshed"
                                on:click|stopPropagation={() => clearModuleCache(course.id, mod.index)}
                                disabled={clearingModuleIdx === mod.index}>
                                {clearingModuleIdx === mod.index ? 'Clearing…' : 'Clear cache'}
                              </button>
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}

              <!-- ── Enrollments tab ─────────────────────────────────── -->
              {#if activeTab === 'enrollments'}
                <div class="p-5 space-y-5">

                  <!-- Groups -->
                  <div>
                    <p class="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">Groups</p>
                    {#if enrollmentsLoading}
                      <p class="text-sm text-gray-400">Loading…</p>
                    {:else}
                      {#if groupEnrollments.length > 0}
                        <div class="space-y-1 mb-3">
                          {#each groupEnrollments as ge}
                            <div class="flex items-center justify-between bg-gray-50 px-3 py-2 rounded-lg border border-gray-100">
                              <span class="text-sm">
                                <span class="font-medium text-gray-800">{ge.name}</span>
                                <span class="text-gray-400 ml-2 text-xs">{ge.member_count} member{ge.member_count !== 1 ? 's' : ''}</span>
                              </span>
                              <button
                                class="text-xs text-red-500 hover:text-red-700"
                                on:click={() => unenrollGroup(course.id, ge.id, ge.name)}>
                                Remove
                              </button>
                            </div>
                          {/each}
                        </div>
                      {/if}
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
                        <p class="text-xs text-gray-400">All groups already enrolled.</p>
                      {:else}
                        <p class="text-xs text-gray-400">No groups available.</p>
                      {/if}
                    {/if}
                  </div>

                  <hr class="border-gray-100" />

                  <!-- Individual users -->
                  <div>
                    <p class="text-xs font-medium text-gray-500 uppercase tracking-wide mb-1">Individual users</p>
                    <p class="text-xs text-gray-400 mb-2">Find the user ID on the Users admin page.</p>
                    <div class="flex gap-2 mb-3">
                      <input
                        type="text"
                        class="input flex-1 font-mono text-sm"
                        placeholder="User ID (UUID)…"
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
                            <div class="flex items-center justify-between bg-gray-50 px-3 py-2 rounded-lg border border-gray-100">
                              <span class="text-sm">
                                <span class="font-medium text-gray-800">{e.username}</span>
                                <span class="text-gray-400 ml-2 text-xs">{e.email}</span>
                              </span>
                              <div class="flex items-center gap-3">
                                <span class="text-xs text-gray-400">{new Date(e.enrolled_at).toLocaleDateString()}</span>
                                <button
                                  class="text-xs text-red-500 hover:text-red-700"
                                  on:click={() => unenrollUser(course.id, e.user_id, e.username)}>
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
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
