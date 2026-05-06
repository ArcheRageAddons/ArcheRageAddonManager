<script>
  import { onMount, onDestroy } from 'svelte';
  import { downloadProgress, showNotification } from '../stores/app.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { OpenBackupFolder } from '../../../wailsjs/go/main/App.js';
  import { createEventDispatcher } from 'svelte';

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
  <div class="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
    <div class="bg-bg-secondary border border-border rounded-xl p-6 max-w-md w-full mx-4 shadow-2xl">
      <h3 class="text-lg font-bold text-text-primary mb-4">
        Downloading {$downloadProgress.addonName || 'Addon'}
      </h3>

      <div class="space-y-3">
        <!-- Progress Message -->
        <div class="flex justify-between items-center text-sm">
          <span class="text-text-muted">{$downloadProgress.message}</span>
          <span class="text-text-primary font-medium">{$downloadProgress.current}%</span>
        </div>

        <!-- Progress Bar -->
        <div class="w-full bg-bg-tertiary rounded-full h-3 overflow-hidden">
          <div
            class="bg-accent h-full transition-all duration-200 ease-out"
            style="width: {$downloadProgress.current}%"
          ></div>
        </div>

        <!-- Note -->
        <p class="text-xs text-text-muted text-center mt-4">
          Please wait while the addon is being downloaded and installed...
        </p>
      </div>
    </div>
  </div>
{/if}
