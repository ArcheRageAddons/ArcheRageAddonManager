<script>
  import { onMount } from 'svelte';
  import { showWelcomeModal, appInitialized, showNotification } from '../stores/app.js';
  import {
    DetectAddonPaths,
    SetAddonPath,
    ConfirmAddonPath,
    SelectFolder,
    GetAddonPath,
  } from '../../../wailsjs/go/main/App.js';
  import { modalBackdrop, modalContent } from '../motion.js';

  let candidates = [];
  let selectedPath = '';
  let loading = true;
  let saving = false;

  onMount(async () => {
    try {
      candidates = (await DetectAddonPaths()) || [];
    } catch (e) {
      console.error('Failed to detect addon paths:', e);
      candidates = [];
    }

    const firstExisting = candidates.find((c) => c.exists);
    if (firstExisting) {
      selectedPath = firstExisting.path;
    } else {
      try {
        selectedPath = (await GetAddonPath()) || '';
      } catch {
        selectedPath = '';
      }
    }
    loading = false;
  });

  async function handleBrowse() {
    try {
      const picked = await SelectFolder();
      if (picked) {
        selectedPath = picked;
        if (!candidates.find((c) => c.path === picked)) {
          candidates = [{ path: picked, source: 'Custom', exists: true }, ...candidates];
        }
      }
    } catch (e) {
      console.error('Folder picker failed:', e);
    }
  }

  async function handleContinue() {
    if (!selectedPath) {
      showNotification('Please select an addon folder', 'error');
      return;
    }
    saving = true;
    try {
      // ConfirmAddonPath flips the setup-complete flag without mutating
      // the path; SetAddonPath does both, only needed when the user changed it.
      const current = await GetAddonPath();
      if (selectedPath === current) {
        await ConfirmAddonPath();
      } else {
        await SetAddonPath(selectedPath);
      }
      showWelcomeModal.set(false);
      appInitialized.set(true);
      showNotification('Addon folder set!', 'success');
    } catch (e) {
      showNotification(`Failed to save: ${e}`, 'error');
    }
    saving = false;
  }
</script>

{#if $showWelcomeModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-md p-4"
    transition:modalBackdrop
  >
    <div
      class="bg-bg-secondary border border-border rounded-2xl shadow-modal w-full max-w-2xl max-h-[90vh] overflow-y-auto"
      transition:modalContent
    >
      <!-- Header -->
      <div class="px-7 py-6 border-b border-border bg-header-grad flex items-center gap-4">
        <div class="relative">
          <div class="absolute inset-0 bg-accent/30 rounded-xl blur-lg"></div>
          <img src="/logo.png" alt="" class="w-12 h-12 relative rounded-xl" />
        </div>
        <div>
          <h2 class="text-2xl font-bold text-text-primary tracking-tight">Welcome to ArcheRage Addon Manager</h2>
          <p class="text-sm text-text-secondary mt-1">
            Choose your ArcheRage <code class="text-text-primary bg-bg-tertiary px-1.5 py-0.5 rounded text-xs">Addon</code> folder to get started.
          </p>
        </div>
      </div>

      <!-- Body -->
      <div class="p-6 space-y-5">
        <!-- OneDrive warning -->
        <div
          class="rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm text-text-primary"
        >
          <div class="flex items-start gap-2">
            <svg
              class="w-5 h-5 text-warning flex-shrink-0 mt-0.5"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M12 9v4M12 17h.01" />
              <path
                d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"
              />
            </svg>
            <div class="space-y-1">
              <div class="font-medium">Using OneDrive?</div>
              <p class="text-text-secondary leading-relaxed">
                If your Documents folder is synced to OneDrive, addons usually need to live inside the
                OneDrive folder for the game to load them. If addons aren't showing up in-game later, come
                back to Settings and double-check this path is the one ArcheRage actually reads from.
              </p>
            </div>
          </div>
        </div>

        <!-- Detected candidates -->
        <div>
          <div class="text-sm font-medium text-text-primary mb-2">Detected locations</div>
          {#if loading}
            <div class="text-sm text-text-muted">Detecting...</div>
          {:else if candidates.length === 0}
            <div class="text-sm text-text-muted">
              Couldn't detect any common locations. Use Browse below.
            </div>
          {:else}
            <div class="space-y-1.5">
              {#each candidates as c}
                <label
                  class="flex items-center gap-3 px-3 py-2.5 bg-bg-primary border rounded-lg cursor-pointer transition-colors {selectedPath ===
                  c.path
                    ? 'border-accent'
                    : 'border-border hover:border-text-muted'}"
                >
                  <input
                    type="radio"
                    name="addon-path"
                    bind:group={selectedPath}
                    value={c.path}
                    class="accent-accent"
                  />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-text-primary truncate">{c.path}</div>
                    <div class="text-xs text-text-muted mt-0.5 flex items-center gap-2">
                      <span>{c.source}</span>
                      {#if c.exists}
                        <span class="text-success">● exists</span>
                      {:else}
                        <span class="text-text-muted">○ not found</span>
                      {/if}
                    </div>
                  </div>
                </label>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Manual entry / browse -->
        <div>
          <div class="text-sm font-medium text-text-primary mb-2">Or pick manually</div>
          <div class="flex gap-2">
            <input
              type="text"
              bind:value={selectedPath}
              placeholder="C:\Users\Username\Documents\ArcheRage\Addon"
              class="flex-1 px-3 py-2 bg-bg-primary border border-border rounded-lg focus:outline-none focus:border-accent text-sm"
            />
            <button
              on:click={handleBrowse}
              class="px-4 py-2 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-sm"
            >
              Browse...
            </button>
          </div>
          <p class="text-xs text-text-muted mt-2">
            The default location is <code class="text-text-secondary"
              >C:\Users\Username\Documents\ArcheRage\Addon</code
            >. The folder will be created on first install if it doesn't exist.
          </p>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-7 py-5 border-t border-border bg-bg-primary/40 flex justify-end gap-2">
        <button
          on:click={handleContinue}
          disabled={saving || !selectedPath}
          class="px-6 py-2.5 bg-accent-grad hover:brightness-110 text-white rounded-xl transition-all disabled:opacity-50 text-sm font-semibold shadow-lift"
        >
          {saving ? 'Saving…' : 'Continue'}
        </button>
      </div>
    </div>
  </div>
{/if}
