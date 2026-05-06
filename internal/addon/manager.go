package addon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"archerage-addon-manager/internal/config"
	"archerage-addon-manager/internal/github"
	"archerage-addon-manager/internal/registry"
)

type InstalledAddon struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Version          string    `json:"version"`
	InstalledAt      time.Time `json:"installed_at"`
	GithubCommitHash string    `json:"github_commit_hash"`
	FolderName       string    `json:"folder_name"`
	// OverlayOf, if set, is the addon ID of the base addon this entry was
	// installed on top of. Marks the row as not owning the folder — when
	// the base is uninstalled, this row is auto-cleaned (the folder is
	// removed by the base's uninstall regardless). Stays empty for normal
	// addons that own their folder outright.
	OverlayOf string `json:"overlay_of,omitempty"`
}

type InstalledAddonsData struct {
	Addons []InstalledAddon `json:"addons"`
}

type AddonManager struct {
	githubClient *github.GitHubClient
}

func NewAddonManager() *AddonManager {
	return &AddonManager{
		githubClient: github.NewGitHubClient(),
	}
}

// SetGithubClient allows setting the GitHub client (useful for passing authenticated client)
func (m *AddonManager) SetGithubClient(client *github.GitHubClient) {
	m.githubClient = client
}

func (m *AddonManager) getInstalledFilePath() string {
	return filepath.Join(config.GetConfigDir(), "installed.json")
}

func (m *AddonManager) LoadInstalledAddons() (*InstalledAddonsData, error) {
	filePath := m.getInstalledFilePath()

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &InstalledAddonsData{Addons: []InstalledAddon{}}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var installed InstalledAddonsData
	if err := json.Unmarshal(data, &installed); err != nil {
		return &InstalledAddonsData{Addons: []InstalledAddon{}}, nil
	}

	return &installed, nil
}

func (m *AddonManager) SaveInstalledAddons(data *InstalledAddonsData) error {
	filePath := m.getInstalledFilePath()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, jsonData, 0644)
}

func (m *AddonManager) IsInstalled(addonID string) bool {
	installed, err := m.LoadInstalledAddons()
	if err != nil {
		return false
	}

	for _, addon := range installed.Addons {
		if addon.ID == addonID {
			return true
		}
	}

	return false
}

func (m *AddonManager) GetInstalledAddon(addonID string) *InstalledAddon {
	installed, err := m.LoadInstalledAddons()
	if err != nil {
		return nil
	}

	for _, addon := range installed.Addons {
		if addon.ID == addonID {
			return &addon
		}
	}

	return nil
}

func (m *AddonManager) BackupAddon(addonName string) error {
	if err := registry.ValidateFolderName(addonName); err != nil {
		return fmt.Errorf("refusing to back up: %v", err)
	}
	cfg := config.Get()
	addonPath := filepath.Join(cfg.AddonPath, addonName)

	if _, err := os.Stat(addonPath); os.IsNotExist(err) {
		return nil
	}

	backupDir := filepath.Join(cfg.AddonPath, "Backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s_%s", addonName, timestamp))

	return os.Rename(addonPath, backupPath)
}

// ProgressCallback is called during installation to report progress
type ProgressCallback func(current, total int, message string)

