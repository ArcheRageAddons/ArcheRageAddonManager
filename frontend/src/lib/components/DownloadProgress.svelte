<script>
  import { onMount, onDestroy } from 'svelte';
  import { downloadProgress, showNotification } from '../stores/app.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { OpenBackupFolder } from '../../../wailsjs/go/main/App.js';
  import { createEventDispatcher } from 'svelte';
  import { modalBackdrop, modalContent } from '../motion.js';

  const dispatch = createEventDispatcher();
  let timeoutId = null;

  // Stall watchdog: fires when no download:progress event arrives for 5s.
  // Each progress event reschedules, so a slow-but-steady link survives
  // and a genuine stall (404, dead connection mid-download) trips it.
  const STALL_MS = 5000;
  function armStallTimer() {
    clearStallTimer();
    timeoutId = setTimeout(() => {
      timeoutId = null;
      if (!$downloadProgress.isDownloading) return;
      showNotification('Addon download stalled — try again', 'error');
      downloadProgress.set({
        isDownloading: false,
        addonId: null,
        addonName: null,
        current: 0,
        total: 100,
        message: '',
      });
    }, STALL_MS);
  }
  function clearStallTimer() {
    if (timeoutId) {
      clearTimeout(timeoutId);
      timeoutId = null;
    }
  }

  $: if ($downloadProgress.isDownloading && !timeoutId) {
    armStallTimer();
  } else if (!$downloadProgress.isDownloading) {
    clearStallTimer();
  }

  onMount(() => {
    EventsOn('download:progress', (data) => {
      downloadProgress.update(state => ({
        ...state,
        current: data.current,
        total: data.total,
        message: data.message
      }));
      armStallTimer();
    });

    EventsOn('download:complete', (result) => {
      clearStallTimer();

      if (result.success) {
        showNotification(`${$downloadProgress.addonName || 'Addon'} installed successfully!`, 'success');

        window.dispatchEvent(new CustomEvent('addon-installed'));

        setTimeout(() => {
          downloadProgress.set({
            isDownloading: false,
            addonId: null,
            addonName: null,
            current: 0,
            total: 100,
            message: ''
          });
        }, 500);
      } else {
        // Failed install — InstallAddon backed up the previous version to
        // <AddonPath>/Backup before touching anything; surface a recovery action.
        showNotification(
          `Failed to install: ${result.error}`,
          'error',
          8000,
          { label: 'Open backup folder', handler: () => OpenBackupFolder() }
        );

        downloadProgress.set({
          isDownloading: false,
          addonId: null,
          addonName: null,
          current: 0,
          total: 100,
          message: ''
        });
      }
    });
  });

  onDestroy(() => {
    clearStallTimer();
    EventsOff('download:progress');
    EventsOff('download:complete');
  });
</script>

{#if $downloadProgress.isDownloading}
  <div class="fixed inset-0 bg-black/75 backdrop-blur-sm flex items-center justify-center z-[100]" transition:modalBackdrop>
    <div class="bg-bg-secondary border border-border rounded-2xl p-6 max-w-md w-full mx-4 shadow-modal" transition:modalContent>
      <div class="flex items-center gap-3 mb-5">
        <div class="relative w-11 h-11 rounded-xl bg-accent/15 border border-accent/40 flex items-center justify-center flex-shrink-0">
          <svg class="w-5 h-5 text-accent animate-pulse" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
          </svg>
        </div>
        <div>
          <h3 class="text-base font-bold text-text-primary tracking-tight">Installing {$downloadProgress.addonName || 'addon'}</h3>
          <p class="text-xs text-text-muted mt-0.5">Hold tight — this only takes a moment.</p>
        </div>
      </div>

      <div class="space-y-2.5">
        <!-- Progress Message -->
        <div class="flex justify-between items-center text-xs">
          <span class="text-text-secondary">{$downloadProgress.message}</span>
          <span class="text-accent font-mono font-semibold">{$downloadProgress.current}%</span>
        </div>

        <!-- Progress Bar -->
        <div class="w-full bg-bg-primary/60 border border-border rounded-full h-2 overflow-hidden">
          <div
            class="h-full transition-all duration-200 ease-out bg-accent-grad"
            style="width: {$downloadProgress.current}%"
          ></div>
        </div>
      </div>
    </div>
  </div>
{/if}
