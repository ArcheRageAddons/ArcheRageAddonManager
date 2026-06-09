<script>
  import { onMount } from 'svelte';
  import { selectedAddon, showAddonDetails, appInitialized, showNotification, refreshAvailableUpdates } from '../stores/app.js';
  import { GetAddons, GetCategories, RefreshAddons } from '../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
  import AddonDetailsModal from './AddonDetailsModal.svelte';
  import { resizable, persistedWidth } from '../resize.js';

  const NEW_WINDOW_MS = 7 * 24 * 60 * 60 * 1000;
  const PANE_DEFAULT = 400;
  const PANE_MIN = 280;
  const PANE_MAX = 720;
  const PANE_KEY = 'archerage-pane-width';
  const pw = persistedWidth(PANE_KEY, PANE_DEFAULT, PANE_MIN, PANE_MAX);
  let listWidth = pw.initial;
  function setListWidth(w) { listWidth = w; pw.save(w); }

  let allAddons = [];
  let categories = [];
  let selectedCategory = 'All';
  let searchQuery = '';
  let sortBy = 'newest';
  let loading = true;
  let initialized = false;

  const VIEW_MODE_KEY = 'archerage-browse-view';
  let viewMode = (typeof localStorage !== 'undefined' && localStorage.getItem(VIEW_MODE_KEY)) === 'grid' ? 'grid' : 'list';
  function setViewMode(mode) {
    viewMode = mode;
    try { localStorage.setItem(VIEW_MODE_KEY, mode); } catch {}
  }

  $: filteredAddons = filterAndSort(allAddons, selectedCategory, searchQuery, sortBy);

  function filterAndSort(list, category, search, sort) {
    if (!list || list.length === 0) return [];

    const q = search.trim().toLowerCase();
    let out = list.filter((a) => {
      if (category !== 'All' && a.category !== category) return false;
      if (!q) return true;
      const hay = [
        a.name, a.description, a.author_name,
        ...(a.keywords || []),
      ].filter(Boolean).join(' ').toLowerCase();
      return hay.includes(q);
    });

    out = out.map((a) => ({ ...a, _isNew: isNew(a.submitted_at) }));

    switch (sort) {
      case 'name':
        out.sort((a, b) => (a.name || '').localeCompare(b.name || ''));
        break;
      case 'installs':
        out.sort((a, b) => (b.download_count || 0) - (a.download_count || 0));
        break;
      case 'rating':
        out.sort((a, b) => (b.rating_avg || 0) - (a.rating_avg || 0));
        break;
      case 'newest':
      default:
        out.sort((a, b) => parseDate(b.submitted_at) - parseDate(a.submitted_at));
    }
    return out;
  }

  function parseDate(s) { if (!s) return 0; const t = Date.parse(s); return isNaN(t) ? 0 : t; }
  function isNew(submittedAt) { const t = parseDate(submittedAt); if (!t) return false; return Date.now() - t < NEW_WINDOW_MS; }

  onMount(() => {
    loadCategories();
    const handleAddonChange = () => { if (initialized) loadAddons(); };
    window.addEventListener('addon-installed', handleAddonChange);
    const offRegistry = EventsOn('registry:refreshed', () => { if (initialized) loadAddons(); });
    return () => {
      window.removeEventListener('addon-installed', handleAddonChange);
      offRegistry();
    };
  });

  $: if ($appInitialized && !initialized) { initialized = true; loadAddons(); }

  async function loadCategories() {
    try { categories = await GetCategories(); } catch (e) { console.error('cats:', e); }
  }

  async function loadAddons() {
    loading = true;
    try {
      allAddons = (await GetAddons()) || [];
    } catch (e) {
      console.error('Failed to load addons:', e);
      showNotification('Failed to load addons', 'error');
      allAddons = [];
    }
    loading = false;
  }

  async function handleRefresh() {
    loading = true;
    try {
      await RefreshAddons();
      allAddons = (await GetAddons()) || [];
      refreshAvailableUpdates();
      showNotification('Addons refreshed', 'success');
    } catch (e) {
      showNotification('Failed to refresh addons', 'error');
    }
    loading = false;
  }

  function selectAddon(addon) {
    selectedAddon.set(addon);
    showAddonDetails.set(true);
  }

  function formatCount(n) {
    if (!n || n < 1) return '';
    if (n < 1000) return String(n);
    if (n < 1_000_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
    return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  }

  $: newCount = filteredAddons.filter((a) => a._isNew).length;
</script>

<!-- 2-column split: list on left, detail pane on right -->
<div class="flex w-full h-full overflow-hidden">

  <!-- ============ Middle pane: list ============ -->
  <div
    class="flex flex-col bg-bg-primary flex-shrink-0"
    style="width: {listWidth}px;"
  >
    <!-- List header -->
    <div class="px-4 pt-4 pb-3 border-b border-border bg-bg-secondary/40">
      <div class="flex items-baseline justify-between mb-3">
        <div>
          <h1 class="text-[17px] font-bold text-text-primary tracking-tight leading-tight">Browse</h1>
          <p class="text-[11px] text-text-muted mt-0.5">
            {#if !loading}
              {allAddons.length} total{#if newCount > 0} · {newCount} new this week{/if}
            {:else}
              Loading…
            {/if}
          </p>
        </div>
        <button
          on:click={handleRefresh}
          class="p-1.5 rounded-md hover:bg-bg-tertiary text-text-muted hover:text-text-primary transition-colors"
          title="Refresh registry"
        >
          <svg class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
        </button>
      </div>

      <!-- Search -->
      <div class="relative mb-2.5">
        <input
          type="text"
          placeholder="Search addons…"
          bind:value={searchQuery}
          class="w-full pl-8 pr-7 py-2 bg-bg-primary border border-border hover:border-border-strong focus:border-accent rounded-lg focus:outline-none text-text-primary placeholder-text-muted text-[12px] transition-colors"
        />
        <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.35-4.35"/>
        </svg>
        {#if searchQuery}
          <button on:click={() => (searchQuery = '')} class="absolute right-2 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary" title="Clear">
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12"/></svg>
          </button>
        {/if}
      </div>

      <!-- Filters -->
      <div class="flex items-center gap-1.5">
        <select
          bind:value={selectedCategory}
          class="flex-1 min-w-0 px-2 py-1.5 bg-bg-primary border border-border hover:border-border-strong rounded-md focus:outline-none focus:border-accent text-text-secondary text-[11px] transition-colors"
        >
          {#each categories as cat}<option value={cat}>{cat}</option>{/each}
        </select>
        <select
          bind:value={sortBy}
          class="flex-1 min-w-0 px-2 py-1.5 bg-bg-primary border border-border hover:border-border-strong rounded-md focus:outline-none focus:border-accent text-text-secondary text-[11px] transition-colors"
          title="Sort"
        >
          <option value="newest">Newest</option>
          <option value="installs">Installs</option>
          <option value="rating">Rated</option>
          <option value="name">Name</option>
        </select>

        <!-- View mode toggle -->
        <div class="flex bg-bg-primary border border-border rounded-md p-0.5 flex-shrink-0" role="group" aria-label="View mode">
          <button
            on:click={() => setViewMode('list')}
            class="p-1 rounded-sm transition-colors {viewMode === 'list' ? 'bg-bg-tertiary text-accent' : 'text-text-muted hover:text-text-primary'}"
            title="List view"
            aria-pressed={viewMode === 'list'}
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <line x1="8" y1="6" x2="21" y2="6"/>
              <line x1="8" y1="12" x2="21" y2="12"/>
              <line x1="8" y1="18" x2="21" y2="18"/>
              <line x1="3" y1="6" x2="3.01" y2="6"/>
              <line x1="3" y1="12" x2="3.01" y2="12"/>
              <line x1="3" y1="18" x2="3.01" y2="18"/>
            </svg>
          </button>
          <button
            on:click={() => setViewMode('grid')}
            class="p-1 rounded-sm transition-colors {viewMode === 'grid' ? 'bg-bg-tertiary text-accent' : 'text-text-muted hover:text-text-primary'}"
            title="Grid view"
            aria-pressed={viewMode === 'grid'}
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7" rx="1"/>
              <rect x="14" y="3" width="7" height="7" rx="1"/>
              <rect x="14" y="14" width="7" height="7" rx="1"/>
              <rect x="3" y="14" width="7" height="7" rx="1"/>
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- List body -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center h-full">
          <svg class="animate-spin w-6 h-6 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
        </div>
      {:else if filteredAddons.length === 0}
        <div class="flex items-center justify-center h-full px-6 text-center">
          <p class="text-xs text-text-muted">No addons match.</p>
        </div>
      {:else if viewMode === 'grid'}
        <!-- Grid view: tiles in CSS grid that auto-fills to pane width -->
        <div class="grid gap-2.5 p-3" style="grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));">
          {#each filteredAddons as addon (addon.id)}
            {@const active = $selectedAddon?.id === addon.id}
            <button
              on:click={() => selectAddon(addon)}
              class="relative flex flex-col items-center gap-2 p-3 rounded-xl bg-card-grad border transition-all elev-card-hover {active ? 'border-accent shadow-glow' : 'border-border hover:border-border-strong'}"
              title={addon.name}
            >
              <!-- Top-right corner badges -->
              <div class="absolute top-1.5 right-1.5 flex items-center gap-1">
                {#if addon._isNew}
                  <span class="px-1.5 py-0.5 text-[8px] font-bold uppercase tracking-wider bg-accent/15 text-accent rounded-md">NEW</span>
                {/if}
                {#if addon.has_dangerous_files}
                  <span class="text-warning" title="Contains executable files">
                    <svg class="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg>
                  </span>
                {/if}
                {#if addon.is_hidden}
                  <span class="text-warning" title="Hidden"><svg class="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg></span>
                {/if}
              </div>
              <!-- Top-left update indicator -->
              {#if addon.has_update}
                <div class="absolute top-1.5 left-1.5 text-warning animate-pulse" title="Update available">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/></svg>
                </div>
              {/if}

              <!-- Icon -->
              <div class="relative mt-2">
                <div class="w-14 h-14 rounded-2xl bg-gradient-to-br from-bg-tertiary to-bg-secondary flex items-center justify-center overflow-hidden ring-1 ring-border">
                  {#if addon.icon}
                    <img src={addon.icon} alt="" referrerpolicy="no-referrer" class="w-full h-full object-cover" />
                  {:else}
                    <svg class="w-6 h-6 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                    </svg>
                  {/if}
                </div>
                {#if addon.is_installed}
                  <span class="absolute -bottom-1 -right-1 w-4 h-4 bg-accent rounded-full ring-2 ring-bg-secondary flex items-center justify-center">
                    <svg class="w-2.5 h-2.5 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
                  </span>
                {/if}
              </div>

              <!-- Name -->
              <span class="text-[11px] {active ? 'text-accent' : 'text-text-primary'} text-center font-semibold leading-tight line-clamp-2 mt-auto px-1">
                {addon.name}
              </span>
            </button>
          {/each}
        </div>
      {:else}
        <div>
          {#each filteredAddons as addon (addon.id)}
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
                {#if addon.is_installed}
                  <span class="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-accent rounded-full ring-2 ring-bg-primary flex items-center justify-center">
                    <svg class="w-1.5 h-1.5 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
                  </span>
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  <span class="text-[13px] font-semibold {active ? 'text-accent' : 'text-text-primary'} truncate">{addon.name}</span>
                  {#if addon._isNew}
                    <span class="text-[8px] font-bold uppercase tracking-wider text-accent flex-shrink-0">NEW</span>
                  {/if}
                  {#if addon.has_update}
                    <span class="w-1.5 h-1.5 rounded-full bg-warning flex-shrink-0 animate-pulse" title="Update available"></span>
                  {/if}
                  {#if addon.is_hidden}
                    <svg class="w-3 h-3 text-warning flex-shrink-0" viewBox="0 0 24 24" fill="currentColor" title="Hidden"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg>
                  {/if}
                </div>
                <div class="flex items-center gap-2 text-[10px] text-text-muted mt-0.5 truncate">
                  <span class="truncate">{addon.author_name || 'Unknown'}</span>
                  {#if addon.download_count > 0}
                    <span class="flex items-center gap-0.5 flex-shrink-0">
                      <svg class="w-2.5 h-2.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/></svg>
                      {formatCount(addon.download_count)}
                    </span>
                  {/if}
                  {#if addon.rating_count > 0}
                    <span class="flex items-center gap-0.5 flex-shrink-0">
                      <svg class="w-2.5 h-2.5 text-warning" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
                      {addon.rating_avg.toFixed(1)}
                    </span>
                  {/if}
                </div>
              </div>
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
      <!-- Welcome / empty state -->
      <div class="flex-1 flex items-center justify-center px-8 relative overflow-hidden">
        <div class="absolute inset-0 pointer-events-none">
          <div class="absolute top-1/4 right-1/4 w-96 h-96 rounded-full bg-accent/8 blur-[120px]"></div>
          <div class="absolute bottom-1/4 left-1/4 w-80 h-80 rounded-full bg-accent/5 blur-[100px]"></div>
        </div>

        <div class="relative text-center max-w-md">
          <div class="mx-auto w-24 h-24 rounded-3xl bg-gradient-to-br from-accent/20 to-accent/5 border border-accent/30 flex items-center justify-center mb-5 shadow-glow">
            <svg class="w-11 h-11 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
            </svg>
          </div>
          <h2 class="text-2xl font-bold text-text-primary tracking-tight mb-2">Pick an addon to get started</h2>
          <p class="text-sm text-text-muted leading-relaxed">
            {#if !loading}
              {allAddons.length} addons in the registry — search or pick one from the list to see its details, install it, or read its changelog.
            {:else}
              Loading the catalog…
            {/if}
          </p>
        </div>
      </div>
    {/if}
  </div>

</div>
