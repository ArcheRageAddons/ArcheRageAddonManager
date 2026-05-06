package github

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type GitHubClient struct {
	client *http.Client
	token  string
}

// Size limits applied to ZIP downloads + extraction (audit #5). A vanilla
// ArcheRage addon is well under 50MB, but full UI overhauls can hit ~500MB
// zipped / 1.2GB unzipped, so we leave a bit of headroom on top of the
// largest known case. A malicious or accidentally huge repo still gets
// blocked before turning an install into a multi-gig disk fill or RAM
// exhaustion. We cap both the wire-bytes (compressed) and the cumulative
// uncompressed size to defeat zip bombs (high compression ratio).
const (
	MaxZipballBytes   int64 = 1024 * 1024 * 1024     // 1 GB downloaded
	MaxExtractedBytes int64 = 2 * 1024 * 1024 * 1024 // 2 GB extracted total
	MaxFilesInZip     int   = 50000                  // pathological-fan-out guard
)

type RepoInfo struct {
	Owner string
	Repo  string
}

func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		// 2 minutes — covers typical ZIP downloads on slow connections.
		// API calls finish in <1s normally, this is the worst-case limit.
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

// SetToken sets the GitHub personal access token for authenticated requests
func (g *GitHubClient) SetToken(token string) {
	g.token = token
}

// addAuthHeader adds the Authorization header if a token is set
func (g *GitHubClient) addAuthHeader(req *http.Request) {
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
}

