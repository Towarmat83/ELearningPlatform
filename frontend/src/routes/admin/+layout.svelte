<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { auth, isAdmin, isLoggedIn } from '$lib/stores';

  onMount(() => {
    auth.init();
    if (!$isLoggedIn || !$isAdmin) {
      goto('/login');
    }
  });

  const navItems = [
    { href: '/admin', label: 'Dashboard', icon: '📊' },
    { href: '/admin/users', label: 'Users', icon: '👥' },
    { href: '/admin/courses', label: 'Courses', icon: '📚' },
    { href: '/admin/labs', label: 'Lab Tools', icon: '🧪' },
    { href: '/admin/monitoring', label: 'Monitoring', icon: '📈' },
    { href: '/admin/settings', label: 'Settings', icon: '⚙️' },
  ];
</script>

<div class="flex min-h-screen bg-gray-50">
  <!-- Sidebar -->
  <aside class="w-64 bg-gray-900 text-white flex flex-col">
    <div class="p-4 border-b border-gray-700">
      <a href="/" class="font-bold text-xl text-primary-400">LearnLab</a>
      <span class="ml-2 text-xs text-gray-400">Admin</span>
    </div>
    <nav class="flex-1 p-4 space-y-1">
      {#each navItems as item}
        <a href={item.href}
          class="flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors
            {$page.url.pathname === item.href
              ? 'bg-primary-600 text-white'
              : 'text-gray-300 hover:bg-gray-800 hover:text-white'}">
          <span>{item.icon}</span>
          {item.label}
        </a>
      {/each}
    </nav>
    <div class="p-4 border-t border-gray-700">
      <a href="/" class="text-gray-400 text-sm hover:text-white">← Public Site</a>
    </div>
  </aside>

  <!-- Main -->
  <div class="flex-1 overflow-auto">
    <slot />
  </div>
</div>
