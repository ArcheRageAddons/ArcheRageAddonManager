<script>
  import { notification } from '../stores/app.js';
  import { toastSlide } from '../motion.js';

  $: bgClass = $notification?.type === 'success' ? 'bg-success' :
               $notification?.type === 'error' ? 'bg-red-500' :
               $notification?.type === 'warning' ? 'bg-warning' :
               'bg-bg-tertiary';

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
    class="fixed bottom-6 right-6 z-[100]"
    transition:toastSlide
  >
    <div class="{bgClass} text-white px-5 py-3 rounded-lg shadow-xl flex items-center gap-3 text-sm">
      {#if $notification.type === 'success'}
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
      <span>{$notification.message}</span>
      {#if $notification.action}
        <button
          on:click={handleAction}
          class="ml-2 px-2.5 py-1 bg-white/20 hover:bg-white/30 rounded text-xs font-medium transition-colors"
        >
          {$notification.action.label}
        </button>
      {/if}
    </div>
  </div>
{/if}

