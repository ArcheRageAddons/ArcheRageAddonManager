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

const (
	DefaultRegistryOwner  = "ArcheRageAddons"
	DefaultRegistryRepo   = "addons"
	DefaultRegistryBranch = "main"
)

// folder_name lands as a directory under cfg.AddonPath; github.path is fed
// to the zipball subtree filter at extract time. Both are treated as
// untrusted to defeat traversal attempts.
//
// folder_name is restricted to letters only (no digits / symbols / spaces)
// because the game's loader refuses to read anything else. The substring
// "addon" is also refused for the same reason — the game treats any folder
// whose name contains it as not-an-addon and skips loading.
var (
	folderNameRe = regexp.MustCompile(`^[A-Za-z]{1,64}$`)
	githubPathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
)

// Public so the addon package can re-validate at install time.
func ValidateFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("folder_name is empty")
	}
	if !folderNameRe.MatchString(name) {
		return fmt.Errorf("folder_name %q is invalid (must be 1-64 letters only — no digits, spaces, or symbols)", name)
	}
	if strings.Contains(strings.ToLower(name), "addon") {
		return fmt.Errorf("folder_name %q cannot contain \"addon\" — the game refuses to load folders with that substring", name)
	}
	return nil
}

func ValidateGithubPath(p string) error {
	if p == "" {
		return nil
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

// HasDangerousFiles, DangerousFiles, OverlayOf, Submitter*, and SubmittedAt
// are populated by the submission-open-pr Edge Function and baked into the
// YAML — the desktop just reads them.
type Addon struct {
	ID                string   `yaml:"-" json:"id"`
	Name              string   `yaml:"name" json:"name"`
	FolderName        string   `yaml:"folder_name,omitempty" json:"folder_name,omitempty"`
	Icon              string   `yaml:"icon,omitempty" json:"icon,omitempty"`
	Description       string   `yaml:"description" json:"description"`
	Author            string   `yaml:"author" json:"author"`
	Version           string   `yaml:"version" json:"version"`
	Category          string   `yaml:"category" json:"category"`
	Keywords          []string `yaml:"keywords" json:"keywords"`
	Dependencies      []string `yaml:"dependencies" json:"dependencies"`
	HasDangerousFiles bool     `yaml:"has_dangerous_files,omitempty" json:"has_dangerous_files"`
	DangerousFiles    []string `yaml:"dangerous_files,omitempty" json:"dangerous_files,omitempty"`
	OverlayOf         string   `yaml:"overlay_of,omitempty" json:"overlay_of,omitempty"`
	SubmitterDiscord  string   `yaml:"submitter_discord,omitempty" json:"submitter_discord,omitempty"`
	SubmitterGithub   string   `yaml:"submitter_github,omitempty" json:"submitter_github,omitempty"`
	SubmittedAt       string   `yaml:"submitted_at,omitempty" json:"submitted_at,omitempty"`
	GitHub            GitHub   `yaml:"github" json:"github"`
}

// Commit is the immutable pin set at submission time and is what the manager
// actually downloads. Branch + Tag are diagnostic only.
type GitHub struct {
	Repo   string `yaml:"repo" json:"repo"`
	Branch string `yaml:"branch" json:"branch"`
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"`
	Path   string `yaml:"path" json:"path"`
	Tag    string `yaml:"tag" json:"tag"`
}

type RegistryClient struct {
	client         *http.Client
	registryOwner  string
	registryRepo   string
	registryBranch string
	githubToken    string
}

type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
	DownloadURL string `json:"download_url"`
	SHA         string `json:"sha"`
}

func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		client:         &http.Client{Timeout: 30 * time.Second},
		registryOwner:  DefaultRegistryOwner,
		registryRepo:   DefaultRegistryRepo,
		registryBranch: DefaultRegistryBranch,
	}
}

func (r *RegistryClient) SetToken(token string) {
	r.githubToken = token
}

func (r *RegistryClient) addAuthHeader(req *http.Request) {
	if r.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.githubToken)
	}
}

func (r *RegistryClient) GetAllAddons() ([]Addon, error) {
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
		if content.Type != "file" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(content.Name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		addon, err := r.fetchAddonFromFile(content)
		if err != nil {
			logger.Warnf("registry: failed to parse %s: %v", content.Name, err)
			continue
		}

		addon.ID = strings.TrimSuffix(content.Name, filepath.Ext(content.Name))
		addons = append(addons, addon)
	}

	return addons, nil
}

// raw.githubusercontent.com doesn't count against the GitHub API rate limit,
// so a Browse refresh costs 1 API call (the listing) regardless of the
// number of addons.
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

	if err := ValidateFolderName(addon.FolderName); err != nil {
		return addon, err
	}
	if err := ValidateGithubPath(addon.GitHub.Path); err != nil {
		return addon, err
	}
	if addon.OverlayOf != "" {
		if err := ValidateFolderName(addon.OverlayOf); err != nil {
			return addon, fmt.Errorf("overlay_of: %v", err)
		}
	}

	if addon.GitHub.Branch == "" {
		addon.GitHub.Branch = "main"
	}
	if addon.Category == "" {
		addon.Category = "Other"
	}

	return addon, nil
}

func ParseRepoURL(repoURL string) (owner, repo string, err error) {
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "github.com/")
	repoURL = strings.TrimSuffix(repoURL, ".git")
	repoURL = strings.TrimPrefix(repoURL, "/")

	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repo format: %s", repoURL)
	}

	return parts[0], parts[1], nil
}

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

