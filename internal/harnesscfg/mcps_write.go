package harnesscfg

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"deck/internal/jsonc"
)

// Save creates, updates or copies one MCP entry.
func (s *McpsManager) Save(change McpChange) error {
	if s.mutating {
		return fmt.Errorf("MCP 操作正在执行，请稍后再试")
	}
	s.mutating = true
	defer func() { s.mutating = false }()
	if change.Target != nil && change.CopyFrom != nil {
		return fmt.Errorf("MCP 修改方式无效")
	}
	id := change.Harness
	if id != "claude" && id != "codex" {
		return fmt.Errorf("MCP Harness 无效")
	}
	fields, err := s.validateFields(change.Fields, id)
	if err != nil {
		return err
	}
	file, err := s.read(id)
	if err != nil {
		return err
	}
	root, servers, err := s.parse(id, file.text)
	if err != nil {
		return err
	}
	var original *jsonc.Obj
	if change.Target != nil {
		target, err := s.target(*change.Target, id)
		if err != nil {
			return err
		}
		original, err = s.entry(servers, target)
		if err != nil {
			return err
		}
	}
	if change.CopyFrom != nil {
		target, err := s.target(*change.CopyFrom, id)
		if err != nil {
			return err
		}
		original, err = s.entry(servers, target)
		if err != nil {
			return err
		}
	}
	if _, exists := servers.Get(fields.Name); exists && (change.Target == nil || fields.Name != change.Target.Name) {
		return fmt.Errorf("已有同名 MCP，请更换名称")
	}
	updated := s.updateEntry(id, fields, original)
	// 重建 servers 映射：替换的键保持原位置，新键追加。
	next := jsonc.NewObj()
	for _, name := range servers.Keys() {
		value, _ := servers.Get(name)
		if change.Target != nil && name == change.Target.Name {
			next.Set(fields.Name, updated)
		} else {
			next.Set(name, value)
		}
	}
	if change.Target == nil {
		next.Set(fields.Name, updated)
	}
	oldName := ""
	if change.Target != nil {
		oldName = change.Target.Name
	}
	text, err := s.render(id, file, root, next, oldName, fields.Name)
	if err != nil {
		return err
	}
	return s.commit(id, file, text)
}

// Delete removes one MCP entry.
func (s *McpsManager) Delete(target McpTarget) error {
	if s.mutating {
		return fmt.Errorf("MCP 操作正在执行，请稍后再试")
	}
	s.mutating = true
	defer func() { s.mutating = false }()
	target, err := s.target(target, "")
	if err != nil {
		return err
	}
	file, err := s.read(target.Harness)
	if err != nil {
		return err
	}
	root, servers, err := s.parse(target.Harness, file.text)
	if err != nil {
		return err
	}
	if _, err := s.entry(servers, target); err != nil {
		return err
	}
	next := jsonc.NewObj()
	for _, name := range servers.Keys() {
		if name == target.Name {
			continue
		}
		value, _ := servers.Get(name)
		next.Set(name, value)
	}
	text, err := s.render(target.Harness, file, root, next, target.Name, "")
	if err != nil {
		return err
	}
	return s.commit(target.Harness, file, text)
}

// render produces the new config text, verifying the result parses back to
// the intended servers while leaving the rest of the document untouched.
func (s *McpsManager) render(id string, file mcpFile, root, servers *jsonc.Obj, oldName, newName string) (string, error) {
	eol := "\n"
	if strings.Contains(file.text, "\r\n") {
		eol = "\r\n"
	}
	var text string
	var err error
	if id == "claude" {
		doc, err := jsonc.ParseDoc(file.text)
		if err != nil {
			return "", err
		}
		text, err = doc.Set([]any{"mcpServers"}, servers, jsonc.DetectFormatOf(file.text))
		if err != nil {
			return "", err
		}
	} else {
		text = s.renderToml(file.text, servers, oldName, newName)
		text = strings.ReplaceAll(text, "\r\n", "\n")
		text = strings.ReplaceAll(text, "\n", eol)
	}
	// 校验写入结果：servers 与预期一致，其余内容不变。
	checkRoot, checkServers, err := s.parse(id, text)
	if err != nil {
		return "", fmt.Errorf("MCP 配置写入校验失败，本次未写入")
	}
	if !jsonc.Equal(checkServers, servers) {
		return "", fmt.Errorf("MCP 配置写入校验失败，本次未写入")
	}
	expectedRoot := root.Clone()
	expectedRoot.Delete(s.key(id))
	checkWithout := checkRoot.Clone()
	checkWithout.Delete(s.key(id))
	if !jsonc.Equal(expectedRoot, checkWithout) {
		return "", fmt.Errorf("MCP 配置写入校验失败，本次未写入")
	}
	if file.bom {
		text = "\uFEFF" + text
	}
	return text, nil
}

type tomlBlockRange struct {
	name    string
	start   int // offset of the header line start
	end     int // offset past the block (start of the next header or EOF)
	isArray bool
	main    bool // [mcp_servers.NAME] as opposed to a sub-table
}

