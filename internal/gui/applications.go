package gui

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"deck/internal/settings"
)

// Report mirrors the launcher's report(message, level).
type Report func(message string, level string)

func noopReport(string, string) {}

type installation struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

var cdpPortPattern = regexp.MustCompile(`CDP 端口 (\d+)`)
var failPattern = regexp.MustCompile(`失败：`)

type detectCacheEntry struct {
	value   *installation
	expires time.Time
}

type applicationRuntime struct {
	scriptsDir string
	home       string
	droid      *droidRuntime

	mu            sync.Mutex
	connections   map[string]*pageConnection
	cache         map[string]detectCacheEntry
	pendingDetect map[string]*detectWaiter
	detectMutex   sync.Mutex
	detectResults map[string]*installation
	detectErrors  map[string]error
}

func newApplicationRuntime(scriptsDir string) *applicationRuntime {
	rt := &applicationRuntime{
		scriptsDir:    scriptsDir,
		connections:   map[string]*pageConnection{},
		cache:         map[string]detectCacheEntry{},
		pendingDetect: map[string]*detectWaiter{},
		detectResults: map[string]*installation{},
		detectErrors:  map[string]error{},
	}
	rt.droid = newDroidRuntime(rt)
	return rt
}

// connection lazily creates the persistent page connection for an app.
func (rt *applicationRuntime) connection(id string) *pageConnection {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	value := rt.connections[id]
	if value == nil {
		styleID := id + "-wide-" + map[bool]string{true: "width", false: "ui"}[id == "codex" || id == "paseo"] + "-override"
		var accepts func(string) bool
		switch id {
		case "qoder":
			accepts = func(url string) bool {
				return regexp.MustCompile(`^qoder(?:-cn)?-app://renderer/`).MatchString(url)
			}
		case "dsh":
			accepts = func(url string) bool { return strings.HasPrefix(url, "dsh-app://app/") }
		}
		value = newPageConnection(settings.Applications[id].Name, styleID, accepts)
		rt.connections[id] = value
	}
	return value
}

// disposeConnections drops every persistent page connection.
func (rt *applicationRuntime) disposeConnections() {
	rt.droid.dispose()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for id, connection := range rt.connections {
		connection.dispose()
		delete(rt.connections, id)
	}
}

func (rt *applicationRuntime) disposeConnection(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if connection, ok := rt.connections[id]; ok {
		connection.dispose()
		delete(rt.connections, id)
	}
}

// DetectApplication resolves an installation with a short cache.
func (rt *applicationRuntime) DetectApplication(id string, path string, force bool) (*installation, error) {
	key := id + "\x00" + path
	rt.detectMutex.Lock()
	if !force {
		if cached, ok := rt.cache[key]; ok && time.Now().Before(cached.expires) {
			if cached.value == nil {
				rt.detectMutex.Unlock()
				return nil, nil
			}
			// 安装更新可能移除旧路径；缓存只省去扫描，不能跳过路径有效性检查。
			if info, err := os.Stat(cached.value.Path); err == nil && !info.IsDir() {
				rt.detectMutex.Unlock()
				return cached.value, nil
			}
			force = true
		}
	}
	if waiter, ok := rt.pendingDetect[key]; ok {
		rt.detectMutex.Unlock()
		waiter.wait()
		rt.detectMutex.Lock()
		value, valueErr := rt.detectResults[key], rt.detectErrors[key]
		rt.detectMutex.Unlock()
		return value, valueErr
	}
	waiter := &detectWaiter{done: make(chan struct{})}
	rt.pendingDetect[key] = waiter
	rt.detectMutex.Unlock()

	value, err := rt.resolveApplication(id, path)
	if err == nil {
		ttl := time.Duration(settings.InstallCacheMs) * time.Millisecond
		if value == nil {
			ttl = time.Duration(settings.MissingInstallCacheMs) * time.Millisecond
		}
		rt.detectMutex.Lock()
		rt.cache[key] = detectCacheEntry{value: value, expires: time.Now().Add(ttl)}
		rt.detectMutex.Unlock()
	}
	rt.detectMutex.Lock()
	rt.detectResults[key] = value
	rt.detectErrors[key] = err
	delete(rt.pendingDetect, key)
	rt.detectMutex.Unlock()
	waiter.finish()
	return value, err
}

type detectWaiter struct {
	done chan struct{}
	once sync.Once
}

