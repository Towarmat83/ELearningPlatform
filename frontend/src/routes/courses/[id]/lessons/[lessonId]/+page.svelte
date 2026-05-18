<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  onMount(() => {
    goto(`/courses/${$page.params.id}`, { replaceState: true });
  });
</script>
