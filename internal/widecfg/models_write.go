package widecfg

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"wide-pure/internal/jsonc"
)

// Save creates, updates or copies one model.
func (m *ModelsManager) Save(change ModelChange) error {
	return m.exclusive(func() error {
		source, err := m.source(change.SourceID)
		if err != nil {
			return err
		}
		file, err := m.load(source)
		if err != nil {
			return err
		}
		if change.Target != nil && change.CopyFrom != nil {
			return fmt.Errorf("模型所属配置无效")
		}
		if change.Target != nil && change.Target.SourceID != source.id {
			return fmt.Errorf("模型所属配置无效")
		}
		if change.CopyFrom != nil && change.CopyFrom.SourceID != source.id {
			return fmt.Errorf("模型所属配置无效")
		}
		var old *jsonc.Obj
		switch {
		case change.Target != nil:
			old, err = m.entry(file.entries, *change.Target)
		case change.CopyFrom != nil:
			old, err = m.entry(file.entries, *change.CopyFrom)
		default:
			if source.harness == "pi" || source.harness == "opencode" {
				// pi/opencode 以最后一条模型为模板。
				if len(file.entries) > 0 {
					old = file.entries[len(file.entries)-1].Clone()
				} else {
					old = jsonc.NewObj()
				}
			} else {
				old = jsonc.NewObj()
			}
		}
		if err != nil {
			return err
		}
		if change.CopyFrom != nil && source.harness == "droid" {
			old.Delete("id")
		}
		model, err := m.model(source.harness, change.Fields, old, change.APIKey)
		if err != nil {
			return err
		}
		next := m.ordered(source.harness, model)
		targetIndex := -1
		if change.Target != nil {
			targetIndex = change.Target.Index
		}
		nextID := modelID(source.harness, next)
		for index, item := range file.entries {
			if index == targetIndex {
				continue
			}
			if modelID(source.harness, item) == nextID {
				return fmt.Errorf("此配置中已存在相同的模型 ID")
			}
		}
		entries := append([]*jsonc.Obj{}, file.entries...)
		if change.Target != nil {
			entries[change.Target.Index] = next
		} else {
			entries = append(entries, next)
		}
		var baseURL *string
		if source.harness == "dsh" || source.harness == "pi" || source.harness == "opencode" {
			address, err := apiAddress(change.Fields.BaseURL)
			if err != nil {
				return err
			}
			baseURL = &address
		}
		return m.commit(source, file, entries, false, nil, baseURL, targetIndex)
	})
}

// Delete removes one model.
func (m *ModelsManager) Delete(target ModelTarget) error {
	return m.exclusive(func() error {
		source, err := m.source(target.SourceID)
		if err != nil {
			return err
		}
		file, err := m.load(source)
		if err != nil {
			return err
		}
		if _, err := m.entry(file.entries, target); err != nil {
			return err
		}
		entries := make([]*jsonc.Obj, 0, len(file.entries))
		for index, item := range file.entries {
			if index == target.Index {
				continue
			}
			entries = append(entries, item)
		}
		return m.commit(source, file, entries, true, nil, nil, target.Index)
	})
}

// Reorder applies a drag order.
func (m *ModelsManager) Reorder(change ModelOrder) error {
	return m.exclusive(func() error {
		source, err := m.source(change.SourceID)
		if err != nil {
			return err
		}
		file, err := m.load(source)
		if err != nil {
			return err
		}
		if len(change.Models) != len(file.entries) {
			return fmt.Errorf("模型列表已变化，请刷新后重新排序")
		}
		seen := map[int]bool{}
		for _, item := range change.Models {
			if item.SourceID != source.id || seen[item.Index] || item.Index >= len(file.entries) {
				return fmt.Errorf("模型列表已变化，请刷新后重新排序")
			}
			seen[item.Index] = true
		}
		entries := make([]*jsonc.Obj, 0, len(change.Models))
		order := make([]int, 0, len(change.Models))
		for _, item := range change.Models {
			entry, err := m.entry(file.entries, item)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
			order = append(order, item.Index)
		}
		identity := true
		for index, item := range order {
			if item != index {
				identity = false
				break
			}
		}
		if source.harness != "dsh" && identity {
			return nil
		}
		return m.commit(source, file, entries, false, order, nil, -1)
	})
}

