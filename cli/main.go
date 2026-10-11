package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"text/tabwriter"

	"deck/internal/appdir"
	"deck/internal/gui"
	"deck/internal/settings"
)

var version = "0.0.1"

const usage = `deck-cli：应用与模型管理命令行

用法：
  deck-cli list [--json]
  deck-cli status [应用...] [--all] [--json]
  deck-cli start <应用...> [--watch] [--json]
  deck-cli restart <应用...> [--watch] [--json]
  deck-cli exit <应用...> [--json]
  deck-cli <start|restart|exit> --all [--json]
  deck-cli models list [--json]
  deck-cli models add <模型ID> [--json]
  deck-cli models delete <模型ID> [--json]
  deck-cli models update <原模型ID> <新模型ID> [--json]
  deck-cli models --help
  deck-cli version

应用：codex、droid、zcode、workbuddy、dsh、qoder、paseo、manus

选项：
  --all     处理全部应用，不能与应用名称同时使用
  --json    将结果以 JSON 输出到标准输出，进度写入标准错误
  --watch   启动或重启后保持界面调试连接；Ctrl+C 结束守护，应用继续运行
  -h, --help  显示帮助

示例：
  deck-cli start codex
  deck-cli restart codex droid
  deck-cli exit --all
  deck-cli start codex --watch
  deck-cli status --json

读取与桌面版相同的 setting.json，支持 DECK_CONFIG_DIR。
启动会跳过已运行的应用；批量操作最多并发处理 5 个应用。
默认完成后退出 CLI；需要刷新、新窗口持续应用界面设置时使用 --watch。
退出码：0 成功，1 操作失败，2 参数错误。
`

type options struct {
	command string
	ids     []string
	all     bool
	json    bool
	watch   bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func parseOptions(args []string) (options, string) {
	opts := options{command: args[0]}
	switch opts.command {
	case "list", "status", "start", "restart", "exit":
	case "stop", "quit":
		opts.command = "exit"
	default:
		return opts, "未知命令：" + opts.command
	}
	seen := map[string]bool{}
	for _, arg := range args[1:] {
		switch arg {
		case "--all":
			opts.all = true
		case "--json":
			opts.json = true
		case "--watch":
			opts.watch = true
		default:
			id := strings.ToLower(arg)
			if _, err := settings.ParseFeatureId(id); err != nil {
				return opts, "未知应用或选项：" + arg
			}
			if !seen[id] {
				seen[id] = true
				opts.ids = append(opts.ids, id)
			}
		}
	}
	if opts.all && len(opts.ids) > 0 {
		return opts, "--all 不能与应用名称同时使用"
	}
	if opts.watch && opts.command != "start" && opts.command != "restart" {
		return opts, "--watch 仅支持 start 和 restart"
	}
	if opts.command == "list" && (opts.all || len(opts.ids) > 0) {
		return opts, "list 不接受应用名称或 --all"
	}
	if opts.command != "list" && opts.command != "status" && !opts.all && len(opts.ids) == 0 {
		return opts, "请指定应用名称，或使用 --all 处理全部应用"
	}
	return opts, ""
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "models" || args[0] == "model") {
		return runModels(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Fprint(stdout, usage)
			return 0
		}
	}
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		fmt.Fprintln(stdout, "deck-cli", version)
		return 0
	}
	opts, message := parseOptions(args)
	if message != "" {
		fmt.Fprintln(stderr, message+"；使用 --help 查看用法。")
		return 2
	}
	if err := appdir.MigrateLegacyConfig(); err != nil {
		fmt.Fprintf(stderr, "读取配置失败：%+v\n", err)
		return 1
	}
	store := settings.NewStore(appdir.Config())
	store.Load()
	if warning := store.Warning(); warning != "" {
		fmt.Fprintln(stderr, warning+" 请修复配置后重试。")
		return 1
	}
	prefs := store.Prefs()
	if opts.command == "list" {
		return listApplications(prefs, opts.json, stdout, stderr)
	}
	if err := gui.ExtractScripts(appdir.ScriptsDir()); err != nil {
		fmt.Fprintf(stderr, "准备辅助脚本失败：%+v\n", err)
		return 1
	}
	manager := gui.NewApplicationManager(appdir.ScriptsDir())
	defer manager.Close()
	if opts.command == "status" {
		return showStatus(manager, prefs, opts, stdout, stderr)
	}
	var interrupted chan os.Signal
	if opts.watch {
		interrupted = make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt)
		defer signal.Stop(interrupted)
	}
	result := manager.Run(opts.command, prefs, opts.ids, progressReporter(stderr))
	code := 0
	if !result.Success {
		code = 1
	}
	// 显式指定的应用未安装时返回失败；--all 保留菜单的跳过语义。
	for _, item := range result.Results {
		if !opts.all && item.SkipReason == "not-installed" {
			result.Success = false
			code = 1
		}
	}
	if opts.json {
		if !writeJSON(stdout, stderr, result) {
			return 1
		}
	} else {
		for _, item := range result.Results {
			fmt.Fprintf(stdout, "%s：%s\n", settings.Applications[item.ID].Name, item.Message)
		}
		fmt.Fprintln(stdout, result.Message)
	}
	if opts.watch && code == 0 {
		for _, item := range result.Results {
			if item.Status == "success" {
				fmt.Fprintln(stderr, "界面守护已连接，按 Ctrl+C 结束守护（应用继续运行）。")
				<-interrupted
				return code
			}
		}
		fmt.Fprintln(stderr, "应用均已跳过，没有建立新的守护连接；需要守护时使用 restart --watch。")
	}
	return code
}

