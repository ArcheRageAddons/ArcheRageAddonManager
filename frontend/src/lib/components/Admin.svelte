<script>
  import { onMount } from 'svelte';
  import { showNotification } from '../stores/app.js';
  import {
    GetPendingSubmissions,
    ApproveSubmission,
    DenySubmission,
    GetAllUsers,
    OpenURL,
  } from '../../../wailsjs/go/main/App.js';
  import Spinner from './Spinner.svelte';
  import { modalBackdrop, modalContent } from '../motion.js';

  let tab = 'submissions';   // 'submissions' | 'users'
  let submissions = [];
  let loading = true;
  let busy = {};        // map: submission_id -> bool
  let expanded = {};
  let denyTarget = null;
  let denyReason = '';

  let users = [];
  let usersLoading = false;
  let usersLoaded = false;
  let userSearch = '';

  $: filteredUsers = userSearch.trim()
    ? users.filter((u) => {
        const q = userSearch.toLowerCase();
        return (
          (u.discord_username || '').toLowerCase().includes(q) ||
          (u.discord_id || '').includes(q) ||
          (u.github_login || '').toLowerCase().includes(q)
        );
      })
    : users;

  async function loadUsers() {
    usersLoading = true;
    try {
      users = (await GetAllUsers()) || [];
      usersLoaded = true;
    } catch (e) {
      showNotification(`Failed to load users: ${e}`, 'error', 6000);
      users = [];
    }
    usersLoading = false;
  }

  function selectTab(name) {
    tab = name;
    if (name === 'users' && !usersLoaded) loadUsers();
  }

  function fmtJoined(s) {
    return new Date(s).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
    });
  }

  async function copy(text) {
    try {
      await navigator.clipboard.writeText(text);
      showNotification('Copied', 'success', 1200);
    } catch {}
  }

  function parseDangerousFromYAML(yaml) {
    if (!yaml) return { has: false, files: [] };
    const flag = yaml.match(/^has_dangerous_files\s*:\s*(true|false)\s*$/m);
    if (!flag || flag[1] !== 'true') return { has: false, files: [] };

    const files = [];
    let inList = false;
    for (const raw of yaml.split('\n')) {
      if (/^dangerous_files\s*:/.test(raw)) { inList = true; continue; }
      if (!inList) continue;
      const item = raw.match(/^\s+-\s*["']?(.+?)["']?\s*$/);
      if (item) {
        files.push(item[1]);
      } else if (raw.trim() && !/^\s/.test(raw)) {
        break;
      }
    }
    return { has: true, files };
  }

  async function load() {
    loading = true;
    try {
      submissions = (await GetPendingSubmissions()) || [];
    } catch (e) {
      showNotification(`Failed to load submissions: ${e}`, 'error', 8000);
      submissions = [];
    }
    loading = false;
  }

  onMount(load);

  async function approve(s) {
    busy = { ...busy, [s.id]: true };
    try {
      await ApproveSubmission(s.id);
      showNotification(`Approved & merged: ${s.addon_slug}`, 'success', 4000);
      submissions = submissions.filter((x) => x.id !== s.id);
    } catch (e) {
      const msg = String(e);
      if (/admin only/i.test(msg)) {
        showNotification('You are not an admin.', 'error', 6000);
      } else if (/already (approved|denied|withdrawn)/i.test(msg)) {
        showNotification('Already decided — refreshing list.', 'info', 4000);
        await load();
      } else {
        showNotification(`Approve failed: ${msg}`, 'error', 8000);
      }
    }
    busy = { ...busy, [s.id]: false };
  }

  function startDeny(s) {
    denyTarget = s;
    denyReason = '';
  }

  function cancelDeny() {
    denyTarget = null;
    denyReason = '';
  }

  async function confirmDeny() {
    if (!denyTarget) return;
    const id = denyTarget.id;
    busy = { ...busy, [id]: true };
    try {
      await DenySubmission(id, denyReason.trim());
      showNotification(`Denied: ${denyTarget.addon_slug}`, 'info', 4000);
      submissions = submissions.filter((x) => x.id !== id);
      denyTarget = null;
      denyReason = '';
    } catch (e) {
      showNotification(`Deny failed: ${e}`, 'error', 8000);
    }
    busy = { ...busy, [id]: false };
  }

  function toggleExpand(id) {
    expanded = { ...expanded, [id]: !expanded[id] };
  }

  function fmtDate(s) {
    return new Date(s).toLocaleString();
  }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <div class="p-4 pr-16 border-b border-border bg-bg-secondary">
    <div class="flex justify-between items-center">
      <div>
        <h2 class="text-lg font-bold text-text-primary">Admin</h2>
        <p class="text-xs text-text-muted mt-0.5">
          {tab === 'submissions'
            ? `${submissions.length} pending submission${submissions.length === 1 ? '' : 's'}.`
            : `${users.length} user${users.length === 1 ? '' : 's'} registered.`}
        </p>
      </div>
      <button
        on:click={() => tab === 'submissions' ? load() : loadUsers()}
        title="Refresh"
        class="p-2.5 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-text-secondary"
      >
        <svg class="w-4 h-4 {(tab === 'submissions' ? loading : usersLoading) ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M23 4v6h-6M1 20v-6h6"/>
          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
        </svg>
      </button>
    </div>
    <!-- Tabs -->
    <div class="flex gap-1 mt-3 border-b border-border -mb-4">
      <button
        on:click={() => selectTab('submissions')}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {tab === 'submissions' ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}"
      >
        Pending submissions
      </button>
      <button
        on:click={() => selectTab('users')}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {tab === 'users' ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}"
      >
        Users
      </button>
    </div>
  </div>

  <!-- Body -->
  <div class="flex-1 overflow-y-auto p-4">
  {#if tab === 'submissions'}
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="text-text-secondary text-sm">Loading...</div>
      </div>
    {:else if submissions.length === 0}
      <div class="flex items-center justify-center h-full">
        <div class="text-center text-text-secondary max-w-md">
          <svg class="w-16 h-16 mx-auto mb-4 opacity-40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
            <path d="M9 12l2 2 4-4M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z"/>
          </svg>
          <p class="text-base text-text-primary">Inbox zero</p>
          <p class="text-sm mt-2 text-text-muted">
            Nothing pending review right now.
          </p>
        </div>
      </div>
    {:else}
      <div class="space-y-2">
        {#each submissions as s}
          {@const danger = parseDangerousFromYAML(s.yaml_content)}
          <div class="bg-bg-secondary border border-border rounded-lg p-4">
            <!-- Top row -->
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-medium text-text-primary">{s.addon_slug}</span>
                  {#if danger.has}
                    <svg class="w-4 h-4 text-warning flex-shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-label="Contains dangerous files">
                      <title>Contains dangerous files ({danger.files.length})</title>
                      <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
                    </svg>
                  {/if}
                  {#if s.github_pr_number}
                    <button
                      on:click={() => OpenURL(s.github_pr_url)}
                      class="text-xs text-accent hover:text-accent-hover"
                      title="Open PR on GitHub"
                    >
                      PR #{s.github_pr_number} ↗
                    </button>
                  {/if}
                </div>
                <div class="text-xs text-text-muted mt-1">
                  Submitted by <span class="text-text-secondary">@{s.submitter_name}</span>
                  · {fmtDate(s.created_at)}
                </div>
                <div class="text-xs text-text-muted mt-0.5 truncate">
                  Source: <span class="font-mono">{s.github_repo}{s.github_path ? '/' + s.github_path : ''}</span>
                </div>
                {#if s.yaml_content}
                  {@const m = s.yaml_content.match(/^\s*commit:\s*["']?([a-f0-9]{7,40})["']?\s*$/m)}
                  {#if m}
                    <div class="text-xs text-text-muted mt-0.5">
                      Pinned commit:
                      <button
                        on:click={() => OpenURL(`https://github.com/${s.github_repo}/commit/${m[1]}`)}
                        class="font-mono text-accent hover:text-accent-hover"
                        title="Open commit on GitHub"
                      >
                        {m[1].slice(0, 7)} ↗
                      </button>
                    </div>
                  {/if}
                {/if}
              </div>
              <div class="flex flex-col gap-1.5 flex-shrink-0">
                <button
                  on:click={() => approve(s)}
                  disabled={busy[s.id]}
                  class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white rounded text-xs font-medium disabled:opacity-50 flex items-center gap-1.5 justify-center min-w-[72px]"
                >
                  {#if busy[s.id]}
                    <Spinner size="sm" />
                  {:else}
                    Approve
                  {/if}
                </button>
                <button
                  on:click={() => startDeny(s)}
                  disabled={busy[s.id]}
                  class="px-3 py-1.5 bg-bg-tertiary hover:bg-warning/20 hover:text-warning border border-border rounded text-xs font-medium disabled:opacity-50 min-w-[72px]"
                >
                  Deny
                </button>
              </div>
            </div>

            <!-- Dangerous-file scan result -->
            {#if danger.has}
              <div class="mt-3 bg-warning/10 border border-warning/40 rounded-md px-3 py-2">
                <div class="flex items-center gap-2 text-xs font-semibold text-warning">
                  <svg class="w-4 h-4 flex-shrink-0" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
                  </svg>
                  Dangerous-file scan: {danger.files.length} file{danger.files.length === 1 ? '' : 's'} flagged
                </div>
                <ul class="text-xs text-text-secondary font-mono space-y-0.5 mt-1.5 ml-6">
                  {#each danger.files as f}
                    <li>· {f}</li>
                  {/each}
                </ul>
              </div>
            {/if}

            <!-- YAML preview toggle -->
            <button
              on:click={() => toggleExpand(s.id)}
              class="text-xs text-text-muted hover:text-text-primary mt-3 flex items-center gap-1"
            >
              <svg class="w-3 h-3 transition-transform {expanded[s.id] ? 'rotate-90' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 18l6-6-6-6"/>
              </svg>
              {expanded[s.id] ? 'Hide' : 'Show'} YAML
            </button>
            {#if expanded[s.id]}
              <pre class="mt-2 bg-bg-primary border border-border rounded p-3 text-xs text-text-secondary font-mono whitespace-pre-wrap break-all max-h-96 overflow-y-auto">{s.yaml_content}</pre>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {:else if tab === 'users'}
    <!-- Users tab (read-only) -->
    <div class="mb-4">
      <input
        type="text"
        bind:value={userSearch}
        placeholder="Search by username, Discord ID, or GitHub login..."
        class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent"
      />
    </div>

    {#if usersLoading}
      <div class="flex items-center justify-center h-32">
        <div class="text-text-secondary text-sm">Loading...</div>
      </div>
    {:else if filteredUsers.length === 0}
      <div class="flex items-center justify-center h-32">
        <div class="text-center text-text-secondary">
          <p class="text-sm">{users.length === 0 ? 'No users yet.' : 'No users match that search.'}</p>
        </div>
      </div>
    {:else}
      <p class="text-xs text-text-muted mb-2 italic">
        Read-only view. Admin and ban flags are set in SQL only — never via the app.
      </p>
      <div class="space-y-1">
        {#each filteredUsers as u}
          <div class="bg-bg-secondary border border-border rounded-lg px-3 py-2.5 flex items-center gap-3">
            <!-- Avatar -->
            <div class="w-9 h-9 rounded-full bg-accent/20 flex items-center justify-center flex-shrink-0 overflow-hidden">
              {#if u.discord_avatar}
                <img src={u.discord_avatar} alt="" referrerpolicy="no-referrer" class="w-full h-full object-cover" />
              {:else}
                <span class="text-accent text-sm font-bold">
                  {(u.discord_username || '?').slice(0, 1).toUpperCase()}
                </span>
              {/if}
            </div>
            <!-- Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-medium text-text-primary truncate">{u.discord_username}</span>
                {#if u.is_admin}
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-accent/20 text-accent">Admin</span>
                {/if}
                {#if u.is_banned}
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-warning/20 text-warning">Banned</span>
                {/if}
              </div>
              <div class="text-xs text-text-muted mt-0.5 flex items-center gap-3 flex-wrap">
                <button
                  on:click={() => copy(u.discord_id)}
                  class="font-mono hover:text-text-secondary"
                  title="Copy Discord ID"
                >
                  {u.discord_id}
                </button>
                {#if u.github_login}
                  <button
                    on:click={() => OpenURL(`https://github.com/${u.github_login}`)}
                    class="hover:text-accent"
                    title="Open GitHub profile"
                  >
                    @{u.github_login} ↗
                  </button>
                {/if}
              </div>
            </div>
            <div class="text-[10px] text-text-muted flex-shrink-0">
              joined {fmtJoined(u.created_at)}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
  </div>
</div>

<!-- Deny reason modal -->
{#if denyTarget}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4"
    on:click={cancelDeny}
    on:keydown={(e) => e.key === 'Escape' && cancelDeny()}
    role="presentation"
    transition:modalBackdrop
  >
    <div
      class="bg-bg-secondary border border-border rounded-xl shadow-2xl w-full max-w-md"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
      transition:modalContent
    >
      <div class="p-5 border-b border-border">
        <h3 class="text-lg font-bold text-text-primary">Deny submission</h3>
        <p class="text-sm text-text-muted mt-1">
          <span class="font-mono text-text-primary">{denyTarget.addon_slug}</span>
          by @{denyTarget.submitter_name}
        </p>
      </div>
      <div class="p-5 space-y-3">
        <label class="block text-xs text-text-secondary">
          Reason (optional, but kind to the submitter)
        </label>
        <textarea
          bind:value={denyReason}
          rows="4"
          placeholder="e.g. addon contains executables that aren't necessary, or please point at a specific subfolder rather than the repo root"
          class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent resize-none"
        ></textarea>
        <p class="text-xs text-text-muted">
          The reason gets posted as a comment on the PR and stored on the submission row.
          The PR will be closed without merging and the source branch deleted.
        </p>
      </div>
      <div class="p-5 border-t border-border flex justify-end gap-2">
        <button
          on:click={cancelDeny}
          class="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
        >
          Cancel
        </button>
        <button
          on:click={confirmDeny}
          disabled={busy[denyTarget?.id]}
          class="px-5 py-2 bg-warning/80 hover:bg-warning text-bg-primary rounded-lg text-sm font-medium disabled:opacity-50 flex items-center gap-2"
        >
          {#if busy[denyTarget?.id]}<Spinner size="sm" />{/if}
          {busy[denyTarget?.id] ? 'Denying...' : 'Confirm deny'}
        </button>
      </div>
    </div>
  </div>
{/if}
