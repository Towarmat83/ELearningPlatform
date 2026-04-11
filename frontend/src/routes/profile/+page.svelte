<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { authApi, coursesApi, type UserPublic, type MyCourse } from '$lib/api';
  import { auth, currentUser, isLoggedIn, toasts } from '$lib/stores';

  let user: UserPublic | null = null;
  let myCourses: MyCourse[] = [];
  let loading = true;

  // Edit form state
  let editing = false;
  let saving = false;
  let editBio = '';
  let editAvatar = '';

  // Password change state
  let changingPassword = false;
  let oldPassword = '';
  let newPassword = '';
  let newPassword2 = '';
  let savingPassword = false;

  onMount(async () => {
    auth.init();
    if (!$isLoggedIn) { goto('/login'); return; }
    try {
      const [me, courses] = await Promise.all([
        authApi.me($auth.token!),
        coursesApi.myCourses($auth.token!),
      ]);
      user = me;
      myCourses = courses.courses;
      editBio = me.bio ?? '';
      editAvatar = me.avatar_url ?? '';
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load profile');
    } finally {
      loading = false;
    }
  });

  async function saveProfile() {
    saving = true;
    try {
      const updated = await authApi.updateProfile(
        { bio: editBio || undefined, avatar_url: editAvatar || undefined },
        $auth.token!
      );
      user = updated;
      // Update the auth store so the navbar username stays in sync
      auth.setUser(updated);
      editing = false;
      toasts.success('Profile updated');
    } catch (e: any) {
      toasts.error(e.message || 'Update failed');
    } finally {
      saving = false;
    }
  }

  async function savePassword() {
    if (newPassword !== newPassword2) { toasts.error('Passwords do not match'); return; }
    if (newPassword.length < 8) { toasts.error('Password must be at least 8 characters'); return; }
    savingPassword = true;
    try {
      await authApi.changePassword(oldPassword, newPassword, $auth.token!);
      toasts.success('Password changed successfully');
      oldPassword = ''; newPassword = ''; newPassword2 = '';
      changingPassword = false;
    } catch (e: any) {
      toasts.error(e.message || 'Password change failed');
    } finally {
      savingPassword = false;
    }
  }

  $: totalPoints = myCourses.reduce((s, c) => s + c.total_score, 0);
  $: totalCompleted = myCourses.reduce((s, c) => s + c.completed_labs, 0);
  $: totalLabs = myCourses.reduce((s, c) => s + c.lab_count, 0);
</script>

<svelte:head><title>Profile — LearnLab</title></svelte:head>

