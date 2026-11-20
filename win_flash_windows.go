//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type flashWInfo struct {
	cbSize    uint32
	hwnd      uintptr
	dwFlags   uint32
	uCount    uint32
	dwTimeout uint32
}

func flashMainWindow() {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindowW := user32.NewProc("FindWindowW")
	flashWindowEx := user32.NewProc("FlashWindowEx")
	titlePtr, err := syscall.UTF16PtrFromString(appTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	const (
		flashwAll       = 0x00000003
		flashwTimerNoFG = 0x0000000C
	)
	info := flashWInfo{
		cbSize:  uint32(unsafe.Sizeof(flashWInfo{})),
		hwnd:    hwnd,
		dwFlags: flashwAll | flashwTimerNoFG,
		uCount:  5,
	}
	_, _, _ = flashWindowEx.Call(uintptr(unsafe.Pointer(&info)))
}
