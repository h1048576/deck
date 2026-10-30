package gui

import (
	_ "embed"
	"encoding/json"
	"strings"

	"deck/internal/settings"
)

//go:embed scripts/application-colors.js
var applicationColorsSource string

//go:embed scripts/native-style-sampler.js
var nativeStyleSamplerSource string

//go:embed scripts/font-weight.js
var applicationFontWeightSource string

// withApplicationStyle 在原有布局注入后应用应用级配色和百分比行高。
func withApplicationStyle(id string, s settings.DroidSettings, layout string) string {
	background, foreground := s.BackgroundColor, s.ForegroundColor
	if background == "" {
		background = "#111111"
	}
	if foreground == "" {
		foreground = "#F0F0F0"
	}
	percent := s.LineHeightPercent
	if percent <= 0 {
		percent = 100
	}
	config, _ := json.Marshal(map[string]string{"app": id, "background": background, "foreground": foreground})
	colors := strings.ReplaceAll(applicationColorsSource, "__DECK_STYLE_CONFIG__", string(config))
	lineHeight := strings.ReplaceAll(applicationLineHeightSource, "__DECK_LINE_HEIGHT_PERCENT__", itoa(percent))
	boldWeight := s.BoldFontWeight
	if boldWeight == 0 {
		boldWeight = 700
	}
	weights := strings.NewReplacer("__DECK_FONT_WEIGHT__", itoa(s.FontWeight), "__DECK_BOLD_FONT_WEIGHT__", itoa(boldWeight)).Replace(applicationFontWeightSource)
	return `(() => { if (window.frameElement?.id === 'deck-native-style-sampler') return true;
const apply = () => { ` + strings.TrimSuffix(strings.TrimSpace(layout), ";") + `;
` + nativeStyleSamplerSource + "\n" + weights + "\n" + colors + "\n" + lineHeight + `
return true; }; if (document.documentElement) return apply();
document.addEventListener('DOMContentLoaded', apply, { once: true }); return true; })()`
}
