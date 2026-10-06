package harnesscfg

import "runtime"

func isWindowsRuntime() bool { return runtime.GOOS == "windows" }
