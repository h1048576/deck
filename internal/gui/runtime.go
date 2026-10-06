package gui

import "runtime"

func isWindowsRuntime() bool { return runtime.GOOS == "windows" }

func isMacRuntime() bool { return runtime.GOOS == "darwin" }

func isUnixDesktop() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "linux" }
