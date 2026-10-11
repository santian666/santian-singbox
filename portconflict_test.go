package main

import (
	"os"
	"runtime"
	"testing"
)

func TestPortConflictDetection(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only test")
	}

	// 1. Test querying free port
	res, err := findTCPPortsProcesses("127.0.0.1", []int{65431, 65432})
	if err != nil {
		t.Fatalf("findTCPPortsProcesses failed on free ports: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty result for free ports, got: %+v", res)
	}

	// 2. Test querying occupied port (e.g. 20808 or 20990 if sing-box is running)
	resOccupied, err := findTCPPortsProcesses("127.0.0.1", []int{20808, 20990})
	if err != nil {
		t.Fatalf("findTCPPortsProcesses failed on occupied ports: %v", err)
	}
	t.Logf("findTCPPortsProcesses on 20808/20990: %+v", resOccupied)

	// 3. Test isWindowsProcessRunning
	curPID := os.Getpid()
	if !isWindowsProcessRunning(curPID) {
		t.Fatalf("isWindowsProcessRunning failed for current PID %d", curPID)
	}
	if isWindowsProcessRunning(999999) {
		t.Fatalf("isWindowsProcessRunning should return false for nonexistent PID 999999")
	}
}
