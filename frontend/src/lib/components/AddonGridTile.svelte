<script>
  // Grid-view counterpart to AddonCard. Strict minimal — icon + name only.
  // Click opens the addon details modal (same flow as the list view), so
  // install / update / overlay-base gating all live there. Small overlay
  // badges in the corner surface the same key states the list view shows
  // inline (NEW, installed, has-update, dangerous-files) so users can scan
  // a grid and still spot what's important.

  export let addon;
</script>

<button
  on:click
  class="relative bg-bg-secondary hover:bg-bg-tertiary rounded-lg p-3 flex flex-col items-center gap-2 transition-colors text-left group min-h-[140px]"
  title={addon.name}
>
  <!-- Corner badges (top-right) -->
  <div class="absolute top-1.5 right-1.5 flex items-center gap-1">
    {#if addon._isNew}
      <span class="px-1 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-accent/20 text-accent rounded">
        New
      </span>
    {/if}
    {#if addon.has_dangerous_files}
      <span class="text-warning" title="Contains executable files">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
          <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
        </svg>
      </span>
    {/if}
  </div>

  <!-- Corner badges (top-left) -->
  <div class="absolute top-1.5 left-1.5 flex items-center gap-1">
    {#if addon.is_installed}
      <span class="text-success" title="Installed">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <path d="M20 6L9 17l-5-5"/>
        </svg>
      </span>
    {/if}
    {#if addon.has_update}
      <span class="text-warning animate-pulse" title="Update available">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
        </svg>
      </span>
    {/if}
  </div>

  <!-- Icon -->
  <div class="w-16 h-16 rounded-lg bg-bg-tertiary flex items-center justify-center overflow-hidden flex-shrink-0 mt-2">
    {#if addon.icon}
      <img src={addon.icon} alt={addon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
    {:else}
      <svg class="w-7 h-7 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
      </svg>
    {/if}
  </div>

  <!-- Name -->
  <span class="text-xs text-text-primary text-center font-medium leading-tight line-clamp-2 mt-auto">
    {addon.name}
  </span>
</button>
