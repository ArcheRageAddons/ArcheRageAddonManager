import { writable } from 'svelte/store';
import { EventsOn } from '../../../wailsjs/runtime/runtime.js';

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
