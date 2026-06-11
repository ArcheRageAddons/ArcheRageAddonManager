<script>
  import { onMount } from 'svelte';
  import { currentPage, currentUser, availableUpdates, refreshAvailableUpdates, downloadProgress, installSerially } from '../stores/app.js';
  import { GetVersion, GetCurrentUser, LoginWithDiscord, Logout } from '../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
  import { showNotification } from '../stores/app.js';
  import { dropdown } from '../motion.js';

  let version = '';
  let busy = false;
  let updatesOpen = false;
  let updateBusyById = {};
  let bulkBusy = false;
  let bulkProgress = { done: 0, total: 0 };

  onMount(() => {
    refreshAvailableUpdates();
    const onAddon = () => refreshAvailableUpdates();
    window.addEventListener('addon-installed', onAddon);
    return () => window.removeEventListener('addon-installed', onAddon);
  });

  onMount(async () => {
    try { version = await GetVersion(); } catch { version = ''; }
    try { const u = await GetCurrentUser(); currentUser.set(u || null); } catch {}
    EventsOn('auth:changed', (u) => currentUser.set(u || null));
  });

  async function handleAccountClick() {
    if ($currentUser) return; // signed-in account uses dropdown/menu elsewhere if needed
    busy = true;
    try {
      const u = await LoginWithDiscord();
      currentUser.set(u || null);
      if (u) showNotification(`Logged in as ${u.discord_username || 'user'}`, 'success');
    } catch (e) {
      showNotification(`Login failed: ${e}`, 'error', 12000);
    }
    busy = false;
  }

  async function handleLogout() {
    busy = true;
    try {
      await Logout();
      currentUser.set(null);
      currentPage.update((p) => (p === 'my-addons' || p === 'admin' ? 'browse' : p));
      showNotification('Logged out', 'info');
    } catch (e) {
      showNotification(`Logout failed: ${e}`, 'error');
    }
    busy = false;
  }

  let accountMenuOpen = false;

  function handleDocClick(e) {
    if (accountMenuOpen && !e.target.closest?.('[data-rail-account]')) accountMenuOpen = false;
    if (updatesOpen && !e.target.closest?.('[data-rail-updates]')) updatesOpen = false;
  }
  function handleKey(e) {
    if (e.key === 'Escape') { accountMenuOpen = false; updatesOpen = false; }
  }

  async function updateOne(addon) {
    if (updateBusyById[addon.id]) return;
    updateBusyById = { ...updateBusyById, [addon.id]: true };
    try {
      const { failed } = await installSerially([addon]);
      if (failed > 0) showNotification(`Failed to update ${addon.name}`, 'error', 6000);
    } finally {
      updateBusyById = { ...updateBusyById, [addon.id]: false };
      await refreshAvailableUpdates();
    }
  }

  async function updateAll() {
    if (bulkBusy) return;
    const targets = $availableUpdates.slice();
    if (!targets.length) return;
    bulkBusy = true;
    bulkProgress = { done: 0, total: targets.length };
    const { ok, failed } = await installSerially(targets, {
      onProgress: ({ done, total }) => { bulkProgress = { done, total }; },
    });
    bulkBusy = false;
    if (failed === 0) showNotification(`Updated ${ok} addon${ok === 1 ? '' : 's'}`, 'success');
    else showNotification(`Updated ${ok} of ${targets.length} (${failed} failed)`, failed === targets.length ? 'error' : 'info');
    await refreshAvailableUpdates();
    if ($availableUpdates.length === 0) updatesOpen = false;
  }
  onMount(() => {
    document.addEventListener('click', handleDocClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('click', handleDocClick);
      document.removeEventListener('keydown', handleKey);
    };
  });

  $: navItems = [
    { id: 'browse',    label: 'Browse',    icon: 'M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z' },
    { id: 'installed', label: 'Installed', icon: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3' },
    $currentUser ? { id: 'my-addons', label: 'My Addons', icon: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z@@M14 2v6h6M12 18v-6M9 15h6' } : null,
    $currentUser?.is_admin ? { id: 'admin', label: 'Admin', icon: 'M9 12l2 2 4-4M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z' } : null,
    { id: 'changelog', label: 'Changelog', icon: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z@@M14 2v6h6M8 13h8M8 17h5' },
  ].filter(Boolean);

  $: updateCount = $availableUpdates.length;
</script>

<aside class="w-[60px] flex flex-col items-center bg-bg-sidebar border-r border-border flex-shrink-0 select-none">
  <!-- Logo -->
  <button on:click={() => currentPage.set('browse')} class="mt-3 mb-2 group" title="ArcheRage Addons">
    <div class="relative">
      <div class="absolute inset-0 bg-accent/30 rounded-xl blur-md group-hover:bg-accent/50 transition-colors"></div>
      <img src="/logo.png" alt="ArcheRage" class="w-9 h-9 relative rounded-xl" />
    </div>
  </button>

  <div class="w-7 h-px bg-border my-2"></div>

  <!-- Nav stack -->
  <nav class="flex-1 w-full flex flex-col items-center gap-1 px-2 pt-1">
    {#each navItems as item (item.id)}
      {@const active = $currentPage === item.id}
      <div class="relative group w-full flex justify-center">
        <button
          on:click={() => currentPage.set(item.id)}
          aria-label={item.label}
          class="relative w-11 h-11 rounded-xl flex items-center justify-center transition-all {active ? 'bg-accent/15 text-accent' : 'text-text-secondary hover:bg-bg-tertiary/60 hover:text-text-primary'}"
        >
          {#if active}
            <span class="absolute -left-2 top-2 bottom-2 w-[3px] rounded-r-full bg-accent"></span>
          {/if}
          <svg class="w-[19px] h-[19px]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            {#each item.icon.split('@@') as p}<path d={p}/>{/each}
          </svg>
          {#if item.id === 'installed' && updateCount > 0}
            <span class="absolute -top-1 -right-1 min-w-[16px] h-4 px-1 rounded-full bg-warning text-bg-primary text-[9px] font-bold flex items-center justify-center ring-2 ring-bg-sidebar">
              {updateCount > 9 ? '9+' : updateCount}
            </span>
          {/if}
        </button>
        <!-- Floating tooltip -->
        <div class="pointer-events-none absolute left-full ml-2 px-2.5 py-1 rounded-md bg-bg-elevated border border-border text-xs text-text-primary font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity z-50 shadow-lift">
          {item.label}
        </div>
      </div>
    {/each}
  </nav>

  <!-- Bottom: updates bell + settings + account -->
  <div class="w-full flex flex-col items-center gap-1 pb-3 px-2">
    <!-- Updates bell -->
    <div class="relative" data-rail-updates>
      <div class="relative group w-full flex justify-center">
        <button
          on:click={() => (updatesOpen = !updatesOpen)}
          aria-label="Addon updates"
          class="relative w-11 h-11 rounded-xl flex items-center justify-center transition-all {updateCount > 0 ? 'text-warning hover:bg-warning/10' : 'text-text-secondary hover:bg-bg-tertiary/60 hover:text-text-primary'}"
        >
          <svg class="w-[19px] h-[19px]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
            <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
          </svg>
          {#if updateCount > 0}
            <span class="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] px-1 rounded-full bg-warning text-bg-primary text-[10px] font-bold flex items-center justify-center ring-2 ring-bg-sidebar {bulkBusy ? '' : 'animate-pulse'}">
              {updateCount > 99 ? '99+' : updateCount}
            </span>
          {/if}
        </button>
        {#if !updatesOpen}
          <div class="pointer-events-none absolute left-full ml-2 top-1/2 -translate-y-1/2 px-2.5 py-1 rounded-md bg-bg-elevated border border-border text-xs text-text-primary font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity z-50 shadow-lift">
            {updateCount > 0 ? `${updateCount} update${updateCount === 1 ? '' : 's'}` : 'Updates'}
          </div>
        {/if}
      </div>

      {#if updatesOpen}
        <div class="absolute bottom-0 left-full ml-2 w-[340px] bg-bg-elevated border border-border rounded-2xl shadow-modal overflow-hidden z-50" transition:dropdown>
          <div class="px-4 py-3 border-b border-border bg-header-grad flex items-center justify-between">
            <div>
              <span class="text-sm font-semibold text-text-primary">Addon updates</span>
              <div class="text-[11px] text-text-muted mt-0.5">
                {updateCount > 0 ? `${updateCount} ${updateCount === 1 ? 'update' : 'updates'} ready` : 'All up to date'}
              </div>
            </div>
            {#if updateCount > 0}
              <span class="px-2 py-0.5 bg-warning/15 text-warning text-[10px] font-bold uppercase tracking-wider rounded-md">New</span>
            {/if}
          </div>

          {#if updateCount === 0}
            <div class="px-4 py-7 text-center">
              <div class="mx-auto w-12 h-12 rounded-2xl bg-accent/10 border border-accent/30 flex items-center justify-center mb-2">
                <svg class="w-6 h-6 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M20 6L9 17l-5-5"/>
                </svg>
              </div>
              <p class="text-sm text-text-primary font-medium">You're up to date</p>
              <p class="text-xs text-text-muted mt-1">Nothing to install right now.</p>
            </div>
          {:else}
            <div class="max-h-72 overflow-y-auto">
              {#each $availableUpdates as addon (addon.id)}
                <div class="px-4 py-2.5 border-b border-border last:border-b-0 flex items-center gap-2.5 hover:bg-bg-tertiary/40 transition-colors">
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-text-primary truncate font-medium">{addon.name}</div>
                    <div class="text-[11px] text-text-muted flex items-center gap-1.5 mt-0.5">
                      <span class="font-mono">v{addon.version}</span>
                      <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
                      <span class="text-accent">newer</span>
                    </div>
                  </div>
                  <button
                    on:click={() => updateOne(addon)}
                    disabled={updateBusyById[addon.id] || bulkBusy || $downloadProgress.isDownloading}
                    class="px-3 py-1.5 bg-accent/10 border border-accent/50 hover:bg-accent hover:border-accent rounded-lg text-xs font-semibold text-accent hover:text-white disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5 transition-all"
                  >
                    {#if updateBusyById[addon.id]}
                      <svg class="animate-spin w-3 h-3" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                      </svg>
                    {/if}
                    Update
                  </button>
                </div>
              {/each}
            </div>
            {#if updateCount > 1}
              <div class="px-4 py-3 border-t border-border bg-bg-primary/40">
                <button
                  on:click={updateAll}
                  disabled={bulkBusy || $downloadProgress.isDownloading}
                  class="w-full px-3 py-2.5 bg-accent-grad hover:brightness-110 text-white rounded-lg text-sm font-semibold disabled:opacity-60 disabled:cursor-not-allowed flex items-center justify-center gap-2 shadow-soft"
                >
                  {#if bulkBusy}
                    <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                    </svg>
                    <span>Updating {bulkProgress.done}/{bulkProgress.total}…</span>
                  {:else}
                    <span>Update all ({updateCount})</span>
                  {/if}
                </button>
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </div>
    <div class="relative group w-full flex justify-center">
      <button
        on:click={() => currentPage.set('settings')}
        aria-label="Settings"
        class="relative w-11 h-11 rounded-xl flex items-center justify-center transition-all {$currentPage === 'settings' ? 'bg-accent/15 text-accent' : 'text-text-secondary hover:bg-bg-tertiary/60 hover:text-text-primary'}"
      >
        {#if $currentPage === 'settings'}
          <span class="absolute -left-2 top-2 bottom-2 w-[3px] rounded-r-full bg-accent"></span>
        {/if}
        <svg class="w-[19px] h-[19px]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="3"/>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
        </svg>
      </button>
      <div class="pointer-events-none absolute left-full ml-2 px-2.5 py-1 rounded-md bg-bg-elevated border border-border text-xs text-text-primary font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity z-50 shadow-lift">
        Settings
      </div>
    </div>

    <div class="relative" data-rail-account>
      <button
        on:click={() => $currentUser ? (accountMenuOpen = !accountMenuOpen) : handleAccountClick()}
        disabled={busy}
        aria-label="Account"
        class="relative w-11 h-11 rounded-xl flex items-center justify-center hover:bg-bg-tertiary/60 transition-colors group disabled:opacity-50"
      >
        <div class="relative">
          <div class="w-8 h-8 rounded-full bg-accent/20 flex items-center justify-center overflow-hidden ring-1 ring-border">
            {#if $currentUser?.discord_avatar}
              <img src={$currentUser.discord_avatar} alt="" referrerpolicy="no-referrer" class="w-full h-full object-cover" />
            {:else if $currentUser}
              <span class="text-accent text-xs font-bold">
                {($currentUser.discord_username || '?').slice(0, 1).toUpperCase()}
              </span>
            {:else}
              <svg class="w-4 h-4 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
                <circle cx="12" cy="7" r="4"/>
              </svg>
            {/if}
          </div>
          {#if $currentUser}
            <span class="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 bg-success rounded-full ring-2 ring-bg-sidebar"></span>
          {/if}
        </div>
        <div class="pointer-events-none absolute left-full ml-2 px-2.5 py-1 rounded-md bg-bg-elevated border border-border text-xs text-text-primary font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity z-50 shadow-lift">
          {$currentUser ? $currentUser.discord_username : 'Sign in'}
        </div>
      </button>

      <div class="text-[9px] text-text-muted font-mono text-center mt-1.5" title="Manager version">{version || '...'}</div>

      {#if accountMenuOpen && $currentUser}
        <div class="absolute bottom-0 left-full ml-2 w-[220px] bg-bg-elevated border border-border rounded-xl shadow-modal overflow-hidden z-50">
          <div class="px-3 py-2.5 border-b border-border bg-header-grad">
            <div class="text-[13px] font-semibold text-text-primary truncate">{$currentUser.discord_username}</div>
            <div class="text-[10px] mt-0.5">
              {#if $currentUser.is_admin}
                <span class="text-accent font-semibold uppercase tracking-wider">Admin</span>
              {:else}
                <span class="text-text-muted">Signed in</span>
              {/if}
            </div>
          </div>
          <div class="p-1.5">
            <button
              on:click={handleLogout}
              disabled={busy}
              class="w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-[13px] text-text-secondary hover:bg-red-500/10 hover:text-red-400 transition-colors disabled:opacity-50"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>
              </svg>
              Sign out
            </button>
          </div>
          <div class="px-3 py-1.5 border-t border-border bg-bg-primary/40 text-[9px] text-text-muted flex items-center justify-between font-mono">
            <span>{version || '...'}</span>
            <span class="flex items-center gap-1"><span class="w-1.5 h-1.5 rounded-full bg-success"></span>Online</span>
          </div>
        </div>
      {/if}
    </div>
  </div>
</aside>
