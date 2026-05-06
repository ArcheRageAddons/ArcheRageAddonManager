<script>
  import { onDestroy } from 'svelte';
  import { showSubmitModal, currentUser, showNotification, submitPrefill } from '../stores/app.js';
  import {
    GetCategories,
    StartGitHubAuth,
    GetGitHubUser,
    LogoutGitHub,
    ListMyRepos,
    SubmitAddon,
    OpenURL,
    GetAddons,
  } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import Spinner from './Spinner.svelte';

  let categories = [];
  let availableAddons = []; // every approved addon — for the dependency picker
  let selectedDeps = new Set(); // addon IDs the user has ticked
  let depSearch = '';
  let depsOpen = false;

  // GitHub connection state
  let ghUser = null;
  let ghBusy = false;
  let ghDeviceFlow = null; // { user_code, verification_uri, expires_in, interval }

  // Repo list state
  let repos = [];
  let reposLoading = false;
  let reposError = '';

  // Form
  let form = {
    name: '',
    folder_name: '',
    description: '',
    author: '',
    version: '1.0.0',
    category: 'Other',
    keywords: '',
    icon: '',
    github_repo: '',
    github_branch: 'main',
    github_path: '',
  };

  let submitting = false;
  let isUpdate = false;

  // Listen for the background polling result.
  let unsub = null;
  $: if ($showSubmitModal && !unsub) {
    unsub = EventsOn('github:auth:done', (payload) => {
      ghBusy = false;
      ghDeviceFlow = null;
      if (payload?.ok) {
        ghUser = payload.user;
        showNotification(`Connected to GitHub as ${payload.user.login}`, 'success');
        loadRepos();
      } else {
        showNotification(`GitHub login failed: ${payload?.error || 'unknown'}`, 'error', 8000);
      }
    });
  }

  onDestroy(() => {
    if (unsub) {
      EventsOff('github:auth:done');
      unsub = null;
    }
  });

  // Initialise on open
  $: if ($showSubmitModal) {
    initOnOpen();
  }

  async function initOnOpen() {
    // Prefill from an Update click on My Addons (consume + clear).
    const prefill = $submitPrefill;
    if (prefill) {
      form = {
        name: prefill.name || '',
        folder_name: prefill.folder_name || '',
        description: prefill.description || '',
        author: prefill.author || $currentUser?.discord_username || '',
        version: prefill.version || '1.0.0',
        category: prefill.category || 'Other',
        keywords: (prefill.keywords || []).join(', '),
        icon: prefill.icon || '',
        github_repo: prefill.github_repo || '',
        github_branch: prefill.github_branch || 'main',
        github_path: prefill.github_path || '',
      };
      selectedDeps = new Set(prefill.dependencies || []);
      isUpdate = true;
      submitPrefill.set(null);
    } else {
      isUpdate = false;
      selectedDeps = new Set();
      if ($currentUser && !form.author) {
        form.author = $currentUser.discord_username || '';
      }
    }
    depSearch = '';

    // Fetch all approved addons for the dependency picker. RegistryClient
    // caches in-memory so repeat opens are instant.
    if (availableAddons.length === 0) {
      try {
        const all = await GetAddons();
        availableAddons = (all || []).slice().sort((a, b) =>
          (a.name || a.id).localeCompare(b.name || b.id),
        );
      } catch (e) {
        console.error('Failed to load addons for dependency picker:', e);
        availableAddons = [];
      }
    }

    if (categories.length === 0) {
      try {
        const c = await GetCategories();
        categories = (c || []).filter((x) => x !== 'All');
      } catch {
        categories = ['Other'];
      }
    }
    if (!ghUser) {
      try {
        const u = await GetGitHubUser();
        if (u) {
          ghUser = u;
          loadRepos();
        }
      } catch (e) {
        console.error('GetGitHubUser failed:', e);
      }
    }
  }

  async function connectGithub() {
    ghBusy = true;
    try {
      ghDeviceFlow = await StartGitHubAuth();
      // Open the verification URL in the user's browser. They'll type the
      // user_code displayed below into the page.
      if (ghDeviceFlow?.verification_uri) {
        await OpenURL(ghDeviceFlow.verification_uri);
      }
      showNotification(
        `Enter the code below at github.com/login/device — code stays valid for ${Math.floor((ghDeviceFlow.expires_in || 0) / 60)} min`,
        'info', 5000,
      );
    } catch (e) {
      ghBusy = false;
      ghDeviceFlow = null;
      showNotification(`Failed to start GitHub login: ${e}`, 'error', 8000);
    }
  }

  async function disconnectGithub() {
    try {
      await LogoutGitHub();
      ghUser = null;
      repos = [];
      form.github_repo = '';
      showNotification('Disconnected from GitHub', 'info');
    } catch (e) {
      showNotification(`Logout failed: ${e}`, 'error');
    }
  }

  async function loadRepos() {
    reposLoading = true;
    reposError = '';
    try {
      const r = await ListMyRepos();
      repos = r || [];
      if (repos.length === 0) {
        reposError = 'No writable repos found on your account.';
      }
    } catch (e) {
      reposError = String(e);
    }
    reposLoading = false;
  }

  // When the user picks a repo, default the branch to that repo's default.
  function onRepoChange() {
    const r = repos.find((x) => x.full_name === form.github_repo);
    if (r) {
      form.github_branch = r.default_branch || 'main';
    }
  }

  async function copyCode() {
    if (ghDeviceFlow?.user_code) {
      await navigator.clipboard.writeText(ghDeviceFlow.user_code);
      showNotification('Code copied', 'success', 1500);
    }
  }

  async function submit() {
    submitting = true;
    try {
      const result = await SubmitAddon({
        name: form.name,
        folder_name: form.folder_name,
        author: form.author,
        version: form.version,
        description: form.description,
        category: form.category,
        keywords: form.keywords.split(',').map((k) => k.trim()).filter(Boolean),
        icon: form.icon,
        dependencies: Array.from(selectedDeps),
        github_repo: form.github_repo,
        github_branch: form.github_branch,
        github_path: form.github_path,
      });
      const verb = isUpdate ? 'Update submitted' : 'Submitted';
      if (result?.pr_url) {
        showNotification(`${verb}! PR #${result.pr_number} opened.`, 'success', 6000);
      } else {
        showNotification(`${verb} for review!`, 'success', 4000);
      }
      resetForm();
      showSubmitModal.set(false);
      window.dispatchEvent(new CustomEvent('submission-created'));
    } catch (e) {
      const raw = String(e);
      // Errors that come back as "submission failed: ..." originate from
      // the Edge Function — almost always a GitHub-side or server-config
      // problem the user can't fix directly. Show a friendly toast and
      // leave the technical detail in the JS console for diagnosis.
      const isServerSide = /submission failed:/i.test(raw);
      console.error('[submit] failed:', raw);
      if (isServerSide) {
        showNotification(
          "Couldn't connect to GitHub. Try signing out of GitHub above and reconnecting — if it keeps failing, please message an admin for help.",
          'error',
          10000,
        );
      } else {
        // Client-side / validation / not-signed-in errors are usually
        // actionable by the user, so show the actual message.
        showNotification(raw, 'error', 8000);
      }
    }
    submitting = false;
  }

  function resetForm() {
    form = {
      name: '',
      folder_name: '',
      description: '',
      author: $currentUser?.discord_username || '',
      version: '1.0.0',
      category: 'Other',
      keywords: '',
      icon: '',
      github_repo: '',
      github_branch: 'main',
      github_path: '',
    };
    selectedDeps = new Set();
    depSearch = '';
    depsOpen = false;
    isUpdate = false;
  }

  function close() {
    showSubmitModal.set(false);
    submitPrefill.set(null);
    resetForm();
  }

  $: canSubmit =
    !!ghUser &&
    !submitting &&
    form.name.trim() &&
    form.folder_name.trim() &&
    form.author.trim() &&
    form.version.trim() &&
    form.github_repo.trim();

  // Dependency picker: filter the registry-wide list by current search,
  // and exclude the addon we're currently submitting/updating (can't depend
  // on yourself). The currently-selected slug is determined by folder_name
  // since that's the canonical addon ID.
  $: depCandidates = availableAddons.filter((a) => {
    const ownSlug = (form.folder_name || '').trim().toLowerCase();
    if (a.id && a.id.toLowerCase() === ownSlug) return false;
    if (!depSearch.trim()) return true;
    const q = depSearch.trim().toLowerCase();
    return (
      (a.name || '').toLowerCase().includes(q) ||
      (a.id || '').toLowerCase().includes(q) ||
      (a.author_name || '').toLowerCase().includes(q)
    );
  });

  function toggleDep(id) {
    const next = new Set(selectedDeps);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selectedDeps = next;
  }

  function clearDeps() {
    selectedDeps = new Set();
  }
