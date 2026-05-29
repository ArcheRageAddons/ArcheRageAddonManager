package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	AddonPath     string `json:"addon_path"`
	SetupComplete bool   `json:"setup_complete"`
	// SkipBackups disables the pre-update backup step entirely. Intended for
	// addon authors who iterate fast and don't need the safety net (their
	// git repo is the source of truth). Exposed in Settings → Dev Settings
	// behind a clearly-labelled warning.
	SkipBackups bool `json:"skip_backups,omitempty"`
}

// AddonPathCandidate is a possible Addon directory the manager has detected.
// Source describes where it came from (e.g. "Documents", "OneDrive Personal").
// Exists is true if the directory is present on disk right now.
type AddonPathCandidate struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Exists bool   `json:"exists"`
}

var appConfig *Config
var configPath string

func Init() error {
	appDataPath := os.Getenv("APPDATA")
	configDir := filepath.Join(appDataPath, "ArcheRageAddonManager")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath = filepath.Join(configDir, "config.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = &Config{
			AddonPath:     pickBestCandidate(),
			SetupComplete: false,
		}
		return Save()
	}

	return Load()
}

func Load() error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	appConfig = &Config{}
	return json.Unmarshal(data, appConfig)
}

// Atomic write so a crash mid-write can't leave an empty config.json
// (which would re-trigger the welcome modal and lose the addon path).
func Save() error {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		return err
	}

	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func Get() *Config {
	return appConfig
}

func SetAddonPath(path string) error {
	appConfig.AddonPath = path
	appConfig.SetupComplete = true
	return Save()
}

// MarkSetupComplete records that the user has confirmed the Addon path.
// Used when the user accepts the auto-detected path in the welcome dialog.
func MarkSetupComplete() error {
	appConfig.SetupComplete = true
	return Save()
}

// SetSkipBackups toggles the dev-only "skip backups on update" flag.
func SetSkipBackups(skip bool) error {
	appConfig.SkipBackups = skip
	return Save()
}

// IsFirstRun reports whether the user has not yet confirmed an Addon path.
func IsFirstRun() bool {
	if appConfig == nil {
		return true
	}
	return !appConfig.SetupComplete
}

// DetectAddonPaths returns the ordered list of likely Addon directories the
// user might want, including OneDrive-redirected variants. Existing paths are
// listed first.
func DetectAddonPaths() []AddonPathCandidate {
	userProfile := os.Getenv("USERPROFILE")

	var candidates []AddonPathCandidate
	add := func(base, source string) {
		if base == "" {
			return
		}
		p := filepath.Join(base, "Documents", "ArcheRage", "Addon")
		// De-duplicate by path
		for _, c := range candidates {
			if c.Path == p {
				return
			}
		}
		_, err := os.Stat(p)
		candidates = append(candidates, AddonPathCandidate{
			Path:   p,
			Source: source,
			Exists: err == nil,
		})
	}

	add(userProfile, "Documents")
	add(os.Getenv("OneDrive"), "OneDrive")
	add(os.Getenv("OneDriveConsumer"), "OneDrive Personal")
	add(os.Getenv("OneDriveCommercial"), "OneDrive Work or School")
	if userProfile != "" {
		add(filepath.Join(userProfile, "OneDrive"), "OneDrive (default)")
	}

	// Existing paths first, preserving relative order otherwise
	sorted := make([]AddonPathCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Exists {
			sorted = append(sorted, c)
		}
	}
	for _, c := range candidates {
		if !c.Exists {
			sorted = append(sorted, c)
		}
	}

	return sorted
}

// pickBestCandidate returns the most likely Addon directory: the first
// existing candidate, or the plain Documents default if none exist yet.
func pickBestCandidate() string {
	cands := DetectAddonPaths()
	for _, c := range cands {
		if c.Exists {
			return c.Path
		}
	}
	if len(cands) > 0 {
		return cands[0].Path
	}
	userProfile := os.Getenv("USERPROFILE")
	return filepath.Join(userProfile, "Documents", "ArcheRage", "Addon")
}

func GetConfigDir() string {
	appDataPath := os.Getenv("APPDATA")
	return filepath.Join(appDataPath, "ArcheRageAddonManager")
}
