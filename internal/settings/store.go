package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"deck/internal/appdir"
)

// Change mutates the current preferences snapshot.
type Change func(current Preferences) Preferences

type queuedChange struct {
	fn   Change
	done chan error
}

// Store persists preferences to setting.json with debounced atomic writes.
type Store struct {
	mu       sync.Mutex
	cond     *sync.Cond
	root     string
	prefs    Preferences
	warning  string
	pending  int
	changes  []queuedChange
	timer    *time.Timer
	queuedAt time.Time
	saveErr  error
}

// NewStore creates a store rooted at the given directory.
func NewStore(root string) *Store {
	s := &Store{root: root}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Prefs returns the current in-memory preferences.
func (s *Store) Prefs() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePreferences(s.prefs)
}

func clonePreferences(prefs Preferences) Preferences {
	prefs.Applications = maps.Clone(prefs.Applications)
	prefs.MenuOrder = slices.Clone(prefs.MenuOrder)
	if prefs.OpenAtLogin != nil {
		enabled := *prefs.OpenAtLogin
		prefs.OpenAtLogin = &enabled
	}
	return prefs
}

// Warning returns the config warning shown to the user.
func (s *Store) Warning() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.warning
}

// IsSaving reports whether changes await persistence.
func (s *Store) IsSaving() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending > 0
}

// Load reads setting.json, falling back to defaults on any error and
// keeping the unreadable file intact for manual recovery.
func (s *Store) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = DefaultPreferences()
	text, err := os.ReadFile(filepath.Join(s.root, appdir.SettingsName))
	if err != nil {
		if !os.IsNotExist(err) {
			s.warning = "本机设置文件无法读取，已暂用默认值。原文件会保留。"
		}
		return
	}
	var data map[string]any
	if err := json.Unmarshal(text, &data); err != nil {
		s.warning = "本机设置文件无法读取，已暂用默认值。原文件会保留。"
		return
	}
	prefs, err := decodePreferences(data)
	if err != nil {
		s.warning = "本机设置文件无法读取，已暂用默认值。原文件会保留。"
		return
	}
	s.prefs = prefs
}

func decodePreferences(data map[string]any) (Preferences, error) {
	defaults := DefaultApplications()
	prefs := DefaultPreferences()
	appsSection, _ := data["applications"].(map[string]any)
	legacyPorts := map[FeatureId]int{Droid: 9335, Zcode: 9332, Workbuddy: 9333, Dsh: 9337, Qoder: 9334, Paseo: 9336}
	applications := map[FeatureId]DroidSettings{}
	for _, id := range DefaultMenuOrder {
		savedRaw, present := appsSection[id]
		if !present && id == Droid {
			savedRaw, present = data["droid"]
		}
		saved, _ := savedRaw.(map[string]any)
		merged := map[string]any{}
		for key, val := range structToMap(defaults[id]) {
			merged[key] = val
		}
		for key, val := range saved {
			merged[key] = val
		}
		if id == Zcode {
			if _, ok := saved["terminalFollow"]; !ok && saved != nil {
				// 旧版 ZCode 的字号未开放，保存的 17 是通用占位值。
				merged["fontSize"] = float64(defaults[Zcode].FontSize)
			}
		}
		versionValue, _ := data["applicationPortsVersion"].(float64)
		if int(versionValue) != ApplicationPortsVersion && id != Codex && saved != nil {
			if rawPort, ok := saved["port"].(float64); ok && int(rawPort) == legacyPorts[id] {
				// 只迁移旧预设端口；Codex 和自定义端口保持原值。
				merged["port"] = float64(defaults[id].Port)
			}
		}
		parsed, err := ParseSettings(merged, id)
		if err != nil {
			return Preferences{}, err
		}
		applications[id] = parsed
	}
	prefs.Applications = applications
	switch theme := data["theme"].(type) {
	case string:
		if theme == "system" || theme == "light" || theme == "dark" {
			prefs.Theme = theme
		}
	}
	if data["startupMode"] == "maximized" {
		prefs.StartupMode = "maximized"
	}
	if enabled, ok := data["openAtLogin"].(bool); ok {
		prefs.OpenAtLogin = &enabled
	}
	if rawAppearance, present := data["appearance"]; present {
		appearanceMap, _ := rawAppearance.(map[string]any)
		appearance, err := ParseAppearance(appearanceMap)
		if err != nil {
			return Preferences{}, err
		}
		prefs.Appearance = appearance
	}
	if rawHarness, present := data["harness"]; present {
		harnessMap, _ := rawHarness.(map[string]any)
		merged := map[string]any{}
		base := DefaultHarnessSettings()
		merged["skillsCollapsed"] = base.SkillsCollapsed
		merged["modelsCollapsed"] = base.ModelsCollapsed
		merged["mcpsCollapsed"] = base.McpsCollapsed
		for key, val := range harnessMap {
			merged[key] = val
		}
		harness, err := ParseHarnessSettings(merged)
		if err != nil {
			return Preferences{}, err
		}
		prefs.Harness = harness
	}
	versionValue, _ := data["menuOrderVersion"].(float64)
	if int(versionValue) == MenuOrderVersion {
		prefs.MenuOrder = NormalizeMenuOrder(data["menuOrder"])
	}
	return prefs, nil
}

