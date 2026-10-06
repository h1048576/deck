# deck

deck 是一个 Windows 桌面工具，用于集中启动 AI 编程应用、调整应用界面，以及管理本地 AI 工具的指令、技能、模型和 MCP 配置。

基于 Go 和 Wails v3 构建，前端使用原生 JavaScript / CSS，界面和辅助脚本内嵌在可执行文件中，无需安装 Node.js 或单独部署前端。

## 快速开始

运行环境：Windows 10 或更高版本（x64），并安装 WebView2 运行时。

1. 解压 `deck-0.0.1-windows-amd64.zip`。
2. 双击 `deck-windows-amd64.exe` 启动。
3. 在对应应用页面检查安装路径；如果自动检测失败，可手动选择应用的 `.exe` 文件。
4. 调整布局和字体后，通过 deck 启动或重启应用，使界面设置生效。

deck 不包含受管应用，请先安装需要使用的 AI 编程工具。发布包为免安装版本，设置保存在当前用户的数据目录中。

## 功能

### 应用管理

支持以下桌面应用，默认调试端口可在设置中修改：

| 应用 | 默认端口 |
| --- | --- |
| Codex | 9331 |
| Droid | 9332 |
| ZCode | 9333 |
| WorkBuddy | 9334 |
| DSH | 9335 |
| Qoder | 9336 |
| Paseo | 9337 |

- 自动检测安装位置，也可手动指定可执行文件。
- 支持单个、选中或全部应用的启动、完整退出和重启，批量操作最多同时处理 5 个应用。
- 按应用能力提供内容宽度、侧栏宽度、最大宽度、输入框高度、字体、字号和字重设置。
- 支持终端跟随、阻止摘要面板自动打开、隐藏变更面板、本地合并入口或 Git Diff 等选项，具体以对应应用页面为准。
- 应用启动后通过本地调试连接应用界面设置。

### 本地配置管理

| 配置类型 | 支持范围 | 操作 |
| --- | --- | --- |
| 指令文件 | Claude、Agents、Droid、Codex | 预览 `~/.claude/CLAUDE.md`，同步到其他工具的 `AGENTS.md` |
| Skills | Claude、Agents、Droid、Codex | 查看、同步、删除技能；Claude 与 Agents 支持双向同步 |
| Models | Claude、Droid、DSH、pi、OpenCode | 新增、编辑、复制、删除、排序和批量修改模型配置 |
| MCPs | Claude、Codex | 新增、编辑、复制和删除 MCP 服务器，支持 stdio 与 HTTP 传输 |

配置编辑按文件格式处理 JSON / JSONC、YAML 和 TOML，尽量保留原有注释和格式。模型和 MCP 配置保存前会自动备份原文件。

### deck 设置

- 跟随系统、亮色或暗色主题。
- 字体、字号与紧凑模式。
- Harness 区块折叠状态记忆。
- 开机启动与应用启动方式。
- 菜单拖拽排序，也可使用 `Alt + ↑ / ↓` 调整顺序。
- 单实例运行，再次启动会唤起已有窗口；操作进行中或设置保存中会阻止立即关闭。

## 构建

需要 Go 1.25 或更高版本。以下命令在项目根目录的 PowerShell 中执行。

### 开发构建

```powershell
$env:GO111MODULE = 'on'
go build -o deck.exe .
.\deck.exe
```

### Windows 发布包

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build\build-windows.ps1 -Version 0.0.1
```

构建脚本自动生成 Windows 图标、版本资源与 DPI 清单，再构建无控制台窗口的桌面程序。首次构建需要下载 Go 依赖和资源生成工具。

产物位于 `dist/`：

| 文件 | 说明 |
| --- | --- |
| `deck-windows-amd64.exe` | Windows x64 可执行文件，版本为 `0.0.1` |
| `deck-0.0.1-windows-amd64.zip` | 包含可执行文件和 README 的免安装包 |
| `deck-0.0.1-windows-amd64.sha256` | 可执行文件和 ZIP 包的 SHA-256 校验值 |

如已安装 GNU Make，也可使用：

```powershell
make build
make serve
make release-windows VERSION=0.0.1
```

### 浏览器预览

```powershell
$env:GO111MODULE = 'on'
go build -tags nogui -o deck-serve.exe .
.\deck-serve.exe serve
```

然后在浏览器中打开 `http://127.0.0.1:3420`。也可通过 `serve 127.0.0.1:3421` 指定地址。浏览器模式用于界面调试，窗口控制和桌面文件选择等功能需要桌面版本。

查看版本：

```powershell
.\dist\deck-windows-amd64.exe version
```

## 数据位置

默认配置目录为 `%APPDATA%\deck`：

| 内容 | 路径 |
| --- | --- |
| 应用设置 | `%APPDATA%\deck\settings.json` |
| 模型配置备份 | `%APPDATA%\deck\model-backups\` |
| MCP 配置备份 | `%APPDATA%\deck\mcp-backups\` |
| WebView2 用户数据 | `%APPDATA%\deck\webview2\` |
| 内嵌辅助脚本 | `%APPDATA%\deck\scripts\` |

可通过环境变量 `DECK_CONFIG_DIR` 指定独立配置目录，例如：

```powershell
$env:DECK_CONFIG_DIR = 'D:\deck-data'
.\deck.exe
```

首次启动时，如果默认配置目录尚不存在，deck 会复制已有版本的设置和模型 / MCP 备份，原文件会保留。已有配置目录或指定了 `DECK_CONFIG_DIR` 时，不执行迁移。

AI 工具自身的指令、技能、模型和 MCP 配置仍位于各工具的用户目录；deck 的配置目录用于保存自身设置、备份和运行数据。

## 目录结构

```text
main.go                     命令入口：桌面启动、浏览器预览和版本信息
gui_on.go / gui_off.go       桌面与 nogui 构建入口
console_windows.go          Windows 命令行输出支持
internal/appdir/            数据目录、配置迁移和路径定位
internal/settings/          设置校验、自动保存和原子写入
internal/jsonc/             保留注释与格式的 JSONC 编辑器
internal/harnesscfg/        指令、技能、模型和 MCP 配置管理
internal/gui/               Wails 窗口、JSON API、应用管理和界面注入
internal/gui/assets/        内嵌 JavaScript、CSS 和图标
internal/gui/scripts/       内嵌应用启动与进程管理脚本
build/build-windows.ps1     Windows 发布构建与打包脚本
build/windows/              Windows 图标、版本资源和清单配置
dist/                       构建产物
```
