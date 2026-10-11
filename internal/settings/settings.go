// Package settings holds the persisted preferences schema and validation,
// mirroring the original app's shared/types.ts.
package settings

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FeatureId identifies one of the managed desktop applications.
type FeatureId = string

// All feature ids in default menu order.
const (
	Codex     FeatureId = "codex"
	Droid     FeatureId = "droid"
	Zcode     FeatureId = "zcode"
	Workbuddy FeatureId = "workbuddy"
	Dsh       FeatureId = "dsh"
	Qoder     FeatureId = "qoder"
	Paseo     FeatureId = "paseo"
	Manus     FeatureId = "manus"
	Claude    FeatureId = "claude"
)

var DefaultMenuOrder = []FeatureId{Codex, Droid, Claude, Zcode, Workbuddy, Dsh, Qoder, Paseo, Manus}

const MenuOrderVersion = 3
const ApplicationPortsVersion = 1
const InstallCacheMs = 60000
const MissingInstallCacheMs = 15000

// DroidSettings is the per-application appearance configuration.
type DroidSettings struct {
	Width          string `json:"width"`
	SidebarWidth   string `json:"sidebarWidth"`
	MaxWidth       string `json:"maxWidth"`
	ChatHeight     string `json:"chatHeight"`
	FontFamily     string `json:"fontFamily"`
	FontSize       int    `json:"fontSize"`
	FontWeight     int    `json:"fontWeight"`
	TerminalFollow bool   `json:"terminalFollow"`
	HideLocalMerge bool   `json:"hideLocalMerge"`
	HideGitDiff    bool   `json:"hideGitDiff"`
	Port           int    `json:"port"`
	ExecutablePath string `json:"executablePath"`
	HideChanges    bool   `json:"hideChanges"`
	PreventSummary bool   `json:"preventSummary"`
}

// AppearanceSettings configures the deck window itself.
type AppearanceSettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	Mode       string `json:"mode"`
}

// HarnessSettings stores collapse state of the Harness sections.
type HarnessSettings struct {
	SkillsCollapsed bool `json:"skillsCollapsed"`
	ModelsCollapsed bool `json:"modelsCollapsed"`
	McpsCollapsed   bool `json:"mcpsCollapsed"`
}

// TerminalSettings 配置 Windows Terminal 的字体。
// SettingsPaths 可以使用 %LOCALAPPDATA% 等 Windows 环境变量，便于在不同电脑上通用。
type TerminalSettings struct {
	FontFamily    string   `json:"fontFamily"`
	SettingsPaths []string `json:"settingsPaths"`
}

// DefaultTerminal 对应正式版、预览版和 Canary 版 Windows Terminal。
func DefaultTerminal() TerminalSettings {
	return TerminalSettings{
		FontFamily: "Cascadia Mono, LXGW WenKai Mono",
		SettingsPaths: []string{
			`%LOCALAPPDATA%\Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json`,
			`%LOCALAPPDATA%\Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json`,
			`%LOCALAPPDATA%\Packages\Microsoft.WindowsTerminalCanary_8wekyb3d8bbwe\LocalState\settings.json`,
		},
	}
}

// ParseTerminal 读取 setting.json 中的终端配置。
// 无效项回退到默认值，避免手动编辑出错时整个设置文件被当作无法读取。
func ParseTerminal(input any) TerminalSettings {
	value := DefaultTerminal()
	data, ok := input.(map[string]any)
	if !ok {
		return value
	}
	if font, ok := data["fontFamily"].(string); ok && ValidFontFamily(font) {
		value.FontFamily = strings.TrimSpace(font)
	}
	if list, ok := data["settingsPaths"].([]any); ok {
		paths := make([]string, 0, len(list))
		for _, item := range list {
			path, ok := item.(string)
			path = strings.TrimSpace(path)
			if !ok || !ValidTerminalSettingsPath(path) || slices.Contains(paths, path) {
				continue
			}
			paths = append(paths, path)
		}
		if len(paths) > 0 {
			value.SettingsPaths = paths
		}
	}
	return value
}

