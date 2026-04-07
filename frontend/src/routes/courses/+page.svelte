<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { coursesApi, type CourseWithStats } from '$lib/api';
  import { auth, isLoggedIn, toasts } from '$lib/stores';

  let courses: CourseWithStats[] = [];
  let total = 0;
  let loading = true;
  let search = '';
  let difficulty = '';
  let category = '';
  let page = 1;
  const perPage = 12;

  async function loadCourses() {
    loading = true;
    try {
      const res = await coursesApi.list({
        search: search || undefined,
        difficulty: difficulty || undefined,
        category: category || undefined,
        page,
        per_page: perPage,
      });
      courses = res.courses;
      total = res.total;
    } catch (e: any) {
      toasts.error('Failed to load courses');
    } finally {
      loading = false;
    }
  }

  onMount(loadCourses);

  async function enrollAndGo(courseId: string) {
    const token = $auth.token ?? (typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null);
    if (!token) {
      goto(`/login?redirect=/courses/${courseId}`);
      return;
    }
    try {
      await coursesApi.enroll(courseId, token);
      toasts.success('Enrolled successfully!');
    } catch (e: any) {
      if (e.status !== 409) {
        toasts.error(e.message || 'Failed to enroll');
      }
    }
    goto(`/courses/${courseId}`);
  }

  function difficultyColor(d: string | null) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }

  let searchTimeout: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => { page = 1; loadCourses(); }, 400);
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

    <select class="input w-auto" bind:value={difficulty} on:change={() => { page = 1; loadCourses(); }}>
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
    <p class="text-sm text-gray-400 mb-4">{total} courses found</p>
    <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
      {#each courses as course}
        <div class="card hover:shadow-md transition-shadow flex flex-col">
          {#if course.thumbnail}
            <img src={course.thumbnail} alt={course.title}
              class="w-full h-40 object-cover rounded-lg mb-4" />
          {:else}
            <div class="w-full h-40 bg-gradient-to-br from-primary-100 to-blue-200 rounded-lg mb-4 flex items-center justify-center text-5xl">
              📖
            </div>
          {/if}

          <div class="flex-1">
            <div class="flex items-start gap-2 mb-2">
              <h3 class="font-semibold text-gray-900 flex-1 leading-snug">{course.title}</h3>
              {#if course.difficulty}
                <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
              {/if}
            </div>
            <p class="text-sm text-gray-500 line-clamp-2 mb-3">{course.description}</p>

            <div class="flex items-center gap-4 text-xs text-gray-400 mb-4">
              <span>📚 {course.lab_count} labs</span>
              <span>👥 {course.enrollment_count} enrolled</span>
              {#if course.category}
                <span class="badge-blue">{course.category}</span>
              {/if}
            </div>
          </div>

          <div class="flex gap-2 mt-auto">
            <a href="/courses/{course.id}"
              class="btn-secondary text-sm flex-1 text-center">View</a>
            <button on:click={() => enrollAndGo(course.id)}
              class="btn-primary text-sm">
              Enroll
            </button>
          </div>
        </div>
      {/each}
    </div>

    <!-- Pagination -->
    {#if total > perPage}
      <div class="flex justify-center gap-2 mt-8">
        <button class="btn-secondary" disabled={page === 1}
          on:click={() => { page--; loadCourses(); }}>← Prev</button>
        <span class="px-4 py-2 text-sm text-gray-500">
          Page {page} of {Math.ceil(total / perPage)}
        </span>
        <button class="btn-secondary" disabled={page >= Math.ceil(total / perPage)}
          on:click={() => { page++; loadCourses(); }}>Next →</button>
      </div>
    {/if}
  {/if}
</div>
