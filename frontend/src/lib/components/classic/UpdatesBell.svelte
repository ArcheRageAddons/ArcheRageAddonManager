<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    availableUpdates,
    refreshAvailableUpdates,
    showNotification,
    downloadProgress,
    installSerially,
  } from '../../stores/app.js';
  import { dropdown } from '../../motion.js';

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

<div class="fixed top-[20px] right-5 z-40" data-updates-bell>
  <button
    type="button"
    on:click={() => (open = !open)}
    class="relative p-2.5 rounded-xl bg-bg-secondary/90 border border-border hover:border-border-strong hover:bg-bg-tertiary transition-all shadow-soft"
    title={count > 0 ? `${count} addon update${count === 1 ? '' : 's'} available` : 'No addon updates'}
    aria-label="Addon updates"
  >
    <svg class="w-[18px] h-[18px] {count > 0 ? 'text-warning' : 'text-text-secondary'}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
      <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
    </svg>
    {#if count > 0}
      <span class="absolute -top-1.5 -right-1.5 min-w-[20px] h-5 px-1 rounded-full bg-warning text-bg-primary text-[10px] font-bold flex items-center justify-center ring-2 ring-bg-primary {bulkBusy ? '' : 'animate-pulse'}">
        {count > 99 ? '99+' : count}
      </span>
    {/if}
  </button>

  {#if open}
    <div class="absolute top-full right-0 mt-2 w-[340px] bg-bg-secondary border border-border rounded-2xl shadow-modal overflow-hidden origin-top-right" transition:dropdown>
      <div class="px-4 py-3 border-b border-border bg-header-grad flex items-center justify-between">
        <div>
          <span class="text-sm font-semibold text-text-primary">Addon updates</span>
          <div class="text-[11px] text-text-muted mt-0.5">
            {count > 0 ? `${count} ${count === 1 ? 'update' : 'updates'} ready` : 'All up to date'}
          </div>
        </div>
        {#if count > 0}
          <span class="px-2 py-0.5 bg-warning/15 text-warning text-[10px] font-bold uppercase tracking-wider rounded-md">New</span>
        {/if}
      </div>

      {#if count === 0}
        <div class="px-4 py-8 text-center">
          <div class="mx-auto w-12 h-12 rounded-2xl bg-bg-tertiary/60 border border-border flex items-center justify-center mb-2">
            <svg class="w-6 h-6 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
                disabled={busyById[addon.id] || bulkBusy || $downloadProgress.isDownloading}
                class="px-3 py-1.5 bg-accent/10 border border-accent/50 hover:bg-accent hover:border-accent rounded-lg text-xs font-semibold text-accent hover:text-white disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5 transition-all"
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
                <span>Update all ({count})</span>
              {/if}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
