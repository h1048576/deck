package gui

import (
	"strconv"
	"strings"
)

type sidebarLayout struct {
	selector string
	scan     string
	css      string
}

// 各应用的侧栏容器和收起状态不同，只标记展开的布局面板。
var sidebarLayouts = map[string]sidebarLayout{
	"codex": {
		selector: `.app-shell-left-panel, [data-app-navigation-rail]`,
		scan: `for (const panel of document.querySelectorAll('.app-shell-left-panel')) {
      const rail = panel.querySelector('[data-app-navigation-rail]');
      const railWidth = rail ? Number.parseFloat(getComputedStyle(rail).width) : 0;
      const nativeWidth = Number.parseFloat(panel.style.width);
      panel.toggleAttribute(attribute, nativeWidth > railWidth + 1 && panel.getAttribute('data-slate-sidebar-peeking') !== 'true');
    }`,
		css: `[data-wide-codex-sidebar] > :first-child > :first-child {
      width: 100% !important; min-width: 0 !important;
    }`,
	},
	"zcode": {
		selector: `[data-workspace-shell], [data-workspace-sidebar-panel], [data-workspace-sidebar-panel] > aside`,
		scan: `for (const panel of document.querySelectorAll('[data-workspace-sidebar-panel="true"]')) {
      const content = panel.querySelector('aside');
      panel.toggleAttribute(attribute, !!content && content.getAttribute('aria-hidden') !== 'true');
    }`,
		css: `[data-workspace-shell]:has(> [data-wide-zcode-sidebar]) {
      --workspace-sidebar-panel-width: WIDTH !important;
      --workspace-sidebar-width: WIDTH !important;
    }`,
	},
	"qoder": {
		selector: `[data-panel][id="sidebar"], [data-layout-region="a"]`,
		scan: `for (const panel of document.querySelectorAll('[data-panel][id="sidebar"]')) {
      const content = panel.querySelector('[data-layout-region="a"]');
      panel.toggleAttribute(attribute, !!content && content.getAttribute('aria-hidden') !== 'true' && Number.parseFloat(panel.style.flexGrow) > 0);
    }`,
		css: `[data-wide-qoder-sidebar] [data-layout-sidebar-content] { width: 100% !important; }`,
	},
	"paseo": {
		selector: `[data-testid="left-sidebar-resize-handle"]`,
		scan: `for (const handle of document.querySelectorAll('[data-testid="left-sidebar-resize-handle"]')) {
      const panel = handle.parentElement?.parentElement;
      if (panel) panel.toggleAttribute(attribute, true);
    }`,
	},
}

func cssLength(value string) string {
	// 百分比按窗口宽度计算，避免嵌套侧栏容器改变百分比的参照。
	if strings.HasSuffix(value, "%") {
		parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err == nil {
			return strconv.FormatFloat(parsed, 'f', -1, 64) + "vw"
		}
	}
	return value
}

func sidebarInjection(id string, sidebarWidth string) string {
	width := cssLength(sidebarWidth)
	layout := sidebarLayouts[id]
	attribute := `data-wide-` + id + `-sidebar`
	css := `[` + attribute + `] {
    box-sizing: border-box !important;
    width: ` + width + ` !important;
    min-width: 0 !important;
    max-width: 100% !important;
    flex: 0 0 ` + width + ` !important;
  }
  ` + strings.ReplaceAll(layout.css, "WIDTH", width)
	var sb strings.Builder
	sb.WriteString(`(() => {
    const apply = () => {
      const key = `)
	sb.WriteString(jsString(`__wide` + id + `SidebarGuard`))
	sb.WriteString(`;
      window[key]?.dispose();
      let style = document.getElementById(`)
	sb.WriteString(jsString(id + `-wide-sidebar-override`))
	sb.WriteString(`);
      if (!style) {
        style = document.createElement('style');
        style.id = `)
	sb.WriteString(jsString(id + `-wide-sidebar-override`))
	sb.WriteString(`;
        (document.head || document.documentElement).appendChild(style);
      }
      const css = `)
	sb.WriteString(jsString(css))
	sb.WriteString(`;
      if (style.textContent !== css) style.textContent = css;
      const attribute = `)
	sb.WriteString(jsString(attribute))
	sb.WriteString(`;
      const selector = `)
	sb.WriteString(jsString(layout.selector + `, [` + attribute + `]`))
	sb.WriteString(`;
      let frame = 0;
      const scan = () => { `)
	sb.WriteString(layout.scan)
	sb.WriteString(` };
      const schedule = () => {
        if (!frame) frame = requestAnimationFrame(() => { frame = 0; scan(); });
      };
      const relevant = node => node instanceof Element && (node.matches(selector) || !!node.querySelector(selector));
      const observer = new MutationObserver(records => {
        if (records.some(record => record.type === 'attributes'
          ? record.target.matches(selector)
          : [...record.addedNodes, ...record.removedNodes].some(relevant))) schedule();
      });
      observer.observe(document.documentElement, {subtree: true, childList: true, attributes: true,
        attributeFilter: ['class', 'style', 'aria-hidden', 'data-slate-sidebar-peeking']});
      window[key] = {dispose: () => { observer.disconnect(); cancelAnimationFrame(frame); }};
      scan();
      return true;
    };
    if (document.documentElement) return apply();
    document.addEventListener('DOMContentLoaded', apply, {once: true});
    return true;
  })()`)
	return sb.String()
}
