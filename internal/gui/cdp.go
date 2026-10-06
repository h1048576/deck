package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// LogFunc receives operation progress; errors surface to the user.
type LogFunc func(message string, level string) // level: "info" | "error"

func (log LogFunc) errorf(format string, args ...any) {
	log(fmt.Sprintf(format, args...), "error")
}

type cdpTarget struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type cdpPending struct {
	reply chan cdpMessage
}

// cdpConn is a single WebSocket debugging session.
type cdpConn struct {
	url       string
	conn      *websocket.Conn
	mu        sync.Mutex
	nextID    int
	pending   map[int]*cdpPending
	listeners map[string]func()
	closed    chan struct{}
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

func newCdpConn(targetURL string) (*cdpConn, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return nil, fmt.Errorf("应用调试地址不是本机地址")
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, fmt.Errorf("应用调试地址不是本机地址")
	}
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	conn, _, err := websocket.Dial(dialCtx, targetURL, &websocket.DialOptions{})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("应用调试连接已关闭")
	}
	conn.SetReadLimit(2 * 1024 * 1024)
	session := &cdpConn{
		url:       targetURL,
		conn:      conn,
		pending:   map[int]*cdpPending{},
		listeners: map[string]func(){},
		closed:    make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	go session.readLoop()
	return session, nil
}

func (c *cdpConn) readLoop() {
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			c.fail(fmt.Errorf("应用调试连接已关闭"))
			return
		}
		var message cdpMessage
		if err := json.Unmarshal(data, &message); err != nil {
			c.close()
			return
		}
		if message.Method != "" {
			c.mu.Lock()
			listener := c.listeners[message.Method]
			c.mu.Unlock()
			if listener != nil {
				listener()
			}
			continue
		}
		c.mu.Lock()
		entry := c.pending[message.ID]
		delete(c.pending, message.ID)
		c.mu.Unlock()
		if entry != nil {
			entry.reply <- message
		}
	}
}

func (c *cdpConn) fail(err error) {
	c.mu.Lock()
	for id, entry := range c.pending {
		select {
		case entry.reply <- cdpMessage{Error: &struct {
			Message string `json:"message"`
		}{Message: err.Error()}}:
		default:
		}
		delete(c.pending, id)
	}
	c.mu.Unlock()
	c.close()
}

func (c *cdpConn) close() {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
		close(c.closed)
	})
}

// command sends a CDP command with the 5s timeout the launcher uses.
func (c *cdpConn) command(method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	entry := &cdpPending{reply: make(chan cdpMessage, 1)}
	c.pending[id] = entry
	c.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	writeCtx, cancelWrite := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancelWrite()
	if err := c.conn.Write(writeCtx, websocket.MessageText, payload); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("应用调试连接已关闭")
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case message := <-entry.reply:
		if message.Error != nil {
			return nil, fmt.Errorf("%s", message.Error.Message)
		}
		return message.Result, nil
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("应用命令超时：%s", method)
	case <-c.closed:
		return nil, fmt.Errorf("调试连接已结束")
	}
}

func (c *cdpConn) on(method string, callback func()) {
	c.mu.Lock()
	c.listeners[method] = callback
	c.mu.Unlock()
}

func (c *cdpConn) isConnected() bool {
	select {
	case <-c.closed:
		return false
	default:
		return true
	}
}

