//go:build !windows

package gui

import "fmt"

// moveToTrash is unavailable outside Windows; the UI gates it off.
func moveToTrash(path string) error {
	return fmt.Errorf("当前操作系统暂不支持移动到回收站")
}
