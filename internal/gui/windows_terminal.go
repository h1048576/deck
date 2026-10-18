package gui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"deck/internal/appdir"
	"deck/internal/jsonc"
	"deck/internal/settings"
)

// terminalDefaultFace 是 Windows Terminal 自带的默认字体。
// default 在 deck 和受管应用里表示系统界面字体，但那是比例字体，终端里会错位，所以终端改用自带的默认字体。
const terminalDefaultFace = "Cascadia Mono"

var windowsEnvPattern = regexp.MustCompile(`%([A-Za-z0-9_()]+)%`)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// terminalConfig 是一个 Windows Terminal 配置文件的状态。
type terminalConfig struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Resolved string  `json:"resolved"`
	Exists   bool    `json:"exists"`
	Face     string  `json:"face"`
	FontSize float64 `json:"fontSize,omitempty"`
	// FontWeight 可能是数字，也可能是 "normal"、"bold" 这类名称。
	FontWeight string `json:"fontWeight,omitempty"`
	Size       string `json:"size"`
	Error      string `json:"error,omitempty"`
}

// terminalEdit 是一次要写入 Windows Terminal 配置的字段。
type terminalEdit struct {
	path  []any
	value any
}

// terminalApplyResult 汇总一次字体修改在各配置文件上的结果。
type terminalApplyResult struct {
	Success bool             `json:"success"`
	Message string           `json:"message"`
	Configs []terminalConfig `json:"configs"`
}

// expandWindowsPath 展开 %LOCALAPPDATA% 这类 Windows 环境变量，未定义的变量原样保留。
func expandWindowsPath(path string) string {
	expanded := windowsEnvPattern.ReplaceAllStringFunc(path, func(match string) string {
		if value, ok := os.LookupEnv(match[1 : len(match)-1]); ok {
			return value
		}
		return match
	})
	return filepath.Clean(expanded)
}

func terminalName(resolved string) string {
	lower := strings.ToLower(resolved)
	switch {
	case strings.Contains(lower, `\microsoft.windowsterminalpreview_`):
		return "Windows Terminal Preview"
	case strings.Contains(lower, `\microsoft.windowsterminalcanary_`):
		return "Windows Terminal Canary"
	case strings.Contains(lower, `\microsoft.windowsterminal_`):
		return "Windows Terminal"
	}
	dir := filepath.Dir(resolved)
	if strings.EqualFold(filepath.Base(dir), "LocalState") {
		dir = filepath.Dir(dir)
	}
	return filepath.Base(dir)
}

