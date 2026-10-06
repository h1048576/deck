package gui

import (
	"fmt"
	"strings"
	"sync"

	"deck/internal/settings"
)

// BatchApplicationResult is one app's outcome in a batch run.
type BatchApplicationResult struct {
	ID         string `json:"id"`
	Status     string `json:"status"` // success | skipped | error
	Message    string `json:"message"`
	SkipReason string `json:"skipReason,omitempty"` // not-installed | already-running
}

// BatchProgress is published while a batch operation runs.
type BatchProgress struct {
	Action         string                   `json:"action"`
	ApplicationIDs []string                 `json:"applicationIds"`
	Stages         map[string]string        `json:"stages"`
	CurrentIDs     []string                 `json:"currentIds"`
	Completed      int                      `json:"completed"`
	Total          int                      `json:"total"`
	Results        []BatchApplicationResult `json:"results"`
}

// BatchResult is the final outcome of runAll.
type BatchResult struct {
	Success bool                     `json:"success"`
	Message string                   `json:"message"`
	Action  string                   `json:"action"`
	Results []BatchApplicationResult `json:"results"`
}

// BatchRunner executes start/restart/exit across applications.
type BatchRunner struct {
	rt        *applicationRuntime
	droid     *droidRuntime
	mu        sync.Mutex
	revision  int
	progress  BatchProgress
	hasActive bool
}

func newBatchRunner(rt *applicationRuntime, droid *droidRuntime) *BatchRunner {
	return &BatchRunner{rt: rt, droid: droid}
}

// Progress snapshots the current batch state for polling clients.
func (b *BatchRunner) Progress() (BatchProgress, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.progress, b.hasActive
}

