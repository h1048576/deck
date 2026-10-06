package widecfg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml/ast"

	"wide-pure/internal/jsonc"
)

// ReservedKey carries the opencode map key inside a model entry (the
// original uses a Symbol; JSON never sees this key because writes strip it).
const ReservedMapKey = "\x00id"

const DefaultModelBaseURL = "http://127.0.0.1:20128"

// CustomModel is one model row shown in the UI.
type CustomModel struct {
	Index    int    `json:"index"`
	Model    string `json:"model"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

// ModelSource groups models from one configuration file.
type ModelSource struct {
	ID       string        `json:"id"`
	Harness  string        `json:"harness"`
	Path     string        `json:"path"`
	Label    string        `json:"label"`
	Models   []CustomModel `json:"models"`
	Editable bool          `json:"editable"`
	BaseURL  string        `json:"baseUrl,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// ModelsInventory wraps all sources.
type ModelsInventory struct {
	Sources []ModelSource `json:"sources"`
}

// ModelFields is the editable model form payload.
type ModelFields struct {
	Model            string  `json:"model"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	BaseURL          *string `json:"baseUrl,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	ReasoningEfforts any     `json:"reasoningEfforts,omitempty"`
}

// ModelTarget identifies one model entry with optimistic concurrency.
type ModelTarget struct {
	SourceID string `json:"sourceId"`
	Index    int    `json:"index"`
	Revision string `json:"revision"`
}

// ModelDetail returns editable fields plus the optional apiKey.
type ModelDetail struct {
	Fields ModelFields `json:"fields"`
	APIKey *string     `json:"apiKey,omitempty"`
}

// ModelChange is a create/update/copy request.
type ModelChange struct {
	SourceID string       `json:"sourceId"`
	Target   *ModelTarget `json:"target,omitempty"`
	CopyFrom *ModelTarget `json:"copyFrom,omitempty"`
	Fields   ModelFields  `json:"fields"`
	APIKey   *string      `json:"apiKey,omitempty"`
}

// ModelDocument is a rendered config preview.
type ModelDocument struct {
	Paths   []string `json:"paths"`
	Content string   `json:"content"`
}

// ModelOrder is a drag reorder request.
type ModelOrder struct {
	SourceID string        `json:"sourceId"`
	Models   []ModelTarget `json:"models"`
}

// ModelBatchChange is a cross-harness one-shot operation.
type ModelBatchChange struct {
	Action        string `json:"action"` // replace | add | delete
	Model         string `json:"model"`
	OriginalModel string `json:"originalModel,omitempty"`
}

// ModelBatchResult summarizes a batch operation.
type ModelBatchResult struct {
	Changed   int      `json:"changed"`
	Skipped   int      `json:"skipped"`
	Harnesses []string `json:"harnesses"`
}

type sourceConfig struct {
	id       string
	harness  string
	path     string
	label    string
	provider string
}

type fileState struct {
	original *string // nil when the file does not exist
	text     string
	bom      bool
}

type parsedFile struct {
	file     fileState
	entries  []*jsonc.Obj
	location []any
	dsh      *dshDoc // non-nil for DSH sources
	baseURL  string
}

type fileChange struct {
	source   sourceConfig
	file     *parsedFile
	entries  []*jsonc.Obj
	index    *int
	deleting bool
	order    []int
	baseURL  *string
	replace  bool
}

var oneMSuffix = regexp.MustCompile(`(?i)\[1m\]`)

var modelKeyOrderMap = map[string][]string{
	"claude":   {"model", "label", "description"},
	"droid":    {"model", "id", "baseUrl", "apiKey", "provider", "displayName"},
	"dsh":      {"id", "name", "reasoningEfforts"},
	"pi":       {"id", "name", "reasoning", "contextWindow"},
	"opencode": {"name"},
}

func entryKeys(item *jsonc.Obj) []string {
	keys := item.Keys()
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == ReservedMapKey {
			continue
		}
		out = append(out, key)
	}
	return out
}

