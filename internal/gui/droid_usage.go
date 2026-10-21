package gui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const factoryLimitsURL = "https://api.factory.ai/api/billing/limits"

var factoryHTTPClient = &http.Client{Timeout: 10 * time.Second}

type droidUsageItem struct {
	Label string `json:"label"`
	Used  int    `json:"used"`
	Reset string `json:"reset"`
}

// droidUsage 是推送给 Droid 页面的用量数据；Error 非空时 Items 无效。
type droidUsage struct {
	Items []droidUsageItem `json:"items"`
	Error string           `json:"error"`
}

func loadDroidUsage() droidUsage {
	failure := func(message string) droidUsage { return droidUsage{Items: make([]droidUsageItem, 0), Error: message} }
	apiKey := strings.TrimSpace(os.Getenv("FACTORY_API_KEY"))
	if apiKey == "" {
		return failure("未设置环境变量 FACTORY_API_KEY，无法查询用量")
	}
	request, err := http.NewRequest(http.MethodGet, factoryLimitsURL, nil)
	if err != nil {
		return failure("用量查询失败")
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := factoryHTTPClient.Do(request)
	if err != nil {
		return failure("用量查询失败，请检查网络")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return failure("FACTORY_API_KEY 无效或无权限")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return failure(fmt.Sprintf("用量查询失败（HTTP %d）", response.StatusCode))
	}
	type window struct {
		UsedPercent *float64 `json:"usedPercent"`
		WindowEnd   *string  `json:"windowEnd"`
	}
	var data struct {
		Limits struct {
			Standard *struct {
				FiveHour window `json:"fiveHour"`
				Weekly   window `json:"weekly"`
				Monthly  window `json:"monthly"`
			} `json:"standard"`
		} `json:"limits"`
	}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil || data.Limits.Standard == nil {
		return failure("用量接口返回的数据格式异常")
	}
	item := func(label string, w window) droidUsageItem {
		result := droidUsageItem{Label: label, Reset: "-"}
		if w.UsedPercent != nil {
			result.Used = int(math.Round(*w.UsedPercent))
		}
		if w.WindowEnd != nil {
			if end, err := time.Parse(time.RFC3339Nano, *w.WindowEnd); err == nil {
				result.Reset = end.Local().Format("01-02 15:04")
			}
		}
		return result
	}
	standard := data.Limits.Standard
	return droidUsage{Items: []droidUsageItem{
		item("5h", standard.FiveHour),
		item("7d", standard.Weekly),
		item("1m", standard.Monthly),
	}}
}

// watchDroidUsage 每分钟查询一次用量并持续推送到页面；连接释放或重连后自动结束。
func watchDroidUsage(conn *pageConnection) {
	generation := conn.currentGeneration()
	go func() {
		var expression string
		var fetchedAt time.Time
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			if conn.currentGeneration() != generation {
				return
			}
			if fetchedAt.IsZero() || time.Since(fetchedAt) >= time.Minute {
				payload, _ := json.Marshal(loadDroidUsage())
				fetchedAt = time.Now()
				version := jsString(string(payload))
				expression = fmt.Sprintf(`(() => { const usage = window.__deckDroidUsage; return !usage || usage.key === %s || usage.update(%s, %s); })()`, version, payload, version)
				if conn.currentGeneration() != generation {
					return
				}
			}
			// 页面刷新后脚本状态会重置，周期推送保证用量尽快恢复显示。
			conn.evaluateAll(expression)
			<-ticker.C
		}
	}()
}
