package gui

import "deck/internal/settings"

// ApplicationManager 向命令行提供与应用菜单相同的操作，不创建桌面窗口。
type ApplicationManager struct {
	rt    *applicationRuntime
	batch *BatchRunner
}

// ApplicationStatus 是应用安装和运行状态的快照。
type ApplicationStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewApplicationManager 使用已经释放到磁盘的辅助脚本。
func NewApplicationManager(scriptsDir string) *ApplicationManager {
	rt := newApplicationRuntime(scriptsDir)
	return &ApplicationManager{rt: rt, batch: newBatchRunner(rt, rt.droid)}
}

// Close 释放调试连接，不退出受管应用。
func (m *ApplicationManager) Close() {
	m.rt.disposeConnections()
}

// Run 执行应用菜单操作。ids 为 nil 时处理全部应用。
func (m *ApplicationManager) Run(action string, prefs settings.Preferences, ids []string, progress func(BatchProgress)) BatchResult {
	if action != "start" && action != "restart" && action != "exit" {
		return BatchResult{Action: action, Message: "不支持的应用操作：" + action}
	}
	if ids != nil && len(ids) == 0 {
		return BatchResult{Action: action, Message: "请指定至少一个应用"}
	}
	for _, id := range ids {
		if _, err := settings.ParseFeatureId(id); err != nil {
			return BatchResult{Action: action, Message: "未知应用：" + id}
		}
	}
	return m.batch.RunAll(action, prefs, progress, ids)
}

// Status 实时检测安装路径和进程状态，不启动或退出应用。
func (m *ApplicationManager) Status(id string, appSettings settings.DroidSettings) ApplicationStatus {
	result := ApplicationStatus{ID: id, Name: settings.Applications[id].Name}
	if _, err := settings.ParseFeatureId(id); err != nil {
		result.Error = "未知应用：" + id
		return result
	}
	app, err := m.rt.DetectApplication(id, appSettings.ExecutablePath, true)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if app == nil {
		return result
	}
	result.Installed = true
	result.Path = app.Path
	result.Version = app.Version
	result.Running, err = m.rt.IsApplicationRunning(id, app)
	if err != nil {
		result.Error = err.Error()
	}
	return result
}