func (m *ModelsManager) retarget(source sourceConfig, action string, requested string, item *jsonc.Obj) (*jsonc.Obj, error) {
	oldID := modelID(source.harness, item)
	nextID := requested
	if source.harness == "claude" && strings.Contains(oldID, "[1m]") {
		if match := oneMSuffix.FindString(oldID); match != "" {
			nextID = requested + match
		} else {
			nextID = requested + "[1m]"
		}
	}
	name := modelName(nextID)
	previousName := displayName(source.harness, item)
	if value, ok := item.Get("label"); ok {
		if text, ok := value.(string); ok {
			previousName = text
		}
	} else if value, ok := item.Get("displayName"); ok {
		if text, ok := value.(string); ok {
			previousName = text
		}
	} else if value, ok := item.Get("name"); ok {
		if text, ok := value.(string); ok {
			previousName = text
		}
	}
	description := any(nil)
	if value, ok := item.Get("description"); ok {
		if text, ok := value.(string); ok {
			description = text
		}
	}
	// 描述文本中的旧模型 ID/名称同步替换。
	replacements := [][2]string{{oldID, nextID}, {previousName, name}}
	patternParts := []string{}
	for _, pair := range replacements {
		if pair[0] != "" {
			patternParts = append(patternParts, regexp.QuoteMeta(pair[0]))
		}
	}
	sort.Slice(patternParts, func(i, j int) bool { return len(patternParts[i]) > len(patternParts[j]) })
	if text, ok := description.(string); ok && len(patternParts) > 0 {
		pattern := regexp.MustCompile("(" + strings.Join(patternParts, "|") + ")")
		description = pattern.ReplaceAllStringFunc(text, func(match string) string {
			for _, pair := range replacements {
				if pair[0] == match {
					return pair[1]
				}
			}
			return match
		})
	}
	old := item.Clone()
	if source.harness == "droid" {
		oldModel, _ := item.Get("model")
		old.Set("id", "custom:"+fmt.Sprintf("%v", oldModel))
	}
	if source.harness == "opencode" {
		if oldIDValue, ok := item.Get("id"); ok {
			if _, isString := oldIDValue.(string); isString {
				old.Set("id", nextID)
			}
		}
	}
	var provider *string
	if value, ok := item.Get("provider"); ok {
		if text, ok := value.(string); ok {
			provider = &text
		}
	}
	var baseURL *string
	if value, ok := item.Get("baseUrl"); ok {
		if text, ok := value.(string); ok {
			baseURL = &text
		}
	}
	fields := ModelFields{
		Model:       nextID,
		Name:        name,
		Description: anyStringPointer(description),
		Provider:    provider,
		BaseURL:     baseURL,
	}
	if value, ok := item.Get("reasoningEfforts"); ok {
		fields.ReasoningEfforts = jsonc.Plain(value)
	}
	next, err := m.model(source.harness, fields, old, nil)
	if err != nil {
		return nil, err
	}
	return m.ordered(source.harness, next), nil
}

func anyStringPointer(value any) *string {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		return &text
	}
	return nil
}

