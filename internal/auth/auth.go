// Package auth implements custom Discord OAuth + Supabase session bridging.
//
// The flow:
//  1. Run a PKCE OAuth dance with Discord directly (NOT via Supabase) so we
//     can request only the `identify` scope — no email is ever asked for.
//  2. Hand the authorization code to our `discord-login` Edge Function, which
//     exchanges it with Discord (using the client secret stored server-side),
//     creates/looks up the user in `auth.users`, and returns a one-time
//     magic-link token_hash.
//  3. Exchange that token_hash with Supabase's `/auth/v1/verify` endpoint for
//     a real Supabase session (access_token + refresh_token), which we then
//     persist in the OS keyring.
//
// This intentionally bypasses Supabase's built-in Discord provider, which
// hardcodes the email scope and cannot be configured to omit it.
package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"archerage-addon-manager/internal/supabase"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "ArcheRageAddonManager"
	keyringKey     = "supabase-tokens"

	// Fixed loopback port — must match the redirect URI registered on the
	// Discord OAuth application.
	callbackPort = 53682
	callbackPath = "/auth/callback"

	loginTimeout = 5 * time.Minute

	discordClientID    = "1500384331556720680"
	discordAuthorizeURL = "https://discord.com/api/oauth2/authorize"
)

// httpClient is package-shared so every outbound request from the auth
// package has a 30s ceiling — prevents the app hanging forever on dead
// connections.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Tokens is the subset of Supabase's /auth/v1/verify response we persist.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         User   `json:"user"`
}

// User is a partial mirror of Supabase's auth.users payload, plus the
// public.profiles fields we hydrate after login.
type User struct {
	ID              string                 `json:"id"`
	UserMetadata    map[string]interface{} `json:"user_metadata,omitempty"`
	AppMetadata     map[string]interface{} `json:"app_metadata,omitempty"`
	DiscordID       string                 `json:"discord_id,omitempty"`
	DiscordUsername string                 `json:"discord_username,omitempty"`
	DiscordAvatar   string                 `json:"discord_avatar,omitempty"`
	IsAdmin         bool                   `json:"is_admin"`
	IsBanned        bool                   `json:"is_banned"`
}

type profileRow struct {
	ID              string `json:"id"`
	DiscordID       string `json:"discord_id"`
	DiscordUsername string `json:"discord_username"`
	DiscordAvatar   string `json:"discord_avatar"`
	IsAdmin         bool   `json:"is_admin"`
	IsBanned        bool   `json:"is_banned"`
}

// Login runs the full Discord OAuth + Supabase session bridge. Blocks until
// the user finishes (or aborts) the browser-side login.
func Login() (*User, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate verifier: %w", err)
	}
	challenge := codeChallenge(verifier)
	redirect := fmt.Sprintf("http://127.0.0.1:%d%s", callbackPort, callbackPath)

	// 1. Build Discord authorize URL (identify scope only — no email).
	authURL := fmt.Sprintf(
		"%s?client_id=%s&response_type=code&scope=identify&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256&prompt=none",
		discordAuthorizeURL,
		discordClientID,
		url.QueryEscape(redirect),
		url.QueryEscape(challenge),
	)

	// 2. Listen on loopback for Discord's redirect.
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return nil, fmt.Errorf("loopback listener (port %d may be in use): %w", callbackPort, err)
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var once sync.Once

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != callbackPath {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			if errStr := q.Get("error"); errStr != "" {
				desc := q.Get("error_description")
				writeHTML(w, fmt.Sprintf(
					`<h2 style="color:#e94560">Login failed</h2><p>%s</p><p style="color:#808080">You can close this tab and try again from the app.</p>`,
					htmlEscape(desc)))
				once.Do(func() { errCh <- fmt.Errorf("%s: %s", errStr, desc) })
				return
			}
			code := q.Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				once.Do(func() { errCh <- errors.New("callback missing 'code' parameter") })
				return
			}
			writeHTML(w, `<h2 style="color:#4a9d7c">Logged in</h2><p style="color:#808080">You can close this tab and return to ArcheRage Addon Manager.</p>`)
			once.Do(func() { codeCh <- code })
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-time.After(loginTimeout):
		return nil, errors.New("login timed out — no callback received within 5 minutes")
	}

	// 3. Hand code to our Edge Function — it exchanges with Discord and
	//    returns a magic-link token_hash we can use to mint a Supabase session.
	bridge, err := callEdgeFunction(code, verifier, redirect)
	if err != nil {
		return nil, fmt.Errorf("edge function: %w", err)
	}

	// 4. Verify the magic link with Supabase to get a real session.
	tokens, err := verifyOTP(bridge.TokenHash)
	if err != nil {
		return nil, fmt.Errorf("verify otp: %w", err)
	}

	if err := saveTokens(tokens); err != nil {
		return nil, fmt.Errorf("save tokens: %w", err)
	}

	user, err := hydrateUser(tokens)
	if err != nil {
		// Non-fatal — we logged in, profile fetch can be retried later.
		return &tokens.User, nil
	}
	return user, nil
}

