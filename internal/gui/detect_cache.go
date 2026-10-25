package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 已安装应用的检测结果跨进程保留，重启 deck 后无需为每个应用重新启动 PowerShell 扫描安装。
// 可执行文件的大小或修改时间变化、路径失效或记录过期时都会重新检测。
const persistedInstallMaxAge = 12 * time.Hour

type persistedInstallation struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Version  string `json:"version"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	Detected int64  `json:"detected"`
}

type installationStore struct {
	file    string
	mu      sync.Mutex
	loaded  bool
	entries map[string]persistedInstallation
}

func newInstallationStore(file string) *installationStore {
	return &installationStore{file: file, entries: map[string]persistedInstallation{}}
}

func (s *installationStore) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	text, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var entries map[string]persistedInstallation
	if json.Unmarshal(text, &entries) == nil && entries != nil {
		s.entries = entries
	}
}

func (s *installationStore) lookup(key string) *installation {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	entry, ok := s.entries[key]
	if !ok {
		return nil
	}
	if time.Since(time.UnixMilli(entry.Detected)) > persistedInstallMaxAge {
		return nil
	}
	info, err := os.Stat(entry.Path)
	if err != nil || info.IsDir() || info.Size() != entry.Size || info.ModTime().UnixNano() != entry.ModTime {
		return nil
	}
	return &installation{Name: entry.Name, Path: entry.Path, Version: entry.Version}
}

func (s *installationStore) save(key string, value *installation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	if value == nil {
		if _, ok := s.entries[key]; !ok {
			return
		}
		delete(s.entries, key)
	} else {
		info, err := os.Stat(value.Path)
		if err != nil || info.IsDir() {
			return
		}
		s.entries[key] = persistedInstallation{
			Name:     value.Name,
			Path:     value.Path,
			Version:  value.Version,
			Size:     info.Size(),
			ModTime:  info.ModTime().UnixNano(),
			Detected: time.Now().UnixMilli(),
		}
	}
	text, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, text, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.file)
}
