//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func listProcesses() ([]processInfo, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	items := make([]processInfo, 0, 256)
	for {
		pid := int(entry.ProcessID)
		path := processImagePath(uint32(pid))
		name := windows.UTF16ToString(entry.ExeFile[:])
		items = append(items, processInfo{
			PID:       pid,
			ParentPID: int(entry.ParentProcessID),
			Name:      name,
			Path:      path,
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return items, nil
}

func processImagePath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}
