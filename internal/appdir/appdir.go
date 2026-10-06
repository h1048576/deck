// Package appdir decides every on-disk location the app uses.
package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

// Name is the application name used for the config directory.
const Name = "wide-pure"

// Config returns the directory holding settings.json and backups.
func Config() string {
	if custom := os.Getenv("WIDE_PURE_CONFIG_DIR"); strings.TrimSpace(custom) != "" {
		return custom
	}
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "."+Name)
	}
	return filepath.Join(base, Name)
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
