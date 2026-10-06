package jsonc

import (
	"strings"
	"testing"
)

const sample = `{
  // top comment
  "name": "test", /* inline */
  "customModels": [
    {
      "model": "a-model",
      "displayName": "A"
    },
    {
      "model": "b-model[1m]",
      "displayName": "B" // trailing b
    }
  ],
  "empty": {},
  "emptyArr": [],
  "nested": {
    "deep": {
      "value": 1
    }
  }
}`

func mustParse(t *testing.T, text string) *Doc {
	t.Helper()
	doc, err := ParseDoc(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return doc
}

func reparsed(t *testing.T, text string) any {
	t.Helper()
	value, err := Parse(text)
	if err != nil {
		t.Fatalf("reparse failed: %v (text=%q)", err, text)
	}
	return value
}

func TestParseDecode(t *testing.T) {
	doc := mustParse(t, sample)
	root := doc.Decode().(*Obj)
	if got, _ := root.Get("name"); got != "test" {
		t.Fatalf("name = %v", got)
	}
	models := root.vals["customModels"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %d", len(models))
	}
	first := models[0].(*Obj)
	if got, _ := first.Get("model"); got != "a-model" {
		t.Fatalf("model = %v", got)
	}
}

func TestReplacePreservesComments(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.Set([]any{"customModels", 0}, NewObj().With("model", "new").With("displayName", "N"), f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "// top comment") || !strings.Contains(text, "/* inline */") {
		t.Fatalf("comments lost:\n%s", text)
	}
	if !strings.Contains(text, "// trailing b") {
		t.Fatalf("trailing comment lost:\n%s", text)
	}
	if !strings.Contains(text, `"model": "new"`) {
		t.Fatalf("replacement missing:\n%s", text)
	}
	value := reparsed(t, text)
	models := value.(*Obj).vals["customModels"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %d", len(models))
	}
}

func TestDeleteItem(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.Delete([]any{"customModels", 0}, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	models := value.vals["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %d", len(models))
	}
	if got, _ := models[0].(*Obj).Get("model"); got != "b-model[1m]" {
		t.Fatalf("remaining model = %v", got)
	}
	if !strings.Contains(text, "// trailing b") {
		t.Fatalf("trailing comment lost:\n%s", text)
	}
}

func TestDeleteLastItem(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.Delete([]any{"customModels", 1}, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	models := value.vals["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %d", len(models))
	}
	// The trailing comment lives inside the deleted item, so it goes with it.
	if strings.Contains(text, "// trailing b") {
		t.Fatalf("deleted item comment kept:\n%s", text)
	}
}

func TestAppendItem(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.InsertArray([]any{"customModels", 2}, NewObj().With("model", "c-model").With("displayName", "C"), f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	models := value.vals["customModels"].([]any)
	if len(models) != 3 {
		t.Fatalf("models = %d\n%s", len(models), text)
	}
	if got, _ := models[2].(*Obj).Get("model"); got != "c-model" {
		t.Fatalf("appended = %v", got)
	}
	if !strings.Contains(text, "// trailing b") {
		t.Fatalf("trailing comment lost:\n%s", text)
	}
}

func TestAppendIntoEmptyArray(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.InsertArray([]any{"emptyArr", 0}, "hello", f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	arr := value.vals["emptyArr"].([]any)
	if len(arr) != 1 || arr[0] != "hello" {
		t.Fatalf("emptyArr = %v\n%s", arr, text)
	}
}

func TestCreateKeyInInlineEmptyObject(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.Set([]any{"empty", "added"}, 42, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	empty := value.vals["empty"].(*Obj)
	if got, _ := empty.Get("added"); got != float64(42) {
		t.Fatalf("added = %v\n%s", got, text)
	}
}

func TestCreateNestedKey(t *testing.T) {
	doc := mustParse(t, `{
  "providers": {
    "proxy": {
      "models": []
    }
  }
}`)
	f := doc.DetectFormat()
	text, err := doc.Set([]any{"providers", "proxy", "baseUrl"}, "http://127.0.0.1:20128", f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	proxy := value.vals["providers"].(*Obj).vals["proxy"].(*Obj)
	if got, _ := proxy.Get("baseUrl"); got != "http://127.0.0.1:20128" {
		t.Fatalf("baseUrl = %v\n%s", got, text)
	}
	if _, ok := proxy.Get("models"); !ok {
		t.Fatalf("models lost:\n%s", text)
	}
}

func TestReplaceWholeArray(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	entries := []any{NewObj().With("model", "x"), NewObj().With("model", "y")}
	text, err := doc.Set([]any{"customModels"}, entries, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	models := value.vals["customModels"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %d", len(models))
	}
	if !strings.Contains(text, "// top comment") {
		t.Fatalf("root comment lost:\n%s", text)
	}
}

func TestDeleteProperty(t *testing.T) {
	doc := mustParse(t, sample)
	f := doc.DetectFormat()
	text, err := doc.Delete([]any{"nested", "deep", "value"}, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, text).(*Obj)
	deep := value.vals["nested"].(*Obj).vals["deep"].(*Obj)
	if deep.Has("value") {
		t.Fatalf("value still present:\n%s", text)
	}
}

func TestCRLF(t *testing.T) {
	text := "{\r\n  \"a\": [\r\n    1\r\n  ]\r\n}"
	doc := mustParse(t, text)
	f := doc.DetectFormat()
	if f.Eol != "\r\n" {
		t.Fatalf("eol = %q", f.Eol)
	}
	out, err := doc.InsertArray([]any{"a", 1}, 2, f)
	if err != nil {
		t.Fatal(err)
	}
	value := reparsed(t, out).(*Obj)
	arr := value.vals["a"].([]any)
	if len(arr) != 2 {
		t.Fatalf("a = %v\n%q", arr, out)
	}
	if strings.Count(out, "\r\n") < 5 {
		t.Fatalf("crlf lost: %q", out)
	}
}

func TestOrderedSetKeepsPosition(t *testing.T) {
	obj := NewObj()
	obj.Set("a", 1)
	obj.Set("b", 2)
	obj.Set("c", 3)
	obj.Set("a", 10)
	obj.Delete("b")
	obj.Set("b", 20)
	keys := obj.Keys()
	if strings.Join(keys, ",") != "a,c,b" {
		t.Fatalf("keys = %v", keys)
	}
}

// With chains Set for brevity in tests and callers.
func (o *Obj) With(key string, value any) *Obj {
	o.Set(key, value)
	return o
}
