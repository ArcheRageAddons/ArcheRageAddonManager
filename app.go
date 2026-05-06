package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"archerage-addon-manager/internal/addon"
	"archerage-addon-manager/internal/auth"
	"archerage-addon-manager/internal/config"
	"archerage-addon-manager/internal/github"
	"archerage-addon-manager/internal/github_auth"
	"archerage-addon-manager/internal/logger"
	"archerage-addon-manager/internal/registry"
	"archerage-addon-manager/internal/supabase"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// httpClient — package-shared with a 30s ceiling on every Supabase REST
// call so the app never hangs forever on a dead connection.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// updateHTTPClient — separate client for the self-update binary download.
// 5-minute ceiling because slow connections legitimately need several
// minutes to pull ~15MB. Other API calls keep the tight 30s default.
var updateHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// uuidRe is the strict UUID v4-shaped regex used to validate every
// submission ID we hand to PostgREST. Stops accidental injection-shaped
// input from being interpolated into URL filters.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateUUID(id string) error {
	if !uuidRe.MatchString(id) {
		return fmt.Errorf("invalid submission id: %q", id)
	}
	return nil
}

type App struct {
	ctx             context.Context
	addonManager    *addon.AddonManager
	githubClient    *github.GitHubClient
	registryClient  *registry.RegistryClient
	cachedAddons    []registry.Addon
	cachedStats     map[string]addonStatRow // shared between Browse + Details
	cachedMyRatings map[string]int          // user's own ratings, slug→1-5
}

// DependencyInfo describes one of an addon's declared dependencies, plus
// whether the user already has it installed.
//
//   - ID   — the registry filename (no .yaml). Stable, what DownloadAddon expects.
//   - Name — the dependency's display name from the registry (e.g. "Bag Counter").
//            Falls back to ID when the dep isn't in the cached registry yet
//            (deleted, or user hasn't refreshed since the dep was added).
//
// Render Name in the UI; pass ID to install / lookup calls.
type DependencyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsInstalled bool   `json:"is_installed"`
}

type AddonListItem struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	FolderName        string            `json:"folder_name"`
	Icon              string            `json:"icon"`
	Description       string            `json:"description"`
	Version           string            `json:"version"`
	Category          string            `json:"category"`
	AuthorName        string            `json:"author_name"`
	DownloadCount     int               `json:"download_count"`
	RatingAvg         float64           `json:"rating_avg"`
	RatingCount       int               `json:"rating_count"`
	HasDangerousFiles bool              `json:"has_dangerous_files"`
	Dependencies      []DependencyInfo  `json:"dependencies"`
	Keywords          []string          `json:"keywords"`
	IsInstalled       bool              `json:"is_installed"`
	HasUpdate         bool              `json:"has_update"`
	GithubRepoURL     string            `json:"github_repo_url"`
	GithubFolderPath  string            `json:"github_folder_path"`
	GithubBranch      string            `json:"github_branch"`
	GithubTag         string            `json:"github_tag"`
	GithubCommitHash  string            `json:"github_commit_hash"`
	SubmitterDiscord  string            `json:"submitter_discord,omitempty"`
	SubmitterGithub   string            `json:"submitter_github,omitempty"`
	SubmittedAt       string            `json:"submitted_at,omitempty"`
	// Overlay relationship. When OverlayOf is set, this addon installs into
	// the named base addon's folder. The frontend uses BaseInstalled to
	// gate the install button (overlays without their base on disk are
	// useless).
	OverlayOf     string `json:"overlay_of,omitempty"`
	BaseInstalled bool   `json:"base_installed,omitempty"`
}

type InstalledAddonInfo struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Version             string `json:"version"`
	InstalledAt         string `json:"installed_at"`
	HasUpdate           bool   `json:"has_update"`
	GithubCommitHash    string `json:"github_commit_hash"`
	RemovedFromRegistry bool   `json:"removed_from_registry"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialise the runtime log file first so anything that fails below
	// shows up in it. Best-effort — failures here only mean we lose disk
	// logs (stdout still works for `wails dev`); the manager keeps booting.
	logDir := filepath.Join(config.GetConfigDir(), "logs")
	if path, err := logger.Init(logDir); err != nil {
		fmt.Println("logger init failed:", err)
	} else {
		logger.Info(logger.Header(Version))
		logger.Infof("log file: %s", path)
	}

	if err := config.Init(); err != nil {
		logger.Errorf("config init failed: %v", err)
	}

	a.githubClient = github.NewGitHubClient()

	a.addonManager = addon.NewAddonManager()
	a.addonManager.SetGithubClient(a.githubClient)

	a.registryClient = registry.NewRegistryClient()

	// If the user's already signed in to GitHub from a prior session, use
	// their token for registry calls — bumps GitHub rate limit from 60/hr
	// (unauthenticated) to 5000/hr (authenticated).
	a.syncRegistryToken()

	// Best-effort: clean up the renamed-aside binary from a previous
	// self-update so the install dir doesn't accumulate "<name>.old" files.
	go cleanupOldBinary()

	// Background poll for new releases (no-op in dev mode).
	go a.updateCheckLoop()

	logger.Info("startup complete")
}

// syncRegistryToken refreshes the registry client's auth token from the
// user's stored GitHub OAuth token. Called on startup and after any
// GitHub login/logout. Empty token = unauthenticated calls.
func (a *App) syncRegistryToken() {
	if a.registryClient == nil {
		return
	}
	token, _ := github_auth.LoadToken()
	a.registryClient.SetToken(token)
}

func (a *App) GetCategories() []string {
	return []string{
		"All",
		"UI Enhancement",
		"Combat",
		"Crafting & Professions",
		"Map & Navigation",
		"Chat & Communication",
		"Trading & Economy",
		"Quality of Life",
		"Class Specific",
		"Utility",
		"Other",
	}
}

// convertDependencies turns the YAML's []string dependency list into
// DependencyInfo rows enriched with display name + install status. Display
// names come from the cached registry — falling back to the raw ID when
// the dep isn't present (deleted from registry, or user hasn't refreshed
// since the dep was added) so the user still sees something readable.
func (a *App) convertDependencies(deps []string) []DependencyInfo {
	var result []DependencyInfo
	for _, depID := range deps {
		name := depID
		for _, addon := range a.cachedAddons {
			if addon.ID == depID {
				name = addon.Name
				break
			}
		}
		result = append(result, DependencyInfo{
			ID:          depID,
			Name:        name,
			IsInstalled: a.addonManager.IsInstalled(depID),
		})
	}
	return result
}

func (a *App) GetAddons() ([]AddonListItem, error) {
	// Fetch from registry if not cached
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return nil, err
		}
		a.cachedAddons = addons
	}

	// Pull addon stats once per request and merge per-addon. Cheap (one
	// round-trip, public-read RLS) and means the frontend only needs the
	// existing GetAddons binding to render counts/ratings.
	stats := a.fetchAddonStats()

	var result []AddonListItem
	for _, addon := range a.cachedAddons {
		item := AddonListItem{
			ID:                addon.ID,
			Name:              addon.Name,
			FolderName:        addon.FolderName,
			Icon:              addon.Icon,
			Description:       addon.Description,
			Version:           addon.Version,
			Category:          addon.Category,
			AuthorName:        addon.Author,
			Dependencies:      a.convertDependencies(addon.Dependencies),
			Keywords:          addon.Keywords,
			IsInstalled:       a.addonManager.IsInstalled(addon.ID),
			GithubRepoURL:     addon.GitHub.Repo,
			GithubFolderPath:  addon.GitHub.Path,
			GithubBranch:      addon.GitHub.Branch,
			GithubTag:         addon.GitHub.Tag,
			HasDangerousFiles: addon.HasDangerousFiles,
			SubmitterDiscord:  addon.SubmitterDiscord,
			SubmitterGithub:   addon.SubmitterGithub,
			SubmittedAt:       addon.SubmittedAt,
			OverlayOf:         addon.OverlayOf,
			BaseInstalled:     addon.OverlayOf != "" && a.addonManager.IsInstalled(addon.OverlayOf),
		}

		// Check for updates if installed
		if item.IsInstalled {
			installed := a.addonManager.GetInstalledAddon(addon.ID)
			if installed != nil {
				if hasUpdate(installed.GithubCommitHash, addon.GitHub.Commit, installed.Version, addon.Version) {
					item.HasUpdate = true
				}
				item.GithubCommitHash = installed.GithubCommitHash
			}
		}

		// Merge stats (download count + rating). Defaults to zero when
		// the addon has no row yet — fine; UI hides empty stats.
		if s, ok := stats[addon.ID]; ok {
			item.DownloadCount = s.DownloadCount
			item.RatingCount = s.RatingCount
			if s.RatingCount > 0 {
				item.RatingAvg = float64(s.RatingSum) / float64(s.RatingCount)
			}
		}

		result = append(result, item)
	}

	return result, nil
}

func (a *App) GetAddonDetails(id string) (*AddonListItem, error) {
	// Ensure we have addons cached
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return nil, err
		}
		a.cachedAddons = addons
	}

	// Find the addon
	var found *registry.Addon
	for _, addon := range a.cachedAddons {
		if addon.ID == id {
			found = &addon
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("addon not found: %s", id)
	}

	item := &AddonListItem{
		ID:                found.ID,
		Name:              found.Name,
		FolderName:        found.FolderName,
		Icon:              found.Icon,
		Description:       found.Description,
		Version:           found.Version,
		Category:          found.Category,
		AuthorName:        found.Author,
		Dependencies:      a.convertDependencies(found.Dependencies),
		Keywords:          found.Keywords,
		IsInstalled:       a.addonManager.IsInstalled(found.ID),
		GithubRepoURL:     found.GitHub.Repo,
		GithubFolderPath:  found.GitHub.Path,
		GithubBranch:      found.GitHub.Branch,
		GithubTag:         found.GitHub.Tag,
		HasDangerousFiles: found.HasDangerousFiles,
		SubmitterDiscord:  found.SubmitterDiscord,
		SubmitterGithub:   found.SubmitterGithub,
		SubmittedAt:       found.SubmittedAt,
		OverlayOf:         found.OverlayOf,
		BaseInstalled:     found.OverlayOf != "" && a.addonManager.IsInstalled(found.OverlayOf),
	}

	// Stats — single-row fetch is fine here, but reuse the bulk helper
	// for consistency. One extra round-trip per details open.
	if s, ok := a.fetchAddonStats()[found.ID]; ok {
		item.DownloadCount = s.DownloadCount
		item.RatingCount = s.RatingCount
		if s.RatingCount > 0 {
			item.RatingAvg = float64(s.RatingSum) / float64(s.RatingCount)
		}
	}

	// Check for updates if installed
	if item.IsInstalled {
		installed := a.addonManager.GetInstalledAddon(found.ID)
		if installed != nil {
			if hasUpdate(installed.GithubCommitHash, found.GitHub.Commit, installed.Version, found.Version) {
				item.HasUpdate = true
			}
			item.GithubCommitHash = installed.GithubCommitHash
		}
	}

	return item, nil
}

type DownloadProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message"`
}

type DownloadResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	AddonID string `json:"addon_id"`
}

func (a *App) DownloadAddon(id string) error {
	// Ensure we have addons cached
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return err
		}
		a.cachedAddons = addons
	}

	// Find the addon
	var found *registry.Addon
	for _, addon := range a.cachedAddons {
		if addon.ID == id {
			found = &addon
			break
		}
	}

	if found == nil {
		return fmt.Errorf("addon not found: %s", id)
	}

	// Overlays require their base to be installed first. The install flow
	// in addon.Manager re-checks this, but failing fast here gives a clean
	// error before we kick off the goroutine + emit progress events.
	if found.OverlayOf != "" && !a.addonManager.IsInstalled(found.OverlayOf) {
		return fmt.Errorf("install %q first — this addon overlays on top of it", found.OverlayOf)
	}

	// Use the YAML's pinned commit if present (immutable — exactly what was
	// reviewed). Fall back to live branch HEAD for legacy addons that
	// haven't been pinned yet.
	commitHash := found.GitHub.Commit
	if commitHash == "" {
		var err error
		commitHash, err = a.registryClient.GetLatestCommit(found)
		if err != nil {
			return fmt.Errorf("failed to get commit hash: %v", err)
		}
	}

	// Build the full GitHub URL
	repoURL := "https://github.com/" + found.GitHub.Repo

	// Run download in goroutine to allow real-time progress updates
	go func() {
		logger.Infof("install start: %s v%s from %s@%s (path=%q overlay=%q)",
			found.ID, found.Version, found.GitHub.Repo, commitHash, found.GitHub.Path, found.OverlayOf)

		// Progress callback that emits events
		progressCallback := func(current, total int, message string) {
			wailsRuntime.EventsEmit(a.ctx, "download:progress", DownloadProgress{
				Current: current,
				Total:   total,
				Message: message,
			})
		}

		err := a.addonManager.InstallAddon(
			found.ID,
			found.FolderName,
			found.Version,
			repoURL,
			found.GitHub.Path,
			found.GitHub.Branch,
			commitHash,
			found.OverlayOf,
			progressCallback,
		)

		if err != nil {
			logger.Errorf("install failed: %s: %v", found.ID, err)
			// Emit error event
			wailsRuntime.EventsEmit(a.ctx, "download:complete", DownloadResult{
				Success: false,
				Error:   err.Error(),
				AddonID: found.ID,
			})
			return
		}
		logger.Infof("install complete: %s v%s", found.ID, found.Version)

		// Emit completion
		wailsRuntime.EventsEmit(a.ctx, "download:progress", DownloadProgress{
			Current: 100,
			Total:   100,
			Message: "Installation complete!",
		})

		// Bump the install counter (best-effort, fire-and-forget) and
		// invalidate the stats cache so the next stats read reflects it.
		go func(id string) {
			a.incrementDownloadCount(id)
			a.cachedStats = nil
		}(found.ID)

		// Emit success event
		wailsRuntime.EventsEmit(a.ctx, "download:complete", DownloadResult{
			Success: true,
			AddonID: found.ID,
		})
	}()

	return nil
}

func (a *App) UninstallAddon(id string) error {
	return a.addonManager.UninstallAddon(id)
}

func (a *App) GetInstalledAddons() ([]InstalledAddonInfo, error) {
	installed, err := a.addonManager.LoadInstalledAddons()
	if err != nil {
		return nil, err
	}

	// Ensure we have addons cached for update checking
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			// Don't fail, just continue without update info
			a.cachedAddons = []registry.Addon{}
		} else {
			a.cachedAddons = addons
		}
	}

	var result []InstalledAddonInfo
	for _, addon := range installed.Addons {
		info := InstalledAddonInfo{
			ID:               addon.ID,
			Name:             addon.Name,
			Version:          addon.Version,
			InstalledAt:      addon.InstalledAt.Format("2006-01-02 15:04"),
			GithubCommitHash: addon.GithubCommitHash,
		}

		// Check for updates AND whether the addon is still in the registry
		// at all (admin may have removed it; author may have deleted it).
		foundInRegistry := false
		for _, regAddon := range a.cachedAddons {
			if regAddon.ID == addon.ID {
				foundInRegistry = true
				if hasUpdate(addon.GithubCommitHash, regAddon.GitHub.Commit, addon.Version, regAddon.Version) {
					info.HasUpdate = true
				}
				break
			}
		}
		info.RemovedFromRegistry = !foundInRegistry

		result = append(result, info)
	}

	return result, nil
}