// ValidTerminalSettingsPath 检查终端配置文件路径能否安全使用。
func ValidTerminalSettingsPath(path string) bool {
	return path != "" && len(path) <= 1024 && !strings.ContainsAny(path, "\r\n\x00")
}

// DefaultHarnessSettings mirrors DEFAULT_HARNESS_SETTINGS.
func DefaultHarnessSettings() HarnessSettings {
	return HarnessSettings{SkillsCollapsed: true, ModelsCollapsed: true, McpsCollapsed: true}
}

// DefaultAppearance mirrors DEFAULT_APPEARANCE.
func DefaultAppearance() AppearanceSettings {
	return AppearanceSettings{FontFamily: "Cascadia Mono, LXGW WenKai Mono", FontSize: 17, Mode: "compact"}
}

// SystemFont is the fallback font stack for '系统默认'.
const SystemFont = `"Segoe UI", "Microsoft YaHei", -apple-system, BlinkMacSystemFont, sans-serif`

// DefaultFontOption 是字体列表里代表系统界面字体的选项。
const DefaultFontOption = "default"

// legacySystemFontOption 是旧版 deck 设置页里系统字体选项的值。
const legacySystemFontOption = "系统默认"

// SystemFontFamily 是 default 在受管应用中对应的字体。
// 不带引号，避免启动脚本转发参数时引号被拆开。
const SystemFontFamily = "Segoe UI, Microsoft YaHei"

// DefaultFontOptions 是字体下拉框的默认选项。
func DefaultFontOptions() []string {
	return []string{DefaultFontOption, "Cascadia Mono, LXGW WenKai Mono", "Maple Mono Normal NF CN"}
}

// IsDefaultFont 判断字体值是否表示系统界面字体。
func IsDefaultFont(value string) bool {
	value = strings.TrimSpace(value)
	return strings.EqualFold(value, DefaultFontOption) || value == legacySystemFontOption
}

// ResolveFontFamily 把 default 换成实际字体，其他值原样返回。
func ResolveFontFamily(value string) string {
	if IsDefaultFont(value) {
		return SystemFontFamily
	}
	return value
}

// ParseFontOptions 读取 setting.json 中的字体列表。
// 跳过无效和重复的字体名；列表缺失或为空时使用默认列表，避免下拉框没有选项。
func ParseFontOptions(input any) []string {
	list, ok := input.([]any)
	if !ok {
		return DefaultFontOptions()
	}
	options := make([]string, 0, len(list))
	for _, item := range list {
		value, ok := item.(string)
		value = strings.TrimSpace(value)
		if !ok || !ValidFontFamily(value) || slices.Contains(options, value) {
			continue
		}
		options = append(options, value)
	}
	if len(options) == 0 {
		return DefaultFontOptions()
	}
	return options
}

// DefaultPorts maps every feature id to its default debug port.
// 端口与菜单顺序无关，调整默认菜单顺序时不能改变已发布的端口。
func DefaultPorts() map[FeatureId]int {
	return map[FeatureId]int{
		Codex: 9331, Droid: 9332, Zcode: 9333, Workbuddy: 9334, Dsh: 9335,
		Qoder: 9336, Paseo: 9337, Manus: 9338, Claude: 9339,
	}
}

// Capabilities lists which settings an application supports.
type Capabilities struct {
	Name       string
	Sidebar    bool
	MaxWidth   bool
	ChatHeight bool
	FontFamily bool
	FontSize   bool
	Merge      bool
	Diff       bool
	Changes    bool
	Summary    bool
	TerminalUI bool // whether the terminal follow switch is offered
	// FontScript 表示应用拒绝调试端口，字体只能通过复制到 DevTools 的脚本手动应用。
	FontScript bool
}

