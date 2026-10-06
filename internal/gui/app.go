package gui

import (
	"errors"
	"fmt"
	"log"
	"os"

	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"deck/internal/appdir"
	"deck/internal/settings"
)

// Windows is the window capability surface the JSON API may touch; the
// desktop host talks to Wails, the serve mode implements no-ops.
type Windows interface {
	Minimize()
	ToggleMaximize()
	RequestClose()
	IsMaximized() bool
	ChooseExecutable(title string) (string, error)
	MessageBox(kind, title, message string)
	ApplyTheme(theme string)
	AfterOperation()
}

// ConfigRoot returns the application configuration directory.
func configDir() string { return appdir.Config() }

func scriptsDir() string { return appdir.ScriptsDir() }

func modelBackupsDir() string { return appdir.ModelBackups() }

func mcpBackupsDir() string { return appdir.McpBackups() }

func platformName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	default:
		return "linux"
	}
}

// Run starts the desktop application.
func Run(version string) error {
	if err := appdir.MigrateLegacyConfig(); err != nil {
		return err
	}
	if err := ExtractScripts(scriptsDir()); err != nil {
		log.Printf("%s: %+v", appdir.Name, err)
	}
	h := newHost(version, false)
	h.init(homeDir())

	var app *application.App
	var window *application.WebviewWindow

	desktopHost := &desktopWindows{}
	h.windows = desktopHost

	app = application.New(application.Options{
		Name:        appdir.Name,
		Description: "launch and re-skin AI coding desktop apps",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: h.Handler(),
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: appdir.WebViewData(),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "app." + appdir.Name + ".desktop",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				application.InvokeAsync(func() {
					if window != nil {
						window.Show()
						window.Focus()
					}
				})
			},
		},
		OnShutdown: func() {
			h.rt.disposeConnections()
		},
		ErrorHandler: func(err error) {
			log.Printf("%s: %+v", appdir.Name, err)
		},
	})
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            appdir.Name,
		URL:              "/",
		Width:            1260,
		Height:           880,
		MinWidth:         960,
		MinHeight:        680,
		Frameless:        true,
		Hidden:           true,
		BackgroundColour: themeRGBA(h.store.Prefs().Theme),
		Windows: application.WindowsWindow{
			Theme:                      application.SystemDefault,
			NonClientRegionSupport:     true,
			WebView2CompositionHosting: true,
		},
	})
	desktopHost.attach(app, window)
	// Wails 运行时就绪后显示窗口（等价于 ready-to-show）。
	// Windows 上 WebViewNavigationCompleted 更可靠：页面加载完成必触发，
	// 而 WindowRuntimeReady 依赖页面成功拉取 /wails/runtime.js。
	window.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(*application.WindowEvent) {
		application.InvokeAsync(func() {
			window.Show()
			window.Focus()
		})
	})

	// 关闭守卫：操作进行中阻止关闭；保存中等待刷盘。
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if h.isBusy() {
			e.Cancel()
			h.windows.MessageBox("info", "应用正在执行操作", "请等待当前操作完成后再关闭窗口。")
			return
		}
		if h.store.IsSaving() {
			e.Cancel()
			go func() {
				if err := h.store.Flush(); err != nil {
					h.windows.MessageBox("error", "自动保存失败", err.Error())
					return
				}
				application.InvokeAsync(func() {
					if window != nil {
						window.Close()
					}
				})
			}()
		}
	})

	if err := app.Run(); err != nil {
		return err
	}
	return nil
}

// Serve runs the same UI in a plain browser for development and testing.
func Serve(version string, addr string) error {
	if err := appdir.MigrateLegacyConfig(); err != nil {
		return err
	}
	if err := ExtractScripts(scriptsDir()); err != nil {
		log.Printf("%s: %+v", appdir.Name, err)
	}
	h := newHost(version, true)
	h.init(homeDir())
	h.windows = &webWindows{}
	handler := h.Handler()
	log.Printf("%s: serving UI at http://%s", appdir.Name, addr)
	return serveHTTP(addr, handler)
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// desktopWindows adapts the Wails window to the Windows capability interface.
type desktopWindows struct {
	app    *application.App
	window *application.WebviewWindow
}

// attach is called before app.Run; the pointers are only dereferenced from
// the main thread or via InvokeAsync/InvokeSync afterwards.
func (d *desktopWindows) attach(app *application.App, window *application.WebviewWindow) {
	d.app = app
	d.window = window
}

func (d *desktopWindows) Minimize() {
	application.InvokeAsync(func() {
		if d.window != nil {
			d.window.Minimise()
		}
	})
}

func (d *desktopWindows) ToggleMaximize() {
	application.InvokeAsync(func() {
		if d.window != nil {
			d.window.ToggleMaximise()
		}
	})
}

func (d *desktopWindows) RequestClose() {
	application.InvokeAsync(func() {
		if d.window != nil {
			d.window.Close()
		}
	})
}

func (d *desktopWindows) IsMaximized() bool {
	if d.window == nil {
		return false
	}
	var maximized bool
	application.InvokeSync(func() {
		if d.window != nil {
			maximized = d.window.IsMaximised()
		}
	})
	return maximized
}

func (d *desktopWindows) ChooseExecutable(title string) (string, error) {
	if d.app == nil {
		return "", errors.New("窗口未就绪")
	}
	var chosen string
	var failure error
	application.InvokeSync(func() {
		dialog := d.app.Dialog.OpenFile()
		dialog.SetTitle(title)
		dialog.AddFilter("桌面应用", "*.exe")
		dialog.CanChooseFiles(true)
		if d.window != nil {
			dialog.AttachToWindow(d.window)
		}
		path, err := dialog.PromptForSingleSelection()
		if err != nil {
			failure = err
			return
		}
		chosen = path
	})
	return chosen, failure
}

func (d *desktopWindows) MessageBox(kind, title, message string) {
	if d.app == nil {
		return
	}
	application.InvokeAsync(func() {
		dialog := d.app.Dialog.Info()
		if kind == "error" {
			dialog = d.app.Dialog.Error()
		}
		dialog.SetTitle(title)
		dialog.SetMessage(message)
		dialog.AddButton("知道了").SetAsDefault()
		if d.window != nil {
			dialog.AttachToWindow(d.window)
		}
		dialog.Show()
	})
}

func (d *desktopWindows) ApplyTheme(theme string) {
	application.InvokeAsync(func() {
		if d.window != nil {
			d.window.SetBackgroundColour(themeRGBA(theme))
		}
	})
}

func (d *desktopWindows) AfterOperation() {}

// webWindows no-ops everything for browser mode.
type webWindows struct{}

func (w *webWindows) Minimize()         {}
func (w *webWindows) ToggleMaximize()   {}
func (w *webWindows) RequestClose()     {}
func (w *webWindows) IsMaximized() bool { return false }
func (w *webWindows) ChooseExecutable(title string) (string, error) {
	return "", fmt.Errorf("桌面连接未启用")
}
func (w *webWindows) MessageBox(kind, title, message string) {}
func (w *webWindows) ApplyTheme(theme string)                {}
func (w *webWindows) AfterOperation()                        {}

var _ Windows = (*desktopWindows)(nil)
var _ Windows = (*webWindows)(nil)

var _ = settings.DefaultPreferences
