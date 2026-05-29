package addon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"archerage-addon-manager/internal/config"
	"archerage-addon-manager/internal/github"
	"archerage-addon-manager/internal/registry"
)

// OverlayOf marks rows that don't own the folder they live in — they
// install on top of a base addon and get cascade-cleaned when the base
// is uninstalled.
type InstalledAddon struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Version          string    `json:"version"`
	InstalledAt      time.Time `json:"installed_at"`
	GithubCommitHash string    `json:"github_commit_hash"`
	FolderName       string    `json:"folder_name"`
	OverlayOf        string    `json:"overlay_of,omitempty"`
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

// Atomic write so a crash mid-write can't leave an empty installed.json
// (which would silently lose the user's installed-addon tracking).
func (m *AddonManager) SaveInstalledAddons(data *InstalledAddonsData) error {
	filePath := m.getInstalledFilePath()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, jsonData, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filePath)
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

type ProgressCallback func(current, total int, message string)

// overlayOf, when non-empty, switches install into overlay mode: extracts on
// top of the named base addon's folder instead of wiping a fresh folder.
// Refuses if the named base isn't installed.
func (m *AddonManager) InstallAddon(addonID, name, version, repoURL, folderPath, branch, commitHash, overlayOf string, skipBackup bool, progressCallback ProgressCallback) error {
	// Defence in depth — registry parse already rejects malformed values,
	// but re-validate so a non-registry caller can't bypass the check.
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

	if branch == "" {
		branch = "main"
	}

	// Pinned commit guarantees the bytes match what was reviewed.
	downloadRef := branch
	if commitHash != "" {
		downloadRef = commitHash
	}

	if progressCallback != nil {
		progressCallback(5, 100, "Downloading addon from GitHub...")
	}

	zipData, err := m.githubClient.DownloadRepoAsZip(repoInfo.Owner, repoInfo.Repo, downloadRef, func(downloaded, total int64, speedMBps float64) {
		if progressCallback == nil {
			return
		}

		downloadedMB := float64(downloaded) / 1024 / 1024

		var progress int
		var message string

		if total > 0 {
			progress = 5 + int(float64(55)*float64(downloaded)/float64(total))
			totalMB := float64(total) / 1024 / 1024
			if speedMBps > 0 {
				message = fmt.Sprintf("Downloading... %.1f / %.1f MB (%.1f MB/s)", downloadedMB, totalMB, speedMBps)
			} else {
				message = fmt.Sprintf("Downloading... %.1f / %.1f MB", downloadedMB, totalMB)
			}
		} else {
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

	cfg := config.Get()
	addonDir := filepath.Join(cfg.AddonPath, destFolder)

	// Overlays write on top of the base addon's folder — same as before.
	// Non-overlays stage to a sibling folder first, copy any user-data
	// files forward from the old install (anything not in the new
	// zipball), then atomically swap. Old folder gets moved to Backup/.
	if overlayOf != "" {
		if err := os.MkdirAll(addonDir, 0755); err != nil {
			return err
		}
		err = m.githubClient.ExtractZipToFolder(zipData, addonDir, folderPath, func(current, total int) {
			if progressCallback != nil && total > 0 {
				progress := 60 + (30 * current / total)
				progressCallback(progress, 100, fmt.Sprintf("Extracting files... (%d/%d)", current, total))
			}
		})
		if err != nil {
			return fmt.Errorf("failed to extract ZIP: %v", err)
		}
	} else {
		stagingDir := addonDir + ".staging"
		_ = os.RemoveAll(stagingDir) // any leftover from a previously-failed install
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			return fmt.Errorf("create staging dir: %v", err)
		}

		err = m.githubClient.ExtractZipToFolder(zipData, stagingDir, folderPath, func(current, total int) {
			if progressCallback != nil && total > 0 {
				progress := 60 + (28 * current / total)
				progressCallback(progress, 100, fmt.Sprintf("Extracting files... (%d/%d)", current, total))
			}
		})
		if err != nil {
			_ = os.RemoveAll(stagingDir)
			return fmt.Errorf("failed to extract ZIP: %v", err)
		}

		if _, statErr := os.Stat(addonDir); statErr == nil {
			if progressCallback != nil {
				progressCallback(88, 100, "Preserving your saved data...")
			}
			if err := preserveUserData(addonDir, stagingDir); err != nil {
				_ = os.RemoveAll(stagingDir)
				return fmt.Errorf("preserve user data: %v", err)
			}
			if !skipBackup {
				if err := m.BackupAddon(destFolder); err != nil {
					_ = os.RemoveAll(stagingDir)
					return fmt.Errorf("backup failed: %v", err)
				}
			} else {
				// Dev "skip backups" flag is set — still have to clear the live
				// folder so the staging rename can land on it.
				if err := os.RemoveAll(addonDir); err != nil {
					_ = os.RemoveAll(stagingDir)
					return fmt.Errorf("clear live folder for skip-backup swap: %v", err)
				}
			}
		}

		if err := os.Rename(stagingDir, addonDir); err != nil {
			_ = os.RemoveAll(stagingDir)
			return fmt.Errorf("swap-in new install: %v", err)
		}
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

	// Cascade: if the target owns its folder (non-overlay), removing it
	// nukes the files of any overlays that lived inside it — drop those
	// rows too so installed.json stays honest.
	idsToDrop := map[string]bool{addonID: true}
	if addonToRemove.OverlayOf == "" {
		for _, a := range installed.Addons {
			if a.OverlayOf == addonID {
				idsToDrop[a.ID] = true
			}
		}
	}

	// Overlays don't own the folder, so don't delete it; uninstalling an
	// overlay stops tracking the patch files but they stay on disk until
	// the base is reinstalled.
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

// preserveUserData walks the existing live folder and copies any files that
// AREN'T present at the same relative path in the staged new install. These
// are presumed to be user-data files (settings, saved state, recipe lists,
// etc.) the addon wrote at runtime. Without this step, every update would
// silently lose them.
//
// Symlinks and other non-regular files are skipped — addons shouldn't be
// shipping or producing those, and copying them blindly is a security risk.
func preserveUserData(liveDir, stagingDir string) error {
	return filepath.Walk(liveDir, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(liveDir, srcPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		mode := info.Mode()

		if info.IsDir() {
			stagingPath := filepath.Join(stagingDir, rel)
			if _, err := os.Stat(stagingPath); os.IsNotExist(err) {
				if err := os.MkdirAll(stagingPath, mode.Perm()); err != nil {
					return err
				}
			}
			return nil
		}

		if mode&os.ModeSymlink != 0 || !mode.IsRegular() {
			return nil
		}

		stagingPath := filepath.Join(stagingDir, rel)
		if _, err := os.Stat(stagingPath); err == nil {
			// The new install ships this file — let it overwrite, don't preserve.
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		return copyFile(srcPath, stagingPath)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
