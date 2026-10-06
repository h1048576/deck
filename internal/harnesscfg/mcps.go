package harnesscfg

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"deck/internal/jsonc"
)

// McpHarnessId identifies an MCP-capable harness.
type McpHarnessId = string

// McpServer is one server row in the UI.
type McpServer struct {
	Name        string `json:"name"`
	Transport   string `json:"transport"`
	Description string `json:"description"`
	Revision    string `json:"revision"`
}

// McpSource groups servers from one config file.
type McpSource struct {
	Harness  string      `json:"harness"`
	Path     string      `json:"path"`
	Servers  []McpServer `json:"servers"`
	Editable bool        `json:"editable"`
	Error    string      `json:"error,omitempty"`
}

// McpsInventory wraps both sources.
type McpsInventory struct {
	Sources []McpSource `json:"sources"`
}

// McpTarget identifies one MCP entry.
type McpTarget struct {
	Harness  string `json:"harness"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

// McpFields is the editable MCP form payload.
type McpFields struct {
	Name              string            `json:"name"`
	Transport         string            `json:"transport"`
	Command           string            `json:"command"`
	Args              []string          `json:"args"`
	Env               map[string]string `json:"env"`
	URL               string            `json:"url"`
	Headers           map[string]string `json:"headers"`
	Cwd               string            `json:"cwd"`
	BearerTokenEnvVar string            `json:"bearerTokenEnvVar"`
	EnvHeaders        map[string]string `json:"envHeaders"`
}

// McpChange is a create/update/copy request.
type McpChange struct {
	Harness  string     `json:"harness"`
	Target   *McpTarget `json:"target,omitempty"`
	CopyFrom *McpTarget `json:"copyFrom,omitempty"`
	Fields   McpFields  `json:"fields"`
}

// McpDocument is a rendered config preview.
type McpDocument struct {
	Path    string `json:"path"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

// McpsManager edits MCP servers in Claude and Codex configs.
type McpsManager struct {
	home     string
	backups  string
	paths    map[string]string
	mutating bool
}

func NewMcpsManager(home, backups string, paths map[string]string) *McpsManager {
	if paths == nil {
		paths = map[string]string{}
	}
	return &McpsManager{home: home, backups: backups, paths: paths}
}

func (s *McpsManager) path(id string) string {
	if custom, ok := s.paths[id]; ok && custom != "" {
		return custom
	}
	if id == "claude" {
		return filepath.Join(s.home, ".claude.json")
	}
	return filepath.Join(s.home, ".codex", "config.toml")
}

func (s *McpsManager) key(id string) string {
	if id == "claude" {
		return "mcpServers"
	}
	return "mcp_servers"
}

type mcpFile struct {
	path     string
	original *string
	text     string
	bom      bool
}

func (s *McpsManager) read(id string) (mcpFile, error) {
	path := s.path(id)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			text := "{}\n"
			if id != "claude" {
				text = ""
			}
			return mcpFile{path: path, text: text}, nil
		}
		return mcpFile{}, err
	}
	if !info.Mode().IsRegular() {
		return mcpFile{}, fmt.Errorf("MCP 配置须为普通文件，暂不支持符号链接")
	}
	if info.Size() > 16*1024*1024 {
		return mcpFile{}, fmt.Errorf("MCP 配置超过 16 MB，无法读取")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return mcpFile{}, err
	}
	original := string(data)
	bom := len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF
	text := original
	if bom {
		text = string(data[3:])
	}
	return mcpFile{path: path, original: &original, text: text, bom: bom}, nil
}

func (s *McpsManager) parse(id string, text string) (*jsonc.Obj, *jsonc.Obj, error) {
	var root *jsonc.Obj
	if id == "claude" {
		parsed, err := parseJSONC(text)
		if err != nil {
			return nil, nil, fmt.Errorf("JSON 配置格式无效，请先修复配置文件")
		}
		root = parsed
	} else {
		value, err := parseTomlValue(text)
		if err != nil {
			return nil, nil, fmt.Errorf("TOML 配置格式无效，请先修复配置文件")
		}
		obj, ok := value.(*jsonc.Obj)
		if !ok {
			return nil, nil, fmt.Errorf("TOML 配置格式无效，请先修复配置文件")
		}
		root = obj
	}
	serversRaw, _ := root.Get(s.key(id))
	servers, ok := serversRaw.(*jsonc.Obj)
	if !ok {
		if serversRaw == nil {
			servers = jsonc.NewObj()
		} else {
			return nil, nil, fmt.Errorf("%s 须为以 MCP 名称为键的配置对象", s.key(id))
		}
	}
	for _, name := range servers.Keys() {
		value, _ := servers.Get(name)
		if _, ok := value.(*jsonc.Obj); !ok {
			return nil, nil, fmt.Errorf("%s 须为以 MCP 名称为键的配置对象", s.key(id))
		}
	}
	return root, servers, nil
}

