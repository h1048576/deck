package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	workbuddyUsageURL      = "https://www.codebuddy.cn/billing/meter/get-enterprise-user-usage"
	workbuddyDailyUsageURL = "https://www.codebuddy.cn/billing/meter/get-user-daily-usage"
	workbuddyAuthPath      = `AppData\Local\CodeBuddyExtension\Data\Public\auth\Tencent-Cloud.coding-copilot.info`
)

// 积分接口按北京时间统计当天用量；固定时区避免依赖系统 tzdata。
var workbuddyCreditsZone = time.FixedZone("CST", 8*60*60)

var workbuddyHTTPClient = &http.Client{Timeout: 15 * time.Second}

// workbuddyCredits 是推送给页面的积分数据；Error 非空时其余字段无效。
type workbuddyCredits struct {
	// Today 是今日使用；当日接口失败时为“查询失败”等提示。
	Today string `json:"today"`
	// Used 是接口 credit 字段，即已使用积分。
	Used  string `json:"used"`
	Limit string `json:"limit"`
	Error string `json:"error"`
}

// creditsTarget 描述积分在各应用中的挂载位置和样式。
type creditsTarget struct {
	// Container 是承载积分的工具栏选择器。
	Container string `json:"container"`
	// Placement 为 first 时放在工具栏最前面，为 last 时放在最后面。
	Placement string `json:"placement"`
	// HideAlone 为 true 时，工具栏没有其他内容就不显示积分。
	HideAlone bool   `json:"hideAlone"`
	CSS       string `json:"css"`
}

var creditsTargets = map[string]creditsTarget{
	// DSH 底部用量栏：放在 context 用量之后，样式与 context 按钮和弹层一致；新会话首页的用量栏为空且被原生样式隐藏。
	"dsh": {Container: `[data-composer-dock="true"]`, Placement: "last", HideAlone: true, CSS: `
    [data-wide-workbuddy-credits] {
      display: flex;
      flex: 0 0 auto;
      align-items: center;
      gap: 6px;
      box-sizing: border-box;
      padding: 1px 8px;
      border: 0;
      border-radius: var(--dsw-radius-sm);
      background: none;
      color: var(--dsw-alias-label-tertiary);
      font: inherit;
      font-variant-numeric: tabular-nums;
      line-height: calc(20px + var(--dsh-content-font-delta-secondary, 0px));
      white-space: nowrap;
      cursor: pointer;
    }
    [data-wide-workbuddy-credits]:hover,
    [data-wide-workbuddy-credits][aria-expanded="true"] {
      background: var(--dsw-alias-interactive-bg-hover);
      color: var(--dsw-alias-label-secondary);
    }
    [data-wide-workbuddy-credits-popover] {
      position: fixed;
      z-index: 1100;
      box-sizing: border-box;
      min-width: 200px;
      max-width: calc(100vw - 16px);
      padding: 12px;
      border: 0;
      border-radius: var(--dsw-radius-lg);
      background: var(--dsw-specific-menu);
      backdrop-filter: var(--dsw-menu-backdrop-filter);
      --dsw-elevation-stroke-color: var(--dsw-alias-border-l1);
      box-shadow: var(--dsw-elevation-prominent);
      color: var(--dsw-alias-label-secondary);
      cursor: default;
    }
    [data-wide-workbuddy-credits-value] {
      color: var(--dsw-alias-label-primary);
    }
  `},
	// WorkBuddy 输入框右侧工具栏：放在上下文用量和模型选择的左侧，样式与上下文用量弹层一致。
	"workbuddy": {Container: `[data-cr-input-toolbar-right="true"]`, Placement: "first", CSS: `
    [data-wide-workbuddy-credits] {
      display: flex;
      flex: 0 0 auto;
      align-items: center;
      gap: 4px;
      box-sizing: border-box;
      height: 32px;
      padding: 0 8px;
      border: 0;
      border-radius: 999px;
      background: none;
      color: var(--cr-text-secondary);
      font: inherit;
      font-variant-numeric: tabular-nums;
      white-space: nowrap;
      cursor: pointer;
      user-select: none;
      transition: background-color 0.15s;
    }
    [data-wide-workbuddy-credits]:hover,
    [data-wide-workbuddy-credits][aria-expanded="true"] {
      background-color: var(--cr-bg-hover);
      color: var(--cr-text-primary);
    }
    [data-wide-workbuddy-credits-popover] {
      position: fixed;
      z-index: 10000;
      box-sizing: border-box;
      min-width: 200px;
      max-width: calc(100vw - 16px);
      padding: 12px 16px;
      border: 1px solid var(--cr-context-popover-border, var(--cr-border-default));
      border-radius: 16px;
      background: var(--cr-context-popover-bg, var(--cr-bg-elevated));
      box-shadow: var(--cr-shadow-popover);
      color: var(--cr-text-secondary);
    }
    [data-wide-workbuddy-credits-value] {
      color: var(--cr-text-primary);
    }
  `},
}