func (w *detectWaiter) wait()   { <-w.done }
func (w *detectWaiter) finish() { w.once.Do(func() { close(w.done) }) }

func (rt *applicationRuntime) scriptFile(id string) string {
	return filepath.Join(rt.scriptsDir, id+"-wide", id+".ps1")
}

func (rt *applicationRuntime) resolveApplication(id string, path string) (*installation, error) {
	if id == settings.Droid {
		return rt.detectDroid(path)
	}
	name := settings.Applications[id].Name
	if isWindowsRuntime() {
		command := "& " + psQuote(rt.scriptFile(id)) + " -DetectOnly"
		if path != "" {
			command += " -ExecutablePath " + psQuote(path)
		}
		stdout, _, failure := psExec(command, 20*time.Second)
		if failure != nil {
			if path != "" {
				return nil, fmt.Errorf("无法识别指定路径，请选择 %s 的可执行文件。", name)
			}
			if failure.code == "ENOENT" {
				return nil, fmt.Errorf("系统 PowerShell 不可用。")
			}
			return nil, nil
		}
		var data struct {
			ApplicationName string `json:"ApplicationName"`
			Executable      string `json:"Executable"`
			Version         string `json:"Version"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &data); err != nil || data.Executable == "" {
			if path != "" {
				return nil, fmt.Errorf("无法识别指定路径，请选择 %s 的可执行文件。", name)
			}
			return nil, nil
		}
		appName := data.ApplicationName
		if appName == "" {
			appName = name
		}
		version := data.Version
		if version == "" {
			version = "未知版本"
		}
		return &installation{Name: appName, Path: data.Executable, Version: version}, nil
	}
	return nil, fmt.Errorf("当前操作系统暂不支持检测应用。")
}

// DetectDroid detects the Droid / Factory desktop app.
func (rt *applicationRuntime) detectDroid(customPath string) (*installation, error) {
	if isWindowsRuntime() {
		command := "& " + psQuote(filepath.Join(rt.scriptsDir, "droid-wide", "droid.ps1")) + " -DetectOnly"
		if customPath != "" {
			command += " -ExecutablePath " + psQuote(customPath)
		}
		stdout, _, failure := psExec(command, 20*time.Second)
		if failure != nil {
			if customPath != "" {
				return nil, fmt.Errorf("无法识别指定路径，请选择 Droid/Factory 桌面应用的可执行文件。")
			}
			if failure.code == "ENOENT" {
				return nil, fmt.Errorf("系统 PowerShell 不可用，无法检测 Droid。")
			}
			return nil, nil
		}
		var data struct {
			ApplicationName string `json:"ApplicationName"`
			Executable      string `json:"Executable"`
			Version         string `json:"Version"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &data); err != nil || data.Executable == "" {
			if customPath != "" {
				return nil, fmt.Errorf("无法识别指定路径，请选择 Droid/Factory 桌面应用的可执行文件。")
			}
			return nil, nil
		}
		return &installation{Name: data.ApplicationName, Path: data.Executable, Version: data.Version}, nil
	}
	return nil, fmt.Errorf("当前操作系统暂不支持检测应用。")
}

// IsApplicationRunning checks whether the app currently runs.
func (rt *applicationRuntime) IsApplicationRunning(id string, app *installation) (bool, error) {
	if isWindowsRuntime() {
		helper := filepath.Join(rt.scriptsDir, "application-processes.ps1")
		command := "$ErrorActionPreference='Stop'; . " + psQuote(helper) + "; Test-WideApplicationRunning " + psQuote(app.Path) + " " + psQuote(id) + " | ConvertTo-Json -Compress"
		stdout, _, failure := psExec(command, 10*time.Second)
		if failure != nil {
			return false, fmt.Errorf("无法确认 %s 的运行状态。", settings.Applications[id].Name)
		}
		var running bool
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &running); err != nil {
			return false, fmt.Errorf("无法确认 %s 的运行状态。", settings.Applications[id].Name)
		}
		return running, nil
	}
	return false, fmt.Errorf("当前操作系统暂不支持检测应用运行状态。")
}

