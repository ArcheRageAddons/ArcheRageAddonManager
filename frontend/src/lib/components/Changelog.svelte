<script>
  import { onMount } from 'svelte';
  import { GetReleaseHistory, OpenURL } from '../../../wailsjs/go/main/App.js';
  import { showNotification } from '../stores/app.js';

  let releases = [];
  let loading = true;
  let error = '';

  onMount(load);

  async function load() {
    loading = true;
    error = '';
    try {
      releases = (await GetReleaseHistory()) || [];
    } catch (e) {
      console.error('GetReleaseHistory failed:', e);
      error = String(e);
      releases = [];
    }
    loading = false;
  }

  function formatDate(s) {
    if (!s) return '';
    const t = Date.parse(s);
    if (isNaN(t)) return s;
    return new Date(t).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  function openOnGitHub(url) {
    if (!url) return;
    OpenURL(url).catch((e) => {
      console.error('OpenURL failed:', e);
      showNotification(`Couldn't open the release page: ${e}`, 'error');
    });
  }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <div class="flex justify-between items-center p-4 pr-16 border-b border-border bg-bg-secondary">
    <div>
      <h2 class="text-lg font-bold text-text-primary">Changelog</h2>
      <p class="text-xs text-text-muted mt-0.5">What's changed in each release of the addon manager.</p>
    </div>
    <button
      on:click={load}
      disabled={loading}
      class="px-4 py-2.5 bg-bg-tertiary hover:bg-border rounded-lg transition-colors flex items-center gap-2 text-sm text-text-secondary disabled:opacity-60"
      title="Re-fetch from GitHub"
    >
      <svg class="w-4 h-4 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M23 4v6h-6M1 20v-6h6"/>
        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
      </svg>
      <span>Refresh</span>
    </button>
  </div>

  <div class="flex-1 overflow-y-auto p-4">
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="flex flex-col items-center gap-4">
          <svg class="animate-spin w-8 h-8 text-accent" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          <span class="text-text-secondary text-sm">Loading release history…</span>
        </div>
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary max-w-md">
          <p class="text-warning mb-2">Couldn't load the changelog.</p>
          <p class="text-xs text-text-muted whitespace-pre-wrap">{error}</p>
          <p class="text-xs text-text-muted mt-3">GitHub's API may be rate-limiting unauthenticated requests. Sign in to GitHub from <strong>My Addons</strong> for a higher limit, or try again in a few minutes.</p>
        </div>
      </div>
    {:else if releases.length === 0}
      <div class="flex items-center justify-center h-full">
        <p class="text-text-secondary text-sm">No releases yet.</p>
      </div>
    {:else}
      <div class="max-w-3xl mx-auto space-y-3">
        {#each releases as r (r.version)}
          <div class="bg-bg-secondary border border-border rounded-lg p-4">
            <div class="flex items-baseline justify-between gap-3 mb-2">
              <div class="flex items-baseline gap-3 min-w-0">
                <span class="text-base font-bold text-accent">{r.version}</span>
                <span class="text-xs text-text-muted">{formatDate(r.published_at)}</span>
              </div>
              <button
                on:click={() => openOnGitHub(r.url)}
                class="text-xs text-text-muted hover:text-text-primary flex items-center gap-1 flex-shrink-0"
                title="View on GitHub"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.34-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.78 1.19 1.78 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.79.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/>
                </svg>
                GitHub
              </button>
            </div>
            {#if r.body && r.body.trim()}
              <pre class="text-xs text-text-secondary whitespace-pre-wrap font-sans leading-relaxed">{r.body}</pre>
            {:else}
              <p class="text-xs text-text-muted italic">No release notes for this version.</p>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
