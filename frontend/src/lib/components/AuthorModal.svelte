<script>
  import {
    showAuthorModal, selectedAuthor,
    selectedAddon, showAddonDetails,
  } from '../stores/app.js';
  import { GetAddons, OpenURL } from '../../../wailsjs/go/main/App.js';

  let allAddons = [];
  let loading = false;
  let lastFetchedFor = null;

  $: if ($showAuthorModal && $selectedAuthor && lastFetchedFor !== $selectedAuthor) {
    lastFetchedFor = $selectedAuthor;
    loadAddons();
  } else if (!$showAuthorModal) {
    lastFetchedFor = null;
  }

  async function loadAddons() {
    loading = true;
    try {
      allAddons = (await GetAddons()) || [];
    } catch (e) {
      console.error('Failed to fetch addons for author modal:', e);
      allAddons = [];
    }
    loading = false;
  }

  $: byThisAuthor = (() => {
    const target = ($selectedAuthor || '').trim().toLowerCase();
    if (!target) return [];
    return allAddons.filter(
      (a) => (a.author_name || '').trim().toLowerCase() === target,
    );
  })();

  // First verified login wins — collisions on the free-text display name
  // are rare and not worth UI gymnastics.
  $: verifiedGithub = byThisAuthor.find((a) => a.submitter_github)?.submitter_github || '';

  $: totalDownloads = byThisAuthor.reduce((sum, a) => sum + (a.download_count || 0), 0);

  function close() {
    showAuthorModal.set(false);
    selectedAuthor.set('');
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) close();
  }

  function openAddon(addon) {
    selectedAddon.set(addon);
    showAddonDetails.set(true);
    close();
  }

  function openGithubProfile() {
    if (verifiedGithub) {
      OpenURL(`https://github.com/${verifiedGithub}`).catch(console.error);
    }
  }

  function formatCount(n) {
    if (!n || n < 1) return '0';
    if (n < 1000) return String(n);
    if (n < 1_000_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
    return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  }
</script>

{#if $showAuthorModal && $selectedAuthor}
  <div
    class="fixed inset-0 bg-black/70 flex items-center justify-center z-50 p-4"
    on:click={handleBackdropClick}
    on:keydown={(e) => e.key === 'Escape' && close()}
    tabindex="-1"
  >
    <div class="bg-bg-secondary border border-border rounded-xl max-w-2xl w-full max-h-[80vh] overflow-hidden flex flex-col shadow-2xl">
      <!-- Header -->
      <div class="p-5 border-b border-border flex justify-between items-start">
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h2 class="text-lg font-bold text-text-primary truncate">{$selectedAuthor}</h2>
            {#if verifiedGithub}
              <button
                on:click={openGithubProfile}
                class="text-accent hover:text-accent-hover flex items-center gap-1 text-xs"
                title="Open verified GitHub profile"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.34-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.78 1.19 1.78 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.79.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/>
                </svg>
                @{verifiedGithub}
              </button>
            {/if}
          </div>
          {#if !loading && byThisAuthor.length > 0}
            <p class="text-sm text-text-secondary mt-1">
              {byThisAuthor.length} addon{byThisAuthor.length === 1 ? '' : 's'}
              {#if totalDownloads > 0} · {formatCount(totalDownloads)} total install{totalDownloads === 1 ? '' : 's'}{/if}
            </p>
          {/if}
        </div>
        <button on:click={close} class="p-1.5 hover:bg-bg-tertiary rounded-lg transition-colors flex-shrink-0">
          <svg class="w-5 h-5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>

      <!-- Addon list -->
      <div class="flex-1 overflow-y-auto p-3">
        {#if loading}
          <div class="flex items-center justify-center py-12">
            <svg class="animate-spin w-6 h-6 text-accent" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
          </div>
        {:else if byThisAuthor.length === 0}
          <div class="text-center py-12 text-text-secondary">
            <p class="text-sm">No addons published under this name.</p>
          </div>
        {:else}
          <div class="space-y-1">
            {#each byThisAuthor as addon (addon.id)}
              <button
                on:click={() => openAddon(addon)}
                class="w-full bg-bg-tertiary hover:bg-bg-primary rounded-lg px-3 py-2.5 flex items-center gap-3 transition-colors text-left"
              >
                <div class="w-9 h-9 rounded-lg bg-bg-primary flex items-center justify-center flex-shrink-0 overflow-hidden">
                  {#if addon.icon}
                    <img src={addon.icon} alt={addon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
                  {:else}
                    <svg class="w-4 h-4 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                    </svg>
                  {/if}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-medium text-text-primary truncate">{addon.name}</span>
                    <span class="text-xs text-text-muted flex-shrink-0">v{addon.version}</span>
                    {#if addon.is_installed}
                      <svg class="w-3.5 h-3.5 text-success flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" title="Installed">
                        <path d="M20 6L9 17l-5-5"/>
                      </svg>
                    {/if}
                  </div>
                  <div class="flex items-center gap-3 text-xs text-text-muted mt-0.5">
                    <span>{addon.category}</span>
                    {#if addon.download_count > 0}
                      <span>{formatCount(addon.download_count)} install{addon.download_count === 1 ? '' : 's'}</span>
                    {/if}
                    {#if addon.rating_count > 0}
                      <span>★ {addon.rating_avg.toFixed(1)}</span>
                    {/if}
                  </div>
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