func stripReserved(item *jsonc.Obj) *jsonc.Obj {
	out := item.Clone()
	out.Delete(ReservedMapKey)
	return out
}

func entryRevision(item *jsonc.Obj) string {
	payload, _ := item.Get(ReservedMapKey)
	encoded := "null"
	if payload != nil {
		if text, ok := payload.(string); ok {
			encoded = quoteJSON(text)
		} else {
			encoded = fmt.Sprintf("%v", payload)
		}
	}
	sum := sha256.Sum256([]byte("[" + encoded + "," + jsonc.Canonical(item) + "]"))
	return hex.EncodeToString(sum[:])
}

func quoteJSON(text string) string {
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(text)
	return strings.TrimSuffix(out.String(), "\n")
}

func modelID(harness string, item *jsonc.Obj) string {
	switch harness {
	case "opencode":
		if value, ok := item.Get(ReservedMapKey); ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
		return ""
	case "dsh", "pi":
		if value, ok := item.Get("id"); ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
		return ""
	default:
		if value, ok := item.Get("model"); ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
		return ""
	}
}

func modelName(id string) string {
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}
	return strings.TrimSpace(oneMSuffix.ReplaceAllString(id, ""))
}

func plainModelID(id string) string { return oneMSuffix.ReplaceAllString(id, "") }

func displayName(harness string, item *jsonc.Obj) string {
	for _, key := range []string{"label", "displayName", "name"} {
		if value, ok := item.Get(key); ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
	}
	if id := modelID(harness, item); id != "" {
		return id
	}
	return "未命名模型"
}

// ModelsManager edits custom models across all harness configuration files.
type ModelsManager struct {
	home     string
	backups  string
	mutating bool
}

func NewModelsManager(home, backups string) *ModelsManager {
	return &ModelsManager{home: home, backups: backups}
}

func (m *ModelsManager) jsonSource(harness string) (sourceConfig, error) {
	folder := filepath.Join(m.home, map[string]string{"claude": ".claude", "droid": ".factory"}[harness])
	path := filepath.Join(folder, "settings.json")
	if _, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			return sourceConfig{}, err
		}
		alternative := filepath.Join(folder, "setting.json")
		if _, err := os.Lstat(alternative); err == nil {
			path = alternative
		} else if !os.IsNotExist(err) {
			return sourceConfig{}, err
		}
	}
	return sourceConfig{id: harness, harness: harness, path: path, label: harness}, nil
}

func (m *ModelsManager) dshPath(profile string) string {
	return filepath.Join(m.home, ".dsh", "profiles", profile, "cordis.patch.yml")
}

func (m *ModelsManager) read(path string, allowMissing bool) (fileState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if allowMissing && os.IsNotExist(err) {
			return fileState{text: "{}\n"}, nil
		}
		return fileState{}, err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, fmt.Errorf("模型配置须为普通文件，暂不支持符号链接")
	}
	if info.Size() > 8*1024*1024 {
		return fileState{}, fmt.Errorf("模型配置超过 8 MB，无法读取")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileState{}, err
	}
	original := string(data)
	bom := len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF
	text := original
	if bom {
		text = string(data[3:])
	}
	return fileState{original: &original, text: text, bom: bom}, nil
}

func parseJSONC(text string) (*jsonc.Obj, error) {
	value, err := jsonc.Parse(text)
	if err != nil {
		return nil, fmt.Errorf("JSON 配置格式无效，请先修复配置文件")
	}
	obj, ok := value.(*jsonc.Obj)
	if !ok {
		return nil, fmt.Errorf("JSON 配置格式无效，请先修复配置文件")
	}
	return obj, nil
}

