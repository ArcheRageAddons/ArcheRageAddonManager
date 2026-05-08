<script>
  import { onMount } from 'svelte';
  import {
    currentPage,
    appInitialized,
    showWelcomeModal,
    showNotification,
  } from './lib/stores/app.js';
  import { GetInstalledAddons, IsFirstRun, LogFromFrontend } from '../wailsjs/go/main/App.js';

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

  import Sidebar from './lib/components/Sidebar.svelte';
  import Browse from './lib/components/Browse.svelte';
  import Installed from './lib/components/Installed.svelte';
  import Settings from './lib/components/Settings.svelte';
  import MyAddons from './lib/components/MyAddons.svelte';
  import Admin from './lib/components/Admin.svelte';
  import AddonDetailsModal from './lib/components/AddonDetailsModal.svelte';
  import AuthorModal from './lib/components/AuthorModal.svelte';
  import UpdatesBell from './lib/components/UpdatesBell.svelte';
  import WarningModal from './lib/components/WarningModal.svelte';
  import UninstallConfirmModal from './lib/components/UninstallConfirmModal.svelte';
  import WelcomeModal from './lib/components/WelcomeModal.svelte';
  import SubmitAddonModal from './lib/components/SubmitAddonModal.svelte';
  import UpdateBanner from './lib/components/UpdateBanner.svelte';
  import Notification from './lib/components/Notification.svelte';
  import DownloadProgress from './lib/components/DownloadProgress.svelte';
  import { pageFade } from './lib/motion.js';

  onMount(async () => {
    let firstRun = false;
    try {
      firstRun = await IsFirstRun();
    } catch (e) {
      console.error('Failed to read first-run flag:', e);
    }

    if (firstRun) {
      showWelcomeModal.set(true);
      return;
    }

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
  <div class="flex-1 flex overflow-hidden">
    <Sidebar />

  <div class="flex-1 flex flex-col overflow-hidden">
    {#key $currentPage}
      <div class="flex-1 flex flex-col overflow-hidden" in:pageFade>
        {#if $currentPage === 'browse'}
          <Browse />
        {:else if $currentPage === 'installed'}
          <Installed />
        {:else if $currentPage === 'my-addons'}
          <MyAddons />
        {:else if $currentPage === 'admin'}
          <Admin />
        {:else if $currentPage === 'settings'}
          <Settings />
        {/if}
      </div>
    {/key}
  </div>

  </div>

  <!-- Floating top-right bell. Page headers reserve `pr-16` so their
       right-side controls don't collide with it. -->
  <UpdatesBell />
  <AddonDetailsModal />
  <AuthorModal />
  <WarningModal />
  <UninstallConfirmModal />
  <WelcomeModal />
  <SubmitAddonModal />
  <Notification />
  <DownloadProgress />
</main>
