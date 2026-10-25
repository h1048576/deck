//go:build !windows

package gui

import "fmt"

func applicationRunning(executablePath string, id string) (bool, error) {
	return false, fmt.Errorf("当前操作系统暂不支持检测应用运行状态。")
}