func listApplications(prefs settings.Preferences, asJSON bool, stdout, stderr io.Writer) int {
	type application struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	apps := make([]application, 0, len(prefs.MenuOrder))
	for _, id := range settings.NormalizeMenuOrder(prefs.MenuOrder) {
		apps = append(apps, application{ID: id, Name: settings.Applications[id].Name})
	}
	if asJSON {
		if !writeJSON(stdout, stderr, apps) {
			return 1
		}
		return 0
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	for _, app := range apps {
		fmt.Fprintf(table, "%s\t%s\n", app.ID, app.Name)
	}
	if err := table.Flush(); err != nil {
		fmt.Fprintf(stderr, "输出失败：%+v\n", err)
		return 1
	}
	return 0
}

func showStatus(manager *gui.ApplicationManager, prefs settings.Preferences, opts options, stdout, stderr io.Writer) int {
	ids := opts.ids
	if len(ids) == 0 {
		ids = settings.NormalizeMenuOrder(prefs.MenuOrder)
	}
	results := make([]gui.ApplicationStatus, 0, len(ids))
	code := 0
	for _, id := range ids {
		result := manager.Status(id, prefs.Applications[id])
		results = append(results, result)
		if result.Error != "" {
			code = 1
		}
	}
	if opts.json {
		if !writeJSON(stdout, stderr, results) {
			return 1
		}
		return code
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "应用\t状态\t版本\t路径 / 错误")
	for _, result := range results {
		state, detail := "未安装", result.Path
		switch {
		case result.Error != "":
			state, detail = "检测失败", result.Error
		case result.Running:
			state = "运行中"
		case result.Installed:
			state = "未运行"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", result.ID, state, result.Version, detail)
	}
	if err := table.Flush(); err != nil {
		fmt.Fprintf(stderr, "输出失败：%+v\n", err)
		return 1
	}
	return code
}

func writeJSON(stdout, stderr io.Writer, value any) bool {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "输出 JSON 失败：%+v\n", err)
		return false
	}
	return true
}

func progressReporter(stderr io.Writer) func(gui.BatchProgress) {
	var mu sync.Mutex
	previous := map[string]string{}
	labels := map[string]string{
		"queued": "等待处理", "detecting": "检测安装", "checking": "检查运行状态",
		"stopping": "正在退出", "starting": "正在启动", "waiting": "等待窗口就绪",
		"applying": "应用界面设置", "maximizing": "最大化窗口",
	}
	return func(progress gui.BatchProgress) {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range progress.ApplicationIDs {
			stage := progress.Stages[id]
			if stage != "" && stage != previous[id] {
				fmt.Fprintf(stderr, "[%s] %s\n", settings.Applications[id].Name, labels[stage])
				previous[id] = stage
			}
		}
		for _, result := range progress.Results {
			if result.Status == "error" && previous[result.ID] != result.Message {
				fmt.Fprintf(stderr, "[%s] %s\n", settings.Applications[result.ID].Name, result.Message)
				previous[result.ID] = result.Message
			}
		}
	}
}
