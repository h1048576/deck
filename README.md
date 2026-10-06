# wide-pure

[wide](https://github.com/) 的纯 Go 重实现：一个启动并"换肤"AI 编程桌面应用的 Windows 工具。
技术方案取自 [magpie](https://github.com/yetone/magpie)：Wails v3 只做窗口壳，前端是无框架的
原生 JS/CSS（go:embed 内嵌，无打包器），Go ↔ JS 通过同一个 http.Handler 上的 JSON API 通信。

## 功能

- **7 个受管应用**：Codex、Droid、ZCode、WorkBuddy、DSH、Qoder、Paseo（端口 9331-9337）。
  - 启动 / 完整退出 / 重启（单个、选中、全部；批量并发 5）。
  - 安装检测（结果缓存 60s / 运行态 15s，缺省自动探测常见安装路径，也可手动指定 exe）。
  - 每个应用的界面定制：布局（侧边栏/内容宽度、最大宽度）、字体（字体、字号、字重）、
    终端跟随、隐藏本地合并 / Git Diff、阻止摘要面板、DSH 思考级别等，随应用启动时
    通过 CDP（`--remote-debugging-port` + WebSocket 注入）生效。
- **Harness 管理**（`~/.claude`、`~/.agents`、`~/.factory`、`~/.codex`、`~/.dsh`、`~/.pi`、`~/.opencode`）：
  - AGENTS.md：以 Claude 的 CLAUDE.md 为源文件，预览 / 一键同步到其余 harness。
  - Skills：跨 harness 查看、同步、删除（claude ↔ agents 双向）。
  - Models：各 harness 配置文件中的模型列表增删改、复制、拖拽排序、批量修改；
    按格式做位置感知编辑——JSON/JSONC 保留注释与格式（Claude、Droid、pi、opencode），
    YAML 保留注释（DSH 的 cordis.patch.yml），TOML 按表块拼接（Codex config.toml）。
    每次保存前自动备份原文件（`model-backups/`、`mcp-backups/`）。
  - MCPs：各 harness 的 MCP 服务器配置增删改（JSONC / TOML），支持 stdio 与 HTTP 传输、
    env / headers / bearerTokenEnvVar。
- **wide-pure 自身设置**：主题（跟随系统/亮/暗）、字体字号、紧凑模式、Harness 区块折叠记忆、
  开机启动（HKCU Run 键）、菜单顺序（拖拽或 Alt+↑/↓，持久化到 settings.json）。
- **安全关闭**：操作进行中或设置未落盘时取消关闭并提示；单实例锁（二次启动唤起已有窗口）。

## 构建

依赖：Go 1.25+、Windows 10+（WebView2 运行时）。

```sh
# 开发构建（带控制台日志）
make build            # 或: go build -o wide-pure.exe .

# 浏览器预览（无窗口，用于前端调试）
make serve            # 然后: ./wide-pure-serve.exe serve   → http://127.0.0.1:3420

# Windows 发布版：图标 + 版本信息 + DPI 清单 + 无控制台（-H windowsgui）
make release-windows  # 产出 dist/wide-pure-windows-amd64.exe

make test             # jsonc / widecfg / settings 单元测试
```

没有 make 时（Makefile 里就三条命令）：

```sh
go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch amd64 --out rsrc \
  --file-version 0.1.0 --product-version 0.1.0
go build -tags production -trimpath -ldflags="-s -w -X main.version=0.1.0 -H windowsgui" -o dist/wide-pure-windows-amd64.exe .
rm rsrc_windows_amd64.syso
```

注意：若机器全局设置了 `GO111MODULE=off`，构建前需 `export GO111MODULE=on`。

## 目录结构

```
main.go                 入口：serve / version / 默认 GUI
gui_on.go gui_off.go    构建标签（默认 GUI，-tags nogui 为纯服务）
console_windows.go      控制台模式下 AttachConsole
internal/appdir         配置目录定位（%APPDATA%/wide-pure，可用 WIDE_PURE_CONFIG_DIR 覆盖）
internal/settings       settings.json 读写：防抖原子写、损坏备份、校验、菜单序规范化
internal/jsonc          JSONC 位置感知编辑器（保留注释/格式；Set/Insert/Delete + Canonical/Pretty）
internal/widecfg        Harness/Models/MCPs 业务：JSONC/YAML(goccy 注释保留)/TOML 三种编辑器、同步与备份
internal/gui            Wails 装配、JSON API、PowerShell 运行器、CDP 注入、图标、前端资产
internal/gui/assets     原生 ES modules 前端（app.js 外壳 + harnesspage + dialogs/controls/resources）
internal/gui/scripts    droid.ps1 等随 exe 解包的辅助脚本
build/                  图标与 winres 资源（build/windows/winres.json）
```

## 数据位置

- 设置：`%APPDATA%/wide-pure/settings.json`（自动保存，写前备份损坏文件）。
- 模型/MCP 备份：`%APPDATA%/wide-pure/model-backups/`、`mcp-backups/`。
- WebView2 用户数据：`%LOCALAPPDATA%/wide-pure/WebViewData`。

## 与 wide（Electron 版）的对应

| wide | wide-pure |
| --- | --- |
| Electron main（窗口/单实例/托盘行为） | Wails v3 窗口 + SingleInstance + 关闭守卫 |
| IPC handlers | `/api/*` JSON 端点（同一个 http.Handler） |
| React + 打包器 | 原生 ES modules + CSS，go:embed 直出 |
| JS 注入脚本 | 同一脚本经 CDP WebSocket 注入（coder/websocket） |
| PowerShell 子进程管理 | EncodedCommand + CLIXML 流式解码（psrun） |
| 回收站删除 / 开机启动 | SHFileOperationW(FOF_ALLOWUNDO) / HKCU Run 键 |
