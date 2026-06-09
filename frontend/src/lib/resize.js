// Svelte action: makes the bound element a horizontal resize handle.
// Calls onResize(newWidth) as the user drags. Clamped to [min, max].
// The action only reports — the caller decides what to do with the width
// (typically: bind to a CSS var or inline style on a sibling pane).
export function resizable(node, options) {
  let { onResize, getCurrent, min = 200, max = 800 } = options;
  let startX = 0;
  let startWidth = 0;

  function onDown(e) {
    if (e.button !== 0) return;
    e.preventDefault();
    startX = e.clientX;
    startWidth = getCurrent();
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    node.classList.add('is-dragging');
  }

  function onMove(e) {
    const delta = e.clientX - startX;
    const next = Math.max(min, Math.min(max, startWidth + delta));
    onResize(next);
  }

  function onUp() {
    document.removeEventListener('mousemove', onMove);
    document.removeEventListener('mouseup', onUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    node.classList.remove('is-dragging');
  }

  function onDouble() {
    // Double-click resets to default
    if (typeof options.defaultWidth === 'number') onResize(options.defaultWidth);
  }

  node.addEventListener('mousedown', onDown);
  node.addEventListener('dblclick', onDouble);

  return {
    update(next) {
      options = next;
      getCurrent = next.getCurrent;
      onResize = next.onResize;
      min = next.min ?? min;
      max = next.max ?? max;
    },
    destroy() {
      node.removeEventListener('mousedown', onDown);
      node.removeEventListener('dblclick', onDouble);
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    },
  };
}

// Small wrapper: persists the width to localStorage under the given key.
// Returns initial value (clamped to [min, max]) and a setter that both
// updates state and saves.
export function persistedWidth(key, defaultValue, min, max) {
  let initial = defaultValue;
  try {
    const raw = localStorage.getItem(key);
    if (raw != null) {
      const n = parseInt(raw, 10);
      if (!isNaN(n)) initial = n;
    }
  } catch {}
  initial = Math.max(min, Math.min(max, initial));
  function save(value) {
    try { localStorage.setItem(key, String(value)); } catch {}
  }
  return { initial, save };
}
