/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{svelte,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'bg-primary': '#121212',
        'bg-secondary': '#1a1a1a',
        'bg-tertiary': '#252525',
        'bg-sidebar': '#0d0d0d',
        'accent': '#4a9d7c',
        'accent-hover': '#5bb890',
        'text-primary': '#e0e0e0',
        'text-secondary': '#808080',
        'text-muted': '#5a5a5a',
        'warning': '#e6a23c',
        'success': '#67c23a',
        'border': '#2a2a2a',
        'tag-bg': '#2a2a2a',
      },
    },
  },
  plugins: [],
}