// Applications mirrors APPLICATIONS from types.ts.
var Applications = map[FeatureId]Capabilities{
	Codex:     {Name: "Codex", Sidebar: true, FontFamily: true, FontSize: true, Summary: true, TerminalUI: true},
	Droid:     {Name: "Droid", Sidebar: true, MaxWidth: true, ChatHeight: true, FontFamily: true, FontSize: true, TerminalUI: true},
	Zcode:     {Name: "ZCode", Sidebar: true, FontFamily: true, FontSize: true, Changes: true, TerminalUI: true},
	Workbuddy: {Name: "WorkBuddy", Sidebar: true, MaxWidth: true, FontFamily: true, FontSize: true},
	Manus:     {Name: "Manus", Sidebar: true, MaxWidth: true, FontFamily: true, FontSize: true},
	Dsh:       {Name: "DSH", Sidebar: true, MaxWidth: true, ChatHeight: true, FontFamily: true, FontSize: true, TerminalUI: true},
	Qoder:     {Name: "Qoder", Sidebar: true, MaxWidth: true, FontFamily: true, FontSize: true, TerminalUI: true},
	Paseo:     {Name: "Paseo", Sidebar: true, Merge: true, Diff: true},
	Claude:    {Name: "Claude", FontFamily: true, FontScript: true},
}

// ApplicationsInOrder returns the capability rows in default menu order.
func ApplicationsInOrder() []FeatureId { return append([]FeatureId(nil), DefaultMenuOrder...) }

// DefaultApplications builds DEFAULT_APPLICATIONS.
func DefaultApplications() map[FeatureId]DroidSettings {
	ports := DefaultPorts()
	base := func(id FeatureId) DroidSettings {
		return DroidSettings{
			Width: "70vw", SidebarWidth: "15vw", MaxWidth: "90rem", ChatHeight: "80px",
			FontFamily: "Cascadia Mono, LXGW WenKai Mono", FontSize: 17, FontWeight: 300, TerminalFollow: true,
			Port: ports[id],
		}
	}
	return map[FeatureId]DroidSettings{
		Codex:     func() DroidSettings { s := base(Codex); s.FontWeight = 100; s.PreventSummary = true; return s }(),
		Droid:     base(Droid),
		Zcode:     func() DroidSettings { s := base(Zcode); s.FontSize = 18; s.HideChanges = true; return s }(),
		Workbuddy: func() DroidSettings { s := base(Workbuddy); s.FontWeight = 200; return s }(),
		Manus:     func() DroidSettings { s := base(Manus); s.FontWeight = 200; return s }(),
		Dsh:       base(Dsh),
		Qoder:     base(Qoder),
		Paseo:     func() DroidSettings { s := base(Paseo); s.HideLocalMerge = true; s.HideGitDiff = true; return s }(),
		Claude:    base(Claude),
	}
}

// DefaultApplicationsOrdered returns defaults keyed in menu order.
func DefaultApplicationsOrdered() []struct {
	ID       FeatureId
	Settings DroidSettings
} {
	defaults := DefaultApplications()
	out := make([]struct {
		ID       FeatureId
		Settings DroidSettings
	}, 0, len(DefaultMenuOrder))
	for _, id := range DefaultMenuOrder {
		out = append(out, struct {
			ID       FeatureId
			Settings DroidSettings
		}{id, defaults[id]})
	}
	return out
}

// Preferences is the persisted settings document.
type Preferences struct {
	Applications            map[FeatureId]DroidSettings `json:"applications"`
	Theme                   string                      `json:"theme"`
	Appearance              AppearanceSettings          `json:"appearance"`
	Harness                 HarnessSettings             `json:"harness"`
	StartupMode             string                      `json:"startupMode"`
	OpenAtLogin             *bool                       `json:"openAtLogin,omitempty"`
	MenuOrder               []FeatureId                 `json:"menuOrder"`
	MenuOrderVersion        int                         `json:"menuOrderVersion"`
	ApplicationPortsVersion int                         `json:"applicationPortsVersion"`
	// FontOptions 是 deck 设置页和各应用页字体下拉框的选项，只能直接编辑 setting.json。
	FontOptions []string         `json:"fontOptions"`
	Terminal    TerminalSettings `json:"terminal"`
}

// DefaultPreferences returns a fully populated default document.
func DefaultPreferences() Preferences {
	return Preferences{
		Applications:            DefaultApplications(),
		Theme:                   "system",
		Appearance:              DefaultAppearance(),
		Harness:                 DefaultHarnessSettings(),
		StartupMode:             "default",
		MenuOrder:               append([]FeatureId(nil), DefaultMenuOrder...),
		MenuOrderVersion:        MenuOrderVersion,
		ApplicationPortsVersion: ApplicationPortsVersion,
		FontOptions:             DefaultFontOptions(),
		Terminal:                DefaultTerminal(),
	}
}

