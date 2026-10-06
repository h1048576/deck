package gui

import (
	"strconv"
	"strings"

	"deck/internal/settings"
)

// dshInjection targets the official DeepSeek Harness desktop app via its
// public layout variables and data markers.
func dshInjection(s settings.DroidSettings) string {
	font := safeFontFamily(s.FontFamily)
	length := func(value string) string {
		// 将百分比换算为对话容器的长度，避免每一层重复缩窄。
		if strings.HasSuffix(value, "%") {
			parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
			if err == nil {
				return "calc(var(--dsh-conversation-column-width, 100vw) * " + strconv.FormatFloat(parsed, 'f', -1, 64) + "/100)"
			}
		}
		return value
	}
	width := "100%"
	if !isAutoSize(s.Width) {
		width = length(s.Width)
	}
	contentWidth := width
	if s.MaxWidth != "none" {
		contentWidth = "min(" + width + ", " + length(s.MaxWidth) + ")"
	}
	// 主框架铺满视口；百分比换成 vw，保证标题栏变量和绝对定位的分隔条使用相同长度。
	sidebarWidth := cssLength(s.SidebarWidth)
	css := `
    [data-wide-dsh-frame]:not([data-sidebar-collapsed="true"]) {
      --dsh-windows-sidebar-width: ` + sidebarWidth + ` !important;
      grid-template-columns: ` + sidebarWidth + ` var(--wide-dsh-layout-tail, minmax(0px, 1fr) minmax(0px, 0px)) !important;
    }
    [data-wide-dsh-frame]:not([data-sidebar-collapsed="true"]) > :first-child > [data-slot="sidebar"] > :first-child {
      box-sizing: border-box !important;
      width: 100% !important;
      min-width: 0 !important;
      max-width: 100% !important;
    }
    [data-wide-dsh-frame]:not([data-sidebar-collapsed="true"]) > [data-side="sidebar"] {
      left: ` + sidebarWidth + ` !important;
    }
    :root, body {
      --dsw-font-family: ` + font + ` !important;
      --ds-font-family-code: ` + font + ` !important;
      --dsh-content-font-size: ` + itoa(s.FontSize) + `px !important;
      --dsh-content-font-size-secondary: ` + itoa(s.FontSize) + `px !important;
      --dsh-content-font-delta: ` + itoa(s.FontSize-14) + `px !important;
      --dsh-content-font-delta-secondary: ` + itoa(s.FontSize-13) + `px !important;
    }
    :root, body, body *:not(.xterm, .xterm *) {
      font-family: ` + font + ` !important;
      font-size: ` + itoa(s.FontSize) + `px !important;
      font-weight: ` + itoa(s.FontWeight) + ` !important;
    }
    [data-conversation-content] {
      --dsh-chat-content-width: ` + contentWidth + ` !important;
      --dsh-composer-card-max-width: calc(` + contentWidth + ` + 32px) !important;
    }
    [data-composer-seat] {
      --dsh-composer-text-max-height: ` + s.ChatHeight + ` !important;
    }
    [data-composer-input] {
      box-sizing: border-box !important;
      height: ` + s.ChatHeight + ` !important;
      min-height: ` + s.ChatHeight + ` !important;
      max-height: ` + s.ChatHeight + ` !important;
      overflow-y: auto !important;
    }
  `
	var sb strings.Builder
	sb.WriteString(`(() => {
    if (location.protocol !== 'dsh-app:' || location.hostname !== 'app') return false;
    const apply = () => {
      let style = document.getElementById('dsh-wide-ui-override');
      if (!style) {
        style = document.createElement('style');
        style.id = 'dsh-wide-ui-override';
        (document.head || document.documentElement).appendChild(style);
      }
      style.textContent = `)
	sb.WriteString(jsString(css))
	sb.WriteString(`;
      `)
	sb.WriteString(terminalInjection("dsh", s))
	sb.WriteString(`;
      // 原生框架将三列宽度写到内联样式；只替换左列，随原生渲染同步中列和右列。
      const syncLayout = () => {
        for (const rightbar of document.querySelectorAll('[data-rightbar-col]')) {
          const frame = rightbar.parentElement;
          if (!frame) continue;
          const columns = /^[0-9.]+px\s+(.+)$/.exec(frame.style.gridTemplateColumns);
          if (!columns) continue;
          frame.setAttribute('data-wide-dsh-frame', '');
          if (frame.style.getPropertyValue('--wide-dsh-layout-tail') !== columns[1]) {
            frame.style.setProperty('--wide-dsh-layout-tail', columns[1]);
          }
        }
      };
      window.__wideDshLayoutObserver?.disconnect();
      const observer = new MutationObserver(records => {
        const layoutChanged = records.some(record => {
          if (record.type === 'attributes') return record.target.hasAttribute('data-wide-dsh-frame');
          return Array.from(record.addedNodes).some(node => node instanceof Element &&
            (node.matches('[data-rightbar-col]') || node.querySelector('[data-rightbar-col]')));
        });
        if (layoutChanged) syncLayout();
      });
      observer.observe(document.documentElement, {subtree: true, childList: true, attributes: true, attributeFilter: ['style']});
      window.__wideDshLayoutObserver = observer;
      syncLayout();
      return true;
    };
    if (document.documentElement) return apply();
    document.addEventListener('DOMContentLoaded', apply, {once: true});
    return true;
  })()`)
	return sb.String()
}