// Logout clears the stored Supabase session.
func Logout() error {
	err := keyring.Delete(keyringService, keyringKey)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// CurrentUser returns the cached/refreshed user if logged in, or nil if not.
func CurrentUser() (*User, error) {
	tokens, err := loadTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, nil
	}
	return hydrateUser(tokens)
}

// LoadStoredTokens exposes the full token bundle (access + refresh) for
// callers that need to make authenticated REST calls against Supabase.
func LoadStoredTokens() (*Tokens, error) {
	return loadTokens()
}

// edgeBridgeResp is what our discord-login Edge Function returns on success.
type edgeBridgeResp struct {
	Email     string `json:"email"`
	TokenHash string `json:"token_hash"`
	Error     string `json:"error,omitempty"`
}

func callEdgeFunction(code, verifier, redirectURI string) (*edgeBridgeResp, error) {
	body, _ := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
	})

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/functions/v1/discord-login", supabase.URL),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// `apikey` identifies the project; we deliberately do NOT send an
	// Authorization header here because the user hasn't authenticated yet —
	// this *is* the login endpoint. The function must be deployed with JWT
	// verification disabled.
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var out edgeBridgeResp
	_ = json.Unmarshal(raw, &out)

	if resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			return nil, fmt.Errorf("%d: %s", resp.StatusCode, out.Error)
		}
		return nil, fmt.Errorf("%d: %s", resp.StatusCode, string(raw))
	}
	if out.TokenHash == "" || out.Email == "" {
		return nil, fmt.Errorf("incomplete response: %s", string(raw))
	}
	return &out, nil
}

func verifyOTP(tokenHash string) (*Tokens, error) {
	body, _ := json.Marshal(map[string]string{
		"type":       "magiclink",
		"token_hash": tokenHash,
	})

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/auth/v1/verify", supabase.URL),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verify returned %d: %s", resp.StatusCode, string(b))
	}

	var t Tokens
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// hydrateUser combines the auth.users payload with the public.profiles row
// (which is the source of truth for is_admin / is_banned).
func hydrateUser(tokens *Tokens) (*User, error) {
	u := tokens.User

	if md := u.AppMetadata; md != nil {
		if v, ok := md["discord_id"].(string); ok {
			u.DiscordID = v
		}
		if v, ok := md["discord_username"].(string); ok {
			u.DiscordUsername = v
		}
	}
	if u.DiscordUsername == "" {
		if md := u.UserMetadata; md != nil {
			if v, ok := md["discord_username"].(string); ok {
				u.DiscordUsername = v
			}
		}
	}

	row, err := fetchProfile(u.ID, tokens.AccessToken)
	if err != nil {
		return &u, err
	}
	if row != nil {
		u.DiscordID = row.DiscordID
		u.DiscordUsername = row.DiscordUsername
		u.DiscordAvatar = row.DiscordAvatar
		u.IsAdmin = row.IsAdmin
		u.IsBanned = row.IsBanned
	}
	return &u, nil
}

func fetchProfile(userID, accessToken string) (*profileRow, error) {
	endpoint := fmt.Sprintf("%s/rest/v1/profiles?id=eq.%s&select=id,discord_id,discord_username,discord_avatar,is_admin,is_banned",
		supabase.URL, url.QueryEscape(userID))

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("profiles fetch returned %d: %s", resp.StatusCode, string(body))
	}

	var rows []profileRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func saveTokens(t *Tokens) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringKey, string(data))
}

func loadTokens() (*Tokens, error) {
	s, err := keyring.Get(keyringService, keyringKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var t Tokens
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func openBrowser(target string) error {
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

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>ArcheRage Addon Manager</title></head><body style="font-family:-apple-system,Segoe UI,sans-serif;background:#121212;color:#e0e0e0;display:flex;align-items:center;justify-content:center;height:100vh;margin:0"><div style="text-align:center;max-width:480px;padding:24px">%s</div></body></html>`, body)
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

func htmlEscape(s string) string { return htmlEscaper.Replace(s) }