func (m *ModelsManager) sources() ([]sourceConfig, error) {
	claude, err := m.jsonSource("claude")
	if err != nil {
		return nil, err
	}
	droid, err := m.jsonSource("droid")
	if err != nil {
		return nil, err
	}
	extra := []sourceConfig{
		{id: "pi", harness: "pi", path: filepath.Join(m.home, ".pi", "agent", "models.json"), label: "proxy", provider: "proxy"},
		{id: "opencode", harness: "opencode", path: filepath.Join(m.home, ".config", "opencode", "opencode.json"), label: "proxy", provider: "proxy"},
	}
	path := m.dshPath("desktop")
	out := []sourceConfig{claude, droid}
	if parsed, err := m.parseDshFile(path, false); err == nil {
		names := parsed.dsh.providerNames()
		if len(names) == 0 {
			out = append(out, sourceConfig{id: "dsh", harness: "dsh", path: path, label: "dsh"})
		}
		for _, name := range names {
			out = append(out, sourceConfig{id: "dsh:" + name, harness: "dsh", path: path, label: name, provider: name})
		}
	} else {
		out = append(out, sourceConfig{id: "dsh", harness: "dsh", path: path, label: "dsh"})
	}
	return append(out, extra...), nil
}

func (m *ModelsManager) source(id any) (sourceConfig, error) {
	text, ok := id.(string)
	if !ok {
		return sourceConfig{}, fmt.Errorf("模型配置标识无效")
	}
	found, err := m.sources()
	if err != nil {
		return sourceConfig{}, err
	}
	for _, item := range found {
		if item.id == text {
			return item, nil
		}
	}
	return sourceConfig{}, fmt.Errorf("模型配置已变化，请刷新后重试")
}

func (m *ModelsManager) parseDshFile(path string, allowMissing bool) (*parsedFile, error) {
	file, err := m.read(path, allowMissing)
	if err != nil {
		return nil, err
	}
	return m.parseDshState(file)
}

func (m *ModelsManager) parseDshState(file fileState) (*parsedFile, error) {
	parsed, err := parseDsh(file.text)
	if err != nil {
		return nil, err
	}
	return &parsedFile{file: file, dsh: parsed}, nil
}

func (m *ModelsManager) load(source sourceConfig) (*parsedFile, error) {
	allowMissing := source.harness == "claude" || source.harness == "droid"
	file, err := m.read(source.path, allowMissing)
	if err != nil {
		return nil, err
	}
	return m.parseFile(source, file)
}

