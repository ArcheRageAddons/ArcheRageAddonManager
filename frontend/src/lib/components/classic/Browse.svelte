<script>
  import { onMount } from 'svelte';
  import { selectedAddon, showAddonDetails, appInitialized, showNotification, refreshAvailableUpdates } from '../../stores/app.js';
  import { GetAddons, GetCategories, RefreshAddons } from '../../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../../wailsjs/runtime/runtime.js';
  import AddonCard from './AddonCard.svelte';
  import AddonGridTile from './AddonGridTile.svelte';

  const NEW_WINDOW_MS = 7 * 24 * 60 * 60 * 1000;
  const VIEW_MODE_KEY = 'archerage-browse-view';

  let allAddons = [];
  let categories = [];
  let selectedCategory = 'All';
  let searchQuery = '';
  let sortBy = 'newest';
  let viewMode = (typeof localStorage !== 'undefined' && localStorage.getItem(VIEW_MODE_KEY)) === 'grid' ? 'grid' : 'list';
  let loading = true;
  let initialized = false;

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

    out = out.map((a) => ({
      ...a,
      _isNew: isNew(a.submitted_at),
    }));

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
        out.sort((a, b) => {
          const ta = parseDate(a.submitted_at);
          const tb = parseDate(b.submitted_at);
          return tb - ta;
        });
    }
    return out;
  }

  function parseDate(s) {
    if (!s) return 0;
    const t = Date.parse(s);
    return isNaN(t) ? 0 : t;
  }

  function isNew(submittedAt) {
    const t = parseDate(submittedAt);
    if (!t) return false;
    return Date.now() - t < NEW_WINDOW_MS;
  }

  onMount(() => {
    loadCategories();

    const handleAddonChange = () => {
      if (initialized) loadAddons();
    };
    window.addEventListener('addon-installed', handleAddonChange);

    // Background registry refresh produced new data — re-pull from Go.
    const offRegistry = EventsOn('registry:refreshed', () => {
      if (initialized) loadAddons();
    });

    return () => {
      window.removeEventListener('addon-installed', handleAddonChange);
      offRegistry();
    };
  });

  $: if ($appInitialized && !initialized) {
    initialized = true;
    loadAddons();
  }

  async function loadCategories() {
    try {
      categories = await GetCategories();
    } catch (e) {
      console.error('Failed to load categories:', e);
    }
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
      console.error('Failed to refresh addons:', e);
      showNotification('Failed to refresh addons', 'error');
    }
    loading = false;
  }

  function openDetails(addon) {
    selectedAddon.set(addon);
    showAddonDetails.set(true);
  }

  $: newCount = filteredAddons.filter((a) => a._isNew).length;
</script>

<div class="h-full flex flex-col overflow-hidden app-canvas-ambient">
  <!-- Page header with title + actions -->
  <div class="px-6 pt-5 pb-4 pr-16 border-b border-border bg-header-grad">
    <div class="flex items-baseline justify-between mb-4">
      <div>
        <h1 class="text-xl font-bold text-text-primary tracking-tight">Browse</h1>
        <p class="text-xs text-text-muted mt-0.5">
          {#if !loading}
            {filteredAddons.length} {filteredAddons.length === 1 ? 'addon' : 'addons'}{#if newCount > 0 && sortBy === 'newest' && !searchQuery && selectedCategory === 'All'} · <span class="text-accent font-medium">{newCount} new this week</span>{/if}
          {:else}
            Loading the registry…
          {/if}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          on:click={handleRefresh}
          class="px-3 py-2 bg-bg-tertiary/60 hover:bg-bg-tertiary border border-border rounded-lg transition-all flex items-center gap-2 text-xs text-text-secondary hover:text-text-primary"
          title="Refresh registry"
        >
          <svg class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
          <span>Refresh</span>
        </button>

        <div class="flex bg-bg-tertiary/60 border border-border rounded-lg overflow-hidden p-0.5" role="group" aria-label="View mode">
          <button
            on:click={() => setViewMode('list')}
            class="p-1.5 rounded-md transition-all {viewMode === 'list' ? 'bg-bg-elevated text-accent shadow-soft' : 'text-text-muted hover:text-text-primary'}"
            title="List view"
            aria-pressed={viewMode === 'list'}
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
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
            class="p-1.5 rounded-md transition-all {viewMode === 'grid' ? 'bg-bg-elevated text-accent shadow-soft' : 'text-text-muted hover:text-text-primary'}"
            title="Grid view"
            aria-pressed={viewMode === 'grid'}
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7" rx="1"/>
              <rect x="14" y="3" width="7" height="7" rx="1"/>
              <rect x="14" y="14" width="7" height="7" rx="1"/>
              <rect x="3" y="14" width="7" height="7" rx="1"/>
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Search + filters row -->
    <div class="flex items-center gap-2">
      <div class="flex-1 relative">
        <input
          type="text"
          placeholder="Search by name, author, description, or keyword…"
          bind:value={searchQuery}
          class="w-full pl-10 pr-4 py-2.5 bg-bg-primary/60 border border-border hover:border-border-strong focus:border-accent rounded-xl focus:outline-none text-text-primary placeholder-text-muted text-sm transition-colors"
        />
        <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.35-4.35"/>
        </svg>
        {#if searchQuery}
          <button
            on:click={() => (searchQuery = '')}
            class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
            title="Clear search"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12"/></svg>
          </button>
        {/if}
      </div>

      <select
        bind:value={selectedCategory}
        class="px-3.5 py-2.5 bg-bg-primary/60 border border-border hover:border-border-strong rounded-xl focus:outline-none focus:border-accent text-text-primary text-sm min-w-[140px] transition-colors"
      >
        {#each categories as category}
          <option value={category}>{category}</option>
        {/each}
      </select>

      <select
        bind:value={sortBy}
        class="px-3.5 py-2.5 bg-bg-primary/60 border border-border hover:border-border-strong rounded-xl focus:outline-none focus:border-accent text-text-primary text-sm min-w-[150px] transition-colors"
        title="Sort"
      >
        <option value="newest">Newest first</option>
        <option value="installs">Most installed</option>
        <option value="rating">Highest rated</option>
        <option value="name">Name (A–Z)</option>
      </select>

    </div>
  </div>

  <div class="flex-1 overflow-y-auto px-6 py-5">
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="flex flex-col items-center gap-4">
          <svg class="animate-spin w-8 h-8 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          <span class="text-text-secondary text-sm">Loading addons…</span>
        </div>
      </div>
    {:else if filteredAddons.length === 0}
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary max-w-sm">
          <div class="mx-auto w-20 h-20 rounded-2xl bg-bg-secondary border border-border flex items-center justify-center mb-4">
            <svg class="w-9 h-9 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <circle cx="11" cy="11" r="8"/>
              <path d="m21 21-4.35-4.35"/>
            </svg>
          </div>
          <p class="text-base text-text-primary font-medium">No addons match</p>
          <p class="text-sm mt-1.5 text-text-muted">Try clearing the search or picking a different category.</p>
        </div>
      </div>
    {:else if viewMode === 'grid'}
      <div class="grid grid-cols-[repeat(auto-fill,minmax(170px,1fr))] gap-3">
        {#each filteredAddons as addon (addon.id)}
          <AddonGridTile {addon} on:click={() => openDetails(addon)} />
        {/each}
      </div>
    {:else}
      <div class="space-y-2">
        {#each filteredAddons as addon (addon.id)}
          <AddonCard {addon} on:click={() => openDetails(addon)} on:refresh={loadAddons} />
        {/each}
      </div>
    {/if}
  </div>
</div>
