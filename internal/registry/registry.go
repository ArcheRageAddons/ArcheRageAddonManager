package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"archerage-addon-manager/internal/logger"
	"archerage-addon-manager/internal/supabase"

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
// folder_name is restricted to letters and digits only (no symbols / spaces)
// because the game's loader refuses to read anything else. The substring
// "addon" is also refused for the same reason — the game treats any folder
// whose name contains it as not-an-addon and skips loading.
var (
	folderNameRe = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	githubPathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
	// Unquotes `has_dangerous_files: 'false'` left behind by an early version
	// of submission-quick-edit — yaml.v3 won't decode a quoted string into bool.
	quotedBoolRe = regexp.MustCompile(`(?m)^(has_dangerous_files:\s*)['"](true|false)['"]\s*$`)
)

// Public so the addon package can re-validate at install time.
func ValidateFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("folder_name is empty")
	}
	if !folderNameRe.MatchString(name) {
		return fmt.Errorf("folder_name %q is invalid (must be 1-64 letters or digits only — no spaces or symbols)", name)
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

// GetAllAddonsFromSupabase fetches the registry from the public.registry_addons
// mirror table — one PostgREST call, no GitHub API budget. Returns (nil, err)
// on any failure so the caller can fall back to GitHub.
func (r *RegistryClient) GetAllAddonsFromSupabase() ([]Addon, []string, error) {
	url := supabase.URL + "/rest/v1/registry_addons?select=id,addon_json,blob_sha&order=id.asc"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("supabase %s: %s", resp.Status, string(body))
	}

	var rows []struct {
		ID        string          `json:"id"`
		AddonJSON json.RawMessage `json:"addon_json"`
		BlobSHA   string          `json:"blob_sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, nil, fmt.Errorf("supabase decode: %w", err)
	}

	addons := make([]Addon, 0, len(rows))
	shas := make([]string, 0, len(rows))
	for _, row := range rows {
		var addon Addon
		if err := json.Unmarshal(row.AddonJSON, &addon); err != nil {
			logger.Warnf("registry: skipping %s — bad json from supabase: %v", row.ID, err)
			continue
		}
		addon.ID = row.ID
		if addon.GitHub.Branch == "" {
			addon.GitHub.Branch = "main"
		}
		if addon.Category == "" {
			addon.Category = "Other"
		}
		addons = append(addons, addon)
		shas = append(shas, row.BlobSHA)
	}
	return addons, shas, nil
}

func (r *RegistryClient) GetAllAddons() ([]Addon, error) {
	if addons, _, err := r.GetAllAddonsFromSupabase(); err == nil && len(addons) > 0 {
		return addons, nil
	} else if err != nil {
		logger.Warnf("registry: supabase fetch failed (%v) — falling back to GitHub", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/addons?ref=%s",
		r.registryOwner, r.registryRepo, r.registryBranch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	req.Header.Set("Cache-Control", "no-cache")
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

func (r *RegistryClient) fetchAddonFromFile(content GitHubContent) (Addon, error) {
	var addon Addon

	body, err := r.fetchYAMLBytes(content)
	if err != nil {
		return addon, err
	}

	body = quotedBoolRe.ReplaceAll(body, []byte("${1}${2}"))

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

// Raw doesn't count against the GitHub API rate limit, so unauthenticated
// users (60/hr) refresh freely. CDN-cached ~5 min after a commit; the post-
// quick-edit path uses FetchAddonFresh to bypass that.
func (r *RegistryClient) fetchYAMLBytes(content GitHubContent) ([]byte, error) {
	url := content.DownloadURL
	if url == "" {
		url = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
			r.registryOwner, r.registryRepo, r.registryBranch, content.Path)
	}
	url += fmt.Sprintf("?_=%d", time.Now().UnixNano())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("raw fetch %s: %s", content.Name, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// FetchAddonFresh re-fetches one addon via the Git Blob API (content-addressed
// by SHA, no CDN staleness). Costs 2 API calls; intended for the post-quick-
// edit flow where the caller is authenticated and needs immediate freshness.
func (r *RegistryClient) FetchAddonFresh(slug string) (Addon, string, error) {
	var addon Addon

	listingURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/addons?ref=%s",
		r.registryOwner, r.registryRepo, r.registryBranch)
	req, err := http.NewRequest("GET", listingURL, nil)
	if err != nil {
		return addon, "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	req.Header.Set("Cache-Control", "no-cache")
	r.addAuthHeader(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return addon, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return addon, "", fmt.Errorf("listing: %s", resp.Status)
	}

	var contents []GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return addon, "", err
	}

	var found *GitHubContent
	for i := range contents {
		name := strings.ToLower(contents[i].Name)
		if name == slug+".yaml" || name == slug+".yml" {
			found = &contents[i]
			break
		}
	}
	if found == nil {
		return addon, "", fmt.Errorf("addon %q not in registry listing", slug)
	}

	body, err := r.fetchBlob(found.SHA)
	if err != nil {
		return addon, "", err
	}
	body = quotedBoolRe.ReplaceAll(body, []byte("${1}${2}"))

	if err := yaml.Unmarshal(body, &addon); err != nil {
		return addon, "", fmt.Errorf("parse: %w", err)
	}
	if addon.Name == "" || addon.FolderName == "" || addon.Author == "" || addon.Version == "" || addon.GitHub.Repo == "" {
		return addon, "", fmt.Errorf("parsed YAML missing required fields")
	}
	addon.ID = slug
	if addon.GitHub.Branch == "" {
		addon.GitHub.Branch = "main"
	}
	if addon.Category == "" {
		addon.Category = "Other"
	}
	return addon, found.SHA, nil
}

func (r *RegistryClient) fetchBlob(sha string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/blobs/%s",
		r.registryOwner, r.registryRepo, sha)
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
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blob %s: %s", sha[:8], resp.Status)
	}
	var blob struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&blob); err != nil {
		return nil, err
	}
	if blob.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding %q", blob.Encoding)
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(blob.Content, "\n", ""))
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

