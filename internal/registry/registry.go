package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"archerage-addon-manager/internal/logger"

	"gopkg.in/yaml.v3"
)

// Default registry repository settings
const (
	DefaultRegistryOwner  = "ArcheRageAddons"
	DefaultRegistryRepo   = "addons"
	DefaultRegistryBranch = "main"
)

// Audit #2 + #3: folder_name lands as a directory under cfg.AddonPath, and
// github.path is forwarded to the GitHub zipball API + used as a subfolder
// filter during extraction. Both come from registry YAML which is reviewed,
// but we still treat them as untrusted (legacy YAMLs predating P4-4 review,
// or admin-bypass merges) and reject anything that could escape the install
// dir or the source repo subtree.
//
//   folder_name: a single path component. Must be alphanumeric / dot / dash
//                / underscore, no separators, no leading dots (no hidden
//                dirs), reasonable length. Mirrors the slug regex but
//                permits dots so existing addon folder names like
//                "Auto.RoleSetter" continue to work.
//   github.path: zero or more path components. No "..", no leading or
//                trailing slash, no absolute paths, no Windows drive
//                letters, no backslashes (forward slash only — GitHub
//                paths use forward slash regardless of host OS).
var (
	folderNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	githubPathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
)

// ValidateFolderName enforces the folder_name shape — package-public so the
// addon manager can re-validate at install time (defence in depth, in case
// installed.json was tampered with between fetch and use).
func ValidateFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("folder_name is empty")
	}
	if !folderNameRe.MatchString(name) {
		return fmt.Errorf("folder_name %q is invalid (allowed: alnum/dot/dash/underscore, must start with alnum, max 64 chars)", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("folder_name %q is reserved", name)
	}
	return nil
}

// ValidateGithubPath enforces the github.path shape: forward-slash separated
// path components, no traversal, no absolute paths.
func ValidateGithubPath(p string) error {
	if p == "" {
		return nil // empty is fine — means "repo root"
	}
	if !githubPathRe.MatchString(p) {
		return fmt.Errorf("github.path %q contains invalid characters", p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("github.path %q contains traversal", p)
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return fmt.Errorf("github.path %q must not start or end with /", p)
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("github.path %q has an invalid segment", p)
		}
	}
	return nil
}

// Addon represents a single addon entry from the YAML registry.
//
// HasDangerousFiles + DangerousFiles are populated AT SUBMISSION TIME by
// the submission-open-pr Edge Function (which scans the source repo at the
// pinned commit using the GitHub App's authenticated rate limit) and
// embedded into the YAML before it lands in the registry. The desktop
// manager just reads them from the cached registry data — no live scan,
// no per-user rate limiting, no fail-open silent miss.
type Addon struct {
	ID                string   `yaml:"-" json:"id"` // Generated from filename
	Name              string   `yaml:"name" json:"name"`
	FolderName        string   `yaml:"folder_name,omitempty" json:"folder_name,omitempty"` // Optional: Override folder name
	Icon              string   `yaml:"icon,omitempty" json:"icon,omitempty"`               // Optional: URL to custom icon image
	Description       string   `yaml:"description" json:"description"`
	Author            string   `yaml:"author" json:"author"`
	Version           string   `yaml:"version" json:"version"`
	Category          string   `yaml:"category" json:"category"`
	Keywords          []string `yaml:"keywords" json:"keywords"`
	Dependencies      []string `yaml:"dependencies" json:"dependencies"`
	HasDangerousFiles bool     `yaml:"has_dangerous_files,omitempty" json:"has_dangerous_files"`
	DangerousFiles    []string `yaml:"dangerous_files,omitempty" json:"dangerous_files,omitempty"`
	// OverlayOf names another addon's ID. When set, this addon installs
	// INTO the target's existing folder rather than its own — the install
	// flow skips the wipe-and-backup step and extracts on top of the base.
	// Used for patch addons that ship small file overrides on a heavy
	// base (e.g. UI overhaul base + frequent patcher). The base must
	// already be installed; the manager refuses to install an overlay
	// otherwise. Uninstalling the base also drops the overlay's tracking
	// row (the folder is removed in either case).
	OverlayOf string `yaml:"overlay_of,omitempty" json:"overlay_of,omitempty"`
	// Trust-signal fields embedded at submission time by the EF.
	SubmitterDiscord string `yaml:"submitter_discord,omitempty" json:"submitter_discord,omitempty"`
	SubmitterGithub  string `yaml:"submitter_github,omitempty" json:"submitter_github,omitempty"`
	SubmittedAt      string `yaml:"submitted_at,omitempty" json:"submitted_at,omitempty"`
	GitHub           GitHub `yaml:"github" json:"github"`
}

// GitHub contains the repository information for downloading the addon.
// Commit is the immutable SHA the registry pinned at submission/approval
// time — the manager downloads from this when present, ignoring branch.
// branch / tag are kept around for diagnostic display only.
type GitHub struct {
	Repo   string `yaml:"repo" json:"repo"`               // Format: "owner/repo"
	Branch string `yaml:"branch" json:"branch"`           // e.g. "main", "master"
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"` // immutable SHA — preferred download ref
	Path   string `yaml:"path" json:"path"`               // Subfolder path, empty for root
	Tag    string `yaml:"tag" json:"tag"`                 // Optional release tag
}

// RegistryClient handles fetching addon definitions from the GitHub registry
type RegistryClient struct {
	client         *http.Client
	registryOwner  string
	registryRepo   string
	registryBranch string
	githubToken    string
}

// GitHubContent represents the GitHub API response for file contents
type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
	DownloadURL string `json:"download_url"`
	SHA         string `json:"sha"`
}

