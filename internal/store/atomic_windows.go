//go:build windows

package store

import (
	"fmt"
	"syscall"
	"unsafe"
)

func replaceFile(source, destination string) error {
	sourcePtr, err := syscall.UTF16PtrFromString(source)
	if err != nil { return err }
	destinationPtr, err := syscall.UTF16PtrFromString(destination)
	if err != nil { return err }
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	moveFileEx := kernel32.NewProc("MoveFileExW")
	const moveFileReplaceExisting = 0x1
	const moveFileWriteThrough = 0x8
	result, _, callErr := moveFileEx.Call(uintptr(unsafe.Pointer(sourcePtr)), uintptr(unsafe.Pointer(destinationPtr)), moveFileReplaceExisting|moveFileWriteThrough)
	if result == 0 { return fmt.Errorf("replace data file: %w", callErr) }
	return nil
}

