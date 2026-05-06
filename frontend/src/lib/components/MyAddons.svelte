<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    currentUser,
    showSubmitModal,
    submitPrefill,
    showNotification,
  } from '../stores/app.js';
  import {
    GetMySubmissions,
    BuildUpdateForm,
    DeleteSubmissions,
    DeleteAddon,
    WithdrawSubmission,
    OpenURL,
  } from '../../../wailsjs/go/main/App.js';
  import Spinner from './Spinner.svelte';

  let submissions = [];
  let loading = true;
  let updating = {};
  let deleting = {};
  let withdrawing = {};
  let historyOpen = true;
  let deleteConfirmTarget = null;   // { addon, displayName } — scary delete-addon flow
  let typedConfirm = '';
  let withdrawTarget = null;         // submission row currently asking to withdraw
  let clearHistoryConfirm = false;   // toggles the "Clear all" confirm modal

  async function load() {
    loading = true;
    try {
      submissions = (await GetMySubmissions()) || [];
    } catch (e) {
      showNotification(`Failed to load submissions: ${e}`, 'error');
      submissions = [];
    }
    loading = false;
  }

  function onSubmissionCreated() { load(); }

  onMount(() => {
    load();
    window.addEventListener('submission-created', onSubmissionCreated);
  });

  onDestroy(() => {
    window.removeEventListener('submission-created', onSubmissionCreated);
  });

  // One entry per slug — the most recent approved submission.
  $: yourAddons = (() => {
    const map = {};
    for (const s of submissions) {
      if (s.status !== 'approved') continue;
      const cur = map[s.addon_slug];
      if (!cur || new Date(s.created_at) > new Date(cur.created_at)) {
        map[s.addon_slug] = s;
      }
    }
    return Object.values(map).sort((a, b) => a.addon_slug.localeCompare(b.addon_slug));
  })();

  $: pendingSlugs = new Set(
    submissions.filter((s) => s.status === 'pending').map((s) => s.addon_slug),
  );

  $: keepIds = new Set(yourAddons.map((a) => a.id));

  function canDelete(s) {
    return s.status !== 'pending' && !keepIds.has(s.id);
  }
  $: deletableIds = submissions.filter(canDelete).map((s) => s.id);

  function topField(yaml, field) {
    const m = yaml?.match(new RegExp(`^\\s*${field}:\\s*["']?([^"\\n]*?)["']?\\s*$`, 'm'));
    return m ? m[1].trim() : '';
  }
  function commitOf(yaml) {
    const m = yaml?.match(/^\s*commit:\s*["']?([a-f0-9]{7,40})["']?\s*$/m);
    return m ? m[1] : '';
  }

  function statusColor(s) {
    switch (s) {
      case 'approved':  return 'text-success';
      case 'denied':    return 'text-warning';
      case 'withdrawn': return 'text-text-muted';
      default:          return 'text-accent';
    }
  }

  function fmtDate(iso) {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
    });
  }

  async function update(s) {
    updating = { ...updating, [s.id]: true };
    try {
      const data = await BuildUpdateForm(s.yaml_content, s.github_repo, s.github_path || '');
      submitPrefill.set(data);
      showSubmitModal.set(true);
    } catch (e) {
      showNotification(`Couldn't load submission for update: ${e}`, 'error', 8000);
    }
    updating = { ...updating, [s.id]: false };
  }

  function startWithdraw(s) {
    withdrawTarget = s;
  }

  function cancelWithdraw() {
    withdrawTarget = null;
  }

  async function confirmWithdraw() {
    if (!withdrawTarget) return;
    const s = withdrawTarget;
    withdrawing = { ...withdrawing, [s.id]: true };
    withdrawTarget = null;
    try {
      await WithdrawSubmission(s.id);
      showNotification(`Withdrew ${s.addon_slug}`, 'info', 4000);
      await load();
    } catch (e) {
      showNotification(`Withdraw failed: ${e}`, 'error', 8000);
    }
    withdrawing = { ...withdrawing, [s.id]: false };
  }

  async function deleteOne(id) {
    try {
      await DeleteSubmissions([id]);
      submissions = submissions.filter((x) => x.id !== id);
    } catch (e) {
      showNotification(`Failed to delete: ${e}`, 'error', 6000);
    }
  }

  function startDeleteAddon(a) {
    const displayName = topField(a.yaml_content, 'name') || a.addon_slug;
    deleteConfirmTarget = { addon: a, displayName };
    typedConfirm = '';
  }

  function cancelDeleteAddon() {
    deleteConfirmTarget = null;
    typedConfirm = '';
  }

  async function confirmDeleteAddon() {
    if (!deleteConfirmTarget) return;
    const { addon, displayName } = deleteConfirmTarget;
    if (typedConfirm.trim() !== addon.addon_slug) return;
    deleting = { ...deleting, [addon.id]: true };
    try {
      await DeleteAddon(addon.addon_slug);
      showNotification(`${displayName} removed from the registry`, 'success', 5000);
      deleteConfirmTarget = null;
      typedConfirm = '';
      await load();
    } catch (e) {
      showNotification(`Delete failed: ${e}`, 'error', 10000);
    }
    deleting = { ...deleting, [addon.id]: false };
  }

  function startClearHistory() {
    if (deletableIds.length === 0) return;
    clearHistoryConfirm = true;
  }

  function cancelClearHistory() {
    clearHistoryConfirm = false;
  }

  async function confirmClearHistory() {
    if (deletableIds.length === 0) return;
    clearHistoryConfirm = false;
    const toDelete = new Set(deletableIds);
    try {
      await DeleteSubmissions(deletableIds);
      submissions = submissions.filter((s) => !toDelete.has(s.id));
      showNotification(`Removed ${toDelete.size} entries`, 'success', 4000);
    } catch (e) {
      showNotification(`Failed to clear history: ${e}`, 'error', 6000);
    }
  }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <div class="flex justify-between items-center p-4 pr-16 border-b border-border bg-bg-secondary">
    <div>
      <h2 class="text-lg font-bold text-text-primary">My Addons</h2>
      <p class="text-xs text-text-muted mt-0.5">
        Your published addons and submission history.
      </p>
    </div>
    <div class="flex items-center gap-2">
      <button
        on:click={load}
        title="Refresh"
        class="p-2.5 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-text-secondary"
      >
        <svg class="w-4 h-4 {loading ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M23 4v6h-6M1 20v-6h6"/>
          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
        </svg>
      </button>
      <button
        on:click={() => { submitPrefill.set(null); showSubmitModal.set(true); }}
        class="px-4 py-2.5 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium flex items-center gap-2"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M12 5v14M5 12h14"/>
        </svg>
        Submit New Addon
      </button>
    </div>
  </div>

  <!-- Body -->
  <div class="flex-1 overflow-y-auto p-4 space-y-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <div class="text-text-secondary text-sm">Loading...</div>
      </div>
    {:else if submissions.length === 0}
      <!-- First-time empty state -->
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary max-w-md">
          <svg class="w-16 h-16 mx-auto mb-4 opacity-40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <path d="M14 2v6h6M16 13H8M16 17H8M10 9H8"/>
          </svg>
          <p class="text-base text-text-primary">You haven't submitted any addons yet</p>
          <p class="text-sm mt-2 text-text-muted">
            Click <strong class="text-text-primary">Submit New Addon</strong> above to publish an addon from one of your GitHub repositories.
            A maintainer will review it before it appears in the public catalogue.
          </p>
        </div>
      </div>
    {:else}
      <!-- Your addons -->
      <section>
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-sm font-semibold text-text-primary uppercase tracking-wide">
            Your published addons
            <span class="text-text-muted font-normal normal-case">({yourAddons.length})</span>
          </h3>
        </div>

        {#if yourAddons.length === 0}
          <div class="bg-bg-secondary border border-border rounded-lg p-6 text-center">
            <p class="text-sm text-text-secondary">
              Nothing published yet. Once a maintainer approves a submission it'll show up here.
            </p>
          </div>
        {:else}
          <div class="space-y-2">
            {#each yourAddons as a}
              {@const name = topField(a.yaml_content, 'name') || a.addon_slug}
              {@const version = topField(a.yaml_content, 'version')}
              {@const category = topField(a.yaml_content, 'category')}
              {@const icon = topField(a.yaml_content, 'icon')}
              {@const commit = commitOf(a.yaml_content)}
              {@const hasPending = pendingSlugs.has(a.addon_slug)}
              <div class="bg-bg-secondary border border-border rounded-lg p-4 flex items-center gap-4 hover:bg-bg-tertiary transition-colors">
                <!-- Icon -->
                <div class="w-12 h-12 rounded-lg bg-bg-tertiary flex items-center justify-center flex-shrink-0 overflow-hidden">
                  {#if icon}
                    <img src={icon} alt="" referrerpolicy="no-referrer" class="w-full h-full object-cover" />
                  {:else}
                    <svg class="w-6 h-6 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                    </svg>
                  {/if}
                </div>
                <!-- Main info -->
                <div class="flex-1 min-w-0">
                  <div class="flex items-baseline gap-2 flex-wrap">
                    <span class="font-medium text-text-primary truncate">{name}</span>
                    {#if version}
                      <span class="text-xs text-text-secondary">v{version}</span>
                    {/if}
                    {#if category}
                      <span class="text-[10px] uppercase tracking-wider text-text-muted">· {category}</span>
                    {/if}
                  </div>
                  <div class="text-xs text-text-muted mt-1 truncate">
                    <span class="font-mono">{a.github_repo}{a.github_path ? '/' + a.github_path : ''}</span>
                    {#if commit}
                      ·
                      <button
                        on:click={() => OpenURL(`https://github.com/${a.github_repo}/commit/${commit}`)}
                        class="font-mono text-accent hover:text-accent-hover"
                        title="Open commit on GitHub"
                      >
                        {commit.slice(0, 7)} ↗
                      </button>
                    {/if}
                  </div>
                  {#if hasPending}
                    <div class="text-xs text-accent mt-1 italic">
                      Update pending review
                    </div>
                  {/if}
                </div>
                <!-- Actions -->
                <div class="flex-shrink-0 flex items-center gap-2">
                  <button
                    on:click={() => update(a)}
                    disabled={updating[a.id] || hasPending}
                    title={hasPending ? 'You already have an update awaiting review' : 'Submit a new version'}
                    class="px-3 py-2 bg-bg-tertiary hover:bg-accent hover:text-white border border-border rounded-lg text-xs font-medium flex items-center gap-1.5 disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-bg-tertiary disabled:hover:text-text-secondary transition-colors text-text-secondary"
                  >
                    {#if updating[a.id]}
                      <Spinner size="sm" />
                    {:else}
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M23 4v6h-6M1 20v-6h6"/>
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                      </svg>
                    {/if}
                    {updating[a.id] ? 'Loading…' : 'Update'}
                  </button>
                  <button
                    on:click={() => startDeleteAddon(a)}
                    disabled={deleting[a.id]}
                    title="Permanently delete this addon from the registry"
                    class="p-2 text-text-muted hover:text-warning hover:bg-warning/10 border border-border rounded-lg transition-colors disabled:opacity-40"
                  >
                    {#if deleting[a.id]}
                      <Spinner size="sm" />
                    {:else}
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M10 11v6M14 11v6"/>
                      </svg>
                    {/if}
                  </button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Submission history -->
      <section>
        <div class="flex items-center justify-between mb-2">
          <button
            on:click={() => historyOpen = !historyOpen}
            class="flex items-center gap-2 group"
          >
            <h3 class="text-sm font-semibold text-text-primary uppercase tracking-wide flex items-center gap-2">
              <svg
                class="w-3 h-3 transition-transform {historyOpen ? 'rotate-90' : ''}"
                viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
              >
                <path d="M9 18l6-6-6-6"/>
              </svg>
              Submission history
              <span class="text-text-muted font-normal normal-case">({submissions.length})</span>
            </h3>
          </button>
          {#if deletableIds.length > 0}
            <button
              on:click={startClearHistory}
              class="text-xs text-text-muted hover:text-warning transition-colors"
              title="Delete all decided entries (keeps pending + latest approved per addon)"
            >
              Clear ({deletableIds.length})
            </button>
          {/if}
        </div>
        {#if historyOpen}
          <div class="space-y-1">
            {#each submissions as s}
              {@const sVersion = topField(s.yaml_content, 'version')}
              <div class="bg-bg-secondary/60 border border-border/60 rounded-md px-3 py-2 flex items-center gap-3 text-sm group">
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="font-mono text-xs text-text-secondary">{s.addon_slug}</span>
                    {#if sVersion}
                      <span class="text-xs text-text-muted">v{sVersion}</span>
                    {/if}
                    <span class="text-[10px] uppercase tracking-wider {statusColor(s.status)}">{s.status}</span>
                    {#if s.github_pr_number}
                      <button
                        on:click={() => OpenURL(s.github_pr_url)}
                        class="text-xs text-accent hover:text-accent-hover"
                        title="Open PR on GitHub"
                      >
                        PR #{s.github_pr_number} ↗
                      </button>
                    {/if}
                    {#if s.status === 'pending'}
                      <button
                        on:click={() => startWithdraw(s)}
                        disabled={withdrawing[s.id]}
                        class="text-xs text-text-muted hover:text-warning disabled:opacity-50 inline-flex items-center gap-1"
                        title="Cancel this submission and close the PR"
                      >
                        {#if withdrawing[s.id]}<Spinner size="sm" />{/if}
                        {withdrawing[s.id] ? 'Withdrawing…' : 'Withdraw'}
                      </button>
                    {/if}
                  </div>
                  {#if s.decision_reason}
                    <div class="text-xs text-text-muted mt-0.5 italic line-clamp-2">
                      {s.decision_reason}
                    </div>
                  {/if}
                </div>
                <div class="text-[10px] text-text-muted flex-shrink-0">
                  {fmtDate(s.created_at)}
                </div>
                {#if canDelete(s)}
                  <button
                    on:click={() => deleteOne(s.id)}
                    title="Delete this history entry"
                    class="opacity-0 group-hover:opacity-100 text-text-muted hover:text-warning transition-opacity p-1"
                  >
                    <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M18 6 6 18M6 6l12 12"/>
                    </svg>
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
</div>

<!-- Withdraw confirmation modal -->
{#if withdrawTarget}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4"
    on:click={cancelWithdraw}
    on:keydown={(e) => e.key === 'Escape' && cancelWithdraw()}
    role="presentation"
  >
    <div
      class="bg-bg-secondary border border-border rounded-xl shadow-2xl w-full max-w-md"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
    >
      <div class="p-5 border-b border-border">
        <h3 class="text-lg font-bold text-text-primary">Withdraw submission?</h3>
      </div>
      <div class="p-5 space-y-3 text-sm">
        <p class="text-text-secondary">
          You're about to withdraw your submission for <strong class="font-mono text-text-primary">{withdrawTarget.addon_slug}</strong>.
        </p>
        <ul class="text-text-secondary space-y-1.5 text-xs list-disc list-inside ml-1">
          <li>The open PR on GitHub will be closed without merging.</li>
          <li>The submission row will be marked withdrawn in your history.</li>
          <li>You can submit again later — withdrawing now doesn't lock anything.</li>
        </ul>
      </div>
      <div class="p-5 border-t border-border flex justify-end gap-2">
        <button
          on:click={cancelWithdraw}
          class="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={confirmWithdraw}
          class="px-5 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium"
        >
          Withdraw
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Clear-history confirmation modal -->
{#if clearHistoryConfirm}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4"
    on:click={cancelClearHistory}
    on:keydown={(e) => e.key === 'Escape' && cancelClearHistory()}
    role="presentation"
  >
    <div
      class="bg-bg-secondary border border-border rounded-xl shadow-2xl w-full max-w-md"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
    >
      <div class="p-5 border-b border-border">
        <h3 class="text-lg font-bold text-text-primary">Clear submission history?</h3>
      </div>
      <div class="p-5 space-y-3 text-sm">
        <p class="text-text-secondary">
          Delete <strong class="text-text-primary">{deletableIds.length}</strong> entries from your submission history.
        </p>
        <ul class="text-text-secondary space-y-1.5 text-xs list-disc list-inside ml-1">
          <li>Pending submissions stay (you can still withdraw them).</li>
          <li>The latest approved version of each addon stays (so you can still update / delete it).</li>
          <li>Everything else — old approved, denied, withdrawn rows — is removed.</li>
          <li><strong class="text-text-primary">This cannot be undone.</strong></li>
        </ul>
      </div>
      <div class="p-5 border-t border-border flex justify-end gap-2">
        <button
          on:click={cancelClearHistory}
          class="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={confirmClearHistory}
          class="px-5 py-2 bg-warning hover:bg-warning/80 text-bg-primary rounded-lg text-sm font-bold"
        >
          Clear {deletableIds.length}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Delete-addon confirmation modal -->
{#if deleteConfirmTarget}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4"
    on:click={cancelDeleteAddon}
    on:keydown={(e) => e.key === 'Escape' && cancelDeleteAddon()}
    role="presentation"
  >
    <div
      class="bg-bg-secondary border-2 border-warning/60 rounded-xl shadow-2xl w-full max-w-md"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
    >
      <div class="p-5 border-b border-border">
        <div class="flex items-center gap-3">
          <svg class="w-6 h-6 text-warning flex-shrink-0" viewBox="0 0 24 24" fill="currentColor">
            <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
          </svg>
          <h3 class="text-lg font-bold text-text-primary uppercase">Are you sure you want to delete?</h3>
        </div>
      </div>
      <div class="p-5 space-y-3 text-sm">
        <p class="text-text-primary">
          You're about to remove <strong class="font-mono text-warning">{deleteConfirmTarget.displayName}</strong> from the ArcheRage addon registry.
        </p>
        <ul class="text-text-secondary space-y-1.5 text-xs list-disc list-inside ml-1">
          <li>The YAML will be deleted from the registry repo.</li>
          <li>Anyone who currently has it installed keeps their copy, but won't get future updates.</li>
          <li>New users won't be able to find or install it.</li>
          <li>Any pending submissions for this addon — yours and others' — will be closed.</li>
          <li><strong class="text-text-primary">This cannot be undone.</strong> Re-publishing later requires a fresh submission.</li>
        </ul>
        <div class="pt-2">
          <label class="block text-xs text-text-secondary mb-1.5">
            Type <code class="text-warning font-mono">{deleteConfirmTarget.addon.addon_slug}</code> to confirm:
          </label>
          <input
            type="text"
            bind:value={typedConfirm}
            placeholder={deleteConfirmTarget.addon.addon_slug}
            autofocus
            class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm font-mono focus:outline-none focus:border-warning"
          />
        </div>
      </div>
      <div class="p-5 border-t border-border flex justify-end gap-2">
        <button
          on:click={cancelDeleteAddon}
          class="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={confirmDeleteAddon}
          disabled={typedConfirm.trim() !== deleteConfirmTarget.addon.addon_slug || deleting[deleteConfirmTarget.addon.id]}
          class="px-5 py-2 bg-warning hover:bg-warning/80 text-bg-primary rounded-lg text-sm font-bold uppercase disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-2"
        >
          {#if deleting[deleteConfirmTarget.addon.id]}<Spinner size="sm" />{/if}
          {deleting[deleteConfirmTarget.addon.id] ? 'Deleting…' : 'Delete forever'}
        </button>
      </div>
    </div>
  </div>
{/if}
