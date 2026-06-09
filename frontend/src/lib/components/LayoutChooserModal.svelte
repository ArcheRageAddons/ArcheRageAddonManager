<script>
  import { layoutMode, showLayoutChooser, dismissLayoutChooser, showWelcomeModal } from '../stores/app.js';
  import { modalBackdrop, modalContent } from '../motion.js';

  // Local preview selection — doesn't commit until the user clicks Continue.
  let preview = $layoutMode || 'studio';

  function pick(mode) { preview = mode; }
  function confirm() {
    layoutMode.set(preview);
    dismissLayoutChooser();
  }

  // Don't show while the welcome modal is up — that flow is for first-run
  // setup of the addon folder, which happens first.
  $: visible = $showLayoutChooser && !$showWelcomeModal;
</script>

{#if visible}
  <div
    class="fixed inset-0 z-[80] flex items-center justify-center bg-black/75 backdrop-blur-md p-4"
    transition:modalBackdrop
    role="dialog"
    aria-modal="true"
  >
    <div
      class="bg-bg-secondary border border-border rounded-2xl shadow-modal w-full max-w-3xl max-h-[90vh] overflow-y-auto"
      transition:modalContent
    >
      <!-- Header -->
      <div class="px-7 py-6 border-b border-border bg-header-grad text-center">
        <div class="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-accent/15 border border-accent/40 mb-3">
          <svg class="w-7 h-7 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="3" width="7" height="7" rx="1"/>
            <rect x="14" y="3" width="7" height="7" rx="1"/>
            <rect x="3" y="14" width="7" height="7" rx="1"/>
            <rect x="14" y="14" width="7" height="7" rx="1"/>
          </svg>
        </div>
        <h2 class="text-2xl font-bold text-text-primary tracking-tight">Pick a layout to get started</h2>
        <p class="text-sm text-text-muted mt-1.5">
          The manager comes with two ways of arranging things. Try one — you can switch anytime in Settings.
        </p>
      </div>

      <!-- Body: two preview cards -->
      <div class="p-6 grid grid-cols-1 sm:grid-cols-2 gap-4">

        <!-- ============ Studio option ============ -->
        <button
          type="button"
          on:click={() => pick('studio')}
          class="text-left rounded-xl border-2 transition-all p-4 flex flex-col {preview === 'studio' ? 'border-accent bg-accent/10 shadow-glow' : 'border-border bg-bg-primary/40 hover:border-border-strong'}"
        >
          <!-- Big preview -->
          <div class="aspect-[16/10] rounded-lg overflow-hidden border border-border bg-bg-primary flex mb-4">
            <div class="w-[14%] bg-bg-sidebar flex flex-col items-center gap-1.5 py-2">
              <div class="w-4 h-4 rounded bg-accent/80"></div>
              <div class="w-px h-1 bg-border my-0.5"></div>
              <div class="w-4 h-4 rounded bg-accent"></div>
              <div class="w-4 h-4 rounded bg-text-muted/30"></div>
              <div class="w-4 h-4 rounded bg-text-muted/30"></div>
              <div class="w-4 h-4 rounded bg-text-muted/30"></div>
            </div>
            <div class="w-[36%] border-r border-l border-border bg-bg-primary flex flex-col gap-1.5 p-2">
              <div class="h-2 w-14 bg-text-muted/30 rounded"></div>
              <div class="h-4 w-full bg-bg-secondary border border-border rounded"></div>
              <div class="h-3 w-full bg-accent/25 rounded"></div>
              <div class="h-3 w-full bg-text-muted/15 rounded"></div>
              <div class="h-3 w-full bg-text-muted/15 rounded"></div>
              <div class="h-3 w-full bg-text-muted/15 rounded"></div>
              <div class="h-3 w-full bg-text-muted/15 rounded"></div>
            </div>
            <div class="flex-1 bg-bg-secondary flex flex-col gap-2 p-3">
              <div class="flex items-center gap-2">
                <div class="w-6 h-6 rounded-md bg-accent/40"></div>
                <div class="flex-1 space-y-1">
                  <div class="h-2 w-2/3 bg-accent/40 rounded"></div>
                  <div class="h-1.5 w-1/3 bg-text-muted/30 rounded"></div>
                </div>
              </div>
              <div class="h-1 w-full bg-text-muted/15 rounded"></div>
              <div class="h-1 w-5/6 bg-text-muted/15 rounded"></div>
              <div class="h-1 w-3/4 bg-text-muted/15 rounded"></div>
              <div class="mt-auto h-5 w-20 bg-accent/40 rounded"></div>
            </div>
          </div>

          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-lg font-bold text-text-primary tracking-tight">Studio</h3>
            {#if preview === 'studio'}
              <span class="px-2 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-accent text-white rounded-full">Selected</span>
            {/if}
          </div>
          <p class="text-xs text-text-secondary leading-relaxed mb-3">
            Three-column split: nav icons, list of addons, and details all visible at once.
          </p>
          <ul class="space-y-1.5 text-[12px] text-text-secondary mb-1">
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Click any addon and see its details instantly — no modal pop-up</span>
            </li>
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Drag the divider to size the list however you like</span>
            </li>
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Compact icon-only sidebar gives more room for content</span>
            </li>
          </ul>
        </button>

        <!-- ============ Classic option ============ -->
        <button
          type="button"
          on:click={() => pick('classic')}
          class="text-left rounded-xl border-2 transition-all p-4 flex flex-col {preview === 'classic' ? 'border-accent bg-accent/10 shadow-glow' : 'border-border bg-bg-primary/40 hover:border-border-strong'}"
        >
          <!-- Big preview -->
          <div class="aspect-[16/10] rounded-lg overflow-hidden border border-border bg-bg-primary flex mb-4">
            <div class="w-[26%] bg-bg-sidebar flex flex-col gap-1.5 p-2">
              <div class="flex items-center gap-1 mb-1">
                <div class="w-3 h-3 rounded bg-accent"></div>
                <div class="h-2 w-12 bg-text-primary/40 rounded"></div>
              </div>
              <div class="h-2.5 w-full bg-accent/40 rounded"></div>
              <div class="h-2.5 w-full bg-text-muted/20 rounded"></div>
              <div class="h-2.5 w-full bg-text-muted/15 rounded"></div>
              <div class="h-2.5 w-full bg-text-muted/15 rounded"></div>
            </div>
            <div class="flex-1 bg-bg-secondary flex flex-col gap-2 p-3">
              <div class="h-3 w-1/3 bg-accent/40 rounded"></div>
              <div class="h-1.5 w-2/5 bg-text-muted/30 rounded"></div>
              <div class="grid grid-cols-3 gap-1.5 mt-1">
                {#each Array(6) as _, i}
                  <div class="aspect-square bg-bg-primary/70 border border-border rounded flex flex-col items-center justify-center gap-0.5 p-1">
                    <div class="w-3 h-3 rounded bg-{i === 0 ? 'accent/40' : 'text-muted/25'}"></div>
                    <div class="h-0.5 w-3/4 bg-text-muted/25 rounded"></div>
                  </div>
                {/each}
              </div>
            </div>
          </div>

          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-lg font-bold text-text-primary tracking-tight">Classic</h3>
            {#if preview === 'classic'}
              <span class="px-2 py-0.5 text-[9px] font-bold uppercase tracking-wider bg-accent text-white rounded-full">Selected</span>
            {/if}
          </div>
          <p class="text-xs text-text-secondary leading-relaxed mb-3">
            Familiar sidebar nav with the addon catalogue spread across the full window.
          </p>
          <ul class="space-y-1.5 text-[12px] text-text-secondary mb-1">
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Labelled sidebar so every page is one click away</span>
            </li>
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Bigger addon cards in a grid — easier to scan visually</span>
            </li>
            <li class="flex items-start gap-2">
              <svg class="w-3.5 h-3.5 text-accent flex-shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
              <span>Details open in a focused modal so you read one addon at a time</span>
            </li>
          </ul>
        </button>
      </div>

      <!-- Footer -->
      <div class="px-7 py-4 border-t border-border bg-bg-primary/40 flex items-center justify-between gap-3">
        <p class="text-[11px] text-text-muted">
          You can switch later in <strong class="text-text-secondary">Settings → Downloads → Layout</strong>.
        </p>
        <button
          on:click={confirm}
          class="px-6 py-2.5 bg-accent-grad hover:brightness-110 text-white rounded-xl text-sm font-semibold shadow-lift transition-all"
        >
          Use {preview === 'studio' ? 'Studio' : 'Classic'}
        </button>
      </div>
    </div>
  </div>
{/if}
