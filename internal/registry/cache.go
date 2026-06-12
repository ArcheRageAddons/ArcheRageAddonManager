package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"archerage-addon-manager/internal/logger"
)

const (
	// Bump to invalidate every user's on-disk cache after a breaking change to
	// the cached shape or to the parsing semantics.
	cacheVersion = 3
	// Defensive cap so a corrupted / hostile cache file can't OOM the parser.
	maxCacheBytes = 10 * 1024 * 1024
)

// ErrCacheNotModified signals the registry listing's ETag matched, so the
// in-memory cache is current and no further work is needed.
var ErrCacheNotModified = errors.New("registry: cache not modified")

// RegistryCache is the on-disk shape of the registry cache. Stored at
// %APPDATA%\ArcheRageAddonManager\registry-cache.json.
type RegistryCache struct {
	Version   int                  `json:"version"`
	ETag      string               `json:"etag"`
	FetchedAt time.Time            `json:"fetched_at"`
	Entries   []RegistryCacheEntry `json:"entries"`
}

// RegistryCacheEntry pairs the git blob SHA of the YAML file with the parsed
// Addon. The SHA lets us skip refetching unchanged YAMLs on revalidation —
// only files whose SHA in the new listing differs from the cached SHA need
// a fresh HTTP request.
type RegistryCacheEntry struct {
	BlobSHA string `json:"blob_sha"`
	Addon   Addon  `json:"addon"`
}

// LoadCache reads the on-disk cache. Returns (nil, nil) when no cache exists
// or when the cache is unusable (wrong version, malformed, oversized) — the
// caller should fall through to a fresh fetch.
func LoadCache(filePath string) (*RegistryCache, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Size() > maxCacheBytes {
		logger.Warnf("registry: cache file %s is %d bytes (max %d) — ignoring", filePath, info.Size(), maxCacheBytes)
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var c RegistryCache
	if err := json.Unmarshal(data, &c); err != nil {
		logger.Warnf("registry: cache file %s is malformed — ignoring", filePath)
		return nil, nil
	}
	if c.Version != cacheVersion {
		logger.Warnf("registry: cache file %s is version %d, expected %d — ignoring", filePath, c.Version, cacheVersion)
		return nil, nil
	}
	return &c, nil
}

// SaveCache atomically writes the cache to disk via tmp + rename so a crash
// mid-write can never leave an empty file.
func SaveCache(filePath string, cache *RegistryCache) error {
	cache.Version = cacheVersion
	if cache.FetchedAt.IsZero() {
		cache.FetchedAt = time.Now()
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filePath)
}

// GetAllAddonsConditional refreshes the registry. Tries the Supabase mirror
// first (one PostgREST call, no GitHub API budget). Falls back to GitHub's
// contents API + conditional ETag when Supabase is unreachable.
func (r *RegistryClient) GetAllAddonsConditional(prevETag string, prevEntries []RegistryCacheEntry) (string, []RegistryCacheEntry, error) {
	if addons, shas, err := r.GetAllAddonsFromSupabase(); err == nil && len(addons) > 0 {
		entries := make([]RegistryCacheEntry, len(addons))
		for i := range addons {
			entries[i] = RegistryCacheEntry{BlobSHA: shas[i], Addon: addons[i]}
		}
		if sameEntries(prevEntries, entries) {
			return prevETag, prevEntries, ErrCacheNotModified
		}
		// Empty ETag — Supabase path doesn't use ETag-based revalidation.
		// Next refresh will re-fetch unconditionally; that's cheap.
		return "", entries, nil
	} else if err != nil {
		logger.Warnf("registry: supabase fetch failed (%v) — falling back to GitHub", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/addons?ref=%s",
		r.registryOwner, r.registryRepo, r.registryBranch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	req.Header.Set("Cache-Control", "no-cache")
	r.addAuthHeader(req)
	if prevETag != "" {
		req.Header.Set("If-None-Match", prevETag)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return prevETag, prevEntries, ErrCacheNotModified
	}
	if resp.StatusCode == 404 {
		return "", nil, fmt.Errorf("registry not found: %s/%s", r.registryOwner, r.registryRepo)
	}
	if resp.StatusCode == 401 {
		return "", nil, fmt.Errorf("authentication failed - GitHub token rejected")
	}
	if resp.StatusCode == 403 {
		body, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(body)), "rate limit") {
			if r.githubToken == "" {
				return "", nil, fmt.Errorf("GitHub rate limit exceeded (60/hr unauthenticated). Sign in with GitHub for 5000/hr.")
			}
			return "", nil, fmt.Errorf("GitHub rate limit exceeded (5000/hr). Wait an hour or check token scopes.")
		}
		return "", nil, fmt.Errorf("forbidden by GitHub: %s", string(body))
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return "", nil, fmt.Errorf("failed to parse registry contents: %v", err)
	}

	prevByID := make(map[string]RegistryCacheEntry, len(prevEntries))
	for _, e := range prevEntries {
		prevByID[e.Addon.ID] = e
	}

	var entries []RegistryCacheEntry
	for _, content := range contents {
		if content.Type != "file" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(content.Name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		id := strings.TrimSuffix(content.Name, filepath.Ext(content.Name))

		if cached, ok := prevByID[id]; ok && cached.BlobSHA == content.SHA {
			entries = append(entries, cached)
			continue
		}

		addon, err := r.fetchAddonFromFile(content)
		if err != nil {
			logger.Warnf("registry: failed to parse %s: %v", content.Name, err)
			continue
		}
		addon.ID = id
		entries = append(entries, RegistryCacheEntry{
			BlobSHA: content.SHA,
			Addon:   addon,
		})
	}

	newETag := resp.Header.Get("ETag")
	return newETag, entries, nil
}

// EntriesToAddons unwraps cache entries into the bare Addon list the rest
// of the codebase consumes.
func EntriesToAddons(entries []RegistryCacheEntry) []Addon {
	out := make([]Addon, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Addon)
	}
	return out
}

// sameEntries returns true when both slices contain the same set of (id,
// blob_sha) pairs — i.e. nothing changed since the last fetch.
func sameEntries(a, b []RegistryCacheEntry) bool {
	if len(a) != len(b) {
		return false
	}
	prev := make(map[string]string, len(a))
	for _, e := range a {
		prev[e.Addon.ID] = e.BlobSHA
	}
	for _, e := range b {
		if prev[e.Addon.ID] != e.BlobSHA {
			return false
		}
	}
	return true
}
