package harnesscfg

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml/ast"

	"deck/internal/jsonc"
)

func timestampMillis() int64 { return time.Now().UnixMilli() }

// yamlOrderedMap converts an entry into a *yaml.Node tree that keeps key
// order when marshaled.
func yamlOrderedMap(item *jsonc.Obj) any {
	return item
}

// dshItemMapping resolves the mapping node of one model entry.
func dshItemMapping(doc *dshDoc, provider string, index int) (*ast.MappingNode, error) {
	providerNode, err := doc.provider(provider)
	if err != nil {
		return nil, err
	}
	modelsValue := mappingValue(providerNode, "models")
	if modelsValue == nil {
		return nil, fmt.Errorf("模型列表已变化，请刷新后重新排序")
	}
	sequence, ok := modelsValue.Value.(*ast.SequenceNode)
	if !ok || index >= len(sequence.Values) {
		return nil, fmt.Errorf("模型列表已变化，请刷新后重新排序")
	}
	item, ok := asMapping(sequence.Values[index])
	if !ok {
		return nil, fmt.Errorf("models 格式无效，须为模型对象列表")
	}
	return item, nil
}

// render produces the new file text for one change.
func (m *ModelsManager) render(source sourceConfig, file *parsedFile, entries []*jsonc.Obj, index *int, deleting bool, order []int, baseURL *string, replace bool) (string, error) {
	eol := "\n"
	if strings.Contains(file.file.text, "\r\n") {
		eol = "\r\n"
	}
	var text string
	if file.dsh != nil {
		logDebugf("dsh render: provider=%q entries=%d index=%v deleting=%v order=%v replace=%v", source.provider, len(entries), index, deleting, order, replace)
		doc, err := parseDsh(file.file.text)
		if err != nil {
			return "", err
		}
		providerNode, err := doc.provider(source.provider)
		if err != nil {
			return "", err
		}
		setModels := func() error {
			values := make([]any, 0, len(entries))
			for _, item := range entries {
				values = append(values, stripReserved(item))
			}
			return yamlMappingSet(providerNode, "models", values)
		}
		switch {
		case replace:
			// profiles 分叉时整体重写 models 节点。
			if err := setModels(); err != nil {
				return "", err
			}
		case order != nil:
			modelsValue := mappingValue(providerNode, "models")
			if modelsValue == nil {
				return "", fmt.Errorf("模型列表已变化，请刷新后重新排序")
			}
			sequence, err := yamlSequence(modelsValue.Value)
			if err != nil {
				return "", err
			}
			if len(order) != len(sequence.Values) {
				return "", fmt.Errorf("模型列表已变化，请刷新后重新排序")
			}
			permuted := make([]ast.Node, 0, len(order))
			for _, position := range order {
				if position < 0 || position >= len(sequence.Values) {
					return "", fmt.Errorf("模型列表已变化，请刷新后重新排序")
				}
				permuted = append(permuted, sequence.Values[position])
			}
			sequence.Values = permuted
		case index == nil:
			modelsValue := mappingValue(providerNode, "models")
			last := entries[len(entries)-1]
			if modelsValue != nil {
				if sequence, ok := modelsValue.Value.(*ast.SequenceNode); ok && len(sequence.Values) > 0 {
					targetColumn, targetLevel := subtreeIndent(sequence.Values[len(sequence.Values)-1])
					grafted, err := graftYamlNode(stripReserved(last), targetColumn, targetLevel)
					if err != nil {
						return "", err
					}
					sequence.Values = append(sequence.Values, grafted)
				} else {
					if err := setModels(); err != nil {
						return "", err
					}
				}
			} else {
				if err := setModels(); err != nil {
					return "", err
				}
			}
		case deleting:
			modelsValue := mappingValue(providerNode, "models")
			if modelsValue == nil {
				return "", fmt.Errorf("模型列表已变化，请刷新后重新排序")
			}
			sequence, ok := modelsValue.Value.(*ast.SequenceNode)
			if !ok || *index >= len(sequence.Values) {
				return "", fmt.Errorf("模型列表已变化，请刷新后重新排序")
			}
			sequence.Values = append(sequence.Values[:*index], sequence.Values[*index+1:]...)
		default:
			item, err := dshItemMapping(doc, source.provider, *index)
			if err != nil {
				return "", err
			}
			old := file.entries[*index]
			next := entries[*index]
			for _, key := range entryKeys(old) {
				if !next.Has(key) {
					yamlMappingDelete(item, key)
				}
			}
			for _, key := range entryKeys(next) {
				nextValue, _ := next.Get(key)
				if oldValue, ok := old.Get(key); ok && jsonc.Canonical(normalizeValue(oldValue)) == jsonc.Canonical(normalizeValue(nextValue)) {
					continue
				}
				if err := yamlMappingSet(item, key, normalizeValue(nextValue)); err != nil {
					return "", err
				}
			}
		}
		if baseURL != nil {
			if err := yamlMappingSet(providerNode, "baseURL", *baseURL); err != nil {
				return "", err
			}
		}
		text = doc.String()
		// 与原实现一致，将 DSH 渲染结果统一为源文件的换行符。
		text = strings.ReplaceAll(text, "\r\n", "\n")
		if eol == "\r\n" {
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		if _, err := parseDsh(text); err != nil {
			return "", err
		}
	} else {
		f := jsonc.DetectFormatOf(file.file.text)
		doc, err := jsonc.ParseDoc(file.file.text)
		if err != nil {
			return "", err
		}
		location := file.location
		if source.harness == "opencode" {
			renamed := index != nil && !deleting && *index < len(file.entries) &&
				mapKeyOf(file.entries[*index]) != mapKeyOf(entries[*index])
			modelsRaw, modelsFound := doc.Get("provider", "proxy", "models")
			_, modelsIsObj := modelsRaw.(*jsonc.Obj)
			replaceMap := order != nil || renamed || !modelsFound || !modelsIsObj
			var item *jsonc.Obj
			if index == nil {
				item = entries[len(entries)-1]
			} else if deleting {
				item = file.entries[*index]
			} else {
				item = entries[*index]
			}
			if replaceMap {
				replacement := jsonc.NewObj()
				for _, entry := range entries {
					replacement.Set(mapKeyOf(entry), stripReserved(entry))
				}
				text, err = doc.Set(location, replacement, f)
			} else if deleting {
				text, err = doc.Delete(append(append([]any{}, location...), mapKeyOf(item)), f)
			} else {
				text, err = doc.Set(append(append([]any{}, location...), mapKeyOf(item)), stripReserved(item), f)
			}
			if err != nil {
				return "", err
			}
		} else {
			hasArray := false
			switch source.harness {
			case "claude":
				if pickerRaw, ok := doc.Get("modelPicker"); ok {
					if picker, ok := pickerRaw.(*jsonc.Obj); ok {
						if optionsRaw, ok := picker.Get("options"); ok {
							_, hasArray = optionsRaw.([]any)
						}
					}
				}
			case "pi":
				if providersRaw, ok := doc.Get("providers"); ok {
					if providers, ok := providersRaw.(*jsonc.Obj); ok {
						if proxyRaw, ok := providers.Get("proxy"); ok {
							if proxy, ok := proxyRaw.(*jsonc.Obj); ok {
								if modelsRaw, ok := proxy.Get("models"); ok {
									_, hasArray = modelsRaw.([]any)
								}
							}
						}
					}
				}
			default:
				if modelsRaw, ok := doc.Get("customModels"); ok {
					_, hasArray = modelsRaw.([]any)
				}
			}
			rewriteWhole := order != nil || !hasArray
			switch {
			case rewriteWhole:
				values := make([]any, 0, len(entries))
				for _, item := range entries {
					values = append(values, stripReserved(item))
				}
				text, err = doc.Set(location, values, f)
			case deleting:
				text, err = doc.Delete(append(append([]any{}, location...), *index), f)
			case index == nil:
				text, err = doc.InsertArray(append(append([]any{}, location...), len(entries)), lastEntry(entries), f)
			default:
				text, err = doc.Set(append(append([]any{}, location...), *index), stripReserved(entries[*index]), f)
			}
			if err != nil {
				return "", err
			}
		}
		if baseURL != nil {
			updated, err := jsonc.ParseDoc(text)
			if err != nil {
				return "", err
			}
			baseLocation := []any{"providers", "proxy", "baseUrl"}
			if source.harness == "opencode" {
				baseLocation = []any{"provider", "proxy", "options", "baseURL"}
			}
			text, err = updated.Set(baseLocation, *baseURL, f)
			if err != nil {
				return "", err
			}
		}
		// 每次 JSON 写入都规范模型字段，包括后补字段和此前保存的旧顺序。
		rendered, err := m.parseFile(source, fileState{text: text, bom: file.file.bom})
		if err != nil {
			return "", err
		}
		for position, item := range rendered.entries {
			orderedItem := m.ordered(source.harness, item)
			if sameKeyOrder(item, orderedItem) {
				continue
			}
			updated, err := jsonc.ParseDoc(text)
			if err != nil {
				return "", err
			}
			entryLocation := append(append([]any{}, rendered.location...), position)
			if source.harness == "opencode" {
				entryLocation = append(append([]any{}, rendered.location...), mapKeyOf(item))
			}
			text, err = updated.Set(entryLocation, stripReserved(orderedItem), f)
			if err != nil {
				return "", err
			}
		}
		if _, err := parseJSONC(text); err != nil {
			return "", err
		}
	}
	if eol == "\r\n" && file.dsh == nil {
		// JSON 路径由编辑器保持原文本；DSH 路径在此统一换行符。
	}
	if file.dsh != nil && eol == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if file.file.bom {
		text = "\uFEFF" + text
	}
	return text, nil
}

// subtreeIndent reports the leftmost content position of a node subtree.
func subtreeIndent(node ast.Node) (int, int) {
	baseline := &positionBaseline{column: int(^uint(0) >> 1), level: int(^uint(0) >> 1)}
	ast.Walk(baseline, node)
	if baseline.column == int(^uint(0)>>1) {
		return 2, 1
	}
	return baseline.column, baseline.level
}

func lastEntry(entries []*jsonc.Obj) *jsonc.Obj {
	if len(entries) == 0 {
		return jsonc.NewObj()
	}
	return stripReserved(entries[len(entries)-1])
}

func mapKeyOf(item *jsonc.Obj) string {
	if value, ok := item.Get(ReservedMapKey); ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

func sameKeyOrder(a, b *jsonc.Obj) bool {
	aKeys := entryKeys(a)
	bKeys := entryKeys(b)
	if len(aKeys) != len(bKeys) {
		return false
	}
	for i := range aKeys {
		if aKeys[i] != bKeys[i] {
			return false
		}
	}
	return true
}

func logDebugf(format string, args ...any) {
	if os.Getenv("WIDE_DEBUG") != "" {
		println(fmt.Sprintf("DEBUG "+format, args...))
	}
}
