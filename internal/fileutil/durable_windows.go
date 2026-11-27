//go:build windows

package fileutil

import (
	"syscall"
	"unsafe"
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replaceDurable(from, to, _ string) error {
	src, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	const movefileReplaceExisting = 0x1
	const movefileWriteThrough = 0x8
	ok, _, callErr := moveFileExW.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(dst)), movefileReplaceExisting|movefileWriteThrough)
	if ok == 0 {
		return callErr
	}
	return nil
}
