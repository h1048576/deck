package widecfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const claudeSettings = `{
  // user comment
  "modelPicker": {
    "options": [
      {
        "model": "old-model",
        "label": "Old"
      }
    ]
  },
  "other": "keep"
}`

func TestModelsClaudeSaveAndComments(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), claudeSettings)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	inv := m.Inventory()
	if len(inv.Sources) < 5 {
		t.Fatalf("sources = %d", len(inv.Sources))
	}
	var claudeSource *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "claude" {
			claudeSource = &inv.Sources[i]
		}
	}
	if claudeSource == nil || !claudeSource.Editable || len(claudeSource.Models) != 1 {
		t.Fatalf("claude source = %+v", claudeSource)
	}
	description := "uses old-model daily"
	change := ModelChange{
		SourceID: claudeSource.ID,
		Target:   &ModelTarget{SourceID: claudeSource.ID, Index: 0, Revision: claudeSource.Models[0].Revision},
		Fields:   ModelFields{Model: "new-model", Name: "New", Description: &description},
	}
	if err := m.Save(change); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	out := string(text)
	if !strings.Contains(out, "// user comment") {
		t.Fatalf("comment lost:\n%s", out)
	}
	if !strings.Contains(out, `"new-model"`) || !strings.Contains(out, `"uses old-model daily"`) {
		t.Fatalf("update missing:\n%s", out)
	}
	if !strings.Contains(out, `"other": "keep"`) {
		t.Fatalf("unrelated key lost:\n%s", out)
	}
}

func TestModelsClaudeAppendAndKeyOrder(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), claudeSettings)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	change := ModelChange{
		SourceID: "claude",
		Fields:   ModelFields{Model: "added-model", Name: "Added"},
	}
	if err := m.Save(change); err != nil {
		t.Fatal(err)
	}
	inv := m.Inventory()
	var claude *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "claude" {
			claude = &inv.Sources[i]
		}
	}
	if len(claude.Models) != 2 {
		t.Fatalf("models = %d", len(claude.Models))
	}
	if claude.Models[1].Model != "added-model" || claude.Models[1].Name != "Added" {
		t.Fatalf("appended = %+v", claude.Models[1])
	}
	// 字段顺序规范：model 在 label 之前。
	text, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	modelPos := strings.Index(string(text), `"model": "added-model"`)
	labelPos := strings.Index(string(text), `"label": "Added"`)
	if modelPos < 0 || labelPos < 0 || modelPos > labelPos {
		t.Fatalf("key order wrong:\n%s", text)
	}
}

func TestModelsClaudeReorderAndDelete(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), claudeSettings)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	if err := m.Save(ModelChange{SourceID: "claude", Fields: ModelFields{Model: "second", Name: "Two"}}); err != nil {
		t.Fatal(err)
	}
	inv := m.Inventory()
	var claude *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "claude" {
			claude = &inv.Sources[i]
		}
	}
	order := []ModelTarget{
		{SourceID: claude.ID, Index: 1, Revision: claude.Models[1].Revision},
		{SourceID: claude.ID, Index: 0, Revision: claude.Models[0].Revision},
	}
	if err := m.Reorder(ModelOrder{SourceID: claude.ID, Models: order}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	first := strings.Index(string(text), `"second"`)
	second := strings.Index(string(text), `"old-model"`)
	if first > second {
		t.Fatalf("reorder failed:\n%s", text)
	}
	if err := m.Delete(ModelTarget{SourceID: claude.ID, Index: 0, Revision: claude.Models[1].Revision}); err != nil {
		t.Fatal(err)
	}
	inv2 := m.Inventory()
	for i := range inv2.Sources {
		if inv2.Sources[i].Harness == "claude" && len(inv2.Sources[i].Models) != 1 {
			t.Fatalf("delete failed: %d", len(inv2.Sources[i].Models))
		}
	}
}

const droidSettings = `{
  "customModels": [
    {
      "model": "droid-old",
      "id": "custom:droid-old",
      "baseUrl": "http://127.0.0.1:20128",
      "provider": "generic-chat-completion-api",
      "displayName": "Old Droid"
    }
  ]
}`

