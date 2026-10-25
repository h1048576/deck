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
	for id, app := range prefs.Applications {
		app.HiddenModelPatterns = slices.Clone(app.HiddenModelPatterns)
		app.ModelSelector = maps.Clone(app.ModelSelector)
		app.ModelSelectorOrder = slices.Clone(app.ModelSelectorOrder)
		prefs.Applications[id] = app
	}
	prefs.MenuOrder = slices.Clone(prefs.MenuOrder)
	prefs.FontOptions = slices.Clone(prefs.FontOptions)
	prefs.Terminal.WindowSizeOptions = slices.Clone(prefs.Terminal.WindowSizeOptions)
	prefs.Terminal.SettingsPaths = slices.Clone(prefs.Terminal.SettingsPaths)
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
	restoreModelSelectorOrder(&prefs, text)
	s.prefs = prefs
	// 新增字段和迁移结果立即写回，手动编辑 setting.json 时能看到完整配置；写入失败时下次保存会重试。
	if normalized, err := marshalPreferencesWithSource(prefs, text); err == nil && !bytes.Equal(normalized, text) {
		_ = writeSettingsFile(s.root, normalized)
	}
}

func writeSettingsFile(root string, text []byte) error {
	file := filepath.Join(root, appdir.SettingsName)
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, text, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func decodePreferences(data map[string]any) (Preferences, error) {
	defaults := DefaultApplications()
	prefs := DefaultPreferences()
	appsSection, _ := data["applications"].(map[string]any)
	legacyPorts := map[FeatureId]int{Droid: 9335, Zcode: 9332, Workbuddy: 9333, Dsh: 9337, Qoder: 9334, Paseo: 9336}
	fontDefaultsValue, _ := data["fontDefaultsVersion"].(float64)
	migrateFontDefaults := int(fontDefaultsValue) != FontDefaultsVersion
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
		if migrateFontDefaults {
			// 只升级保存的旧默认值；用户手动选择的字体和颜色保持原样。
			if font, ok := merged["fontFamily"].(string); ok && font == LegacyDefaultFontFamily {
				merged["fontFamily"] = defaults[id].FontFamily
			}
			if foreground, ok := merged["foregroundColor"].(string); ok && foreground == LegacyDefaultForeground {
				merged["foregroundColor"] = defaults[id].ForegroundColor
			}
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
		if migrateFontDefaults && appearanceMap != nil {
			if font, ok := appearanceMap["fontFamily"].(string); ok && font == LegacyDefaultFontFamily {
				appearanceMap["fontFamily"] = DefaultAppearance().FontFamily
			}
		}
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
	switch int(versionValue) {
	case MenuOrderVersion:
		prefs.MenuOrder = NormalizeMenuOrder(data["menuOrder"])
	case 2:
		// 版本 2 的顺序里没有 Claude，或被补齐到了末尾；去掉后由补齐逻辑放到 Droid 之后。
		saved, _ := data["menuOrder"].([]any)
		prefs.MenuOrder = NormalizeMenuOrder(slices.DeleteFunc(slices.Clone(saved), func(item any) bool { return item == Claude }))
	}
	prefs.FontOptions = ParseFontOptions(data["fontOptions"])
	if migrateFontDefaults {
		if terminalMap, ok := data["terminal"].(map[string]any); ok {
			if font, ok := terminalMap["fontFamily"].(string); ok && font == LegacyDefaultFontFamily {
				terminalMap["fontFamily"] = DefaultTerminal().FontFamily
			}
		}
	}
	prefs.Terminal = ParseTerminal(data["terminal"])
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

// mergeFileOnlySettings 在启动和保存前读取文件专属字段，避免界面中的旧快照覆盖手动配置。
// 调用方须持有 s.mu；合并隐藏规则和快捷模型菜单，保留正在编辑的其他界面设置。
func (s *Store) mergeFileOnlySettings(prefs *Preferences) ([]byte, error) {
	text, err := os.ReadFile(filepath.Join(s.root, appdir.SettingsName))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(text, &data); err != nil {
		return nil, err
	}
	apps, _ := data["applications"].(map[string]any)
	for _, id := range []FeatureId{Workbuddy, Codex, Droid} {
		config, _ := apps[id].(map[string]any)
		app := prefs.Applications[id]
		app.HiddenModelPatterns = parseHiddenModelPatterns(config["hiddenModelPatterns"])
		if id == Codex {
			if items, present := config["modelSelector"]; present {
				app.ModelSelector = parseModelSelector(items)
			} else {
				app.ModelSelector = defaultCodexModelSelector()
			}
		}
		prefs.Applications[id] = app
	}
	restoreModelSelectorOrder(prefs, text)
	return text, nil
}

// restoreModelSelectorOrder 在 map 解码之外保留 JSON 对象的书写顺序。
func restoreModelSelectorOrder(prefs *Preferences, source []byte) {
	var root struct {
		Applications map[string]struct {
			ModelSelector json.RawMessage `json:"modelSelector"`
		} `json:"applications"`
	}
	if json.Unmarshal(source, &root) != nil {
		return
	}
	app := prefs.Applications[Codex]
	app.ModelSelectorOrder = nil
	decoder := json.NewDecoder(bytes.NewReader(root.Applications[Codex].ModelSelector))
	if token, err := decoder.Token(); err == nil && token == json.Delim('{') {
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			var value json.RawMessage
			if decoder.Decode(&value) != nil {
				break
			}
			name, ok := token.(string)
			if _, valid := app.ModelSelector[name]; ok && valid && !slices.Contains(app.ModelSelectorOrder, name) {
				app.ModelSelectorOrder = append(app.ModelSelectorOrder, name)
			}
		}
	}
	prefs.Applications[Codex] = app
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
		// 启动时读取失败也必须重新检查文件。文件可能已被手动修复；
		// 若仍无法解析，拒绝用默认快照覆盖，避免连带清空隐藏规则。
		original, err := s.mergeFileOnlySettings(&current)
		if err != nil {
			return err
		}
		text, err := marshalPreferencesWithSource(current, original)
		if err != nil {
			return err
		}
		if s.warning == "" && bytes.Equal(text, original) {
			return nil
		}
		if err := os.MkdirAll(s.root, 0o755); err != nil {
			return err
		}
		if s.warning != "" {
			original, err := os.ReadFile(filepath.Join(s.root, appdir.SettingsName))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err == nil {
				if err := os.MkdirAll(appdir.SettingsBackups(), 0o755); err != nil {
					return err
				}
				backup := filepath.Join(appdir.SettingsBackups(), fmt.Sprintf("setting.backup-%d.json", time.Now().UnixMilli()))
				if err := os.WriteFile(backup, original, 0o600); err != nil {
					return err
				}
			}
		}
		if err := writeSettingsFile(s.root, text); err != nil {
			return err
		}
		s.warning = ""
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

// marshalPreferencesWithSource 保留文件中当前版本不认识的字段。
// 文件专属配置直接使用原始 JSON，解析或规范化不能静默删改用户的配置。
func marshalPreferencesWithSource(prefs Preferences, source []byte) ([]byte, error) {
	current, err := marshalPreferences(prefs)
	if err != nil || len(source) == 0 {
		return current, err
	}
	merged, err := mergeSettingsObjects(current, source, nil)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, merged, "", "  "); err != nil {
		return nil, err
	}
	return append(buf.Bytes(), '\n'), nil
}

func mergeSettingsObjects(current, source json.RawMessage, path []string) (json.RawMessage, error) {
	current = bytes.TrimSpace(current)
	source = bytes.TrimSpace(source)
	if len(current) == 0 || len(source) == 0 || current[0] != '{' || source[0] != '{' {
		return current, nil
	}
	var values, original map[string]json.RawMessage
	if err := json.Unmarshal(current, &values); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(source, &original); err != nil {
		return nil, err
	}
	for key, raw := range original {
		value, known := values[key]
		fileOnly := len(path) == 2 && path[0] == "applications" &&
			(key == "modelSelector" || key == "hiddenModelPatterns")
		if !known || fileOnly {
			values[key] = raw
			continue
		}
		merged, err := mergeSettingsObjects(value, raw, append(slices.Clone(path), key))
		if err != nil {
			return nil, err
		}
		values[key] = merged
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(values); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// Flush waits until every queued change is persisted and returns the last error.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.pending > 0 {
		s.cond.Wait()
	}
	if s.saveErr == nil {
		_, err := s.mergeFileOnlySettings(&s.prefs)
		return err
	}
	return s.saveErr
}
