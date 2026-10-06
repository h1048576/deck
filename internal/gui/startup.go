package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"deck/internal/appdir"

	"golang.org/x/sys/windows/registry"
)

// StartupManager toggles the per-user Run key entry.
type StartupManager struct{}

const startupValueName = appdir.Name

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// NewStartupManager creates the manager; available only on Windows.
func NewStartupManager() *StartupManager { return &StartupManager{} }

// Available reports whether login startup can be managed.
func (s *StartupManager) Available() bool { return isWindowsRuntime() }

func (s *StartupManager) executable() string {
	if custom := os.Getenv("PORTABLE_EXECUTABLE_FILE"); custom != "" {
		return custom
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(exe)
}

// Enabled reads back the Run entry, mirroring the original verification.
func (s *StartupManager) Enabled() bool {
	if !s.Available() {
		return false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(startupValueName)
	if err != nil {
		return false
	}
	expected := `"` + s.executable() + `"`
	return value == expected
}

// Set enables or disables login startup with rollback on failure.
func (s *StartupManager) Set(input any) (bool, error) {
	enabled, ok := input.(bool)
	if !ok {
		return false, fmt.Errorf("开机启动设置无效")
	}
	if !s.Available() {
		return false, fmt.Errorf("开机启动仅支持 Windows 桌面发布版")
	}
	previous := s.Enabled()
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("无法更新 Windows 开机启动设置，请重试")
	}
	defer key.Close()
	if enabled {
		value := `"` + s.executable() + `"`
		if err := key.SetStringValue(startupValueName, value); err != nil {
			return false, fmt.Errorf("无法更新 Windows 开机启动设置，请重试")
		}
	} else {
		if err := key.DeleteValue(startupValueName); err != nil && err != registry.ErrNotExist {
			return false, fmt.Errorf("无法更新 Windows 开机启动设置，请重试")
		}
	}
	now := s.Enabled()
	if now != enabled {
		// 回滚到之前的状态。
		if previous {
			_ = key.SetStringValue(startupValueName, `"`+s.executable()+`"`)
		} else {
			_ = key.DeleteValue(startupValueName)
		}
		return false, fmt.Errorf("无法更新 Windows 开机启动设置，请重试")
	}
	return now, nil
}
