<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { coursesApi, type Course, type MyCourse } from '$lib/api';
  import { auth, toasts } from '$lib/stores';

  let courses: Course[] = [];
  let total = 0;
  let loading = true;
  let search = '';
  let difficulty = '';
  let category = '';
  // Maps course id → MyCourse (includes completed_labs and total_score)
  let myCoursesMap = new Map<string, MyCourse>();

  async function loadCourses() {
    loading = true;
    try {
      const res = await coursesApi.list({
        search: search || undefined,
        difficulty: difficulty || undefined,
        category: category || undefined,
      });
      courses = res.courses;
      total = res.total;
    } catch {
      toasts.error('Failed to load courses');
    } finally {
      loading = false;
    }
  }

  async function loadEnrolled() {
    const token = $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
    if (!token) return;
    try {
      const res = await coursesApi.myCourses(token);
      myCoursesMap = new Map(res.courses.map(c => [c.id, c]));
    } catch {
      // non-critical
    }
  }

  onMount(async () => {
    auth.init();
    await Promise.all([loadCourses(), loadEnrolled()]);
  });

  // Build a slug→title lookup from loaded courses
  $: courseMap = new Map(courses.map(c => [c.id, c.title]));

  // A prerequisite course is considered satisfied when the user has actual
  // progress in it (completed_labs > 0 or total_score > 0). Just being
  // enrolled without any progress does NOT satisfy a prerequisite, which
  // matches the backend enforcement in checkCoursePrerequisites.
  function prereqSatisfied(prereqCourseId: string, minScore = 0): boolean {
    const mc = myCoursesMap.get(prereqCourseId);
    if (!mc) return false;
    if (minScore > 0) return (mc.total_score ?? 0) >= minScore;
    return (mc.completed_labs ?? 0) > 0 || (mc.total_score ?? 0) > 0;
  }

  function prereqsMet(course: Course): boolean {
    if (!course.prerequisites?.length) return true;
    return course.prerequisites.every(p => prereqSatisfied(p.course, p.min_score));
  }

  async function enrollAndGo(id: string) {
    const token = $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
    if (!token) {
      goto(`/login?redirect=/courses/${id}`);
      return;
    }
    try {
      await coursesApi.enroll(id, token);
      toasts.success('Enrolled successfully!');
    } catch (e: any) {
      if (e.status !== 409) toasts.error(e.message || 'Failed to enroll');
    }
    goto(`/courses/${id}`);
  }

  function difficultyColor(d: string) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }

  let searchTimeout: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(loadCourses, 400);
  }
</script>

<svelte:head><title>Courses — LearnLab</title></svelte:head>

<div class="max-w-7xl mx-auto px-6 py-8">
  <div class="mb-8">
    <h1 class="text-3xl font-bold text-gray-900">Explore Courses</h1>
    <p class="text-gray-500 mt-1">Find your next challenge</p>
  </div>

  <!-- Filters -->
  <div class="flex flex-wrap gap-3 mb-6">
    <input type="text" class="input max-w-xs" placeholder="Search courses..."
      bind:value={search} on:input={onSearch} />

    <select class="input w-auto" bind:value={difficulty} on:change={loadCourses}>
      <option value="">All Difficulties</option>
      <option value="beginner">Beginner</option>
      <option value="intermediate">Intermediate</option>
      <option value="advanced">Advanced</option>
    </select>

    <input type="text" class="input max-w-xs" placeholder="Category..."
      bind:value={category} on:input={onSearch} />
  </div>

  {#if loading}
    <div class="text-center py-16 text-gray-400 text-lg">Loading courses...</div>
  {:else if courses.length === 0}
    <div class="text-center py-16">
      <p class="text-gray-400 text-xl mb-2">No courses found</p>
      <p class="text-gray-300">Try adjusting your filters</p>
    </div>
  {:else}
    <p class="text-sm text-gray-400 mb-4">{total} course{total !== 1 ? 's' : ''} found</p>
    <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
      {#each courses as course}
        <div class="card hover:shadow-md transition-shadow flex flex-col">
          <div class="w-full h-40 bg-gradient-to-br from-primary-100 to-blue-200 rounded-lg mb-4 flex items-center justify-center text-5xl">
            📖
          </div>

          <div class="flex-1">
            <div class="flex items-start gap-2 mb-2">
              <h3 class="font-semibold text-gray-900 flex-1 leading-snug">{course.title}</h3>
              {#if course.difficulty}
                <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
              {/if}
            </div>
            <p class="text-sm text-gray-500 line-clamp-2 mb-3">{course.description}</p>

            <div class="flex items-center gap-3 text-xs text-gray-400 mb-3">
              <span>📝 {course.lab_count} lab{course.lab_count !== 1 ? 's' : ''}</span>
              {#if course.category}
                <span class="badge-blue">{course.category}</span>
              {/if}
            </div>

            {#if course.prerequisites && course.prerequisites.length > 0}
              <div class="mb-3 text-xs rounded-lg px-3 py-2 border
                {prereqsMet(course)
                  ? 'bg-green-50 border-green-200 text-green-700'
                  : 'bg-amber-50 border-amber-200 text-amber-700'}">
                <p class="font-semibold mb-1">
                  {prereqsMet(course) ? '✅ Prerequisites met' : '🔒 Prerequisites required'}
                </p>
                {#each course.prerequisites as p}
                  {@const title = courseMap.get(p.course) ?? p.course}
                  <div class="flex items-center gap-1.5">
                    <span>{prereqSatisfied(p.course, p.min_score) ? '✓' : '•'}</span>
                    <a href="/courses/{p.course}"
                       class="underline underline-offset-2 hover:opacity-75"
                       on:click|stopPropagation>{title}</a>
                    {#if p.min_score}
                      <span class="opacity-75">— {p.min_score} pts min</span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>

          {#if !course.prerequisites?.length || prereqsMet(course)}
            <div class="flex gap-2 mt-auto">
              {#if myCoursesMap.has(course.id)}
                <a href="/courses/{course.id}" class="btn-primary text-sm flex-1 text-center">Continue</a>
              {:else}
                <button on:click={() => enrollAndGo(course.id)} class="btn-primary text-sm flex-1">
                  Enroll
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
