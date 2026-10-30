package gui

import (
	"strconv"
	"strings"

	"deck/internal/settings"
)

// Manus Studio 2.0 的聊天容器与 WorkBuddy 不同，配置保持一致，选择器单独适配。
func manusInjection(s settings.DroidSettings) string {
	width := s.Width
	if !isAutoSize(width) {
		width = "min(100%, " + width + ")"
	}
	maxWidth := "100%"
	if s.MaxWidth != "none" {
		maxWidth = "min(100%, " + s.MaxWidth + ")"
	}
	css := strings.NewReplacer(
		"FONT_FAMILY", safeFontFamily(s.FontFamily),
		"FONT_SIZE", strconv.Itoa(s.FontSize),
		"MAX_WIDTH", maxWidth,
		"CONTENT_WIDTH", width,
	).Replace(`:root, body, body * {
  font-family: FONT_FAMILY !important;
  font-size: FONT_SIZEpx !important;
}
.mx-auto[class~="sm:max-w-[800px]"] {
  box-sizing: border-box !important;
  width: CONTENT_WIDTH !important;
  min-width: 0 !important;
  max-width: MAX_WIDTH !important;
  margin-left: auto !important;
  margin-right: auto !important;
}
.mx-auto[class~="sm:max-w-[800px]"] .mx-auto[class~="sm:max-w-[800px]"] {
  width: 100% !important;
  max-width: 100% !important;
}
[data-manus-wide-composer] {
  box-sizing: border-box !important;
  width: var(--manus-wide-composer-width) !important;
  min-width: 0 !important;
  max-width: var(--manus-wide-composer-width) !important;
}`)
	return `(() => {
  const apply = () => {
    let style = document.getElementById('manus-wide-ui-override');
    if (!style) {
      style = document.createElement('style');
      style.id = 'manus-wide-ui-override';
      (document.head || document.documentElement).appendChild(style);
    }
    style.textContent = ` + jsString(css) + `;
    window.__manusWideComposerGuard?.dispose();
    // 输入区使用 fixed 布局，必须跟随实际聊天容器，不能沿用原生 768px 上限。
    const observed = new Set();
    const resize = new ResizeObserver(() => schedule());
    let frame = 0;
    const scan = () => {
      const containers = new Set();
      for (const input of document.querySelectorAll('[data-testid="chat-input-composer"]')) {
        const composer = input.closest('.fixed[class~="sm:max-w-[768px]"]');
        const container = composer?.parentElement?.closest('.mx-auto[class~="sm:max-w-[800px]"]');
        if (!container) continue;
        containers.add(container);
        const box = getComputedStyle(container);
        const width = Math.max(0, container.clientWidth - parseFloat(box.paddingLeft) - parseFloat(box.paddingRight));
        composer.setAttribute('data-manus-wide-composer', '');
        composer.style.setProperty('--manus-wide-composer-width', width + 'px');
      }
      for (const container of observed) {
        if (!containers.has(container)) { resize.unobserve(container); observed.delete(container); }
      }
      for (const container of containers) {
        if (!observed.has(container)) { resize.observe(container); observed.add(container); }
      }
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(() => { frame = 0; scan(); });
    };
    const mutations = new MutationObserver(schedule);
    mutations.observe(document.documentElement, { childList: true, subtree: true });
    window.__manusWideComposerGuard = { dispose: () => {
      mutations.disconnect(); resize.disconnect(); cancelAnimationFrame(frame);
    }};
    scan();
    return true;
  };
  if (document.head || document.documentElement) return apply();
  document.addEventListener('DOMContentLoaded', apply, { once: true });
  return true;
})() && ` + sidebarInjection(settings.Manus, s.SidebarWidth)
}
