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

var httpClient = &http.Client{Timeout: 30 * time.Second}

// 5-minute ceiling — slow connections legitimately need this long to pull ~15MB.
var updateHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// Defends against injection-shaped input being interpolated into PostgREST URL filters.
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
	cachedStats     map[string]addonStatRow
	cachedMyRatings map[string]int
	// Registry-cache state — kept alongside cachedAddons so background
	// revalidation can ship `If-None-Match` and skip refetching unchanged
	// YAMLs based on per-blob SHA.
	cachedRegistryEntries []registry.RegistryCacheEntry
	cachedRegistryETag    string
}

// Render Name in the UI; pass ID to install / lookup calls. Name falls back
// to ID when the dep isn't in the cached registry yet.
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
	// BaseInstalled gates the install button for overlay addons.
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

	// Initialise logging first so anything that fails below shows up in it.
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

	// Authenticated registry calls get GitHub's 5000/hr rate limit (vs 60/hr).
	a.syncRegistryToken()

	// Hydrate cachedAddons from the on-disk cache so Browse renders instantly.
	a.loadRegistryCacheFromDisk()
	go a.refreshRegistryInBackground()

	go cleanupOldBinary()
	go a.updateCheckLoop()

	logger.Info("startup complete")
}

func (a *App) registryCachePath() string {
	return filepath.Join(config.GetConfigDir(), "registry-cache.json")
}

func (a *App) loadRegistryCacheFromDisk() {
	cache, err := registry.LoadCache(a.registryCachePath())
	if err != nil {
		logger.Warnf("registry: load cache failed: %v", err)
		return
	}
	if cache == nil {
		return
	}
	a.cachedRegistryEntries = cache.Entries
	a.cachedRegistryETag = cache.ETag
	a.cachedAddons = registry.EntriesToAddons(cache.Entries)
	logger.Infof("registry: loaded %d addons from cache (etag=%q)", len(cache.Entries), cache.ETag)
}