var (
	tomlHeaderPattern = regexp.MustCompile(`^\s*\[\s*([^\]]*?)\s*\]\s*(#.*)?$`)
	tomlArrayHeader   = regexp.MustCompile(`^\s*\[\[\s*([^\]]*?)\s*\]\]\s*(#.*)?$`)
	tomlGlobalAssign  = regexp.MustCompile(`^\s*(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9_-]+))\s*[.=]`)
)

// splitTomlKey splits a dotted header key into segments, honoring quotes.
func splitTomlKey(key string) []string {
	segments := []string{}
	current := strings.Builder{}
	inQuote := byte(0)
	for i := 0; i < len(key); i++ {
		ch := key[i]
		switch {
		case inQuote != 0:
			if ch == inQuote {
				inQuote = 0
			} else {
				current.WriteByte(ch)
			}
		case ch == '"' || ch == '\'':
			inQuote = ch
		case ch == '.':
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	segments = append(segments, strings.TrimSpace(current.String()))
	return segments
}

type tomlScan struct {
	blocks map[string][]tomlBlockRange
	simple bool
}

// scanToml finds [mcp_servers.NAME] blocks and checks that all
// mcp_servers data lives in standard tables.
func scanToml(text string) tomlScan {
	scan := tomlScan{blocks: map[string][]tomlBlockRange{}, simple: true}
	var open *tomlBlockRange
	closeBlock := func(nextStart int) {
		if open != nil {
			open.end = nextStart
			if open.name != "" {
				scan.blocks[open.name] = append(scan.blocks[open.name], *open)
			}
			open = nil
		}
	}
	offset := 0
	for offset < len(text) {
		lineEnd := offset
		for lineEnd < len(text) && text[lineEnd] != '\n' {
			lineEnd++
		}
		lineEnd++
		if lineEnd > len(text) {
			lineEnd = len(text)
		}
		line := strings.TrimRight(text[offset:lineEnd], "\r\n")
		trimmed := strings.TrimSpace(line)
		header := false
		if strings.HasPrefix(trimmed, "[[") {
			if match := tomlArrayHeader.FindStringSubmatch(line); match != nil {
				closeBlock(offset)
				segments := splitTomlKey(match[1])
				name := ""
				if len(segments) >= 2 && segments[0] == "mcp_servers" {
					name = segments[1]
				}
				if len(segments) >= 1 && segments[0] == "mcp_servers" {
					scan.simple = false
				}
				open = &tomlBlockRange{name: name, start: offset, end: len(text), isArray: true}
				header = true
			}
		} else if strings.HasPrefix(trimmed, "[") {
			if match := tomlHeaderPattern.FindStringSubmatch(line); match != nil {
				closeBlock(offset)
				segments := splitTomlKey(match[1])
				block := tomlBlockRange{start: offset, end: len(text)}
				if len(segments) >= 1 && segments[0] == "mcp_servers" {
					if len(segments) < 2 {
						// 裸 [mcp_servers] 表无法按名字替换。
						scan.simple = false
					} else {
						block.name = segments[1]
						block.main = len(segments) == 2
					}
				}
				open = &block
				header = true
			}
		}
		if !header && open == nil && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			// 全局区域的键值：mcp_servers 出现在此处则不能做块级拼接。
			if match := tomlGlobalAssign.FindStringSubmatch(line); match != nil {
				first := match[1]
				if first == "" {
					first = match[2]
				}
				if first == "" {
					first = match[3]
				}
				if first == "mcp_servers" {
					scan.simple = false
				}
			}
		}
		offset = lineEnd
	}
	closeBlock(len(text))
	return scan
}

// renderToml splices the [mcp_servers.NAME] table blocks of one entry in
// place, keeping every other table's text and comments untouched.
func (s *McpsManager) renderToml(text string, servers *jsonc.Obj, oldName, newName string) string {
	scan := scanToml(text)
	if !scan.simple {
		return s.rewriteAllTomlServers(text, servers)
	}
	type splice struct {
		start, end  int
		replacement string
	}
	var splices []splice
	if oldName != "" {
		// 只替换目标表；其他表的文本、注释和顺序保持原样。
		for i, r := range scan.blocks[oldName] {
			replacement := ""
			if i == 0 && newName != "" {
				value, _ := servers.Get(newName)
				// 块范围包含结尾换行，替换文本也以换行结束。
				replacement = strings.TrimRight(s.marshalServerBlock(newName, value.(*jsonc.Obj)), "\n") + "\n"
			}
			splices = append(splices, splice{start: r.start, end: r.end, replacement: replacement})
		}
	}
	var appended []string
	if oldName == "" && newName != "" {
		value, _ := servers.Get(newName)
		appended = append(appended, strings.TrimRight(s.marshalServerBlock(newName, value.(*jsonc.Obj)), "\n")+"\n")
	}
	sort.Slice(splices, func(i, j int) bool { return splices[i].start > splices[j].start })
	for _, item := range splices {
		text = text[:item.start] + item.replacement + text[item.end:]
	}
	if len(appended) > 0 {
		trimmed := strings.TrimRight(text, "\n \t")
		if trimmed == "" {
			text = strings.Join(appended, "\n")
		} else {
			text = trimmed + "\n\n" + strings.Join(appended, "\n")
		}
	}
	return text
}

// rewriteAllTomlServers rewrites the entire mcp_servers section.
func (s *McpsManager) rewriteAllTomlServers(text string, servers *jsonc.Obj) string {
	stripped := s.stripTomlServers(text)
	data, err := toml.Marshal(map[string]any{"mcp_servers": plainTOML(servers)})
	if err != nil {
		data = []byte("")
	}
	block := strings.TrimRight(string(data), "\n")
	trimmed := strings.TrimRight(stripped, "\n \t")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

func (s *McpsManager) stripTomlServers(text string) string {
	scan := scanToml(text)
	ranges := [][2]int{}
	for _, list := range scan.blocks {
		for _, r := range list {
			ranges = append(ranges, [2]int{r.start, r.end})
		}
	}
	// 全局区域的 mcp_servers 键值行也要移除。
	offset := 0
	inHeader := false
	for offset < len(text) {
		lineEnd := offset
		for lineEnd < len(text) && text[lineEnd] != '\n' {
			lineEnd++
		}
		lineEnd++
		if lineEnd > len(text) {
			lineEnd = len(text)
		}
		line := strings.TrimRight(text[offset:lineEnd], "\r\n")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inHeader = true
		}
		if !inHeader && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if match := tomlGlobalAssign.FindStringSubmatch(line); match != nil {
				first := match[1]
				if first == "" {
					first = match[2]
				}
				if first == "" {
					first = match[3]
				}
				if first == "mcp_servers" {
					ranges = append(ranges, [2]int{offset, lineEnd})
				}
			}
		}
		offset = lineEnd
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] > ranges[j][0] })
	for _, r := range ranges {
		text = text[:r[0]] + text[r[1]:]
	}
	return text
}