// Batch applies a one-shot change across every harness.
func (m *ModelsManager) Batch(change ModelBatchChange) (ModelBatchResult, error) {
	if change.Action != "replace" && change.Action != "add" && change.Action != "delete" {
		return ModelBatchResult{}, fmt.Errorf("批量模型参数无效")
	}
	valid := func(value string) bool {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
		return plainModelID(trimmed) != ""
	}
	if !valid(change.Model) || (change.Action == "replace" && !valid(change.OriginalModel)) {
		return ModelBatchResult{}, fmt.Errorf("请输入有效的模型 ID")
	}
	requested := plainModelID(strings.TrimSpace(change.Model))
	original := ""
	if change.OriginalModel != "" {
		original = plainModelID(strings.TrimSpace(change.OriginalModel))
	}
	if change.Action == "replace" && requested == original {
		return ModelBatchResult{}, fmt.Errorf("原模型与新模型不能相同")
	}
	result := ModelBatchResult{Harnesses: []string{}}
	err := m.exclusive(func() error {
		var changes []*fileChange
		harnessSeen := map[string]bool{}
		changed, skipped := 0, 0
		sources, err := m.sources()
		if err != nil {
			return err
		}
		for _, source := range sources {
			file, err := m.load(source)
			if err != nil {
				detail := err.Error()
				if missingFile(err) {
					detail = "未找到配置文件"
				} else {
					detail = "读取配置失败"
				}
				return fmt.Errorf("%s：%s", source.harness, detail)
			}
			matchedID := requested
			if change.Action == "replace" {
				matchedID = original
			}
			matched := []int{}
			if change.Action != "add" {
				for index, item := range file.entries {
					if plainModelID(modelID(source.harness, item)) == matchedID {
						matched = append(matched, index)
					}
				}
			}
			alreadyExists := false
			for _, item := range file.entries {
				if plainModelID(modelID(source.harness, item)) == requested {
					alreadyExists = true
					break
				}
			}
			if (change.Action == "add" && alreadyExists) || (change.Action != "add" && len(matched) == 0) {
				skipped++
				continue
			}
			if change.Action == "replace" && alreadyExists {
				return fmt.Errorf("%s 已存在新模型，无法替换；本次未写入配置", source.harness)
			}
			entries := append([]*jsonc.Obj{}, file.entries...)
			if change.Action == "delete" {
				remaining := make([]*jsonc.Obj, 0, len(entries))
				for index, item := range entries {
					keep := true
					for _, matchedIndex := range matched {
						if index == matchedIndex {
							keep = false
							break
						}
					}
					if keep {
						remaining = append(remaining, item)
					}
				}
				entries = remaining
				// 从后向前删除，保持后续索引有效，并保留其他模型的顺序和注释。
				for i := len(matched) - 1; i >= 0; i-- {
					index := matched[i]
					changes = append(changes, &fileChange{source: source, file: file, entries: remaining, index: &index, deleting: true})
				}
				changed += len(matched)
			} else if change.Action == "add" {
				if len(file.entries) == 0 {
					return fmt.Errorf("%s 没有可复制的最后一条模型配置；本次未写入配置", source.harness)
				}
				last := file.entries[len(file.entries)-1]
				retargeted, err := m.retarget(source, change.Action, requested, last)
				if err != nil {
					return err
				}
				entries = append(entries, retargeted)
				changes = append(changes, &fileChange{source: source, file: file, entries: entries})
				changed++
			} else {
				for _, index := range matched {
					retargeted, err := m.retarget(source, change.Action, requested, file.entries[index])
					if err != nil {
						return err
					}
					entries[index] = retargeted
				}
				for _, index := range matched {
					idx := index
					changes = append(changes, &fileChange{source: source, file: file, entries: entries, index: &idx})
				}
				changed += len(matched)
			}
			if source.harness == "dsh" {
				mirror := source
				mirror.path = m.dshPath("web")
				other, err := m.load(mirror)
				if err != nil {
					return err
				}
				changes = append(changes, &fileChange{source: mirror, file: other, entries: entries, replace: true})
			}
			if !harnessSeen[source.harness] {
				harnessSeen[source.harness] = true
				result.Harnesses = append(result.Harnesses, source.harness)
			}
		}
		if change.Action == "replace" && changed == 0 {
			return fmt.Errorf("未找到原模型，请检查完整模型 ID")
		}
		if change.Action == "delete" && changed == 0 {
			return fmt.Errorf("未找到对应模型，请检查完整模型 ID")
		}
		result.Changed = changed
		result.Skipped = skipped
		return m.commitAll(changes)
	})
	if err != nil {
		return ModelBatchResult{}, err
	}
	return result, nil
}

func (m *ModelsManager) prepare(source sourceConfig, file *parsedFile, entries []*jsonc.Obj, index *int, deleting bool, order []int, baseURL *string) ([]*fileChange, error) {
	changes := []*fileChange{{source: source, file: file, entries: entries, index: index, deleting: deleting, order: order, baseURL: baseURL}}
	if source.harness == "dsh" {
		mirror := source
		mirror.path = m.dshPath("web")
		other, err := m.load(mirror)
		if err != nil {
			return nil, err
		}
		replace := canonicalEntries(other.entries) != canonicalEntries(entries)
		changes = append(changes, &fileChange{source: mirror, file: other, entries: entries, index: index, deleting: deleting, order: order, baseURL: baseURL, replace: replace})
	}
	return changes, nil
}