// evaluate runs an expression and returns the Runtime.evaluate result.
func (c *cdpConn) evaluate(expression string) (json.RawMessage, json.RawMessage, error) {
	result, err := c.command("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true})
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		Result           json.RawMessage `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	_ = json.Unmarshal(result, &parsed)
	return parsed.Result, parsed.ExceptionDetails, nil
}

// pageConnection keeps style injection alive across reloads and new windows.
type pageConnection struct {
	name        string
	styleID     string
	acceptsPage func(url string) bool

	mu          sync.Mutex
	sessions    map[string]*cdpConn
	watchGen    int
	watchStop   chan struct{}
	stoppedOnce sync.Once
}

func newPageConnection(name string, styleID string, acceptsPage func(string) bool) *pageConnection {
	if acceptsPage == nil {
		acceptsPage = func(string) bool { return true }
	}
	return &pageConnection{
		name:        name,
		styleID:     styleID,
		acceptsPage: acceptsPage,
		sessions:    map[string]*cdpConn{},
	}
}

func (p *pageConnection) dispose() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.watchGen++
	if p.watchStop != nil {
		p.stoppedOnce.Do(func() {})
		close(p.watchStop)
		p.watchStop = nil
	}
	for url, session := range p.sessions {
		session.close()
		delete(p.sessions, url)
	}
}

func readTargets(port int, acceptsPage func(string) bool) ([]cdpTarget, error) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("无法读取 应用调试页面")
	}
	var data []cdpTarget
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("应用调试响应格式无效")
	}
	targets := make([]cdpTarget, 0, len(data))
	for _, target := range data {
		if target.Type != "page" || target.WebSocketDebuggerURL == "" || target.URL == "" {
			continue
		}
		if strings.HasPrefix(target.URL, "devtools://") || target.URL == "about:blank" {
			continue
		}
		if !acceptsPage(target.URL) {
			continue
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (p *pageConnection) attachTarget(target cdpTarget, source string, log LogFunc, generation int) error {
	session, err := newCdpConn(target.WebSocketDebuggerURL)
	if err != nil {
		return err
	}
	defer func() {
		if session.isConnected() && p.sessionLive(target.WebSocketDebuggerURL) != target.WebSocketDebuggerURL {
			session.close()
		}
	}()
	if _, err := session.command("Page.enable", map[string]any{}); err != nil {
		return err
	}
	ready, exceptions, err := session.evaluate(`!!document.documentElement && document.readyState !== "loading"`)
	if err != nil {
		return err
	}
	if exceptions != nil || !rawIsTrue(ready) {
		return fmt.Errorf("主页面尚未完成加载。")
	}
	result, exceptions, err := session.evaluate(source)
	if err != nil {
		return err
	}
	if exceptions != nil {
		detail := rawExceptionDetail(exceptions)
		return fmt.Errorf("应用界面注入失败：%s", detail)
	}
	if !rawIsTrue(result) {
		return fmt.Errorf("应用界面尚未就绪，设置未生效。")
	}
	if _, err := session.command("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": source}); err != nil {
		return err
	}
	session.on("Page.domContentEventFired", func() {
		if generation != p.currentGeneration() {
			return
		}
		go func() {
			_, exceptions, err := session.evaluate(source)
			if err == nil && exceptions != nil {
				log("应用页面刷新后应用设置失败，请再次应用。", "error")
			}
		}()
	})
	if generation != p.currentGeneration() {
		session.close()
		return nil
	}
	p.mu.Lock()
	p.sessions[target.WebSocketDebuggerURL] = session
	p.mu.Unlock()
	// 此会话持续保留，以确保页面刷新时注入规则仍然有效。
	if !isWindowsRuntime() {
		if windowResult, err := session.command("Browser.getWindowForTarget", map[string]any{}); err == nil {
			var window struct {
				WindowID int `json:"windowId"`
			}
			if json.Unmarshal(windowResult, &window) == nil {
				if _, err := session.command("Browser.setWindowBounds", map[string]any{
					"windowId": window.WindowID,
					"bounds":   map[string]any{"windowState": "maximized"},
				}); err != nil {
					log("界面设置已生效；当前平台未提供窗口最大化接口。", "info")
				}
			}
		}
	}
	return nil
}

func (p *pageConnection) sessionLive(url string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions[url] != nil {
		return url
	}
	return ""
}

func (p *pageConnection) currentGeneration() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.watchGen
}

func rawIsTrue(raw json.RawMessage) bool {
	if raw == nil {
		return false
	}
	var value struct {
		Value bool `json:"value"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return value.Value
}

func rawExceptionDetail(raw json.RawMessage) string {
	var detail struct {
		Exception *struct {
			Description string `json:"description"`
		} `json:"exception"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return "未知脚本错误"
	}
	if detail.Exception != nil && detail.Exception.Description != "" {
		return detail.Exception.Description
	}
	if detail.Text != "" {
		return detail.Text
	}
	return "未知脚本错误"
}

// connect attaches to every debug page and keeps the injection alive.
func (p *pageConnection) connect(port int, source string, log LogFunc) error {
	p.dispose()
	p.mu.Lock()
	generation := p.watchGen
	p.mu.Unlock()
	targets, err := readTargets(port, p.acceptsPage)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("应用没有可用的调试页面")
	}
	var firstError error
	attached := 0
	for _, target := range targets {
		if attachErr := p.attachTarget(target, source, log, generation); attachErr != nil {
			if firstError == nil {
				firstError = attachErr
			}
			continue
		}
		attached++
	}
	if attached == 0 {
		detail := "主页面尚未就绪"
		if firstError != nil {
			detail = firstError.Error()
		}
		return fmt.Errorf("%s 应用设置失败：%s", p.name, detail)
	}
	go p.watch(port, source, log, generation)
	return nil
}

func (p *pageConnection) watch(port int, source string, log LogFunc, generation int) {
	stop := make(chan struct{})
	p.mu.Lock()
	p.watchStop = stop
	p.mu.Unlock()
	checking := false
	unavailable := 0
	reported := map[string]bool{}
	failingSince := map[string]time.Time{}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		if checking || generation != p.currentGeneration() {
			continue
		}
		checking = true
		targets, err := readTargets(port, p.acceptsPage)
		if err != nil {
			checking = false
			unavailable++
			if unavailable >= 5 && generation == p.currentGeneration() {
				p.dispose()
				log("应用调试连接已结束。下次启动时请再次应用设置。", "info")
				return
			}
			continue
		}
		if generation != p.currentGeneration() {
			return
		}
		unavailable = 0
		urls := map[string]bool{}
		for _, target := range targets {
			urls[target.WebSocketDebuggerURL] = true
		}
		p.mu.Lock()
		for url := range failingSince {
			if !urls[url] {
				delete(failingSince, url)
				delete(reported, url)
			}
		}
		for url, session := range p.sessions {
			if !urls[url] || !session.isConnected() {
				session.close()
				delete(p.sessions, url)
			}
		}
		sessions := map[string]*cdpConn{}
		for url, session := range p.sessions {
			sessions[url] = session
		}
		p.mu.Unlock()
		for _, target := range targets {
			if existing, ok := sessions[target.WebSocketDebuggerURL]; ok {
				// 页面加载事件之外，再检查一次守护状态，兼容应用内部替换页面。
				result, _, err := existing.evaluate(fmt.Sprintf(`!!document.getElementById(%q)`, p.styleID))
				if err == nil && !rawIsTrue(result) {
					if _, _, err := existing.evaluate(source); err != nil {
						// 页面导航结束后会在下一轮重试。
					}
				}
				continue
			}
			if attachErr := p.attachTarget(target, source, log, generation); attachErr != nil {
				if generation != p.currentGeneration() {
					return
				}
				since, ok := failingSince[target.WebSocketDebuggerURL]
				if !ok {
					since = time.Now()
					failingSince[target.WebSocketDebuggerURL] = since
				}
				// 新窗口和导航期间持续重试，避免瞬时失败把整次批量启动改成失败。
				if time.Since(since) >= 60*time.Second && !reported[target.WebSocketDebuggerURL] {
					reported[target.WebSocketDebuggerURL] = true
					log(fmt.Sprintf("%s 新页面应用设置失败：%s", p.name, attachErr.Error()), "error")
				}
				continue
			}
			delete(reported, target.WebSocketDebuggerURL)
			delete(failingSince, target.WebSocketDebuggerURL)
		}
		checking = false
	}
}

var isWindows = isWindowsRuntime