func (a *App) GetAddonPath() string {
	cfg := config.Get()
	return cfg.AddonPath
}

func (a *App) SetAddonPath(path string) error {
	return config.SetAddonPath(path)
}

func (a *App) SelectFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select ArcheRage Addon Folder",
	})
}

// IsFirstRun returns true if the user has not yet confirmed an Addon path.
// The frontend uses this to decide whether to show the welcome dialog.
func (a *App) IsFirstRun() bool {
	return config.IsFirstRun()
}

// DetectAddonPaths returns candidate Addon directories, including OneDrive
// variants. Used to populate the welcome dialog with suggestions.
func (a *App) DetectAddonPaths() []config.AddonPathCandidate {
	return config.DetectAddonPaths()
}

// ConfirmAddonPath records that the user has accepted the current AddonPath
// without changing it (used when accepting the auto-detected default).
func (a *App) ConfirmAddonPath() error {
	return config.MarkSetupComplete()
}

// ReleaseInfo describes a newer release found on GitHub. Returned to the
// frontend's update banner.
type ReleaseInfo struct {
	Version     string `json:"version"`      // tag name, e.g. "v0.4.0"
	URL         string `json:"url"`          // release page (where the .exe lives)
	SourceURL   string `json:"source_url"`   // /tree/<tag> — browse source at that commit
	Body        string `json:"body"`         // release notes
	PublishedAt string `json:"published_at"` // ISO timestamp
	// AssetURL is the direct download URL for the .exe attached to this
	// release. Used by InstallUpdate for the in-app self-updater. Empty
	// when no .exe asset is found — UI falls back to "open release page".
	AssetURL  string `json:"asset_url,omitempty"`
	AssetSize int64  `json:"asset_size,omitempty"`
}

// GetVersion returns the version baked in at build time. "dev" means
// running under `wails dev` (or built without -ldflags) — update checks
// are skipped in that case.
func (a *App) GetVersion() string { return Version }

// semverRe matches "v?X.Y.Z" with optional v prefix. CheckForUpdate
// short-circuits if the running binary doesn't have a parseable version.
var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// CheckForUpdate hits GitHub's releases-latest endpoint and returns
// release info if the latest is newer than our running version. nil
// (with no error) means "you're up to date" or "we can't tell".
func (a *App) CheckForUpdate() (*ReleaseInfo, error) {
	if !semverRe.MatchString(Version) {
		return nil, nil // dev build / unversioned — never claim there's an update
	}

	req, err := http.NewRequest("GET",
		"https://api.github.com/repos/ArcheRageAddons/ArcheRageAddonManager/releases/latest",
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	if t, _ := github_auth.LoadToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t) // higher rate limit when signed in
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("releases/latest returned %d: %s", resp.StatusCode, body)
	}

	var latest struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return nil, err
	}

	if latest.Draft || latest.Prerelease || latest.TagName == "" {
		return nil, nil
	}

	currentNum := strings.TrimPrefix(Version, "v")
	latestNum := strings.TrimPrefix(latest.TagName, "v")
	if !isVersionHigher(latestNum, currentNum) {
		return nil, nil
	}

	// Pick the .exe asset for the in-app self-updater. We prefer one whose
	// name matches the running binary's name (case-insensitive, no suffix
	// requirement); fall back to any *.exe attached. Empty when none —
	// banner UI then falls back to "open release page".
	var assetURL string
	var assetSize int64
	for _, a := range latest.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
			assetURL = a.BrowserDownloadURL
			assetSize = a.Size
			break
		}
	}

	return &ReleaseInfo{
		Version:     latest.TagName,
		URL:         latest.HTMLURL,
		SourceURL:   "https://github.com/ArcheRageAddons/ArcheRageAddonManager/tree/" + latest.TagName,
		Body:        latest.Body,
		PublishedAt: latest.PublishedAt,
		AssetURL:    assetURL,
		AssetSize:   assetSize,
	}, nil
}

// updateCheckLoop polls GitHub every 60 minutes for a newer release and
// emits an "update:available" event when one is found. Quietly does
// nothing in dev mode.
func (a *App) updateCheckLoop() {
	if !semverRe.MatchString(Version) {
		return
	}
	// Initial check after a short delay so we don't compete with login on startup
	time.Sleep(30 * time.Second)
	a.emitUpdateIfAvailable()

	ticker := time.NewTicker(60 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.emitUpdateIfAvailable()
	}
}

func (a *App) emitUpdateIfAvailable() {
	info, err := a.CheckForUpdate()
	if err != nil {
		return
	}
	if info == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "update:available", info)
}

// InstallUpdate downloads the supplied .exe URL into the directory next to
// the running binary, swaps the running binary out via Windows-safe rename
// trick, then relaunches the new binary and exits the current one.
//
// "Rename trick": Windows refuses to delete or overwrite a running .exe but
// allows renaming it. We rename current → "<exe>.old", drop the new binary
// at the original path, spawn it, and exit. On next launch the new binary
// cleans up the .old file (see cleanupOldBinary, called from startup).
//
// Caller must pass a URL pulled from CheckForUpdate's AssetURL — we don't
// trust an arbitrary URL here, just sanity-check the host and scheme.
//
// Emits update:download events for the UI:
//   - "update:download:progress" — { current, total, message }
//   - "update:download:complete" — { success, error? }
// On success the app exits ~500ms after emitting "complete" so the UI has a
// frame to show the success state before the window closes.
func (a *App) InstallUpdate(downloadURL string) error {
	logger.Infof("self-update start: %s", downloadURL)
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid update URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("update URL must be https")
	}
	// GitHub release-asset downloads start at github.com and 302 to
	// objects.githubusercontent.com — both are legitimate. Reject anything
	// else so a poisoned ReleaseInfo can't make us download from random
	// hosts (defence in depth; the response is already validated upstream).
	if parsed.Host != "github.com" && !strings.HasSuffix(parsed.Host, ".githubusercontent.com") {
		return fmt.Errorf("update URL host not allowed: %s", parsed.Host)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("can't locate running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	// OneDrive guard — File-on-Demand virtualisation periodically locks
	// files for sync, which makes the rename trick race-prone and the
	// new binary may end up as a stub. Refuse and tell the user to move
	// the manager out of OneDrive first.
	if isOneDrivePath(exePath) {
		return fmt.Errorf("the manager is in a OneDrive folder (%s) — auto-update can't safely run there. Move the .exe to a non-synced folder (e.g. %%LOCALAPPDATA%%\\Programs\\ArcheRageAddonManager) and try again", exePath)
	}

	dir := filepath.Dir(exePath)
	updatePath := filepath.Join(dir, "update.exe")
	oldPath := exePath + ".old"

	// Clean any leftovers from a previous failed update attempt before we
	// start writing fresh ones.
	_ = os.Remove(updatePath)
	_ = os.Remove(oldPath)

	emit := func(current, total int64, msg string) {
		wailsRuntime.EventsEmit(a.ctx, "update:download:progress", map[string]interface{}{
			"current": current,
			"total":   total,
			"message": msg,
		})
	}

	// Download the new binary. Streaming write so we don't buffer the whole
	// thing in RAM (~15MB but no reason to load it all).
	emit(0, 0, "Starting download...")

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	// Sanity cap — same ceiling as the addon zipball cap. A self-update
	// .exe should be ~15MB; anything close to the cap is suspicious.
	const maxUpdateBytes = 250 * 1024 * 1024
	if resp.ContentLength > maxUpdateBytes {
		return fmt.Errorf("update too large: %d bytes (max %d)", resp.ContentLength, maxUpdateBytes)
	}
	totalSize := resp.ContentLength

	out, err := os.Create(updatePath)
	if err != nil {
		return fmt.Errorf("can't create %s: %w", updatePath, err)
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	lastEmit := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if downloaded+int64(n) > maxUpdateBytes {
				out.Close()
				_ = os.Remove(updatePath)
				return fmt.Errorf("update exceeded %d bytes mid-download", maxUpdateBytes)
			}
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				_ = os.Remove(updatePath)
				return fmt.Errorf("write failed: %w", werr)
			}
			downloaded += int64(n)
			if time.Since(lastEmit) >= 50*time.Millisecond {
				emit(downloaded, totalSize, fmt.Sprintf("Downloading update... %.1f / %.1f MB",
					float64(downloaded)/1024/1024, float64(totalSize)/1024/1024))
				lastEmit = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			_ = os.Remove(updatePath)
			return fmt.Errorf("read failed: %w", rerr)
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(updatePath)
		return fmt.Errorf("close failed: %w", err)
	}
	emit(downloaded, downloaded, "Download complete, swapping binary...")

	// Rename trick.
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(updatePath)
		return fmt.Errorf("can't move running binary aside: %w", err)
	}
	if err := os.Rename(updatePath, exePath); err != nil {
		// Try to roll back so the user isn't left with a broken setup.
		_ = os.Rename(oldPath, exePath)
		_ = os.Remove(updatePath)
		return fmt.Errorf("can't move new binary into place: %w", err)
	}

	// Spawn the new binary detached so it doesn't die when this process
	// exits. exec.Cmd's default behaviour on Windows already gives the
	// child its own console; calling Release explicitly here so we don't
	// hold a process handle open across our own os.Exit.
	cmd := exec.Command(exePath)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		// Best-effort rollback again — the user just lost auto-update but
		// shouldn't lose their manager. They'll have to manually launch
		// the .old file or re-download.
		return fmt.Errorf("can't launch new binary: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	wailsRuntime.EventsEmit(a.ctx, "update:download:complete", map[string]interface{}{
		"success": true,
	})

	// Give the UI a frame to render the "complete" state, then exit so the
	// new binary takes over.
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

