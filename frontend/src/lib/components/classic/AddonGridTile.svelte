<script>
  export let addon;
</script>

<button
  on:click
  class="relative bg-card-grad rounded-xl p-4 flex flex-col items-center gap-2.5 transition-all text-left group min-h-[160px] border elev-card-hover {addon.is_installed ? 'border-accent/60 hover:border-accent shadow-glow' : 'border-border hover:border-border-strong'}"
  title={addon.name}
>
  <!-- Top-right corner badges -->
  <div class="absolute top-2 right-2 flex items-center gap-1">
    {#if addon._isNew}
      <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-accent/15 text-accent rounded-md">
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
    {#if addon.is_hidden}
      <span class="px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-warning/15 text-warning border border-warning/40 rounded-md" title="Temporarily hidden">
        Hid
      </span>
    {/if}
  </div>

  <!-- Top-left status badges -->
  <div class="absolute top-2 left-2 flex items-center gap-1">
    {#if addon.has_update}
      <span class="text-warning animate-pulse" title="Update available">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
        </svg>
      </span>
    {/if}
  </div>

  <!-- Icon -->
  <div class="relative mt-3">
    <div class="w-[68px] h-[68px] rounded-2xl bg-gradient-to-br from-bg-tertiary to-bg-secondary flex items-center justify-center overflow-hidden flex-shrink-0 ring-1 ring-border">
      {#if addon.icon}
        <img src={addon.icon} alt={addon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
      {:else}
        <svg class="w-7 h-7 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
        </svg>
      {/if}
    </div>
    {#if addon.is_installed}
      <span class="absolute -bottom-1 -right-1 w-5 h-5 bg-accent rounded-full ring-2 ring-bg-secondary flex items-center justify-center">
        <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4" stroke-linecap="round" stroke-linejoin="round">
          <path d="M20 6L9 17l-5-5"/>
        </svg>
      </span>
    {/if}
  </div>

  <span class="text-[12px] text-text-primary text-center font-semibold leading-tight line-clamp-2 mt-auto">
    {addon.name}
  </span>
</button>
