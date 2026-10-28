package gui

import (
	_ "embed"
	"encoding/json"
)

//go:embed scripts/codex-wide/read-usage.js
var codexReadUsageSource string

type codexUsageItem struct {
	Label string   `json:"label"`
	Used  *float64 `json:"used"`
	Reset string   `json:"reset"`
}

type codexUsage struct {
	Items []codexUsageItem `json:"items"`
	Error string           `json:"error"`
}

func loadCodexUsage(port int) codexUsage {
	failure := func(message string) codexUsage {
		return codexUsage{Items: make([]codexUsageItem, 0), Error: message}
	}
	targets, err := readTargets(port, func(string) bool { return true })
	if err != nil || len(targets) == 0 {
		return failure("无法连接 Codex，请先通过 deck 启动 Codex")
	}
	result := failure("尚未读取到 Codex 用量，请登录并打开一个聊天后刷新")
	for _, target := range targets {
		session, err := newCdpConn(target.WebSocketDebuggerURL)
		if err != nil {
			continue
		}
		params := make(map[string]any)
		params["expression"] = codexReadUsageSource
		params["returnByValue"] = true
		params["awaitPromise"] = true
		response, err := session.command("Runtime.evaluate", params)
		session.close()
		if err != nil {
			result = failure("Codex 用量查询超时或连接已断开，请稍后刷新")
			continue
		}
		var envelope struct {
			Result struct {
				Value *codexUsage `json:"value"`
			} `json:"result"`
			ExceptionDetails json.RawMessage `json:"exceptionDetails"`
		}
		if json.Unmarshal(response, &envelope) != nil || len(envelope.ExceptionDetails) != 0 || envelope.Result.Value == nil {
			continue
		}
		value := *envelope.Result.Value
		if value.Error == "" && len(value.Items) > 0 {
			return value
		}
		if value.Error != "" {
			result = failure(value.Error)
		}
	}
	return result
}