func structToMap(v any) map[string]any {
	text, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	_ = json.Unmarshal(text, &out)
	return out
}

// Update queues a change; the returned channel receives the flush outcome.
func (s *Store) Update(fn Change) <-chan error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending++
	entry := queuedChange{fn: fn, done: make(chan error, 1)}
	s.changes = append(s.changes, entry)
	now := time.Now()
	if s.queuedAt.IsZero() {
		s.queuedAt = now
	}
	delay := 600*time.Millisecond - now.Sub(s.queuedAt)
	if delay > 150*time.Millisecond {
		delay = 150 * time.Millisecond
	}
	if delay < 0 {
		delay = 0
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	// 连续输入合并保存，持续输入时最多等待 600 毫秒；执行操作和关闭时立即刷盘。
	s.timer = time.AfterFunc(delay, s.flushBatch)
	return entry.done
}

func (s *Store) flushBatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timer = nil
	s.queuedAt = time.Time{}
	if len(s.changes) == 0 {
		return
	}
	changes := s.changes
	s.changes = nil
	current := clonePreferences(s.prefs)
	for _, entry := range changes {
		current = entry.fn(current)
	}
	err := func() error {
		text, err := marshalPreferences(current)
		if err != nil {
			return err
		}
		if s.warning == "" && bytes.Equal(text, mustMarshal(s.prefs)) {
			if _, err := os.Stat(filepath.Join(s.root, appdir.SettingsName)); err == nil {
				return nil
			}
		}
		if err := os.MkdirAll(s.root, 0o755); err != nil {
			return err
		}
		if s.warning != "" {
			backup := filepath.Join(s.root, fmt.Sprintf("setting.backup-%d.json", time.Now().UnixMilli()))
			if err := os.Rename(filepath.Join(s.root, appdir.SettingsName), backup); err != nil && !os.IsNotExist(err) {
				return err
			}
			s.warning = ""
		}
		file := filepath.Join(s.root, appdir.SettingsName)
		tmp := file + ".tmp"
		if err := os.WriteFile(tmp, text, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, file); err != nil {
			return err
		}
		return nil
	}()
	if err == nil {
		s.prefs = current
		s.saveErr = nil
	} else {
		s.saveErr = err
	}
	for _, entry := range changes {
		entry.done <- err
		close(entry.done)
	}
	s.pending -= len(changes)
	s.cond.Broadcast()
}

func marshalPreferences(prefs Preferences) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(prefs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustMarshal(prefs Preferences) []byte {
	text, err := marshalPreferences(prefs)
	if err != nil {
		return nil
	}
	return text
}

// Flush waits until every queued change is persisted and returns the last error.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.pending > 0 {
		s.cond.Wait()
	}
	return s.saveErr
}
