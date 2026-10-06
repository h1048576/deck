package widecfg

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func stealRoot(doc *yamlDoc) ast.Node { return doc.Root() }

const yamlSample = `# top comment
- id: llm-pi-ai
  config:
    # providers comment
    providers:
      custom-gate:
        baseURL: http://127.0.0.1:20128
        models:
          # first model comment
          - id: gpt-x
            name: X
            reasoningEfforts:
              xhigh: high
          - id: gpt-y
            name: "Y: colon"
`

func dshModelsNode(t *testing.T, text string) (*yamlDoc, *dshDoc, *ast.MappingNode, *ast.SequenceNode) {
	t.Helper()
	parsed, err := parseDsh(text)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := parsed.provider("custom-gate")
	if err != nil {
		t.Fatal(err)
	}
	modelsValue := mappingValue(provider, "models")
	seq, ok := modelsValue.Value.(*ast.SequenceNode)
	if !ok {
		t.Fatalf("models is %T", modelsValue.Value)
	}
	return parsed.doc, parsed, provider, seq
}

func TestYamlSetScalarKey(t *testing.T) {
	text := yamlSample
	doc, _, provider, _ := dshModelsNode(t, text)
	if err := yamlMappingSet(provider, "baseURL", "http://127.0.0.1:9999"); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	t.Logf("out:\n%s", out)
	if !strings.Contains(out, "http://127.0.0.1:9999") {
		t.Fatal("baseURL not updated")
	}
	if !strings.Contains(out, "# providers comment") || !strings.Contains(out, "# first model comment") {
		t.Fatal("comments lost")
	}
	value, err := yamlToValue(stealRoot(doc))
	if err != nil {
		t.Fatal(err)
	}
	_ = value
	if _, err := parseDsh(out); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
}

func TestYamlSetNewKey(t *testing.T) {
	doc, _, provider, _ := dshModelsNode(t, yamlSample)
	yamlMappingDelete(provider, "models")
	if err := yamlMappingSet(provider, "models", []any{map[string]any{"id": "fresh", "name": "F"}}); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	t.Logf("out:\n%s", out)
	if !strings.Contains(out, "fresh") {
		t.Fatal("new models missing")
	}
	if _, err := parseDsh(out); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
}

func TestYamlSetMappingValue(t *testing.T) {
	doc, _, _, seq := dshModelsNode(t, yamlSample)
	first := seq.Values[0]
	firstMapping, ok := asMapping(first)
	if !ok {
		t.Fatalf("item is %T", first)
	}
	if err := yamlMappingSet(firstMapping, "reasoningEfforts", map[string]any{"xhigh": "ultra", "deep": "low"}); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	t.Logf("out:\n%s", out)
	if !strings.Contains(out, "ultra") || !strings.Contains(out, "deep") {
		t.Fatal("reasoningEfforts not updated")
	}
	if !strings.Contains(out, "# first model comment") {
		t.Fatal("comment lost")
	}
	if _, err := parseDsh(out); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
}

func TestYamlDeleteKey(t *testing.T) {
	doc, _, provider, _ := dshModelsNode(t, yamlSample)
	if !yamlMappingDelete(provider, "baseURL") {
		t.Fatal("delete failed")
	}
	out := doc.String()
	if strings.Contains(out, "baseURL") {
		t.Fatal("baseURL still present")
	}
	if !strings.Contains(out, "# providers comment") {
		t.Fatal("comments lost")
	}
	if _, err := parseDsh(out); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
}

func TestYamlPermuteItems(t *testing.T) {
	doc, _, _, seq := dshModelsNode(t, yamlSample)
	values := seq.Values
	seq.Values = []ast.Node{values[1], values[0]}
	out := doc.String()
	t.Logf("out:\n%s", out)
	if !strings.Contains(out, "# first model comment") {
		t.Fatal("comment did not follow item")
	}
	if _, err := parseDsh(out); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
}
