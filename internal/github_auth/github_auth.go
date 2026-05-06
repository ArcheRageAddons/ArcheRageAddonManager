// Package github_auth implements GitHub Device Flow OAuth for the desktop
// app, plus the small set of REST calls we need for the publish UI:
// listing the user's writable repos and fetching their identity. The
// Device Flow lets us authenticate a user without a callback server or a
// client secret — perfect for a portable desktop app.
package github_auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// httpClient is package-shared so every outbound request from this package
// has a 30s ceiling — prevents the app hanging forever on dead connections.
var httpClient = &http.Client{Timeout: 30 * time.Second}

const (
	keyringService = "ArcheRageAddonManager"
	keyringKey     = "github-token"

	// GitHub OAuth App Client ID (Device Flow enabled).
	// Public value — safe to embed in the binary.
	clientID = "Ov23libhmDh0CobLduO9"

	deviceCodeURL = "https://github.com/login/device/code"
	tokenURL      = "https://github.com/login/oauth/access_token"
	apiUserURL    = "https://api.github.com/user"
	apiReposURL   = "https://api.github.com/user/repos"

	// Empty scope keeps the consent screen minimal — by default a token can
	// list the user's own public repos via /user/repos, which is all we need
	// to verify "this user has a relationship with this repo".
	scopes = ""
)

// DeviceFlowInit is the response from GitHub's /login/device/code endpoint.
type DeviceFlowInit struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// User is a partial mirror of GitHub's /user response.
type User struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
}

// Repo is the subset of /user/repos we surface to the publish form.
type Repo struct {
	FullName      string      `json:"full_name"`
	Description   string      `json:"description"`
	DefaultBranch string      `json:"default_branch"`
	Private       bool        `json:"private"`
	Permissions   Permissions `json:"permissions"`
	HTMLURL       string      `json:"html_url"`
}

type Permissions struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage"`
	Pull     bool `json:"pull"`
}

// StartDeviceFlow initiates the Device Flow with GitHub. The returned
// DeviceFlowInit contains the user_code that the user must enter at
// VerificationURI in their browser.
func StartDeviceFlow() (*DeviceFlowInit, error) {
	form := fmt.Sprintf("client_id=%s&scope=%s", clientID, scopes)
	req, err := http.NewRequest("POST", deviceCodeURL, strings.NewReader(form))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, body)
	}

	var d DeviceFlowInit
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	if d.Interval < 5 {
		d.Interval = 5 // GitHub's recommended floor
	}
	return &d, nil
}

// PollForToken polls GitHub's token endpoint until the user completes the
// browser flow, the request expires, or the user denies access. Blocking.
func PollForToken(deviceCode string, interval, expiresIn int) (string, error) {
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	pollEvery := time.Duration(interval) * time.Second

	for time.Now().Before(deadline) {
		time.Sleep(pollEvery)

		form := fmt.Sprintf(
			"client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code",
			clientID, deviceCode,
		)
		req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form))
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var r struct {
			AccessToken      string `json:"access_token"`
			TokenType        string `json:"token_type"`
			Scope            string `json:"scope"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Interval         int    `json:"interval"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return "", fmt.Errorf("bad poll response: %s", body)
		}

		switch r.Error {
		case "":
			if r.AccessToken == "" {
				return "", fmt.Errorf("empty token")
			}
			return r.AccessToken, nil
		case "authorization_pending":
			// keep polling
		case "slow_down":
			// GitHub asks for a longer interval
			if r.Interval > 0 {
				pollEvery = time.Duration(r.Interval) * time.Second
			} else {
				pollEvery += 5 * time.Second
			}
		case "expired_token":
			return "", errors.New("login expired before completion — try again")
		case "access_denied":
			return "", errors.New("login denied")
		default:
			return "", fmt.Errorf("github: %s — %s", r.Error, r.ErrorDescription)
		}
	}
	return "", errors.New("login timed out")
}

// SaveToken persists the GitHub access token to the OS keyring.
func SaveToken(token string) error {
	return keyring.Set(keyringService, keyringKey, token)
}