// ParseFeatureId validates a feature id.
func ParseFeatureId(input any) (FeatureId, error) {
	id, ok := input.(string)
	if !ok {
		return "", fmt.Errorf("应用标识无效")
	}
	for _, known := range DefaultMenuOrder {
		if known == id {
			return id, nil
		}
	}
	return "", fmt.Errorf("应用标识无效")
}

var (
	sizePattern     = `(?:0|[1-9][0-9]{0,3})(?:\.[0-9]+)?`
	widthRegexp     = regexp.MustCompile(`^(?:auto|fit-content|` + sizePattern + `(?:px|rem|em|vw|vh|%))$`)
	maxRegexp       = regexp.MustCompile(`^(?:none|` + sizePattern + `(?:px|rem|em|vw|vh|%))$`)
	sidebarRegexp   = regexp.MustCompile(`^` + sizePattern + `(?:px|rem|em|vw|vh|%)$`)
	heightRegexp    = regexp.MustCompile(`^(?:auto|` + sizePattern + `(?:px|rem|em|vh|%))$`)
	fontControlRune = regexp.MustCompile(`[;{}<>\r\n]`)
)

// FontFamilyError 是应用字体名称无效时的提示。
const FontFamilyError = "请填写字体名称，不能包含 CSS 控制字符"

// ValidFontFamily 检查应用字体名称能否安全写入 CSS。
func ValidFontFamily(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 300 && !fontControlRune.MatchString(value)
}

// SettingsErrors validates per-application settings, mirroring settingsErrors().
func SettingsErrors(v DroidSettings) map[string]string {
	errors := map[string]string{}
	if !widthRegexp.MatchString(v.Width) {
		errors["width"] = "请输入有效宽度，例如 70vw、80% 或 1200px"
	}
	if !sidebarRegexp.MatchString(v.SidebarWidth) {
		errors["sidebarWidth"] = "请输入有效侧栏宽度，例如 15vw、15% 或 280px"
	}
	if !maxRegexp.MatchString(v.MaxWidth) {
		errors["maxWidth"] = "请输入有效最大宽度，例如 90rem 或 1200px"
	}
	if !heightRegexp.MatchString(v.ChatHeight) {
		errors["chatHeight"] = "请输入有效高度，例如 80px"
	}
	if v.FontSize < 8 || v.FontSize > 72 {
		errors["fontSize"] = "字号须为 8–72 的整数"
	}
	if v.FontWeight < 100 || v.FontWeight > 1000 {
		errors["fontWeight"] = "字重须为 100–1000 的整数"
	}
	if !ValidFontFamily(v.FontFamily) {
		errors["fontFamily"] = FontFamilyError
	}
	if v.Port < 1024 || v.Port > 65535 {
		errors["port"] = "端口须为 1024–65535 的整数"
	}
	if strings.ContainsAny(v.ExecutablePath, "\r\n\x00") {
		errors["executablePath"] = "应用路径无效"
	}
	return errors
}

