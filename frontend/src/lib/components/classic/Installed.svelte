<script>
  import { onMount } from 'svelte';
  import { showNotification, uninstallAddon, showUninstallConfirm, installSerially, refreshAvailableUpdates } from '../../stores/app.js';
  import { GetInstalledAddons, OpenAddonFolder } from '../../../../wailsjs/go/main/App.js';

  async function openFolder() {
    try { await OpenAddonFolder(); }
    catch (e) { showNotification(`Couldn't open folder: ${e}`, 'error'); }
  }

  let installedAddons = [];
  let loading = true;
  let bulkUpdating = false;
  let bulkProgress = { done: 0, total: 0 };

  $: addonsWithUpdate = installedAddons.filter((a) => a.has_update);

  onMount(() => {
    loadInstalled();

    const handleAddonChange = () => loadInstalled();
    window.addEventListener('addon-installed', handleAddonChange);

    return () => {
      window.removeEventListener('addon-installed', handleAddonChange);
    };
  });

  async function loadInstalled() {
    loading = true;
    try {
      installedAddons = await GetInstalledAddons() || [];
      refreshAvailableUpdates();
    } catch (e) {
      console.error('Failed to load installed addons:', e);
      installedAddons = [];
    }
    loading = false;
  }

  async function handleUpdate(addon) {
    await installSerially([addon]);
    await loadInstalled();
  }

  async function handleBulkUpdate() {
    if (bulkUpdating) return;
    const targets = installedAddons.filter((a) => a.has_update);
    if (targets.length === 0) return;

    bulkUpdating = true;
    bulkProgress = { done: 0, total: targets.length };

    const { ok, failed } = await installSerially(targets, {
      onProgress: ({ done, total }) => { bulkProgress = { done, total }; },
    });

    bulkUpdating = false;
    if (failed === 0) {
      showNotification(`Updated ${ok} addon${ok === 1 ? '' : 's'}`, 'success');
    } else {
      showNotification(`Updated ${ok} of ${targets.length} (${failed} failed)`, failed === targets.length ? 'error' : 'info');
    }
    await loadInstalled();
  }

  function handleUninstall(addon) {
    uninstallAddon.set(addon);
    showUninstallConfirm.set(true);
  }
</script>

