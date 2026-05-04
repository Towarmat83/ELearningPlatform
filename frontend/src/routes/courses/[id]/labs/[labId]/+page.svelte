<script lang="ts">
  import { page } from '$app/stores';
</script>

<svelte:head><title>Lab — LearnLab</title></svelte:head>

<div class="max-w-3xl mx-auto px-6 py-16 text-center text-gray-400">
  <p class="text-4xl mb-4">🚧</p>
  <p class="text-lg font-medium mb-2">Labs coming soon</p>
  <p class="text-sm mb-6">Labs are being redesigned. Lessons are available now.</p>
  <a href="/courses/{$page.params.id}" class="btn-secondary text-sm">← Back to course</a>
</div>
