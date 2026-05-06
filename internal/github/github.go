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

// Caps stop runaway / malicious downloads from filling disk or RAM. The 1 GB
// compressed cap accommodates current real outliers (~500 MB UI overhauls)
// with headroom; the 2 GB extracted cap defeats high-ratio zip bombs.
const (
	MaxZipballBytes   int64 = 1024 * 1024 * 1024
	MaxExtractedBytes int64 = 2 * 1024 * 1024 * 1024
	MaxFilesInZip     int   = 50000
)

type RepoInfo struct {
	Owner string
	Repo  string
}

func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		// Wall-clock cap covers a ~500 MB addon down to ~280 KB/s. Slower
		// links would need an idle timeout instead of a total one.
		client: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (g *GitHubClient) SetToken(token string) {
	g.token = token
}

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

type ProgressCallback func(downloaded, total int64, speedMBps float64)

func (g *GitHubClient) DownloadRepoAsZip(owner, repo, branch string, progressCallback ProgressCallback) ([]byte, error) {
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

	totalSize := resp.ContentLength
	if totalSize > MaxZipballBytes {
		return nil, fmt.Errorf("zipball too large: %d bytes (max %d)", totalSize, MaxZipballBytes)
	}

	if progressCallback != nil {
		if totalSize > 0 {
			progressCallback(0, totalSize, 0)
		} else {
			progressCallback(0, -1, 0)
		}
	}

	var buf bytes.Buffer
	var downloaded int64

	startTime := time.Now()
	lastUpdate := startTime
	lastDownloaded := int64(0)

	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Defends against missing-Content-Length (chunked transfer) where
			// the up-front size check above couldn't see the real size.
			if downloaded+int64(n) > MaxZipballBytes {
				return nil, fmt.Errorf("zipball exceeded %d bytes during download", MaxZipballBytes)
			}
			buf.Write(buffer[:n])
			downloaded += int64(n)

			now := time.Now()
			if progressCallback != nil && now.Sub(lastUpdate) >= 50*time.Millisecond {
				elapsed := now.Sub(lastUpdate).Seconds()
				bytesSinceLastUpdate := downloaded - lastDownloaded
				speedMBps := float64(bytesSinceLastUpdate) / elapsed / (1024 * 1024)

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

// Refuses Zip Slip (entries that would resolve outside destPath), symlinks /
// devices / sockets (can target arbitrary filesystem locations when followed),
// and oversized archives (file count + cumulative uncompressed bytes).
func (g *GitHubClient) ExtractZipToFolder(zipData []byte, destPath, subfolderFilter string, progressCallback func(current, total int)) error {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to read ZIP: %v", err)
	}

	if len(reader.File) > MaxFilesInZip {
		return fmt.Errorf("zip contains %d entries (max %d)", len(reader.File), MaxFilesInZip)
	}

	// EvalSymlinks would be ideal but the addon dir doesn't exist yet on
	// first install, so Abs+Clean is the strongest we get without races.
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %v", err)
	}
	absDest = filepath.Clean(absDest)

	// GitHub zipballs have a root folder like "owner-repo-commithash/".
	var rootPrefix string
	if len(reader.File) > 0 {
		firstPath := reader.File[0].Name
		if idx := strings.Index(firstPath, "/"); idx != -1 {
			rootPrefix = firstPath[:idx+1]
		}
	}

	totalFiles := 0
	for _, file := range reader.File {
		if file.Name == rootPrefix {
			continue
		}
		relativePath := strings.TrimPrefix(file.Name, rootPrefix)
		if subfolderFilter != "" {
			filterPrefix := subfolderFilter + "/"
			if !strings.HasPrefix(relativePath, filterPrefix) && relativePath != subfolderFilter {
				continue
			}
		}
		totalFiles++
	}

	currentFile := 0
	var extractedBytes int64
	for _, file := range reader.File {
		if file.Name == rootPrefix {
			continue
		}

		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip entry %q is a symlink (refused)", file.Name)
		}
		if mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice|os.ModeIrregular) != 0 {
			return fmt.Errorf("zip entry %q has non-regular mode %v (refused)", file.Name, mode)
		}

		relativePath := strings.TrimPrefix(file.Name, rootPrefix)
		if subfolderFilter != "" {
			filterPrefix := subfolderFilter + "/"
			if !strings.HasPrefix(relativePath, filterPrefix) && relativePath != subfolderFilter {
				continue
			}
			relativePath = strings.TrimPrefix(relativePath, filterPrefix)
		}
		if relativePath == "" {
			continue
		}

		// Zip Slip: filepath.Join calls Clean which folds ".." but a name
		// like "../../etc/passwd" can still place the result outside
		// destPath on some platforms. The Rel/HasPrefix check is canonical.
		destFilePath := filepath.Clean(filepath.Join(absDest, relativePath))
		rel, err := filepath.Rel(absDest, destFilePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("zip entry %q escapes destination (refused)", file.Name)
		}

		currentFile++
		if progressCallback != nil {
			progressCallback(currentFile, totalFiles)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destFilePath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", destFilePath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in ZIP: %v", err)
		}

		outFile, err := os.Create(destFilePath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %v", destFilePath, err)
		}

		// CopyN with remaining+1 detects overrun without trusting the zip
		// header's UncompressedSize (attacker-controlled in zip bombs).
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
