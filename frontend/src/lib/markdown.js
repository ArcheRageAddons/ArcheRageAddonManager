import { Marked } from 'marked';

// Renders addon README markdown to safe HTML.
// - Raw HTML stripped (defence against <img onerror>, <script>, etc.)
// - Relative image URLs resolved against the addon's pinned-commit base URL
// - External links tagged so AddonDetailsModal can intercept clicks and
//   route through Go's OpenReadmeLink rather than navigating the webview.
export function renderReadme(markdown, baseURL) {
  if (!markdown) return '';

  const marked = new Marked({
    gfm: true,
    breaks: false,
    pedantic: false,
  });

  const renderer = {
    image({ href, title, text }) {
      const resolved = resolveURL(href, baseURL);
      if (!resolved) return escapeHTML(text || '');
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : '';
      return `<img src="${escapeAttr(resolved)}" alt="${escapeAttr(text || '')}"${titleAttr} loading="lazy" referrerpolicy="no-referrer" />`;
    },
    link({ href, title, tokens }) {
      const text = this.parser.parseInline(tokens);
      const resolved = resolveURL(href, baseURL);
      if (!resolved) return text;
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : '';
      return `<a href="${escapeAttr(resolved)}" data-readme-link="1"${titleAttr}>${text}</a>`;
    },
    html() {
      // Strip any raw HTML the author tried to embed.
      return '';
    },
  };

  marked.use({ renderer });
  return marked.parse(markdown);
}

// Resolve a possibly-relative URL against the README's base URL.
// Returns null for refused schemes (javascript:, data:, file:, etc.).
function resolveURL(href, baseURL) {
  if (!href) return null;
  const trimmed = href.trim();
  if (!trimmed) return null;

  // Absolute https — accept as-is.
  if (/^https:\/\//i.test(trimmed)) return trimmed;
  // Absolute http — upgrade-refuse (CSP would block img-src http: anyway).
  if (/^https?:\/\//i.test(trimmed)) return null;
  // Mailto links — accept.
  if (/^mailto:/i.test(trimmed)) return trimmed;
  // Anchor links — accept (resolves within the rendered README).
  if (trimmed.startsWith('#')) return trimmed;
  // Anything else with a scheme is suspect (javascript:, data:, file:, etc.).
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return null;

  // Relative path — resolve against baseURL.
  if (!baseURL) return null;
  try {
    return new URL(trimmed, baseURL).href;
  } catch {
    return null;
  }
}

function escapeHTML(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function escapeAttr(s) {
  return escapeHTML(s);
}
