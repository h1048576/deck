package gui

import (
	_ "embed"
	"strconv"
	"strings"
	"time"

	"deck/internal/settings"
)

//go:embed scripts/antigravity-wide/read-usage.js
var antigravityReadUsageSource string

func (rt *applicationRuntime) loadAntigravityUsage(executablePath string) modelUsage {
	failure := func(message string) modelUsage {
		return modelUsage{Items: []modelUsageItem{}, Error: message}
	}
	// 启动脚本在预设端口占用时会顺延。读取当前进程实际监听的端口。
	command := "& " + psQuote(rt.scriptFile(settings.Antigravity)) + " -DebugPortOnly"
	if executablePath != "" {
		command += " -ExecutablePath " + psQuote(executablePath)
	}
	stdout, _, psErr := psExec(command, 8*time.Second)
	port, err := strconv.Atoi(strings.TrimSpace(stdout))
	if psErr != nil || err != nil || port < 1024 || port > 65535 {
		return failure("无法连接 Antigravity，请先通过 deck 启动 Antigravity")
	}

	targets, err := readTargets(port, antigravityPagePattern.MatchString)
	if err != nil || len(targets) == 0 {
		return failure("Antigravity 主界面尚未就绪，请打开应用后刷新")
	}
	result := failure("尚未读取到 Antigravity 用量，请登录并打开主界面后刷新")
	for _, target := range targets {
		session, err := newCdpConn(target.WebSocketDebuggerURL)
		if err != nil {
			continue
		}
		response, err := session.command("Runtime.evaluate", map[string]any{
			"expression":    antigravityReadUsageSource,
			"returnByValue": true,
			"awaitPromise":  true,
		})
		session.close()
		if err != nil {
			result = failure("Antigravity 用量查询超时或连接已断开，请稍后刷新")
			continue
		}
		if usage := decodeModelUsage(response); usage != nil {
			if usage.Error == "" && len(usage.Items) > 0 {
				return *usage
			}
			if usage.Error != "" {
				result = *usage
			}
		}
	}
	return result
}
