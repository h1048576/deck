package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

// decodePreferencesInput 构造一个最小可解析的 setting.json 映射。
func decodePreferencesInput() map[string]any {
	return map[string]any{
		"applications": map[string]any{
			"droid": map[string]any{"fontFamily": "Cascadia Mono, LXGW WenKai Mono", "foregroundColor": "#FCFCFC"},
			"codex": map[string]any{"fontFamily": "Cascadia Mono, LXGW WenKai Mono", "foregroundColor": "#F0F0F0"},
		},
		"appearance": map[string]any{"fontFamily": "Cascadia Mono, LXGW WenKai Mono"},
		"terminal":   map[string]any{"fontFamily": "Cascadia Mono, LXGW WenKai Mono"},
	}
}

func TestDecodePreferencesMigratesLegacyFontDefaults(t *testing.T) {
	prefs, err := decodePreferences(decodePreferencesInput())
	if err != nil {
		t.Fatalf("decodePreferences 出错：%v", err)
	}
	if prefs.FontDefaultsVersion != FontDefaultsVersion {
		t.Errorf("fontDefaultsVersion = %d，应为 %d", prefs.FontDefaultsVersion, FontDefaultsVersion)
	}
	for id, want := range map[FeatureId]string{Droid: "#F0F0F0", Codex: "#F0F0F0"} {
		if got := prefs.Applications[id].ForegroundColor; got != want {
			t.Errorf("%s 前景色 = %q，迁移后应为 %q", id, got, want)
		}
	}
	for id := range prefs.Applications {
		if got := prefs.Applications[id].FontFamily; got != "Maple Mono NF CN" {
			t.Errorf("%s 字体 = %q，迁移后应为 Maple Mono NF CN", id, got)
		}
	}
	if got := prefs.Appearance.FontFamily; got != "Maple Mono NF CN" {
		t.Errorf("外观字体 = %q，迁移后应为 Maple Mono NF CN", got)
	}
	if got := prefs.Terminal.FontFamily; got != "Maple Mono NF CN" {
		t.Errorf("终端字体 = %q，迁移后应为 Maple Mono NF CN", got)
	}
}

func TestDecodePreferencesKeepsLegacyFontsAfterMigration(t *testing.T) {
	input := decodePreferencesInput()
	input["fontDefaultsVersion"] = float64(FontDefaultsVersion)
	prefs, err := decodePreferences(input)
	if err != nil {
		t.Fatalf("decodePreferences 出错：%v", err)
	}
	if got := prefs.Applications[Droid].FontFamily; got != LegacyDefaultFontFamily {
		t.Errorf("已迁移版本不应再改字体，得到 %q", got)
	}
	if got := prefs.Applications[Droid].ForegroundColor; got != LegacyDefaultForeground {
		t.Errorf("已迁移版本不应再改前景色，得到 %q", got)
	}
}

func TestDecodePreferencesKeepsCustomFonts(t *testing.T) {
	input := decodePreferencesInput()
	input["applications"].(map[string]any)["droid"] = map[string]any{
		"fontFamily": "Maple Mono Normal NF CN", "foregroundColor": "#ABCDEF",
	}
	prefs, err := decodePreferences(input)
	if err != nil {
		t.Fatalf("decodePreferences 出错：%v", err)
	}
	if got := prefs.Applications[Droid].FontFamily; got != "Maple Mono Normal NF CN" {
		t.Errorf("手动选择的字体被改写：%q", got)
	}
	if got := prefs.Applications[Droid].ForegroundColor; got != "#ABCDEF" {
		t.Errorf("手动选择的前景色被改写：%q", got)
	}
}

func TestPreferencesMarshalIncludesFontDefaultsVersion(t *testing.T) {
	text, err := json.Marshal(DefaultPreferences())
	if err != nil {
		t.Fatalf("marshal 出错：%v", err)
	}
	if !strings.Contains(string(text), `"fontDefaultsVersion":`) {
		t.Errorf("序列化结果缺少 fontDefaultsVersion 字段")
	}
}