// LoadToken returns the stored GitHub token, or "" if not logged in.
func LoadToken() (string, error) {
	t, err := keyring.Get(keyringService, keyringKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return t, nil
}

// ClearToken deletes the stored GitHub token.
func ClearToken() error {
	err := keyring.Delete(keyringService, keyringKey)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// IsConnected is a cheap "do we have any token" check.
func IsConnected() bool {
	t, _ := LoadToken()
	return t != ""
}

// GetUser fetches the authenticated user's profile from /user.
func GetUser() (*User, error) {
	token, err := LoadToken()
	if err != nil || token == "" {
		return nil, errors.New("not connected to GitHub")
	}

	req, _ := http.NewRequest("GET", apiUserURL, nil)
	setGitHubHeaders(req, token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		_ = ClearToken()
		return nil, errors.New("github token rejected — please sign in again")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("/user returned %d: %s", resp.StatusCode, body)
	}

	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ListWritableRepos returns repos where the authenticated user has push
// access (i.e. owners + collaborators with write or higher). Up to 100 per
// page across multiple pages.
func ListWritableRepos() ([]Repo, error) {
	token, err := LoadToken()
	if err != nil || token == "" {
		return nil, errors.New("not connected to GitHub")
	}

	var all []Repo
	page := 1
	for {
		url := fmt.Sprintf("%s?affiliation=owner,collaborator&per_page=100&sort=updated&page=%d",
			apiReposURL, page)
		req, _ := http.NewRequest("GET", url, nil)
		setGitHubHeaders(req, token)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			_ = ClearToken()
			return nil, errors.New("github token rejected — please sign in again")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("/user/repos returned %d: %s", resp.StatusCode, body)
		}

		var batch []Repo
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			if r.Permissions.Push || r.Permissions.Maintain || r.Permissions.Admin {
				all = append(all, r)
			}
		}
		if len(batch) < 100 {
			break
		}
		page++
		if page > 10 {
			break // sanity cap at 1000 repos
		}
	}
	return all, nil
}

// ResolveRef returns the commit SHA the given ref currently points at on
// the user's repo. ref is like "heads/main" or "tags/v1.0.0".
//
// Used at submission time to pin the YAML to an immutable commit so that
// users only ever download the exact bytes a maintainer reviewed.
func ResolveRef(ownerRepo, ref string) (string, error) {
	token, err := LoadToken()
	if err != nil || token == "" {
		return "", errors.New("not connected to GitHub")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/git/ref/%s", ownerRepo, ref)
	req, _ := http.NewRequest("GET", url, nil)
	setGitHubHeaders(req, token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		_ = ClearToken()
		return "", errors.New("github token rejected — please sign in again")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ref %s lookup returned %d: %s", ref, resp.StatusCode, body)
	}

	var r struct {
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.Object.SHA == "" {
		return "", fmt.Errorf("empty SHA in ref response: %s", body)
	}
	// Annotated tags have type "tag" — they reference the commit via another
	// API hop. For now we just return whatever SHA came back; for plain
	// branch heads and lightweight tags this is the commit already.
	return r.Object.SHA, nil
}

// PostJSON is a small helper to POST JSON with auth (used by app.go for
// SubmitAddon → Supabase). Kept here so all keyring/token plumbing lives
// in one place. Uses the package's default 30s timeout.
func PostJSON(url string, body interface{}, headers map[string]string) (int, []byte, error) {
	return PostJSONWithTimeout(url, body, headers, 0)
}

// PostJSONWithTimeout is PostJSON with a custom HTTP timeout. Used by
// SubmitAddon when calling submission-open-pr, because that EF does enough
// GitHub work (branch creation + dangerous-file scan walking the source
// repo + commit + PR open) that big repos can run past the default 30s
// ceiling. Pass 0 to fall back to the shared httpClient's default.
func PostJSONWithTimeout(url string, body interface{}, headers map[string]string, timeout time.Duration) (int, []byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := httpClient
	if timeout > 0 {
		// One-off client per call — cheap, and avoids mutating the shared
		// httpClient's timeout (which would race with concurrent calls).
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
}
