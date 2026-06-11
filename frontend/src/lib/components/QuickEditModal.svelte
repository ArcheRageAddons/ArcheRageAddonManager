<script>
  import { createEventDispatcher } from 'svelte';
  import { showNotification } from '../stores/app.js';
  import {
    GetCategories,
    GetAddons,
    QuickEditAddon,
    RefreshAddons,
  } from '../../../wailsjs/go/main/App.js';
  import Spinner from './Spinner.svelte';
  import { modalBackdrop, modalContent } from '../motion.js';

  /** @type {{ id: string, addon_name?: string, addon_slug: string, yaml_content?: string } | null} */
  export let target = null;

  const dispatch = createEventDispatcher();

  let loaded = false;
  let loading = false;
  let saving = false;
  let categories = [];
  let availableAddons = [];

  let form = {
    name: '',
    author: '',
    description: '',
    category: 'Other',
    icon: '',
    keywords: '',
    dependencies: new Set(),
  };
  let original = null;

  let depsOpen = false;
  let depSearch = '';

  $: if (target) openFor(target);

  async function openFor(t) {
    loaded = false;
    loading = true;
    original = null;
    form = {
      name: '',
      author: '',
      description: '',
      category: 'Other',
      icon: '',
      keywords: '',
      dependencies: new Set(),
    };

    try {
      const [cats, all] = await Promise.all([
        GetCategories(),
        GetAddons(),
      ]);
      categories = (cats || []).filter((c) => c !== 'All');
      availableAddons = (all || []).slice().sort((a, b) =>
        (a.name || a.id).localeCompare(b.name || b.id),
      );
      const slug = (t.addon_slug || t.folder_name || '').toLowerCase();
      const current = availableAddons.find((a) => (a.id || '').toLowerCase() === slug);
      if (!current) {
        showNotification(`Couldn't find ${slug} in the registry`, 'error', 6000);
        loading = false;
        close();
        return;
      }

      form.name = current.name || '';
      form.author = current.author_name || '';
      form.description = current.description || '';
      form.category = current.category || 'Other';
      form.icon = current.icon || '';
      form.keywords = (current.keywords || []).join(', ');
      form.dependencies = new Set(
        (current.dependencies || []).map((d) => (typeof d === 'string' ? d : d.id)).filter(Boolean),
      );

      original = {
        name: form.name,
        author: form.author,
        description: form.description,
        category: form.category,
        icon: form.icon,
        keywords: form.keywords,
        dependencies: new Set(form.dependencies),
      };

      loaded = true;
    } catch (e) {
      showNotification(`Failed to load current values: ${e}`, 'error', 8000);
      close();
    }
    loading = false;
  }

  function close() {
    target = null;
    depsOpen = false;
    depSearch = '';
  }

  // Inline IIFE so Svelte 4 sees `form` / `original` as deps of this `$:`
  // block — a helper function call would hide them.
  $: changes = (() => {
    if (!original) return null;
    const out = {};
    const nameTrim = form.name.trim();
    const authorTrim = form.author.trim();
    const iconTrim = form.icon.trim();
    const kwArr = form.keywords.split(',').map((k) => k.trim()).filter(Boolean);
    const origKwArr = original.keywords.split(',').map((k) => k.trim()).filter(Boolean);
    const depArr = Array.from(form.dependencies).sort();
    const origDepArr = Array.from(original.dependencies).sort();

    if (nameTrim !== original.name.trim()) out.name = nameTrim;
    if (authorTrim !== original.author.trim()) out.author = authorTrim;
    if (form.description !== original.description) out.description = form.description;
    if (form.category !== original.category) out.category = form.category;
    if (iconTrim !== original.icon.trim()) out.icon = iconTrim;
    if (kwArr.join('|') !== origKwArr.join('|')) {
      out.keywords = kwArr;
      out._has_keywords = true;
    }
    if (depArr.join('|') !== origDepArr.join('|')) {
      out.dependencies = depArr;
      out._has_dependencies = true;
    }
    return out;
  })();
  $: hasChanges = changes && Object.keys(changes).length > 0;

  $: nameError = (() => {
    const n = (form.name || '').trim();
    if (!n) return 'Name is required.';
    if (n.length > 80) return 'Name is too long (max 80 chars).';
    return null;
  })();
  $: authorError = (() => {
    const n = (form.author || '').trim();
    if (!n) return 'Author is required.';
    if (n.length > 80) return 'Author is too long (max 80 chars).';
    return null;
  })();
  $: descriptionError = (() => {
    if ((form.description || '').length > 2000) return 'Description is too long (max 2000 chars).';
    return null;
  })();
  $: iconError = (() => {
    const v = (form.icon || '').trim();
    if (!v) return null;
    if (!/^https:\/\//i.test(v)) return 'Icon URL must start with https://';
    if (v.length > 512) return 'Icon URL too long (max 512 chars).';
    return null;
  })();
  $: keywordCount = form.keywords.split(',').map((k) => k.trim()).filter(Boolean).length;
  $: keywordsError = keywordCount > 20 ? 'Too many keywords (max 20).' : null;

  $: anyError = !!(nameError || authorError || descriptionError || iconError || keywordsError);
  $: canSubmit = !saving && loaded && hasChanges && !anyError;

  async function save() {
    if (!canSubmit || !target) return;
    saving = true;
    try {
      const result = await QuickEditAddon(target.addon_slug || target.folder_name, changes);
      const ndone = (result?.fields || []).length;
      showNotification(`Saved (${ndone} field${ndone === 1 ? '' : 's'} updated)`, 'success', 4000);
      // Refresh BEFORE dispatching 'saved' so the parent's reload sees the
      // updated cache. Otherwise it races and renders stale data.
      try { await RefreshAddons(); } catch {}
      dispatch('saved');
      close();
    } catch (e) {
      showNotification(`Save failed: ${e}`, 'error', 12000);
    }
    saving = false;
  }

  function toggleDep(id) {
    const next = new Set(form.dependencies);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    form.dependencies = next;
  }

  $: ownSlug = (target?.addon_slug || target?.folder_name || '').toLowerCase();
  $: depCandidates = availableAddons.filter((a) => {
    if ((a.id || '').toLowerCase() === ownSlug) return false;
    if (!depSearch.trim()) return true;
    const q = depSearch.trim().toLowerCase();
    return (
      (a.name || '').toLowerCase().includes(q) ||
      (a.id || '').toLowerCase().includes(q) ||
      (a.author_name || '').toLowerCase().includes(q)
    );
  });
</script>

{#if target}
  <div
    class="fixed inset-0 z-[70] flex items-center justify-center bg-black/75 backdrop-blur-md p-4"
    on:click={close}
    on:keydown={(e) => e.key === 'Escape' && close()}
    role="presentation"
    transition:modalBackdrop
  >
    <div
      class="bg-bg-secondary border border-border rounded-2xl shadow-modal w-full max-w-xl max-h-[90vh] overflow-y-auto"
      on:click|stopPropagation
      role="dialog"
      aria-modal="true"
      transition:modalContent
    >
      <!-- Header -->
      <div class="px-6 py-5 border-b border-border bg-header-grad flex items-start justify-between gap-3">
        <div class="min-w-0">
          <h3 class="text-lg font-bold text-text-primary tracking-tight">Quick edit</h3>
          <p class="text-xs text-text-muted mt-1">
            <span class="font-mono text-text-secondary">{target.addon_name || target.addon_slug}</span>
            — changes commit straight to the registry. No PR, no review.
          </p>
        </div>
        <button
          on:click={close}
          class="p-1.5 -mt-1 -mr-1 rounded-md text-text-muted hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          title="Close"
          aria-label="Close"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
      </div>

      <!-- Body -->
      <div class="p-6">
        {#if loading || !loaded}
          <div class="flex items-center justify-center py-10 text-text-muted text-sm gap-2">
            <Spinner /> Loading current values…
          </div>
        {:else}
          <div class="space-y-4">

            <!-- README explainer -->
            <div class="rounded-lg border border-accent/40 bg-accent/10 px-3 py-2.5 text-xs text-text-secondary leading-relaxed">
              <strong class="text-text-primary">Tip:</strong> Code and version updates need <em>New version</em>.
              READMEs are read from your source repo's branch HEAD, so pushing a README update there
              shows up here automatically.
            </div>

            <!-- Name -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Name *</label>
              <input
                type="text"
                bind:value={form.name}
                maxlength="80"
                class="w-full px-3 py-2 bg-bg-primary border {nameError ? 'border-warning' : 'border-border'} rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary"
              />
              {#if nameError}<p class="text-[11px] text-warning mt-1">{nameError}</p>{/if}
            </div>

            <!-- Author display name -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Author (display name) *</label>
              <input
                type="text"
                bind:value={form.author}
                maxlength="80"
                class="w-full px-3 py-2 bg-bg-primary border {authorError ? 'border-warning' : 'border-border'} rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary"
              />
              {#if authorError}<p class="text-[11px] text-warning mt-1">{authorError}</p>{/if}
              <p class="text-[10px] text-text-muted mt-1">
                Cosmetic only — your verified Discord and GitHub identities don't change.
              </p>
            </div>

            <!-- Category -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Category</label>
              <select
                bind:value={form.category}
                class="w-full px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary"
              >
                {#each categories as c}<option value={c}>{c}</option>{/each}
              </select>
            </div>

            <!-- Icon URL -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Icon URL (HTTPS)</label>
              <div class="flex gap-2 items-stretch">
                <input
                  type="text"
                  bind:value={form.icon}
                  placeholder="https://…"
                  maxlength="512"
                  class="flex-1 px-3 py-2 bg-bg-primary border {iconError ? 'border-warning' : 'border-border'} rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary"
                />
                {#if form.icon.trim() && !iconError}
                  <div class="w-10 h-10 rounded-lg border border-border bg-bg-primary flex items-center justify-center overflow-hidden flex-shrink-0">
                    <img src={form.icon.trim()} alt="" class="w-full h-full object-cover" referrerpolicy="no-referrer" />
                  </div>
                {/if}
              </div>
              {#if iconError}<p class="text-[11px] text-warning mt-1">{iconError}</p>{/if}
              <p class="text-[10px] text-text-muted mt-1">Leave blank to clear.</p>
            </div>

            <!-- Description -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Description</label>
              <textarea
                bind:value={form.description}
                rows="4"
                maxlength="2000"
                class="w-full px-3 py-2 bg-bg-primary border {descriptionError ? 'border-warning' : 'border-border'} rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary leading-relaxed"
              ></textarea>
              <p class="text-[10px] text-text-muted text-right mt-1">{(form.description || '').length} / 2000</p>
              {#if descriptionError}<p class="text-[11px] text-warning mt-1">{descriptionError}</p>{/if}
              <p class="text-[10px] text-text-muted mt-1">
                If you ship a README in your source repo it overrides this for the in-app details view.
              </p>
            </div>

            <!-- Keywords -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Keywords (comma-separated)</label>
              <input
                type="text"
                bind:value={form.keywords}
                placeholder="dps, combat, meter"
                class="w-full px-3 py-2 bg-bg-primary border {keywordsError ? 'border-warning' : 'border-border'} rounded-lg text-sm focus:outline-none focus:border-accent text-text-primary"
              />
              <p class="text-[10px] text-text-muted mt-1">{keywordCount} / 20 keywords</p>
              {#if keywordsError}<p class="text-[11px] text-warning mt-1">{keywordsError}</p>{/if}
            </div>

            <!-- Dependencies -->
            <div>
              <label class="block text-xs text-text-secondary mb-1.5">Dependencies</label>
              <button
                type="button"
                on:click={() => (depsOpen = !depsOpen)}
                class="w-full flex items-center justify-between px-3 py-2 bg-bg-primary border border-border rounded-lg text-sm hover:border-text-muted transition-colors"
              >
                <span class="text-text-primary">
                  {#if form.dependencies.size === 0}
                    <span class="text-text-muted">None</span>
                  {:else}
                    {form.dependencies.size} addon{form.dependencies.size === 1 ? '' : 's'} required
                  {/if}
                </span>
                <svg class="w-4 h-4 text-text-muted transition-transform {depsOpen ? 'rotate-180' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
                  </div>
                  <div class="max-h-48 overflow-y-auto">
                    {#if depCandidates.length === 0}
                      <div class="text-xs text-text-muted px-3 py-4 text-center">No addons match.</div>
                    {:else}
                      {#each depCandidates as a}
                        {@const checked = form.dependencies.has(a.id)}
                        <button
                          type="button"
                          on:click={() => toggleDep(a.id)}
                          class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-bg-tertiary text-sm text-left transition-colors"
                        >
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
                            <span class="text-xs text-text-muted ml-2">v{a.version}{a.author_name ? ` · ${a.author_name}` : ''}</span>
                          </span>
                          <span class="text-[10px] text-text-muted font-mono flex-shrink-0">{a.id}</span>
                        </button>
                      {/each}
                    {/if}
                  </div>
                </div>
              {/if}
            </div>

          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-border bg-bg-primary/40 flex items-center justify-between gap-2">
        <div class="text-[11px] text-text-muted">
          {#if !loaded}
            &nbsp;
          {:else if !hasChanges}
            No changes yet.
          {:else}
            {Object.keys(changes).filter((k) => !k.startsWith('_')).length} field{Object.keys(changes).filter((k) => !k.startsWith('_')).length === 1 ? '' : 's'} ready to save
          {/if}
        </div>
        <div class="flex items-center gap-2">
          <button
            on:click={close}
            class="px-4 py-2 bg-bg-tertiary hover:bg-bg-elevated border border-border rounded-lg text-sm text-text-secondary hover:text-text-primary transition-colors"
          >
            Cancel
          </button>
          <button
            on:click={save}
            disabled={!canSubmit}
            class="px-5 py-2 bg-accent hover:bg-accent-hover text-white rounded-lg text-sm font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {#if saving}<Spinner size="sm" />{/if}
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