// creditsCommonCSS 是两个应用共用的弹层布局。
const creditsCommonCSS = `
    [data-wide-workbuddy-credits] svg {
      flex: 0 0 auto;
    }
    [data-wide-workbuddy-credits-row] {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 16px;
      padding: 2px 0;
      white-space: nowrap;
    }
    [data-wide-workbuddy-credits-value] {
      font-variant-numeric: tabular-nums;
    }
  `

// installCreditsJS 在应用工具栏中显示「今日|已用」积分按钮，点击弹出详情；数据由 deck 定时推送。
const installCreditsJS = `function installWorkbuddyCredits(options) {
  const key = '__wideWorkbuddyCredits';
  const icon = '<svg viewBox="0 0 14 14" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.1" stroke-linecap="round" aria-hidden="true"><circle cx="7" cy="7" r="5.5"></circle><path d="M8.5 5.3C8.2 4.8 7.7 4.5 7 4.5c-.9 0-1.5.5-1.5 1.1 0 1.5 3.1.9 3.1 2.6 0 .7-.7 1.3-1.6 1.3-.8 0-1.4-.3-1.7-.9M7 3.6v.9M7 9.5v.9"></path></svg>';
  const install = () => {
    // 早期版本只在 DSH 中以另一名称安装，替换时一并移除（render 中也会清理其残留节点），避免出现两份积分。
    window.__wideDshCredits?.dispose();
    delete window.__wideDshCredits;
    const previous = window[key];
    previous?.dispose();
    if (!options.enabled) { delete window[key]; return true; }
    let style = document.getElementById('wide-workbuddy-credits-style');
    if (!style) {
      style = document.createElement('style');
      style.id = 'wide-workbuddy-credits-style';
      (document.head || document.documentElement).appendChild(style);
    }
    style.textContent = options.css;
    let state = previous?.state || { today: '', used: '', limit: '', error: '' };
    let stateKey = previous?.key || '';
    let frame = 0;
    let popover = null;
    let opened = null;
    const loading = () => !state.error && !state.used;
    const number = value => /^\d+$/.test(value) ? value : '--';
    const rows = () => state.error ? [[state.error, '']]
      : loading() ? [['正在查询积分…', '']]
      : [['today', state.today || '--'], ['remain', state.used + '/' + state.limit]];
    const fillPopover = () => {
      if (!popover) return;
      popover.replaceChildren(...rows().map(([name, value]) => {
        const row = document.createElement('div');
        row.setAttribute('data-wide-workbuddy-credits-row', '');
        const label = document.createElement('span');
        label.textContent = name;
        row.append(label);
        if (value) {
          const text = document.createElement('span');
          text.setAttribute('data-wide-workbuddy-credits-value', '');
          text.textContent = value;
          row.append(text);
        }
        return row;
      }));
    };
    const place = () => {
      if (!popover || !opened) return;
      const anchor = opened.getBoundingClientRect();
      const box = popover.getBoundingClientRect();
      let top = anchor.top - box.height - 8;
      if (top < 8) top = anchor.bottom + 8;
      popover.style.left = Math.max(8, Math.min(anchor.left, innerWidth - box.width - 8)) + 'px';
      popover.style.top = top + 'px';
    };
    const close = () => {
      opened?.setAttribute('aria-expanded', 'false');
      opened = null;
      popover?.remove();
      popover = null;
    };
    const open = trigger => {
      close();
      opened = trigger;
      trigger.setAttribute('aria-expanded', 'true');
      popover = document.createElement('div');
      popover.setAttribute('data-wide-workbuddy-credits-popover', '');
      popover.setAttribute('role', 'dialog');
      popover.setAttribute('aria-label', 'WorkBuddy 积分');
      fillPopover();
      document.body.append(popover);
      place();
    };
    const render = () => {
      frame = 0;
      for (const legacy of document.querySelectorAll('[data-wide-dsh-credits]')) legacy.remove();
      const compact = number(state.today) + '|' + number(state.used);
      const label = 'WorkBuddy 积分：' + rows().map(row => row.filter(Boolean).join(' ')).join('，');
      for (const container of document.querySelectorAll(options.container)) {
        // 每个工具栏只保留一份积分，防止多次注入或原生重渲染留下重复节点。
        const [first, ...extra] = container.querySelectorAll(':scope > [data-wide-workbuddy-credits]');
        for (const node of extra) node.remove();
        let item = first;
        if (options.hideAlone && ![...container.children].some(child => child !== item)) { item?.remove(); continue; }
        if (!item) {
          item = document.createElement('button');
          item.type = 'button';
          item.setAttribute('data-wide-workbuddy-credits', '');
          item.setAttribute('aria-haspopup', 'dialog');
          item.setAttribute('aria-expanded', 'false');
          item.innerHTML = icon + '<span data-wide-workbuddy-credits-text></span>';
        }
        if (options.placement === 'first') {
          if (container.firstElementChild !== item) container.prepend(item);
        } else if (container.lastElementChild !== item) {
          container.append(item);
        }
        const text = item.querySelector('[data-wide-workbuddy-credits-text]');
        if (text.textContent !== compact) text.textContent = compact;
        if (item.getAttribute('aria-label') !== label) item.setAttribute('aria-label', label);
      }
      if (opened && !opened.isConnected) close();
    };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(render); };
    const onClick = event => {
      const trigger = event.target instanceof Element ? event.target.closest('[data-wide-workbuddy-credits]') : null;
      if (!trigger) return;
      event.preventDefault();
      event.stopPropagation();
      if (opened === trigger) close(); else open(trigger);
    };
    const onPointerDown = event => {
      if (!popover || event.target instanceof Node && (popover.contains(event.target) || opened?.contains(event.target))) return;
      close();
    };
    const onKeyDown = event => {
      if (event.key !== 'Escape' || !popover) return;
      event.stopPropagation();
      close();
    };
    document.addEventListener('click', onClick, true);
    document.addEventListener('pointerdown', onPointerDown, true);
    document.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('resize', close);
    window.addEventListener('blur', close);
    const observer = new MutationObserver(schedule);
    observer.observe(document.documentElement, { childList: true, subtree: true });
    window[key] = {
      get state() { return state; },
      get key() { return stateKey; },
      update(next, nextKey) {
        if (next && typeof next === 'object' && nextKey !== stateKey) {
          state = next;
          stateKey = nextKey;
          render();
          fillPopover();
          place();
        }
        return true;
      },
      dispose() {
        observer.disconnect();
        cancelAnimationFrame(frame);
        close();
        document.removeEventListener('click', onClick, true);
        document.removeEventListener('pointerdown', onPointerDown, true);
        document.removeEventListener('keydown', onKeyDown, true);
        window.removeEventListener('resize', close);
        window.removeEventListener('blur', close);
        style.remove();
        for (const item of document.querySelectorAll('[data-wide-workbuddy-credits]')) item.remove();
      },
    };
    render();
    return true;
  };
  if (document.documentElement) return install();
  document.addEventListener('DOMContentLoaded', install, { once: true });
  return true;
}`

