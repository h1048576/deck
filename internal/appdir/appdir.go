// Package appdir decides every on-disk location the app uses.
package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

// Name is the application name used for the config directory.
const Name = "deck"

// SettingsName is shared by the settings store and configuration migration.
const SettingsName = "setting.json"

// Config returns the directory holding setting.json.
func Config() string {
	if custom := os.Getenv("DECK_CONFIG_DIR"); strings.TrimSpace(custom) != "" {
		return custom
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+Name)
}

// MigrateLegacyConfig copies existing preferences on first launch.
// Existing setting.json files are never replaced; legacy files are retained.
func MigrateLegacyConfig() error {
	target := Config()
	if _, err := os.Stat(SettingsFile()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	candidates := []string{filepath.Join(target, "settings.json")}
	if strings.TrimSpace(os.Getenv("DECK_CONFIG_DIR")) == "" {
		if base, err := os.UserConfigDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(base, Name, SettingsName),
				filepath.Join(base, Name, "settings.json"),
				filepath.Join(base, "wide-pure", "settings.json"))
		}
		home, _ := os.UserHomeDir()
		candidates = append(candidates, filepath.Join(home, ".wide-pure", "settings.json"))
	}
	source := ""
	for _, path := range candidates {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			source = path
			break
		}
	}
	if source == "" {
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(target, ".config-migration-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.WriteFile(filepath.Join(stage, SettingsName), data, 0o600); err != nil {
		return err
	}
	if _, err := os.Stat(SettingsFile()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(filepath.Join(stage, SettingsName), SettingsFile())
}

// SettingsFile returns the setting.json path.
func SettingsFile() string { return filepath.Join(Config(), SettingsName) }

// RuntimeData returns the directory for generated files and recovery backups.
// It is independent of Config so deleting its contents never resets preferences.
func RuntimeData() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, Name)
}

// SettingsBackups returns the directory for unreadable settings recovery copies.
func SettingsBackups() string { return filepath.Join(RuntimeData(), "setting-backups") }

// ModelBackups returns the directory for pre-write model config backups.
func ModelBackups() string { return filepath.Join(RuntimeData(), "model-backups") }

// McpBackups returns the directory for pre-write MCP config backups.
func McpBackups() string { return filepath.Join(RuntimeData(), "mcp-backups") }

// WebViewData returns the WebView2 user data directory.
func WebViewData() string { return filepath.Join(RuntimeData(), "webview2") }

// ScriptsDir returns the directory where embedded PowerShell helpers are extracted.
func ScriptsDir() string { return filepath.Join(RuntimeData(), "scripts") }
