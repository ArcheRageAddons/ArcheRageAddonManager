// Package github_auth implements GitHub Device Flow OAuth — no callback
// server or client secret required, ideal for a portable desktop app.
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

var httpClient = &http.Client{Timeout: 30 * time.Second}

const (
	keyringService = "ArcheRageAddonManager"
	keyringKey     = "github-token"

	// Public value — safe to embed.
	clientID = "Ov23libhmDh0CobLduO9"

	deviceCodeURL = "https://github.com/login/device/code"
	tokenURL      = "https://github.com/login/oauth/access_token"
	apiUserURL    = "https://api.github.com/user"
	apiReposURL   = "https://api.github.com/user/repos"

	// Empty scope keeps consent minimal; the resulting token can still
	// hit /user and /user/repos which is all we need.
	scopes = ""
)

type DeviceFlowInit struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type User struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
}

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

// Blocks until the user completes the browser flow, denies, or the
// request expires.
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
		case "slow_down":
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

func SaveToken(token string) error {
	return keyring.Set(keyringService, keyringKey, token)
}

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

func ClearToken() error {
	err := keyring.Delete(keyringService, keyringKey)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func IsConnected() bool {
	t, _ := LoadToken()
	return t != ""
}

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

// Returns repos where the authenticated user has push access. Paginates up
// to 1000 repos.
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

// ref is like "heads/main" or "tags/v1.0.0". Annotated tags resolve to the
// tag object's SHA, not the underlying commit's — adequate for our pinning
// use case since the tag object itself is immutable.
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
	return r.Object.SHA, nil
}

func PostJSON(url string, body interface{}, headers map[string]string) (int, []byte, error) {
	return PostJSONWithTimeout(url, body, headers, 0)
}

// timeout = 0 uses the shared 30s client; pass a longer value for endpoints
// like submission-open-pr that legitimately exceed it on big repos.
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
		// Don't mutate the shared httpClient — concurrent callers race on it.
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
