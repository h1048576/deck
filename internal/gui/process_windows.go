//go:build windows

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"deck/internal/settings"
)

// 与 application-processes.ps1 的 Get-WideApplicationScope 保持一致：启动入口与升级后的实际程序
// 可能位于不同目录，只识别该安装下的已知版本布局。
var (
	droidExecutableName = regexp.MustCompile(`(?i)^(?:factory-desktop|droid-desktop|Factory|Droid)\.exe$`)
	droidVersionDir     = regexp.MustCompile(`(?i)^app-[0-9][0-9A-Za-z.+_-]*$`)
	qoderExecutableName = regexp.MustCompile(`(?i)^Qoder(?: CN)?\.exe$`)
	qoderVersionDir     = regexp.MustCompile(`(?i)^[0-9][0-9A-Za-z.+_-]*$`)
)

type applicationScope struct {
	executable     string
	rootExecutable string
	imageName      string
	versionPattern *regexp.Regexp
}

func newApplicationScope(executablePath string, id string) (applicationScope, error) {
	full, err := filepath.Abs(executablePath)
	if err != nil {
		return applicationScope{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return applicationScope{}, err
	}
	if info.IsDir() {
		return applicationScope{}, fmt.Errorf("应用路径不是可执行文件。")
	}
	name := filepath.Base(full)
	directory := filepath.Dir(full)
	root := directory
	versionPattern := ""
	switch {
	case id == settings.Droid && droidExecutableName.MatchString(name):
		if droidVersionDir.MatchString(filepath.Base(directory)) {
			root = filepath.Dir(directory)
		}
		versionPattern = `app-[0-9][0-9A-Za-z.+_-]*`
	case id == settings.Qoder && qoderExecutableName.MatchString(name):
		parent := filepath.Dir(directory)
		if strings.EqualFold(filepath.Base(parent), ".qoder-versions") && qoderVersionDir.MatchString(filepath.Base(directory)) {
			root = filepath.Dir(parent)
		}
		versionPattern = `\.qoder-versions\\[0-9][0-9A-Za-z.+_-]*`
	}
	scope := applicationScope{
		executable:     full,
		rootExecutable: filepath.Join(root, name),
		imageName:      strings.TrimSuffix(name, filepath.Ext(name)) + ".exe",
	}
	if versionPattern != "" {
		prefix := strings.TrimRight(root, `\`) + `\`
		scope.versionPattern, err = regexp.Compile(`(?i)^` + regexp.QuoteMeta(prefix) + versionPattern + `\\` + regexp.QuoteMeta(name) + `$`)
		if err != nil {
			return applicationScope{}, err
		}
	}
	return scope, nil
}

func (s applicationScope) matches(path string) bool {
	if path == "" {
		return false
	}
	if strings.EqualFold(path, s.executable) {
		return true
	}
	if s.versionPattern == nil {
		return false
	}
	return strings.EqualFold(path, s.rootExecutable) || s.versionPattern.MatchString(path)
}

// applicationRunning 直接枚举进程，避免为每次状态检查启动 PowerShell 并查询 CIM。
func applicationRunning(executablePath string, id string) (bool, error) {
	scope, err := newApplicationScope(executablePath, id)
	if err != nil {
		return false, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), scope.imageName) {
			continue
		}
		if path, ok := processImagePath(entry.ProcessID); ok && scope.matches(path) {
			return true, nil
		}
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return false, err
	}
	return false, nil
}

func processImagePath(pid uint32) (string, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buffer[:size]), true
}
