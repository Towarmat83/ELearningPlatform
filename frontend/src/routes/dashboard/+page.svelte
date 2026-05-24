<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { coursesApi } from '$lib/api';
  import { auth, currentUser, isLoggedIn, toasts } from '$lib/stores';
  import type { MyCourse, CoursePrerequisite } from '$lib/api';

  let myCourses: MyCourse[] = [];
  // slug → prerequisites / slug → title (from full catalog)
  let prereqMap   = new Map<string, CoursePrerequisite[]>();
  let courseTitles = new Map<string, string>();
  let loading = true;

  onMount(async () => {
    auth.init();
    if (!$isLoggedIn) { goto('/login'); return; }
    try {
      const [myRes, allRes] = await Promise.all([
        coursesApi.myCourses($auth.token!),
        coursesApi.list(),
      ]);
      myCourses = myRes.courses;
      prereqMap = new Map(
        allRes.courses
          .filter(c => c.prerequisites?.length)
          .map(c => [c.id, c.prerequisites!])
      );
      courseTitles = new Map(allRes.courses.map(c => [c.id, c.title]));
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load courses');
    } finally {
      loading = false;
    }
  });

  // Set of enrolled course slugs (for prerequisite status check)
  $: enrolledIds = new Set(myCourses.map(c => c.id));

  function prereqsFor(courseId: string) {
    return prereqMap.get(courseId) ?? [];
  }
  function allPrereqsMet(courseId: string) {
    return prereqsFor(courseId).every(p => enrolledIds.has(p.course));
  }

  function difficultyColor(d: string) {
    if (d === 'beginner') return 'badge-green';
    if (d === 'intermediate') return 'badge-yellow';
    if (d === 'advanced') return 'badge-red';
    return 'badge-blue';
  }

  $: totalCompleted = myCourses.reduce((sum, c) => sum + c.completed_labs, 0);
  $: totalLabs = myCourses.reduce((sum, c) => sum + c.lab_count, 0);
  $: totalScore = myCourses.reduce((sum, c) => sum + c.total_score, 0);
</script>

<svelte:head><title>My Dashboard — LearnLab</title></svelte:head>

<div class="max-w-7xl mx-auto px-6 py-8">
  <!-- Header -->
  <div class="mb-8 flex items-start justify-between">
    <div>
      <h1 class="text-3xl font-bold text-gray-900">
        Welcome back, {$currentUser?.username} 👋
      </h1>
      <p class="text-gray-500 mt-1">Track your learning progress</p>
    </div>
  </div>

  <!-- Stats -->
  <div class="grid grid-cols-2 md:grid-cols-3 gap-4 mb-8">
    {#each [
      { label: 'Enrolled Courses', value: myCourses.length, icon: '📚', color: 'text-blue-600' },
      { label: 'Labs Completed', value: totalCompleted, icon: '✅', color: 'text-green-600' },
      { label: 'Total Score', value: totalScore, icon: '🏆', color: 'text-purple-600' },
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
        {@const blocked = !allPrereqsMet(course.id)}
        <svelte:element
          this={blocked ? 'div' : 'a'}
          href={blocked ? undefined : `/courses/${course.id}`}
          class="card flex flex-col {blocked ? 'opacity-70 cursor-not-allowed' : 'hover:shadow-md transition-shadow cursor-pointer group'}"
        >
          <div class="w-full h-36 bg-gradient-to-br from-primary-100 to-primary-200 rounded-lg mb-4 flex items-center justify-center">
            <span class="text-4xl">📚</span>
          </div>

          <div class="flex items-start justify-between gap-2 mb-2">
            <h3 class="font-semibold line-clamp-2
              {blocked ? 'text-gray-400' : 'text-gray-900 group-hover:text-primary-600 transition-colors'}">
              {course.title}
            </h3>
            {#if course.difficulty}
              <span class={difficultyColor(course.difficulty)}>{course.difficulty}</span>
            {/if}
          </div>

          {#if prereqsFor(course.id).length > 0}
            <div class="mb-3 text-xs rounded-lg px-3 py-2 border
              {allPrereqsMet(course.id)
                ? 'bg-green-50 border-green-200 text-green-700'
                : 'bg-amber-50 border-amber-200 text-amber-700'}">
              <p class="font-semibold mb-1">
                {allPrereqsMet(course.id) ? '✅ Prerequisites met' : '🔒 Prerequisites required'}
              </p>
              {#each prereqsFor(course.id) as p}
                <div class="flex items-center gap-1.5">
                  <span>{enrolledIds.has(p.course) ? '✓' : '•'}</span>
                  <span class="font-medium">{courseTitles.get(p.course) ?? p.course}</span>
                  {#if p.min_score}
                    <span class="opacity-75">— {p.min_score} pts min</span>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}

          <div class="mt-auto">
            <div class="flex justify-between text-xs text-gray-500 mb-1">
              <span>{course.completed_labs}/{course.lab_count} labs</span>
              <span>{pct}%</span>
            </div>
            <div class="w-full bg-gray-100 rounded-full h-2">
              <div class="bg-primary-500 h-2 rounded-full transition-all" style="width: {pct}%"></div>
            </div>
          </div>

          {#if pct === 100}
            <div class="mt-2"><span class="badge-green">Completed!</span></div>
          {/if}
        </svelte:element>
      {/each}
    </div>
  {/if}
</div>