// ParseSettings normalizes and validates one application's settings.
func ParseSettings(input map[string]any, id FeatureId) (DroidSettings, error) {
	if input == nil {
		return DroidSettings{}, fmt.Errorf("设置格式无效")
	}
	defaults := DefaultApplications()
	defaultDroid := defaults[Droid]
	fallback := defaults[id]
	if _, ok := input["sidebarWidth"]; !ok {
		input["sidebarWidth"] = fallback.SidebarWidth
	}
	if _, ok := input["terminalFollow"]; !ok {
		input["terminalFollow"] = fallback.TerminalFollow
	}
	var value DroidSettings
	schema := []struct {
		key      string
		kind     string
		str      *string
		num      *int
		bol      *bool
		fallback any
	}{
		{"width", "string", &value.Width, nil, nil, defaultDroid.Width},
		{"sidebarWidth", "string", &value.SidebarWidth, nil, nil, defaultDroid.SidebarWidth},
		{"maxWidth", "string", &value.MaxWidth, nil, nil, defaultDroid.MaxWidth},
		{"chatHeight", "string", &value.ChatHeight, nil, nil, defaultDroid.ChatHeight},
		{"fontFamily", "string", &value.FontFamily, nil, nil, defaultDroid.FontFamily},
		{"fontSize", "number", nil, &value.FontSize, nil, defaultDroid.FontSize},
		{"fontWeight", "number", nil, &value.FontWeight, nil, defaultDroid.FontWeight},
		{"terminalFollow", "boolean", nil, nil, &value.TerminalFollow, defaultDroid.TerminalFollow},
		{"hideLocalMerge", "boolean", nil, nil, &value.HideLocalMerge, defaultDroid.HideLocalMerge},
		{"hideGitDiff", "boolean", nil, nil, &value.HideGitDiff, defaultDroid.HideGitDiff},
		{"port", "number", nil, &value.Port, nil, defaultDroid.Port},
		{"executablePath", "string", &value.ExecutablePath, nil, nil, defaultDroid.ExecutablePath},
		{"hideChanges", "boolean", nil, nil, &value.HideChanges, defaultDroid.HideChanges},
		{"preventSummary", "boolean", nil, nil, &value.PreventSummary, defaultDroid.PreventSummary},
	}
	for _, field := range schema {
		raw, ok := input[field.key]
		if !ok {
			return DroidSettings{}, fmt.Errorf("设置 %s 的类型无效", field.key)
		}
		var valid bool
		switch field.kind {
		case "string":
			s, is := raw.(string)
			valid = is
			if is {
				*field.str = s
			}
		case "boolean":
			b, is := raw.(bool)
			valid = is
			if is {
				*field.bol = b
			}
		case "number":
			f, is := raw.(float64)
			valid = is && f == float64(int64(f))
			if is && f == float64(int64(f)) {
				*field.num = int(f)
			}
		}
		if !valid {
			return DroidSettings{}, fmt.Errorf("设置 %s 的类型无效", field.key)
		}
	}
	if errs := SettingsErrors(value); len(errs) > 0 {
		// Report errors in a stable order matching the renderer's first-error semantics.
		for _, key := range []string{"width", "sidebarWidth", "maxWidth", "chatHeight", "fontSize", "fontWeight", "fontFamily", "port", "executablePath"} {
			if msg, ok := errs[key]; ok {
				return DroidSettings{}, fmt.Errorf("%s", msg)
			}
		}
		return DroidSettings{}, fmt.Errorf("设置格式无效")
	}
	capability := Applications[id]
	if !capability.Merge {
		value.HideLocalMerge = false
	}
	if !capability.Diff {
		value.HideGitDiff = false
	}
	if !capability.Changes {
		value.HideChanges = false
	}
	if !capability.Summary {
		value.PreventSummary = false
	}
	return value, nil
}

// NormalizeMenuOrder mirrors normalizeMenuOrder().
func NormalizeMenuOrder(input any) []FeatureId {
	var saved []FeatureId
	if list, ok := input.([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				for _, known := range DefaultMenuOrder {
					if known == s {
						saved = append(saved, s)
						break
					}
				}
			}
		}
	}
	if len(saved) > 0 && !contains(saved, Dsh) {
		workbuddy := indexOf(saved, Workbuddy)
		qoder := indexOf(saved, Qoder)
		at := len(saved)
		if workbuddy >= 0 {
			at = workbuddy + 1
		} else if qoder >= 0 {
			at = qoder
		}
		saved = append(saved[:at], append([]FeatureId{Dsh}, saved[at:]...)...)
	}
	if len(saved) > 0 && !contains(saved, Claude) {
		at := len(saved)
		if droid := indexOf(saved, Droid); droid >= 0 {
			at = droid + 1
		} else if zcode := indexOf(saved, Zcode); zcode >= 0 {
			at = zcode
		}
		saved = append(saved[:at], append([]FeatureId{Claude}, saved[at:]...)...)
	}
	merged := append([]FeatureId(nil), saved...)
	for _, id := range DefaultMenuOrder {
		if !contains(merged, id) {
			merged = append(merged, id)
		}
	}
	return merged
}