// isOneDrivePath returns true if the path looks like it lives inside a
// OneDrive-synced folder. Heuristic only — OneDrive's exact paths vary
// per-user, so we check for common substrings.
func isOneDrivePath(p string) bool {
	lower := strings.ToLower(p)
	return strings.Contains(lower, "onedrive") || strings.Contains(lower, "\\onedrive\\")
}

// cleanupOldBinary removes "<exe>.old" left over from a previous self-update.
// Best-effort: errors (e.g. file doesn't exist) are silently ignored.
func cleanupOldBinary() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	_ = os.Remove(exePath + ".old")
}

// LoginWithDiscord runs the PKCE OAuth flow against Supabase. Blocks until
// the user finishes (or aborts) the browser-side login. Emits "auth:changed"
// on success so the frontend can refresh user state.
func (a *App) LoginWithDiscord() (*auth.User, error) {
	user, err := auth.Login()
	if err != nil {
		logger.Errorf("discord login failed: %v", err)
		return nil, err
	}
	logger.Infof("discord login: %s (id=%s admin=%v)", user.DiscordUsername, user.DiscordID, user.IsAdmin)
	wailsRuntime.EventsEmit(a.ctx, "auth:changed", user)
	return user, nil
}

// Logout clears stored Supabase tokens.
func (a *App) Logout() error {
	if err := auth.Logout(); err != nil {
		logger.Errorf("logout failed: %v", err)
		return err
	}
	logger.Info("discord logout")
	wailsRuntime.EventsEmit(a.ctx, "auth:changed", nil)
	return nil
}

// GetCurrentUser returns the logged-in user (with profile fields hydrated)
// or nil if no session is active.
func (a *App) GetCurrentUser() (*auth.User, error) {
	return auth.CurrentUser()
}

// ===========================================================================
// GitHub Device Flow auth — second leg of identity (proves repo ownership).
// ===========================================================================

// StartGitHubAuth kicks off the GitHub Device Flow and returns the user_code
// the user must enter at verification_uri. Polling for the eventual token
// runs in the background; the frontend listens for the "github:auth:done"
// event for success/failure.
func (a *App) StartGitHubAuth() (*github_auth.DeviceFlowInit, error) {
	init, err := github_auth.StartDeviceFlow()
	if err != nil {
		return nil, err
	}

	go func() {
		token, err := github_auth.PollForToken(init.DeviceCode, init.Interval, init.ExpiresIn)
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "github:auth:done", map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		if err := github_auth.SaveToken(token); err != nil {
			wailsRuntime.EventsEmit(a.ctx, "github:auth:done", map[string]interface{}{
				"ok":    false,
				"error": "failed to store token: " + err.Error(),
			})
			return
		}
		a.syncRegistryToken()
		user, err := github_auth.GetUser()
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "github:auth:done", map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "github:auth:done", map[string]interface{}{
			"ok":   true,
			"user": user,
		})
	}()

	return init, nil
}

// GetGitHubUser returns the cached GitHub user, or nil if not connected.
func (a *App) GetGitHubUser() (*github_auth.User, error) {
	if !github_auth.IsConnected() {
		return nil, nil
	}
	return github_auth.GetUser()
}

// LogoutGitHub clears the stored GitHub token.
func (a *App) LogoutGitHub() error {
	if err := github_auth.ClearToken(); err != nil {
		return err
	}
	a.syncRegistryToken()
	return nil
}

// ListMyRepos returns the user's writable GitHub repos for the publish form.
func (a *App) ListMyRepos() ([]github_auth.Repo, error) {
	return github_auth.ListWritableRepos()
}

// ===========================================================================
// Submission flow — generates the registry YAML and POSTs it to Supabase.
// ===========================================================================

// SubmitAddonRequest is the form payload from the frontend's publish modal.
type SubmitAddonRequest struct {
	Name         string   `json:"name"`
	FolderName   string   `json:"folder_name"`
	Author       string   `json:"author"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Keywords     []string `json:"keywords"`
	Icon         string   `json:"icon"`
	Dependencies []string `json:"dependencies"`
	GithubRepo   string   `json:"github_repo"`
	GithubBranch string   `json:"github_branch"`
	GithubPath   string   `json:"github_path"`
	// Optional. When set, this addon installs on top of the named base addon
	// (must be an existing registry id). The desktop's installer routes the
	// extraction into the base's folder rather than the overlay's own.
	OverlayOf string `json:"overlay_of"`
}

// AdminSubmission is what the admin review panel sees per submission.
// Includes embedded submitter info (joined from public.profiles).
type AdminSubmission struct {
	ID              string  `json:"id"`
	AddonSlug       string  `json:"addon_slug"`
	YAMLContent     string  `json:"yaml_content"`
	GithubRepo      string  `json:"github_repo"`
	GithubPath      string  `json:"github_path"`
	Status          string  `json:"status"`
	DecisionReason  *string `json:"decision_reason,omitempty"`
	CreatedAt       string  `json:"created_at"`
	DecidedAt       *string `json:"decided_at,omitempty"`
	GithubPRNumber  *int    `json:"github_pr_number,omitempty"`
	GithubPRURL     *string `json:"github_pr_url,omitempty"`
	SubmitterName   string  `json:"submitter_name"`
	SubmitterDiscID string  `json:"submitter_discord_id,omitempty"`
}

// SubmissionRow mirrors public.submissions for the GET response.
type SubmissionRow struct {
	ID              string  `json:"id"`
	AddonSlug       string  `json:"addon_slug"`
	YAMLContent     string  `json:"yaml_content"`
	GithubRepo      string  `json:"github_repo"`
	GithubPath      string  `json:"github_path"`
	Status          string  `json:"status"`
	DecisionReason  *string `json:"decision_reason,omitempty"`
	CreatedAt       string  `json:"created_at"`
	DecidedAt       *string `json:"decided_at,omitempty"`
	GithubPRNumber  *int    `json:"github_pr_number,omitempty"`
	GithubPRURL     *string `json:"github_pr_url,omitempty"`
	GithubPRBranch  *string `json:"github_pr_branch,omitempty"`
}

// addonYAML is what we marshal into yaml_content.
type addonYAML struct {
	Name         string         `yaml:"name"`
	FolderName   string         `yaml:"folder_name"`
	Author       string         `yaml:"author"`
	Version      string         `yaml:"version"`
	Description  string         `yaml:"description,omitempty"`
	Category     string         `yaml:"category,omitempty"`
	Icon         string         `yaml:"icon,omitempty"`
	Keywords     []string       `yaml:"keywords,omitempty"`
	Dependencies []string       `yaml:"dependencies,omitempty"`
	OverlayOf    string         `yaml:"overlay_of,omitempty"`
	GitHub       addonYAMLGitHb `yaml:"github"`
}

type addonYAMLGitHb struct {
	Repo   string `yaml:"repo"`
	Branch string `yaml:"branch,omitempty"`
	Commit string `yaml:"commit,omitempty"`
	Path   string `yaml:"path,omitempty"`
}

// SubmitAddonResult is what the desktop app shows the user on success.
type SubmitAddonResult struct {
	SubmissionID string `json:"submission_id"`
	PRNumber     int    `json:"pr_number,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
}