// marshalServerBlock renders one [mcp_servers.NAME] block including sub-tables.
func (s *McpsManager) marshalServerBlock(name string, entry *jsonc.Obj) string {
	quoteKey := func(key string) string {
		if mcpNamePattern.MatchString(key) {
			return key
		}
		return strconv.Quote(key)
	}
	var sb bytes.Buffer
	subTables := map[string]*jsonc.Obj{}
	var simpleKeys []string
	for _, key := range entry.Keys() {
		value, _ := entry.Get(key)
		if sub, ok := value.(*jsonc.Obj); ok {
			subTables[key] = sub
			continue
		}
		simpleKeys = append(simpleKeys, key)
	}
	sb.WriteString("[mcp_servers." + quoteKey(name) + "]\n")
	for _, key := range simpleKeys {
		value, _ := entry.Get(key)
		sb.WriteString(quoteKey(key) + " = " + renderTOMLScalarOrArray(value) + "\n")
	}
	for _, key := range entry.Keys() {
		sub, ok := subTables[key]
		if !ok {
			continue
		}
		sb.WriteString("\n[mcp_servers." + quoteKey(name) + "." + quoteKey(key) + "]\n")
		for _, subKey := range sub.Keys() {
			value, _ := sub.Get(subKey)
			if _, isTable := value.(*jsonc.Obj); isTable {
				// 更深的嵌套交给整体序列化（罕见）。
				if data, err := toml.Marshal(plainTOML(sub)); err == nil {
					for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
						sb.WriteString(line + "\n")
					}
					continue
				}
			}
			sb.WriteString(quoteKey(subKey) + " = " + renderTOMLScalarOrArray(value) + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderTOMLScalarOrArray(value any) string {
	switch v := value.(type) {
	case string:
		return renderTOMLString(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, renderTOMLScalarOrArray(item))
		}
		return "[ " + strings.Join(parts, ", ") + " ]"
	default:
		if v == nil {
			return `""`
		}
		return renderTOMLString(fmt.Sprintf("%v", v))
	}
}

func renderTOMLString(value string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// commit performs the atomic write with a pre-write backup.
func (s *McpsManager) commit(id string, file mcpFile, text string) error {
	if text == derefOriginal(file.original) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(file.path), ".wide-mcps-"+randomHex()+".tmp")
	if err := os.WriteFile(temporary, []byte(text), 0o600); err != nil {
		return err
	}
	defer os.Remove(temporary)
	if file.original != nil {
		if err := os.MkdirAll(s.backups, 0o755); err != nil {
			return err
		}
		extension := ".json"
		if id == "codex" {
			extension = ".toml"
		}
		backup := filepath.Join(s.backups, fmt.Sprintf("%s-%d-%s%s", id, timestampMillis(), randomHex(), extension))
		if err := os.WriteFile(backup, []byte(*file.original), 0o600); err != nil {
			return err
		}
	}
	current, err := s.read(id)
	if err != nil {
		return fmt.Errorf("配置文件已被其他程序修改，本次未写入，请刷新后重试")
	}
	if !sameOriginal(current.original, file.original) {
		return fmt.Errorf("配置文件已被其他程序修改，本次未写入，请刷新后重试")
	}
	return os.Rename(temporary, file.path)
}
