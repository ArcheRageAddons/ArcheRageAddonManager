<script>
  import { onMount } from 'svelte';
  import { showNotification, uninstallAddon, showUninstallConfirm, installSerially, refreshAvailableUpdates } from '../stores/app.js';
  import { GetInstalledAddons } from '../../../wailsjs/go/main/App.js';

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

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <div class="flex justify-between items-center p-4 pr-16 border-b border-border bg-bg-secondary">
    <h2 class="text-lg font-bold text-text-primary">Installed Addons</h2>
    <div class="flex items-center gap-2">
      {#if addonsWithUpdate.length > 0}
        <button
          on:click={handleBulkUpdate}
          disabled={bulkUpdating}
          class="px-4 py-2.5 bg-accent hover:bg-accent-hover text-white rounded-lg transition-colors flex items-center gap-2 text-sm disabled:opacity-60 disabled:cursor-not-allowed"
          title="Update every addon that has a newer version"
        >
          {#if bulkUpdating}
            <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
            <span>Updating {bulkProgress.done}/{bulkProgress.total}…</span>
          {:else}
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
            </svg>
            <span>Update all ({addonsWithUpdate.length})</span>
          {/if}
        </button>
      {/if}
      <button
        on:click={loadInstalled}
        disabled={bulkUpdating}
        class="px-4 py-2.5 bg-bg-tertiary hover:bg-border rounded-lg transition-colors flex items-center gap-2 text-sm text-text-secondary disabled:opacity-60"
      >
        <svg class="w-4 h-4 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M23 4v6h-6M1 20v-6h6"/>
          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
        </svg>
        <span>Refresh</span>
      </button>
    </div>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto p-4">
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="flex flex-col items-center gap-4">
          <svg class="animate-spin w-8 h-8 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          <span class="text-text-secondary text-sm">Loading installed addons...</span>
        </div>
      </div>
    {:else if installedAddons.length === 0}
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary">
          <svg class="w-16 h-16 mx-auto mb-4 opacity-50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
            <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
          </svg>
          <p class="text-lg">No addons installed</p>
          <p class="text-sm mt-2">Browse the addon catalog to get started</p>
        </div>
      </div>
    {:else}
      <div class="space-y-1">
        {#each installedAddons as addon}
          <div class="bg-bg-secondary hover:bg-bg-tertiary rounded-lg px-4 py-3 flex items-center gap-4 transition-colors">
            <!-- Icon -->
            <div class="w-10 h-10 rounded-lg bg-bg-tertiary flex items-center justify-center flex-shrink-0 overflow-hidden">
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
                <span class="font-medium text-text-primary">{addon.name}</span>
                <span class="text-xs text-text-muted">v{addon.version}</span>
                {#if addon.has_update}
                  <span class="w-2 h-2 rounded-full bg-warning animate-pulse" title="Update Available"></span>
                {/if}
                {#if addon.removed_from_registry}
                  <span class="text-[10px] uppercase tracking-wide text-warning bg-warning/10 border border-warning/40 rounded px-1.5 py-0.5" title="The author or a maintainer removed this addon from the public registry. Your installed copy still works but won't get updates.">
                    No longer in registry
                  </span>
                {/if}
              </div>
              <div class="text-xs text-text-muted mt-0.5">Installed: {addon.installed_at}</div>
              {#if addon.removed_from_registry}
                <div class="text-xs text-text-muted mt-1 italic">
                  This addon was removed from the registry. You can keep using it but it won't receive updates.
                </div>
              {/if}
            </div>

            <!-- Actions -->
            <div class="flex gap-2">
              {#if addon.has_update}
                <button
                  on:click={() => handleUpdate(addon)}
                  class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white rounded-lg transition-colors text-sm"
                >
                  Update
                </button>
              {/if}
              <button
                on:click={() => handleUninstall(addon)}
                class="p-1.5 bg-red-500/10 border border-red-500 hover:bg-red-500 rounded-md transition-colors text-red-500 hover:text-white"
                title="Uninstall"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