func parseTomlValue(text string) (any, error) {
	var out map[string]any
	if err := toml.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	return normalizeValue(out), nil
}

func mcpTransport(id string, entry *jsonc.Obj) string {
	if id == "claude" {
		if value, ok := entry.Get("type"); ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
		return "stdio"
	}
	if _, ok := entry.Get("url"); ok {
		return "http"
	}
	return "stdio"
}

// visibleServers filters display-only entries (codex's builtin node_repl).
func visibleServers(id string, servers *jsonc.Obj) [][2]any {
	out := [][2]any{}
	for _, name := range servers.Keys() {
		if id == "codex" && name == "node_repl" {
			continue
		}
		value, _ := servers.Get(name)
		out = append(out, [2]any{name, value})
	}
	return out
}

func stringMapOf(value any, label string) (map[string]string, error) {
	obj, ok := value.(*jsonc.Obj)
	if value != nil && !ok {
		return nil, fmt.Errorf("%s须为名称和文本值", label)
	}
	out := map[string]string{}
	if obj == nil {
		return out, nil
	}
	for _, key := range obj.Keys() {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\x00\r\n") {
			return nil, fmt.Errorf("%s须为名称和文本值", label)
		}
		item, _ := obj.Get(key)
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s须为名称和文本值", label)
		}
		out[key] = text
	}
	return out, nil
}

// Source lists one harness's MCP servers.
func (s *McpsManager) Source(id any) (McpSource, error) {
	harness, ok := id.(string)
	if !ok || (harness != "claude" && harness != "codex") {
		return McpSource{}, fmt.Errorf("MCP Harness 无效")
	}
	file, err := s.read(harness)
	if err != nil {
		return McpSource{Harness: harness, Path: s.path(harness), Servers: []McpServer{}, Editable: false, Error: err.Error()}, nil
	}
	_, servers, err := s.parse(harness, file.text)
	if err != nil {
		return McpSource{Harness: harness, Path: file.path, Servers: []McpServer{}, Editable: false, Error: err.Error()}, nil
	}
	list := []McpServer{}
	for _, pair := range visibleServers(harness, servers) {
		name := pair[0].(string)
		entry := pair[1].(*jsonc.Obj)
		description := ""
		if value, ok := entry.Get("url"); ok {
			if text, ok := value.(string); ok {
				description = text
			}
		}
		if description == "" {
			if value, ok := entry.Get("command"); ok {
				if text, ok := value.(string); ok {
					description = text
				}
			}
		}
		list = append(list, McpServer{Name: name, Transport: mcpTransport(harness, entry), Description: description, Revision: entryRevision(entry)})
	}
	return McpSource{Harness: harness, Path: file.path, Servers: list, Editable: true}, nil
}

// Inventory lists both harnesses.
func (s *McpsManager) Inventory() McpsInventory {
	out := McpsInventory{Sources: []McpSource{}}
	for _, id := range []string{"claude", "codex"} {
		source, _ := s.Source(id)
		out.Sources = append(out.Sources, source)
	}
	return out
}

func (s *McpsManager) target(input McpTarget, expect string) (McpTarget, error) {
	if input.Harness != "claude" && input.Harness != "codex" {
		return McpTarget{}, fmt.Errorf("MCP Harness 无效")
	}
	if expect != "" && input.Harness != expect {
		return McpTarget{}, fmt.Errorf("MCP 所属 Harness 不一致")
	}
	if input.Name == "" {
		return McpTarget{}, fmt.Errorf("MCP 标识无效")
	}
	return input, nil
}

func (s *McpsManager) entry(servers *jsonc.Obj, target McpTarget) (*jsonc.Obj, error) {
	value, ok := servers.Get(target.Name)
	if !ok {
		return nil, fmt.Errorf("MCP 已被其他程序修改，请刷新后重试")
	}
	entry, ok := value.(*jsonc.Obj)
	if !ok || entryRevision(entry) != target.Revision {
		return nil, fmt.Errorf("MCP 已被其他程序修改，请刷新后重试")
	}
	return entry, nil
}

