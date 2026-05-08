<script>
  import { onMount } from 'svelte';
  import { showNotification } from '../stores/app.js';
  import { GetAddonPath, SetAddonPath, SelectFolder, OpenLogFolder } from '../../../wailsjs/go/main/App.js';

  let tab = 'downloads';

  let addonPath = '';
  let saving = false;

  onMount(async () => {
    try {
      addonPath = await GetAddonPath();
    } catch (e) {
      console.error('Failed to load settings:', e);
    }
  });

  async function handleBrowse() {
    try {
      const selected = await SelectFolder();
      if (selected) addonPath = selected;
    } catch (e) {
      console.error('Failed to select folder:', e);
    }
  }

  async function handleSave() {
    saving = true;
    try {
      await SetAddonPath(addonPath);
      showNotification('Settings saved', 'success');
    } catch (e) {
      showNotification(`Failed to save: ${e}`, 'error');
    }
    saving = false;
  }

  async function openLogs() {
    try {
      await OpenLogFolder();
    } catch (e) {
      showNotification(`Couldn't open log folder: ${e}`, 'error');
    }
  }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header + tabs -->
  <div class="p-4 pr-16 border-b border-border bg-bg-secondary">
    <h2 class="text-lg font-bold text-text-primary">Settings</h2>
    <div class="flex gap-1 mt-3 border-b border-border -mb-4">
      <button
        on:click={() => (tab = 'downloads')}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {tab === 'downloads' ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}"
      >
        Downloads
      </button>
      <button
        on:click={() => (tab = 'how-to')}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {tab === 'how-to' ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}"
      >
        How to
      </button>
      <button
        on:click={() => (tab = 'faq')}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {tab === 'faq' ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}"
      >
        FAQ
      </button>
    </div>
  </div>

  <!-- Body -->
  <div class="flex-1 overflow-y-auto p-6">
    <div class="max-w-2xl mx-auto w-full space-y-6">

      {#if tab === 'downloads'}
        <div class="bg-bg-secondary border border-border rounded-lg p-6">
          <h3 class="font-medium text-text-primary mb-2">Addon Installation Path</h3>
          <p class="text-sm text-text-muted mb-4">
            This is where addons will be installed. Make sure this matches your ArcheRage addon folder.
          </p>
          <div class="flex gap-2">
            <input
              type="text"
              bind:value={addonPath}
              class="flex-1 px-4 py-2.5 bg-bg-primary border border-border rounded-lg focus:outline-none focus:border-accent text-sm"
              placeholder="C:\Users\Username\Documents\ArcheRage\Addon"
            />
            <button
              on:click={handleBrowse}
              class="px-4 py-2.5 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-sm"
            >
              Browse
            </button>
            <button
              on:click={handleSave}
              disabled={saving}
              class="px-4 py-2.5 bg-accent hover:bg-accent-hover text-white rounded-lg transition-colors disabled:opacity-50 text-sm"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>

          <!-- OneDrive warning -->
          <div class="mt-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2.5 text-xs text-text-secondary">
            <div class="flex items-start gap-2">
              <svg class="w-4 h-4 text-warning flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 9v4M12 17h.01"/>
                <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
              </svg>
              <span>
                <strong class="text-text-primary">Using OneDrive?</strong> If your Documents folder is synced to OneDrive,
                addons usually need to live inside the OneDrive copy of <code>Documents\ArcheRage\Addon</code> for the game
                to load them. If addons aren't appearing in-game, double-check this path matches the folder ArcheRage actually
                reads from.
              </span>
            </div>
          </div>
        </div>

        <!-- Diagnostics -->
        <div class="bg-bg-secondary border border-border rounded-lg p-6">
          <h3 class="font-medium text-text-primary mb-2">Diagnostics</h3>
          <p class="text-sm text-text-muted mb-4">
            The manager writes a fresh log file on every launch. If you hit a bug, click below
            to find <code>manager.log</code> and attach it to your report.
          </p>
          <button
            on:click={openLogs}
            class="px-4 py-2 bg-bg-tertiary hover:bg-border rounded-lg transition-colors text-sm flex items-center gap-2"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
              <path d="M14 2v6h6M16 13H8M16 17H8M10 9H8"/>
            </svg>
            Open log folder
          </button>
        </div>

      {:else if tab === 'how-to'}
        <div class="bg-bg-secondary border border-border rounded-lg p-6 space-y-4">
          <div>
            <h3 class="font-medium text-text-primary mb-1">How to publish an addon</h3>
            <p class="text-sm text-text-muted">
              Everything happens inside the manager — no need to write YAML or open pull requests by hand.
            </p>
          </div>

          <ol class="space-y-3 text-sm text-text-secondary list-decimal list-inside marker:text-accent">
            <li>
              <strong class="text-text-primary">Sign in with Discord</strong> — bottom-left of the sidebar.
              This identifies you to the registry. We never ask for your email.
            </li>
            <li>
              <strong class="text-text-primary">Open <em>My Addons</em></strong> from the sidebar (only visible once signed in).
            </li>
            <li>
              <strong class="text-text-primary">Click <em>Submit New Addon</em></strong>. A 3-step form opens.
            </li>
            <li>
              <strong class="text-text-primary">Step 1 — Connect GitHub</strong>. We ask GitHub to confirm you have access
              to the repo your addon lives in. The code requests no write permissions.
            </li>
            <li>
              <strong class="text-text-primary">Step 2 — Pick repo, branch, subfolder</strong>. Use a subfolder if your repo
              has multiple addons or non-addon files. The current branch HEAD gets pinned automatically — see the security
              note below.
            </li>
            <li>
              <strong class="text-text-primary">Step 3 — Fill in details</strong>. Display name, folder name (must match
              what the game looks for inside <code>Addon/</code>), version, description, category, optional icon URL,
              keywords, and dependencies.
            </li>
            <li>
              <strong class="text-text-primary">Submit</strong>. A maintainer reviews your submission. You'll see status
              updates in My Addons (pending → approved or denied with a reason).
            </li>
          </ol>
        </div>

        <div class="bg-bg-secondary border border-border rounded-lg p-6 space-y-3">
          <h3 class="font-medium text-text-primary">Updating an existing addon</h3>
          <p class="text-sm text-text-secondary">
            In <em>My Addons</em>, click <strong class="text-text-primary">Update</strong> next to your addon. The form
            pre-fills with the previous values and auto-bumps the version. The latest commit on your source branch gets
            re-pinned, so the maintainer sees exactly what changed since the last approval.
          </p>
        </div>

        <div class="bg-bg-secondary border border-border rounded-lg p-6 space-y-3">
          <h3 class="font-medium text-text-primary">Removing an addon</h3>
          <p class="text-sm text-text-secondary">
            Click the trash icon on the addon's row in <em>My Addons</em> and type the addon name to confirm. The YAML
            gets deleted from the registry, any pending updates are closed, and users with it installed keep their copy
            but stop receiving updates.
          </p>
        </div>

        <div class="bg-bg-secondary border border-border rounded-lg p-6 space-y-3">
          <h3 class="font-medium text-text-primary">Security checks we run automatically</h3>
          <ul class="text-sm text-text-secondary space-y-1.5 list-disc list-inside marker:text-accent">
            <li>
              <strong class="text-text-primary">Commit pinning</strong> — every submission locks in the exact commit SHA
              from your source branch. Users only ever download the bytes a maintainer reviewed; pushing new code to
              your branch later won't reach users until you submit an update.
            </li>
            <li>
              <strong class="text-text-primary">Dangerous-file scan</strong> — we scan the source folder for
              <code>.bat / .ps1 / .exe / .cmd / .vbs</code> files at submission time. Any matches show up in the PR for
              the maintainer and as a warning badge for users on the addon's page.
            </li>
            <li>
              <strong class="text-text-primary">Manual review</strong> — every submission is approved or denied by a
              human. There's no auto-merge.
            </li>
          </ul>
        </div>

      {:else if tab === 'faq'}
        <div class="bg-bg-secondary border border-border rounded-lg p-6 space-y-2">
          <p class="text-sm text-text-muted mb-3">
            Common questions. Click a question to expand the answer.
          </p>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Is the manager safe to install?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                The source code lives at
                <code>github.com/ArcheRageAddons/ArcheRageAddonManager</code> and every release on the
                Releases page is built directly from a tagged commit by GitHub Actions — what's in the
                <code>.exe</code> matches what's in the source.
              </p>
              <p>
                The app doesn't require administrator privileges, doesn't install anything system-wide,
                and only writes to <code>%APPDATA%\ArcheRageAddonManager</code> (its own settings) and the
                ArcheRage <code>Addon</code> folder you configure.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Why does the manager ask me to sign in with Discord?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                Only authors who want to <strong>publish</strong> addons need to sign in. Browsing and
                installing don't require any login at all.
              </p>
              <p>
                Discord acts as the identity layer for the registry — it ties each submission to a real
                Discord account so maintainers can moderate consistently. We request only the
                <code>identify</code> scope, which gives us your Discord ID, username, and avatar.
                <strong>We don't ask for or receive your email.</strong>
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Why does the manager ask me to sign in with GitHub when I publish an addon?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                We use it to confirm you actually have access to the repo you're claiming the addon comes from.
                Without it, anyone could publish someone else's code.
              </p>
              <p>
                The login flow uses GitHub's Device Flow with no scopes requested — only the bare minimum
                needed to list the repos you can write to and resolve the current commit on the branch you pick.
                <strong>The manager never gets write access to your repos.</strong>
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Where do my login tokens get stored?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                On Windows, both the Discord and GitHub tokens live in the
                <strong>Windows Credential Manager</strong> — the same OS-level encrypted store browsers and email
                clients use. They're never written to a file in plaintext.
              </p>
              <p>
                Click <em>Sign out</em> in the sidebar to clear them. To wipe them manually:
                <em>Control Panel → Credential Manager → Windows Credentials</em> and delete entries
                under <code>ArcheRageAddonManager</code>.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">My addon isn't showing up in-game even though it installed. What now?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>Common causes, in order of likelihood:</p>
              <ol class="list-decimal list-inside space-y-1 marker:text-accent">
                <li>
                  <strong>Addon not enabled on the character select screen</strong> — installing an
                  addon only puts the files in place; ArcheRage still needs you to tick it on. Open the
                  <em>Addons</em> menu on the character select screen, find the addon in the list, and
                  enable it. Then enter the world.
                </li>
                <li>
                  <strong>OneDrive path mismatch</strong> — if your Documents folder is synced to OneDrive,
                  ArcheRage often expects the addon under the OneDrive copy of <code>Documents\ArcheRage\Addon</code>
                  rather than the regular one. Update the path in the <em>Downloads</em> tab and reinstall.
                </li>
                <li>
                  <strong>Game wasn't restarted</strong> — ArcheRage only loads addons at startup. Fully close and
                  relaunch the game.
                </li>
                <li>
                  <strong>Folder name conflict</strong> — if you previously installed the addon under a different
                  folder name, the leftover folder can confuse the loader. Uninstall via the manager's
                  <em>Installed</em> tab and reinstall fresh.
                </li>
              </ol>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Why isn't my submitted addon live yet?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                Every submission is reviewed by a human maintainer — there's no auto-merge. Until it's
                approved, your addon stays in the <em>pending</em> queue. You can see its current status
                under <em>My Addons → Submission history</em>.
              </p>
              <p>
                Review times vary depending on how busy the maintainers are. If you've been waiting more
                than a few days, ping in the community Discord to nudge.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">My update was denied — what should I do?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                The reason from the reviewing maintainer is shown on the denied submission row in
                <em>My Addons</em> (the italic "Reviewer note: …" line). Read it, address the issue in your
                source repo, then click <em>Submit New Addon</em> again with the fixes.
              </p>
              <p>
                The most common reasons for denial are: dangerous executables in the addon folder, the
                <code>folder_name</code> not matching what the game expects, source repo or path that
                doesn't resolve, and metadata that doesn't match what the addon actually does.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">How do I report an addon that's broken or malicious?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p><strong class="text-text-primary">Broken addons</strong> — contact the addon author directly. The
                author's GitHub repo is linked from the addon's details modal under <em>View on GitHub</em>; open an
                issue or reach out however the author has documented. Manager maintainers don't write or maintain the
                addons themselves, so they can't fix bugs in someone else's code.</p>
              <p><strong class="text-text-primary">Malicious addons</strong> — contact a maintainer of the manager.
                The fastest paths are pinging a maintainer in the ArcheRage community Discord, or opening an issue at
                <code>github.com/ArcheRageAddons/addons</code> describing what you've seen.</p>
              <p>
                Maintainers can remove an approved addon at any time — it disappears from the registry
                and stops showing up for new installs. Users who already installed it keep their copy
                but stop receiving updates.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Where can I see what changed between addon versions?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>
                Each approved version of an addon is pinned to a specific GitHub commit. The addon's
                source repository and that commit are visible in the addon details modal — click through
                to GitHub and use its compare view to diff between releases.
              </p>
              <p>
                Authors are encouraged (but not required) to bump the <code>version</code> field
                meaningfully when they publish updates so users can tell at a glance what kind of change
                it is.
              </p>
            </div>
          </details>

          <details class="group bg-bg-primary border border-border rounded-lg overflow-hidden">
            <summary class="px-4 py-3 cursor-pointer text-sm text-text-primary hover:bg-bg-tertiary flex items-center gap-2 list-none">
              <svg class="w-3 h-3 text-text-muted transition-transform group-open:rotate-90 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
              <span class="flex-1">Can I run the manager on Mac or Linux?</span>
            </summary>
            <div class="px-4 pb-4 pt-1 text-sm text-text-secondary space-y-2">
              <p>At this time we do not support macOS or Linux.</p>
            </div>
          </details>
        </div>
      {/if}

    </div>
  </div>
</div>