// SubmitAddon validates the form, builds the YAML, then hands the whole
// payload to the submission-open-pr Edge Function which atomically inserts
// the submissions row + opens the PR. If the EF fails for any reason (PR
// creation, GitHub permissions, anything) it rolls back the row, so the
// desktop app never has to clean up orphan state. Returns submission ID +
// PR info on success, an error on failure.
func (a *App) SubmitAddon(r SubmitAddonRequest) (*SubmitAddonResult, error) {
	logger.Infof("submission start: slug=%s repo=%s path=%q", r.FolderName, r.GithubRepo, r.GithubPath)
	if err := validateSubmission(&r); err != nil {
		logger.Warnf("submission validation failed: %v", err)
		return nil, err
	}

	user, err := auth.CurrentUser()
	if err != nil {
		return nil, fmt.Errorf("not signed in: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("not signed in")
	}
	if user.IsBanned {
		return nil, fmt.Errorf("your account is banned from submitting")
	}

	// Resolve the user's branch (or tag if we ever expose it) to an
	// immutable commit SHA, then pin the YAML to that SHA. Users only ever
	// download exactly the bytes a maintainer reviewed — no "author pushes
	// new code post-approval" attack window.
	branch := defaultStr(r.GithubBranch, "main")
	commit, err := github_auth.ResolveRef(r.GithubRepo, "heads/"+branch)
	if err != nil {
		return nil, fmt.Errorf("couldn't resolve %s @ %s to a commit (is the branch correct? are you signed in to GitHub?): %w", r.GithubRepo, branch, err)
	}

	yamlBytes, err := yaml.Marshal(addonYAML{
		Name:         r.Name,
		FolderName:   r.FolderName,
		Author:       r.Author,
		Version:      r.Version,
		Description:  r.Description,
		Category:     defaultStr(r.Category, "Other"),
		Icon:         r.Icon,
		Keywords:     trimAll(r.Keywords),
		Dependencies: trimAll(r.Dependencies),
		OverlayOf:    strings.TrimSpace(r.OverlayOf),
		GitHub: addonYAMLGitHb{
			Repo:   r.GithubRepo,
			Branch: branch,
			Commit: commit,
			Path:   r.GithubPath,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}

	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, fmt.Errorf("not signed in to Supabase")
	}

	// We pass the user's GitHub OAuth token to the EF so it can verify
	// (server-side) that the caller actually has push access to the repo
	// they're claiming as the addon source. Token has zero scopes — only
	// lets the EF read the user's login + check repo permissions.
	ghToken, _ := github_auth.LoadToken()
	if ghToken == "" {
		return nil, fmt.Errorf("connect GitHub before submitting")
	}

	// 5-minute ceiling — submission-open-pr does enough GitHub work that
	// big-repo submissions (deep dangerous-file scan, plus branch + commit
	// + PR creation) can easily run past 30s. Supabase's own EF execution
	// limit is 400s, so this is well under the platform cap. Identity calls
	// elsewhere keep using the default tight 30s.
	status, body, err := github_auth.PostJSONWithTimeout(
		supabase.URL+"/functions/v1/submission-open-pr",
		map[string]string{
			"addon_slug":   strings.ToLower(strings.TrimSpace(r.FolderName)),
			"yaml_content": string(yamlBytes),
			"github_repo":  r.GithubRepo,
			"github_path":  r.GithubPath,
			"github_token": ghToken,
		},
		map[string]string{
			"apikey":        supabase.PublishableKey,
			"Authorization": "Bearer " + tokens.AccessToken,
		},
		5*time.Minute,
	)
	if err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}

	var resp struct {
		SubmissionID string `json:"submission_id"`
		PRNumber     int    `json:"pr_number"`
		PRURL        string `json:"pr_url"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(body, &resp)

	if status != 200 {
		if resp.Error != "" {
			return nil, fmt.Errorf("submission failed: %s", resp.Error)
		}
		return nil, fmt.Errorf("submission failed (%d): %s", status, string(body))
	}

	return &SubmitAddonResult{
		SubmissionID: resp.SubmissionID,
		PRNumber:     resp.PRNumber,
		PRURL:        resp.PRURL,
	}, nil
}

// GetMySubmissions returns the current user's submissions, newest first.
// RLS naturally scopes this — users only see their own (admins see all but
// the desktop app deliberately doesn't expose admin views here).
func (a *App) GetMySubmissions() ([]SubmissionRow, error) {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, nil
	}

	url := supabase.URL + "/rest/v1/submissions?select=id,addon_slug,yaml_content,github_repo,github_path,status,decision_reason,created_at,decided_at,github_pr_number,github_pr_url,github_pr_branch&order=created_at.desc"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("submissions fetch returned %d: %s", resp.StatusCode, body)
	}

	var rows []SubmissionRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// BuildUpdateForm parses a stored submission's YAML into a
// SubmitAddonRequest the publish form can pre-fill from. Takes the row
// data the frontend already has from GetMySubmissions — no Supabase
// round-trip on every "Update" click.
//
// Bumps the patch component of the version automatically as a starting
// suggestion (1.0.0 → 1.0.1) — author can edit before submitting.
func (a *App) BuildUpdateForm(yamlContent, githubRepo, githubPath string) (*SubmitAddonRequest, error) {
	if strings.TrimSpace(yamlContent) == "" {
		return nil, fmt.Errorf("empty YAML")
	}

	var parsed addonYAML
	if err := yaml.Unmarshal([]byte(yamlContent), &parsed); err != nil {
		return nil, fmt.Errorf("parse stored YAML: %w", err)
	}

	branch := parsed.GitHub.Branch
	if branch == "" {
		branch = "main"
	}

	return &SubmitAddonRequest{
		Name:         parsed.Name,
		FolderName:   parsed.FolderName,
		Author:       parsed.Author,
		Version:      bumpPatch(parsed.Version),
		Description:  parsed.Description,
		Category:     parsed.Category,
		Keywords:     parsed.Keywords,
		Icon:         parsed.Icon,
		Dependencies: parsed.Dependencies,
		OverlayOf:    parsed.OverlayOf,
		GithubRepo:   defaultStr(githubRepo, parsed.GitHub.Repo),
		GithubBranch: branch,
		GithubPath:   defaultStr(githubPath, parsed.GitHub.Path),
	}, nil
}

// bumpPatch increments the last numeric component of a semver-like string.
// "1.2.3" → "1.2.4", "1.2" → "1.2.1", "v1.0" → "v1.0.1" (etc).
// Falls back to returning the input unchanged if no digits found.
func bumpPatch(v string) string {
	parts := parseVersion(v)
	if len(parts) == 0 {
		return v
	}
	parts[len(parts)-1]++
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "."
		}
		out += fmt.Sprintf("%d", p)
	}
	return out
}

// WithdrawSubmission lets the submitter cancel their own pending entry.
// Closes the PR via the GitHub App and flips the row to status='withdrawn'.
func (a *App) WithdrawSubmission(submissionID string) error {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}
	status, body, err := github_auth.PostJSON(
		supabase.URL+"/functions/v1/submission-withdraw",
		map[string]string{"submission_id": submissionID},
		map[string]string{
			"apikey":        supabase.PublishableKey,
			"Authorization": "Bearer " + tokens.AccessToken,
		},
	)
	if err != nil {
		return err
	}
	if status != 200 {
		var resp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &resp)
		if resp.Error != "" {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("returned %d: %s", status, string(body))
	}
	return nil
}

// AdminUserRow is what the admin Users tab displays per profile. Strictly
// read-only; the desktop app never exposes ban/unban or admin
// grant/revoke — those are done by hand in SQL only, intentionally.
type AdminUserRow struct {
	ID              string  `json:"id"`
	DiscordID       string  `json:"discord_id"`
	DiscordUsername string  `json:"discord_username"`
	DiscordAvatar   *string `json:"discord_avatar,omitempty"`
	GithubLogin     *string `json:"github_login,omitempty"`
	IsAdmin         bool    `json:"is_admin"`
	IsBanned        bool    `json:"is_banned"`
	CreatedAt       string  `json:"created_at"`
}

// GetAllUsers returns every profile in the system. RLS gates this — the
// "admins read all" policy lets admins see all rows; non-admins get an
// empty array (still gated server-side).
func (a *App) GetAllUsers() ([]AdminUserRow, error) {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, fmt.Errorf("not signed in")
	}

	url := supabase.URL +
		"/rest/v1/profiles?select=id,discord_id,discord_username,discord_avatar," +
		"github_login,is_admin,is_banned,created_at&order=created_at.desc"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("users fetch returned %d: %s", resp.StatusCode, body)
	}

	var rows []AdminUserRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteAddon removes the addon from the registry entirely (deletes the
// YAML file from ArcheRageAddons/addons), closes any pending PRs for the
// slug, and wipes the caller's submission rows for it. Caller must own
// the addon (be the submitter of the latest approved version) — enforced
// inside the addon-delete Edge Function.
func (a *App) DeleteAddon(slug string) error {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}

	status, body, err := github_auth.PostJSON(
		supabase.URL+"/functions/v1/addon-delete",
		map[string]string{"addon_slug": slug},
		map[string]string{
			"apikey":        supabase.PublishableKey,
			"Authorization": "Bearer " + tokens.AccessToken,
		},
	)
	if err != nil {
		return err
	}
	if status != 200 {
		var resp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &resp)
		if resp.Error != "" {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("returned %d: %s", status, string(body))
	}
	return nil
}

// DeleteSubmissions removes the given submission rows. RLS gates this:
// the user can only delete their own non-pending submissions. IDs they
// don't own or that are still pending are silently skipped (PostgREST
// returns 204 either way). Used for both per-row delete and Clear-all.
func (a *App) DeleteSubmissions(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := validateUUID(id); err != nil {
			return err
		}
	}
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}

	// Build the PostgREST in.() filter: id=in.(uuid1,uuid2,...)
	deleteURL := fmt.Sprintf("%s/rest/v1/submissions?id=in.(%s)",
		supabase.URL, strings.Join(ids, ","))

	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetPendingSubmissions returns all pending submissions for admin review.
// Server-side RLS ("admins read all subs") allows this; non-admins get an
// empty array because the policy filter excludes them.
//
// Filters to rows that have an actual github_pr_number populated. Without
// this filter, rows whose submission-open-pr Edge Function died mid-flight
// (Supabase WORKER_RESOURCE_LIMIT, timeout, etc., before the rollback
// could fire) show up as pending submissions the admin can't act on
// because the underlying PR doesn't exist. Approve/Deny EFs all need a PR
// number to operate, so hiding rows without one is the right answer here.
// Orphan rows still exist in the DB but stay invisible to admins; users
// can withdraw their own orphans via MyAddons (the withdraw EF tolerates
// NULL pr_number cleanly).
func (a *App) GetPendingSubmissions() ([]AdminSubmission, error) {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, fmt.Errorf("not signed in")
	}

	url := supabase.URL +
		"/rest/v1/submissions?status=eq.pending&github_pr_number=not.is.null&order=created_at.asc" +
		"&select=id,addon_slug,yaml_content,github_repo,github_path,status," +
		"decision_reason,created_at,decided_at,github_pr_number,github_pr_url," +
		"submitter:profiles!submitted_by(discord_username,discord_id)"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("submissions fetch returned %d: %s", resp.StatusCode, body)
	}

	var raw []struct {
		ID             string  `json:"id"`
		AddonSlug      string  `json:"addon_slug"`
		YAMLContent    string  `json:"yaml_content"`
		GithubRepo     string  `json:"github_repo"`
		GithubPath     string  `json:"github_path"`
		Status         string  `json:"status"`
		DecisionReason *string `json:"decision_reason,omitempty"`
		CreatedAt      string  `json:"created_at"`
		DecidedAt      *string `json:"decided_at,omitempty"`
		GithubPRNumber *int    `json:"github_pr_number,omitempty"`
		GithubPRURL    *string `json:"github_pr_url,omitempty"`
		Submitter      struct {
			DiscordUsername string `json:"discord_username"`
			DiscordID       string `json:"discord_id"`
		} `json:"submitter"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	out := make([]AdminSubmission, 0, len(raw))
	for _, r := range raw {
		out = append(out, AdminSubmission{
			ID:              r.ID,
			AddonSlug:       r.AddonSlug,
			YAMLContent:     r.YAMLContent,
			GithubRepo:      r.GithubRepo,
			GithubPath:      r.GithubPath,
			Status:          r.Status,
			DecisionReason:  r.DecisionReason,
			CreatedAt:       r.CreatedAt,
			DecidedAt:       r.DecidedAt,
			GithubPRNumber:  r.GithubPRNumber,
			GithubPRURL:     r.GithubPRURL,
			SubmitterName:   r.Submitter.DiscordUsername,
			SubmitterDiscID: r.Submitter.DiscordID,
		})
	}
	return out, nil
}

// ApproveSubmission triggers the submission-approve Edge Function which
// merges the PR via the GitHub App and flips the row to approved.
func (a *App) ApproveSubmission(id string) error {
	return a.callDecisionEdgeFn("submission-approve", id, "")
}

// DenySubmission triggers the submission-deny Edge Function which posts a
// comment on the PR (with the optional reason), closes it without merging,
// and flips the row to denied. Reason is optional but encouraged.
func (a *App) DenySubmission(id, reason string) error {
	return a.callDecisionEdgeFn("submission-deny", id, reason)
}

func (a *App) callDecisionEdgeFn(name, submissionID, reason string) error {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}
	payload := map[string]string{"submission_id": submissionID}
	if reason != "" {
		payload["reason"] = reason
	}

	status, body, err := github_auth.PostJSON(
		supabase.URL+"/functions/v1/"+name,
		payload,
		map[string]string{
			"apikey":        supabase.PublishableKey,
			"Authorization": "Bearer " + tokens.AccessToken,
		},
	)
	if err != nil {
		return err
	}
	if status != 200 {
		var resp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &resp)
		if resp.Error != "" {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("returned %d: %s", status, string(body))
	}
	return nil
}

func validateSubmission(r *SubmitAddonRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.FolderName = strings.TrimSpace(r.FolderName)
	r.Author = strings.TrimSpace(r.Author)
	r.Version = strings.TrimSpace(r.Version)
	r.GithubRepo = strings.TrimSpace(r.GithubRepo)
	r.GithubBranch = strings.TrimSpace(r.GithubBranch)
	r.GithubPath = strings.TrimSpace(r.GithubPath)
	r.OverlayOf = strings.TrimSpace(r.OverlayOf)

	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.FolderName == "" {
		return errors.New("folder name is required")
	}
	if r.Author == "" {
		return errors.New("author is required")
	}
	if r.Version == "" {
		return errors.New("version is required")
	}
	if r.GithubRepo == "" {
		return errors.New("github repo is required")
	}
	if !strings.Contains(r.GithubRepo, "/") {
		return errors.New("github repo must look like owner/repo")
	}
	if r.OverlayOf != "" {
		// Same shape as a registry id (filename without .yaml). Reuses the
		// folder_name validator since both have the same character class.
		if err := registry.ValidateFolderName(r.OverlayOf); err != nil {
			return fmt.Errorf("overlay base id is invalid: %v", err)
		}
		if r.OverlayOf == strings.ToLower(r.FolderName) {
			return errors.New("overlay base must be a different addon")
		}
	}
	return nil
}

func trimAll(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func defaultStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// loadSupabaseTokens fetches the freshest available access + refresh tokens
// via auth.RefreshIfNeeded, which transparently refreshes near-expiry tokens
// against Supabase. When the underlying refresh token is rejected (revoked,
// expired beyond its 60-day TTL, etc.), the auth package wipes the stored
// tokens and returns ErrSessionExpired — here we translate that to a
// user-friendly error and notify the frontend so the AccountPanel + sidebar
// reflect the logged-out state immediately.
//
// Method on App rather than a free function so we have a.ctx for the
// auth:changed event emit.
func (a *App) loadSupabaseTokens() (*auth.Tokens, error) {
	t, err := auth.RefreshIfNeeded()
	if err != nil {
		if errors.Is(err, auth.ErrSessionExpired) {
			logger.Warn("supabase session expired; wiping stored tokens and notifying frontend")
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "auth:changed", nil)
			}
			return nil, fmt.Errorf("Your Discord session has expired. Please log out and log back in to continue. (Your installed addons and settings are unaffected.)")
		}
		return nil, err
	}
	return t, nil
}

