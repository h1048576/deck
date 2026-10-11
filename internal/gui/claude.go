package gui

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"deck/internal/settings"
)

//go:embed scripts/claude-font/font.js
var claudeFontSource string

var claudeFontDefaultPattern = regexp.MustCompile(`const fontFamily = '[^'\r\n]*'`)

// claudeFontScript 生成在 Claude 开发者工具 Console 中执行的字体脚本。
// Claude 拒绝带调试端口启动，deck 无法注入，只能由用户手动执行。
func claudeFontScript(fontFamily string) (string, error) {
	if !settings.ValidFontFamily(fontFamily) {
		return "", fmt.Errorf("%s", settings.FontFamilyError)
	}
	location := claudeFontDefaultPattern.FindStringIndex(claudeFontSource)
	if location == nil {
		return "", fmt.Errorf("Claude 字体脚本模板无效")
	}
	return claudeFontSource[:location[0]] + "const fontFamily = " + jsString(strings.TrimSpace(fontFamily)) + claudeFontSource[location[1]:], nil
}
