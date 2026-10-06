package gui

import (
	"encoding/json"

	"wide-pure/internal/settings"
)

// installTerminalFollowJS is the compiled form of the original TypeScript
// installTerminalFollow function; it is evaluated inside the target apps.
const installTerminalFollowJS = `function installTerminalFollow(settings) {
  const scope = window;
  const key = settings.key;
  if (scope[key]) { scope[key].update(settings); return true; }
  const entries = new Map();
  let current = settings;
  let scheduled = false;
  function apply(entry) {
    const next = current.enabled ? { fontFamily: current.fontFamily, fontSize: current.fontSize, fontWeight: current.fontWeight } : entry.original;
    let changed = false;
    for (const [name, value] of Object.entries(next)) {
      if (entry.terminal.options[name] !== value) { entry.terminal.options[name] = value; changed = true; }
    }
    if (changed || entry.terminal.element?.getClientRects().length) entry.fit?.fit();
  }
  function scan() {
    scheduled = false;
    for (const [element] of entries) if (!element.isConnected) { resize.unobserve(element); entries.delete(element); }
    for (const element of document.querySelectorAll('.xterm')) {
      if (entries.has(element)) continue;
      // React 函数组件的 ref 保存着 xterm 和 FitAddon，仅检查终端所在的祖先链。
      let terminal, fit;
      for (let node = element; node && !terminal; node = node.parentElement) {
        const fiberKey = Object.keys(node).find(name => name.startsWith('__reactFiber$'));
        let fiber = fiberKey ? node[fiberKey] : null;
        for (let depth = 0; fiber && depth < 32; depth++, fiber = fiber.return) {
          let hook = fiber.memoizedState;
          for (let count = 0; hook && count < 80; count++, hook = hook.next) {
            const value = hook.memoizedState?.current;
            if (value?.element === element && value.options && typeof value.resize === 'function') terminal = value;
            if (typeof value?.fit === 'function' && typeof value?.proposeDimensions === 'function') fit = value;
          }
          if (terminal) break;
        }
      }
      // Qoder 未将 FitAddon 保存在 React ref 中，从已加载的终端插件获取。
      if (terminal && !fit) fit = terminal._addonManager?._addons?.find(addon => !addon.isDisposed && typeof addon.instance?.fit === 'function' && typeof addon.instance?.proposeDimensions === 'function')?.instance;
      if (!terminal || !fit) continue;
      const original = { fontFamily: terminal.options.fontFamily, fontSize: terminal.options.fontSize, fontWeight: terminal.options.fontWeight };
      const entry = { terminal, fit, original };
      entries.set(element, entry);
      resize.observe(element);
      try { apply(entry) } catch { entries.delete(element); resize.unobserve(element); }
    }
  }
  function schedule() { if (!scheduled) { scheduled = true; setTimeout(scan, 0); } }
  const resize = new ResizeObserver(records => {
    for (const record of records) {
      const entry = entries.get(record.target);
      if (entry && record.contentRect.width && record.contentRect.height) { try { apply(entry) } catch { /* 终端可能正在卸载。 */ } }
    }
  });
  const observer = new MutationObserver(records => {
    if (records.some(record => [...record.addedNodes, ...record.removedNodes].some(node => node instanceof Element && (node.matches('.xterm') || node.querySelector('.xterm'))))) schedule();
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  scope[key] = {
    update(next) {
      current = next;
      for (const entry of entries.values()) { try { apply(entry) } catch { /* 忽略已释放的实例。 */ } }
      schedule();
    }
  };
  document.fonts.ready.then(() => { for (const entry of entries.values()) { try { apply(entry) } catch { /* 终端可能已释放。 */ } } });
  scan();
  return true;
}`

var terminalKeys = map[string]string{
	"droid": "__wideDroidTerminalFollow",
	"zcode": "__wideZcodeTerminalFollow",
	"dsh":   "__wideDshTerminalFollow",
	"qoder": "__wideQoderTerminalFollow",
}

type terminalFollowSettings struct {
	Key        string `json:"key"`
	Enabled    bool   `json:"enabled"`
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	FontWeight int    `json:"fontWeight"`
}

// terminalInjection syncs xterm options so terminal fonts follow the settings.
func terminalInjection(id string, s settings.DroidSettings) string {
	payload, _ := json.Marshal(terminalFollowSettings{
		Key:        terminalKeys[id],
		Enabled:    s.TerminalFollow,
		FontFamily: s.FontFamily,
		FontSize:   s.FontSize,
		FontWeight: s.FontWeight,
	})
	return "(" + installTerminalFollowJS + ")(" + string(payload) + ")"
}