// terminalFace 把 deck 的字体设置转换成 Windows Terminal 的 face，多个字体用英文逗号分隔。
func terminalFace(fontFamily string) string {
	if settings.IsDefaultFont(fontFamily) {
		return terminalDefaultFace
	}
	names := make([]string, 0, 4)
	for _, name := range strings.Split(fontFamily, ",") {
		if name = strings.Trim(strings.TrimSpace(name), `'"`); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func loadTerminalConfig(path string) (terminalConfig, []byte, *jsonc.Doc) {
	resolved := expandWindowsPath(path)
	config := terminalConfig{Name: terminalName(resolved), Path: path, Resolved: resolved}
	data, err := os.ReadFile(resolved)
	if err != nil {
		if !os.IsNotExist(err) {
			config.Error = "无法读取配置文件：" + err.Error()
		}
		return config, nil, nil
	}
	config.Exists = true
	doc, err := jsonc.ParseDoc(string(bytes.TrimPrefix(data, utf8BOM)))
	if err != nil {
		config.Error = "配置文件格式无法解析"
		return config, data, nil
	}
	readTerminalState(&config, doc)
	return config, data, doc
}

func readTerminalState(config *terminalConfig, doc *jsonc.Doc) {
	config.Face, config.FontSize, config.FontWeight, config.Size = "", 0, "", ""
	if weight, ok := doc.Get("profiles", "defaults", "font", "weight"); ok {
		switch value := weight.(type) {
		case float64:
			config.FontWeight = fmt.Sprintf("%g", value)
		case string:
			config.FontWeight = value
		}
	}
	if face, ok := doc.Get("profiles", "defaults", "font", "face"); ok {
		config.Face, _ = face.(string)
	}
	if size, ok := doc.Get("profiles", "defaults", "font", "size"); ok {
		config.FontSize, _ = size.(float64)
	}
	cols, colsOK := doc.Get("initialCols")
	rows, rowsOK := doc.Get("initialRows")
	if colsOK || rowsOK {
		config.Size = fmt.Sprintf("%vx%v", terminalSizeValue(cols, colsOK), terminalSizeValue(rows, rowsOK))
	}
}

func terminalSizeValue(value any, ok bool) string {
	if number, isNumber := value.(float64); ok && isNumber {
		return fmt.Sprintf("%g", number)
	}
	return "?"
}

func terminalConfigs(paths []string) []terminalConfig {
	configs := make([]terminalConfig, 0, len(paths))
	for _, path := range paths {
		config, _, _ := loadTerminalConfig(path)
		configs = append(configs, config)
	}
	return configs
}

// terminalFontTargets 列出需要修改的字体字段，key 是 font 对象里的字段名，legacyKey 是旧版平铺写法的字段名。
// profiles.defaults.font.<key> 不存在时会被创建；单个 profile 自己设置的值会覆盖默认值，存在时一起修改。
func terminalFontTargets(doc *jsonc.Doc, key string, legacyKey string) [][]any {
	targets := [][]any{}
	var list []any
	var prefix []any
	profiles, _ := doc.Get("profiles")
	switch value := profiles.(type) {
	case []any:
		// 旧版配置的 profiles 直接是数组，没有 defaults。
		list, prefix = value, []any{"profiles"}
	default:
		targets = append(targets, []any{"profiles", "defaults", "font", key})
		if object, ok := value.(*jsonc.Obj); ok {
			if defaults, ok := object.Get("defaults"); ok {
				if defaultsObject, ok := defaults.(*jsonc.Obj); ok && defaultsObject.Has(legacyKey) {
					targets = append(targets, []any{"profiles", "defaults", legacyKey})
				}
			}
			if items, ok := object.Get("list"); ok {
				list, _ = items.([]any)
				prefix = []any{"profiles", "list"}
			}
		}
	}
	for index, item := range list {
		profile, ok := item.(*jsonc.Obj)
		if !ok {
			continue
		}
		if font, ok := profile.Get("font"); ok {
			if fontObject, ok := font.(*jsonc.Obj); ok && fontObject.Has(key) {
				targets = append(targets, append(slices.Clone(prefix), index, "font", key))
			}
		}
		if profile.Has(legacyKey) {
			targets = append(targets, append(slices.Clone(prefix), index, legacyKey))
		}
	}
	return targets
}

func terminalFontEdits(key string, legacyKey string, value any) func(doc *jsonc.Doc) []terminalEdit {
	return func(doc *jsonc.Doc) []terminalEdit {
		targets := terminalFontTargets(doc, key, legacyKey)
		edits := make([]terminalEdit, 0, len(targets))
		for _, target := range targets {
			edits = append(edits, terminalEdit{path: target, value: value})
		}
		return edits
	}
}

// terminalSizeEdits 修改根节点的 initialCols 和 initialRows，Windows Terminal 用它们决定新窗口的列数和行数。
func terminalSizeEdits(cols int, rows int) func(doc *jsonc.Doc) []terminalEdit {
	return func(*jsonc.Doc) []terminalEdit {
		return []terminalEdit{
			{path: []any{"initialCols"}, value: float64(cols)},
			{path: []any{"initialRows"}, value: float64(rows)},
		}
	}
}

// applyTerminalEdits 修改一个配置文件，返回 updated、unchanged、missing 或 error。
func applyTerminalEdits(path string, editsOf func(doc *jsonc.Doc) []terminalEdit) (terminalConfig, string) {
	config, data, doc := loadTerminalConfig(path)
	if config.Error != "" {
		return config, "error"
	}
	if !config.Exists {
		return config, "missing"
	}
	text := string(bytes.TrimPrefix(data, utf8BOM))
	format := jsonc.DetectFormatOf(text)
	updated := text
	for _, edit := range editsOf(doc) {
		current, err := jsonc.ParseDoc(updated)
		if err != nil {
			config.Error = "配置文件格式无法解析"
			return config, "error"
		}
		if value, ok := current.Get(edit.path...); ok && value == edit.value {
			continue
		}
		if updated, err = current.Set(edit.path, edit.value, format); err != nil {
			config.Error = "无法修改配置字段：" + err.Error()
			return config, "error"
		}
	}
	if updated == text {
		return config, "unchanged"
	}
	output := []byte(updated)
	if bytes.HasPrefix(data, utf8BOM) {
		output = append(slices.Clone(utf8BOM), output...)
	}
	if err := writeTerminalConfig(config, data, output); err != nil {
		config.Error = err.Error()
		return config, "error"
	}
	if current, err := jsonc.ParseDoc(updated); err == nil {
		readTerminalState(&config, current)
	}
	return config, "updated"
}

// writeTerminalConfig 先备份原文件，再用同目录临时文件替换，避免 Windows Terminal 读到写了一半的配置。
func writeTerminalConfig(config terminalConfig, original []byte, output []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(config.Resolved), ".deck-terminal-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(output); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(appdir.TerminalBackups(), 0o755); err != nil {
		return err
	}
	backup := filepath.Join(appdir.TerminalBackups(), fmt.Sprintf("%s-%d.json", strings.ReplaceAll(config.Name, " ", "-"), time.Now().UnixMilli()))
	if err := os.WriteFile(backup, original, 0o600); err != nil {
		return err
	}
	// Windows Terminal 在设置界面保存时也会写这个文件，替换前确认内容没变，避免覆盖它刚写入的修改。
	current, err := os.ReadFile(config.Resolved)
	if err != nil || !bytes.Equal(current, original) {
		return fmt.Errorf("配置文件刚被其他程序修改，本次未写入，请重试")
	}
	return os.Rename(temporary.Name(), config.Resolved)
}

// applyTerminalFont 把字体写入所有已配置的 Windows Terminal 配置文件。
func applyTerminalFont(paths []string, fontFamily string) terminalApplyResult {
	return applyTerminalSettings(paths, "已是该字体", terminalFontEdits("face", "fontFace", terminalFace(fontFamily)))
}

// applyTerminalFontWeight 把字重写入所有已配置的 Windows Terminal 配置文件。
func applyTerminalFontWeight(paths []string, fontWeight int) terminalApplyResult {
	return applyTerminalSettings(paths, "已是该字重", terminalFontEdits("weight", "fontWeight", float64(fontWeight)))
}

// applyTerminalFontSize 把字号写入所有已配置的 Windows Terminal 配置文件。
func applyTerminalFontSize(paths []string, fontSize int) terminalApplyResult {
	return applyTerminalSettings(paths, "已是该字号", terminalFontEdits("size", "fontSize", float64(fontSize)))
}

// applyTerminalWindowSize 把窗口大小写入所有已配置的 Windows Terminal 配置文件。
func applyTerminalWindowSize(paths []string, cols int, rows int) terminalApplyResult {
	return applyTerminalSettings(paths, "已是该窗口大小", terminalSizeEdits(cols, rows))
}

func applyTerminalSettings(paths []string, unchangedText string, editsOf func(doc *jsonc.Doc) []terminalEdit) terminalApplyResult {
	result := terminalApplyResult{Success: true, Configs: make([]terminalConfig, 0, len(paths))}
	updated, unchanged, missing := 0, 0, 0
	failures := []string{}
	for _, path := range paths {
		config, status := applyTerminalEdits(path, editsOf)
		result.Configs = append(result.Configs, config)
		switch status {
		case "updated":
			updated++
		case "unchanged":
			unchanged++
		case "missing":
			missing++
		default:
			failures = append(failures, fmt.Sprintf("%s 更新失败：%s", config.Name, config.Error))
		}
	}
	if updated+unchanged == 0 && len(failures) == 0 {
		result.Success = false
		result.Message = "没有找到 Windows Terminal 配置文件，请检查 setting.json 中的 terminal.settingsPaths。"
		return result
	}
	parts := []string{}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("已更新 %d 个 Windows Terminal 配置", updated))
	}
	if unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d 个%s", unchanged, unchangedText))
	}
	if missing > 0 {
		parts = append(parts, fmt.Sprintf("%d 个未安装，已跳过", missing))
	}
	if len(failures) > 0 {
		result.Success = false
		parts = append(parts, failures...)
	}
	result.Message = strings.Join(parts, "；") + "。"
	return result
}