func (m *ModelsManager) parseFile(source sourceConfig, file fileState) (*parsedFile, error) {
	if source.harness == "dsh" {
		parsed, err := parseDsh(file.text)
		if err != nil {
			return nil, err
		}
		if source.provider == "" {
			return nil, fmt.Errorf("尚未配置模型提供商")
		}
		providerNode, err := parsed.provider(source.provider)
		if err != nil {
			return nil, err
		}
		decoded, err := yamlToValue(providerNode)
		if err != nil {
			return nil, fmt.Errorf("models 格式无效，须为模型对象列表")
		}
		providerMap, ok := decoded.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("尚未配置模型提供商")
		}
		if _, has := providerMap["modelOverrides"]; has {
			return nil, fmt.Errorf("此提供商使用 modelOverrides，请先改为 models 后再管理自定义模型")
		}
		entries := []*jsonc.Obj{}
		if rawModels, has := providerMap["models"]; has {
			list, ok := rawModels.([]any)
			if !ok {
				return nil, fmt.Errorf("models 格式无效，须为模型对象列表")
			}
			for _, item := range list {
				obj, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("models 格式无效，须为模型对象列表")
				}
				entries = append(entries, objFromMap(obj))
			}
		}
		baseURL := DefaultModelBaseURL
		if text, ok := providerMap["baseURL"].(string); ok && text != "" {
			baseURL = text
		}
		location := []any{parsed.PluginIndex(), "config", "providers", source.provider, "models"}
		return &parsedFile{file: file, entries: entries, location: location, dsh: parsed, baseURL: baseURL}, nil
	}
	root, err := parseJSONC(file.text)
	if err != nil {
		return nil, err
	}
	switch source.harness {
	case "pi", "opencode":
		location := []any{"providers", "proxy", "models"}
		lookupKey := "providers"
		if source.harness == "opencode" {
			location = []any{"provider", "proxy", "models"}
			lookupKey = "provider"
		}
		providerRaw, _ := root.Get(lookupKey)
		provider, ok := providerRaw.(*jsonc.Obj)
		if !ok {
			return nil, fmt.Errorf("尚未配置 proxy 模型提供商")
		}
		proxyRaw, _ := provider.Get("proxy")
		proxy, ok := proxyRaw.(*jsonc.Obj)
		if !ok {
			return nil, fmt.Errorf("尚未配置 proxy 模型提供商")
		}
		baseURL := DefaultModelBaseURL
		entries := []*jsonc.Obj{}
		if source.harness == "pi" {
			modelsRaw, has := proxy.Get("models")
			if !has {
				modelsRaw = []any{}
			}
			list, ok := modelsRaw.([]any)
			if !ok {
				return nil, fmt.Errorf("pi models 须为模型对象列表")
			}
			for _, item := range list {
				obj, ok := item.(*jsonc.Obj)
				if !ok {
					return nil, fmt.Errorf("pi models 须为模型对象列表")
				}
				entries = append(entries, obj)
			}
			if value, ok := proxy.Get("baseUrl"); ok {
				if text, ok := value.(string); ok && text != "" {
					baseURL = text
				}
			}
		} else {
			modelsRaw, has := proxy.Get("models")
			if !has {
				modelsRaw = jsonc.NewObj()
			}
			models, ok := modelsRaw.(*jsonc.Obj)
			if !ok {
				return nil, fmt.Errorf("opencode models 须为以模型 ID 为键的对象")
			}
			for _, key := range models.Keys() {
				value, _ := models.Get(key)
				obj, ok := value.(*jsonc.Obj)
				if !ok {
					return nil, fmt.Errorf("opencode models 须为以模型 ID 为键的对象")
				}
				entry := obj.Clone()
				entry.Set(ReservedMapKey, key)
				entries = append(entries, entry)
			}
			if optionsRaw, ok := proxy.Get("options"); ok {
				if options, ok := optionsRaw.(*jsonc.Obj); ok {
					if value, ok := options.Get("baseURL"); ok {
						if text, ok := value.(string); ok && text != "" {
							baseURL = text
						}
					}
				}
			}
		}
		return &parsedFile{file: file, entries: entries, location: location, baseURL: baseURL}, nil
	}
	// claude / droid
	if source.harness == "claude" {
		if raw, has := root.Get("modelPicker"); has && raw != nil {
			if _, ok := raw.(*jsonc.Obj); !ok {
				return nil, fmt.Errorf("modelPicker 须为对象")
			}
		}
	}
	entries := []*jsonc.Obj{}
	location := []any{"customModels"}
	if source.harness == "claude" {
		location = []any{"modelPicker", "options"}
		if pickerRaw, has := root.Get("modelPicker"); has {
			if picker, ok := pickerRaw.(*jsonc.Obj); ok {
				if optionsRaw, has := picker.Get("options"); has {
					list, ok := optionsRaw.([]any)
					if !ok {
						return nil, fmt.Errorf("自定义模型配置须为模型对象列表")
					}
					for _, item := range list {
						obj, ok := item.(*jsonc.Obj)
						if !ok {
							return nil, fmt.Errorf("自定义模型配置须为模型对象列表")
						}
						entries = append(entries, obj)
					}
				}
			}
		}
	} else {
		if modelsRaw, has := root.Get("customModels"); has {
			list, ok := modelsRaw.([]any)
			if !ok {
				return nil, fmt.Errorf("自定义模型配置须为模型对象列表")
			}
			for _, item := range list {
				obj, ok := item.(*jsonc.Obj)
				if !ok {
					return nil, fmt.Errorf("自定义模型配置须为模型对象列表")
				}
				entries = append(entries, obj)
			}
		}
	}
	return &parsedFile{file: file, entries: entries, location: location}, nil
}

