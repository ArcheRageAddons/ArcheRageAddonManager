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
// Author modal — opened from any "by {author_name}" click. Holds just the
// author display string; the modal filters the cached addon list itself so
// no Go round-trip is needed when switching between authors.
export const showAuthorModal = writable(false);
export const selectedAuthor = writable('');
// When set, the SubmitAddonModal opens pre-filled from this object (e.g. for
// an "Update" flow). Modal clears it after consuming.
export const submitPrefill = writable(null);

// Global download progress
export const downloadProgress = writable({
    isDownloading: false,
    addonId: null,
    addonName: null,
    current: 0,
    total: 100,
    message: ''
});

// showNotification accepts an optional `action` (`{ label, handler }`) to
// render a clickable button on the toast — used for things like "Open
// backup folder" on a failed install. Toasts with an action stick around
// 2x as long as plain toasts so users have time to click.
export function showNotification(message, type = 'info', duration = 3000, action = null) {
    notification.set({ message, type, action });
    const dismissAfter = action ? Math.max(duration, 8000) : duration;
    setTimeout(() => {
        notification.set(null);
    }, dismissAfter);
}

// availableUpdates — drives the floating notification bell. Refreshed on
// mount and after every install completion (via the addon-installed window
// event DownloadProgress emits). Pages that manually refresh their data
// (Browse, Installed) call refreshAvailableUpdates() directly rather than
// firing a separate window event. Holds the InstalledAddon rows where
// has_update === true; bell badge shows the array length.
export const availableUpdates = writable([]);

export async function refreshAvailableUpdates() {
    try {
        const all = (await GetInstalledAddons()) || [];
        availableUpdates.set(all.filter((a) => a.has_update));
    } catch (e) {
        // Non-fatal — bell stays at its previous count.
        console.warn('refreshAvailableUpdates failed:', e);
    }
}

// kickOffInstall fires the global download-progress modal for `addon` and
// invokes the Go-side install. Single chokepoint for "start an install"
// so future changes (different progress shape, telemetry, etc.) land in
// one place. Returns a Promise that resolves with the download:complete
// payload — callers can fire-and-forget (don't await) or chain operations
// off the result (e.g. installSerially).
//
// `addonNameOverride` lets callers tweak the displayed name (e.g. for
// bulk loops that want to show "foo (2/5)" instead of just "foo").
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

// installSerially runs N installs back-to-back with the global progress
// modal driven for each one. Coordinates with DownloadProgress.svelte's
// own ~500 ms post-complete reset so the modal flashes cleanly between
// installs instead of clobbering the next iteration's state.
//
// onProgress: optional callback fired AFTER each install completes
//   ({ done, total, addon, success }). UI components use this to update
//   their own per-row spinner / "Updating 2/5…" header counter.
//
// Returns { ok, failed, errors[] }. Caller decides what to surface (toast,
// notification, etc.) — installSerially never emits notifications itself
// because some callers want a single summary toast and others want
// per-install toasts.
export async function installSerially(addons, { onProgress = null, label = null } = {}) {
    const results = { ok: 0, failed: 0, errors: [] };
    if (!addons || addons.length === 0) return results;

    for (let i = 0; i < addons.length; i++) {
        const addon = addons[i];
        const total = addons.length;
        const overrideName = total > 1
            ? (label ? `${addon.name} (${label(i + 1, total)})` : `${addon.name} (${i + 1}/${total})`)
            : null;

        // Track THIS iteration's success in a local. Earlier versions
        // computed `results.ok > i` here, which silently produced wrong
        // values once any iteration failed (the running ok-count never
        // caught back up to the iteration index). Per-iteration is the
        // only correct shape.
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

        // DownloadProgress's own download:complete handler calls
        // downloadProgress.set({ isDownloading: false, ... }) ~500 ms after
        // success. If the next iteration sets isDownloading=true before that
        // setTimeout fires, the timeout's later set() clobbers the new
        // state. Polling for isDownloading=false drains the cleanup before
        // we proceed; capped at 2 s in case the timer never fires.
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

// Wait for the next download:complete event matching addonId. Resolves
// with the Wails event payload ({ success, addon_id, error? }).
//
// IMPORTANT: uses the unsubscribe function returned by EventsOn — do NOT
// switch to EventsOff('download:complete') here, that's the global
// remove-all-listeners API and it would kill the long-lived listener in
// DownloadProgress.svelte that drives the floating progress widget. An
// earlier version of this helper had that bug (codebase audit B2).
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
