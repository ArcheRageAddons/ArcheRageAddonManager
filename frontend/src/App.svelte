<script>
  import { onMount } from 'svelte';
  import {
    currentPage,
    appInitialized,
    showWelcomeModal,
    showLayoutChooser,
    showNotification,
    layoutMode,
  } from './lib/stores/app.js';
  import { GetInstalledAddons, IsFirstRun, LogFromFrontend, GetLayoutChooserShown } from '../wailsjs/go/main/App.js';

  // Mirror console.* to manager.log via the Go side (which scrubs tokens).
  function pipeConsoleToLog() {
    for (const level of ['error', 'warn', 'log']) {
      const original = console[level].bind(console);
      console[level] = (...args) => {
        original(...args);
        try {
          const formatted = args
            .map((a) => {
              if (a instanceof Error) return `${a.name}: ${a.message}\n${a.stack || ''}`;
              if (typeof a === 'object') {
                try { return JSON.stringify(a); } catch { return String(a); }
              }
              return String(a);
            })
            .join(' ');
          LogFromFrontend(level === 'log' ? 'info' : level, formatted);
        } catch {}
      };
    }
  }
  pipeConsoleToLog();

  // Fires after appInitialized flips true — covers both fresh-install
  // (welcome modal → addon path → init) and existing installs.
  let layoutChecked = false;
  async function checkLayoutChooser() {
    try {
      const shown = await GetLayoutChooserShown();
      if (!shown) showLayoutChooser.set(true);
    } catch (e) {
      console.warn('Failed to read layout-chooser flag:', e);
    }
  }
  $: if ($appInitialized && !layoutChecked) {
    layoutChecked = true;
    checkLayoutChooser();
  }

  // ===== Studio shell components (split-view, icon rail) =====
  import IconRail from './lib/components/IconRail.svelte';
  import BrowseStudio from './lib/components/Browse.svelte';
  import InstalledStudio from './lib/components/Installed.svelte';

  // ===== Classic shell components (sidebar, full-width pages, modal details) =====
  import Sidebar from './lib/components/classic/Sidebar.svelte';
  import BrowseClassic from './lib/components/classic/Browse.svelte';
  import InstalledClassic from './lib/components/classic/Installed.svelte';
  import AddonDetailsClassic from './lib/components/classic/AddonDetailsModal.svelte';
  import UpdatesBellClassic from './lib/components/classic/UpdatesBell.svelte';

  // ===== Shared across layouts =====
  import Settings from './lib/components/Settings.svelte';
  import MyAddons from './lib/components/MyAddons.svelte';
  import Admin from './lib/components/Admin.svelte';
  import Changelog from './lib/components/Changelog.svelte';
  import AuthorModal from './lib/components/AuthorModal.svelte';
  import WarningModal from './lib/components/WarningModal.svelte';
  import UninstallConfirmModal from './lib/components/UninstallConfirmModal.svelte';
  import WelcomeModal from './lib/components/WelcomeModal.svelte';
  import LayoutChooserModal from './lib/components/LayoutChooserModal.svelte';
  import SubmitAddonModal from './lib/components/SubmitAddonModal.svelte';
  import UpdateBanner from './lib/components/UpdateBanner.svelte';
  import Notification from './lib/components/Notification.svelte';
  import DownloadProgress from './lib/components/DownloadProgress.svelte';
  import { pageFade } from './lib/motion.js';

  onMount(async () => {
    let firstRun = false;
    try { firstRun = await IsFirstRun(); } catch (e) { console.error('Failed to read first-run flag:', e); }

    if (firstRun) { showWelcomeModal.set(true); return; }

    appInitialized.set(true);

    setTimeout(async () => {
      try {
        const installed = await GetInstalledAddons();
        const updatesAvailable = installed.filter((addon) => addon.has_update);

        if (updatesAvailable.length > 0) {
          const message =
            updatesAvailable.length === 1
              ? `1 addon has an update available`
              : `${updatesAvailable.length} addons have updates available`;
          showNotification(message, 'warning');
        }
      } catch (e) {
        console.error('Failed to check for updates:', e);
      }
    }, 2000);
  });
</script>

<main class="h-screen flex flex-col bg-bg-primary text-text-primary overflow-hidden">
  <UpdateBanner />

  {#if $layoutMode === 'classic'}
    <!-- ============ Classic shell: sidebar + full-width pages + floating bell ============ -->
    <div class="flex-1 flex overflow-hidden">
      <Sidebar />
      <div class="flex-1 flex flex-col overflow-hidden">
        {#key $currentPage}
          <div class="flex-1 flex flex-col overflow-hidden" in:pageFade>
            {#if $currentPage === 'browse'}
              <BrowseClassic />
            {:else if $currentPage === 'installed'}
              <InstalledClassic />
            {:else if $currentPage === 'my-addons'}
              <MyAddons />
            {:else if $currentPage === 'admin'}
              <Admin />
            {:else if $currentPage === 'changelog'}
              <Changelog />
            {:else if $currentPage === 'settings'}
              <Settings />
            {/if}
          </div>
        {/key}
      </div>
    </div>
    <UpdatesBellClassic />
    <AddonDetailsClassic />
  {:else}
    <!-- ============ Studio shell: icon rail + split-view list/detail ============ -->
    <div class="flex-1 flex overflow-hidden">
      <IconRail />
      <div class="flex-1 flex flex-col overflow-hidden">
        {#key $currentPage}
          <div class="flex-1 flex flex-col overflow-hidden" in:pageFade>
            {#if $currentPage === 'browse'}
              <BrowseStudio />
            {:else if $currentPage === 'installed'}
              <InstalledStudio />
            {:else if $currentPage === 'my-addons'}
              <MyAddons />
            {:else if $currentPage === 'admin'}
              <Admin />
            {:else if $currentPage === 'changelog'}
              <Changelog />
            {:else if $currentPage === 'settings'}
              <Settings />
            {/if}
          </div>
        {/key}
      </div>
    </div>
  {/if}

  <AuthorModal />
  <WarningModal />
  <UninstallConfirmModal />
  <WelcomeModal />
  <LayoutChooserModal />
  <SubmitAddonModal />
  <Notification />
  <DownloadProgress />
</main>
