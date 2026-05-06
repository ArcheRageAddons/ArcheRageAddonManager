<script>
  import { onMount } from 'svelte';
  import { currentPage, currentUser } from '../stores/app.js';
  import { GetVersion } from '../../../wailsjs/go/main/App.js';
  import AccountPanel from './AccountPanel.svelte';

  let version = '';
  onMount(async () => {
    try { version = await GetVersion(); } catch { version = ''; }
  });
</script>

<aside class="w-44 bg-bg-sidebar flex flex-col h-full border-r border-border">
  <!-- Logo -->
  <div class="p-4 flex items-center gap-2">
    <img src="/logo.png" alt="ArcheRage" class="w-8 h-8" />
    <span class="text-lg font-bold text-text-primary">ArcheRage</span>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 px-2 py-4">
    <button
      on:click={() => currentPage.set('browse')}
      class="w-full flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors mb-1 {$currentPage === 'browse' ? 'bg-accent text-white' : 'text-text-secondary hover:bg-bg-tertiary hover:text-text-primary'}"
    >
      <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
      </svg>
      <span class="text-sm font-medium">Addons</span>
    </button>

    <button
      on:click={() => currentPage.set('installed')}
      class="w-full flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors mb-1 {$currentPage === 'installed' ? 'bg-accent text-white' : 'text-text-secondary hover:bg-bg-tertiary hover:text-text-primary'}"
    >
      <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
      </svg>
      <span class="text-sm font-medium">Installed</span>
    </button>

    {#if $currentUser}
      <button
        on:click={() => currentPage.set('my-addons')}
        class="w-full flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors mb-1 {$currentPage === 'my-addons' ? 'bg-accent text-white' : 'text-text-secondary hover:bg-bg-tertiary hover:text-text-primary'}"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <path d="M14 2v6h6M12 18v-6M9 15h6"/>
        </svg>
        <span class="text-sm font-medium">My Addons</span>
      </button>
    {/if}

    {#if $currentUser?.is_admin}
      <button
        on:click={() => currentPage.set('admin')}
        class="w-full flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors mb-1 {$currentPage === 'admin' ? 'bg-accent text-white' : 'text-text-secondary hover:bg-bg-tertiary hover:text-text-primary'}"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M9 12l2 2 4-4M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z"/>
        </svg>
        <span class="text-sm font-medium">Admin</span>
      </button>
    {/if}

    <button
      on:click={() => currentPage.set('settings')}
      class="w-full flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors mb-1 {$currentPage === 'settings' ? 'bg-accent text-white' : 'text-text-secondary hover:bg-bg-tertiary hover:text-text-primary'}"
    >
      <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="3"/>
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
      </svg>
      <span class="text-sm font-medium">Settings</span>
    </button>
  </nav>

  <!-- Account -->
  <AccountPanel />

  <!-- Version -->
  <div class="px-4 pb-4 text-xs text-text-muted">
    {version || '...'}
  </div>
</aside>