// Detail returns the editable fields of one MCP.
func (s *McpsManager) Detail(target McpTarget) (McpFields, error) {
	target, err := s.target(target, "")
	if err != nil {
		return McpFields{}, err
	}
	file, err := s.read(target.Harness)
	if err != nil {
		return McpFields{}, err
	}
	_, servers, err := s.parse(target.Harness, file.text)
	if err != nil {
		return McpFields{}, err
	}
	entry, err := s.entry(servers, target)
	if err != nil {
		return McpFields{}, err
	}
	transport := mcpTransport(target.Harness, entry)
	if transport != "stdio" && transport != "http" && transport != "sse" && transport != "ws" {
		return McpFields{}, fmt.Errorf("此 MCP 传输方式暂不支持编辑")
	}
	args := []string{}
	if raw, ok := entry.Get("args"); ok {
		list, ok := raw.([]any)
		if !ok {
			return McpFields{}, fmt.Errorf("MCP args 须为文本列表")
		}
		for _, item := range list {
			text, ok := item.(string)
			if !ok {
				return McpFields{}, fmt.Errorf("MCP args 须为文本列表")
			}
			args = append(args, text)
		}
	}
	env, err := stringMapOf(mustGet(entry, "env"), "环境变量")
	if err != nil {
		return McpFields{}, err
	}
	headersKey := "headers"
	if target.Harness == "codex" {
		headersKey = "http_headers"
	}
	headers, err := stringMapOf(mustGet(entry, headersKey), "请求头")
	if err != nil {
		return McpFields{}, err
	}
	envHeaders, err := stringMapOf(mustGet(entry, "env_http_headers"), "请求头环境变量")
	if err != nil {
		return McpFields{}, err
	}
	fields := McpFields{
		Name: target.Name, Transport: transport, Command: stringValue(entry, "command"),
		Args: args, Env: env, URL: stringValue(entry, "url"), Headers: headers,
		Cwd: stringValue(entry, "cwd"), BearerTokenEnvVar: stringValue(entry, "bearer_token_env_var"),
		EnvHeaders: envHeaders,
	}
	return fields, nil
}

func mustGet(entry *jsonc.Obj, key string) any {
	value, _ := entry.Get(key)
	return value
}

