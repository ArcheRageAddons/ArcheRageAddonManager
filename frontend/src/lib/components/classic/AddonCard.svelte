<script>
  import { showNotification, warningAddon, showWarningModal, downloadProgress, uninstallAddon, showUninstallConfirm, selectedAuthor, showAuthorModal, kickOffInstall } from '../../stores/app.js';

  export let addon;

  function formatCount(n) {
    if (!n || n < 1) return '';
    if (n < 1000) return String(n);
    if (n < 1_000_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
    return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  }

  async function handleDownload(e) {
    e.stopPropagation();

    if (addon.overlay_of && !addon.base_installed) {
      showNotification(`Install ${addon.overlay_of} first — this addon overlays on top of it`, 'warning', 5000);
      return;
    }

    if (!addon.is_installed) {
      const missing = (addon.dependencies || []).filter((d) => !d.is_installed);
      if (missing.length > 0) {
        showNotification(
          `Install the missing dependenc${missing.length === 1 ? 'y' : 'ies'} first: ${missing.map((d) => d.name).join(', ')}`,
          'warning', 5000,
        );
        return;
      }
    }

    if (addon.has_dangerous_files) {
      warningAddon.set(addon);
      showWarningModal.set(true);
      return;
    }

    kickOffInstall(addon);
  }

  function handleUninstall(e) {
    e.stopPropagation();
    uninstallAddon.set(addon);
    showUninstallConfirm.set(true);
  }

  function openAuthor(e) {
    e.stopPropagation();
    if (!addon.author_name) return;
    selectedAuthor.set(addon.author_name);
    showAuthorModal.set(true);
  }
</script>

<button
  on:click
  class="w-full bg-card-grad rounded-xl px-4 py-3 flex items-center gap-4 transition-all text-left group border elev-card-hover {addon.is_installed ? 'border-accent/60 hover:border-accent shadow-glow' : 'border-border hover:border-border-strong'}"
>
  <!-- Icon block with subtle inner gradient -->
  <div class="relative w-11 h-11 rounded-xl bg-gradient-to-br from-bg-tertiary to-bg-secondary flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border">
    {#if addon.icon}
      <img src={addon.icon} alt={addon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
    {:else}
      <svg class="w-5 h-5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
      </svg>
    {/if}
    {#if addon.is_installed}
      <span class="absolute -bottom-0.5 -right-0.5 w-3.5 h-3.5 bg-accent rounded-full ring-2 ring-bg-secondary flex items-center justify-center">
        <svg class="w-2 h-2 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4" stroke-linecap="round" stroke-linejoin="round">
          <path d="M20 6L9 17l-5-5"/>
        </svg>
      </span>
    {/if}
  </div>

  <!-- Addon Name & Meta -->
  <div class="flex-1 min-w-0">
    <div class="flex items-center gap-2 flex-wrap">
      <span class="font-semibold text-text-primary text-[14px] leading-tight">{addon.name}</span>
      {#if addon._isNew}
        <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-accent/15 text-accent rounded-md" title="Added to the registry within the last 7 days">
          New
        </span>
      {/if}
      {#if addon.overlay_of}
        <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-bg-tertiary text-text-secondary rounded-md" title="Overlays on top of {addon.overlay_of} — install that first">
          Patch
        </span>
      {/if}
      {#if addon.is_hidden}
        <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/15 text-warning border border-warning/40 rounded-md" title="Temporarily hidden from non-admin users: {addon.hidden_reason || 'no reason given'}">
          Hidden
        </span>
      {/if}
      {#if addon.has_dangerous_files}
        <svg class="w-4 h-4 text-warning flex-shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-label="Contains dangerous files">
          <title>Contains executable files (e.g. .bat / .exe / .dll / .lnk). Only install if you trust the author.</title>
          <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
        </svg>
      {/if}
      {#if addon.has_update}
        <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/15 text-warning rounded-md animate-pulse" title="Update Available">
          Update
        </span>
      {/if}
    </div>
    <div class="flex items-center gap-3 text-[11px] text-text-muted mt-1">
      <span
        role="button"
        tabindex="0"
        on:click={openAuthor}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openAuthor(e); } }}
        class="hover:text-accent transition-colors cursor-pointer"
        title="See all addons by this author"
      >
        {addon.author_name || 'Unknown'}
      </span>
      <span class="text-text-muted/40">•</span>
      <span class="text-text-secondary font-mono">v{addon.version}</span>
      {#if addon.download_count > 0}
        <span class="text-text-muted/40">•</span>
        <span class="flex items-center gap-1" title="{addon.download_count.toLocaleString()} install{addon.download_count === 1 ? '' : 's'}">
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
          </svg>
          {formatCount(addon.download_count)}
        </span>
      {/if}
      {#if addon.rating_count > 0}
        <span class="text-text-muted/40">•</span>
        <span class="flex items-center gap-1" title="{addon.rating_avg.toFixed(2)} from {addon.rating_count} rating{addon.rating_count === 1 ? '' : 's'}">
          <svg class="w-3 h-3 text-warning" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
          </svg>
          {addon.rating_avg.toFixed(1)}
        </span>
      {/if}
    </div>
  </div>

  <!-- Category tag -->
  <div class="hidden md:flex items-center gap-1.5">
    <span class="px-2.5 py-1 bg-bg-primary/60 border border-border text-text-secondary text-[11px] font-medium rounded-lg">
      {addon.category}
    </span>
  </div>

  <!-- Action Buttons -->
  <div class="flex items-center gap-1.5">
    {#if addon.is_installed}
      <button
        on:click={handleUninstall}
        class="p-2 bg-red-500/10 border border-red-500/50 hover:bg-red-500 hover:border-red-500 rounded-lg transition-all text-red-400 hover:text-white"
        title="Uninstall"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
        </svg>
      </button>
    {/if}

    <button
      on:click={handleDownload}
      disabled={$downloadProgress.isDownloading}
      class="p-2 bg-accent/10 border border-accent/50 hover:bg-accent hover:border-accent rounded-lg transition-all disabled:opacity-50 text-accent hover:text-white"
      title={addon.is_installed ? (addon.has_update ? 'Update' : 'Reinstall') : 'Download'}
    >
      {#if $downloadProgress.isDownloading && $downloadProgress.addonId === addon.id}
        <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
        </svg>
      {:else}
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
        </svg>
      {/if}
    </button>
  </div>
</button>