// Conditional revalidation. If the listing's ETag matches, this is a single
// small request and we keep using cached data. Otherwise we refetch only the
// YAMLs whose blob SHA changed.
func (a *App) refreshRegistryInBackground() {
	newETag, entries, err := a.registryClient.GetAllAddonsConditional(a.cachedRegistryETag, a.cachedRegistryEntries)
	if errors.Is(err, registry.ErrCacheNotModified) {
		return
	}
	if err != nil {
		logger.Warnf("registry: background refresh failed: %v", err)
		return
	}

	a.cachedRegistryEntries = entries
	a.cachedRegistryETag = newETag
	a.cachedAddons = registry.EntriesToAddons(entries)

	if err := registry.SaveCache(a.registryCachePath(), &registry.RegistryCache{
		ETag:      newETag,
		FetchedAt: time.Now(),
		Entries:   entries,
	}); err != nil {
		logger.Warnf("registry: save cache failed: %v", err)
	}

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "registry:refreshed")
	}
	logger.Infof("registry: background refresh updated cache (%d addons, etag=%q)", len(entries), newETag)
}

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
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return nil, err
		}
		a.cachedAddons = addons
	}

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

		if item.IsInstalled {
			installed := a.addonManager.GetInstalledAddon(addon.ID)
			if installed != nil {
				if hasUpdate(installed.GithubCommitHash, addon.GitHub.Commit, installed.Version, addon.Version) {
					item.HasUpdate = true
				}
				item.GithubCommitHash = installed.GithubCommitHash
			}
		}

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
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return nil, err
		}
		a.cachedAddons = addons
	}

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

	if s, ok := a.fetchAddonStats()[found.ID]; ok {
		item.DownloadCount = s.DownloadCount
		item.RatingCount = s.RatingCount
		if s.RatingCount > 0 {
			item.RatingAvg = float64(s.RatingSum) / float64(s.RatingCount)
		}
	}

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
	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			return err
		}
		a.cachedAddons = addons
	}

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

	// Failing fast here avoids spinning up the install goroutine + emitting
	// progress events for an install we'll immediately reject inside addon.Manager.
	if found.OverlayOf != "" && !a.addonManager.IsInstalled(found.OverlayOf) {
		return fmt.Errorf("install %q first — this addon overlays on top of it", found.OverlayOf)
	}

	commitHash := found.GitHub.Commit
	if commitHash == "" {
		var err error
		commitHash, err = a.registryClient.GetLatestCommit(found)
		if err != nil {
			return fmt.Errorf("failed to get commit hash: %v", err)
		}
	}

	repoURL := "https://github.com/" + found.GitHub.Repo

	go func() {
		logger.Infof("install start: %s v%s from %s@%s (path=%q overlay=%q)",
			found.ID, found.Version, found.GitHub.Repo, commitHash, found.GitHub.Path, found.OverlayOf)

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
			wailsRuntime.EventsEmit(a.ctx, "download:complete", DownloadResult{
				Success: false,
				Error:   err.Error(),
				AddonID: found.ID,
			})
			return
		}
		logger.Infof("install complete: %s v%s", found.ID, found.Version)

		wailsRuntime.EventsEmit(a.ctx, "download:progress", DownloadProgress{
			Current: 100,
			Total:   100,
			Message: "Installation complete!",
		})

		go func(id string) {
			a.incrementDownloadCount(id)
			a.cachedStats = nil
		}(found.ID)

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

	if a.cachedAddons == nil {
		addons, err := a.registryClient.GetAllAddons()
		if err != nil {
			// Continue without update info rather than failing the whole call.
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

func (a *App) IsFirstRun() bool {
	return config.IsFirstRun()
}

func (a *App) DetectAddonPaths() []config.AddonPathCandidate {
	return config.DetectAddonPaths()
}

func (a *App) ConfirmAddonPath() error {
	return config.MarkSetupComplete()
}

type ReleaseInfo struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	SourceURL   string `json:"source_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	// Empty when the release has no .exe asset; UI falls back to opening the release page.
	AssetURL  string `json:"asset_url,omitempty"`
	AssetSize int64  `json:"asset_size,omitempty"`
}

func (a *App) GetVersion() string { return Version }

var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// nil with no error means "up to date" or "can't tell". Skipped in dev mode.
func (a *App) CheckForUpdate() (*ReleaseInfo, error) {
	if !semverRe.MatchString(Version) {
		return nil, nil
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
		req.Header.Set("Authorization", "Bearer "+t)
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

func (a *App) updateCheckLoop() {
	if !semverRe.MatchString(Version) {
		return
	}
	// Stagger so we don't race with login on startup.
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

// Windows-safe self-update via the rename trick: a running .exe can be
// renamed but not deleted or overwritten, so we rename current → "<exe>.old",
// drop the new binary at the original path, spawn it, and exit.
// downloadURL must come from CheckForUpdate's AssetURL — we host-check it
// for defence in depth.
func (a *App) InstallUpdate(downloadURL string) error {
	logger.Infof("self-update start: %s", downloadURL)
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid update URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("update URL must be https")
	}
	// github.com 302s to objects.githubusercontent.com for release assets.
	// raw.* / gist.* are user-controllable so they're not on the list.
	if parsed.Host != "github.com" && parsed.Host != "objects.githubusercontent.com" {
		return fmt.Errorf("update URL host not allowed: %s", parsed.Host)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("can't locate running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	// OneDrive's File-on-Demand sync makes the rename trick race-prone.
	if isOneDrivePath(exePath) {
		return fmt.Errorf("the manager is in a OneDrive folder (%s) — auto-update can't safely run there. Move the .exe to a non-synced folder (e.g. %%LOCALAPPDATA%%\\Programs\\ArcheRageAddonManager) and try again", exePath)
	}

	dir := filepath.Dir(exePath)
	updatePath := filepath.Join(dir, "update.exe")
	oldPath := exePath + ".old"

	_ = os.Remove(updatePath)
	_ = os.Remove(oldPath)

	emit := func(current, total int64, msg string) {
		wailsRuntime.EventsEmit(a.ctx, "update:download:progress", map[string]interface{}{
			"current": current,
			"total":   total,
			"message": msg,
		})
	}

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

	// A self-update .exe should be ~15 MB; anything close to the cap is suspicious.
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

	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(updatePath)
		return fmt.Errorf("can't move running binary aside: %w", err)
	}
	if err := os.Rename(updatePath, exePath); err != nil {
		_ = os.Rename(oldPath, exePath) // rollback
		_ = os.Remove(updatePath)
		return fmt.Errorf("can't move new binary into place: %w", err)
	}

	cmd := exec.Command(exePath)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("can't launch new binary: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	wailsRuntime.EventsEmit(a.ctx, "update:download:complete", map[string]interface{}{
		"success": true,
	})

	// Lets the UI render the "complete" state before the window closes.
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

// Heuristic — OneDrive paths vary per user but reliably contain "onedrive".
func isOneDrivePath(p string) bool {
	lower := strings.ToLower(p)
	return strings.Contains(lower, "onedrive") || strings.Contains(lower, "\\onedrive\\")
}

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

func (a *App) Logout() error {
	if err := auth.Logout(); err != nil {
		logger.Errorf("logout failed: %v", err)
		return err
	}
	logger.Info("discord logout")
	wailsRuntime.EventsEmit(a.ctx, "auth:changed", nil)
	return nil
}

func (a *App) GetCurrentUser() (*auth.User, error) {
	return auth.CurrentUser()
}

// Polling for the eventual token runs in the background; the frontend
// listens for the "github:auth:done" event for success / failure.
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

func (a *App) GetGitHubUser() (*github_auth.User, error) {
	if !github_auth.IsConnected() {
		return nil, nil
	}
	return github_auth.GetUser()
}

func (a *App) LogoutGitHub() error {
	if err := github_auth.ClearToken(); err != nil {
		return err
	}
	a.syncRegistryToken()
	return nil
}

func (a *App) ListMyRepos() ([]github_auth.Repo, error) {
	return github_auth.ListWritableRepos()
}

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
	OverlayOf    string   `json:"overlay_of"`
}

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

type SubmitAddonResult struct {
	SubmissionID string `json:"submission_id"`
	PRNumber     int    `json:"pr_number,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
}

// The submission-open-pr Edge Function inserts the row + opens the PR
// atomically and rolls back on any failure, so we never leak orphan state.
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

	// Pin the YAML to a specific commit SHA so users always download the
	// exact bytes the maintainer reviewed.
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

	// EF uses this (zero-scope) token to verify push access on the claimed repo.
	ghToken, _ := github_auth.LoadToken()
	if ghToken == "" {
		return nil, fmt.Errorf("connect GitHub before submitting")
	}

	// 5-minute ceiling — big-repo submissions (deep dangerous-file scan +
	// PR creation) can run past the default 30 s. Well under Supabase's
	// 400 s platform cap.
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

// Explicit submitted_by filter — RLS would normally scope this to the
// caller's rows, but the `admins read all subs` policy lets admins see
// every row in the table. We don't want the My Addons page to balloon
// to "every submission ever" for maintainers.
func (a *App) GetMySubmissions() ([]SubmissionRow, error) {
	tokens, err := a.loadSupabaseTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, nil
	}
	if tokens.User.ID == "" {
		return nil, fmt.Errorf("session has no user id")
	}

	endpoint := supabase.URL + "/rest/v1/submissions?submitted_by=eq." + url.QueryEscape(tokens.User.ID) +
		"&select=id,addon_slug,yaml_content,github_repo,github_path,status,decision_reason,created_at,decided_at,github_pr_number,github_pr_url,github_pr_branch&order=created_at.desc"
	req, err := http.NewRequest("GET", endpoint, nil)
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

// Bumps the patch component as a starting suggestion (1.0.0 → 1.0.1).
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

// Read-only — ban/unban and admin grants are done in SQL only, by design.
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

// RLS-gated: admins see all rows, non-admins get an empty array.
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

// RLS gates per-row: users can only delete their own non-pending rows.
// Unauthorised IDs are silently skipped.
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

// Filters to rows with an actual github_pr_number — orphan rows whose EF died
// mid-flight have no PR for the approve/deny EFs to act on, so they're hidden
// from admins. Users can still withdraw orphans from MyAddons; a pg_cron job
// also cleans them up after 5 minutes.
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

func (a *App) ApproveSubmission(id string) error {
	return a.callDecisionEdgeFn("submission-approve", id, "")
}

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

// On ErrSessionExpired, emit auth:changed so the frontend logs out immediately.
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
	// Stats and ratings always re-fetch on manual Refresh; the registry uses
	// the conditional path so a 304 from GitHub is essentially free.
	a.cachedStats = nil
	a.cachedMyRatings = nil

	newETag, entries, err := a.registryClient.GetAllAddonsConditional(a.cachedRegistryETag, a.cachedRegistryEntries)
	if errors.Is(err, registry.ErrCacheNotModified) {
		return nil
	}
	if err != nil {
		return err
	}

	a.cachedRegistryEntries = entries
	a.cachedRegistryETag = newETag
	a.cachedAddons = registry.EntriesToAddons(entries)

	if err := registry.SaveCache(a.registryCachePath(), &registry.RegistryCache{
		ETag:      newETag,
		FetchedAt: time.Now(),
		Entries:   entries,
	}); err != nil {
		logger.Warnf("registry: save cache failed: %v", err)
	}
	return nil
}

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
	// url.Parse keeps Host=github.com on "https://github.com/../evil.com/path"
	// but the browser normalises and visits evil.com. Use decoded Path so that
	// percent-encoded ".." (%2E%2E) is caught by Clean too.
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

