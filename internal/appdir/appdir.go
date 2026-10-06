// Package appdir decides every on-disk location the app uses.
package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

// Name is the application name used for the config directory.
const Name = "deck"

// Config returns the directory holding settings.json and backups.
func Config() string {
	if custom := os.Getenv("DECK_CONFIG_DIR"); strings.TrimSpace(custom) != "" {
		return custom
	}
	return defaultConfig(Name)
}

func defaultConfig(name string) string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "."+name)
	}
	return filepath.Join(base, name)
}

// MigrateLegacyConfig copies existing preferences and backups on first launch.
// Custom directories and existing configurations are left intact.
func MigrateLegacyConfig() error {
	if strings.TrimSpace(os.Getenv("DECK_CONFIG_DIR")) != "" {
		return nil
	}
	target := Config()
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	source := defaultConfig("wide-pure")
	if info, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".deck-config-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, name := range []string{"settings.json", "model-backups", "mcp-backups"} {
		path := filepath.Join(source, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		destination := filepath.Join(stage, name)
		if info.IsDir() {
			if err := os.CopyFS(destination, os.DirFS(path)); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
	}
	return os.Rename(stage, target)
}

// SettingsFile returns the settings.json path.
func SettingsFile() string { return filepath.Join(Config(), "settings.json") }

// ModelBackups returns the directory for pre-write model config backups.
func ModelBackups() string { return filepath.Join(Config(), "model-backups") }

// McpBackups returns the directory for pre-write MCP config backups.
func McpBackups() string { return filepath.Join(Config(), "mcp-backups") }

// WebViewData returns the WebView2 user data directory.
func WebViewData() string { return filepath.Join(Config(), "webview2") }

// ScriptsDir returns the directory where embedded PowerShell helpers are extracted.
func ScriptsDir() string { return filepath.Join(Config(), "scripts") }
