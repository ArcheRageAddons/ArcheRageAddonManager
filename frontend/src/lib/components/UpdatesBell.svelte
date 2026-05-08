<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    availableUpdates,
    refreshAvailableUpdates,
    showNotification,
    downloadProgress,
    installSerially,
  } from '../stores/app.js';
  import { dropdown } from '../motion.js';

  let open = false;
  let busyById = {};
  let bulkBusy = false;
  let bulkProgress = { done: 0, total: 0 };

  function handleRefresh() { refreshAvailableUpdates(); }

  onMount(() => {
    refreshAvailableUpdates();
    window.addEventListener('addon-installed', handleRefresh);
  });

  onDestroy(() => {
    window.removeEventListener('addon-installed', handleRefresh);
  });

  function handleDocClick(e) {
    if (!open) return;
    const root = e.target.closest?.('[data-updates-bell]');
    if (!root) open = false;
  }
  function handleKey(e) {
    if (e.key === 'Escape') open = false;
  }
  onMount(() => {
    document.addEventListener('click', handleDocClick);
    document.addEventListener('keydown', handleKey);
  });
  onDestroy(() => {
    document.removeEventListener('click', handleDocClick);
    document.removeEventListener('keydown', handleKey);
  });

  $: count = $availableUpdates.length;

  async function updateOne(addon) {
    if (busyById[addon.id]) return;
    busyById = { ...busyById, [addon.id]: true };
    try {
      const { failed } = await installSerially([addon]);
      if (failed > 0) {
        showNotification(`Failed to update ${addon.name}`, 'error', 6000);
      }
    } finally {
      busyById = { ...busyById, [addon.id]: false };
      await refreshAvailableUpdates();
    }
  }

  async function updateAll() {
    if (bulkBusy) return;
    const targets = $availableUpdates.slice();
    if (targets.length === 0) return;

    bulkBusy = true;
    bulkProgress = { done: 0, total: targets.length };

    const { ok, failed } = await installSerially(targets, {
      onProgress: ({ done, total }) => { bulkProgress = { done, total }; },
    });

    bulkBusy = false;
    if (failed === 0) {
      showNotification(`Updated ${ok} addon${ok === 1 ? '' : 's'}`, 'success');
    } else {
      showNotification(`Updated ${ok} of ${targets.length} (${failed} failed)`, failed === targets.length ? 'error' : 'info');
    }
    await refreshAvailableUpdates();
    if (count === 0) open = false;
  }
</script>

<div class="fixed top-[18px] right-4 z-40" data-updates-bell>
  <button
    type="button"
    on:click={() => (open = !open)}
    class="relative p-2 rounded-lg bg-bg-secondary border border-border hover:bg-bg-tertiary transition-colors"
    title={count > 0 ? `${count} addon update${count === 1 ? '' : 's'} available` : 'No addon updates'}
    aria-label="Addon updates"
  >
    <svg class="w-5 h-5 text-text-secondary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
      <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
    </svg>
    {#if count > 0}
      <span class="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 rounded-full bg-warning text-bg-primary text-[10px] font-bold flex items-center justify-center {bulkBusy ? '' : 'animate-pulse'}">
        {count > 99 ? '99+' : count}
      </span>
    {/if}
  </button>

  {#if open}
    <div class="absolute top-full right-0 mt-2 w-80 bg-bg-secondary border border-border rounded-xl shadow-2xl overflow-hidden origin-top-right" transition:dropdown>
      <div class="px-4 py-3 border-b border-border flex items-center justify-between">
        <span class="text-sm font-semibold text-text-primary">Addon updates</span>
        {#if count > 0}
          <span class="text-xs text-text-muted">{count} available</span>
        {/if}
      </div>

      {#if count === 0}
        <div class="px-4 py-6 text-sm text-text-muted text-center">
          You're up to date.
        </div>
      {:else}
        <div class="max-h-72 overflow-y-auto">
          {#each $availableUpdates as addon (addon.id)}
            <div class="px-4 py-2.5 border-b border-border/40 last:border-b-0 flex items-center gap-2">
              <div class="flex-1 min-w-0">
                <div class="text-sm text-text-primary truncate">{addon.name}</div>
                <div class="text-xs text-text-muted">v{addon.version} → newer version</div>
              </div>
              <button
                on:click={() => updateOne(addon)}
                disabled={busyById[addon.id] || bulkBusy || $downloadProgress.isDownloading}
                class="px-2.5 py-1 bg-accent hover:bg-accent-hover text-white rounded text-xs font-medium disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
              >
                {#if busyById[addon.id]}
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

        {#if count > 1}
          <div class="px-4 py-3 border-t border-border bg-bg-secondary/50">
            <button
              on:click={updateAll}
              disabled={bulkBusy || $downloadProgress.isDownloading}
              class="w-full px-3 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium disabled:opacity-60 disabled:cursor-not-allowed flex items-center justify-center gap-2"
            >
              {#if bulkBusy}
                <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                </svg>
                <span>Updating {bulkProgress.done}/{bulkProgress.total}…</span>
              {:else}
                <span>Update all ({count})</span>
              {/if}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