// executeScript streams a launcher script and reports progress lines.
func (rt *applicationRuntime) executeScript(command string, name string, report Report, onStage func(string)) (int, error) {
	cmd := exec.Command(powershellPath(), psArgs(command)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return 0, err
	}
	cmd.Env = append(os.Environ(), "WIDE_PROGRESS_STREAM=1")
	if err := cmd.Start(); err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return 0, &psError{message: err.Error(), code: "ENOENT"}
		}
		return 0, err
	}
	var (
		mu         sync.Mutex
		selected   int
		failureMsg string
		settled    bool
		settleErr  error
		done       = make(chan struct{})
	)
	consume := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		if match := cdpPortPattern.FindStringSubmatch(line); match != nil {
			selected, _ = strconv.Atoi(match[1])
			if onStage != nil {
				onStage("waiting")
			}
		}
		if failPattern.MatchString(line) {
			idx := failPattern.FindStringIndex(line)
			failureMsg = line[idx[0]+len("失败："):]
			report(failureMsg, "error")
		}
	}
	outDecoder := newPSOutput(consume)
	errDecoder := newPSOutput(consume)
	var readerWG sync.WaitGroup
	readerWG.Add(2)
	go func() { defer readerWG.Done(); readLines(stdoutPipe, outDecoder) }()
	go func() { defer readerWG.Done(); readLines(stderrPipe, errDecoder) }()
	finish := func(code int, err error) {
		mu.Lock()
		defer mu.Unlock()
		if settled {
			return
		}
		settled = true
		if err != nil {
			settleErr = err
		} else if code != 0 {
			if failureMsg != "" {
				settleErr = fmt.Errorf("%s", failureMsg)
			} else {
				codeText := "未知"
				if code >= 0 {
					codeText = strconv.Itoa(code)
				}
				settleErr = fmt.Errorf("%s 操作失败（退出码 %s），请检查安装路径。", name, codeText)
			}
		}
		close(done)
	}
	go func() {
		waitErr := cmd.Wait()
		readerWG.Wait()
		outDecoder.drain(true)
		errDecoder.drain(true)
		code := 0
		if waitErr != nil {
			if exitError, ok := waitErr.(*exec.ExitError); ok {
				code = exitError.ExitCode()
			} else {
				code = -1
			}
		}
		finish(code, nil)
	}()
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		return selected, settleErr
	case <-timer.C:
		_ = cmd.Process.Kill()
		return 0, fmt.Errorf("%s 操作超过 120 秒，请检查应用后重试。", name)
	}
}