<div class="h-full flex flex-col overflow-hidden app-canvas-ambient">
  <!-- Header -->
  <div class="px-6 pt-5 pb-5 pr-16 border-b border-border bg-header-grad">
    <div class="flex justify-between items-baseline">
      <div>
        <h1 class="text-xl font-bold text-text-primary tracking-tight">Installed</h1>
        <p class="text-xs text-text-muted mt-0.5">
          {#if !loading}
            {installedAddons.length} installed{#if addonsWithUpdate.length > 0} · <span class="text-warning font-medium">{addonsWithUpdate.length} {addonsWithUpdate.length === 1 ? 'update' : 'updates'} available</span>{/if}
          {:else}
            Loading…
          {/if}
        </p>
      </div>
      <div class="flex items-center gap-2">
        {#if addonsWithUpdate.length > 0}
          <button
            on:click={handleBulkUpdate}
            disabled={bulkUpdating}
            class="px-4 py-2.5 bg-accent-grad hover:brightness-110 text-white rounded-xl transition-all flex items-center gap-2 text-sm font-semibold disabled:opacity-60 disabled:cursor-not-allowed shadow-lift"
            title="Update every addon that has a newer version"
          >
            {#if bulkUpdating}
              <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
              </svg>
              <span>Updating {bulkProgress.done}/{bulkProgress.total}…</span>
            {:else}
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
              </svg>
              <span>Update all ({addonsWithUpdate.length})</span>
            {/if}
          </button>
        {/if}
        <button
          on:click={openFolder}
          class="px-3 py-2 bg-bg-tertiary/60 hover:bg-bg-tertiary border border-border rounded-lg transition-all flex items-center gap-2 text-xs text-text-secondary hover:text-text-primary"
          title="Open the addon folder in File Explorer"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>
            <path d="M14 11l3 3-3 3M17 14H9"/>
          </svg>
          <span>Open folder</span>
        </button>
        <button
          on:click={loadInstalled}
          disabled={bulkUpdating}
          class="px-3 py-2 bg-bg-tertiary/60 hover:bg-bg-tertiary border border-border rounded-lg transition-all flex items-center gap-2 text-xs text-text-secondary hover:text-text-primary disabled:opacity-60"
        >
          <svg class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
          <span>Refresh</span>
        </button>
      </div>
    </div>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto px-6 py-5">
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="flex flex-col items-center gap-4">
          <svg class="animate-spin w-8 h-8 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          <span class="text-text-secondary text-sm">Loading installed addons…</span>
        </div>
      </div>
    {:else if installedAddons.length === 0}
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary max-w-sm">
          <div class="mx-auto w-20 h-20 rounded-2xl bg-bg-secondary border border-border flex items-center justify-center mb-4">
            <svg class="w-9 h-9 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
            </svg>
          </div>
          <p class="text-base text-text-primary font-medium">No addons installed yet</p>
          <p class="text-sm mt-1.5 text-text-muted">Head to Browse and pick something to get started.</p>
        </div>
      </div>
    {:else}
      <div class="space-y-2">
        {#each installedAddons as addon}
          <div class="bg-card-grad border border-border hover:border-border-strong rounded-xl px-4 py-3 flex items-center gap-4 transition-all elev-card-hover">
            <!-- Icon -->
            <div class="relative w-11 h-11 rounded-xl bg-gradient-to-br from-bg-tertiary to-bg-secondary flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border">
              {#if addon.icon}
                <img src={addon.icon} alt={addon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
              {:else}
                <svg class="w-5 h-5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                </svg>
              {/if}
            </div>

            <!-- Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-semibold text-text-primary text-[14px] leading-tight">{addon.name}</span>
                <span class="text-[11px] font-mono text-text-muted">v{addon.version}</span>
                {#if addon.has_update}
                  <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/15 text-warning rounded-md animate-pulse" title="Update Available">
                    Update
                  </span>
                {/if}
                {#if addon.removed_from_registry}
                  <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/10 border border-warning/40 text-warning rounded-md" title="Removed from registry">
                    Orphaned
                  </span>
                {/if}
                {#if addon.is_hidden}
                  <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/10 border border-warning/40 text-warning rounded-md" title="Hidden by admin">
                    Hidden
                  </span>
                {/if}
              </div>
              <div class="text-[11px] text-text-muted mt-1 flex items-center gap-2">
                <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 8v4l3 3M3 12a9 9 0 1 0 18 0 9 9 0 0 0-18 0z"/></svg>
                Installed {addon.installed_at}
              </div>
              {#if addon.removed_from_registry}
                <div class="text-[11px] text-text-muted mt-1.5 italic">
                  Removed from the registry. Your copy keeps working but won't get updates.
                </div>
              {/if}
              {#if addon.is_hidden}
                <div class="text-[11px] text-warning mt-1.5 italic whitespace-pre-wrap">
                  Hidden by an admin: {addon.hidden_reason || 'no reason given.'}
                </div>
              {/if}
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-1.5">
              {#if addon.has_update}
                <button
                  on:click={() => handleUpdate(addon)}
                  class="px-3 py-2 bg-accent-grad hover:brightness-110 text-white rounded-lg transition-all text-xs font-semibold shadow-soft"
                >
                  Update
                </button>
              {/if}
              <button
                on:click={() => handleUninstall(addon)}
                class="p-2 bg-red-500/10 border border-red-500/50 hover:bg-red-500 hover:border-red-500 rounded-lg transition-all text-red-400 hover:text-white"
                title="Uninstall"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                </svg>
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
