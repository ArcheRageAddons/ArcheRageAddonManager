<script>
  import { onMount, onDestroy } from 'svelte';
  import { CheckForUpdate, OpenURL, InstallUpdate } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { showNotification } from '../stores/app.js';
  import { modalBackdrop, modalContent, bannerSlide } from '../motion.js';
  import { slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';

  let update = null;
  let dismissedVersion = null;
  let expanded = false;

  let phase = 'idle'; // idle | confirming | downloading | done | error
  let progressCurrent = 0;
  let progressTotal = 0;
  let progressMessage = '';
  let errorMessage = '';

  function applyInfo(info) {
    if (!info) {
      update = null;
      expanded = false;
      return;
    }
    // Collapse on a new version landing so the next banner appears compact.
    if (!update || update.version !== info.version) {
      expanded = false;
    }
    update = info;
  }

  onMount(async () => {
    try {
      const info = await CheckForUpdate();
      if (info) applyInfo(info);
    } catch (e) {
      console.warn('[update] initial check failed:', e);
    }

    EventsOn('update:available', (info) => applyInfo(info));

    EventsOn('update:download:progress', (data) => {
      progressCurrent = data?.current || 0;
      progressTotal = data?.total || 0;
      progressMessage = data?.message || '';
    });

    EventsOn('update:download:complete', (result) => {
      if (result?.success) {
        phase = 'done';
        progressMessage = 'Update applied — restarting…';
      } else {
        phase = 'error';
        errorMessage = result?.error || 'Update failed';
      }
    });
  });

  onDestroy(() => {
    EventsOff('update:available');
    EventsOff('update:download:progress');
    EventsOff('update:download:complete');
  });

  function dismiss() {
    if (update) dismissedVersion = update.version;
  }

  function openSource() {
    if (update) OpenURL(update.source_url).catch(console.error);
  }

  function openReleasePage() {
    if (update) OpenURL(update.url).catch(console.error);
  }

  function startUpdate() {
    if (!update) return;
    if (!update.asset_url) {
      // Pre-self-update release with no .exe asset — manual flow.
      OpenURL(update.url).catch(console.error);
      return;
    }
    phase = 'confirming';
  }

  async function confirmAndInstall() {
    phase = 'downloading';
    progressCurrent = 0;
    progressTotal = update.asset_size || 0;
    progressMessage = 'Starting download...';
    errorMessage = '';

    try {
      // App exits on success; the update:download:complete handler flips
      // phase if we're still here (i.e. the install failed).
      await InstallUpdate(update.asset_url);
    } catch (e) {
      phase = 'error';
      errorMessage = String(e);
      showNotification(`Update failed: ${errorMessage}`, 'error', 8000);
    }
  }

  function cancelConfirm() {
    if (phase === 'confirming') phase = 'idle';
  }

  function dismissError() {
    phase = 'idle';
    errorMessage = '';
  }

  $: visible = update && update.version !== dismissedVersion;
  $: percent = progressTotal > 0 ? Math.min(100, Math.round((progressCurrent / progressTotal) * 100)) : 0;
  $: hasBody = !!(update?.body && update.body.trim().length > 0);
</script>

{#if visible}
  <div class="bg-accent/15 border-b border-accent/40 flex-shrink-0 flex flex-col" transition:bannerSlide>
    <div class="px-4 py-2 flex items-center gap-3">
      <svg class="w-4 h-4 text-accent flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
      </svg>
      <div class="flex-1 text-sm text-text-primary truncate">
        <strong class="text-accent">{update.version}</strong> is available — click <strong>Install now</strong> to update.
      </div>
      {#if hasBody}
        <button
          on:click={() => (expanded = !expanded)}
          class="px-2 py-1 text-text-secondary hover:text-text-primary text-xs font-medium flex items-center gap-1.5"
          aria-expanded={expanded}
        >
          <span>{expanded ? 'Hide Changes' : 'Show Changes'}</span>
          <svg
            class="w-3.5 h-3.5 transition-transform duration-200"
            style="transform: rotate({expanded ? 180 : 0}deg);"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <path d="M6 9l6 6 6-6"/>
          </svg>
        </button>
      {/if}
      <button
        on:click={startUpdate}
        class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white rounded text-xs font-medium"
        title={update.asset_url ? 'Download and install the update — the app will close and reopen' : 'No .exe asset on this release; opens the release page'}
      >
        {update.asset_url ? 'Install now' : 'Download'}
      </button>
      <button
        on:click={openSource}
        class="px-3 py-1.5 bg-bg-tertiary hover:bg-border text-text-secondary rounded text-xs font-medium flex items-center gap-1.5"
        title="View source code at this release"
      >
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.34-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.78 1.19 1.78 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.79.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/>
        </svg>
        GitHub
      </button>
      <button
        on:click={dismiss}
        title="Dismiss for this session"
        class="text-text-muted hover:text-text-primary p-1"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 6 6 18M6 6l12 12"/>
        </svg>
      </button>
    </div>

    {#if expanded && hasBody}
      <div
        class="border-t border-accent/20 px-4 py-3 max-h-[40vh] overflow-y-auto"
        transition:slide={{ duration: 200, easing: cubicOut }}
      >
        <pre class="text-xs text-text-secondary whitespace-pre-wrap font-sans leading-relaxed">{update.body}</pre>
      </div>
    {/if}
  </div>
{/if}

{#if phase === 'confirming' && update}
  <div class="fixed inset-0 bg-black/70 flex items-center justify-center z-[100] p-4" transition:modalBackdrop>
    <div class="bg-bg-secondary border border-border rounded-xl max-w-md w-full p-5 shadow-2xl" transition:modalContent>
      <h3 class="text-lg font-bold text-text-primary mb-2">Install {update.version}?</h3>
      <p class="text-sm text-text-secondary leading-relaxed">
        The manager will download the new build (~{update.asset_size ? (update.asset_size / 1024 / 1024).toFixed(1) + ' MB' : 'a few MB'}), close itself, and reopen on the new version. Make sure no addon downloads are in progress.
      </p>
      <div class="flex justify-end gap-2 mt-4">
        <button
          on:click={cancelConfirm}
          class="px-4 py-2 bg-bg-tertiary hover:bg-border rounded-lg text-sm text-text-secondary"
        >
          Cancel
        </button>
        <button
          on:click={confirmAndInstall}
          class="px-4 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium"
        >
          Install now
        </button>
      </div>
    </div>
  </div>
{/if}

{#if phase === 'downloading' || phase === 'done'}
  <div class="fixed inset-0 bg-black/70 flex items-center justify-center z-[100] p-4" transition:modalBackdrop>
    <div class="bg-bg-secondary border border-border rounded-xl max-w-md w-full p-5 shadow-2xl" transition:modalContent>
      <h3 class="text-lg font-bold text-text-primary mb-3">
        {phase === 'done' ? 'Update applied' : `Installing ${update?.version || 'update'}…`}
      </h3>
      <div class="flex justify-between items-center text-xs text-text-muted mb-1.5">
        <span>{progressMessage}</span>
        {#if progressTotal > 0}<span>{percent}%</span>{/if}
      </div>
      <div class="w-full bg-bg-tertiary rounded-full h-2 overflow-hidden">
        <div class="bg-accent h-full transition-all duration-200 ease-out" style="width: {percent}%"></div>
      </div>
      <p class="text-xs text-text-muted text-center mt-4">
        {phase === 'done' ? 'Restarting on the new version…' : 'Please wait — the app will close and reopen automatically.'}
      </p>
    </div>
  </div>
{/if}

{#if phase === 'error'}
  <div class="fixed inset-0 bg-black/70 flex items-center justify-center z-[100] p-4" transition:modalBackdrop>
    <div class="bg-bg-secondary border border-red-500/40 rounded-xl max-w-md w-full p-5 shadow-2xl" transition:modalContent>
      <h3 class="text-lg font-bold text-red-400 mb-2">Update failed</h3>
      <p class="text-sm text-text-secondary leading-relaxed whitespace-pre-wrap">{errorMessage}</p>
      <div class="flex justify-end gap-2 mt-4">
        <button
          on:click={dismissError}
          class="px-4 py-2 bg-bg-tertiary hover:bg-border rounded-lg text-sm text-text-secondary"
        >
          Close
        </button>
        <button
          on:click={() => { dismissError(); openReleasePage(); }}
          class="px-4 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium"
        >
          Download manually
        </button>
      </div>
    </div>
  </div>
{/if}
