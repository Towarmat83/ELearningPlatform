<script lang="ts">
  import '../app.css';
  import 'highlight.js/styles/github.css';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth, currentUser, isAdmin, isLoggedIn, toasts } from '$lib/stores';

  onMount(() => { auth.init(); });

  function logout() {
    auth.logout();
    goto('/login');
  }

  let reposOpen = false;

  $: isAuthPage = ['/login', '/register'].includes($page.url.pathname);
  $: isActive = (href: string) => $page.url.pathname.startsWith(href);
</script>

{#if !isAuthPage}
  <!-- Navbar -->
  <nav class="bg-white border-b border-gray-200 sticky top-0 z-50 shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">

        <!-- Logo -->
        <a href="/" class="flex items-center gap-2 font-bold text-xl text-primary-600">
          <svg class="w-7 h-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.746 0 3.332.477 4.5 1.253v13C19.832 18.477 18.246 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
          </svg>
          LearnLab
        </a>

        <!-- Nav links -->
        <div class="hidden md:flex items-center gap-1">
          <a href="/courses"
            class="px-3 py-2 rounded-lg text-sm font-medium transition-colors
              {isActive('/courses') ? 'text-primary-600 bg-primary-50' : 'text-gray-600 hover:text-primary-600 hover:bg-gray-50'}">
            Courses
          </a>

          {#if $isLoggedIn}
            <a href="/dashboard"
              class="px-3 py-2 rounded-lg text-sm font-medium transition-colors
                {$page.url.pathname === '/dashboard' ? 'text-primary-600 bg-primary-50' : 'text-gray-600 hover:text-primary-600 hover:bg-gray-50'}">
              My Learning
            </a>

            <!-- Repos dropdown -->
            <div class="relative">
              <button
                on:click={() => reposOpen = !reposOpen}
                on:blur={() => setTimeout(() => reposOpen = false, 150)}
                class="px-3 py-2 rounded-lg text-sm font-medium transition-colors flex items-center gap-1
                  {isActive('/dashboard/repos') ? 'text-primary-600 bg-primary-50' : 'text-gray-600 hover:text-primary-600 hover:bg-gray-50'}">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
                </svg>
                Repos
                <svg class="w-3 h-3 transition-transform {reposOpen ? 'rotate-180' : ''}"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>

              {#if reposOpen}
                <div class="absolute left-0 mt-1 w-56 bg-white rounded-xl shadow-lg border border-gray-100 py-1 z-50">
                  <a href="/dashboard/repos"
                    class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-700 hover:bg-gray-50 transition-colors">
                    <svg class="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                        d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                    </svg>
                    My Repositories
                  </a>
                  <div class="border-t border-gray-100 my-1"></div>
                  <p class="px-4 py-1.5 text-xs text-gray-400">
                    Connect GitHub, GitLab, Gitea<br>or any self-hosted git instance.
                  </p>
                </div>
              {/if}
            </div>

            <a href="/profile"
              class="px-3 py-2 rounded-lg text-sm font-medium transition-colors
                {isActive('/profile') ? 'text-primary-600 bg-primary-50' : 'text-gray-600 hover:text-primary-600 hover:bg-gray-50'}">
              Profile
            </a>

            {#if $isAdmin}
              <a href="/admin"
                class="px-3 py-2 rounded-lg text-sm font-medium transition-colors
                  {isActive('/admin') ? 'text-primary-600 bg-primary-50' : 'text-gray-600 hover:text-primary-600 hover:bg-gray-50'}">
                Admin
              </a>
            {/if}
          {/if}
        </div>

        <!-- Auth -->
        <div class="flex items-center gap-3">
          {#if $isLoggedIn}
            <span class="text-sm text-gray-500 hidden sm:block">
              {$currentUser?.username}
              {#if $isAdmin}
                <span class="badge-red ml-1">Admin</span>
              {/if}
            </span>
            <button on:click={logout} class="btn-secondary text-sm">Logout</button>
          {:else}
            <a href="/login" class="text-gray-600 hover:text-primary-600 font-medium text-sm">Login</a>
            <a href="/register" class="btn-primary text-sm">Sign Up</a>
          {/if}
        </div>

      </div>
    </div>
  </nav>
{/if}

<!-- Main content -->
<main class:pt-0={isAuthPage}>
  <slot />
</main>

<!-- Toast notifications -->
<div class="fixed bottom-4 right-4 z-[100] flex flex-col gap-2">
  {#each $toasts as toast (toast.id)}
    <div class="flex items-center gap-3 px-4 py-3 rounded-lg shadow-lg text-white text-sm max-w-sm animate-pulse-once"
      class:bg-green-500={toast.type === 'success'}
      class:bg-red-500={toast.type === 'error'}
      class:bg-blue-500={toast.type === 'info'}
      class:bg-yellow-500={toast.type === 'warning'}
    >
      {#if toast.type === 'success'}✓{:else if toast.type === 'error'}✕{:else if toast.type === 'info'}ℹ{:else}⚠{/if}
      {toast.message}
      <button on:click={() => toasts.remove(toast.id)} class="ml-auto text-white/80 hover:text-white">✕</button>
    </div>
  {/each}
</div>
