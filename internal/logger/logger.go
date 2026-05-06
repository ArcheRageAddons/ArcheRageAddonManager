// Package logger writes the manager's runtime log to %APPDATA%\ArcheRageAddonManager\logs\manager.log,
// rotated per session, mirrored to stdout. Falls back to stdout-only on file open failure.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	std     = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	file    *os.File
	logPath string
)

const (
	maxArchives = 5
	baseLogName = "manager.log"
)

// Init rotates the previous session's log into manager.log.1 (and shifts older archives one step
// further), then opens a fresh manager.log for this session.
func Init(logDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}

	rotateLogs(logDir)

	p := filepath.Join(logDir, baseLogName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("open log file: %w", err)
	}

	file = f
	logPath = p
	std = log.New(io.MultiWriter(os.Stdout, f), "", log.LstdFlags|log.Lmicroseconds)
	return p, nil
}

// Best-effort: a failed rename leaves a gap in the archive chain but doesn't block startup.
func rotateLogs(logDir string) {
	oldest := filepath.Join(logDir, fmt.Sprintf("%s.%d", baseLogName, maxArchives))
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stdout, "logger: drop %s: %v\n", oldest, err)
	}

	for i := maxArchives - 1; i >= 1; i-- {
		from := filepath.Join(logDir, fmt.Sprintf("%s.%d", baseLogName, i))
		to := filepath.Join(logDir, fmt.Sprintf("%s.%d", baseLogName, i+1))
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			fmt.Fprintf(os.Stdout, "logger: rotate %s: %v\n", from, err)
		}
	}

	current := filepath.Join(logDir, baseLogName)
	first := filepath.Join(logDir, baseLogName+".1")
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, first); err != nil {
			fmt.Fprintf(os.Stdout, "logger: rotate %s: %v\n", current, err)
		}
	}
}

func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Sync()
		_ = file.Close()
		file = nil
		std = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	}
}

func write(level, msg string) {
	mu.Lock()
	defer mu.Unlock()
	std.Printf("[%s] %s", level, redact(msg))
}

func Info(msg string)                   { write("INFO", msg) }
func Infof(format string, args ...any)  { write("INFO", fmt.Sprintf(format, args...)) }
func Warn(msg string)                   { write("WARN", msg) }
func Warnf(format string, args ...any)  { write("WARN", fmt.Sprintf(format, args...)) }
func Error(msg string)                  { write("ERROR", msg) }
func Errorf(format string, args ...any) { write("ERROR", fmt.Sprintf(format, args...)) }
func Debug(msg string)                  { write("DEBUG", msg) }
func Debugf(format string, args ...any) { write("DEBUG", fmt.Sprintf(format, args...)) }

func FromFrontend(level, msg string) {
	switch level {
	case "error":
		write("FE-ERROR", msg)
	case "warn":
		write("FE-WARN", msg)
	default:
		write("FE-INFO", msg)
	}
}

// Token-shaped secrets we never want in a log file users might paste publicly.
var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-]{20,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`sb_(?:publishable|secret|service)_[A-Za-z0-9_\-]+`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`),
}

func redact(s string) string {
	for _, re := range redactPatterns {
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			subs := re.FindStringSubmatch(match)
			if len(subs) > 1 && subs[1] != "" {
				return subs[1] + "<redacted>"
			}
			return "<redacted>"
		})
	}
	return s
}

func LogReader() (io.ReadCloser, error) {
	mu.Lock()
	p := logPath
	mu.Unlock()
	if p == "" {
		return nil, fmt.Errorf("logger not initialized")
	}
	return os.Open(p)
}

func FlushSync() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Sync()
	}
}

func Header(version string) string {
	return fmt.Sprintf("manager %s | started %s", version, time.Now().Format(time.RFC3339))
}
