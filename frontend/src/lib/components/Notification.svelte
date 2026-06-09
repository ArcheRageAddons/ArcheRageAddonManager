<script>
  import { notification } from '../stores/app.js';
  import { toastSlide } from '../motion.js';

  $: meta = ({
    success: { wrap: 'border-accent/40 bg-accent/10', icon: 'text-accent', accent: 'bg-accent' },
    error:   { wrap: 'border-red-500/40 bg-red-500/10', icon: 'text-red-400', accent: 'bg-red-500' },
    warning: { wrap: 'border-warning/40 bg-warning/10', icon: 'text-warning', accent: 'bg-warning' },
    info:    { wrap: 'border-border-strong bg-bg-elevated', icon: 'text-text-secondary', accent: 'bg-text-muted' },
  })[$notification?.type || 'info'];

  async function handleAction() {
    const action = $notification?.action;
    if (!action || typeof action.handler !== 'function') return;
    try {
      await action.handler();
    } catch (e) {
      console.error('notification action failed:', e);
    }
    notification.set(null);
  }
</script>

{#if $notification}
  <div
    class="fixed bottom-6 right-6 z-[100] max-w-md"
    transition:toastSlide
  >
    <div class="relative bg-bg-secondary border {meta.wrap} text-text-primary pl-4 pr-5 py-3.5 rounded-xl shadow-modal flex items-start gap-3 text-sm overflow-hidden">
      <span class="absolute top-0 left-0 bottom-0 w-1 {meta.accent}"></span>
      <div class="{meta.icon} flex-shrink-0 mt-0.5">
        {#if $notification.type === 'success'}
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M20 6L9 17l-5-5"/>
          </svg>
        {:else if $notification.type === 'error'}
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <path d="M15 9l-6 6M9 9l6 6"/>
          </svg>
        {:else if $notification.type === 'warning'}
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
            <path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/>
          </svg>
        {:else}
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <path d="M12 16v-4M12 8h.01"/>
          </svg>
        {/if}
      </div>
      <span class="flex-1 leading-relaxed">{$notification.message}</span>
      {#if $notification.action}
        <button
          on:click={handleAction}
          class="ml-1 px-2.5 py-1 bg-bg-tertiary hover:bg-bg-elevated border border-border rounded-md text-xs font-medium transition-colors flex-shrink-0"
        >
          {$notification.action.label}
        </button>
      {/if}
    </div>
  </div>
{/if}
