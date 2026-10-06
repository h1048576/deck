//go:build !nogui

package main

import (
	"wide-pure/internal/gui"
)

const hasGUI = true

func runGUI(version string) error {
	return gui.Run(version)
}
