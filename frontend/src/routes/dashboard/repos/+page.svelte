<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { isAdmin } from '$lib/stores';

  onMount(() => {
    if ($isAdmin) {
      goto('/admin/repos');
    } else {
      goto('/dashboard');
    }
  });
</script>