// InstallAddon installs (or reinstalls / updates) an addon.
//
// `overlayOf` switches the install into overlay mode: when non-empty it's
// the addon ID of an installed base. Behavior changes:
//   - Destination folder is the base's folder, not the overlay's own.
//   - The wipe-and-backup step is skipped — overlay extracts on top of the
//     base, replacing matching files only.
//   - The installed.json row records OverlayOf so uninstalling the base
//     can cascade-clean the overlay's tracking entry.
//
// Refuses with an explicit error if `overlayOf` is set but the base isn't
// installed — overlays without a base would be useless on disk.
func (m *AddonManager) InstallAddon(addonID, name, version, repoURL, folderPath, branch, commitHash, overlayOf string, progressCallback ProgressCallback) error {
	// Audit #2 / #3 (defence in depth): the registry parse already rejects
	// malformed folder_name / github.path, but re-validate here so a future
	// caller that bypasses the registry (e.g. a hand-built test harness or
	// a feature that loads an addon from JSON on disk) can't sneak a
	// traversal-shaped value through this entry point.
	if err := registry.ValidateFolderName(name); err != nil {
		return fmt.Errorf("refusing to install: %v", err)
	}
	if err := registry.ValidateGithubPath(folderPath); err != nil {
		return fmt.Errorf("refusing to install: %v", err)
	}
	if overlayOf != "" {
		if err := registry.ValidateFolderName(overlayOf); err != nil {
			return fmt.Errorf("refusing to install: invalid overlay_of: %v", err)
		}
	}

	// Resolve the destination folder. For overlays this is the base's
	// folder (looked up from installed.json); for normal addons it's the
	// addon's own name. The "name" parameter is also the registry's
	// folder_name — we keep the param name unchanged since most addons
	// install into a folder that matches their name.
	destFolder := name
	if overlayOf != "" {
		base := m.GetInstalledAddon(overlayOf)
		if base == nil {
			return fmt.Errorf("can't install overlay: base addon %q is not installed", overlayOf)
		}
		destFolder = base.FolderName
	}

	repoInfo, err := m.githubClient.ParseRepoURL(repoURL)
	if err != nil {
		return err
	}

	// Default to "main" if no branch specified
	if branch == "" {
		branch = "main"
	}

	// Prefer the immutable commit SHA when the registry pinned one — this
	// guarantees we download exactly the bytes a maintainer reviewed, not
	// whatever's currently on the branch HEAD.
	downloadRef := branch
	if commitHash != "" {
		downloadRef = commitHash
	}

	// Overlays skip the wipe-and-backup phase: their whole point is to
	// layer on top of an existing base install. Normal installs back up
	// any existing folder so users can recover from a botched install.
	if overlayOf == "" {
		if progressCallback != nil {
			progressCallback(0, 100, "Backing up existing addon...")
		}
		if err := m.BackupAddon(destFolder); err != nil {
			return fmt.Errorf("backup failed: %v", err)
		}
	}

	if progressCallback != nil {
		progressCallback(5, 100, "Downloading addon from GitHub...")
	}

	// Download repository as ZIP
	zipData, err := m.githubClient.DownloadRepoAsZip(repoInfo.Owner, repoInfo.Repo, downloadRef, func(downloaded, total int64, speedMBps float64) {
		if progressCallback == nil {
			return
		}

		downloadedMB := float64(downloaded) / 1024 / 1024

		var progress int
		var message string

		if total > 0 {
			// We know the total size - show accurate progress from 5-60%
			progress = 5 + int(float64(55)*float64(downloaded)/float64(total))
			totalMB := float64(total) / 1024 / 1024

			if speedMBps > 0 {
				message = fmt.Sprintf("Downloading... %.1f / %.1f MB (%.1f MB/s)", downloadedMB, totalMB, speedMBps)
			} else {
				message = fmt.Sprintf("Downloading... %.1f / %.1f MB", downloadedMB, totalMB)
			}
		} else {
			// Unknown total size - show indeterminate progress at 30%
			progress = 30

			if speedMBps > 0 {
				message = fmt.Sprintf("Downloading... %.1f MB (%.1f MB/s)", downloadedMB, speedMBps)
			} else {
				message = fmt.Sprintf("Downloading... %.1f MB", downloadedMB)
			}
		}

		progressCallback(progress, 100, message)
	})
	if err != nil {
		return fmt.Errorf("failed to download ZIP: %v", err)
	}

	if progressCallback != nil {
		progressCallback(60, 100, "Extracting addon files...")
	}

	// Prepare destination directory
	cfg := config.Get()
	addonDir := filepath.Join(cfg.AddonPath, destFolder)

	// Normal installs wipe the destination first to avoid stale files
	// from a previous version sticking around. Overlays leave the base's
	// files in place — the extraction step writes new files on top,
	// overwriting only matching paths.
	if overlayOf == "" {
		if err := os.RemoveAll(addonDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing directory: %v", err)
		}
	}

	if err := os.MkdirAll(addonDir, 0755); err != nil {
		return err
	}

	// Extract ZIP with subfolder filtering
	err = m.githubClient.ExtractZipToFolder(zipData, addonDir, folderPath, func(current, total int) {
		if progressCallback != nil && total > 0 {
			// Progress from 60-90% for extraction
			progress := 60 + (30 * current / total)
			progressCallback(progress, 100, fmt.Sprintf("Extracting files... (%d/%d)", current, total))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to extract ZIP: %v", err)
	}

	if progressCallback != nil {
		progressCallback(90, 100, "Updating installation records...")
	}

	installed, err := m.LoadInstalledAddons()
	if err != nil {
		installed = &InstalledAddonsData{Addons: []InstalledAddon{}}
	}

	row := InstalledAddon{
		ID:               addonID,
		Name:             name,
		Version:          version,
		InstalledAt:      time.Now(),
		GithubCommitHash: commitHash,
		FolderName:       destFolder,
		OverlayOf:        overlayOf,
	}

	found := false
	for i, addon := range installed.Addons {
		if addon.ID == addonID {
			installed.Addons[i] = row
			found = true
			break
		}
	}

	if !found {
		installed.Addons = append(installed.Addons, row)
	}

	return m.SaveInstalledAddons(installed)
}

func (m *AddonManager) UninstallAddon(addonID string) error {
	installed, err := m.LoadInstalledAddons()
	if err != nil {
		return err
	}

	var addonToRemove *InstalledAddon
	for i := range installed.Addons {
		if installed.Addons[i].ID == addonID {
			a := installed.Addons[i]
			addonToRemove = &a
			break
		}
	}
	if addonToRemove == nil {
		return fmt.Errorf("addon not found in installed list")
	}

	if err := registry.ValidateFolderName(addonToRemove.FolderName); err != nil {
		return fmt.Errorf("refusing to uninstall (folder_name invalid in installed.json): %v", err)
	}

	cfg := config.Get()
	addonPath := filepath.Join(cfg.AddonPath, addonToRemove.FolderName)

	// Build the filtered installed list. Always drop the target row.
	// Additionally, when the target is a normal (non-overlay) addon and
	// we're about to remove its folder, drop any overlays that pointed
	// at it — their files just got nuked along with the base, so leaving
	// orphan tracking rows would lie to the user about what's installed.
	idsToDrop := map[string]bool{addonID: true}
	if addonToRemove.OverlayOf == "" {
		// Normal addon being uninstalled — cascade to its overlays.
		for _, a := range installed.Addons {
			if a.OverlayOf == addonID {
				idsToDrop[a.ID] = true
			}
		}
	}

	// Overlays don't own the folder. Skip the os.RemoveAll for them; the
	// base addon (if still installed) keeps its folder. Uninstalling an
	// overlay is essentially "stop tracking these patch files" — they
	// stay on disk and can only be reverted by reinstalling the base.
	if addonToRemove.OverlayOf == "" {
		if err := os.RemoveAll(addonPath); err != nil {
			return fmt.Errorf("failed to remove addon folder: %v", err)
		}
	}

	var kept []InstalledAddon
	for _, a := range installed.Addons {
		if !idsToDrop[a.ID] {
			kept = append(kept, a)
		}
	}
	installed.Addons = kept
	return m.SaveInstalledAddons(installed)
}