<div class="max-w-4xl mx-auto px-6 py-8">
  {#if loading}
    <div class="text-center py-16 text-gray-400">Loading...</div>
  {:else if user}
    <div class="grid md:grid-cols-3 gap-6">

      <!-- ═══════════════════════ LEFT: AVATAR + STATS ═══════════════════ -->
      <div class="space-y-4">
        <!-- Avatar card -->
        <div class="card text-center">
          {#if user.avatar_url}
            <img src={user.avatar_url} alt={user.username}
              class="w-24 h-24 rounded-full object-cover mx-auto mb-3 border-2 border-primary-100" />
          {:else}
            <div class="w-24 h-24 rounded-full bg-gradient-to-br from-primary-100 to-primary-300 mx-auto mb-3 flex items-center justify-center text-3xl font-bold text-primary-600">
              {user.username[0].toUpperCase()}
            </div>
          {/if}
          <h2 class="text-lg font-bold text-gray-900">{user.username}</h2>
          <p class="text-sm text-gray-400">{user.email}</p>
          {#if user.role === 'admin'}
            <span class="badge-red mt-1">Admin</span>
          {/if}
          <p class="text-sm text-gray-500 mt-2 italic">{user.bio || 'No bio yet.'}</p>
          <p class="text-xs text-gray-300 mt-2">Member since {new Date(user.created_at).toLocaleDateString('fr-FR', { year: 'numeric', month: 'long' })}</p>
        </div>

        <!-- Stats card -->
        <div class="card">
          <h3 class="text-sm font-semibold text-gray-600 mb-3">My Stats</h3>
          <div class="space-y-3">
            {#each [
              { label: 'Courses', value: myCourses.length, icon: '📚' },
              { label: 'Labs completed', value: `${totalCompleted}/${totalLabs}`, icon: '✅' },
              { label: 'Total points', value: totalPoints, icon: '⭐' },
            ] as stat}
              <div class="flex items-center justify-between">
                <span class="text-sm text-gray-500">{stat.icon} {stat.label}</span>
                <span class="font-semibold text-gray-800">{stat.value}</span>
              </div>
            {/each}
          </div>
        </div>
      </div>

      <!-- ═══════════════════════ RIGHT: EDIT + COURSES ═══════════════════ -->
      <div class="md:col-span-2 space-y-4">

        <!-- Edit profile card -->
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h3 class="font-semibold text-gray-800">Profile Settings</h3>
            {#if !editing}
              <button class="btn-secondary text-sm" on:click={() => editing = true}>Edit</button>
            {/if}
          </div>

          {#if !editing}
            <div class="space-y-2 text-sm">
              <div class="flex gap-3">
                <span class="text-gray-400 w-24">Avatar URL</span>
                <span class="text-gray-700 truncate">{user.avatar_url || '—'}</span>
              </div>
              <div class="flex gap-3">
                <span class="text-gray-400 w-24">Bio</span>
                <span class="text-gray-700">{user.bio || '—'}</span>
              </div>
            </div>
          {:else}
            <div class="space-y-3">
              <div>
                <label class="label">Avatar URL</label>
                <input class="input" bind:value={editAvatar} placeholder="https://example.com/avatar.png" />
              </div>
              <div>
                <label class="label">Bio</label>
                <textarea class="input" rows="3" bind:value={editBio} placeholder="Tell us about yourself..."></textarea>
              </div>
              <div class="flex gap-2">
                <button class="btn-primary text-sm" on:click={saveProfile} disabled={saving}>
                  {saving ? 'Saving...' : 'Save'}
                </button>
                <button class="btn-secondary text-sm" on:click={() => editing = false} disabled={saving}>
                  Cancel
                </button>
              </div>
            </div>
          {/if}
        </div>

        <!-- Password change card -->
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h3 class="font-semibold text-gray-800">Security</h3>
            {#if !changingPassword}
              <button class="btn-secondary text-sm" on:click={() => changingPassword = true}>Change Password</button>
            {/if}
          </div>
          {#if changingPassword}
            <div class="space-y-3">
              <div>
                <label class="label">Current password</label>
                <input type="password" class="input" bind:value={oldPassword} />
              </div>
              <div>
                <label class="label">New password</label>
                <input type="password" class="input" bind:value={newPassword} />
              </div>
              <div>
                <label class="label">Confirm new password</label>
                <input type="password" class="input" bind:value={newPassword2}
                  class:border-red-300={newPassword2 && newPassword !== newPassword2} />
              </div>
              <div class="flex gap-2">
                <button class="btn-primary text-sm" on:click={savePassword} disabled={savingPassword}>
                  {savingPassword ? 'Saving...' : 'Update Password'}
                </button>
                <button class="btn-secondary text-sm" on:click={() => changingPassword = false} disabled={savingPassword}>
                  Cancel
                </button>
              </div>
            </div>
          {:else}
            <p class="text-sm text-gray-400">Use a strong password with at least 8 characters.</p>
          {/if}
        </div>

        <!-- Enrolled courses card -->
        <div class="card">
          <h3 class="font-semibold text-gray-800 mb-4">My Courses ({myCourses.length})</h3>
          {#if myCourses.length === 0}
            <p class="text-sm text-gray-400">No courses yet. <a href="/courses" class="text-primary-600 hover:underline">Browse courses</a></p>
          {:else}
            <div class="space-y-2">
              {#each myCourses as course}
                {@const pct = course.lab_count > 0 ? Math.round(course.completed_labs / course.lab_count * 100) : 0}
                <a href="/courses/{course.id}" class="flex items-center gap-3 p-2 rounded-lg hover:bg-gray-50 transition-colors group">
                  <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-primary-100 to-primary-200 flex items-center justify-center text-sm shrink-0">
                    📚
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium text-gray-800 group-hover:text-primary-600 truncate">{course.title}</div>
                    <div class="flex items-center gap-2 mt-0.5">
                      <div class="flex-1 bg-gray-100 rounded-full h-1.5">
                        <div class="bg-primary-500 h-1.5 rounded-full" style="width:{pct}%"></div>
                      </div>
                      <span class="text-xs text-gray-400 shrink-0">{pct}%</span>
                    </div>
                  </div>
                  <span class="text-xs text-gray-400 shrink-0">⭐ {course.total_score}</span>
                </a>
              {/each}
            </div>
          {/if}
        </div>

      </div>
    </div>
  {/if}
</div>