type addonStatRow struct {
	AddonSlug     string `json:"addon_slug"`
	DownloadCount int    `json:"download_count"`
	RatingSum     int    `json:"rating_sum"`
	RatingCount   int    `json:"rating_count"`
}

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
	if tokens, err := a.loadSupabaseTokens(); err == nil && tokens != nil {
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

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

func (a *App) GetMyRating(slug string) (int, error) {
	ratings := a.fetchMyRatings()
	if ratings == nil {
		return 0, nil
	}
	return ratings[slug], nil
}

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
	return cmd.Start()
}

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

func (a *App) LogFromFrontend(level, message string) {
	logger.FromFrontend(level, message)
}

// Sums blob sizes from git/trees?recursive=1 — uncompressed (zipball is
// ~30-50% smaller). Returns -1 when the tree is truncated or unreachable.
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

// Prefer commit-SHA equality (immutable); fall back to semver for unpinned legacy entries.
func hasUpdate(installedCommit, registryCommit, installedVersion, registryVersion string) bool {
	if installedCommit != "" && registryCommit != "" {
		return installedCommit != registryCommit
	}
	return isVersionHigher(registryVersion, installedVersion)
}

func isVersionHigher(newVersion, currentVersion string) bool {
	if newVersion == currentVersion {
		return false
	}

	newParts := parseVersion(newVersion)
	currentParts := parseVersion(currentVersion)

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
	}

	return false
}

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

	if hasDigit {
		parts = append(parts, current)
	}

	return parts
}
