<script>
  import { onMount } from 'svelte';
  import { showNotification, uninstallAddon, showUninstallConfirm, installSerially, refreshAvailableUpdates, selectedAddon, showAddonDetails } from '../stores/app.js';
  import { GetInstalledAddons, GetAddonDetails } from '../../../wailsjs/go/main/App.js';
  import AddonDetailsModal from './AddonDetailsModal.svelte';
  import { resizable, persistedWidth } from '../resize.js';

  const PANE_DEFAULT = 400;
  const PANE_MIN = 280;
  const PANE_MAX = 720;
  // Shared key with Browse — drag once, both pages remember it.
  const PANE_KEY = 'archerage-pane-width';
  const pw = persistedWidth(PANE_KEY, PANE_DEFAULT, PANE_MIN, PANE_MAX);
  let listWidth = pw.initial;
  function setListWidth(w) { listWidth = w; pw.save(w); }

  let installedAddons = [];
  let loading = true;
  let bulkUpdating = false;
  let bulkProgress = { done: 0, total: 0 };

  $: addonsWithUpdate = installedAddons.filter((a) => a.has_update);

  onMount(() => {
    loadInstalled();
    const handleAddonChange = () => loadInstalled();
    window.addEventListener('addon-installed', handleAddonChange);
    return () => window.removeEventListener('addon-installed', handleAddonChange);
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
    if (failed === 0) showNotification(`Updated ${ok} addon${ok === 1 ? '' : 's'}`, 'success');
    else showNotification(`Updated ${ok} of ${targets.length} (${failed} failed)`, failed === targets.length ? 'error' : 'info');
    await loadInstalled();
  }

  function handleUninstall(addon, e) {
    e?.stopPropagation?.();
    uninstallAddon.set(addon);
    showUninstallConfirm.set(true);
  }

  async function selectAddon(installed) {
    // Load full details for the right pane
    try {
      const full = await GetAddonDetails(installed.id);
      if (full) {
        selectedAddon.set(full);
        showAddonDetails.set(true);
      } else {
        showNotification(`Couldn't open ${installed.name} — it may no longer be in the registry.`, 'warning', 6000);
      }
    } catch (e) {
      showNotification(`${e}`, 'error', 6000);
    }
  }
</script>

<div class="flex w-full h-full overflow-hidden">

  <!-- ============ Middle pane: installed list ============ -->
  <div
    class="flex flex-col bg-bg-primary flex-shrink-0"
    style="width: {listWidth}px;"
  >
    <div class="px-4 pt-4 pb-3 border-b border-border bg-bg-secondary/40">
      <div class="flex items-baseline justify-between mb-3">
        <div>
          <h1 class="text-[17px] font-bold text-text-primary tracking-tight leading-tight">Installed</h1>
          <p class="text-[11px] text-text-muted mt-0.5">
            {#if !loading}
              {installedAddons.length} installed{#if addonsWithUpdate.length > 0} · <span class="text-warning">{addonsWithUpdate.length} update{addonsWithUpdate.length === 1 ? '' : 's'}</span>{/if}
            {:else}
              Loading…
            {/if}
          </p>
        </div>
        <button
          on:click={loadInstalled}
          disabled={bulkUpdating}
          class="p-1.5 rounded-md hover:bg-bg-tertiary text-text-muted hover:text-text-primary transition-colors disabled:opacity-50"
          title="Refresh"
        >
          <svg class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
        </button>
      </div>

      {#if addonsWithUpdate.length > 0}
        <button
          on:click={handleBulkUpdate}
          disabled={bulkUpdating}
          class="w-full px-3 py-2 bg-accent-grad hover:brightness-110 text-white rounded-lg transition-all flex items-center justify-center gap-2 text-[12px] font-semibold disabled:opacity-60 disabled:cursor-not-allowed shadow-soft"
        >
          {#if bulkUpdating}
            <svg class="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
            <span>Updating {bulkProgress.done}/{bulkProgress.total}…</span>
          {:else}
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
            </svg>
            <span>Update all ({addonsWithUpdate.length})</span>
          {/if}
        </button>
      {/if}
    </div>

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center h-full">
          <svg class="animate-spin w-6 h-6 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
        </div>
      {:else if installedAddons.length === 0}
        <div class="flex items-center justify-center h-full px-6 text-center">
          <p class="text-xs text-text-muted leading-relaxed">No addons installed yet.<br/>Head to Browse and pick one.</p>
        </div>
      {:else}
        <div>
          {#each installedAddons as addon (addon.id)}
            {@const active = $selectedAddon?.id === addon.id}
            <button
              on:click={() => selectAddon(addon)}
              class="w-full px-3 py-2.5 flex items-center gap-3 text-left transition-colors border-l-2 {active ? 'bg-accent/10 border-l-accent' : 'border-l-transparent hover:bg-bg-tertiary/40'}"
            >
              <div class="relative w-9 h-9 rounded-lg bg-gradient-to-br from-bg-tertiary to-bg-secondary flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border">
                {#if addon.icon}
                  <img src={addon.icon} alt="" referrerpolicy="no-referrer" class="w-full h-full object-cover" />
                {:else}
                  <svg class="w-4 h-4 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                  </svg>
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  <span class="text-[13px] font-semibold {active ? 'text-accent' : 'text-text-primary'} truncate">{addon.name}</span>
                  {#if addon.has_update}
                    <span class="text-[8px] font-bold uppercase tracking-wider text-warning flex-shrink-0 animate-pulse">UPDATE</span>
                  {/if}
                  {#if addon.removed_from_registry}
                    <span class="text-[8px] font-bold uppercase tracking-wider text-warning flex-shrink-0">ORPHAN</span>
                  {/if}
                  {#if addon.is_hidden}
                    <svg class="w-3 h-3 text-warning flex-shrink-0" viewBox="0 0 24 24" fill="currentColor" title="Hidden"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg>
                  {/if}
                </div>
                <div class="flex items-center gap-2 text-[10px] text-text-muted mt-0.5 truncate">
                  <span class="font-mono">v{addon.version}</span>
                  <span>·</span>
                  <span class="truncate">{addon.installed_at}</span>
                </div>
              </div>
              <button
                on:click={(e) => handleUninstall(addon, e)}
                class="p-1.5 rounded-md text-text-muted hover:bg-red-500/15 hover:text-red-400 transition-colors flex-shrink-0"
                title="Uninstall"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                </svg>
              </button>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <!-- ============ Drag handle ============ -->
  <div
    class="pane-resizer"
    use:resizable={{
      onResize: setListWidth,
      getCurrent: () => listWidth,
      defaultWidth: PANE_DEFAULT,
      min: PANE_MIN,
      max: PANE_MAX,
    }}
    role="separator"
    aria-orientation="vertical"
    title="Drag to resize — double-click to reset"
  ></div>

  <!-- ============ Right pane: details ============ -->
  <div class="flex-1 overflow-hidden flex flex-col">
    {#if $selectedAddon && $showAddonDetails}
      <AddonDetailsModal />
    {:else}
      <div class="flex-1 flex items-center justify-center px-8 relative overflow-hidden">
        <div class="absolute inset-0 pointer-events-none">
          <div class="absolute top-1/3 right-1/4 w-96 h-96 rounded-full bg-accent/6 blur-[120px]"></div>
        </div>
        <div class="relative text-center max-w-md">
          <div class="mx-auto w-24 h-24 rounded-3xl bg-gradient-to-br from-bg-secondary to-bg-tertiary border border-border flex items-center justify-center mb-5">
            <svg class="w-11 h-11 text-text-secondary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
            </svg>
          </div>
          <h2 class="text-2xl font-bold text-text-primary tracking-tight mb-2">
            {installedAddons.length === 0 ? 'Nothing installed yet' : 'Pick an installed addon'}
          </h2>
          <p class="text-sm text-text-muted leading-relaxed">
            {#if installedAddons.length === 0}
              Head to Browse and pick something to get started.
            {:else}
              Select one from the list to see details, check for updates, or uninstall.
            {/if}
          </p>
        </div>
      </div>
    {/if}
  </div>

</div>