func (rt *applicationRuntime) connectPages(id string, port int, s settings.DroidSettings, report Report) error {
	var source string
	if id == settings.Droid {
		source = droidInjectionSource(s)
	} else {
		source = applicationInjection(id, s)
	}
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error = fmt.Errorf("应用没有提供可用的调试页面。")
	for {
		err := rt.connection(id).connect(port, source, LogFunc(report))
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}

func (rt *applicationRuntime) runWindows(id string, action string, s settings.DroidSettings, report Report, onStage func(string)) error {
	capability := settings.Applications[id]
	args := []string{"& " + psQuote(rt.scriptFile(id)), "-Width", psQuote(s.Width), "-FontWeight", strconv.Itoa(s.FontWeight), "-Port", strconv.Itoa(s.Port)}
	if capability.FontFamily {
		args = append(args, "-FontFamily", psQuote(s.FontFamily))
	}
	if capability.FontSize {
		args = append(args, "-FontSize", strconv.Itoa(s.FontSize))
	}
	if capability.MaxWidth {
		args = append(args, "-MaxWidth", psQuote(s.MaxWidth))
	}
	if capability.ChatHeight {
		args = append(args, "-ChatHeight", psQuote(s.ChatHeight))
	}
	if capability.Merge {
		args = append(args, "-HideLocalMerge", boolInt(s.HideLocalMerge))
	}
	if capability.Diff {
		args = append(args, "-HideGitDiff", boolInt(s.HideGitDiff))
	}
	if capability.Changes {
		args = append(args, "-HideChanges", boolInt(s.HideChanges))
	}
	if capability.Summary {
		args = append(args, "-PreventSummary", boolInt(s.PreventSummary))
	}
	if s.ExecutablePath != "" {
		args = append(args, "-ExecutablePath", psQuote(s.ExecutablePath))
	}
	if action == "normal" {
		args = append(args, "-Normal")
	} else if id == settings.Zcode || id == settings.Qoder || id == settings.Paseo {
		args = append(args, "-LaunchOnly")
	}
	port, err := rt.executeScript(strings.Join(args, " "), capability.Name, report, onStage)
	if err != nil {
		return err
	}
	if action == "apply" {
		if port == 0 {
			return fmt.Errorf("%s 实际调试端口未返回，请重新启动。", capability.Name)
		}
		if onStage != nil {
			onStage("applying")
		}
		return rt.connectPages(id, port, s, report)
	}
	return nil
}

func boolInt(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func selectPort(preferred int, message string) (int, error) {
	for port := preferred; port <= min(preferred+50, 65535); port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("%s", message)
}

// RunApplication launches an app with UI injection applied.
func (rt *applicationRuntime) RunApplication(id string, action string, s settings.DroidSettings, report Report, startupMode string, onStage func(string)) error {
	if id == settings.Droid && startupMode == "default" {
		return rt.droid.Run(action, s, LogFunc(report), onStage)
	}
	app, err := rt.DetectApplication(id, s.ExecutablePath, false)
	if err != nil {
		return err
	}
	if app == nil {
		return fmt.Errorf("没有找到 %s，请在高级设置中选择安装路径。", settings.Applications[id].Name)
	}
	if id != settings.Droid {
		rt.disposeConnection(id)
	}
	switch {
	case id == settings.Droid:
		err = rt.droid.Run(action, s, LogFunc(report), onStage)
	case isWindowsRuntime():
		err = rt.runWindows(id, action, s, report, onStage)
	default:
		err = fmt.Errorf("当前操作系统暂不支持启动。")
	}
	if err != nil {
		return err
	}
	if startupMode == "maximized" && isWindowsRuntime() {
		if onStage != nil {
			onStage("maximizing")
		}
		helper := filepath.Join(rt.scriptsDir, "maximize-window.ps1")
		command := "& " + psQuote(helper) + " -ExecutablePath " + psQuote(app.Path) + " -ApplicationId " + psQuote(id)
		if _, _, failure := psExec(command, 55*time.Second); failure != nil {
			return fmt.Errorf("%s 已启动，但未能最大化主窗口，请确认主窗口已打开。", settings.Applications[id].Name)
		}
	}
	return nil
}

// QuitApplication fully exits an app, protecting this process's tree.
func (rt *applicationRuntime) QuitApplication(id string, path string, detected *installation) error {
	app := detected
	var err error
	if app == nil {
		app, err = rt.DetectApplication(id, path, true)
		if err != nil {
			return err
		}
	}
	if app == nil {
		return fmt.Errorf("没有找到 %s，请先选择正确的安装路径。", settings.Applications[id].Name)
	}
	if id == settings.Droid {
		rt.droid.dispose()
	} else {
		rt.disposeConnection(id)
	}
	if !isWindowsRuntime() {
		return fmt.Errorf("当前操作系统暂不支持退出应用。")
	}
	helper := filepath.Join(rt.scriptsDir, "quit-application.ps1")
	timeout := 45 * time.Second
	command := fmt.Sprintf("try { & %s -ExecutablePath %s -ApplicationId %s -ProtectedProcessId %d } catch { [Console]::Error.WriteLine('WIDE_QUIT_ERROR:' + (ConvertTo-Json -InputObject $_.Exception.Message -Compress)); exit 1 }",
		psQuote(helper), psQuote(app.Path), psQuote(id), os.Getpid())
	_, stderr, failure := psExec(command, timeout)
	name := settings.Applications[id].Name
	if failure != nil {
		if failure.killed {
			return fmt.Errorf("退出 %s 超过 %d 秒，请检查应用是否仍在运行后重试。", name, int(timeout.Seconds()))
		}
		if failure.code == "ENOENT" {
			return fmt.Errorf("系统 PowerShell 不可用。")
		}
		detail := ""
		if match := regexp.MustCompile(`(?m)(?:^|\r?\n)WIDE_QUIT_ERROR:([^\r\n]+)`).FindStringSubmatch(failure.stderr); match != nil {
			var message string
			if json.Unmarshal([]byte(match[1]), &message) == nil {
				detail = message
			}
		}
		if detail == "" {
			detail = failure.message
		}
		return fmt.Errorf("未能完整退出 %s：%s", name, detail)
	}
	_ = stderr
	return nil
}

// QuitUnixPath is unused on Windows; kept for parity when running on Unix.
func (rt *applicationRuntime) QuitUnixPath(path string) { _ = path }

var _ = filepath.Join