func itoa(value int) string { return strconv.Itoa(value) }

// droidSidebarInjection mirrors the VS Code-style split view of Droid.
func droidSidebarInjection(sidebarWidth string) string {
	width := cssLength(sidebarWidth)
	css := `
    [data-wide-droid-sidebar-frame] > [data-testid="window-sidebar"] {
      width: ` + width + ` !important; min-width: 0 !important;
    }
    [data-wide-droid-sidebar-frame] {
      --window-history-inset: calc(var(--wide-droid-native-history-inset) + ` + width + ` - var(--wide-droid-native-sidebar-width)) !important;
    }
    [data-wide-droid-sidebar-layout] > .split-view-container > .split-view-view:first-child {
      width: ` + width + ` !important;
    }
    [data-wide-droid-sidebar-main] {
      left: calc(var(--wide-droid-native-main-left) + ` + width + ` - var(--wide-droid-native-sidebar-width)) !important;
      width: max(0px, calc(var(--wide-droid-native-main-width) + var(--wide-droid-native-sidebar-width) - ` + width + `)) !important;
    }
    [data-wide-droid-sidebar-sash] {
      left: calc(var(--wide-droid-native-sash-left) + ` + width + ` - var(--wide-droid-native-sidebar-width)) !important;
    }`
	var sb strings.Builder
	sb.WriteString(`(() => {
    const apply = () => {
      window.__wideDroidSidebarGuard?.dispose();
      let style = document.getElementById('droid-wide-sidebar-override');
      if (!style) { style = document.createElement('style'); style.id = 'droid-wide-sidebar-override'; (document.head || document.documentElement).appendChild(style); }
      const css = `)
	sb.WriteString(jsString(css))
	sb.WriteString(`;
      if (style.textContent !== css) style.textContent = css;
      const setLength = (el, name, value) => {
        const next = value + 'px';
        if (el.style.getPropertyValue(name) !== next) el.style.setProperty(name, next);
      };
      const scan = () => {
        for (const sidebar of document.querySelectorAll('[data-testid="window-sidebar"]')) {
          const frame = sidebar.parentElement;
          const panes = frame?.querySelector('[data-window-sidebar-panes] > .split-view');
          const views = panes?.querySelector(':scope > .split-view-container');
          const first = views?.firstElementChild, main = first?.nextElementSibling;
          const nativeWidth = Number.parseFloat(first?.style.width);
          const mainLeft = Number.parseFloat(main?.style.left), mainWidth = Number.parseFloat(main?.style.width);
          const active = sidebar.getAttribute('data-sidebar-visible') === 'true' && nativeWidth > 0 && [mainLeft, mainWidth].every(Number.isFinite);
          frame?.toggleAttribute('data-wide-droid-sidebar-frame', active);
          panes?.toggleAttribute('data-wide-droid-sidebar-layout', active);
          main?.toggleAttribute('data-wide-droid-sidebar-main', active);
          if (!panes) continue;
          for (const sash of panes.querySelectorAll(':scope > .sash-container > .sash')) {
            const sashLeft = Number.parseFloat(sash.style.left);
            const boundary = active && Math.abs(sashLeft - nativeWidth) <= 4;
            sash.toggleAttribute('data-wide-droid-sidebar-sash', boundary);
            if (boundary) setLength(sash, '--wide-droid-native-sash-left', sashLeft);
          }
          if (!active) continue;
          setLength(frame, '--wide-droid-native-sidebar-width', nativeWidth);
          setLength(main, '--wide-droid-native-main-left', mainLeft);
          setLength(main, '--wide-droid-native-main-width', mainWidth);
          const history = Number.parseFloat(frame.style.getPropertyValue('--window-history-inset'));
          setLength(frame, '--wide-droid-native-history-inset', Number.isFinite(history) ? history : nativeWidth);
        }
      };
      let animationFrame = 0;
      const selector = '[data-testid="window-sidebar"], [data-window-sidebar-panes], [data-wide-droid-sidebar-frame], [data-wide-droid-sidebar-layout], [data-wide-droid-sidebar-main], [data-wide-droid-sidebar-sash]';
      const relevant = node => node instanceof Element && (node.matches(selector) || !!node.querySelector(selector));
      const observer = new MutationObserver(records => {
        if (!records.some(record => record.type === 'attributes'
          ? record.target.matches(selector) || record.target.matches('.split-view-view, .sash') && !!record.target.closest('[data-window-sidebar-panes]')
          : [...record.addedNodes, ...record.removedNodes].some(relevant))) return;
        if (!animationFrame) animationFrame = requestAnimationFrame(() => { animationFrame = 0; scan(); });
      });
      observer.observe(document.documentElement, {subtree: true, childList: true, attributes: true, attributeFilter: ['class', 'style', 'data-sidebar-visible']});
      window.__wideDroidSidebarGuard = {dispose: () => { observer.disconnect(); cancelAnimationFrame(animationFrame); }};
      scan(); return true;
    };
    if (document.documentElement) return apply();
    document.addEventListener('DOMContentLoaded', apply, {once: true}); return true;
  })()`)
	return sb.String()
}