// ParseMenuOrder validates a complete menu order.
func ParseMenuOrder(input any) ([]FeatureId, error) {
	list, ok := input.([]any)
	if !ok || len(list) != len(DefaultMenuOrder) {
		return nil, fmt.Errorf("应用菜单顺序无效")
	}
	seen := map[string]bool{}
	out := make([]FeatureId, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok || seen[s] || !contains(DefaultMenuOrder, s) {
			return nil, fmt.Errorf("应用菜单顺序无效")
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

// AppearanceErrors validates appearance settings.
func AppearanceErrors(v AppearanceSettings) map[string]string {
	errors := map[string]string{}
	if strings.TrimSpace(v.FontFamily) == "" || len(v.FontFamily) > 300 || fontControlRune.MatchString(v.FontFamily) {
		errors["fontFamily"] = "请输入有效的字体名称"
	}
	if v.FontSize < 10 || v.FontSize > 24 {
		errors["fontSize"] = "字号须为 10–24 的整数"
	}
	if v.Mode != "normal" && v.Mode != "compact" {
		errors["mode"] = "界面模式无效"
	}
	return errors
}

// ParseAppearance validates appearance settings from decoded JSON.
func ParseAppearance(input map[string]any) (AppearanceSettings, error) {
	if input == nil {
		return AppearanceSettings{}, fmt.Errorf("应用字体设置格式无效")
	}
	value := AppearanceSettings{Mode: DefaultAppearance().Mode}
	if raw, ok := input["fontFamily"].(string); ok {
		value.FontFamily = raw
	} else if _, present := input["fontFamily"]; present {
		return AppearanceSettings{}, fmt.Errorf("请输入有效的字体名称")
	} else {
		value.FontFamily = DefaultAppearance().FontFamily
	}
	if IsDefaultFont(value.FontFamily) {
		value.FontFamily = DefaultFontOption
	}
	switch raw := input["fontSize"].(type) {
	case float64:
		if raw != float64(int(raw)) {
			return AppearanceSettings{}, fmt.Errorf("字号须为 10–24 的整数")
		}
		value.FontSize = int(raw)
	case nil:
		if _, present := input["fontSize"]; present {
			return AppearanceSettings{}, fmt.Errorf("字号须为 10–24 的整数")
		}
		value.FontSize = DefaultAppearance().FontSize
	default:
		return AppearanceSettings{}, fmt.Errorf("字号须为 10–24 的整数")
	}
	switch raw := input["mode"].(type) {
	case string:
		value.Mode = raw
	case nil:
		if _, present := input["mode"]; present {
			return AppearanceSettings{}, fmt.Errorf("界面模式无效")
		}
	default:
		return AppearanceSettings{}, fmt.Errorf("界面模式无效")
	}
	if errs := AppearanceErrors(value); len(errs) > 0 {
		for _, key := range []string{"fontFamily", "fontSize", "mode"} {
			if msg, ok := errs[key]; ok {
				return AppearanceSettings{}, fmt.Errorf("%s", msg)
			}
		}
	}
	return value, nil
}

// ParseHarnessSettings validates harness collapse settings.
func ParseHarnessSettings(input map[string]any) (HarnessSettings, error) {
	if input == nil {
		return HarnessSettings{}, fmt.Errorf("Harness 设置格式无效")
	}
	value := DefaultHarnessSettings()
	for key, target := range map[string]*bool{
		"skillsCollapsed": &value.SkillsCollapsed,
		"modelsCollapsed": &value.ModelsCollapsed,
		"mcpsCollapsed":   &value.McpsCollapsed,
	} {
		switch raw := input[key].(type) {
		case bool:
			*target = raw
		case nil:
			if _, present := input[key]; present {
				return HarnessSettings{}, fmt.Errorf("Harness 折叠设置无效")
			}
		default:
			return HarnessSettings{}, fmt.Errorf("Harness 折叠设置无效")
		}
	}
	return value, nil
}

func contains(list []FeatureId, id FeatureId) bool { return indexOf(list, id) >= 0 }

func indexOf(list []FeatureId, id FeatureId) int {
	for i, item := range list {
		if item == id {
			return i
		}
	}
	return -1
}
