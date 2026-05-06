<script>
  import { onMount, onDestroy } from 'svelte';
  import { downloadProgress, showNotification } from '../stores/app.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { OpenBackupFolder } from '../../../wailsjs/go/main/App.js';
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();
  let timeoutId = null;

  // Watch for download start and set timeout
  $: if ($downloadProgress.isDownloading && !timeoutId) {
    // Set 5-second timeout to detect stalled downloads
    timeoutId = setTimeout(() => {
      // If still downloading and progress is very low, it's stalled
      if ($downloadProgress.isDownloading && $downloadProgress.current < 10) {
        showNotification('Addon cannot be downloaded', 'error');

        // Reset progress
        downloadProgress.set({
          isDownloading: false,
          addonId: null,
          addonName: null,
          current: 0,
          total: 100,
          message: ''
        });

        timeoutId = null;
      }
    }, 5000);
  } else if (!$downloadProgress.isDownloading && timeoutId) {
    // Clear timeout if download finishes
    clearTimeout(timeoutId);
    timeoutId = null;
  }

  // Listen for download progress events
  onMount(() => {
    EventsOn('download:progress', (data) => {
      downloadProgress.update(state => ({
        ...state,
        current: data.current,
        total: data.total,
        message: data.message
      }));

      // Clear timeout once we see real progress
      if (data.current > 10 && timeoutId) {
        clearTimeout(timeoutId);
        timeoutId = null;
      }
    });

    // Listen for download completion
    EventsOn('download:complete', (result) => {
      // Clear timeout
      if (timeoutId) {
        clearTimeout(timeoutId);
        timeoutId = null;
      }

      if (result.success) {
        showNotification(`${$downloadProgress.addonName || 'Addon'} installed successfully!`, 'success');

        // Trigger refresh immediately so UI updates
        window.dispatchEvent(new CustomEvent('addon-installed'));

        // Reset progress after short delay
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
        // Failed install — InstallAddon backed up the existing folder before
        // touching anything (see addon.Manager.BackupAddon), so the user's
        // previous version still exists in <AddonPath>/Backup/<name>_<ts>.
        // Surface a recovery action on the toast so they can grab it.
        showNotification(
          `Failed to install: ${result.error}`,
          'error',
          8000,
          { label: 'Open backup folder', handler: () => OpenBackupFolder() }
        );

        // Reset progress immediately on error
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
    if (timeoutId) {
      clearTimeout(timeoutId);
    }
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
