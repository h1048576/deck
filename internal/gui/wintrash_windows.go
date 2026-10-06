//go:build windows

package gui

import (
	"runtime"
	"syscall"
	"unsafe"
)

const (
	foDelete     = 3
	fofAllowUndo = 0x40
	fofNoConfirm = 0x10
	fofNoErrorUI = 0x400
	fofSilent    = 0x4
)

var shell32 = syscall.NewLazyDLL("shell32.dll")
var procSHFileOperationW = shell32.NewProc("SHFileOperationW")

type shFileOpStructW struct {
	hwnd              syscall.Handle
	wFunc             uint32
	pFrom             *uint16
	pTo               *uint16
	fFlags            uint16
	fAnyOpsAborted    int32
	hNameMappings     uintptr
	lpszProgressTitle *uint16
}

// moveToTrash sends a path to the recycle bin (FOF_ALLOWUNDO), mirroring
// Electron's shell.trashItem behavior.
func moveToTrash(path string) error {
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	// SHFileOperationW 接收以两个空字符结尾的路径列表。
	from = append(from, 0)
	operation := shFileOpStructW{
		hwnd:   0,
		wFunc:  foDelete,
		pFrom:  &from[0],
		pTo:    nil,
		fFlags: fofAllowUndo | fofNoConfirm | fofNoErrorUI | fofSilent,
	}
	ret, _, callErr := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&operation)))
	runtime.KeepAlive(from)
	if ret != 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.EINVAL
	}
	if operation.fAnyOpsAborted != 0 {
		return syscall.EINVAL
	}
	return nil
}