// creditsInjection 返回安装或移除积分显示的脚本；不支持积分的应用返回空串。
func creditsInjection(id string, enabled bool) string {
	target, ok := creditsTargets[id]
	if !ok {
		return ""
	}
	target.CSS += creditsCommonCSS
	payload, _ := json.Marshal(struct {
		creditsTarget
		Enabled bool `json:"enabled"`
	}{target, enabled})
	return "(" + installCreditsJS + ")(" + string(payload) + ")"
}

// watchWorkbuddyCredits 每分钟查询一次积分，并持续推送到应用页面；连接释放或重连后自动结束。
func watchWorkbuddyCredits(conn *pageConnection) {
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
				payload, _ := json.Marshal(loadWorkbuddyCredits())
				fetchedAt = time.Now()
				version := jsString(string(payload))
				expression = fmt.Sprintf(`(() => { const credits = window.__wideWorkbuddyCredits; return !credits || credits.key === %s || credits.update(%s, %s); })()`, version, payload, version)
				if conn.currentGeneration() != generation {
					return
				}
			}
			// 页面刷新后脚本状态会重置，周期推送保证积分尽快恢复显示。
			conn.evaluateAll(expression)
			<-ticker.C
		}
	}()
}

func loadWorkbuddyCredits() workbuddyCredits {
	failure := func(message string) workbuddyCredits { return workbuddyCredits{Error: message} }
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return failure("无法确定用户目录")
	}
	text, err := os.ReadFile(filepath.Join(home, workbuddyAuthPath))
	if err != nil {
		return failure("无法读取登录信息，请先登录 CodeBuddy")
	}
	var info struct {
		Account map[string]any `json:"account"`
		Auth    map[string]any `json:"auth"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(text, []byte("\uFEFF")), &info); err != nil {
		return failure("无法读取登录信息，请先登录 CodeBuddy")
	}
	enterpriseID, _ := info.Account["enterpriseId"].(string)
	accessToken, _ := info.Auth["accessToken"].(string)
	if strings.TrimSpace(enterpriseID) == "" || strings.TrimSpace(accessToken) == "" {
		return failure("登录信息缺少 enterpriseId 或 accessToken")
	}
	headers := map[string]string{
		"Authorization":   "Bearer " + accessToken,
		"x-enterprise-id": enterpriseID,
		"Content-Type":    "application/json",
	}
	// 不输出原始异常、响应或认证信息，避免泄露令牌。
	status, data, err := postWorkbuddy(workbuddyUsageURL, headers, "{}")
	if err != nil {
		return failure("积分查询失败，请检查网络或登录状态")
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return failure("登录失效或无权限，请重新登录 CodeBuddy")
	}
	if status < 200 || status >= 300 {
		return failure(fmt.Sprintf("积分查询失败（HTTP %d）", status))
	}
	if data == nil {
		return failure("积分接口返回业务错误，请稍后重试")
	}
	used, okUsed := creditPoints(data["credit"])
	limit, okLimit := creditPoints(data["limitNum"])
	if !okUsed || !okLimit {
		return failure("积分接口返回的数据格式异常")
	}
	return workbuddyCredits{Today: loadWorkbuddyDailyUsage(headers), Used: formatCredit(used), Limit: formatCredit(limit)}
}
func loadWorkbuddyDailyUsage(headers map[string]string) string {
	date := time.Now().In(workbuddyCreditsZone).Format("2006-01-02")
	body, _ := json.Marshal(map[string]any{
		"startTime": date + " 00:00:00", "endTime": date + " 23:59:59", "pageNum": 1, "pageSize": 1,
	})
	status, data, err := postWorkbuddy(workbuddyDailyUsageURL, headers, string(body))
	if err != nil || status < 200 || status >= 300 || data == nil {
		return "查询失败"
	}
	list, ok := data["data"].([]any)
	if !ok {
		return "数据异常"
	}
	if len(list) == 0 {
		if total, ok := creditPoints(data["total"]); ok && total == 0 {
			return "0"
		}
		return "数据异常"
	}
	first, _ := list[0].(map[string]any)
	credit, ok := creditPoints(first["credit"])
	if !ok {
		return "数据异常"
	}
	return formatCredit(credit)
}

// postWorkbuddy 返回 HTTP 状态和 code 为 0 时的 data；业务错误时 data 为 nil。
func postWorkbuddy(url string, headers map[string]string, body string) (int, map[string]any, error) {
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := workbuddyHTTPClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, nil, nil
	}
	text, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	var result struct {
		Code *float64       `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(text, &result); err != nil {
		return 0, nil, err
	}
	if result.Code == nil || *result.Code != 0 {
		return response.StatusCode, nil, nil
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	return response.StatusCode, result.Data, nil
}

func creditPoints(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, !math.IsInf(v, 0) && !math.IsNaN(v)
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	}
	return 0, false
}

func formatCredit(value float64) string {
	return strconv.FormatFloat(math.Trunc(value), 'f', -1, 64)
}
