package gui

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"

	"deck/internal/settings"
)

//go:embed scripts/antigravity-wide/interface.js
var antigravityInterfaceSource string

//go:embed scripts/antigravity-wide/current-directory.js
var antigravityCurrentDirectorySource string

func antigravityInjection(s settings.DroidSettings) string {
	width := s.Width
	if isAutoSize(width) {
		width = "100%"
	}
	patterns, _ := json.Marshal(s.HiddenModelPatterns)
	layout := strings.NewReplacer(
		"__DECK_ANTIGRAVITY_WIDTH__", width,
		"__DECK_ANTIGRAVITY_SIDEBAR__", cssLength(s.SidebarWidth),
		"__DECK_ANTIGRAVITY_FONT__", jsString(safeFontFamily(s.FontFamily)),
		"__DECK_ANTIGRAVITY_FONT_SIZE__", strconv.Itoa(s.FontSize),
		"__DECK_HIDDEN_MODEL_PATTERNS__", string(patterns),
	).Replace(antigravityInterfaceSource)
	layout = strings.TrimSuffix(strings.TrimSpace(layout), ";") + " && " +
		terminalInjection(settings.Antigravity, s) + " && " + antigravityCurrentDirectorySource + " && " +
		composerUsageInjection(settings.Antigravity)
	source := withApplicationStyle(settings.Antigravity, s, layout)
	// 同一个调试端口会列出内置浏览器和辅助窗口。每次加载都重新确认是本机 Hub 主页面。
	return `(() => {
  if (window.top !== window || !/^https:\/\/127\.0\.0\.1:\d+$/.test(location.origin)) return false;
  const apply = () => {
    if (window.__APP_CONFIG__?.productName !== 'antigravity' ||
        typeof window.nativeStorage?.updateItems !== 'function' ||
        !window.electronNative || !document.getElementById('root')) return false;
    return ` + source + `;
  };
  if (document.readyState !== 'loading') return apply();
  document.addEventListener('DOMContentLoaded', apply, { once: true });
  return true;
})()`
}
