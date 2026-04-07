<script lang="ts">
  import { isLoggedIn, isAdmin } from '$lib/stores';
</script>

<svelte:head>
  <title>LearnLab — Practice. Learn. Master.</title>
</svelte:head>

<!-- Hero -->
<div class="bg-gradient-to-br from-primary-600 to-primary-900 text-white">
  <div class="max-w-7xl mx-auto px-6 py-24 text-center">
    <h1 class="text-5xl font-bold mb-6">Learn by Doing</h1>
    <p class="text-xl text-primary-100 mb-8 max-w-2xl mx-auto">
      Hands-on labs, CTF challenges, and structured courses to level up your skills.
    </p>
    <div class="flex gap-4 justify-center">
      <a href="/courses" class="bg-white text-primary-700 font-semibold px-6 py-3 rounded-lg hover:bg-primary-50 transition-colors">
        Browse Courses
      </a>
      {#if !$isLoggedIn}
        <a href="/register" class="border-2 border-white text-white font-semibold px-6 py-3 rounded-lg hover:bg-white/10 transition-colors">
          Get Started Free
        </a>
      {:else}
        <a href="/dashboard" class="border-2 border-white text-white font-semibold px-6 py-3 rounded-lg hover:bg-white/10 transition-colors">
          My Dashboard
        </a>
      {/if}
    </div>
  </div>
</div>

<!-- Features -->
<div class="max-w-7xl mx-auto px-6 py-20">
  <h2 class="text-3xl font-bold text-center mb-12 text-gray-800">Two Ways to Learn</h2>
  <div class="grid md:grid-cols-2 gap-8">
    <!-- Form Labs -->
    <div class="card border-l-4 border-blue-500">
      <div class="flex items-center gap-3 mb-4">
        <div class="p-3 bg-blue-100 rounded-xl">
          <svg class="w-6 h-6 text-blue-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
        </div>
        <h3 class="text-xl font-semibold">Form Labs</h3>
        <span class="badge-form">Quiz</span>
      </div>
      <p class="text-gray-600">
        Structured assessments with multiple choice, text answers, and code exercises. Perfect for testing theoretical knowledge with instant feedback.
      </p>
    </div>

    <!-- CTF Labs -->
    <div class="card border-l-4 border-emerald-500">
      <div class="flex items-center gap-3 mb-4">
        <div class="p-3 bg-emerald-100 rounded-xl">
          <svg class="w-6 h-6 text-emerald-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 21v-4m0 0V5a2 2 0 012-2h6.5l1 1H21l-3 6 3 6h-8.5l-1-1H5a2 2 0 00-2 2zm9-13.5V9" />
          </svg>
        </div>
        <h3 class="text-xl font-semibold">CTF Challenges</h3>
        <span class="badge-ctf">Hacking</span>
      </div>
      <p class="text-gray-600">
        Capture The Flag style challenges like Root-Me. Find the flag, submit it, level up. Real-world scenarios for cybersecurity and problem-solving.
      </p>
    </div>
  </div>

  <!-- Stats -->
  <div class="mt-16 grid grid-cols-2 md:grid-cols-4 gap-6 text-center">
    {#each [
      { label: 'Lab Types', value: '2', icon: '🧪' },
      { label: 'Difficulty Levels', value: '3', icon: '📊' },
      { label: 'Instant Feedback', value: '✓', icon: '⚡' },
      { label: 'Progress Tracking', value: '✓', icon: '📈' },
    ] as stat}
      <div class="card text-center">
        <div class="text-3xl mb-2">{stat.icon}</div>
        <div class="text-2xl font-bold text-primary-600">{stat.value}</div>
        <div class="text-sm text-gray-500 mt-1">{stat.label}</div>
      </div>
    {/each}
  </div>
</div>
