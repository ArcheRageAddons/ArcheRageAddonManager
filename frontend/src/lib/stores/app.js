import { writable, get } from 'svelte/store';
import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
import { DownloadAddon, GetInstalledAddons } from '../../../wailsjs/go/main/App.js';

export const currentPage = writable('browse');
export const notification = writable(null);
export const selectedAddon = writable(null);
export const showAddonDetails = writable(false);
export const showWarningModal = writable(false);
export const warningAddon = writable(null);
export const showUninstallConfirm = writable(false);
export const uninstallAddon = writable(null);
export const appInitialized = writable(false);
export const showWelcomeModal = writable(false);
export const currentUser = writable(null);
export const showSubmitModal = writable(false);
export const showAuthorModal = writable(false);
export const selectedAuthor = writable('');
export const submitPrefill = writable(null);

// UI shell preference: 'studio' (icon rail + split list/detail panes) or
// 'classic' (sidebar + full-width content with modal details). Persisted
// to localStorage so it survives restarts.
const LAYOUT_KEY = 'archerage-layout';
function initialLayout() {
  try {
    const raw = localStorage.getItem(LAYOUT_KEY);
    if (raw === 'classic' || raw === 'studio') return raw;
  } catch {}
  return 'studio';
}
export const layoutMode = writable(initialLayout());
layoutMode.subscribe((v) => {
  try { localStorage.setItem(LAYOUT_KEY, v); } catch {}
});

// One-time first-run prompt that asks the user which layout they prefer.
// The "shown" flag lives in config.json on the Go side (see
// GetLayoutChooserShown / MarkLayoutChooserShown) — localStorage proved
// flaky between rebuilds because WebView2's user-data folder can drift.
// The store starts false and gets flipped to true by App.svelte on launch
// if the Go side reports the picker hasn't been dismissed yet.
export const showLayoutChooser = writable(false);
import { MarkLayoutChooserShown } from '../../../wailsjs/go/main/App.js';
export async function dismissLayoutChooser() {
  try { await MarkLayoutChooserShown(); } catch (e) { console.warn('dismiss layout chooser:', e); }
  showLayoutChooser.set(false);
}

export const downloadProgress = writable({
    isDownloading: false,
    addonId: null,
    addonName: null,
    current: 0,
    total: 100,
    message: ''
});

// action: optional `{ label, handler }` for a toast button. Action toasts
// stick around longer so the user has time to click.
export function showNotification(message, type = 'info', duration = 3000, action = null) {
    notification.set({ message, type, action });
    const dismissAfter = action ? Math.max(duration, 8000) : duration;
    setTimeout(() => {
        notification.set(null);
    }, dismissAfter);
}

// Drives the bell badge — installed rows with has_update === true.
export const availableUpdates = writable([]);

export async function refreshAvailableUpdates() {
    try {
        const all = (await GetInstalledAddons()) || [];
        availableUpdates.set(all.filter((a) => a.has_update));
    } catch (e) {
        console.warn('refreshAvailableUpdates failed:', e);
    }
}

// Single chokepoint for starting an install. Resolves with the
// download:complete payload so callers can chain (see installSerially).
export function kickOffInstall(addon, { addonNameOverride = null } = {}) {
    downloadProgress.set({
        isDownloading: true,
        addonId: addon.id,
        addonName: addonNameOverride ?? addon.name,
        current: 0,
        total: 100,
        message: 'Starting download...',
    });
    DownloadAddon(addon.id);
    return waitForDownloadComplete(addon.id);
}

// Runs installs back-to-back, returns { ok, failed, errors[] }. Caller
// decides what to surface — this never emits notifications itself.
export async function installSerially(addons, { onProgress = null, label = null } = {}) {
    const results = { ok: 0, failed: 0, errors: [] };
    if (!addons || addons.length === 0) return results;

    for (let i = 0; i < addons.length; i++) {
        const addon = addons[i];
        const total = addons.length;
        const overrideName = total > 1
            ? (label ? `${addon.name} (${label(i + 1, total)})` : `${addon.name} (${i + 1}/${total})`)
            : null;

        let success = false;
        try {
            const result = await kickOffInstall(addon, { addonNameOverride: overrideName });
            success = !!result?.success;
            if (success) {
                results.ok++;
            } else {
                results.failed++;
                if (result?.error) results.errors.push(result.error);
            }
        } catch (e) {
            results.failed++;
            results.errors.push(String(e));
        }

        // Wait for DownloadProgress's ~500ms post-complete reset to land
        // before the next iteration sets isDownloading=true (else the
        // delayed reset clobbers the new state). 2s cap as a safety net.
        const start = Date.now();
        while (get(downloadProgress).isDownloading && Date.now() - start < 2000) {
            await new Promise((r) => setTimeout(r, 50));
        }

        if (onProgress) {
            try { onProgress({ done: i + 1, total, addon, success }); }
            catch (e) { console.warn('installSerially onProgress failed:', e); }
        }
    }

    return results;
}

// Use the per-listener off() returned by EventsOn — never EventsOff('download:complete'),
// which would also unhook DownloadProgress.svelte's long-lived listener.
export function waitForDownloadComplete(addonId) {
    return new Promise((resolve) => {
        const off = EventsOn('download:complete', (result) => {
            if (result && result.addon_id === addonId) {
                off();
                resolve(result);
            }
        });
    });
}
