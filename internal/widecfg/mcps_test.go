package widecfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wide-pure/internal/jsonc"
)

func TestMcpsClaudeLifecycle(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{
  // keep me
  "other": 1,
  "mcpServers": {
    "fs": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@x/fs"]
    }
  }
}`)
	m := NewMcpsManager(home, filepath.Join(home, "backups"), nil)
	inv := m.Inventory()
	if len(inv.Sources) != 2 {
		t.Fatalf("sources = %d", len(inv.Sources))
	}
	claude := inv.Sources[0]
	if !claude.Editable || len(claude.Servers) != 1 || claude.Servers[0].Name != "fs" {
		t.Fatalf("claude = %+v", claude)
	}
	detail, err := m.Detail(McpTarget{Harness: "claude", Name: "fs", Revision: claude.Servers[0].Revision})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Command != "npx" || len(detail.Args) != 2 || detail.Transport != "stdio" {
		t.Fatalf("detail = %+v", detail)
	}
	// 新增 http 型 MCP
	if err := m.Save(McpChange{
		Harness: "claude",
		Fields: McpFields{Name: "web", Transport: "http", URL: "https://example.com/mcp",
			Headers: map[string]string{"X-Test": "1"}, Env: map[string]string{}, Args: nil},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	out := string(text)
	if !strings.Contains(out, "// keep me") || !strings.Contains(out, `"other": 1`) {
		t.Fatalf("comment or sibling lost:\n%s", out)
	}
	if !strings.Contains(out, "https://example.com/mcp") {
		t.Fatalf("new mcp missing:\n%s", out)
	}
	// 修改：stdio → http 时清除 command/args
	inv = m.Inventory()
	var fsServer *McpServer
	for i := range inv.Sources[0].Servers {
		if inv.Sources[0].Servers[i].Name == "fs" {
			fsServer = &inv.Sources[0].Servers[i]
		}
	}
	if err := m.Save(McpChange{
		Harness: "claude",
		Target:  &McpTarget{Harness: "claude", Name: "fs", Revision: fsServer.Revision},
		Fields:  McpFields{Name: "fs", Transport: "http", URL: "http://127.0.0.1:9", Env: map[string]string{}, Args: nil},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(home, ".claude.json"))
	if strings.Contains(string(text), "npx") {
		t.Fatalf("command not cleared on transport change:\n%s", text)
	}
	// 删除
	inv = m.Inventory()
	var webServer *McpServer
	for i := range inv.Sources[0].Servers {
		if inv.Sources[0].Servers[i].Name == "web" {
			webServer = &inv.Sources[0].Servers[i]
		}
	}
	if err := m.Delete(McpTarget{Harness: "claude", Name: "web", Revision: webServer.Revision}); err != nil {
		t.Fatal(err)
	}
	inv = m.Inventory()
	if len(inv.Sources[0].Servers) != 1 {
		t.Fatalf("delete failed: %+v", inv.Sources[0])
	}
}

const codexToml = `# codex config
model = "gpt-5"
approve = "on-request"

[mcp_servers.docs]
url = "https://docs.example/mcp"
http_headers = { X-A = "b" }

[mcp_servers.tools]
command = "uvx"
args = ["tool-mcp"]

