<script>
  import { onMount } from 'svelte';
  import { currentPage, currentUser } from '../../stores/app.js';
  import { GetVersion } from '../../../../wailsjs/go/main/App.js';
  import AccountPanel from './AccountPanel.svelte';

  let version = '';
  onMount(async () => {
    try { version = await GetVersion(); } catch { version = ''; }
  });

  $: navItems = [
    { id: 'browse',     label: 'Browse',     iconPath: 'M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z' },
    { id: 'installed',  label: 'Installed',  iconPath: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3' },
    $currentUser ? { id: 'my-addons', label: 'My Addons', iconPath: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z|M14 2v6h6M12 18v-6M9 15h6' } : null,
    $currentUser?.is_admin ? { id: 'admin', label: 'Admin', iconPath: 'M9 12l2 2 4-4M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z' } : null,
    { id: 'changelog',  label: 'Changelog',  iconPath: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z|M14 2v6h6M8 13h8M8 17h5' },
  ].filter(Boolean);
</script>

<aside class="w-52 bg-sidebar-grad flex flex-col h-full border-r border-border">
  <!-- Logo / brand -->
  <div class="px-4 pt-5 pb-4">
    <div class="flex items-center gap-2.5">
      <div class="relative">
        <div class="absolute inset-0 bg-accent/30 rounded-lg blur-md"></div>
        <img src="/logo.png" alt="ArcheRage" class="w-9 h-9 relative rounded-lg" />
      </div>
      <div class="min-w-0">
        <div class="text-[15px] font-bold text-text-primary leading-tight">ArcheRage</div>
        <div class="text-[10px] uppercase tracking-[0.15em] text-text-muted leading-tight">Addon Manager</div>
      </div>
    </div>
  </div>

  <div class="mx-3 h-px bg-border"></div>

  <!-- Navigation -->
  <nav class="flex-1 px-3 pt-3">
    <div class="text-[10px] uppercase tracking-[0.15em] text-text-muted font-semibold px-3 mb-2">Library</div>
    {#each navItems as item (item.id)}
      {@const active = $currentPage === item.id}
      <button
        on:click={() => currentPage.set(item.id)}
        class="w-full flex items-center gap-3 px-3 py-2 rounded-lg transition-all mb-0.5 relative {active ? 'bg-accent/15 text-accent nav-active-pill' : 'text-text-secondary hover:bg-bg-tertiary/60 hover:text-text-primary'}"
      >
        <svg class="w-[18px] h-[18px] flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          {#each item.iconPath.split('|') as p}
            <path d={p}/>
          {/each}
        </svg>
        <span class="text-[13px] font-medium">{item.label}</span>
      </button>
    {/each}
  </nav>

  <!-- Settings — clear standalone affordance -->
  <div class="px-3 pb-2">
    <button
      on:click={() => currentPage.set('settings')}
      title="Settings"
      class="group flex items-center gap-3 w-full px-3 py-2 rounded-lg transition-all {$currentPage === 'settings' ? 'bg-accent/15 text-accent nav-active-pill' : 'text-text-muted hover:bg-bg-tertiary/60 hover:text-text-primary'}"
    >
      <svg class="w-[18px] h-[18px] flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="3"/>
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
      </svg>
      <span class="text-[13px] font-medium">Settings</span>
    </button>
  </div>

  <!-- Account -->
  <AccountPanel />

  <!-- Version -->
  <div class="px-4 pb-3 pt-1 flex items-center justify-between text-[10px] text-text-muted">
    <span class="font-mono">{version || '...'}</span>
    <span class="w-1.5 h-1.5 rounded-full bg-success" title="Online"></span>
  </div>
</aside>
