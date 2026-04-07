<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { coursesApi } from '$lib/api';
  import { auth, currentUser, isLoggedIn, toasts } from '$lib/stores';
  import type { MyCourse } from '$lib/api';

  let myCourses: MyCourse[] = [];
  let loading = true;

  onMount(async () => {
    auth.init();
    if (!$isLoggedIn) {
      goto('/login');
      return;
    }
    try {
      const res = await coursesApi.myCourses($auth.token!);
      myCourses = res.courses;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  });

  function difficultyColor(d: string | null) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }

  $: totalPoints = myCourses.reduce((sum, c) => sum + c.total_score, 0);
  $: totalCompleted = myCourses.reduce((sum, c) => sum + c.completed_labs, 0);
</script>

<svelte:head><title>My Dashboard — LearnLab</title></svelte:head>

<div class="max-w-7xl mx-auto px-6 py-8">
  <!-- Header -->
  <div class="mb-8">
    <h1 class="text-3xl font-bold text-gray-900">
      Welcome back, {$currentUser?.username} 👋
    </h1>
    <p class="text-gray-500 mt-1">Track your learning progress</p>
  </div>

  <!-- Stats cards -->
  <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
    {#each [
      { label: 'Enrolled Courses', value: myCourses.length, icon: '📚', color: 'text-blue-600' },
      { label: 'Labs Completed', value: totalCompleted, icon: '✅', color: 'text-green-600' },
      { label: 'Total Points', value: totalPoints, icon: '⭐', color: 'text-yellow-500' },
      { label: 'Completion Rate', value: myCourses.length > 0 ? Math.round(totalCompleted / myCourses.reduce((s,c) => s+c.lab_count, 0) * 100 || 0) + '%' : '0%', icon: '📈', color: 'text-purple-600' },
    ] as stat}
      <div class="card text-center">
        <div class="text-2xl mb-1">{stat.icon}</div>
        <div class="text-2xl font-bold {stat.color}">{stat.value}</div>
        <div class="text-xs text-gray-500 mt-1">{stat.label}</div>
      </div>
    {/each}
  </div>

  <!-- My Courses -->
  <div class="flex items-center justify-between mb-4">
    <h2 class="text-xl font-semibold">My Courses</h2>
    <a href="/courses" class="btn-primary text-sm">Browse More</a>
  </div>

  {#if loading}
    <div class="text-center py-12 text-gray-400">Loading...</div>
  {:else if myCourses.length === 0}
    <div class="card text-center py-12">
      <p class="text-gray-400 text-lg mb-4">No courses yet!</p>
      <a href="/courses" class="btn-primary">Explore Courses</a>
    </div>
  {:else}
    <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
      {#each myCourses as course}
        {@const pct = course.lab_count > 0
          ? Math.round(course.completed_labs / course.lab_count * 100)
          : 0}
        <a href="/courses/{course.id}" class="card hover:shadow-md transition-shadow cursor-pointer group">
          {#if course.thumbnail}
            <img src={course.thumbnail} alt={course.title}
              class="w-full h-36 object-cover rounded-lg mb-4" />
          {:else}
            <div class="w-full h-36 bg-gradient-to-br from-primary-100 to-primary-200 rounded-lg mb-4 flex items-center justify-center">
              <span class="text-4xl">📚</span>
            </div>
          {/if}

          <div class="flex items-start justify-between gap-2 mb-2">
            <h3 class="font-semibold text-gray-900 group-hover:text-primary-600 transition-colors line-clamp-2">
              {course.title}
            </h3>
            {#if course.difficulty}
              <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
            {/if}
          </div>

          <!-- Progress bar -->
          <div class="mt-3">
            <div class="flex justify-between text-xs text-gray-500 mb-1">
              <span>{course.completed_labs}/{course.lab_count} labs</span>
              <span>{pct}%</span>
            </div>
            <div class="w-full bg-gray-100 rounded-full h-2">
              <div class="bg-primary-500 h-2 rounded-full transition-all"
                style="width: {pct}%"></div>
            </div>
          </div>

          <div class="mt-3 flex items-center justify-between text-sm text-gray-400">
            <span>⭐ {course.total_score} pts</span>
            {#if pct === 100}
              <span class="badge-green">Completed!</span>
            {/if}
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>
