/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{svelte,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'bg-primary':    '#0e0f12',
        'bg-secondary':  '#16181d',
        'bg-tertiary':   '#1f2228',
        'bg-sidebar':    '#0a0b0d',
        'bg-elevated':   '#252830',
        'accent':        '#54b787',
        'accent-hover':  '#6dc99c',
        'text-primary':  '#e6e7eb',
        'text-secondary':'#9095a0',
        'text-muted':    '#5b606b',
        'warning':       '#e6a23c',
        'success':       '#54b787',
        'border':        '#23252b',
        'border-strong': '#2f3239',
        'tag-bg':        '#23252b',
      },
      boxShadow: {
        'soft':     '0 1px 2px 0 rgba(0,0,0,0.3), 0 1px 6px -1px rgba(0,0,0,0.2)',
        'lift':     '0 4px 12px -2px rgba(0,0,0,0.4), 0 2px 4px -2px rgba(0,0,0,0.3)',
        'modal':    '0 24px 64px -8px rgba(0,0,0,0.6), 0 8px 24px -4px rgba(0,0,0,0.4)',
        'glow':     '0 0 24px -4px rgba(84, 183, 135, 0.25)',
      },
      backgroundImage: {
        'sidebar-grad': 'linear-gradient(180deg, #0c0d10 0%, #0a0b0d 100%)',
        'card-grad':    'linear-gradient(160deg, #1a1c22 0%, #16181d 100%)',
        'header-grad':  'linear-gradient(180deg, #1a1c22 0%, #16181d 100%)',
        'accent-grad':  'linear-gradient(135deg, #54b787 0%, #3e9a6e 100%)',
      },
    },
  },
  plugins: [],
}