// NewRegistryClient creates a new registry client with default settings
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		client:         &http.Client{Timeout: 30 * time.Second},
		registryOwner:  DefaultRegistryOwner,
		registryRepo:   DefaultRegistryRepo,
		registryBranch: DefaultRegistryBranch,
	}
}

// SetToken sets the GitHub personal access token for private repo access
func (r *RegistryClient) SetToken(token string) {
	r.githubToken = token
}

// addAuthHeader adds the Authorization header if a token is set
func (r *RegistryClient) addAuthHeader(req *http.Request) {
	if r.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.githubToken)
	}
}

// GetAllAddons fetches all addon definitions from the registry
func (r *RegistryClient) GetAllAddons() ([]Addon, error) {
	// Get the list of YAML files in the addons directory
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/addons?ref=%s",
		r.registryOwner, r.registryRepo, r.registryBranch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	r.addAuthHeader(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("registry not found: %s/%s", r.registryOwner, r.registryRepo)
	}

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - GitHub token rejected")
	}
	if resp.StatusCode == 403 {
		body, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(body)), "rate limit") {
			if r.githubToken == "" {
				return nil, fmt.Errorf("GitHub rate limit exceeded (60/hr unauthenticated). Sign in with GitHub for 5000/hr.")
			}
			return nil, fmt.Errorf("GitHub rate limit exceeded (5000/hr). Wait an hour or check token scopes.")
		}
		return nil, fmt.Errorf("forbidden by GitHub: %s", string(body))
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, fmt.Errorf("failed to parse registry contents: %v", err)
	}

	var addons []Addon
	for _, content := range contents {
		// Only process .yaml and .yml files
		if content.Type != "file" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(content.Name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		addon, err := r.fetchAddonFromFile(content)
		if err != nil {
			// Log error but continue with other addons
			logger.Warnf("registry: failed to parse %s: %v", content.Name, err)
			continue
		}

		// Generate ID from filename (without extension)
		addon.ID = strings.TrimSuffix(content.Name, filepath.Ext(content.Name))
		addons = append(addons, addon)
	}

	return addons, nil
}

// fetchAddonFromFile fetches and parses a single addon YAML file.
//
// Uses raw.githubusercontent.com unconditionally — for public repos it
// works without auth and crucially does NOT count against the GitHub API
// rate limit (separate hostname, separate / much higher budget). Means a
// Browse refresh costs 1 API call (the listing) regardless of how many
// addons live in the registry.
func (r *RegistryClient) fetchAddonFromFile(content GitHubContent) (Addon, error) {
	var addon Addon

	url := content.DownloadURL
	if url == "" {
		url = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
			r.registryOwner, r.registryRepo, r.registryBranch, content.Path)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return addon, err
	}
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	// No auth header on raw-content fetches — they don't need it for public
	// repos and using one would (a) needlessly tie this to the user's token
	// and (b) potentially burn against an unrelated per-token limit.

	resp, err := r.client.Do(req)
	if err != nil {
		return addon, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return addon, fmt.Errorf("failed to download %s: %s", content.Name, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return addon, err
	}

	if err := yaml.Unmarshal(body, &addon); err != nil {
		return addon, fmt.Errorf("invalid YAML: %v", err)
	}

	// Validate required fields
	if addon.Name == "" {
		return addon, fmt.Errorf("missing required field: name")
	}
	if addon.FolderName == "" {
		return addon, fmt.Errorf("missing required field: folder_name")
	}
	if addon.Author == "" {
		return addon, fmt.Errorf("missing required field: author")
	}
	if addon.Version == "" {
		return addon, fmt.Errorf("missing required field: version")
	}
	if addon.GitHub.Repo == "" {
		return addon, fmt.Errorf("missing required field: github.repo")
	}

	// Audit #2 / #3: reject YAMLs whose folder_name or github.path could
	// escape the install dir / source subtree. Logged + skipped at the
	// caller so one bad entry doesn't break the whole registry load.
	if err := ValidateFolderName(addon.FolderName); err != nil {
		return addon, err
	}
	if err := ValidateGithubPath(addon.GitHub.Path); err != nil {
		return addon, err
	}
	// overlay_of is an addon id (registry filename without .yaml). Same
	// shape as folder_name — reuse the validator.
	if addon.OverlayOf != "" {
		if err := ValidateFolderName(addon.OverlayOf); err != nil {
			return addon, fmt.Errorf("overlay_of: %v", err)
		}
	}

	// Optional fields with sensible defaults
	if addon.GitHub.Branch == "" {
		addon.GitHub.Branch = "main"
	}
	if addon.Category == "" {
		addon.Category = "Other"
	}

	return addon, nil
}

// ParseRepoURL extracts owner and repo from a GitHub URL or owner/repo format
func ParseRepoURL(repoURL string) (owner, repo string, err error) {
	// Handle full URLs
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "github.com/")
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Handle paths like /owner/repo/...
	repoURL = strings.TrimPrefix(repoURL, "/")

	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repo format: %s", repoURL)
	}

	return parts[0], parts[1], nil
}

// GetLatestCommit fetches the latest commit hash for an addon's source
func (r *RegistryClient) GetLatestCommit(addon *Addon) (string, error) {
	owner, repo, err := ParseRepoURL(addon.GitHub.Repo)
	if err != nil {
		return "", err
	}

	ref := addon.GitHub.Branch
	if addon.GitHub.Tag != "" {
		ref = addon.GitHub.Tag
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?sha=%s&path=%s&per_page=1",
		owner, repo, ref, addon.GitHub.Path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	r.addAuthHeader(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to get commits")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &commits); err != nil {
		return "", err
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found")
	}

	return commits[0].SHA, nil
}