func (a *App) RefreshAddons() error {
	// Clear all session caches so the next read pulls fresh.
	a.cachedAddons = nil
	a.cachedStats = nil
	a.cachedMyRatings = nil

	addons, err := a.registryClient.GetAllAddons()
	if err != nil {
		return err
	}
	a.cachedAddons = addons
	return nil
}

// openURLAllowedHosts is the strict allow-list of hosts the desktop app is
// allowed to open in the user's browser. Anything else fails fast — stops
// `OpenURL` from launching arbitrary URI schemes (file:, javascript:, vendor
// schemes, etc.) or redirect-shaped URLs like `https://github.com/../evil`
// even if a server-supplied string sneaks past validation upstream.
var openURLAllowedHosts = map[string]bool{
	"github.com": true,
}

func (a *App) OpenURL(target string) error {
	parsed, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("OpenURL refused: only https allowed (got %q)", parsed.Scheme)
	}
	if !openURLAllowedHosts[parsed.Host] {
		return fmt.Errorf("OpenURL refused: host %q not in allow-list", parsed.Host)
	}
	// P4-7 + P5-1: Go's url.Parse keeps Host=github.com on inputs like
	// "https://github.com/../evil.example.com/path", but the browser will
	// normalise the path and visit evil.example.com. Reject any URL whose
	// path normalises to something different — that catches `..` traversal,
	// double slashes, etc. before they reach rundll32.
	//
	// Use the *decoded* path (parsed.Path) rather than EscapedPath: pass-5
	// found that percent-encoded "..", e.g. "%2E%2E", contains no literal
	// ".." in the escaped form so a substring check there doesn't fire. The
	// decoded path turns "%2E%2E" back into ".." and path.Clean folds it,
	// so a single "Clean(p) != p" check catches every traversal variant.
	if parsed.Path != "" {
		if cleaned := path.Clean(parsed.Path); cleaned != parsed.Path {
			return fmt.Errorf("OpenURL refused: path traversal in URL")
		}
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

// addonStatRow mirrors a row of public.addon_stats for the bulk fetch.
type addonStatRow struct {
	AddonSlug     string `json:"addon_slug"`
	DownloadCount int    `json:"download_count"`
	RatingSum     int    `json:"rating_sum"`
	RatingCount   int    `json:"rating_count"`
}

// fetchAddonStats reads the entire addon_stats table in one shot. RLS
// makes the table public-read so this works without auth. Result is
// cached on the App struct and reused across Browse → Details opens
// during the same session — Refresh clears it. Failed fetches don't
// poison the cache.
func (a *App) fetchAddonStats() map[string]addonStatRow {
	if a.cachedStats != nil {
		return a.cachedStats
	}

	out := map[string]addonStatRow{}
	req, err := http.NewRequest("GET",
		supabase.URL+"/rest/v1/addon_stats?select=addon_slug,download_count,rating_sum,rating_count",
		nil)
	if err != nil {
		return out
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out
	}

	var rows []addonStatRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return out
	}
	for _, r := range rows {
		out[r.AddonSlug] = r
	}
	a.cachedStats = out
	return out
}

