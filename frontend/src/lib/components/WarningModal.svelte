<script>
  import { showWarningModal, warningAddon, showAddonDetails, downloadProgress, kickOffInstall } from '../stores/app.js';
  import { modalBackdrop, modalContent } from '../motion.js';

  function close() {
    showWarningModal.set(false);
    warningAddon.set(null);
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  async function handleConfirm() {
    kickOffInstall($warningAddon);
    showAddonDetails.set(false);
    close();
  }
</script>

{#if $showWarningModal && $warningAddon}
  <div
    class="fixed inset-0 bg-black/75 backdrop-blur-sm flex items-center justify-center z-[60] p-4"
    on:click={handleBackdropClick}
    on:keydown={(e) => e.key === 'Escape' && close()}
    tabindex="-1"
    transition:modalBackdrop
  >
    <div class="bg-bg-secondary border border-border rounded-2xl max-w-md w-full shadow-modal" transition:modalContent>
      <!-- Header -->
      <div class="px-6 py-5 border-b border-border bg-header-grad flex items-center gap-4">
        <div class="p-2.5 bg-warning/15 border border-warning/40 rounded-xl">
          <svg class="w-6 h-6 text-warning" viewBox="0 0 24 24" fill="currentColor">
            <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
          </svg>
        </div>
        <h2 class="text-lg font-bold text-text-primary tracking-tight">Security warning</h2>
      </div>

      <!-- Content -->
      <div class="p-5">
        <p class="text-sm text-text-secondary mb-4">
          The addon <strong class="text-text-primary">{$warningAddon.name}</strong> contains potentially dangerous files:
        </p>

        <div class="p-3 bg-bg-tertiary rounded-lg mb-4">
          <p class="text-sm text-text-muted">
            May include <code class="text-warning px-1 py-0.5 bg-warning/10 rounded">.bat</code>, <code class="text-warning px-1 py-0.5 bg-warning/10 rounded">.ps1</code>, or <code class="text-warning px-1 py-0.5 bg-warning/10 rounded">.exe</code> files.
          </p>
        </div>

        <p class="text-text-muted text-xs">
          These files could harm your computer. Only download if you trust the addon author.
        </p>
      </div>

      <!-- Footer -->
      <div class="px-6 py-5 border-t border-border bg-bg-primary/40 flex justify-end gap-2">
        <button
          on:click={close}
          class="px-4 py-2.5 bg-bg-tertiary hover:bg-bg-elevated border border-border rounded-lg transition-colors text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={handleConfirm}
          disabled={$downloadProgress.isDownloading}
          class="px-5 py-2.5 bg-warning hover:brightness-110 text-bg-primary font-semibold rounded-lg transition-all disabled:opacity-50 flex items-center gap-2 text-sm shadow-soft"
        >
          {#if $downloadProgress.isDownloading && $downloadProgress.addonId === $warningAddon?.id}
            <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
          {/if}
          Download Anyway
        </button>
      </div>
    </div>
  </div>
{/if}
