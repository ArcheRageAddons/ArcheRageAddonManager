<script>
  import { onMount } from 'svelte';
  import { currentUser, currentPage, showNotification } from '../stores/app.js';
  import {
    LoginWithDiscord,
    Logout,
    GetCurrentUser,
  } from '../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
  import Spinner from './Spinner.svelte';

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
    console.log('[login] LoginWithDiscord called');
    try {
      const u = await LoginWithDiscord();
      console.log('[login] returned user:', u);
      currentUser.set(u || null);
      if (u) {
        showNotification(`Logged in as ${u.discord_username || 'user'}`, 'success');
      } else {
        showNotification('Login returned no user — check terminal for [auth] logs', 'error', 10000);
      }
    } catch (e) {
      const msg = (e && e.toString) ? e.toString() : String(e);
      console.error('[login] FAILED:', e);
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

<div class="px-3 pb-3">
  {#if $currentUser}
    <div class="rounded-lg bg-bg-tertiary border border-border p-2.5">
      <div class="flex items-center gap-2 min-w-0">
        <div class="w-7 h-7 rounded-full bg-accent/20 flex items-center justify-center flex-shrink-0 overflow-hidden">
          {#if $currentUser.discord_avatar}
            <img
              src={$currentUser.discord_avatar}
              alt=""
              referrerpolicy="no-referrer"
              class="w-full h-full object-cover"
            />
          {:else}
            <span class="text-accent text-xs font-bold">
              {($currentUser.discord_username || '?').slice(0, 1).toUpperCase()}
            </span>
          {/if}
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-xs font-medium text-text-primary truncate">
            {$currentUser.discord_username || 'unknown'}
          </div>
          {#if $currentUser.is_admin}
            <div class="text-[10px] text-accent uppercase tracking-wide">Admin</div>
          {/if}
        </div>
        <button
          on:click={handleLogout}
          disabled={busy}
          title="Sign out"
          class="text-text-muted hover:text-text-primary disabled:opacity-50 p-1"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>
          </svg>
        </button>
      </div>
    </div>
  {:else}
    <button
      on:click={handleLogin}
      disabled={busy}
      class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg bg-[#5865F2] hover:bg-[#4752C4] text-white text-xs font-medium transition-colors disabled:opacity-60"
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