func objFromMap(data map[string]any) *jsonc.Obj {
	obj := jsonc.NewObj()
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		obj.Set(key, normalizeValue(data[key]))
	}
	return obj
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return objFromMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return value
	}
}

// Inventory lists every source with its models.
func (m *ModelsManager) Inventory() ModelsInventory {
	sources, err := m.sources()
	if err != nil {
		return ModelsInventory{Sources: []ModelSource{}}
	}
	out := make([]ModelSource, 0, len(sources))
	for _, source := range sources {
		out = append(out, m.sourceSummary(source))
	}
	return ModelsInventory{Sources: out}
}

func missingFile(err error) bool {
	return os.IsNotExist(err) || strings.Contains(err.Error(), "cannot find the file") || strings.Contains(err.Error(), "The system cannot find")
}

func (m *ModelsManager) sourceSummary(source sourceConfig) ModelSource {
	parsed, err := m.load(source)
	if err == nil && source.harness == "dsh" {
		// 校验 web profile 也能解析。
		_, err = m.parseDshFile(m.dshPath("web"), false)
	}
	if err != nil {
		message := err.Error()
		if missingFile(err) {
			message = "未找到配置文件"
		} else if strings.Contains(err.Error(), "读取模型失败") {
			message = "读取模型失败"
		}
		return ModelSource{ID: source.id, Harness: source.harness, Path: source.path, Label: source.label, Models: []CustomModel{}, Editable: false, Error: message}
	}
	models := make([]CustomModel, 0, len(parsed.entries))
	for index, item := range parsed.entries {
		models = append(models, CustomModel{
			Index:    index,
			Model:    modelID(source.harness, item),
			Name:     displayName(source.harness, item),
			Revision: entryRevision(item),
		})
	}
	return ModelSource{ID: source.id, Harness: source.harness, Path: source.path, Label: source.label, Models: models, Editable: true, BaseURL: parsed.baseURL}
}

func (m *ModelsManager) target(input any) (ModelTarget, error) {
	change, ok := input.(ModelTarget)
	if !ok || change.Index < 0 {
		return ModelTarget{}, fmt.Errorf("模型标识无效")
	}
	return change, nil
}

func (m *ModelsManager) entry(entries []*jsonc.Obj, target ModelTarget) (*jsonc.Obj, error) {
	if target.Index >= len(entries) {
		return nil, fmt.Errorf("模型已被其他程序修改，请刷新后重试")
	}
	item := entries[target.Index]
	if entryRevision(item) != target.Revision {
		return nil, fmt.Errorf("模型已被其他程序修改，请刷新后重试")
	}
	return item, nil
}

// Detail returns the editable fields of one model.
func (m *ModelsManager) Detail(input any) (ModelDetail, error) {
	target, err := m.target(input)
	if err != nil {
		return ModelDetail{}, err
	}
	source, err := m.source(target.SourceID)
	if err != nil {
		return ModelDetail{}, err
	}
	file, err := m.load(source)
	if err != nil {
		return ModelDetail{}, err
	}
	item, err := m.entry(file.entries, target)
	if err != nil {
		return ModelDetail{}, err
	}
	fields := ModelFields{Model: modelID(source.harness, item), Name: displayName(source.harness, item)}
	if value, ok := item.Get("description"); ok {
		if text, ok := value.(string); ok {
			fields.Description = &text
		}
	}
	switch source.harness {
	case "droid":
		baseURL := DefaultModelBaseURL
		if value, ok := item.Get("baseUrl"); ok {
			if text, ok := value.(string); ok {
				baseURL = text
			}
		}
		fields.BaseURL = &baseURL
		if value, ok := item.Get("provider"); ok {
			if text, ok := value.(string); ok {
				fields.Provider = &text
			}
		}
	case "dsh":
		fields.BaseURL = &file.baseURL
		if value, ok := item.Get("reasoningEfforts"); ok {
			fields.ReasoningEfforts = jsonc.Plain(value)
		}
	case "pi", "opencode":
		baseURL := file.baseURL
		fields.BaseURL = &baseURL
	}
	detail := ModelDetail{Fields: fields}
	if source.harness == "droid" {
		apiKey := ""
		if value, ok := item.Get("apiKey"); ok {
			if text, ok := value.(string); ok {
				apiKey = text
			}
		}
		detail.APIKey = &apiKey
	}
	return detail, nil
}

