# deck

deck 是一个 Windows 桌面工具，用于集中启动 AI 编程应用、调整应用界面，以及管理本地 AI 工具的指令、技能、模型和 MCP 配置。

基于 Go 和 Wails v3 构建，前端使用原生 JavaScript / CSS，界面和辅助脚本内嵌在可执行文件中，无需安装 Node.js 或单独部署前端。

## 快速开始

运行环境：Windows 10 或更高版本（x64），并安装 WebView2 运行时。

1. 解压 `deck-0.0.1-windows-amd64.zip`。
2. 双击 `deck.exe` 启动。
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
| Manus（Manus Studio） | 9338 |
| Claude（Claude Desktop） | 不使用 |

- 自动检测安装位置，也可手动指定可执行文件。
- 支持单个、选中或全部应用的启动、完整退出和重启，批量操作最多同时处理 5 个应用。
- 按应用能力提供内容宽度、侧栏宽度、最大宽度、输入框高度、字体、字号和字重设置。
- 支持终端跟随、阻止摘要面板自动打开、隐藏变更面板、本地合并入口或 Git Diff 等选项，具体以对应应用页面为准。
- 应用启动后通过本地调试连接应用界面设置。
- Claude 带调试端口会拒绝启动，deck 只负责启动、退出和重启，字体通过“复制脚本”手动应用：先在 Claude 菜单的 Troubleshooting 中启用 Developer Mode，再按 `Ctrl+Alt+I` 打开开发者工具，在 Console 中粘贴并回车。脚本同时把行内代码的红色改为正文颜色。页面刷新或重启后需要重新执行。

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
- Windows Terminal 字体：修改后同步写入正式版、预览版和 Canary 版的配置。
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
| `deck.exe` | Windows x64 可执行文件，版本为 `0.0.1` |
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
.\dist\deck.exe version
```

## 数据位置

默认配置目录为 `%USERPROFILE%\.deck`，只保存 `setting.json`。应用设置和界面偏好从这个文件恢复。

运行文件单独保存在 `%LOCALAPPDATA%\deck`：

| 内容 | 路径 |
| --- | --- |
| 应用设置 | `%USERPROFILE%\.deck\setting.json` |
| 内嵌辅助脚本 | `%LOCALAPPDATA%\deck\scripts\` |
| WebView2 用户数据 | `%LOCALAPPDATA%\deck\webview2\` |
| 模型配置备份 | `%LOCALAPPDATA%\deck\model-backups\` |
| MCP 配置备份 | `%LOCALAPPDATA%\deck\mcp-backups\` |
| Windows Terminal 配置备份 | `%LOCALAPPDATA%\deck\terminal-backups\` |
| 无法读取的设置恢复副本 | `%LOCALAPPDATA%\deck\setting-backups\` |

辅助脚本和 WebView2 数据在启动时自动生成；模型、MCP 和 Windows Terminal 备份仅在修改相应配置时生成。退出 deck 后，可以删除这些运行数据，下一次启动会重新生成必要文件，`setting.json` 中的设置会保留。

升级后也可以删除旧版本在 `.deck` 下生成的 `scripts`、`webview2`、`model-backups` 和 `mcp-backups` 目录；新版不会再使用或重新创建这些目录。

可通过环境变量 `DECK_CONFIG_DIR` 指定独立的设置目录；它只影响 `setting.json` 的位置，例如：

```powershell
$env:DECK_CONFIG_DIR = 'D:\deck-data'
.\deck.exe
```

首次启动时，如果 `setting.json` 尚不存在，deck 会复制已有版本的设置，原文件会保留，不复制旧备份或运行数据。已有 `setting.json` 时不会覆盖；使用 `DECK_CONFIG_DIR` 时，也支持将该目录中的旧设置文件迁移为 `setting.json`。

所有受管应用的界面配置，以及 deck 的主题、字体、字号、模式、Harness 折叠、开机启动、启动方式、菜单顺序和字体下拉选项，都保存在同一个 `setting.json` 中。

WorkBuddy 支持隐藏模型选择下拉列表中的指定模型。在 `applications.workbuddy` 中添加 `hiddenModelPatterns` 数组，例如：

```json
"hiddenModelPatterns": [
  "^Auto$",
  "Xiaomi"
]
```

每一项使用 JavaScript 正则表达式，忽略大小写，匹配模型名称或模型 ID；命中任意一项就隐藏。上述配置隐藏 Auto 入口及名称或 ID 包含 Xiaomi 的模型。空数组 `[]` 显示全部模型，无效正则会被忽略。JSON 中的反斜杠需要转义，例如 `"\\bXiaomi\\b"`。此项只控制下拉列表显示，不会更换当前已选模型或关闭已启用的 Auto 模式。

保存后通过 deck 启动或重启 WorkBuddy，使配置生效。deck 每次启动操作和保存设置前都会从文件重新读取这项规则，无需为此重启 deck；已运行的 WorkBuddy 不会自动热更新。

Codex App 也支持同样的规则，在 `applications.codex.hiddenModelPatterns` 中配置，默认 `[]`。例如 `["^GPT-5[.]6", "^gpt-6-luna$"]` 可隐藏名称以 GPT-5.6 开头的模型，以及 ID 为 gpt-6-luna 的模型。匹配模型名称或 ID，忽略大小写；仅隐藏模型菜单选项，不改变当前会话的已选模型。保存后通过新版 deck 启动或重启 Codex 生效。

Droid（Factory）也支持相同规则，在 `applications.droid.hiddenModelPatterns` 中配置。匹配模型名称或 ID，忽略大小写；例如 `["Auto Model$", "^GPT-", "^kimi-"]`。仅隐藏模型选项，不修改当前已选模型，也不隐藏 Legacy 展开入口。Droid 的隐藏规则同样会在保存设置时从文件重新读取，避免被界面旧快照覆盖。保存后通过新版 deck 启动或重启 Droid 生效。

字体下拉框的选项来自 `setting.json` 的 `fontOptions`，deck 设置页和各应用页共用。这一项没有界面入口，请在退出 deck 后直接编辑，下次启动生效：

```json
"fontOptions": [
  "default",
  "Cascadia Mono, LXGW WenKai Mono",
  "Maple Mono Normal NF CN"
]
```

- `default` 表示系统界面字体（Segoe UI、微软雅黑）。
- 一项可以写多个字体，用英文逗号分隔，前面的字体缺字时使用后面的字体。
- 包含 `;{}<>` 或换行的字体名、重复项会被忽略；列表为空或缺失时恢复为上面的默认列表。
- 下拉框只提供候选，输入框仍可直接填写列表外的字体。

设置页“终端”分组的字体保存在 `terminal.fontFamily`，修改后会写入 `terminal.settingsPaths` 中每个 Windows Terminal 配置文件的 `profiles.defaults.font.face`；某个 profile 自己设置了字体时也会一起修改。路径可以使用 `%LOCALAPPDATA%` 等环境变量，换电脑后无需改动：

```json
"terminal": {
  "fontFamily": "Cascadia Mono, LXGW WenKai Mono",
  "settingsPaths": [
    "%LOCALAPPDATA%\\Packages\\Microsoft.WindowsTerminal_8wekyb3d8bbwe\\LocalState\\settings.json",
    "%LOCALAPPDATA%\\Packages\\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\\LocalState\\settings.json",
    "%LOCALAPPDATA%\\Packages\\Microsoft.WindowsTerminalCanary_8wekyb3d8bbwe\\LocalState\\settings.json"
  ]
}
```

- 不存在的配置文件视为未安装，会被跳过。
- 终端里的 `default` 表示 Windows Terminal 自带的 Cascadia Mono，因为系统界面字体是比例字体，不适合终端。
- 写入前会把原文件备份到 `terminal-backups`，并保留原有注释和格式。
- 路径列表为空或缺失时恢复为上面的三个默认路径。

“终端”分组的字号保存在 `terminal.fontSize`（单位为磅，6–72 的整数，默认 12），修改后会写入同一批配置文件的 `profiles.defaults.font.size`；某个 profile 自己设置了字号时也会一起修改。

“终端”分组的字重保存在 `terminal.fontWeight`（100–950 的整数，默认 100），修改后会写入同一批配置文件的 `profiles.defaults.font.weight`；某个 profile 自己设置了字重时也会一起修改。

“终端”分组的窗口大小保存在 `terminal.windowSize`，格式为“列数x行数”，修改后会写入同一批配置文件根节点的 `initialCols` 和 `initialRows`。下拉框的选项来自 `terminal.windowSizeOptions`，没有界面入口，请在退出 deck 后直接编辑，下次启动生效：

```json
"terminal": {
  "windowSize": "168x48",
  "windowSizeOptions": [
    "120x30",
    "168x48"
  ]
}
```

- 列数和行数须为 1–999 的整数，分隔符可写 `x`、`X`、`×` 或 `*`，保存时统一为 `168x48` 这种格式。
- 无效项和重复项会被忽略；列表为空或缺失时恢复为上面的默认列表。
- 下拉框只提供候选，输入框仍可直接填写列表外的大小。
- 只影响之后新打开的 Windows Terminal 窗口。

AI 工具自身的指令、技能、模型和 MCP 配置仍位于各工具的用户目录。

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
