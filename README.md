# ArcheRage Addon Manager

A lightweight, portable Windows desktop app for browsing, installing, and managing ArcheRage UI / quality-of-life addons. Addons are sourced from a community-maintained GitHub registry — no backend account is required to browse and install.

Download the latest `ArcheRageAddonManager.exe` from the [Releases](https://github.com/ArcheRageAddons/ArcheRageAddonManager/releases/latest) page, drop it anywhere outside OneDrive, and run it.

---

## Submitting an addon

Submitting is fully in-app. You don't write or open YAML by hand and you don't open the PR yourself — the manager does both for you.

1. **Sign in with Discord** (Account panel, top-right of the manager).
2. **Connect GitHub.** When prompted in the submission flow, the manager opens GitHub's device-flow login in your browser. The OAuth token has zero scopes — it's only used to verify your identity and confirm you have push access to the source repo you're claiming.
3. **Click My Addons → Submit new addon.** Fill out the 3-step form:
    - **Step 1 — basics.** Name, addon ID (becomes the registry filename), category, description, keywords, optional icon URL, optional dependencies.
    - **Step 2 — source.** Pick the GitHub repo your addon lives in (auto-listed from your account), the branch, and the optional subfolder path inside it.
    - **Step 3 — review.** Confirm the YAML preview and submit.
4. **The manager opens a PR** against the public registry repo (`ArcheRageAddons/addons`) under your verified GitHub identity. The PR includes:
    - Your generated YAML, pinned to the exact commit you're shipping (so users always download exactly the bytes that were reviewed).
    - A trust strip listing your verified Discord + GitHub usernames.
    - An automated dangerous-file scan result (flags any `.exe` / `.dll` / `.bat` / `.lnk` / similar).
5. **An admin reviews the PR.** Once merged, your addon appears in the manager's Browse list within a minute or two.

You'll see the live status of every submission under **My Addons → Submission history**. Pending submissions can be **withdrawn** (closes the PR without merging).

### Updating a published addon

In **My Addons**, click **Update** on the addon. The form pre-fills from the current YAML — bump the version, point at the new commit, submit. Same review flow.

### What the registry YAML looks like

Behind the scenes, each addon is a single YAML file. Most fields are populated automatically by the form, but if you ever need to inspect or edit one directly:

```yaml
name: "Auto Role Setter"
folder_name: "autorolesetter"
author: "Koala"
version: "1.0.0"
description: "Automatically sets your role on combat..."
category: "Quality of Life"
keywords: [role, automation]
github:
  repo: "Koalazau/PersonalAddons"
  branch: "main"
  commit: "<full-sha>"          # immutable pin set at submission time
  path: "autorolesetter"        # optional subfolder
dependencies: [other-addon-id]   # optional
icon: "https://..."              # optional, any HTTPS URL
```

The filename without `.yaml` becomes the addon's stable ID — that's also what other addons use in their `dependencies:` list.

---

*The manager itself is independent from the ArcheRage server team. ArcheRage is a registered trademark of its owners.*
