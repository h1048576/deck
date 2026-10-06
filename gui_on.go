//go:build !nogui

package main

import (
	"deck/internal/gui"
)

const hasGUI = true

func runGUI(version string) error {
	return gui.Run(version)
}
