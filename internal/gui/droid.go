package gui

import (
	"strings"

	"deck/internal/settings"
)

// droidInjectionSource builds Droid's combined style/terminal/sidebar script.
func droidInjectionSource(s settings.DroidSettings) string {
	width := s.Width
	if !isAutoSize(s.Width) {
		width = "min(100%, " + s.Width + ")"
	}
	maxWidth := "100%"
	if s.MaxWidth != "none" {
		maxWidth = "min(100%, " + s.MaxWidth + ")"
	}
	hideLocalMerge := ""
	if s.HideLocalMerge {
		hideLocalMerge = `[data-testid="changes-primary-cta"],[data-testid="changes-primary-cta-caret"] {display:none !important;}`
	}
	hideGitDiff := ""
	if s.HideGitDiff {
		hideGitDiff = `[data-testid="composer-diff-stat-pill"] {display:none !important;}`
	}
	css := `:root,body,body *:not(.xterm, .xterm *) { font-family:` + s.FontFamily + ` !important;font-size:` + itoa(s.FontSize) + `px !important;font-weight:` + itoa(s.FontWeight) + ` !important; }
    [data-droid-wide-content],[data-new-session-composer-v2="true"] { box-sizing:border-box !important;min-width:0 !important;width:` + width + ` !important;max-width:` + maxWidth + ` !important; }
    [data-testid="chat-composer-wrapper"] > [data-direction="row"][data-flex-row="true"]:first-child,
    [data-testid="chat-composer-wrapper"] [contenteditable="true"][role="textbox"] { box-sizing:border-box !important;height:` + s.ChatHeight + ` !important;min-height:` + s.ChatHeight + ` !important;max-height:` + s.ChatHeight + ` !important; }
    ` + hideLocalMerge + `
    ` + hideGitDiff
	var sb strings.Builder
	sb.WriteString(`(() => {
    const apply = () => {
      let style = document.getElementById('droid-wide-ui-override');
      if (!style) { style = document.createElement('style'); style.id = 'droid-wide-ui-override'; (document.head || document.documentElement).appendChild(style); }
      style.textContent = `)
	sb.WriteString(jsString(css))
	sb.WriteString(`;
      `)
	sb.WriteString(terminalInjection("droid", s))
	sb.WriteString(`;
      `)
	sb.WriteString(droidSidebarInjection(s.SidebarWidth))
	sb.WriteString(`;
      if (window.__droidWideContentGuard) { window.__droidWideContentGuard.scan(); return true; }
      const roots = new Set(); let scheduled = false;
      const mark = root => { if (!(root instanceof Element)) return; [root, ...root.querySelectorAll('*')].forEach(el => { if (!el.hasAttribute('data-droid-wide-content') && getComputedStyle(el).maxWidth === '768px') el.setAttribute('data-droid-wide-content', ''); }); };
      const scan = (root = document.documentElement) => { if (!(root instanceof Element)) return; roots.add(root); if (scheduled) return; scheduled = true; requestAnimationFrame(() => { scheduled = false; const batch = [...roots]; roots.clear(); batch.forEach(root => { if (root.isConnected) mark(root); }); }); };
      const observer = new MutationObserver(records => records.forEach(record => { if (record.type === 'attributes') scan(record.target); record.addedNodes.forEach(scan); }));
      observer.observe(document.documentElement, {attributes:true, attributeFilter:['class','style'], childList:true, subtree:true});
      window.__droidWideContentGuard = {observer, scan}; scan(); [250,1000,3000].forEach(delay => setTimeout(() => scan(), delay)); return true;
    };
    if (document.documentElement) return apply();
    document.addEventListener('DOMContentLoaded', apply, {once:true}); return true;
  })()`)
	return sb.String()
}
