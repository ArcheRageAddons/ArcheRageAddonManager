<script>
  import { onMount } from 'svelte';
  import { currentUser, currentPage, showNotification } from '../../stores/app.js';
  import {
    LoginWithDiscord,
    Logout,
    GetCurrentUser,
  } from '../../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../../wailsjs/runtime/runtime.js';
  import Spinner from '../Spinner.svelte';

  let busy = false;

  onMount(async () => {
    try {
      const u = await GetCurrentUser();
      currentUser.set(u || null);
    } catch (e) {
      console.error('GetCurrentUser failed:', e);
    }
    EventsOn('auth:changed', (u) => currentUser.set(u || null));
  });

  async function handleLogin() {
    busy = true;
    try {
      const u = await LoginWithDiscord();
      currentUser.set(u || null);
      if (u) {
        showNotification(`Logged in as ${u.discord_username || 'user'}`, 'success');
      } else {
        showNotification('Login failed. Try again, or check the log folder from Settings if it keeps happening.', 'error', 10000);
      }
    } catch (e) {
      const msg = (e && e.toString) ? e.toString() : String(e);
      console.error('Login failed:', e);
      // Keep the error toast on screen for 15s so it's actually readable.
      showNotification(`Login failed: ${msg}`, 'error', 15000);
    }
    busy = false;
  }

  async function handleLogout() {
    busy = true;
    try {
      await Logout();
      currentUser.set(null);
      // Bounce off any logged-in-only pages.
      currentPage.update((p) => (p === 'my-addons' || p === 'admin' ? 'browse' : p));
      showNotification('Logged out', 'info');
    } catch (e) {
      showNotification(`Logout failed: ${e}`, 'error');
    }
    busy = false;
  }
</script>

<div class="px-3 pb-2">
  {#if $currentUser}
    <div class="rounded-xl bg-bg-tertiary/60 border border-border p-2.5 hover:border-border-strong transition-colors">
      <div class="flex items-center gap-2.5 min-w-0">
        <div class="relative flex-shrink-0">
          <div class="w-8 h-8 rounded-full bg-accent/20 flex items-center justify-center overflow-hidden ring-2 ring-bg-sidebar">
            {#if $currentUser.discord_avatar}
              <img
                src={$currentUser.discord_avatar}
                alt=""
                referrerpolicy="no-referrer"
                class="w-full h-full object-cover"
              />
            {:else}
              <span class="text-accent text-sm font-bold">
                {($currentUser.discord_username || '?').slice(0, 1).toUpperCase()}
              </span>
            {/if}
          </div>
          <span class="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 bg-success rounded-full ring-2 ring-bg-sidebar"></span>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[12px] font-semibold text-text-primary truncate leading-tight">
            {$currentUser.discord_username || 'unknown'}
          </div>
          <div class="text-[10px] text-text-muted leading-tight mt-0.5">
            {#if $currentUser.is_admin}
              <span class="text-accent font-semibold uppercase tracking-wider">Admin</span>
            {:else}
              Signed in
            {/if}
          </div>
        </div>
        <button
          on:click={handleLogout}
          disabled={busy}
          title="Sign out"
          class="text-text-muted hover:text-warning hover:bg-warning/10 disabled:opacity-50 p-1.5 rounded-md transition-colors"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>
          </svg>
        </button>
      </div>
    </div>
  {:else}
    <button
      on:click={handleLogin}
      disabled={busy}
      class="w-full flex items-center justify-center gap-2 px-3 py-2.5 rounded-xl bg-[#5865F2] hover:bg-[#4752C4] text-white text-[12px] font-semibold transition-all disabled:opacity-60 shadow-soft hover:shadow-lift"
    >
      {#if busy}
        <Spinner size="sm" />
      {:else}
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path d="M19.27 5.33A19.79 19.79 0 0 0 14.39 4l-.21.42a17.84 17.84 0 0 0-4.36 0L9.61 4A19.79 19.79 0 0 0 4.73 5.33C1.94 9.51 1.18 13.58 1.56 17.59a19.94 19.94 0 0 0 6.06 3.06l1.22-1.65a12.93 12.93 0 0 1-1.94-.93l.39-.31a14.16 14.16 0 0 0 11.42 0l.39.31c-.62.36-1.27.67-1.94.93l1.22 1.65a19.94 19.94 0 0 0 6.06-3.06c.41-4.62-.59-8.65-3.17-12.26zM8.84 15.33a2.32 2.32 0 0 1-2.16-2.42 2.31 2.31 0 0 1 2.16-2.42A2.31 2.31 0 0 1 11 12.91a2.32 2.32 0 0 1-2.16 2.42zm6.32 0A2.32 2.32 0 0 1 13 12.91a2.31 2.31 0 0 1 2.16-2.42 2.31 2.31 0 0 1 2.16 2.42 2.32 2.32 0 0 1-2.16 2.42z"/>
        </svg>
      {/if}
      <span>{busy ? 'Connecting...' : 'Sign in with Discord'}</span>
    </button>
  {/if}
</div>
