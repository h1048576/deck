package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"deck/internal/settings"
)

//go:embed injection-templates.json
var injectionTemplatesJSON []byte

//go:embed scripts/codex-wide/content-width.js
var codexContentWidthSource string

//go:embed scripts/line-height.js
var applicationLineHeightSource string

//go:embed scripts/codex-wide/remaining-usage.js
var codexRemainingUsageSource string

//go:embed scripts/codex-wide/current-project.js
var codexCurrentProjectSource string

//go:embed scripts/codex-wide/model-filter.js
var codexModelFilterSource string

//go:embed scripts/codex-wide/model-selector.js
var codexModelSelectorSource string

//go:embed scripts/current-project.js
var currentProjectSource string

//go:embed scripts/zcode-wide/model-label.js
var zcodeModelLabelSource string

//go:embed scripts/dsh-wide/reasoning-effort.js
var dshReasoningEffortSource string

//go:embed scripts/workbuddy-wide/model-filter.js
var workbuddyModelFilterSource string

func currentProjectInjection(id string) string {
	return strings.ReplaceAll(currentProjectSource, "__DECK_PROJECT_APP__", id)
}

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

type injectionTemplate struct {
	CSS    string `json:"css"`
	Source string `json:"source"`
}

var injectionTemplates = sync.OnceValue(func() map[string]injectionTemplate {
	templates := map[string]injectionTemplate{}
	_ = json.Unmarshal(injectionTemplatesJSON, &templates)
	return templates
})

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
	return withApplicationStyle(id, s, applicationLayoutInjection(id, s))
}

func applicationLayoutInjection(id string, s settings.DroidSettings) string {
	if id == "dsh" {
		return dshInjection(s)
	}
	if id == "manus" {
		return manusInjection(s)
	}
	template := injectionTemplates()[id]
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
	if (id == "qoder" || id == "workbuddy" || id == "paseo") && !isAutoSize(s.Width) {
		safeWidth = fmt.Sprintf("min(100%%, %s)", s.Width)
	}
	safeMaxWidth := s.MaxWidth
	if id == "qoder" || id == "workbuddy" || id == "paseo" {
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
	widthGuardSource := codexContentWidthSource
	if id == "codex" {
		widthGuardSource += "\n" + codexRemainingUsageSource + "\n" + codexCurrentProjectSource
		patterns, _ := json.Marshal(s.HiddenModelPatterns)
		widthGuardSource += "\n" + strings.ReplaceAll(codexModelFilterSource, "__DECK_HIDDEN_MODEL_PATTERNS__", string(patterns))
		// 使用数组传递顺序，避免 Go map 排序以及 JS 对数字键的重新排序。
		ordered := make([][2]any, 0, len(s.ModelSelector))
		seen := make(map[string]bool, len(s.ModelSelector))
		for _, name := range s.ModelSelectorOrder {
			if value, ok := s.ModelSelector[name]; ok && !seen[name] {
				ordered = append(ordered, [2]any{name, value})
				seen[name] = true
			}
		}
		var remaining []string
		for name := range s.ModelSelector {
			if !seen[name] {
				remaining = append(remaining, name)
			}
		}
		slices.Sort(remaining)
		for _, name := range remaining {
			ordered = append(ordered, [2]any{name, s.ModelSelector[name]})
		}
		selector, _ := json.Marshal(ordered)
		widthGuardSource += "\n" + strings.ReplaceAll(codexModelSelectorSource, "__DECK_MODEL_SELECTOR__", string(selector))
	}
	source = strings.ReplaceAll(source, "$widthGuardSource", widthGuardSource)
	source = strings.ReplaceAll(source, "$PreventSummary", map[bool]string{true: "1", false: "0"}[s.PreventSummary])
	sidebar := sidebarInjection(id, s.SidebarWidth)
	if id == "workbuddy" {
		sidebar = workbuddySidebarInjection(s.SidebarWidth)
	}
	layoutSource := strings.TrimSpace(source)
	layoutSource = strings.TrimSuffix(layoutSource, ";")
	if id == "zcode" {
		return layoutSource + " && " + terminalInjection(id, s) + " && " + sidebar + " && " + currentProjectInjection(id) + " && " + zcodeModelLabelSource
	}
	if id == "qoder" {
		return layoutSource + " && " + terminalInjection(id, s) + " && " + sidebar
	}
	if id == "workbuddy" {
		patterns, _ := json.Marshal(s.HiddenModelPatterns)
		modelFilter := strings.ReplaceAll(workbuddyModelFilterSource, "__DECK_HIDDEN_MODEL_PATTERNS__", string(patterns))
		return layoutSource + " && " + sidebar + " && " + creditsInjection(id, s.ShowWorkbuddyCredits) + " && " + modelFilter
	}
	return layoutSource + " && " + sidebar
}
