# ArcheRage Addon Manager

A lightweight, portable Windows desktop app for browsing, installing, and managing ArcheRage UI / quality-of-life addons. Addons are sourced from a community-maintained GitHub registry — no account is required to browse and install.

Download the latest `ArcheRageAddonManager.exe` from the [Releases](https://github.com/ArcheRageAddons/ArcheRageAddonManager/releases/latest) page, drop it anywhere outside OneDrive, and run it.

---

## Submitting an addon

Submitting is fully in-app giving you a very user friendly way to submit your addon's

Steps to Submit.

1. **Sign in with Discord**
2. **Connect GitHub.** When prompted in the submit addon window at the top, the manager opens the GitHub Auth login page. The OAuth token has zero scopes — it's only used to verify your identity and confirm you have push access to the source repo you're claiming.
3. **Click My Addons → Submit new addon.** Fill out the 3-step form:
    - **Step 1 — source.** Pick the GitHub repo your addon lives in (auto-listed from your account), the branch, and the optional subfolder path inside it.
    - **Step 2 — basics.** Name, addon ID (becomes the registry filename usually either the repo itself or the subfolder if you've entered one), category, description, keywords, optional icon URL (any url works), optional                  dependencies (must be an existing addon).
    - **Step 3 — review.** Confirm everything is correct and then Submit.
4. **The manager opens a PR** against the public registry repo (`ArcheRageAddons/addons`) under your verified GitHub identity. The PR includes:
    - Your generated YAML, pinned to the exact commit you're shipping (so users always download exactly the bytes that were reviewed).
    - A trust strip listing your verified Discord + GitHub usernames.
    - An automated dangerous-file scan result (flags any `.exe` / `.dll` / `.bat` / `.lnk` / similar).
5. **An admin reviews the PR.** Once merged, your addon appears in the manager's Browse list within a minute or two.

You'll see the live status of every submission under **My Addons → Submission history**. Pending submissions can be **withdrawn** (closes the PR without merging).

### Updating a published addon

In **My Addons**, click **Update** on the addon. The form pre-fills from the current addon information — bump the version, point at the new commit, submit. Same review flow.