// Preview renders the raw configuration of one source.
func (m *ModelsManager) Preview(id any) (ModelDocument, error) {
	source, err := m.source(id)
	if err != nil {
		return ModelDocument{}, err
	}
	file, err := m.load(source)
	if err != nil {
		return ModelDocument{}, err
	}
	paths := []string{source.path}
	if source.harness == "dsh" {
		paths = []string{source.path, m.dshPath("web")}
	}
	if file.dsh != nil {
		// 预览完整模型插件，保留 YAML 层级、提供商顺序和注释。
		clone, err := parseYamlDoc(file.file.text)
		if err != nil {
			return ModelDocument{}, err
		}
		if seq, ok := clone.Root().(*ast.SequenceNode); ok && file.dsh.PluginIndex() < len(seq.Values) {
			seq.Values = []ast.Node{seq.Values[file.dsh.PluginIndex()]}
		}
		return ModelDocument{Paths: paths, Content: clone.String()}, nil
	}
	var value any
	switch source.harness {
	case "claude":
		picker := jsonc.NewObj()
		options := make([]any, 0, len(file.entries))
		for _, item := range file.entries {
			options = append(options, item)
		}
		picker.Set("options", options)
		root := jsonc.NewObj()
		root.Set("modelPicker", picker)
		value = root
	case "droid":
		root := jsonc.NewObj()
		options := make([]any, 0, len(file.entries))
		for _, item := range file.entries {
			options = append(options, item)
		}
		root.Set("customModels", options)
		value = root
	case "pi":
		models := make([]any, 0, len(file.entries))
		for _, item := range file.entries {
			models = append(models, item)
		}
		proxy := jsonc.NewObj()
		proxy.Set("models", models)
		providers := jsonc.NewObj()
		providers.Set("proxy", proxy)
		root := jsonc.NewObj()
		root.Set("providers", providers)
		value = root
	default: // opencode
		models := jsonc.NewObj()
		for _, item := range file.entries {
			key, _ := item.Get(ReservedMapKey)
			if text, ok := key.(string); ok {
				models.Set(text, stripReserved(item))
			}
		}
		proxy := jsonc.NewObj()
		proxy.Set("models", models)
		provider := jsonc.NewObj()
		provider.Set("proxy", proxy)
		root := jsonc.NewObj()
		root.Set("provider", provider)
		value = root
	}
	return ModelDocument{Paths: paths, Content: jsonc.Pretty(value)}, nil
}

