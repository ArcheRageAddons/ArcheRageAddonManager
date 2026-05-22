<script>
  import { selectedAddon, showAddonDetails, showNotification, warningAddon, showWarningModal, downloadProgress, currentUser, uninstallAddon, showUninstallConfirm, selectedAuthor, showAuthorModal, kickOffInstall, installSerially } from '../stores/app.js';
  import {
    OpenURL,
    OpenReadmeLink,
    GetMyRating,
    SetAddonRating,
    ClearAddonRating,
    GetAddonSize,
    GetAddonDetails,
    GetAddonReadme,
    GetAddonCommitHistory,
  } from '../../../wailsjs/go/main/App.js';
  import { modalBackdrop, modalContent } from '../motion.js';
  import { renderReadme } from '../markdown.js';
  import { slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';

  let myRating = 0;       // 0 = not rated by this user
  let hoverRating = 0;
  let rateBusy = false;
  let depBulkBusy = false;
  let downloadSize = null; // null=not yet fetched, -1=unknown, 0+=bytes
  let sizeFetchedFor = null;
  let readmeHTML = '';
  let readmeFetchedFor = null;
  let commits = [];                 // [] = not fetched yet; non-empty once GetAddonCommitHistory resolves
  let commitsFetchedFor = null;
  let commitsExpanded = false;

  $: isInstalledNoUpdate = !!($selectedAddon && $selectedAddon.is_installed && !$selectedAddon.has_update);

  $: baseMissing = !!($selectedAddon?.overlay_of && !$selectedAddon.base_installed && !isInstalledNoUpdate);
  $: depsMissing = !isInstalledNoUpdate && missingDeps.length > 0;
  $: blockReason = baseMissing
    ? `Install ${$selectedAddon.overlay_of} first — this addon overlays on top of it`
    : depsMissing
      ? `Install missing dependenc${missingDeps.length === 1 ? 'y' : 'ies'} first: ${missingDeps.map((d) => d.name).join(', ')}`
      : '';

  $: missingDeps = ($selectedAddon?.dependencies || []).filter((d) => !d.is_installed);
  $: installedDeps = ($selectedAddon?.dependencies || []).filter((d) => d.is_installed);

  $: if ($showAddonDetails && $selectedAddon && sizeFetchedFor !== $selectedAddon.id) {
    sizeFetchedFor = $selectedAddon.id;
    downloadSize = null;
    fetchSize($selectedAddon.id);
  } else if (!$showAddonDetails) {
    sizeFetchedFor = null;
    downloadSize = null;
  }

  $: if ($showAddonDetails && $selectedAddon && readmeFetchedFor !== $selectedAddon.id) {
    readmeFetchedFor = $selectedAddon.id;
    readmeHTML = '';
    fetchReadme($selectedAddon.id);
  } else if (!$showAddonDetails) {
    readmeFetchedFor = null;
    readmeHTML = '';
  }

  $: if ($showAddonDetails && $selectedAddon && commitsFetchedFor !== $selectedAddon.id) {
    commitsFetchedFor = $selectedAddon.id;
    commits = [];
    commitsExpanded = false;
    fetchCommits($selectedAddon.id);
  } else if (!$showAddonDetails) {
    commitsFetchedFor = null;
    commits = [];
    commitsExpanded = false;
  }

  async function fetchSize(id) {
    try {
      const bytes = await GetAddonSize(id);
      // Drop result if user clicked through to another addon meanwhile.
      if (sizeFetchedFor === id) {
        downloadSize = bytes;
      }
    } catch {
      if (sizeFetchedFor === id) downloadSize = -1;
    }
  }

  async function fetchReadme(id) {
    try {
      const result = await GetAddonReadme(id);
      if (readmeFetchedFor !== id) return; // user moved on
      if (result?.markdown) {
        readmeHTML = renderReadme(result.markdown, result.base_url || '');
      }
    } catch (e) {
      console.warn('README fetch failed:', e);
    }
  }

  async function fetchCommits(id) {
    try {
      const result = await GetAddonCommitHistory(id);
      if (commitsFetchedFor !== id) return; // user moved on
      commits = result || [];
    } catch (e) {
      console.warn('Commit history fetch failed:', e);
    }
  }

  function formatCommitDate(s) {
    if (!s) return '';
    const t = Date.parse(s);
    if (isNaN(t)) return s;
    return new Date(t).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  function openCommit(url) {
    if (!url) return;
    OpenReadmeLink(url).catch((err) => console.error('OpenReadmeLink failed:', err));
  }

  function handleReadmeClick(e) {
    const link = e.target.closest('a[data-readme-link]');
    if (!link) return;
    e.preventDefault();
    const href = link.getAttribute('href');
    if (!href) return;
    OpenReadmeLink(href).catch((err) => console.error('OpenReadmeLink failed:', err));
  }

  function formatBytes(n) {
    if (n == null) return '…';
    if (n < 0) return '—';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(0) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
    return (n / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
  }

  $: if ($showAddonDetails && $selectedAddon && $currentUser) {
    loadMyRating($selectedAddon.id);
  } else if (!$showAddonDetails) {
    myRating = 0;
    hoverRating = 0;
  }

  async function loadMyRating(slug) {
    try {
      myRating = await GetMyRating(slug) || 0;
    } catch {
      myRating = 0;
    }
  }

  async function rate(n) {
    if (!$currentUser || !$selectedAddon) return;
    rateBusy = true;
    try {
      if (n === myRating) {
        await ClearAddonRating($selectedAddon.id);
        myRating = 0;
        showNotification('Rating cleared', 'info', 2500);
      } else {
        await SetAddonRating($selectedAddon.id, n);
        myRating = n;
        showNotification(`Rated ${n} / 5`, 'success', 2500);
      }
    } catch (e) {
      showNotification(`Couldn't save rating: ${e}`, 'error', 6000);
    }
    rateBusy = false;
  }

  function close() {
    showAddonDetails.set(false);
    selectedAddon.set(null);
  }

  function openAuthor() {
    if (!$selectedAddon?.author_name) return;
    selectedAuthor.set($selectedAddon.author_name);
    showAuthorModal.set(true);
    close();
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  async function handlePrimaryAction() {
    if ($selectedAddon.is_installed && !$selectedAddon.has_update) {
      uninstallAddon.set($selectedAddon);
      showUninstallConfirm.set(true);
      close();
      return;
    }

    if (missingDeps.length > 0) {
      showNotification(
        `Install the missing dependenc${missingDeps.length === 1 ? 'y' : 'ies'} first (${missingDeps.map((d) => d.name).join(', ')})`,
        'warning', 5000,
      );
      return;
    }

    if ($selectedAddon.has_dangerous_files) {
      warningAddon.set($selectedAddon);
      showWarningModal.set(true);
      return;
    }

    await performDownload();
  }

  async function performDownload() {
    kickOffInstall($selectedAddon);
    setTimeout(close, 500);
  }

  async function openGitHub() {
    try {
      const url = `https://github.com/${$selectedAddon.github_repo_url}`;
      await OpenURL(url);
    } catch (e) {
      console.error('Failed to open URL:', e);
    }
  }

  async function installMissingDeps() {
    if (depBulkBusy) return;
    const missing = missingDeps.slice().map((d) => ({ id: d.id, name: d.name }));
    if (missing.length === 0) return;

    depBulkBusy = true;
    const { ok, failed } = await installSerially(missing, {
      label: (i, n) => `dependency ${i}/${n}`,
    });
    depBulkBusy = false;

    if ($selectedAddon) {
      try {
        const fresh = await GetAddonDetails($selectedAddon.id);
        if (fresh) selectedAddon.set(fresh);
      } catch (e) {
        console.warn('failed to refresh addon details after dep install:', e);
      }
    }

    if (failed === 0) {
      showNotification(`Installed ${ok} missing dependenc${ok === 1 ? 'y' : 'ies'}`, 'success');
    } else {
      showNotification(`Installed ${ok} of ${missing.length} (${failed} failed)`, failed === missing.length ? 'error' : 'info');
    }
  }
</script>

{#if $showAddonDetails && $selectedAddon}
  <div
    class="fixed inset-0 bg-black/70 flex items-center justify-center z-50 p-4"
    on:click={handleBackdropClick}
    on:keydown={(e) => e.key === 'Escape' && close()}
    tabindex="-1"
    transition:modalBackdrop
  >
    <div class="bg-bg-secondary border border-border rounded-xl max-w-2xl w-full max-h-[80vh] overflow-hidden flex flex-col shadow-2xl" transition:modalContent>
      <!-- Header -->
      <div class="p-5 border-b border-border flex justify-between items-start gap-4">
        <div class="flex items-center gap-4 min-w-0">
          <div class="w-14 h-14 rounded-xl bg-bg-tertiary flex items-center justify-center flex-shrink-0 overflow-hidden">
            {#if $selectedAddon.icon}
              <img src={$selectedAddon.icon} alt={$selectedAddon.name} referrerpolicy="no-referrer" class="w-full h-full object-cover" />
            {:else}
              <svg class="w-7 h-7 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
              </svg>
            {/if}
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-3">
              <h2 class="text-lg font-bold text-text-primary truncate">{$selectedAddon.name}</h2>
              <span class="text-sm text-text-muted flex-shrink-0">v{$selectedAddon.version}</span>
            </div>
            <p class="text-sm text-text-secondary mt-1">
              by
              <button
                on:click={openAuthor}
                class="text-text-secondary hover:text-accent hover:underline transition-colors"
                title="See all addons by this author"
              >
                {$selectedAddon.author_name}
              </button>
            </p>
          </div>
        </div>
        <button
          on:click={close}
          class="p-1.5 hover:bg-bg-tertiary rounded-lg transition-colors flex-shrink-0"
        >
          <svg class="w-5 h-5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>

      <!-- Content -->
      <div class="p-5 overflow-y-auto flex-1">
        <div class="space-y-4">
          <!-- Category, stats, GitHub link -->
          <div class="flex flex-wrap gap-3 text-sm items-center">
            <span class="px-2.5 py-1 bg-tag-bg text-text-secondary rounded-md">{$selectedAddon.category}</span>
            <span class="flex items-center gap-1.5 text-text-muted text-xs" title="Approximate uncompressed size of the addon's files. The actual download (compressed zip) is typically smaller.">
              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 12H2m20 0-7-7m7 7-7 7"/>
                <path d="M6 6h.01M6 18h.01"/>
              </svg>
              {formatBytes(downloadSize)}
            </span>
            {#if $selectedAddon.download_count > 0}
              <span class="flex items-center gap-1.5 text-text-muted text-xs" title="{$selectedAddon.download_count.toLocaleString()} install{$selectedAddon.download_count === 1 ? '' : 's'}">
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
                </svg>
                {$selectedAddon.download_count.toLocaleString()}
              </span>
            {/if}
            {#if $selectedAddon.rating_count > 0}
              <span class="flex items-center gap-1.5 text-text-muted text-xs" title="{$selectedAddon.rating_avg.toFixed(2)} from {$selectedAddon.rating_count} rating{$selectedAddon.rating_count === 1 ? '' : 's'}">
                <svg class="w-3.5 h-3.5 text-warning" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                </svg>
                {$selectedAddon.rating_avg.toFixed(1)}
                <span class="text-text-muted">({$selectedAddon.rating_count})</span>
              </span>
            {/if}
            {#if $selectedAddon.github_repo_url}
              <button
                on:click={openGitHub}
                class="flex items-center gap-1.5 text-text-muted hover:text-accent transition-colors"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                </svg>
                View on GitHub
              </button>
            {/if}
          </div>

          <!-- Rating picker (only shown when signed in) -->
          {#if $currentUser}
            <div class="flex items-center gap-2 text-sm">
              <span class="text-text-secondary">Your rating:</span>
              {#each [1, 2, 3, 4, 5] as n}
                {@const filled = (hoverRating || myRating) >= n}
                <button
                  on:click={() => rate(n)}
                  on:mouseenter={() => (hoverRating = n)}
                  on:mouseleave={() => (hoverRating = 0)}
                  disabled={rateBusy}
                  class="p-0.5 transition-colors disabled:opacity-50"
                  title={n === myRating ? 'Click to clear your rating' : `Rate ${n} / 5`}
                >
                  <svg class="w-5 h-5 {filled ? 'text-warning' : 'text-text-muted'}" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                  </svg>
                </button>
              {/each}
              {#if myRating > 0}
                <span class="text-xs text-text-muted ml-1">— click again to clear</span>
              {/if}
            </div>
          {/if}

          <!-- Changelog (commit history of the addon's source path) -->
          {#if commits.length > 0}
            <div>
              <h3 class="text-sm font-medium text-text-primary mb-2">Changelog</h3>
              <div class="bg-bg-tertiary border border-border rounded-lg overflow-hidden">
                <!-- Latest commit (always visible) -->
                <div class="px-3 py-2.5">
                  <div class="flex items-baseline justify-between gap-3 mb-1">
                    <div class="flex items-baseline gap-2 min-w-0">
                      <span class="text-[10px] font-bold uppercase tracking-wider text-accent bg-accent/15 px-1.5 py-0.5 rounded">Latest</span>
                      <span class="font-mono text-[11px] text-text-muted">{commits[0].short_sha}</span>
                      <span class="text-[11px] text-text-muted">{formatCommitDate(commits[0].date)}</span>
                    </div>
                    <button
                      on:click={() => openCommit(commits[0].url)}
                      class="text-[11px] text-text-muted hover:text-text-primary flex-shrink-0"
                      title="View commit on GitHub"
                    >
                      GitHub →
                    </button>
                  </div>
                  <pre class="text-xs text-text-secondary whitespace-pre-wrap font-sans leading-relaxed">{commits[0].message}</pre>
                </div>

                {#if commits.length > 1}
                  <button
                    on:click={() => (commitsExpanded = !commitsExpanded)}
                    class="w-full px-3 py-1.5 border-t border-border text-xs text-text-secondary hover:text-text-primary hover:bg-bg-secondary flex items-center justify-center gap-1.5 transition-colors"
                  >
                    <span>{commitsExpanded ? 'Hide' : 'Show'} {commits.length - 1} older commit{commits.length - 1 === 1 ? '' : 's'}</span>
                    <svg
                      class="w-3.5 h-3.5 transition-transform duration-200"
                      style="transform: rotate({commitsExpanded ? 180 : 0}deg);"
                      viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
                    >
                      <path d="M6 9l6 6 6-6"/>
                    </svg>
                  </button>

                  {#if commitsExpanded}
                    <div
                      class="border-t border-border max-h-[40vh] overflow-y-auto"
                      transition:slide={{ duration: 200, easing: cubicOut }}
                    >
                      {#each commits.slice(1) as c (c.sha)}
                        <div class="px-3 py-2.5 border-b border-border last:border-b-0">
                          <div class="flex items-baseline justify-between gap-3 mb-1">
                            <div class="flex items-baseline gap-2 min-w-0">
                              <span class="font-mono text-[11px] text-text-muted">{c.short_sha}</span>
                              <span class="text-[11px] text-text-muted">{formatCommitDate(c.date)}</span>
                              {#if c.author}
                                <span class="text-[11px] text-text-muted truncate">· {c.author}</span>
                              {/if}
                            </div>
                            <button
                              on:click={() => openCommit(c.url)}
                              class="text-[11px] text-text-muted hover:text-text-primary flex-shrink-0"
                              title="View commit on GitHub"
                            >
                              GitHub →
                            </button>
                          </div>
                          <pre class="text-xs text-text-secondary whitespace-pre-wrap font-sans leading-relaxed">{c.message}</pre>
                        </div>
                      {/each}
                    </div>
                  {/if}
                {/if}
              </div>
            </div>
          {/if}

          <!-- Submitter trust strip -->
          {#if $selectedAddon.submitter_discord || $selectedAddon.submitter_github}
            <div class="bg-bg-tertiary/40 border border-border rounded-lg px-3 py-2 flex items-center flex-wrap gap-x-3 gap-y-1 text-xs">
              <span class="text-text-muted">Submitted by</span>
              {#if $selectedAddon.submitter_discord}
                <span class="text-text-primary font-medium">
                  @{$selectedAddon.submitter_discord} <span class="text-text-muted font-normal">on Discord</span>
                </span>
              {/if}
              {#if $selectedAddon.submitter_github}
                <button
                  on:click={() => OpenURL(`https://github.com/${$selectedAddon.submitter_github}`)}
                  class="text-accent hover:text-accent-hover flex items-center gap-1"
                  title="Open submitter's GitHub profile"
                >
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.34-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.78 1.19 1.78 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.79.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/>
                  </svg>
                  @{$selectedAddon.submitter_github}
                </button>
              {/if}
              {#if $selectedAddon.submitted_at}
                <span class="text-text-muted ml-auto">
                  {new Date($selectedAddon.submitted_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}
                </span>
              {/if}
            </div>
          {/if}

          {#if $selectedAddon.overlay_of}
            <div class="bg-bg-tertiary/40 border border-border rounded-lg px-3 py-2 flex items-center gap-2 text-xs">
              <svg class="w-4 h-4 text-accent flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16zM3.27 6.96L12 12.01l8.73-5.05M12 22.08V12"/>
              </svg>
              <span class="text-text-secondary">
                Overlays on top of <span class="text-text-primary font-medium">{$selectedAddon.overlay_of}</span>
              </span>
              {#if $selectedAddon.base_installed}
                <span class="ml-auto text-success font-medium">Base installed ✓</span>
              {:else}
                <span class="ml-auto text-warning font-medium">Base required — install {$selectedAddon.overlay_of} first</span>
              {/if}
            </div>
          {/if}

          <!-- Description (README if author shipped one, else YAML description) -->
          <div>
            <h3 class="text-sm font-medium text-text-primary mb-2">Description</h3>
            {#if readmeHTML}
              <div
                class="markdown-body text-sm text-text-secondary leading-relaxed max-h-[50vh] overflow-y-auto pr-2"
                on:click={handleReadmeClick}
              >
                {@html readmeHTML}
              </div>
            {:else}
              <p class="text-sm text-text-secondary whitespace-pre-wrap leading-relaxed">
                {$selectedAddon.description || 'No description provided.'}
              </p>
            {/if}
          </div>

          <!-- Keywords -->
          {#if $selectedAddon.keywords && $selectedAddon.keywords.length > 0}
            <div>
              <h3 class="text-sm font-medium text-text-primary mb-2">Keywords</h3>
              <div class="flex flex-wrap gap-2">
                {#each $selectedAddon.keywords as keyword}
                  <span class="px-2 py-0.5 bg-bg-tertiary text-text-muted rounded text-xs">{keyword}</span>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Dependencies -->
          {#if $selectedAddon.dependencies && $selectedAddon.dependencies.length > 0}
            <div>
              <div class="flex items-center justify-between mb-2">
                <h3 class="text-sm font-medium text-text-primary">
                  Dependencies
                  <span class="text-text-muted font-normal">({installedDeps.length}/{$selectedAddon.dependencies.length} installed)</span>
                </h3>
                {#if missingDeps.length > 0}
                  <button
                    on:click={installMissingDeps}
                    disabled={depBulkBusy || $downloadProgress.isDownloading}
                    class="px-3 py-1 bg-accent hover:bg-accent-hover text-white rounded-md text-xs transition-colors disabled:opacity-60 disabled:cursor-not-allowed flex items-center gap-1.5"
                  >
                    {#if depBulkBusy}
                      <svg class="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                      </svg>
                      <span>Installing…</span>
                    {:else}
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
                      </svg>
                      <span>Install missing ({missingDeps.length})</span>
                    {/if}
                  </button>
                {/if}
              </div>

              {#if missingDeps.length > 0}
                <div class="text-xs uppercase tracking-wide text-text-muted mb-1">Missing</div>
                <div class="space-y-2 mb-3">
                  {#each missingDeps as dep}
                    <div class="flex items-center gap-2 px-3 py-2 bg-red-500/5 border border-red-500/20 rounded-lg">
                      <svg class="w-4 h-4 text-red-500 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <path d="M18 6L6 18M6 6l12 12"/>
                      </svg>
                      <span class="text-sm text-text-primary">{dep.name}</span>
                      <span class="text-xs text-red-400 ml-auto">Not installed</span>
                    </div>
                  {/each}
                </div>
              {/if}

              {#if installedDeps.length > 0}
                <div class="text-xs uppercase tracking-wide text-text-muted mb-1">Installed</div>
                <div class="space-y-2">
                  {#each installedDeps as dep}
                    <div class="flex items-center gap-2 px-3 py-2 bg-bg-tertiary rounded-lg">
                      <svg class="w-4 h-4 text-success flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <path d="M20 6L9 17l-5-5"/>
                      </svg>
                      <span class="text-sm text-text-primary">{dep.name}</span>
                      <span class="text-xs text-success ml-auto">Installed</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}

          <!-- Warning -->
          {#if $selectedAddon.has_dangerous_files}
            <div class="p-4 bg-warning/10 border border-warning/20 rounded-lg flex items-start gap-3">
              <svg class="w-5 h-5 text-warning flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
              </svg>
              <div>
                <p class="text-sm font-medium text-warning">Contains executable files</p>
                <p class="text-xs text-text-muted mt-1">
                  This addon contains executable or script files (e.g. .bat / .exe / .dll / .lnk / .ps1). Only install if you trust the author.
                </p>
              </div>
            </div>
          {/if}

          <!-- Status -->
          {#if $selectedAddon.is_installed}
            <div class="p-4 bg-success/10 border border-success/20 rounded-lg flex items-center gap-3">
              <svg class="w-5 h-5 text-success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M20 6L9 17l-5-5"/>
              </svg>
              <span class="text-sm text-success">Installed</span>
              {#if $selectedAddon.has_update}
                <span class="px-2 py-1 bg-warning/20 text-warning text-xs rounded-md ml-auto">Update Available</span>
              {/if}
            </div>
          {/if}
        </div>
      </div>

      <!-- Footer -->
      <div class="p-5 border-t border-border">
        <!-- Buttons -->
        <div class="flex justify-end gap-3">
          <button
            on:click={close}
            disabled={$downloadProgress.isDownloading}
            class="px-4 py-2 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-sm text-text-secondary disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Close
          </button>
          <button
            on:click={handlePrimaryAction}
            disabled={$downloadProgress.isDownloading || baseMissing || depsMissing}
            title={blockReason}
            class="px-5 py-2 {isInstalledNoUpdate ? 'bg-red-500 hover:bg-red-600' : 'bg-accent hover:bg-accent-hover'} text-white rounded-lg transition-colors disabled:opacity-50 flex items-center gap-2 text-sm disabled:cursor-not-allowed"
          >
            {#if $downloadProgress.isDownloading && $downloadProgress.addonId === $selectedAddon?.id}
              <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
              </svg>
            {/if}
            {$selectedAddon.is_installed ? ($selectedAddon.has_update ? 'Update' : 'Uninstall') : 'Download'}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