// fetchMyRatings reads the caller's own ratings (RLS-scoped) into a
// slug→rating map and caches it for the session. Returns nil if not
// signed in. Same Refresh-clears-it lifecycle as fetchAddonStats.
func (a *App) fetchMyRatings() map[string]int {
	if a.cachedMyRatings != nil {
		return a.cachedMyRatings
	}
	tokens, err := a.loadSupabaseTokens()
	if err != nil || tokens == nil {
		return nil
	}

	req, err := http.NewRequest("GET",
		supabase.URL+"/rest/v1/addon_ratings?select=addon_slug,rating",
		nil)
	if err != nil {
		return nil
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}

	var rows []struct {
		AddonSlug string `json:"addon_slug"`
		Rating    int    `json:"rating"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.AddonSlug] = r.Rating
	}
	a.cachedMyRatings = out
	return out
}

// incrementDownloadCount fires-and-forgets a bump to the download counter
// for the given slug. Anonymous-callable — counts every successful install
// regardless of login state. Trade-off accepted: counter is trivially
// inflatable but has no downstream effect (no ranking / rewards / etc.),
// and the maintainer can reset values in SQL if abuse becomes visible.
func (a *App) incrementDownloadCount(slug string) {
	body, _ := json.Marshal(map[string]string{"p_slug": slug})
	req, err := http.NewRequest("POST",
		supabase.URL+"/rest/v1/rpc/increment_addon_downloads",
		bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Content-Type", "application/json")
	// If we have a Supabase session, send it — minor benefit: PostgREST
	// route uses the user's role, fewer anonymous spikes from one machine.
	// Not required for the call to succeed.
	if tokens, err := a.loadSupabaseTokens(); err == nil && tokens != nil {
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// SetAddonRating records the caller's rating (1-5) for an addon. Calls
// the set_addon_rating RPC which atomically updates the aggregate.
func (a *App) SetAddonRating(slug string, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"p_slug":   slug,
		"p_rating": rating,
	})
	req, err := http.NewRequest("POST",
		supabase.URL+"/rest/v1/rpc/set_addon_rating",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rating RPC returned %d: %s", resp.StatusCode, b)
	}
	a.cachedStats = nil
	a.cachedMyRatings = nil
	return nil
}

// ClearAddonRating removes the caller's rating for an addon.
func (a *App) ClearAddonRating(slug string) error {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return err
	}
	if tokens == nil {
		return fmt.Errorf("not signed in")
	}
	body, _ := json.Marshal(map[string]string{"p_slug": slug})
	req, err := http.NewRequest("POST",
		supabase.URL+"/rest/v1/rpc/clear_addon_rating",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clear-rating RPC returned %d: %s", resp.StatusCode, b)
	}
	a.cachedStats = nil
	a.cachedMyRatings = nil
	return nil
}

// GetMyRating returns the caller's rating for an addon, or 0 if none.
// Reads from the in-memory ratings cache populated once per session by
// fetchMyRatings (one call per session for the whole user instead of one
// per modal open).
func (a *App) GetMyRating(slug string) (int, error) {
	ratings := a.fetchMyRatings()
	if ratings == nil {
		return 0, nil
	}
	return ratings[slug], nil
}

// OpenBackupFolder opens the addon backup directory in the user's file
// manager. The directory is created on first install (when something needs
// backing up) so it may not exist yet — we MkdirAll it as a courtesy so the
// "Open backup folder" button never silently fails on a fresh user.
func (a *App) OpenBackupFolder() error {
	addonPath := config.Get().AddonPath
	if addonPath == "" {
		return fmt.Errorf("addon path not configured")
	}
	backupDir := filepath.Join(addonPath, "Backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to ensure backup folder: %v", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", backupDir)
	case "darwin":
		cmd = exec.Command("open", backupDir)
	default:
		cmd = exec.Command("xdg-open", backupDir)
	}
	// explorer.exe returns exit code 1 even on success (Windows quirk), so
	// we Start without checking its exit status.
	return cmd.Start()
}

// OpenLogFolder opens the manager's log directory in the user's file
// manager. Used by the Settings page so users sending bug reports can
// grab manager.log without hunting through %APPDATA%. Like
// OpenBackupFolder, MkdirAll's defensively in case the directory hasn't
// been created yet (logger.Init creates it on every startup, so this is
// only relevant on a brand-new install where startup hasn't run).
func (a *App) OpenLogFolder() error {
	logDir := filepath.Join(config.GetConfigDir(), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to ensure log folder: %v", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", logDir)
	case "darwin":
		cmd = exec.Command("open", logDir)
	default:
		cmd = exec.Command("xdg-open", logDir)
	}
	return cmd.Start()
}

// LogFromFrontend pipes a Svelte console.error / console.warn / console.log
// into the same manager.log file that the Go side writes to. Wired up
// once globally in App.svelte so any frontend error hits the log without
// per-component plumbing.
//
// `level` is the JS console level ("error", "warn", "info", or anything
// else which maps to INFO). `message` is whatever the call site formatted.
// Token redaction is handled inside logger.write so an accidentally-logged
// auth header never leaks to disk.
func (a *App) LogFromFrontend(level, message string) {
	logger.FromFrontend(level, message)
}

// GetAddonSize returns the total uncompressed size in bytes of the addon's
// files at the pinned commit. Sums blob sizes from GitHub's git/trees
// endpoint with ?recursive=1 — one API call regardless of file count.
//
// Originally tried HEAD against the zipball endpoint but GitHub's zipball is
// dynamically generated with chunked transfer encoding, so Content-Length is
// absent and the result was always "unknown."
//
// Caveat: this is the uncompressed total. The actual download (zipball) is
// compressed ~30–50% smaller. UI labels it "approximate" so users aren't
// surprised when the download bar shows fewer MB.
//
// Returns -1 (no error) when:
//   - GitHub returns truncated=true (rare; happens for >100k-file repos)
//   - The tree endpoint is unreachable for the pinned commit
// UI renders "—" in that case.
func (a *App) GetAddonSize(id string) (int64, error) {
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return 0, err
		}
		a.cachedAddons = addons
	}

	var found *registry.Addon
	for i, addon := range a.cachedAddons {
		if addon.ID == id {
			found = &a.cachedAddons[i]
			break
		}
	}
	if found == nil {
		return 0, fmt.Errorf("addon not found: %s", id)
	}

	ref := found.GitHub.Branch
	if found.GitHub.Commit != "" {
		ref = found.GitHub.Commit
	} else if found.GitHub.Tag != "" {
		ref = found.GitHub.Tag
	}

	owner, repo, err := registry.ParseRepoURL(found.GitHub.Repo)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, ref)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	if t, _ := github_auth.LoadToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1, nil
	}

	var treeResp struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&treeResp); err != nil {
		return -1, nil
	}
	if treeResp.Truncated {
		// Partial answer — better to show unknown than a misleading low number.
		return -1, nil
	}

	pathPrefix := strings.TrimSuffix(found.GitHub.Path, "/")
	if pathPrefix != "" {
		pathPrefix += "/"
	}

	var total int64
	for _, item := range treeResp.Tree {
		if item.Type != "blob" {
			continue
		}
		if pathPrefix != "" && !strings.HasPrefix(item.Path, pathPrefix) {
			continue
		}
		total += item.Size
	}
	return total, nil
}

// CheckDangerousFiles returns the cached dangerous-file scan result that
// was performed at submission time and embedded in the YAML. No live
// GitHub call — the scan happened once, server-side, at the pinned commit
// the user actually downloads. See submission-open-pr Edge Function for
// the scan logic itself.
func (a *App) CheckDangerousFiles(id string) (bool, []string, error) {
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return false, nil, err
		}
		a.cachedAddons = addons
	}

	for _, addon := range a.cachedAddons {
		if addon.ID == id {
			return addon.HasDangerousFiles, addon.DangerousFiles, nil
		}
	}
	return false, nil, fmt.Errorf("addon not found: %s", id)
}

// hasUpdate returns true if the registry entry represents a different
// version of what the user has installed. Prefers commit-SHA comparison
// (immutable, set by commit pinning) and falls back to semver-style
// version comparison for legacy addons that pre-date pinning.
func hasUpdate(installedCommit, registryCommit, installedVersion, registryVersion string) bool {
	if installedCommit != "" && registryCommit != "" {
		return installedCommit != registryCommit
	}
	return isVersionHigher(registryVersion, installedVersion)
}

// isVersionHigher compares two version strings and returns true if newVersion is higher than currentVersion
// Supports semantic versioning (e.g., "1.2.3") and simple version strings
func isVersionHigher(newVersion, currentVersion string) bool {
	// If versions are identical, no update needed
	if newVersion == currentVersion {
		return false
	}

	// Parse and compare versions
	newParts := parseVersion(newVersion)
	currentParts := parseVersion(currentVersion)

	// Compare each part (major, minor, patch, etc.)
	maxLen := len(newParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}

	for i := 0; i < maxLen; i++ {
		newVal := 0
		currentVal := 0

		if i < len(newParts) {
			newVal = newParts[i]
		}
		if i < len(currentParts) {
			currentVal = currentParts[i]
		}

		if newVal > currentVal {
			return true
		} else if newVal < currentVal {
			return false
		}
		// If equal, continue to next part
	}

	// Versions are equal
	return false
}

// parseVersion converts a version string like "1.2.3" into []int{1, 2, 3}
func parseVersion(version string) []int {
	var parts []int
	current := 0
	hasDigit := false

	for _, ch := range version {
		if ch >= '0' && ch <= '9' {
			current = current*10 + int(ch-'0')
			hasDigit = true
		} else {
			if hasDigit {
				parts = append(parts, current)
				current = 0
				hasDigit = false
			}
		}
	}

	// Add the last number if exists
	if hasDigit {
		parts = append(parts, current)
	}

	return parts
}