func TestModelsDroidCopyAndBatch(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".factory", "settings.json"), droidSettings)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), claudeSettings)
	writeFile(t, filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"), dshYamlDoc)
	writeFile(t, filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml"), dshYamlDoc)
	writeFile(t, filepath.Join(home, ".pi", "agent", "models.json"), "{\n \"providers\": { \"proxy\": { \"models\": [] } }\n}")
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "{\n \"provider\": { \"proxy\": { \"models\": {} } }\n}")
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	result, err := m.Batch(ModelBatchChange{Action: "replace", Model: "brand-new", OriginalModel: "droid-old"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Skipped != 4 {
		t.Fatalf("batch = %+v", result)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".factory", "settings.json"))
	if !strings.Contains(string(text), "custom:brand-new") || !strings.Contains(string(text), "brand-new") {
		t.Fatalf("replace failed:\n%s", text)
	}
	if strings.Contains(string(text), "droid-old") {
		t.Fatalf("old id remains:\n%s", text)
	}
}

const dshYamlDoc = `# patch header
- id: llm-pi-ai
  config:
    providers:
      # custom gate
      custom-gate:
        baseURL: http://127.0.0.1:20128
        models:
          # first
          - id: gpt-x
            name: X
            reasoningEfforts:
              xhigh: high
`

func TestModelsDshYamlLifecycle(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"), dshYamlDoc)
	writeFile(t, filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml"), dshYamlDoc)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	inv := m.Inventory()
	var dshSource *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].ID == "dsh:custom-gate" {
			dshSource = &inv.Sources[i]
		}
	}
	if dshSource == nil || !dshSource.Editable || len(dshSource.Models) != 1 {
		t.Fatalf("dsh source = %+v", dshSource)
	}
	// 修改现有模型
	if err := m.Save(ModelChange{
		SourceID: dshSource.ID,
		Target:   &ModelTarget{SourceID: dshSource.ID, Index: 0, Revision: dshSource.Models[0].Revision},
		Fields:   ModelFields{Model: "gpt-x", Name: "X2", BaseURL: &dshSource.BaseURL, ReasoningEfforts: map[string]any{"xhigh": "ultra"}},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	if !strings.Contains(string(text), "X2") || !strings.Contains(string(text), "ultra") {
		t.Fatalf("dsh update failed:\n%s", text)
	}
	if !strings.Contains(string(text), "# first") || !strings.Contains(string(text), "# custom gate") {
		t.Fatalf("dsh comments lost:\n%s", text)
	}
	// web profile 镜像写回
	webText, _ := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml"))
	if !strings.Contains(string(webText), "X2") {
		t.Fatalf("web mirror missing:\n%s", webText)
	}
	// 新增模型
	if err := m.Save(ModelChange{
		SourceID: dshSource.ID,
		Fields:   ModelFields{Model: "gpt-z", Name: "Z", BaseURL: &dshSource.BaseURL, ReasoningEfforts: map[string]any{"xhigh": "high"}},
	}); err != nil {
		t.Fatal(err)
	}
	inv = m.Inventory()
	for i := range inv.Sources {
		if inv.Sources[i].ID == "dsh:custom-gate" && len(inv.Sources[i].Models) != 2 {
			t.Fatalf("dsh append failed: %d", len(inv.Sources[i].Models))
		}
	}
}

func TestModelsOpencodeObjectForm(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "proxy": {
      "options": { "baseURL": "http://127.0.0.1:20128" },
      "models": {
        "legacy": { "name": "Legacy" }
      }
    }
  }
}`)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	inv := m.Inventory()
	var source *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "opencode" {
			source = &inv.Sources[i]
		}
	}
	if source == nil || len(source.Models) != 1 || source.Models[0].Model != "legacy" {
		t.Fatalf("opencode source = %+v", source)
	}
	change := ModelChange{
		SourceID: source.ID,
		Fields:   ModelFields{Model: "fresh", Name: "Fresh", BaseURL: &source.BaseURL},
	}
	if err := m.Save(change); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if !strings.Contains(string(text), `"fresh"`) || !strings.Contains(string(text), "$schema") {
		t.Fatalf("opencode save failed:\n%s", text)
	}
	inv = m.Inventory()
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "opencode" && len(inv.Sources[i].Models) != 2 {
			t.Fatalf("opencode append failed")
		}
	}
}

func TestModelsPiArrayForm(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "models.json"), `{
  "providers": {
    "proxy": {
      "baseUrl": "http://127.0.0.1:20128",
      "models": [
        { "id": "pi-one", "name": "One" }
      ]
    }
  }
}`)
	m := NewModelsManager(home, filepath.Join(home, "backups"))
	inv := m.Inventory()
	var source *ModelSource
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "pi" {
			source = &inv.Sources[i]
		}
	}
	if source == nil || len(source.Models) != 1 {
		t.Fatalf("pi source = %+v", source)
	}
	if err := m.Save(ModelChange{SourceID: source.ID, Fields: ModelFields{Model: "pi-two", Name: "Two", BaseURL: &source.BaseURL}}); err != nil {
		t.Fatal(err)
	}
	inv = m.Inventory()
	for i := range inv.Sources {
		if inv.Sources[i].Harness == "pi" {
			if len(inv.Sources[i].Models) != 2 || inv.Sources[i].Models[1].Model != "pi-two" {
				t.Fatalf("pi append failed: %+v", inv.Sources[i].Models)
			}
		}
	}
}
