package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"deck/internal/appdir"
	"deck/internal/harnesscfg"
)

const modelsUsage = `模型管理：与桌面版 Models 的一键操作使用相同逻辑

用法：
  deck-cli models list [--json]
  deck-cli models add <模型ID> [--json]
  deck-cli models delete <模型ID> [--json]
  deck-cli models update <原模型ID> <新模型ID> [--json]

范围：Claude、Droid、DSH、pi、OpenCode 的模型配置。
  add     复制各配置的最后一条模型，替换模型 ID 和名称；已存在则跳过。
  delete  删除匹配模型；所有配置均未匹配时返回失败。
  update  替换原模型 ID，保留原有连接等配置；新 ID 已存在时返回失败。
  list    列出配置来源、模型 ID 和名称，不修改配置。

update 也可写为 replace。模型 ID 区分大小写，包含特殊字符时请加引号。
操作前检查所有配置，写入前自动备份到本机 deck 的 model-backups 目录。
配置缺失、无法读取或添加时没有可复制的模型，将沿用桌面版规则返回失败。
模型文件位于各工具的用户目录；DECK_CONFIG_DIR 不改变模型文件位置。
--json 将结果输出为 JSON；退出码：0 成功，1 操作失败，2 参数错误。
`

func runModels(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprint(stdout, modelsUsage)
		return 0
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Fprint(stdout, modelsUsage)
			return 0
		}
	}
	action := args[0]
	if action == "update" {
		action = "replace"
	}
	count, known := map[string]int{"list": 0, "add": 1, "delete": 1, "replace": 2}[action]
	if !known {
		fmt.Fprintln(stderr, "未知模型命令；使用 models --help 查看用法。")
		return 2
	}
	asJSON := false
	var ids []string
	for _, arg := range args[1:] {
		if arg == "--json" {
			asJSON = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintln(stderr, "未知模型选项；使用 models --help 查看用法。")
			return 2
		}
		id := strings.TrimSpace(arg)
		if id == "" || len(arg) > 1024 || strings.ContainsAny(arg, "\x00\r\n\t") {
			fmt.Fprintln(stderr, "请输入有效的模型 ID。")
			return 2
		}
		ids = append(ids, id)
	}
	if len(ids) != count {
		fmt.Fprintf(stderr, "该模型命令需要 %d 个模型 ID；使用 models --help 查看用法。\n", count)
		return 2
	}
	if action == "replace" && ids[0] == ids[1] {
		fmt.Fprintln(stderr, "原模型与新模型不能相同。")
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "读取用户目录失败：%+v\n", err)
		return 1
	}
	manager := harnesscfg.NewModelsManager(home, appdir.ModelBackups())
	if action == "list" {
		return listModels(manager, asJSON, stdout, stderr)
	}
	change := harnesscfg.ModelBatchChange{Action: action, Model: ids[0]}
	if action == "replace" {
		change.OriginalModel, change.Model = ids[0], ids[1]
	}
	result, err := manager.Batch(change)
	if err != nil {
		fmt.Fprintf(stderr, "模型操作失败：%+v\n", err)
		return 1
	}
	if asJSON {
		if !writeJSON(stdout, stderr, result) {
			return 1
		}
		return 0
	}
	label := map[string]string{"add": "添加", "delete": "删除", "replace": "更新"}[action]
	fmt.Fprintf(stdout, "已%s %d 条模型，跳过 %d 个配置来源。\n", label, result.Changed, result.Skipped)
	if len(result.Harnesses) > 0 {
		fmt.Fprintf(stdout, "涉及工具：%s\n", strings.Join(result.Harnesses, "、"))
		fmt.Fprintf(stdout, "备份目录：%s\n", appdir.ModelBackups())
	}
	return 0
}

func listModels(manager *harnesscfg.ModelsManager, asJSON bool, stdout, stderr io.Writer) int {
	inventory := manager.Inventory()
	code := 0
	if len(inventory.Sources) == 0 {
		fmt.Fprintln(stderr, "无法读取模型配置来源。")
		code = 1
	}
	for _, source := range inventory.Sources {
		if source.Error != "" {
			code = 1
		}
	}
	if asJSON {
		if !writeJSON(stdout, stderr, inventory) {
			return 1
		}
		return code
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	// 表头使用 ASCII，避免中英文字符显示宽度不同造成列错位。
	fmt.Fprintln(table, "SOURCE\tMODEL\tNAME")
	for _, source := range inventory.Sources {
		if source.Error != "" {
			fmt.Fprintf(table, "%s\t-\t%v\n", source.ID, source.Error)
			continue
		}
		if len(source.Models) == 0 {
			fmt.Fprintf(table, "%s\t-\t无模型\n", source.ID)
		}
		for _, model := range source.Models {
			fmt.Fprintf(table, "%s\t%s\t%s\n", source.ID, model.Model, model.Name)
		}
	}
	if err := table.Flush(); err != nil {
		fmt.Fprintf(stderr, "输出失败：%+v\n", err)
		return 1
	}
	return code
}
