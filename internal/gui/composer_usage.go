package gui

import (
	_ "embed"
	"strings"

	"deck/internal/settings"
)

//go:embed scripts/composer-usage.js
var composerUsageSource string

func composerUsageInjection(id string) string {
	args := jsString(id)
	if id == settings.Antigravity {
		args += ", () => " + strings.TrimSpace(antigravityReadUsageSource)
	}
	return "(" + composerUsageSource + ")(" + args + ")"
}