func (m *ModelsManager) model(harness string, fields ModelFields, old *jsonc.Obj, apiKey *string) (*jsonc.Obj, error) {
	model := strings.TrimSpace(fields.Model)
	if model == "" || len(model) > 1024 || strings.ContainsAny(model, "\x00\r\n") {
		return nil, fmt.Errorf("请输入有效的模型 ID")
	}
	if len(fields.Name) > 1024 {
		return nil, fmt.Errorf("模型名称无效")
	}
	next := old.Clone()
	set := func(key string, value any) {
		if value == nil || value == "" {
			next.Delete(key)
			return
		}
		next.Set(key, value)
	}
	switch harness {
	case "claude":
		next.Set("model", model)
		set("label", strings.TrimSpace(fields.Name))
		if fields.Description != nil {
			set("description", strings.TrimSpace(*fields.Description))
		} else {
			next.Delete("description")
		}
	case "droid":
		next.Set("model", model)
		set("displayName", strings.TrimSpace(fields.Name))
		provider := ""
		if fields.Provider != nil {
			provider = *fields.Provider
		}
		if provider != "openai" && provider != "anthropic" && provider != "generic-chat-completion-api" {
			return nil, fmt.Errorf("请选择有效的 Provider")
		}
		next.Set("provider", provider)
		baseURL, err := apiAddress(fields.BaseURL)
		if err != nil {
			return nil, err
		}
		set("baseUrl", baseURL)
		if apiKey != nil {
			if strings.ContainsAny(*apiKey, "\x00\r\n") {
				return nil, fmt.Errorf("API Key 无效")
			}
			set("apiKey", strings.TrimSpace(*apiKey))
		}
		// 补齐新增或复制模型的 ID；已有自动生成的 ID 随模型 ID 更新。
		oldID, _ := old.Get("id")
		oldModel, _ := old.Get("model")
		oldModelText, _ := oldModel.(string)
		idText, _ := oldID.(string)
		if strings.TrimSpace(idText) == "" || idText == "custom:"+oldModelText {
			next.Set("id", "custom:"+model)
		}
	case "dsh", "pi":
		next.Set("id", model)
		set("name", strings.TrimSpace(fields.Name))
		if harness == "dsh" {
			if fields.ReasoningEfforts != nil {
				if efforts := fields.ReasoningEfforts; efforts != false {
					obj, ok := efforts.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("请填写有效的思考级别 key、value")
					}
					for key, value := range obj {
						if strings.TrimSpace(key) == "" || key == "__proto__" || key == "constructor" || key == "prototype" {
							return nil, fmt.Errorf("请填写有效的思考级别 key、value")
						}
						if value != nil {
							if _, ok := value.(string); !ok {
								return nil, fmt.Errorf("请填写有效的思考级别 key、value")
							}
						}
					}
				}
			}
			set("reasoningEfforts", fields.ReasoningEfforts)
		}
	default: // opencode
		next.Set(ReservedMapKey, model)
		set("name", strings.TrimSpace(fields.Name))
		if oldID, ok := old.Get("id"); ok {
			if text, ok := oldID.(string); ok {
				if mapKey, _ := old.Get(ReservedMapKey); fmt.Sprintf("%v", mapKey) == text {
					next.Set("id", model)
				}
			}
		}
	}
	return next, nil
}

func (m *ModelsManager) ordered(harness string, item *jsonc.Obj) *jsonc.Obj {
	next := jsonc.NewObj()
	seen := map[string]bool{}
	for _, key := range modelKeyOrderMap[harness] {
		if value, ok := item.Get(key); ok {
			next.Set(key, value)
			seen[key] = true
		}
	}
	for _, key := range item.Keys() {
		if key == ReservedMapKey || seen[key] {
			continue
		}
		if value, ok := item.Get(key); ok {
			next.Set(key, value)
		}
	}
	if value, ok := item.Get(ReservedMapKey); ok {
		next.Set(ReservedMapKey, value)
	}
	return next
}

func apiAddress(input *string) (string, error) {
	if input == nil || strings.TrimSpace(*input) == "" {
		return "", fmt.Errorf("请填写 API 地址")
	}
	value := strings.TrimSpace(*input)
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return "", fmt.Errorf("API 地址须以 http:// 或 https:// 开头")
	}
	return value, nil
}

func (m *ModelsManager) exclusive(task func() error) error {
	if m.mutating {
		return fmt.Errorf("正在写入模型配置，请稍后再试")
	}
	m.mutating = true
	defer func() { m.mutating = false }()
	return task()
}
