<script>
  import { uninstallAddon, showUninstallConfirm, showNotification } from '../stores/app.js';
  import { UninstallAddon } from '../../../wailsjs/go/main/App.js';
  import { createEventDispatcher } from 'svelte';
  import { modalBackdrop, modalContent } from '../motion.js';

  const dispatch = createEventDispatcher();
  let uninstalling = false;

  function close() {
    showUninstallConfirm.set(false);
    uninstallAddon.set(null);
  }

  async function handleConfirm() {
    if (!$uninstallAddon) return;

    uninstalling = true;
    try {
      await UninstallAddon($uninstallAddon.id);
      showNotification(`${$uninstallAddon.name} uninstalled successfully!`, 'success');

      window.dispatchEvent(new CustomEvent('addon-installed'));

      close();
    } catch (e) {
      showNotification(`Failed to uninstall: ${e}`, 'error');
    }
    uninstalling = false;
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') close();
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if $showUninstallConfirm && $uninstallAddon}
  <div class="fixed inset-0 bg-black/75 backdrop-blur-sm flex items-center justify-center z-[100]" on:click={close} transition:modalBackdrop>
    <div
      class="bg-bg-secondary border border-border rounded-2xl p-6 max-w-md w-full mx-4 shadow-modal"
      on:click|stopPropagation
      transition:modalContent
    >
      <!-- Header -->
      <div class="flex items-start gap-3.5 mb-5">
        <div class="w-11 h-11 rounded-xl bg-red-500/15 border border-red-500/40 flex items-center justify-center flex-shrink-0">
          <svg class="w-5 h-5 text-red-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </div>
        <div class="flex-1">
          <h3 class="text-lg font-bold text-text-primary tracking-tight">Uninstall addon?</h3>
          <p class="text-sm text-text-muted mt-1 leading-relaxed">
            <span class="text-text-primary font-semibold">{$uninstallAddon.name}</span> will be removed from your game directory.
          </p>
        </div>
      </div>

      <!-- Message -->
      <div class="bg-bg-primary/60 border border-border rounded-xl p-3.5 mb-6">
        <p class="text-xs text-text-secondary leading-relaxed">
          All addon files in the install folder will be deleted. This can't be undone — you'll have to reinstall from Browse if you change your mind.
        </p>
      </div>

      <!-- Actions -->
      <div class="flex items-center gap-2 justify-end">
        <button
          on:click={close}
          disabled={uninstalling}
          class="px-4 py-2.5 bg-bg-tertiary hover:bg-bg-elevated border border-border rounded-lg transition-colors disabled:opacity-50 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={handleConfirm}
          disabled={uninstalling}
          class="px-4 py-2.5 bg-red-500 hover:bg-red-600 text-white rounded-lg transition-all disabled:opacity-50 flex items-center gap-2 text-sm font-semibold shadow-soft"
        >
          {#if uninstalling}
            <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
            Uninstalling…
          {:else}
            Uninstall
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