// RunAll performs a batch operation following the saved menu order.
func (b *BatchRunner) RunAll(action string, prefs settings.Preferences, onProgress func(BatchProgress), ids []string) BatchResult {
	b.mu.Lock()
	b.revision++
	revision := b.revision
	b.mu.Unlock()
	order := settings.NormalizeMenuOrder(prefs.MenuOrder)
	if ids != nil {
		filtered := make([]string, 0, len(order))
		for _, id := range order {
			for _, wanted := range ids {
				if id == wanted {
					filtered = append(filtered, id)
					break
				}
			}
		}
		order = filtered
	}
	stages := map[string]string{}
	for _, id := range order {
		stages[id] = "queued"
	}
	var (
		stateMu     sync.Mutex
		resultsByID = map[string]BatchApplicationResult{}
		activeIDs   = map[string]bool{}
	)
	orderedResults := func() []BatchApplicationResult {
		stateMu.Lock()
		defer stateMu.Unlock()
		out := make([]BatchApplicationResult, 0, len(order))
		for _, id := range order {
			if result, ok := resultsByID[id]; ok {
				out = append(out, result)
			}
		}
		return out
	}
	publish := func() {
		stateMu.Lock()
		current := make([]string, 0, len(activeIDs))
		for _, id := range order {
			if activeIDs[id] {
				current = append(current, id)
			}
		}
		stagesCopy := map[string]string{}
		for key, value := range stages {
			stagesCopy[key] = value
		}
		completed := len(resultsByID)
		stateMu.Unlock()
		progress := BatchProgress{
			Action:         action,
			ApplicationIDs: append([]string(nil), order...),
			Stages:         stagesCopy,
			CurrentIDs:     current,
			Completed:      completed,
			Total:          len(order),
			Results:        orderedResults(),
		}
		b.mu.Lock()
		b.progress = progress
		b.hasActive = true
		b.mu.Unlock()
		if onProgress != nil {
			onProgress(progress)
		}
	}
	publish()
	completedText := map[string]string{"start": "已启动", "restart": "已重启", "exit": "已退出"}[action]

	// 按菜单顺序分配任务；不同应用并发，同一应用的退出、启动仍顺序执行。
	var processApplication = func(id string) {
		stateMu.Lock()
		activeIDs[id] = true
		stateMu.Unlock()
		stage := func(value string) {
			stateMu.Lock()
			if stages[id] != value {
				stages[id] = value
				stateMu.Unlock()
				publish()
				return
			}
			stateMu.Unlock()
		}
		stage("detecting")
		appSettings := prefs.Applications[id]
		var (
			reportMu         sync.Mutex
			failureDetail    = ""
			operationRunning = true
		)
		report := func(message string, level string) {
			if level != "error" {
				return
			}
			reportMu.Lock()
			defer reportMu.Unlock()
			if operationRunning {
				failureDetail = strings.TrimPrefix(message, "失败：")
			} else if revision == b.currentRevision() {
				// 后台连接的错误也更新页面内结果，批量操作不发送弹出通知。
				stateMu.Lock()
				if result, ok := resultsByID[id]; ok {
					result.Status = "error"
					result.Message = message
					resultsByID[id] = result
				}
				stateMu.Unlock()
				publish()
			}
		}
		func() {
			defer func() {
				reportMu.Lock()
				operationRunning = false
				reportMu.Unlock()
				stateMu.Lock()
				activeIDs[id] = false
				delete(activeIDs, id)
				delete(stages, id)
				stateMu.Unlock()
				publish()
			}()
			// 启动复用短期安装缓存；运行状态仍实时查询，重启和退出仍重新识别安装。
			app, err := b.rt.DetectApplication(id, appSettings.ExecutablePath, action != "start")
			if err != nil {
				stateMu.Lock()
				resultsByID[id] = BatchApplicationResult{ID: id, Status: "error", Message: err.Error()}
				stateMu.Unlock()
				return
			}
			if app == nil {
				stateMu.Lock()
				resultsByID[id] = BatchApplicationResult{ID: id, Status: "skipped", SkipReason: "not-installed", Message: "未安装，已跳过"}
				stateMu.Unlock()
				return
			}
			if action == "start" {
				stage("checking")
				running, runningErr := b.rt.IsApplicationRunning(id, app)
				if runningErr != nil {
					stateMu.Lock()
					resultsByID[id] = BatchApplicationResult{ID: id, Status: "error", Message: runningErr.Error()}
					stateMu.Unlock()
					return
				}
				if running {
					stateMu.Lock()
					resultsByID[id] = BatchApplicationResult{ID: id, Status: "skipped", SkipReason: "already-running", Message: "已运行，已跳过"}
					stateMu.Unlock()
					return
				}
			}
			if action == "restart" || action == "exit" {
				stage("stopping")
				if err := b.rt.QuitApplication(id, appSettings.ExecutablePath, app); err != nil {
					stateMu.Lock()
					resultsByID[id] = BatchApplicationResult{ID: id, Status: "error", Message: err.Error()}
					stateMu.Unlock()
					return
				}
			}
			if action != "exit" {
				stage("starting")
				if err := b.rt.RunApplication(id, "apply", appSettings, report, prefs.StartupMode, stage); err != nil {
					reportMu.Lock()
					detail := failureDetail
					reportMu.Unlock()
					if detail == "" {
						detail = err.Error()
					}
					stateMu.Lock()
					resultsByID[id] = BatchApplicationResult{ID: id, Status: "error", Message: detail}
					stateMu.Unlock()
					return
				}
			}
			stateMu.Lock()
			resultsByID[id] = BatchApplicationResult{ID: id, Status: "success", Message: completedText}
			stateMu.Unlock()
		}()
	}
	queue := make(chan string, len(order))
	for _, id := range order {
		queue <- id
	}
	close(queue)
	workers := 5
	if len(order) < workers {
		workers = len(order)
	}
	var workerWG sync.WaitGroup
	workerWG.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer workerWG.Done()
			for id := range queue {
				processApplication(id)
			}
		}()
	}
	workerWG.Wait()
	publish()
	b.mu.Lock()
	b.hasActive = false
	b.mu.Unlock()

	results := orderedResults()
	succeeded, alreadyRunning, notInstalled, failed := 0, 0, 0, 0
	for _, result := range results {
		switch {
		case result.Status == "success":
			succeeded++
		case result.SkipReason == "already-running":
			alreadyRunning++
		case result.SkipReason == "not-installed":
			notInstalled++
		case result.Status == "error":
			failed++
		}
	}
	details := []string{}
	if succeeded > 0 {
		details = append(details, fmt.Sprintf("%s %d 个应用", completedText, succeeded))
	}
	if alreadyRunning > 0 {
		details = append(details, fmt.Sprintf("%d 个已运行，已跳过", alreadyRunning))
	}
	if notInstalled > 0 {
		details = append(details, fmt.Sprintf("%d 个未安装，已跳过", notInstalled))
	}
	if failed > 0 {
		details = append(details, fmt.Sprintf("%d 个失败", failed))
	}
	return BatchResult{
		Success: failed == 0 && (action == "exit" || succeeded > 0 || alreadyRunning > 0),
		Message: strings.Join(details, "；") + "。",
		Action:  action,
		Results: results,
	}
}

func (b *BatchRunner) currentRevision() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revision
}
