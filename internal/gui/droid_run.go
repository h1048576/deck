package gui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"wide-pure/internal/settings"
)

// droidConnection is Droid's single persistent page connection.
type droidRuntime struct {
	rt         *applicationRuntime
	mu         sync.Mutex
	connection *pageConnection
}

func newDroidRuntime(rt *applicationRuntime) *droidRuntime {
	return &droidRuntime{rt: rt, connection: newPageConnection("Droid", "droid-wide-ui-override", nil)}
}

func (d *droidRuntime) dispose() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connection.dispose()
}

func (d *droidRuntime) connectPages(port int, s settings.DroidSettings, log LogFunc) error {
	return d.connection.connect(port, droidInjectionSource(s), log)
}

func (d *droidRuntime) runWindows(action string, s settings.DroidSettings, log LogFunc, onStage func(string)) (int, error) {
	args := []string{"& " + psQuote(filepath.Join(d.rt.scriptsDir, "droid-wide", "droid.ps1")),
		"-Width", psQuote(s.Width), "-MaxWidth", psQuote(s.MaxWidth), "-ChatHeight", psQuote(s.ChatHeight),
		"-FontFamily", psQuote(s.FontFamily), "-FontSize", strconv.Itoa(s.FontSize), "-FontWeight", strconv.Itoa(s.FontWeight),
		"-Port", strconv.Itoa(s.Port), "-HideLocalMerge", boolInt(s.HideLocalMerge), "-HideGitDiff", boolInt(s.HideGitDiff)}
	if s.ExecutablePath != "" {
		args = append(args, "-ExecutablePath", psQuote(s.ExecutablePath))
	}
	if action == "normal" {
		args = append(args, "-Normal")
	}
	cmd := exec.Command(powershellPath(), psArgs(strings.Join(args, " "))...)
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
		return 0, err
	}
	var (
		mu        sync.Mutex
		selected  int
		settled   bool
		settleErr error
		done      = make(chan struct{})
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
		if strings.Contains(line, "正在最大化主窗口") && onStage != nil {
			onStage("maximizing")
		}
		if strings.TrimSpace(line) != "" {
			plain := line
			level := "info"
			if strings.Contains(line, "失败") || strings.Contains(line, "错误") {
				level = "error"
			}
			log(strings.TrimPrefix(plain, "[Droid Wide]"), level)
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
			settleErr = &psError{message: "Droid 操作失败（退出码 " + exitCodeText(code) + "），请检查应用路径后重试。"}
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
		return 0, &psError{message: "操作超过 120 秒，请检查 Droid 是否正常启动。"}
	}
}

func exitCodeText(code int) string {
	if code < 0 {
		return "未知"
	}
	return strconv.Itoa(code)
}

func (d *droidRuntime) waitForPages(port int, s settings.DroidSettings, log LogFunc) error {
	deadline := time.Now().Add(60 * time.Second)
	var failure error = &psError{message: "Droid 未提供可用的调试页面。"}
	for {
		err := d.connectPages(port, s, log)
		if err == nil {
			return nil
		}
		failure = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return failure
}

// RunDroid launches or applies the Droid desktop app.
func (d *droidRuntime) Run(action string, s settings.DroidSettings, log LogFunc, onStage func(string)) error {
	// 先确认安装路径，避免路径无效时中断已有的界面守护连接。
	app, err := d.rt.detectDroid(s.ExecutablePath)
	if err != nil {
		return err
	}
	if app == nil {
		return &psError{message: "没有找到 Droid/Factory 桌面应用，请在高级设置中选择安装路径。"}
	}
	d.dispose()
	if !isWindowsRuntime() {
		return &psError{message: "当前操作系统暂不支持 Droid 启动。"}
	}
	port, err := d.runWindows(action, s, log, onStage)
	if err != nil {
		return err
	}
	if action == "apply" {
		if port == 0 {
			return &psError{message: "无法确认 Droid 的实际调试端口，请重新检测应用后重试。"}
		}
		if onStage != nil {
			onStage("applying")
		}
		if err := d.waitForPages(port, s, log); err != nil {
			return err
		}
		log("页面守护已连接，刷新或打开新窗口时会自动应用设置。", "info")
	}
	return nil
}
