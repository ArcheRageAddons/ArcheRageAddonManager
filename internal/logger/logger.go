// Package logger writes the manager's runtime log to a single file under
// %APPDATA%\ArcheRageAddonManager\logs\manager.log, replaced on every app
// start. The file mirrors stdout so `wails dev` keeps console output, but
// production GUI builds (which have no attached console) get persistent
// disk-backed logs users can send back when something breaks.
//
// Design notes:
//   - One file, truncated on Init. Previous session's log is gone the
//     moment a new instance starts. If users need a crash log they have
//     to grab the file before relaunching.
//   - Best-effort. If the file can't be opened (permissions, disk full,
//     etc.) the package falls back to stdout-only logging — the manager
//     keeps working, the diagnostics surface just disappears.
//   - Safe to call any logging function before Init; output goes to
//     stdout until Init replaces the writer.
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

// Init opens (and truncates) the log file under logDir. Returns the file
// path on success. Errors are non-fatal — caller can ignore and the
// package will continue logging to stdout only.
func Init(logDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}
	p := filepath.Join(logDir, "manager.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("open log file: %w", err)
	}

	file = f
	logPath = p
	std = log.New(io.MultiWriter(os.Stdout, f), "", log.LstdFlags|log.Lmicroseconds)
	return p, nil
}

// Path returns the active log file path (empty string if Init has not
// run successfully).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Close flushes and closes the log file. Idempotent. Logging after Close
// falls back to stdout.
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

// FromFrontend records a log line that originated in the Svelte side via
// the LogFromFrontend Wails method. Level is the JS console level
// ("error", "warn", "info", "log"); anything else maps to INFO.
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

// ============================================================
// Redaction
// ============================================================

// Token-shaped secret patterns that we never want in a log file users
// might paste publicly. Matches:
//   - Bearer tokens in Authorization-header form
//   - GitHub PATs (ghp_, gho_, ghu_, ghs_, ghr_)
//   - Supabase anon/publishable/service keys (sb_*, eyJ JWTs)
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

// LogReader returns the log file content. Used by future "send diagnostics"
// flows; the desktop currently surfaces it via OpenLogFolder.
func LogReader() (io.ReadCloser, error) {
	mu.Lock()
	p := logPath
	mu.Unlock()
	if p == "" {
		return nil, fmt.Errorf("logger not initialized")
	}
	return os.Open(p)
}

// FlushSync forces a write to disk. Useful right before exit so users
// who hit a fatal error don't lose the last line.
func FlushSync() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Sync()
	}
}

// Header returns the boilerplate first-line banner: app version, OS
// timestamp, etc. Caller should write this immediately after Init so
// the log identifies which build / session produced it.
func Header(version string) string {
	return fmt.Sprintf("manager %s | started %s", version, time.Now().Format(time.RFC3339))
}
