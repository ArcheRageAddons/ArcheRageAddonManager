import { fade, scale, fly } from 'svelte/transition';
import { cubicOut } from 'svelte/easing';

// Single source of truth for animation timings. Tune here and every
// modal / page / toast / dropdown changes together.
const reduced =
  typeof window !== 'undefined' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches;

const noop = () => ({ duration: 0 });

export const modalBackdrop = reduced
  ? noop
  : (node, params = {}) =>
      fade(node, { duration: 150, easing: cubicOut, ...params });

export const modalContent = reduced
  ? noop
  : (node, params = {}) =>
      scale(node, { duration: 180, start: 0.96, opacity: 0, easing: cubicOut, ...params });

export const pageFade = reduced
  ? noop
  : (node, params = {}) =>
      fade(node, { duration: 120, easing: cubicOut, ...params });

export const toastSlide = reduced
  ? noop
  : (node, params = {}) =>
      fly(node, { duration: 200, x: 32, opacity: 0, easing: cubicOut, ...params });

export const dropdown = reduced
  ? noop
  : (node, params = {}) =>
      scale(node, { duration: 150, start: 0.96, opacity: 0, easing: cubicOut, ...params });

export const bannerSlide = reduced
  ? noop
  : (node, params = {}) =>
      fly(node, { duration: 200, y: -16, opacity: 0, easing: cubicOut, ...params });
