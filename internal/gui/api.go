package gui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"deck/internal/harnesscfg"
	"deck/internal/settings"
)

//go:embed all:assets
var assets embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}

// JobResult mirrors the original JobResult shape.
type JobResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// BootstrapResponse is the initial state payload.
type BootstrapResponse struct {
	Preferences      settings.Preferences `json:"preferences"`
	Platform         string               `json:"platform"`
	Version          string               `json:"version"`
	WindowMaximized  bool                 `json:"windowMaximized"`
	OpenAtLogin      bool                 `json:"openAtLogin"`
	StartupAvailable bool                 `json:"startupAvailable"`
	ConfigWarning    string               `json:"configWarning,omitempty"`
	Desktop          bool                 `json:"desktop"`
}

// host carries every service the JSON API needs; the same handler serves the
// desktop window and the plain-browser serve mode.
type host struct {
	mu       sync.Mutex
	busy     bool
	isWeb    bool
	version  string
	store    *settings.Store
	rt       *applicationRuntime
	batch    *BatchRunner
	harness  *harnesscfg.HarnessManager
	models   *harnesscfg.ModelsManager
	mcps     *harnesscfg.McpsManager
	startup  *StartupManager
	windows  Windows
	notices  []JobResult
	noticeMu sync.Mutex
}

func newHost(version string, isWeb bool) *host {
	h := &host{isWeb: isWeb, version: version}
	return h
}

// init wires services after the config root is known.
func (h *host) init(home string) {
	h.store = settings.NewStore(configDir())
	h.store.Load()
	h.rt = newApplicationRuntime(scriptsDir())
	h.batch = newBatchRunner(h.rt, h.rt.droid)
	h.harness = harnesscfg.NewHarnessManager(home, moveToTrash)
	h.models = harnesscfg.NewModelsManager(home, modelBackupsDir())
	h.mcps = harnesscfg.NewMcpsManager(home, mcpBackupsDir(), map[string]string{})
	h.startup = NewStartupManager()
}

func (h *host) configRoot() string { return configDir() }

// setBusy acquires the operation mutex; report mirrors the original
// report(message, level): errors during an operation become the failure
// detail, later errors become toast notices.
func (h *host) beginOperation(message string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy {
		return fmt.Errorf("%s", message)
	}
	h.busy = true
	return nil
}

func (h *host) endOperation() {
	h.mu.Lock()
	h.busy = false
	h.mu.Unlock()
}

func (h *host) isBusy() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.busy
}

func (h *host) pushNotice(result JobResult) {
	h.noticeMu.Lock()
	h.notices = append(h.notices, result)
	h.noticeMu.Unlock()
}

func writeJSON(rw http.ResponseWriter, status int, value any) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(status)
	encoder := json.NewEncoder(rw)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(rw http.ResponseWriter, err error) {
	writeJSON(rw, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func writeResult(rw http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(rw, err)
		return
	}
	writeJSON(rw, http.StatusOK, value)
}

func decodeBody(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		return fmt.Errorf("读取参数无效")
	}
	if len(body) == 0 {
		return fmt.Errorf("读取参数无效")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("读取参数无效")
	}
	return nil
}

// Handler builds the complete HTTP surface: static assets plus the JSON API.
func (h *host) Handler() http.Handler {
	return h.buildRouter()
}