// workbuddySidebarInjection reassigns space between the sidebar and main
// areas while preserving the native right panel boundary.
func workbuddySidebarInjection(value string) string {
	width := cssLength(value)
	css := `
    [data-wide-wb-grid="expanded"] > [data-view-id="sidebar"] {
      width: ` + width + ` !important;
    }
    [data-wide-wb-grid="expanded"] > [data-view-id="sidebar"] > .conversation-sidebar,
    [data-wide-wb-grid="expanded"] > [data-view-id="sidebar"] > .conversation-sidebar > :first-child {
      width: 100% !important;
    }
    [data-wide-wb-grid="expanded"] > [data-wide-wb-main] {
      left: calc(var(--wide-wb-native-main-left) + ` + width + ` - var(--wide-wb-native-sidebar-width)) !important;
      width: max(0px, calc(var(--wide-wb-native-main-width) + var(--wide-wb-native-sidebar-width) - ` + width + `)) !important;
    }
    [data-wide-wb-grid="expanded"] > [data-wide-wb-sidebar-sash] {
      left: calc(var(--wide-wb-native-sidebar-left) + ` + width + ` - 2px) !important;
    }
    .teams-container [class*="gridViewDrawerItemOpen"]:has(> .conversation-sidebar) {
      width: ` + width + ` !important;
    }
    .teams-container [class*="gridViewDrawerItemOpen"] > .conversation-sidebar,
    .teams-container [class*="gridViewDrawerItemOpen"] > .conversation-sidebar > :first-child {
      width: 100% !important;
    }
  `
	var sb strings.Builder
	sb.WriteString(`(() => {
    const apply = () => {
      let style = document.getElementById('workbuddy-wide-sidebar-override');
      if (!style) {
        style = document.createElement('style');
        style.id = 'workbuddy-wide-sidebar-override';
        (document.head || document.documentElement).appendChild(style);
      }
      style.textContent = `)
	sb.WriteString(jsString(css))
	sb.WriteString(`;
      const setLength = (element, name, value) => {
        const length = value + 'px';
        if (element.style.getPropertyValue(name) !== length) element.style.setProperty(name, length);
      };
      const syncLayout = () => {
        for (const grid of document.querySelectorAll('[data-wide-wb-grid]')) grid.removeAttribute('data-wide-wb-grid');
        for (const sidebar of document.querySelectorAll('.teams-container .teams-grid-scroll-content [data-view-id="sidebar"]')) {
          const grid = sidebar.parentElement;
          const container = sidebar.closest('.teams-container');
          const main = grid?.querySelector(':scope > [data-view-id="main-content"], :scope > [data-view-id="login-view"]');
          const nativeWidth = Number.parseFloat(sidebar.style.width);
          if (!grid || !main || container.classList.contains('sidebar-collapsed') || !(nativeWidth > 0)) continue;
          const mainLeft = Number.parseFloat(main.style.left);
          const mainWidth = Number.parseFloat(main.style.width);
          const sidebarLeft = Number.parseFloat(sidebar.style.left);
          if (![mainLeft, mainWidth, sidebarLeft].every(Number.isFinite) || mainWidth <= 0) continue;
          grid.setAttribute('data-wide-wb-grid', 'expanded');
          main.setAttribute('data-wide-wb-main', '');
          setLength(grid, '--wide-wb-native-sidebar-width', nativeWidth);
          setLength(grid, '--wide-wb-native-sidebar-left', sidebarLeft);
          setLength(grid, '--wide-wb-native-main-left', mainLeft);
          setLength(grid, '--wide-wb-native-main-width', mainWidth);
          for (const sash of grid.querySelectorAll(':scope > [class*="sash"]')) {
            const isBoundary = Math.abs(Number.parseFloat(sash.style.left) - (sidebarLeft + nativeWidth - 2)) < 1;
            sash.toggleAttribute('data-wide-wb-sidebar-sash', isBoundary);
          }
        }
      };
      window.__wideWorkbuddySidebarObserver?.disconnect();
      const selector = '.teams-container, [data-view-id], .conversation-sidebar';
      const observer = new MutationObserver(records => {
        const changed = records.some(record => record.type === 'attributes'
          ? record.target.matches(selector + ', [data-wide-wb-grid], [data-wide-wb-sidebar-sash]')
          : [...record.addedNodes, ...record.removedNodes].some(node => node instanceof Element &&
            (node.matches(selector) || node.querySelector(selector))));
        if (changed) syncLayout();
      });
      observer.observe(document.documentElement, {subtree: true, childList: true, attributes: true, attributeFilter: ['style', 'class']});
      window.__wideWorkbuddySidebarObserver = observer;
      syncLayout();
      return true;
    };
    if (document.documentElement) return apply();
    document.addEventListener('DOMContentLoaded', apply, {once: true});
    return true;
  })()`)
	return sb.String()
}