func (g *GitHubClient) ParseRepoURL(repoURL string) (*RepoInfo, error) {
	patterns := []string{
		`github\.com[/:]([^/]+)/([^/]+?)(?:\.git)?(?:/.*)?$`,
		`^([^/]+)/([^/]+)$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(repoURL)
		if len(matches) >= 3 {
			repo := strings.TrimSuffix(matches[2], ".git")
			return &RepoInfo{
				Owner: matches[1],
				Repo:  repo,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid GitHub repository URL")
}

// ProgressCallback is called during download to report progress
type ProgressCallback func(downloaded, total int64, speedMBps float64)

// DownloadRepoAsZip downloads a GitHub repository as a ZIP file
func (g *GitHubClient) DownloadRepoAsZip(owner, repo, branch string, progressCallback ProgressCallback) ([]byte, error) {
	// Use GitHub's archive API to get the repo as a zipball
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball/%s", owner, repo, branch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ArcheRage-Addon-Manager")
	g.addAuthHeader(req)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("repository or branch not found")
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	// Get total size from Content-Length header
	totalSize := resp.ContentLength

	// Audit #5: reject up-front if the server claimed a size beyond our cap.
	// Doesn't help when Content-Length is missing (chunked transfer) — we
	// still enforce it during the read loop below.
	if totalSize > MaxZipballBytes {
		return nil, fmt.Errorf("zipball too large: %d bytes (max %d)", totalSize, MaxZipballBytes)
	}

	// Send initial progress update
	if progressCallback != nil {
		if totalSize > 0 {
			progressCallback(0, totalSize, 0)
		} else {
			// Unknown size - send -1 to indicate indeterminate
			progressCallback(0, -1, 0)
		}
	}

	// Read response body with progress tracking
	var buf bytes.Buffer
	var downloaded int64

	startTime := time.Now()
	lastUpdate := startTime
	lastDownloaded := int64(0)

	buffer := make([]byte, 32*1024) // 32KB buffer for more frequent updates
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Audit #5: defence against missing-Content-Length / lying server.
			// Stop reading the moment we cross the wire-bytes cap.
			if downloaded+int64(n) > MaxZipballBytes {
				return nil, fmt.Errorf("zipball exceeded %d bytes during download", MaxZipballBytes)
			}
			buf.Write(buffer[:n])
			downloaded += int64(n)

			// Throttle progress updates to every 50ms for more responsive UI
			now := time.Now()
			if progressCallback != nil && now.Sub(lastUpdate) >= 50*time.Millisecond {
				// Calculate speed based on last interval
				elapsed := now.Sub(lastUpdate).Seconds()
				bytesSinceLastUpdate := downloaded - lastDownloaded
				speedBytesPerSecond := float64(bytesSinceLastUpdate) / elapsed
				speedMBps := speedBytesPerSecond / (1024 * 1024)

				// If we know the total size, use it; otherwise use -1 for indeterminate
				if totalSize > 0 {
					progressCallback(downloaded, totalSize, speedMBps)
				} else {
					progressCallback(downloaded, -1, speedMBps)
				}

				lastUpdate = now
				lastDownloaded = downloaded
			}
		}
		if err == io.EOF {
			// Final progress update - now we know the actual size
			if progressCallback != nil {
				elapsed := time.Since(startTime).Seconds()
				if elapsed > 0 {
					speedMBps := float64(downloaded) / (1024 * 1024) / elapsed
					progressCallback(downloaded, downloaded, speedMBps)
				}
			}
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// ExtractZipToFolder extracts a ZIP file to a destination folder, optionally filtering by subfolder.
//
// Hardening (audit #1, #5, #12):
//   - Every extracted entry is validated to land inside destPath. Entries
//     whose joined path escapes the destination via "..", absolute paths,
//     or path normalization tricks are rejected outright (Zip Slip).
//   - Only regular files and directories are extracted. Symlinks, devices,
//     pipes, sockets, etc. are skipped — they can target arbitrary paths
//     outside the addon folder when followed by ArcheRage or any tool that
//     opens the addon.
//   - Cumulative uncompressed bytes are capped at MaxExtractedBytes to
//     defeat zip-bomb-style high-ratio archives.
//   - Total entry count is capped at MaxFilesInZip so we don't churn
//     forever on a pathologically-fanned archive.
func (g *GitHubClient) ExtractZipToFolder(zipData []byte, destPath, subfolderFilter string, progressCallback func(current, total int)) error {
	// Create ZIP reader from bytes
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to read ZIP: %v", err)
	}

	if len(reader.File) > MaxFilesInZip {
		return fmt.Errorf("zip contains %d entries (max %d)", len(reader.File), MaxFilesInZip)
	}

	// Resolve destPath to an absolute, cleaned form once so every Zip Slip
	// check below compares apples-to-apples. EvalSymlinks would be ideal
	// but the addon dir doesn't exist yet on first install, so Abs+Clean
	// is the strongest we get without races.
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %v", err)
	}
	absDest = filepath.Clean(absDest)

	// GitHub zipballs have a root folder like "owner-repo-commithash/"
	// We need to strip this prefix
	var rootPrefix string
	if len(reader.File) > 0 {
		// First entry is usually the root folder
		firstPath := reader.File[0].Name
		if idx := strings.Index(firstPath, "/"); idx != -1 {
			rootPrefix = firstPath[:idx+1]
		}
	}

	// Count files to extract (for progress)
	totalFiles := 0
	for _, file := range reader.File {
		// Skip root folder entry
		if file.Name == rootPrefix {
			continue
		}

		// Remove root prefix
		relativePath := strings.TrimPrefix(file.Name, rootPrefix)

		// If subfolder filter is specified, only include files from that folder
		if subfolderFilter != "" {
			filterPrefix := subfolderFilter + "/"
			if !strings.HasPrefix(relativePath, filterPrefix) && relativePath != subfolderFilter {
				continue
			}
		}

		totalFiles++
	}

	// Extract files
	currentFile := 0
	var extractedBytes int64
	for _, file := range reader.File {
		// Skip root folder entry
		if file.Name == rootPrefix {
			continue
		}

		// Audit #12: skip non-regular entries up front. Symlinks in particular
		// can point outside destPath even when their entry name is benign;
		// we never want them in an addon folder.
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip entry %q is a symlink (refused)", file.Name)
		}
		if mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice|os.ModeIrregular) != 0 {
			return fmt.Errorf("zip entry %q has non-regular mode %v (refused)", file.Name, mode)
		}

		// Remove root prefix
		relativePath := strings.TrimPrefix(file.Name, rootPrefix)

		// If subfolder filter is specified, only include files from that folder
		if subfolderFilter != "" {
			filterPrefix := subfolderFilter + "/"
			if !strings.HasPrefix(relativePath, filterPrefix) && relativePath != subfolderFilter {
				continue
			}
			// Remove the subfolder prefix from the destination path
			relativePath = strings.TrimPrefix(relativePath, filterPrefix)
		}

		// Skip if relativePath is empty (was just the subfolder itself)
		if relativePath == "" {
			continue
		}

		// Audit #1 (Zip Slip): canonicalize the joined destination and confirm
		// it stays inside destPath. filepath.Join already calls Clean which
		// folds ".." segments, but a name like "../../etc/passwd" would Clean
		// to "..\..\etc\passwd" on Windows and Join may still place it
		// outside. The Rel/HasPrefix dance below is the canonical check.
		destFilePath := filepath.Join(absDest, relativePath)
		destFilePath = filepath.Clean(destFilePath)
		rel, err := filepath.Rel(absDest, destFilePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("zip entry %q escapes destination (refused)", file.Name)
		}

		currentFile++
		if progressCallback != nil {
			progressCallback(currentFile, totalFiles)
		}

		// Create directory if it's a directory entry
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destFilePath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", destFilePath, err)
			}
			continue
		}

		// Create parent directories for file
		if err := os.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}

		// Extract file
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in ZIP: %v", err)
		}

		outFile, err := os.Create(destFilePath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %v", destFilePath, err)
		}

		// Audit #5: cap cumulative extracted bytes — io.CopyN with the
		// remaining budget plus a one-byte sentinel detects overrun without
		// trusting the zip header's UncompressedSize (which is attacker-
		// controlled and wrong on purpose for zip bombs).
		remaining := MaxExtractedBytes - extractedBytes
		written, err := io.CopyN(outFile, rc, remaining+1)
		rc.Close()
		outFile.Close()
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to write file %s: %v", destFilePath, err)
		}
		if written > remaining {
			return fmt.Errorf("zip extraction exceeded %d bytes", MaxExtractedBytes)
		}
		extractedBytes += written
	}

	return nil
}
