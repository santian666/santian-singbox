package main

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	singleInstanceMu     sync.Mutex
	singleInstanceHandle windows.Handle
)

func acquireSingleInstanceLock() (bool, error) {
	singleInstanceMu.Lock()
	defer singleInstanceMu.Unlock()
	if singleInstanceHandle != 0 {
		return true, nil
	}
	name, err := windows.UTF16PtrFromString(`Local\FelixSingboxGUI`)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	singleInstanceHandle = handle
	return true, nil
}

func releaseSingleInstanceLock() {
	singleInstanceMu.Lock()
	defer singleInstanceMu.Unlock()
	if singleInstanceHandle == 0 {
		return
	}
	_ = windows.ReleaseMutex(singleInstanceHandle)
	_ = windows.CloseHandle(singleInstanceHandle)
	singleInstanceHandle = 0
}