</script>

{#if $showSubmitModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4"
    on:click={close}
    on:keydown={(e) => e.key === 'Escape' && close()}
    role="presentation"
  >
    <div
      class="bg-bg-secondary border border-border rounded-xl shadow-2xl w-full max-w-2xl max-h-[90vh] overflow-y-auto"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="p-5 border-b border-border flex items-center justify-between sticky top-0 bg-bg-secondary z-10">
        <div>
          <h2 class="text-lg font-bold text-text-primary">
            {isUpdate ? `Update ${form.folder_name || 'addon'}` : 'Submit a new addon'}
          </h2>
          <p class="text-xs text-text-muted mt-0.5">
            {isUpdate
              ? "Latest commit on the source branch will be pinned automatically. Bump the version if you'd like."
              : 'A maintainer reviews every submission before it goes live.'}
          </p>
        </div>
        <button
          on:click={close}
          class="text-text-muted hover:text-text-primary p-1"
          aria-label="Close"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6 6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>

      <!-- Step 1: GitHub connection -->
      <div class="p-5 border-b border-border">
        <div class="flex items-center gap-3 mb-3">
          <div class="w-7 h-7 rounded-full {ghUser ? 'bg-success' : 'bg-bg-tertiary border border-border'} flex items-center justify-center text-xs font-bold {ghUser ? 'text-white' : 'text-text-secondary'}">
            {ghUser ? '✓' : '1'}
          </div>
          <h3 class="font-medium text-text-primary text-sm">Connect GitHub</h3>
        </div>

        {#if ghUser}
          <div class="ml-10 flex items-center gap-3">
            <img src={ghUser.avatar_url} alt="" class="w-7 h-7 rounded-full" />
            <div class="flex-1">
              <div class="text-sm text-text-primary">@{ghUser.login}</div>
              {#if ghUser.name}
                <div class="text-xs text-text-muted">{ghUser.name}</div>
              {/if}
            </div>
            <button
              on:click={disconnectGithub}
              class="text-xs text-text-muted hover:text-text-primary"
            >
              Disconnect
            </button>
          </div>
        {:else if ghDeviceFlow}
          <div class="ml-10 space-y-3">
            <p class="text-xs text-text-muted">
              We've opened <a href="#" on:click|preventDefault={() => OpenURL(ghDeviceFlow.verification_uri)} class="text-accent underline">{ghDeviceFlow.verification_uri}</a> in your browser. Enter this code:
            </p>
            <div class="flex items-center gap-2">
              <code class="flex-1 px-4 py-3 bg-bg-primary border border-border rounded-lg text-2xl font-mono text-text-primary text-center tracking-[0.3em]">
                {ghDeviceFlow.user_code}
              </code>
              <button
                on:click={copyCode}
                class="px-3 py-3 bg-bg-tertiary hover:bg-border rounded-lg text-xs text-text-secondary"
                title="Copy code"
              >
                Copy
              </button>
            </div>
            <p class="text-xs text-text-muted">
              Waiting for you to authorize... this dialog will update automatically.
            </p>
          </div>
        {:else}
          <div class="ml-10">
            <p class="text-xs text-text-muted mb-3">
              We use your GitHub login to confirm you have write access to the repo this addon comes from. No write permissions are requested — only the right to read your profile.
            </p>
            <button
              on:click={connectGithub}
              disabled={ghBusy}
              class="px-4 py-2 bg-[#1f2328] hover:bg-[#2d333b] text-white rounded-lg text-sm font-medium flex items-center gap-2 border border-border disabled:opacity-60"
            >
              {#if ghBusy}
                <Spinner size="sm" />
              {:else}
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.34-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.78 1.19 1.78 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.79.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/>
                </svg>
              {/if}
              {ghBusy ? 'Starting...' : 'Continue with GitHub'}
            </button>
          </div>
        {/if}
      </div>

      <!-- Step 2: Repo + folder -->
      <div class="p-5 border-b border-border {!ghUser ? 'opacity-50 pointer-events-none' : ''}">
        <div class="flex items-center gap-3 mb-3">
          <div class="w-7 h-7 rounded-full bg-bg-tertiary border border-border flex items-center justify-center text-xs font-bold text-text-secondary">2</div>
          <h3 class="font-medium text-text-primary text-sm">Choose repository &amp; folder</h3>
        </div>
        <div class="ml-10 space-y-3">
          <div>
            <label class="block text-xs text-text-secondary mb-1.5">Repository</label>
            {#if reposLoading}
              <div class="text-xs text-text-muted">Loading your repos...</div>
            {:else if reposError && repos.length === 0}
              <div class="text-xs text-warning">{reposError}</div>
            {:else}
              <select
                bind:value={form.github_repo}
                on:change={onRepoChange}
                disabled={!ghUser}
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent disabled:opacity-60"
              >
                <option value="">{ghUser ? `Select a repo... (${repos.length} available)` : 'Connect GitHub first'}</option>
                {#each repos as r}
                  <option value={r.full_name}>{r.full_name}{r.private ? ' (private)' : ''}</option>
                {/each}
              </select>
            {/if}
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Branch</label>
              <input
                type="text"
                bind:value={form.github_branch}
                disabled={!ghUser}
                placeholder="main"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent disabled:opacity-60"
              />
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Subfolder (optional)</label>
              <input
                type="text"
                bind:value={form.github_path}
                disabled={!ghUser}
                placeholder="leave empty for repo root"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent disabled:opacity-60"
              />
            </div>
          </div>
          <p class="text-xs text-text-muted">
            For multi-addon repos, pick the subfolder that contains just this addon.
          </p>
        </div>
      </div>

      <!-- Step 3: Addon details -->
      <div class="p-5">
        <div class="flex items-center gap-3 mb-3">
          <div class="w-7 h-7 rounded-full bg-bg-tertiary border border-border flex items-center justify-center text-xs font-bold text-text-secondary">3</div>
          <h3 class="font-medium text-text-primary text-sm">Addon details</h3>
        </div>
        <div class="ml-10 space-y-3">
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Display name <span class="text-warning">*</span></label>
              <input
                type="text"
                bind:value={form.name}
                placeholder="DPS Meter"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Folder name <span class="text-warning">*</span></label>
              <input
                type="text"
                bind:value={form.folder_name}
                placeholder="dpsmeter"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              />
            </div>
          </div>
          <p class="text-xs text-text-muted -mt-1">
            Folder name is what the game looks for inside <code>Addon/</code>. Often must match exactly.
          </p>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Author <span class="text-warning">*</span></label>
              <input
                type="text"
                bind:value={form.author}
                placeholder="Your name or handle"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Version <span class="text-warning">*</span></label>
              <input
                type="text"
                bind:value={form.version}
                placeholder="1.0.0"
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              />
            </div>
          </div>

          <div>
            <label class="block text-xs text-text-secondary mb-1.5">Description</label>
            <textarea
              bind:value={form.description}
              rows="3"
              placeholder="What does this addon do?"
              class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent resize-none"
            ></textarea>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Category</label>
              <select
                bind:value={form.category}
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              >
                {#each categories as c}
                  <option value={c}>{c}</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Icon URL (optional)</label>
              <input
                type="text"
                bind:value={form.icon}
                placeholder="https://..."
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
              />
              <p class="text-[10px] text-text-muted mt-1.5 break-all leading-snug">
                e.g. <code>https://raw.githubusercontent.com/owner/repo/main/icon.png</code> — point at a raw image file (PNG, ~128×128).
              </p>
            </div>
          </div>

          <div>
            <label class="block text-xs text-text-secondary mb-1.5">Keywords (comma-separated)</label>
            <input
              type="text"
              bind:value={form.keywords}
              placeholder="dps, combat, meter"
              class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
            />
          </div>

          <!-- Dependency picker (collapsible dropdown) -->
          <div>
            <label class="block text-xs text-text-secondary mb-1.5">Dependencies (optional)</label>
            <button
              type="button"
              on:click={() => (depsOpen = !depsOpen)}
              class="w-full flex items-center justify-between px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm hover:border-text-muted transition-colors"
            >
              <span class="text-text-primary">
                {#if selectedDeps.size === 0}
                  <span class="text-text-muted">Select dependencies…</span>
                {:else}
                  {selectedDeps.size} addon{selectedDeps.size === 1 ? '' : 's'} required
                {/if}
              </span>
              <svg
                class="w-4 h-4 text-text-muted transition-transform {depsOpen ? 'rotate-180' : ''}"
                viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
              >
                <path d="M6 9l6 6 6-6"/>
              </svg>
            </button>

            {#if depsOpen}
              <div class="mt-1.5 bg-bg-primary border border-border rounded-lg overflow-hidden">
                <div class="flex items-center gap-2 px-3 py-2 border-b border-border">
                  <input
                    type="text"
                    bind:value={depSearch}
                    placeholder="Search by name, ID, or author…"
                    class="flex-1 px-2 py-1 bg-bg-secondary border border-border rounded text-sm focus:outline-none focus:border-accent"
                  />
                  {#if selectedDeps.size > 0}
                    <button
                      type="button"
                      on:click={clearDeps}
                      class="text-[10px] text-text-muted hover:text-warning whitespace-nowrap"
                    >
                      Clear all
                    </button>
                  {/if}
                </div>
                <div class="max-h-56 overflow-y-auto">
                  {#if availableAddons.length === 0}
                    <div class="text-xs text-text-muted px-3 py-4 text-center">
                      Loading addons…
                    </div>
                  {:else if depCandidates.length === 0}
                    <div class="text-xs text-text-muted px-3 py-4 text-center">
                      No addons match.
                    </div>
                  {:else}
                    {#each depCandidates as a}
                      {@const checked = selectedDeps.has(a.id)}
                      <button
                        type="button"
                        on:click={() => toggleDep(a.id)}
                        class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-bg-tertiary text-sm text-left transition-colors"
                      >
                        <!-- Custom dark checkbox -->
                        <span
                          class="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 transition-colors {checked
                            ? 'bg-accent border-accent'
                            : 'bg-bg-secondary border-border'}"
                        >
                          {#if checked}
                            <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                              <path d="M20 6L9 17l-5-5"/>
                            </svg>
                          {/if}
                        </span>
                        <span class="flex-1 min-w-0 truncate">
                          <span class="text-text-primary">{a.name || a.id}</span>
                          <span class="text-xs text-text-muted ml-2">
                            v{a.version}{a.author_name ? ` · ${a.author_name}` : ''}
                          </span>
                        </span>
                        <span class="text-[10px] text-text-muted font-mono flex-shrink-0">{a.id}</span>
                      </button>
                    {/each}
                  {/if}
                </div>
              </div>
            {/if}

            <p class="text-[10px] text-text-muted mt-1.5 leading-snug">
              Users installing your addon will be told to also install these. The manager shows their install status in the addon's details.
            </p>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="p-5 border-t border-border flex items-center justify-between sticky bottom-0 bg-bg-secondary">
        <button
          on:click={close}
          class="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={submit}
          disabled={!canSubmit}
          class="px-5 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
        >
          {#if submitting}<Spinner size="sm" />{/if}
          {submitting ? 'Submitting...' : (isUpdate ? 'Submit update for review' : 'Submit for review')}
        </button>
      </div>
    </div>
  </div>
{/if}