func canonicalEntries(entries []*jsonc.Obj) string {
	parts := make([]string, 0, len(entries))
	for _, item := range entries {
		parts = append(parts, jsonc.Canonical(item))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (m *ModelsManager) commit(source sourceConfig, file *parsedFile, entries []*jsonc.Obj, deleting bool, order []int, baseURL *string, index int) error {
	var indexPtr *int
	if index >= 0 {
		indexPtr = &index
	}
	var orderPtr []int
	if order != nil {
		orderPtr = order
	}
	changes, err := m.prepare(source, file, entries, indexPtr, deleting, orderPtr, baseURL)
	if err != nil {
		return err
	}
	return m.commitAll(changes)
}

func (m *ModelsManager) commitAll(changes []*fileChange) error {
	type groupedChange struct {
		source sourceConfig
		file   *parsedFile
		text   string
	}
	grouped := map[string]*groupedChange{}
	var orderList []string
	for _, change := range changes {
		previous := grouped[change.source.path]
		if previous != nil && !sameOriginal(previous.file.file.original, change.file.file.original) {
			return fmt.Errorf("配置文件已被其他程序修改，本次未写入，请刷新后重试")
		}
		current := change.file
		if previous != nil {
			reparse := fileState{original: previous.file.file.original, text: strings.TrimPrefix(previous.text, "\uFEFF"), bom: previous.file.file.bom}
			parsed, err := m.parseFile(change.source, reparse)
			if err != nil {
				return err
			}
			current = parsed
		}
		text, err := m.render(change.source, current, change.entries, change.index, change.deleting, change.order, change.baseURL, change.replace)
		if err != nil {
			return err
		}
		if previous == nil {
			orderList = append(orderList, change.source.path)
			grouped[change.source.path] = &groupedChange{source: change.source, file: change.file, text: text}
		} else {
			grouped[change.source.path].text = text
		}
	}
	files := make([]*groupedChange, 0, len(orderList))
	for _, path := range orderList {
		files = append(files, grouped[path])
	}
	type stagedFile struct {
		path      string
		temporary string
		original  *string
		written   bool
	}
	var staged []*stagedFile
	defer func() {
		for _, item := range staged {
			_ = os.Remove(item.temporary)
		}
	}()
	for _, item := range files {
		if item.text == derefOriginal(item.file.file.original) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.source.path), 0o755); err != nil {
			return err
		}
		temporary := filepath.Join(filepath.Dir(item.source.path), ".wide-models-"+randomHex()+".tmp")
		if err := os.WriteFile(temporary, []byte(item.text), 0o600); err != nil {
			return err
		}
		entry := &stagedFile{path: item.source.path, temporary: temporary, original: item.file.file.original}
		staged = append(staged, entry)
		if item.file.file.original != nil {
			if err := os.MkdirAll(m.backups, 0o755); err != nil {
				return err
			}
			profile := ""
			if item.source.harness == "dsh" {
				if item.source.path == m.dshPath("web") {
					profile = "-web"
				} else {
					profile = "-desktop"
				}
			}
			id := regexp.MustCompile(`[^a-zA-Z0-9-]`).ReplaceAllString(item.source.id, "_")
			extension := ".json"
			if item.source.harness == "dsh" {
				extension = ".yml"
			}
			backup := filepath.Join(m.backups, fmt.Sprintf("%s%s-%d-%s%s", id, profile, timestampMillis(), randomHex(), extension))
			if err := os.WriteFile(backup, []byte(*item.file.file.original), 0o600); err != nil {
				return err
			}
		}
	}
	// 所有配置通过检查并备份后再替换，失败时按相反顺序恢复。
	for _, item := range files {
		if item.text == derefOriginal(item.file.file.original) {
			continue
		}
		current, err := m.read(item.source.path, true)
		if err != nil {
			return fmt.Errorf("配置文件已被其他程序修改，本次未写入，请刷新后重试")
		}
		if !sameOriginal(current.original, item.file.file.original) {
			return fmt.Errorf("配置文件已被其他程序修改，本次未写入，请刷新后重试")
		}
	}
	for _, item := range staged {
		if err := os.Rename(item.temporary, item.path); err != nil {
			var rollbackFailed bool
			for i := len(staged) - 1; i >= 0; i-- {
				rollback := staged[i]
				if !rollback.written {
					continue
				}
				if rollback.original == nil {
					if err := os.Remove(rollback.path); err != nil {
						rollbackFailed = true
					}
				} else if err := os.WriteFile(rollback.path, []byte(*rollback.original), 0o600); err != nil {
					rollbackFailed = true
				}
			}
			if rollbackFailed {
				return fmt.Errorf("写入配置失败，部分文件未能恢复，请从 wide-pure 的 model-backups 目录恢复")
			}
			return fmt.Errorf("写入模型配置失败：%v", err)
		}
		item.written = true
	}
	return nil
}

func sameOriginal(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func derefOriginal(value *string) string {
	if value == nil {
		return "\x00missing"
	}
	return *value
}
