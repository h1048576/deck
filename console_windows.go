//go:build windows

package main

import (
	"os"
	"syscall"
)

// Attach the parent console so `deck serve` prints output even though
// the release binary is built with -H windowsgui.
func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attachConsole := kernel32.NewProc("AttachConsole")
	attached, _, _ := attachConsole.Call(uintptr(0xFFFFFFFF)) // ATTACH_PARENT_PROCESS
	if attached == 0 {
		return
	}
	for _, name := range []string{"CONOUT$", "CONIN$"} {
		handle, err := os.OpenFile(name, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if name == "CONOUT$" {
			os.Stdout = handle
			os.Stderr = handle
		} else {
			os.Stdin = handle
		}
	}
}
