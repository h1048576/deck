package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"deck/internal/settings"
)

//go:embed injection-templates.json
var injectionTemplatesJSON []byte

//go:embed scripts/codex-wide/content-width.js
var codexContentWidthSource string

// jsString renders a JS string literal exactly like JSON.stringify.
func jsString(value string) string {
	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return `""`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

var templateVarPattern = regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

var genericFontPattern = regexp.MustCompile(`(?i)^(serif|sans-serif|monospace|system-ui|ui-[a-z-]+|cursive|fantasy)$`)

func safeFontFamily(fontFamily string) string {
	parts := make([]string, 0, 4)
	for _, name := range strings.Split(fontFamily, ",") {
		value := strings.TrimSpace(name)
		value = strings.Trim(value, `'"`)
		if genericFontPattern.MatchString(value) {
			parts = append(parts, value)
		} else {
			parts = append(parts, jsString(value))
		}
	}
	return strings.Join(parts, ", ") + ", monospace"
}

func isAutoSize(value string) bool { return value == "auto" || value == "fit-content" }

// applicationInjection builds the injected JS for non-droid applications.
func applicationInjection(id string, s settings.DroidSettings) string {
	if id == "dsh" {
		return dshInjection(s)
	}
	templates := map[string]struct {
		CSS    string `json:"css"`
		Source string `json:"source"`
	}{}
	_ = json.Unmarshal(injectionTemplatesJSON, &templates)
	template := templates[id]
	font := safeFontFamily(s.FontFamily)
	hiddenUiCss := []string{}
	if s.HideLocalMerge {
		hiddenUiCss = append(hiddenUiCss, `[data-testid="changes-primary-cta"],[data-testid="changes-primary-cta-caret"]{display:none !important;}`)
	}
	if s.HideGitDiff {
		hiddenUiCss = append(hiddenUiCss, `[data-testid="composer-diff-stat-pill"]{display:none !important;}`)
	}
	hiddenChangesCss := ""
	if s.HideChanges {
		hiddenChangesCss = `[data-testid="chat-summary-panel"]{display:none !important;}`
	}
	safeWidth := s.Width
	if id == "codex" && isAutoSize(s.Width) {
		safeWidth = "100%"
	}
	if (id == "qoder" || id == "workbuddy") && !isAutoSize(s.Width) {
		safeWidth = fmt.Sprintf("min(100%%, %s)", s.Width)
	}
	safeMaxWidth := s.MaxWidth
	if id == "qoder" || id == "workbuddy" {
		if s.MaxWidth == "none" {
			safeMaxWidth = "100%"
		} else {
			safeMaxWidth = fmt.Sprintf("min(100%%, %s)", s.MaxWidth)
		}
	}
	// WorkBuddy 首页左右各有 24px 内边距，容器宽度需包含这部分空间。
	homePageWidth := "100%"
	if !isAutoSize(s.Width) {
		homePageWidth = fmt.Sprintf("min(100%%, calc(%s + 48px))", s.Width)
	}
	homePageMaxWidth := "100%"
	if s.MaxWidth != "none" {
		homePageMaxWidth = fmt.Sprintf("min(100%%, calc(%s + 48px))", s.MaxWidth)
	}
	values := map[string]string{
		"safeWidth":         safeWidth,
		"safeMaxWidth":      safeMaxWidth,
		"homePageWidth":     homePageWidth,
		"homePageMaxWidth":  homePageMaxWidth,
		"safeFontFamily":    font,
		"cssFontFamily":     font,
		"ContentFontSize":   fmt.Sprintf("%d", s.FontSize),
		"ContentFontWeight": fmt.Sprintf("%d", s.FontWeight),
		"hiddenUiCss":       strings.Join(hiddenUiCss, "\n"),
		"hiddenChangesCss":  hiddenChangesCss,
	}
	css := templateVarPattern.ReplaceAllStringFunc(template.CSS, func(match string) string {
		sub := templateVarPattern.FindStringSubmatch(match)
		key := sub[1]
		if key == "" {
			key = sub[2]
		}
		value, ok := values[key]
		if !ok {
			return match
		}
		return value
	})
	source := strings.ReplaceAll(template.Source, "$cssJson", jsString(css))
	source = strings.ReplaceAll(source, "$widthGuardSource", codexContentWidthSource)
	source = strings.ReplaceAll(source, "$PreventSummary", map[bool]string{true: "1", false: "0"}[s.PreventSummary])
	sidebar := sidebarInjection(id, s.SidebarWidth)
	if id == "workbuddy" {
		sidebar = workbuddySidebarInjection(s.SidebarWidth)
	}
	layoutSource := strings.TrimSpace(source)
	layoutSource = strings.TrimSuffix(layoutSource, ";")
	if id == "zcode" || id == "qoder" {
		return layoutSource + " && " + terminalInjection(id, s) + " && " + sidebar
	}
	return layoutSource + " && " + sidebar
}