// buildRouter wires every endpoint; kept separate for clarity.
func (h *host) buildRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS())))

	mux.HandleFunc("GET /api/bootstrap", func(rw http.ResponseWriter, r *http.Request) {
		prefs := h.store.Prefs()
		writeJSON(rw, http.StatusOK, BootstrapResponse{
			Preferences:      prefs,
			Platform:         platformName(),
			Version:          h.version,
			WindowMaximized:  h.windows.IsMaximized(),
			OpenAtLogin:      h.startup.Enabled(),
			StartupAvailable: h.startup.Available(),
			ConfigWarning:    h.store.Warning(),
			Desktop:          !h.isWeb,
		})
	})

	mux.HandleFunc("POST /api/open-at-login", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled any `json:"enabled"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		previous := h.startup.Enabled()
		enabled, err := h.startup.Set(body.Enabled)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.OpenAtLogin = &enabled
			return current
		}); err != nil {
			if _, rollbackErr := h.startup.Set(previous); rollbackErr != nil {
				h.pushNotice(JobResult{Success: false, Message: rollbackErr.Error()})
			}
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, enabled)
	})

	mux.HandleFunc("GET /api/harness/inventory", func(rw http.ResponseWriter, r *http.Request) {
		includeSkills := r.URL.Query().Get("skills") != "0"
		writeJSON(rw, http.StatusOK, h.harness.Inventory(includeSkills))
	})
	mux.HandleFunc("GET /api/harness/skills", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, http.StatusOK, h.harness.SkillsInventory())
	})
	mux.HandleFunc("GET /api/harness/agents-preview", func(rw http.ResponseWriter, r *http.Request) {
		if value, err := h.harness.PreviewAgents(); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/harness/sync-agents", func(rw http.ResponseWriter, r *http.Request) {
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		result, err := h.harness.SyncAgents()
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, result, err)
	})
	mux.HandleFunc("POST /api/harness/sync-skills", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Source  any `json:"source"`
			SkillID any `json:"skillId"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		result, err := h.harness.SyncSkills(body.Source, body.SkillID)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, result, err)
	})
	mux.HandleFunc("POST /api/harness/delete-skills", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			ID      any `json:"id"`
			SkillID any `json:"skillId"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		result, err := h.harness.DeleteSkills(body.ID, body.SkillID)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, result, err)
	})

	mux.HandleFunc("GET /api/models/inventory", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, http.StatusOK, h.models.Inventory())
	})
	mux.HandleFunc("POST /api/models/detail", func(rw http.ResponseWriter, r *http.Request) {
		var target harnesscfg.ModelTarget
		if err := decodeBody(r, &target); err != nil {
			writeError(rw, err)
			return
		}
		if value, err := h.models.Detail(target); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/models/preview", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			SourceID any `json:"sourceId"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if value, err := h.models.Preview(body.SourceID); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/models/save", func(rw http.ResponseWriter, r *http.Request) {
		var change harnesscfg.ModelChange
		if err := decodeBody(r, &change); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		err := h.models.Save(change)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, map[string]bool{"ok": err == nil}, err)
	})
	mux.HandleFunc("POST /api/models/delete", func(rw http.ResponseWriter, r *http.Request) {
		var target harnesscfg.ModelTarget
		if err := decodeBody(r, &target); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		err := h.models.Delete(target)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, map[string]bool{"ok": err == nil}, err)
	})
	mux.HandleFunc("POST /api/models/reorder", func(rw http.ResponseWriter, r *http.Request) {
		var order harnesscfg.ModelOrder
		if err := decodeBody(r, &order); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		err := h.models.Reorder(order)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, map[string]bool{"ok": err == nil}, err)
	})
	mux.HandleFunc("POST /api/models/batch", func(rw http.ResponseWriter, r *http.Request) {
		var change harnesscfg.ModelBatchChange
		if err := decodeBody(r, &change); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		result, err := h.models.Batch(change)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, result, err)
	})

	mux.HandleFunc("GET /api/mcps/inventory", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, http.StatusOK, h.mcps.Inventory())
	})
	mux.HandleFunc("POST /api/mcps/refresh", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Harness any `json:"harness"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if value, err := h.mcps.Source(body.Harness); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/mcps/detail", func(rw http.ResponseWriter, r *http.Request) {
		var target harnesscfg.McpTarget
		if err := decodeBody(r, &target); err != nil {
			writeError(rw, err)
			return
		}
		if value, err := h.mcps.Detail(target); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/mcps/preview", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Harness any `json:"harness"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if value, err := h.mcps.Preview(body.Harness); err != nil {
			writeError(rw, err)
		} else {
			writeJSON(rw, http.StatusOK, value)
		}
	})
	mux.HandleFunc("POST /api/mcps/save", func(rw http.ResponseWriter, r *http.Request) {
		var change harnesscfg.McpChange
		if err := decodeBody(r, &change); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		err := h.mcps.Save(change)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, map[string]bool{"ok": err == nil}, err)
	})
	mux.HandleFunc("POST /api/mcps/delete", func(rw http.ResponseWriter, r *http.Request) {
		var target harnesscfg.McpTarget
		if err := decodeBody(r, &target); err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		err := h.mcps.Delete(target)
		h.endOperation()
		h.flushAfterOperation()
		writeResult(rw, map[string]bool{"ok": err == nil}, err)
	})

	mux.HandleFunc("POST /api/save", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID any            `json:"featureId"`
			Settings  map[string]any `json:"settings"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		id, err := settings.ParseFeatureId(body.FeatureID)
		if err != nil {
			writeError(rw, err)
			return
		}
		parsed, err := settings.ParseSettings(body.Settings, id)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.Applications[id] = parsed
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, parsed)
	})
	mux.HandleFunc("POST /api/theme", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Theme string `json:"theme"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if body.Theme != "system" && body.Theme != "light" && body.Theme != "dark" {
			writeError(rw, fmt.Errorf("主题无效"))
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.Theme = body.Theme
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		h.windows.ApplyTheme(body.Theme)
		writeJSON(rw, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/appearance", func(rw http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		appearance, err := settings.ParseAppearance(body)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.Appearance = appearance
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, appearance)
	})
	mux.HandleFunc("POST /api/harness-settings", func(rw http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		harness, err := settings.ParseHarnessSettings(body)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.Harness = harness
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, harness)
	})
	mux.HandleFunc("POST /api/startup-mode", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if body.Mode != "default" && body.Mode != "maximized" {
			writeError(rw, fmt.Errorf("启动方式无效"))
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.StartupMode = body.Mode
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/menu-order", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Order any `json:"order"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		order, err := settings.ParseMenuOrder(body.Order)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.MenuOrder = order
			return current
		}); err != nil {
			writeError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, order)
	})

	mux.HandleFunc("POST /api/detect", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID any    `json:"featureId"`
			Path      string `json:"path"`
			Force     any    `json:"force"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		id, err := settings.ParseFeatureId(body.FeatureID)
		if err != nil {
			writeError(rw, err)
			return
		}
		if len(body.Path) > 4096 || strings.ContainsAny(body.Path, "\x00\r\n") {
			writeError(rw, fmt.Errorf("应用路径无效"))
			return
		}
		force, ok := body.Force.(bool)
		if !ok {
			writeError(rw, fmt.Errorf("检测参数无效"))
			return
		}
		installation, err := h.rt.DetectApplication(id, body.Path, force)
		writeResult(rw, installation, err)
	})
	mux.HandleFunc("POST /api/choose", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID any `json:"featureId"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		id, err := settings.ParseFeatureId(body.FeatureID)
		if err != nil {
			writeError(rw, err)
			return
		}
		title := fmt.Sprintf("选择 %s 桌面应用", settings.Applications[id].Name)
		if id == settings.Droid {
			title = "选择 Droid / Factory 桌面应用"
		}
		path, err := h.windows.ChooseExecutable(title)
		writeResult(rw, path, err)
	})
	mux.HandleFunc("POST /api/run", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID any            `json:"featureId"`
			Action    string         `json:"action"`
			Settings  map[string]any `json:"settings"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		id, err := settings.ParseFeatureId(body.FeatureID)
		if err != nil {
			writeError(rw, err)
			return
		}
		name := settings.Applications[id].Name
		if body.Action != "apply" && body.Action != "normal" {
			writeError(rw, fmt.Errorf("操作无效"))
			return
		}
		parsed, err := settings.ParseSettings(body.Settings, id)
		if err != nil {
			writeError(rw, err)
			return
		}
		if err := h.beginOperation("应用操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		var (
			reportMu         sync.Mutex
			failureDetail    string
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
			} else {
				h.pushNotice(JobResult{Success: false, Message: message})
			}
		}
		if err := <-h.store.Update(func(current settings.Preferences) settings.Preferences {
			current.Applications[id] = parsed
			return current
		}); err != nil {
			h.endOperation()
			writeError(rw, err)
			return
		}
		runErr := h.rt.RunApplication(id, body.Action, parsed, report, h.store.Prefs().StartupMode, nil)
		reportMu.Lock()
		operationRunning = false
		detail := failureDetail
		reportMu.Unlock()
		h.endOperation()
		h.flushAfterOperation()
		if runErr != nil {
			message := detail
			if message == "" {
				message = runErr.Error()
			}
			writeJSON(rw, http.StatusOK, JobResult{Success: false, Message: message})
			return
		}
		if body.Action == "normal" {
			writeJSON(rw, http.StatusOK, JobResult{Success: true, Message: fmt.Sprintf("%s 已以默认界面重新启动。", name)})
			return
		}
		writeJSON(rw, http.StatusOK, JobResult{Success: true, Message: fmt.Sprintf("%s 界面设置已生效。", name)})
	})
	mux.HandleFunc("POST /api/window", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		switch body.Action {
		case "minimize":
			h.windows.Minimize()
		case "maximize":
			h.windows.ToggleMaximize()
		case "close":
			h.windows.RequestClose()
		default:
			writeError(rw, fmt.Errorf("操作无效"))
			return
		}
		writeJSON(rw, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/window/state", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, http.StatusOK, map[string]bool{"maximized": h.windows.IsMaximized()})
	})
	mux.HandleFunc("POST /api/quit", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID any    `json:"featureId"`
			Path      string `json:"path"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		id, err := settings.ParseFeatureId(body.FeatureID)
		if err != nil {
			writeError(rw, err)
			return
		}
		if len(body.Path) > 4096 || strings.ContainsAny(body.Path, "\x00\r\n") {
			writeError(rw, fmt.Errorf("应用路径无效"))
			return
		}
		if err := h.beginOperation("应用操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		quitErr := h.rt.QuitApplication(id, body.Path, nil)
		h.endOperation()
		h.flushAfterOperation()
		if quitErr != nil {
			writeJSON(rw, http.StatusOK, JobResult{Success: false, Message: quitErr.Error()})
			return
		}
		writeJSON(rw, http.StatusOK, JobResult{Success: true, Message: fmt.Sprintf("%s 已完全退出。", settings.Applications[id].Name)})
	})
	mux.HandleFunc("POST /api/run-all", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string   `json:"action"`
			IDs    []string `json:"ids"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(rw, err)
			return
		}
		if body.Action != "start" && body.Action != "restart" && body.Action != "exit" {
			writeError(rw, fmt.Errorf("全局操作无效"))
			return
		}
		var ids []string
		if body.IDs != nil {
			if len(body.IDs) == 0 {
				writeError(rw, fmt.Errorf("请选择要操作的应用"))
				return
			}
			for _, id := range body.IDs {
				parsed, err := settings.ParseFeatureId(id)
				if err != nil {
					writeError(rw, err)
					return
				}
				ids = append(ids, parsed)
			}
		}
		if err := h.beginOperation("应用操作正在执行，请稍后再试。"); err != nil {
			writeError(rw, err)
			return
		}
		// 退出仍使用最后成功保存的设置，避免字体等设置保存失败时无法退出应用。
		if err := h.store.Flush(); err != nil && body.Action != "exit" {
			h.endOperation()
			writeError(rw, err)
			return
		}
		result := h.batch.RunAll(body.Action, h.store.Prefs(), nil, ids)
		h.endOperation()
		h.flushAfterOperation()
		writeJSON(rw, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/batch/progress", func(rw http.ResponseWriter, r *http.Request) {
		progress, active := h.batch.Progress()
		writeJSON(rw, http.StatusOK, map[string]any{"progress": progress, "active": active})
	})
	mux.HandleFunc("GET /api/notices", func(rw http.ResponseWriter, r *http.Request) {
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		h.noticeMu.Lock()
		defer h.noticeMu.Unlock()
		if since > len(h.notices) {
			since = len(h.notices)
		}
		writeJSON(rw, http.StatusOK, map[string]any{"notices": h.notices[since:], "cursor": len(h.notices)})
	})
	return mux
}

// flushAfterOperation mirrors "if (quitting) app.quit()" after operations.
func (h *host) flushAfterOperation() {
	h.windows.AfterOperation()
}

// harnesscfgValue adapts (value, error) returns to writeResult's signature.
func harnesscfgValue[T any](value T, err error) (T, error) { return value, err }
