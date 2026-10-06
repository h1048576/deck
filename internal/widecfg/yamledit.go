package widecfg

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"gopkg.in/yaml.v3"

	"wide-pure/internal/jsonc"
)

// yamlDoc is a comment-preserving editable YAML document.
type yamlDoc struct {
	file *ast.File
}

// parseYamlDoc parses with comments so edits keep them intact.
func parseYamlDoc(text string) (*yamlDoc, error) {
	file, err := parser.ParseBytes([]byte(text), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return &yamlDoc{file: file}, nil
}

// String renders the document back to text.
func (d *yamlDoc) String() string { return d.file.String() }

// Root returns the document's root node.
func (d *yamlDoc) Root() ast.Node { return d.file.Docs[0].Body }

func asMapping(node ast.Node) (*ast.MappingNode, bool) {
	mapping, ok := node.(*ast.MappingNode)
	return mapping, ok
}

func mappingValue(m *ast.MappingNode, key string) *ast.MappingValueNode {
	for _, value := range m.Values {
		if value.Key.GetToken() != nil && value.Key.GetToken().Value == key {
			return value
		}
	}
	return nil
}

// yamlMappingDelete removes a key from a mapping node.
func yamlMappingDelete(m *ast.MappingNode, key string) bool {
	for i, value := range m.Values {
		if value.Key.GetToken() != nil && value.Key.GetToken().Value == key {
			m.Values = append(m.Values[:i], m.Values[i+1:]...)
			return true
		}
	}
	return false
}

type positionShifter struct {
	columnDelta int
	levelDelta  int
}

// Visit implements ast.Visitor.
func (s *positionShifter) Visit(node ast.Node) ast.Visitor {
	if tok := node.GetToken(); tok != nil && tok.Position != nil {
		tok.Position.Column += s.columnDelta
		tok.Position.IndentLevel += s.levelDelta
	}
	if comment := node.GetComment(); comment != nil {
		s.shiftComment(comment)
	}
	return s
}

func (s *positionShifter) shiftComment(group *ast.CommentGroupNode) {
	if group == nil {
		return
	}
	for _, comment := range group.Comments {
		if tok := comment.GetToken(); tok != nil && tok.Position != nil {
			tok.Position.Column += s.columnDelta
			tok.Position.IndentLevel += s.levelDelta
		}
	}
}

// buildYamlNode converts a Go value (including ordered jsonc objects) into
// a yaml.v3 node tree that encodes with keys in insertion order.
func buildYamlNode(value any) *yaml.Node {
	switch v := value.(type) {
	case *jsonc.Obj:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range v.Keys() {
			item, _ := v.Get(key)
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			node.Content = append(node.Content, keyNode, buildYamlNode(item))
		}
		return node
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range keys {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			node.Content = append(node.Content, keyNode, buildYamlNode(v[key]))
		}
		return node
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range v {
			node.Content = append(node.Content, buildYamlNode(item))
		}
		return node
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	case bool:
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool"}
		node.Value = fmt.Sprintf("%v", v)
		return node
	case float64:
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float"}
		node.Value = strconv.FormatFloat(v, 'g', -1, 64)
		if node.Value == fmt.Sprintf("%d", int64(v)) {
			node.Value = strconv.FormatInt(int64(v), 10)
		}
		return node
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
	default:
		if v == nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}
		node.Value = fmt.Sprintf("%v", v)
		return node
	}
}

// graftYamlNode converts a Go value into an AST node via marshal+parse so
// every token is well-formed, then shifts the subtree into the target
// column/indent context.
func graftYamlNode(value any, targetColumn, targetLevel int) (ast.Node, error) {
	itemNode := buildYamlNode(normalizeYamlValue(value))
	wrapper := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{itemNode}}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(wrapper); err != nil {
		return nil, err
	}
	_ = encoder.Close()
	file, err := parser.ParseBytes(buffer.Bytes(), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, fmt.Errorf("empty YAML fragment")
	}
	body := file.Docs[0].Body
	seq, ok := body.(*ast.SequenceNode)
	if !ok || len(seq.Values) != 1 {
		return nil, fmt.Errorf("unexpected YAML fragment shape")
	}
	node := seq.Values[0]
	// GetToken on containers returns an inner colon token; measure the true
	// leftmost content instead.
	baseline := &positionBaseline{column: int(^uint(0) >> 1), level: int(^uint(0) >> 1)}
	ast.Walk(baseline, node)
	if baseline.column == int(^uint(0)>>1) {
		return nil, fmt.Errorf("grafted node lacks position")
	}
	shifter := &positionShifter{
		columnDelta: targetColumn - baseline.column,
		levelDelta:  targetLevel - baseline.level,
	}
	ast.Walk(shifter, node)
	return node, nil
}

func normalizeYamlValue(value any) any {
	if obj, ok := value.(*jsonc.Obj); ok {
		out := map[string]any{}
		for _, key := range obj.Keys() {
			item, _ := obj.Get(key)
			out[key] = normalizeYamlValue(item)
		}
		return out
	}
	if list, ok := value.([]any); ok {
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = normalizeYamlValue(item)
		}
		return out
	}
	return value
}

type positionBaseline struct {
	column int
	level  int
}

