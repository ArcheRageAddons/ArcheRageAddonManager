// Package auth implements custom Discord PKCE + Supabase session bridging.
// We bypass Supabase's built-in Discord provider because it hardcodes the
// email scope; this flow asks Discord directly with `identify` only.
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

	// Must match the redirect URI registered on the Discord OAuth application.
	callbackPort = 53682
	callbackPath = "/auth/callback"

	loginTimeout = 5 * time.Minute

	discordClientID     = "1500384331556720680"
	discordAuthorizeURL = "https://discord.com/api/oauth2/authorize"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	TokenType    string `json:"token_type"`
	User         User   `json:"user"`
}

// 30s headroom for clock skew + the refresh round-trip itself.
const expirySafetyMargin = 30 * time.Second

func (t *Tokens) computeExpiresAt() {
	if t.ExpiresIn > 0 {
		t.ExpiresAt = time.Now().Add(time.Duration(t.ExpiresIn)*time.Second - expirySafetyMargin).Unix()
	}
}

// ExpiresAt == 0 means a legacy token from before we tracked this — refresh proactively.
func (t *Tokens) expiringSoon() bool {
	if t.ExpiresAt <= 0 {
		return true
	}
	return time.Now().Unix() >= t.ExpiresAt
}

// ErrSessionExpired surfaces when the refresh token itself was rejected
// (revoked / 60-day TTL exceeded). Stored tokens are wiped before this returns.
var ErrSessionExpired = errors.New("session expired; please log in again")

// Internal sentinel for the 4xx-from-token-endpoint case, mapped to
// ErrSessionExpired at the public boundary.
var errRefreshRejected = errors.New("refresh token rejected")

// Concurrent refreshes would race against Supabase's refresh-token rotation —
// the second caller's token would be invalidated by the first.
var refreshMu sync.Mutex

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

// Login blocks until the browser-side OAuth flow completes (or aborts).
func Login() (*User, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate verifier: %w", err)
	}
	challenge := codeChallenge(verifier)
	// state defends the loopback listener against local CSRF — any other
	// browser tab during the 5-minute window can hit /auth/callback, but
	// can't guess this value.
	state, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d%s", callbackPort, callbackPath)

	authURL := fmt.Sprintf(
		"%s?client_id=%s&response_type=code&scope=identify&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
		discordAuthorizeURL,
		discordClientID,
		url.QueryEscape(redirect),
		url.QueryEscape(challenge),
		url.QueryEscape(state),
	)

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
			// Reject any callback that doesn't echo the state we issued —
			// stops a malicious browser tab from completing the flow with
			// an attacker-supplied code.
			if got := q.Get("state"); got != state {
				w.WriteHeader(http.StatusBadRequest)
				once.Do(func() { errCh <- errors.New("callback state mismatch") })
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

	bridge, err := callEdgeFunction(code, verifier, redirect)
	if err != nil {
		return nil, fmt.Errorf("edge function: %w", err)
	}

	tokens, err := verifyOTP(bridge.TokenHash)
	if err != nil {
		return nil, fmt.Errorf("verify otp: %w", err)
	}

	if err := saveTokens(tokens); err != nil {
		return nil, fmt.Errorf("save tokens: %w", err)
	}

	user, err := hydrateUser(tokens)
	if err != nil {
		// Login succeeded; profile hydration is retryable later.
		return &tokens.User, nil
	}
	return user, nil
}

func Logout() error {
	err := keyring.Delete(keyringService, keyringKey)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// CurrentUser returns nil (no error) when not logged in OR when the session
// has genuinely expired, so callers can treat both as "logged out" cleanly.
func CurrentUser() (*User, error) {
	tokens, err := RefreshIfNeeded()
	if err != nil {
		if errors.Is(err, ErrSessionExpired) {
			return nil, nil
		}
		return nil, err
	}
	if tokens == nil {
		return nil, nil
	}
	return hydrateUser(tokens)
}

// RefreshIfNeeded returns the freshest available tokens, refreshing against
// Supabase when within the safety margin. Returns ErrSessionExpired (with
// stored tokens wiped) when the refresh token is rejected.
func RefreshIfNeeded() (*Tokens, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	tokens, err := loadTokens()
	if err != nil {
		return nil, err
	}
	if tokens == nil {
		return nil, nil
	}
	if !tokens.expiringSoon() {
		return tokens, nil
	}
	if tokens.RefreshToken == "" {
		_ = keyring.Delete(keyringService, keyringKey)
		return nil, ErrSessionExpired
	}

	fresh, err := refreshTokens(tokens.RefreshToken)
	if err != nil {
		if errors.Is(err, errRefreshRejected) {
			_ = keyring.Delete(keyringService, keyringKey)
			return nil, ErrSessionExpired
		}
		// Transient (network / 5xx). Don't wipe — caller can retry.
		return nil, err
	}

	fresh.computeExpiresAt()
	if err := saveTokens(fresh); err != nil {
		return nil, fmt.Errorf("save refreshed tokens: %w", err)
	}
	return fresh, nil
}

func refreshTokens(refreshToken string) (*Tokens, error) {
	body, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/auth/v1/token?grant_type=refresh_token", supabase.URL),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", supabase.PublishableKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		return nil, errRefreshRejected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh returned %d: %s", resp.StatusCode, string(respBody))
	}

	var t Tokens
	if err := json.Unmarshal(respBody, &t); err != nil {
		return nil, fmt.Errorf("refresh: parse response: %w", err)
	}
	return &t, nil
}

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
	// No Authorization header — this IS the login endpoint, the user has no
	// JWT yet. The function is deployed with JWT verification disabled.
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

// public.profiles is the source of truth for is_admin / is_banned.
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
	t.computeExpiresAt()
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
