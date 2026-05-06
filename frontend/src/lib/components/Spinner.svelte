<script>
  // size: 'sm' (12px) | 'md' (16px) | 'lg' (20px)
  export let size = 'md';
  const sizeClass = size === 'sm' ? 'w-3 h-3' : size === 'lg' ? 'w-5 h-5' : 'w-4 h-4';
</script>

<svg
  class="animate-spin {sizeClass} flex-shrink-0"
  viewBox="0 0 24 24"
  fill="none"
  aria-hidden="true"
>
  <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-opacity="0.25" stroke-width="3"/>
  <path d="M22 12 a10 10 0 0 1 -10 10" stroke="currentColor" stroke-width="3" stroke-linecap="round"/>
</svg>
