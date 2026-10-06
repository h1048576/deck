package widecfg

import "runtime"

func isWindowsRuntime() bool { return runtime.GOOS == "windows" }