[profiles.fast]
model = "gpt-5-mini"
`

func TestMcpsCodexLifecycle(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), codexToml)
	m := NewMcpsManager(home, filepath.Join(home, "backups"), nil)
	inv := m.Inventory()
	codex := inv.Sources[1]
	if !codex.Editable || len(codex.Servers) != 2 {
		t.Fatalf("codex = %+v", codex)
	}
	// node_repl 应被隐藏；此配置没有 → 全部可见
	// 修改 tools 的 command
	var tools *McpServer
	for i := range codex.Servers {
		if codex.Servers[i].Name == "tools" {
			tools = &codex.Servers[i]
		}
	}
	detail, err := m.Detail(McpTarget{Harness: "codex", Name: "tools", Revision: tools.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Command != "uvx" || detail.Args[0] != "tool-mcp" {
		t.Fatalf("detail = %+v", detail)
	}
	if err := m.Save(McpChange{
		Harness: "codex",
		Target:  &McpTarget{Harness: "codex", Name: "tools", Revision: tools.Revision},
		Fields:  McpFields{Name: "tools", Transport: "stdio", Command: "npx", Args: []string{"tools-mcp"}, Env: map[string]string{}},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	out := string(text)
	if !strings.Contains(out, "# codex config") {
		t.Fatalf("header comment lost:\n%s", out)
	}
	if !strings.Contains(out, `model = "gpt-5"`) || !strings.Contains(out, `[profiles.fast]`) || !strings.Contains(out, `model = "gpt-5-mini"`) {
		t.Fatalf("unrelated content lost:\n%s", out)
	}
	if !strings.Contains(out, `command = "npx"`) {
		t.Fatalf("update missing:\n%s", out)
	}
	if !strings.Contains(out, `url = "https://docs.example/mcp"`) {
		t.Fatalf("docs table lost:\n%s", out)
	}
	// 新增
	if err := m.Save(McpChange{
		Harness: "codex",
		Fields:  McpFields{Name: "extra", Transport: "stdio", Command: "extra-cmd", Env: map[string]string{"K": "V"}},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(text), "extra-cmd") || !strings.Contains(string(text), `K = "V"`) {
		t.Fatalf("append missing:\n%s", text)
	}
	// 删除 docs
	inv = m.Inventory()
	var docs *McpServer
	for i := range inv.Sources[1].Servers {
		if inv.Sources[1].Servers[i].Name == "docs" {
			docs = &inv.Sources[1].Servers[i]
		}
	}
	if err := m.Delete(McpTarget{Harness: "codex", Name: "docs", Revision: docs.Revision}); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if strings.Contains(string(text), "docs.example") {
		t.Fatalf("docs not removed:\n%s", text)
	}
	if !strings.Contains(string(text), "extra-cmd") {
		t.Fatalf("extra lost after delete:\n%s", text)
	}
	// 解析校验
	if _, servers, err := m.parse("codex", string(text)); err != nil {
		t.Fatal(err)
	} else {
		if _, ok := servers.Get("tools"); !ok {
			t.Fatal("tools missing after delete")
		}
	}
}

func TestMcpsCodexFallbackRewrite(t *testing.T) {
	home := t.TempDir()
	// 内联表写法 → 触发整体重写路径
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "mcp_servers = { inline = { command = \"x\" } }\n")
	m := NewMcpsManager(home, filepath.Join(home, "backups"), nil)
	if err := m.Save(McpChange{
		Harness: "codex",
		Fields:  McpFields{Name: "proper", Transport: "stdio", Command: "y", Env: map[string]string{}},
	}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	_, servers, err := m.parse("codex", string(text))
	if err != nil {
		t.Fatalf("fallback rewrite invalid: %v\n%s", err, text)
	}
	if _, ok := servers.Get("inline"); !ok {
		t.Fatalf("inline entry lost:\n%s", text)
	}
	if _, ok := servers.Get("proper"); !ok {
		t.Fatalf("proper entry missing:\n%s", text)
	}
}

func TestMcpsRevisionGuard(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"a":{"type":"stdio","command":"x"}}}`)
	m := NewMcpsManager(home, "", nil)
	inv := m.Inventory()
	rev := inv.Sources[0].Servers[0].Revision
	// 错误 revision 应拒绝写入
	err := m.Save(McpChange{
		Harness: "claude",
		Target:  &McpTarget{Harness: "claude", Name: "a", Revision: rev + "x"},
		Fields:  McpFields{Name: "a", Transport: "stdio", Command: "y", Env: map[string]string{}},
	})
	if err == nil || !strings.Contains(err.Error(), "已被其他程序修改") {
		t.Fatalf("revision guard failed: %v", err)
	}
	value, _ := jsonc.Parse(`{"mcpServers":{"a":{"type":"stdio","command":"x"}}}`)
	_ = value
}