func stringValue(entry *jsonc.Obj, key string) string {
	if value, ok := entry.Get(key); ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

// Preview renders the visible MCP configuration.
func (s *McpsManager) Preview(id any) (McpDocument, error) {
	harness, ok := id.(string)
	if !ok || (harness != "claude" && harness != "codex") {
		return McpDocument{}, fmt.Errorf("MCP Harness 无效")
	}
	file, err := s.read(harness)
	if err != nil {
		return McpDocument{}, err
	}
	_, servers, err := s.parse(harness, file.text)
	if err != nil {
		return McpDocument{}, err
	}
	visible := jsonc.NewObj()
	for _, pair := range visibleServers(harness, servers) {
		visible.Set(pair[0].(string), pair[1])
	}
	var content string
	if harness == "claude" {
		root := jsonc.NewObj()
		root.Set("mcpServers", visible)
		content = jsonc.Pretty(root)
		return McpDocument{Path: file.path, Format: "json", Content: content}, nil
	}
	data, err := toml.Marshal(plainTOML(visible))
	if err != nil {
		return McpDocument{}, err
	}
	return McpDocument{Path: file.path, Format: "toml", Content: string(data)}, nil
}

func plainTOML(obj *jsonc.Obj) map[string]any {
	out := map[string]any{}
	for _, key := range obj.Keys() {
		value, _ := obj.Get(key)
		out[key] = plainTOMLValue(value)
	}
	return out
}

func plainTOMLValue(value any) any {
	switch v := value.(type) {
	case *jsonc.Obj:
		return plainTOML(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = plainTOMLValue(item)
		}
		return out
	default:
		return value
	}
}

var mcpNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func (s *McpsManager) validateFields(fields McpFields, id string) (McpFields, error) {
	if !mcpNamePattern.MatchString(fields.Name) || fields.Name == "__proto__" || fields.Name == "constructor" || fields.Name == "prototype" {
		return McpFields{}, fmt.Errorf("MCP 名称仅支持字母、数字、下划线和连字符")
	}
	allowed := []string{"stdio", "http"}
	if id == "claude" {
		allowed = []string{"stdio", "http", "sse", "ws"}
	}
	valid := false
	for _, item := range allowed {
		if fields.Transport == item {
			valid = true
		}
	}
	if !valid {
		return McpFields{}, fmt.Errorf("MCP 传输方式无效")
	}
	for _, value := range []string{fields.Command, fields.URL, fields.Cwd, fields.BearerTokenEnvVar} {
		if len(value) > 8192 || strings.ContainsRune(value, 0) {
			return McpFields{}, fmt.Errorf("MCP 字段格式无效")
		}
	}
	for _, arg := range fields.Args {
		if strings.ContainsRune(arg, 0) {
			return McpFields{}, fmt.Errorf("启动参数须为文本列表")
		}
	}
	if fields.Transport == "stdio" && strings.TrimSpace(fields.Command) == "" {
		return McpFields{}, fmt.Errorf("请填写启动命令")
	}
	if fields.Transport != "stdio" {
		// 保留 Claude 支持的环境变量引用，不展开也不发送请求。
		address := regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(fields.URL, "value")
		validSchemes := []string{"http://", "https://"}
		if fields.Transport == "ws" {
			validSchemes = []string{"ws://", "wss://"}
		}
		ok := false
		for _, scheme := range validSchemes {
			if strings.HasPrefix(address, scheme) {
				ok = true
			}
		}
		if !ok || strings.ContainsAny(address, " \t\r\n") {
			return McpFields{}, fmt.Errorf("请填写有效的 MCP 地址")
		}
	}
	var err error
	if fields.Env, err = stringMapInput(fields.Env, "环境变量"); err != nil {
		return McpFields{}, err
	}
	if fields.Headers, err = stringMapInput(fields.Headers, "请求头"); err != nil {
		return McpFields{}, err
	}
	if fields.EnvHeaders, err = stringMapInput(fields.EnvHeaders, "请求头环境变量"); err != nil {
		return McpFields{}, err
	}
	return fields, nil
}

func stringMapInput(input map[string]string, label string) (map[string]string, error) {
	out := map[string]string{}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("请填写%s的名称", label)
		}
		if strings.ContainsAny(key, "\x00\r\n") {
			return nil, fmt.Errorf("%s须为名称和文本值", label)
		}
		out[key] = input[key]
	}
	return out, nil
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j] < list[j-1]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// updateEntry rebuilds one MCP entry from validated fields.
func (s *McpsManager) updateEntry(id string, fields McpFields, original *jsonc.Obj) *jsonc.Obj {
	next := jsonc.NewObj()
	previousTransport := "stdio"
	if original != nil {
		previousTransport = mcpTransport(id, original)
		for _, key := range original.Keys() {
			value, _ := original.Get(key)
			next.Set(key, value)
		}
	}
	if fields.Transport != previousTransport {
		for _, key := range []string{"type", "command", "args", "env", "env_vars", "cwd", "url", "headers", "headersHelper", "http_headers", "env_http_headers", "http_headers_helper", "bearer_token_env_var", "oauth", "auth", "scopes", "oauth_resource"} {
			next.Delete(key)
		}
	}
	optional := func(key string, value any, present bool) {
		if present {
			next.Set(key, value)
		} else {
			next.Delete(key)
		}
	}
	if id == "claude" {
		next.Set("type", fields.Transport)
	}
	if fields.Transport == "stdio" {
		next.Set("command", fields.Command)
		optional("args", stringSliceValue(fields.Args), len(fields.Args) > 0)
		optional("env", stringMapValue(fields.Env), len(fields.Env) > 0)
		if id == "codex" {
			optional("cwd", fields.Cwd, fields.Cwd != "")
		}
	} else {
		next.Set("url", fields.URL)
		headersKey := "headers"
		if id == "codex" {
			headersKey = "http_headers"
		}
		optional(headersKey, stringMapValue(fields.Headers), len(fields.Headers) > 0)
		if id == "codex" {
			optional("bearer_token_env_var", fields.BearerTokenEnvVar, fields.BearerTokenEnvVar != "")
			optional("env_http_headers", stringMapValue(fields.EnvHeaders), len(fields.EnvHeaders) > 0)
		}
	}
	return next
}

func stringSliceValue(values []string) any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func stringMapValue(values map[string]string) *jsonc.Obj {
	obj := jsonc.NewObj()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for _, key := range keys {
		obj.Set(key, values[key])
	}
	return obj
}
