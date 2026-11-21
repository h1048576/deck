package gui

import (
	_ "embed"
	"encoding/json"
)

//go:embed scripts/codex-wide/read-usage.js
var codexReadUsageSource string

type modelUsageItem struct {
	Label string   `json:"label"`
	Used  *float64 `json:"used"`
	Reset string   `json:"reset"`
}

type modelUsage struct {
	Items []modelUsageItem `json:"items"`
	Error string           `json:"error"`
}

func loadCodexUsage(port int) modelUsage {
	failure := func(message string) modelUsage {
		return modelUsage{Items: []modelUsageItem{}, Error: message}
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
		value := decodeModelUsage(response)
		if value == nil {
			continue
		}
		if value.Error == "" && len(value.Items) > 0 {
			return *value
		}
		if value.Error != "" {
			result = failure(value.Error)
		}
	}
	return result
}

func decodeModelUsage(response json.RawMessage) *modelUsage {
	var envelope struct {
		Result struct {
			Value *modelUsage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if json.Unmarshal(response, &envelope) != nil || len(envelope.ExceptionDetails) != 0 {
		return nil
	}
	return envelope.Result.Value
}