// Visit implements ast.Visitor.
func (b *positionBaseline) Visit(node ast.Node) ast.Visitor {
	if tok := node.GetToken(); tok != nil && tok.Position != nil {
		if tok.Position.Column < b.column {
			b.column = tok.Position.Column
			b.level = tok.Position.IndentLevel
		}
	}
	return b
}

// yamlMappingSet assigns a key's value in a mapping node, creating the key
// when missing.
func yamlMappingSet(m *ast.MappingNode, key string, value any) error {
	if existing := mappingValue(m, key); existing != nil {
		if keyToken := existing.Key.GetToken(); keyToken != nil && keyToken.Position != nil {
			grafted, err := graftYamlNode(value, keyToken.Position.Column+2, keyToken.Position.IndentLevel+1)
			if err != nil {
				return err
			}
			existing.Value = grafted
			return nil
		}
		return fmt.Errorf("grafted key lacks position")
	}
	// Appending: align with the last sibling key, or nest under an empty mapping.
	column, level := 2, 1
	if len(m.Values) > 0 {
		last := m.Values[len(m.Values)-1]
		if lastToken := last.Key.GetToken(); lastToken != nil && lastToken.Position != nil {
			column = lastToken.Position.Column
			level = lastToken.Position.IndentLevel
		}
	} else if token := m.GetToken(); token != nil && token.Position != nil {
		column = token.Position.Column + 2
		level = token.Position.IndentLevel + 1
	}
	keyNode, err := graftYamlNode(key, column, level)
	if err != nil {
		return err
	}
	keyCast, ok := keyNode.(ast.MapKeyNode)
	if !ok {
		return fmt.Errorf("grafted key is %T", keyNode)
	}
	grafted, err := graftYamlNode(value, column+2, level+1)
	if err != nil {
		return err
	}
	m.Values = append(m.Values, ast.MappingValue(nil, keyCast, grafted))
	return nil
}

// yamlSequence reinterprets a node as a sequence.
func yamlSequence(node ast.Node) (*ast.SequenceNode, error) {
	seq, ok := node.(*ast.SequenceNode)
	if !ok {
		return nil, fmt.Errorf("模型列表已变化，请刷新后重新排序")
	}
	return seq, nil
}

// yamlToValue decodes an AST subtree through a plain re-parse of its text.
func yamlToValue(node ast.Node) (any, error) {
	text := node.String()
	var out any
	if err := yaml.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	return out, nil
}

var errYamlRoot = errors.New("DSH YAML 配置格式无效，根节点须为列表")

// dshDoc locates the llm-pi-ai plugin inside a cordis.patch.yml document.
type dshDoc struct {
	doc         *yamlDoc
	providers   *ast.MappingNode
	pluginIndex int
}

// PluginIndex returns the plugin position in the root list.
func (d *dshDoc) PluginIndex() int { return d.pluginIndex }

// String renders the document text.
func (d *dshDoc) String() string { return d.doc.String() }

func parseDsh(text string) (*dshDoc, error) {
	var root any
	if err := yaml.Unmarshal([]byte(text), &root); err != nil {
		return nil, errYamlRoot
	}
	list, ok := root.([]any)
	if !ok {
		return nil, errYamlRoot
	}
	index := -1
	for i, item := range list {
		if obj, ok := item.(map[string]any); ok && obj["id"] == "llm-pi-ai" {
			if index >= 0 {
				return nil, fmt.Errorf("DSH 配置须包含一个 id 为 llm-pi-ai 的插件")
			}
			index = i
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("DSH 配置须包含一个 id 为 llm-pi-ai 的插件")
	}
	doc, err := parseYamlDoc(text)
	if err != nil {
		return nil, errYamlRoot
	}
	seq, ok := doc.Root().(*ast.SequenceNode)
	if !ok || index >= len(seq.Values) {
		return nil, errYamlRoot
	}
	plugin, ok := asMapping(seq.Values[index])
	if !ok {
		return nil, fmt.Errorf("DSH 配置须包含一个 id 为 llm-pi-ai 的插件")
	}
	configValue := mappingValue(plugin, "config")
	if configValue == nil {
		return nil, fmt.Errorf("llm-pi-ai 尚未配置 providers")
	}
	config, ok := asMapping(configValue.Value)
	if !ok {
		return nil, fmt.Errorf("llm-pi-ai 尚未配置 providers")
	}
	providersValue := mappingValue(config, "providers")
	if providersValue == nil {
		return nil, fmt.Errorf("llm-pi-ai 尚未配置 providers")
	}
	providers, ok := asMapping(providersValue.Value)
	if !ok {
		return nil, fmt.Errorf("llm-pi-ai 尚未配置 providers")
	}
	return &dshDoc{doc: doc, providers: providers, pluginIndex: index}, nil
}

func (d *dshDoc) providerNames() []string {
	names := []string{}
	for _, value := range d.providers.Values {
		if value.Key.GetToken() == nil {
			continue
		}
		if _, ok := asMapping(value.Value); ok {
			names = append(names, value.Key.GetToken().Value)
		}
	}
	return names
}

func (d *dshDoc) provider(name string) (*ast.MappingNode, error) {
	value := mappingValue(d.providers, name)
	if value == nil {
		return nil, fmt.Errorf("尚未配置模型提供商")
	}
	mapping, ok := asMapping(value.Value)
	if !ok {
		return nil, fmt.Errorf("尚未配置模型提供商")
	}
	return mapping, nil
}

var _ = token.InvalidType
