package main

import (
	"runtime"
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
	sa, sd := restrictedLocalIPCSecurityAttributes()
	handle, err := windows.CreateMutex(sa, true, name)
	runtime.KeepAlive(sd)
	runtime.KeepAlive(sa)
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
