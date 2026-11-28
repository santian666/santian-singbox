//go:build windows

package main

import (
	"testing"
)

func TestBootProxyCleanupArgParsingAndCommand(t *testing.T) {
	cmd := buildBootProxyCleanupCommand(`C:\Program Files\SanTian Singbox\SanTian-Singbox.exe`, 20808)
	want := `"C:\Program Files\SanTian Singbox\SanTian-Singbox.exe" --boot-cleanup-proxy --port=20808`
	if cmd != want {
		t.Fatalf("command = %q, want %q", cmd, want)
	}

	if _, handled, err := parseBootProxyCleanupArgs([]string{"--other"}); handled || err != nil {
		t.Fatalf("expected unhandled for unrelated args, got handled=%v err=%v", handled, err)
	}
	port, handled, err := parseBootProxyCleanupArgs([]string{bootProxyCleanupArg, "--port=20808"})
	if !handled || err != nil || port != 20808 {
		t.Fatalf("expected port=20808 handled=true err=nil, got port=%d handled=%v err=%v", port, handled, err)
	}
	if _, handled, err := parseBootProxyCleanupArgs([]string{bootProxyCleanupArg, "--port=0"}); !handled || err == nil {
		t.Fatalf("expected error for port=0, got handled=%v err=%v", handled, err)
	}
	if _, handled, err := parseBootProxyCleanupArgs([]string{bootProxyCleanupArg}); !handled || err == nil {
		t.Fatalf("expected error for missing --port, got handled=%v err=%v", handled, err)
	}
}

func TestRegisterAndUnregisterSystemProxyBootCleanup(t *testing.T) {
	origExe := bootCleanupExePath
	origSet := bootCleanupRegSet
	origDel := bootCleanupRegDelete
	t.Cleanup(func() {
		bootCleanupExePath = origExe
		bootCleanupRegSet = origSet
		bootCleanupRegDelete = origDel
	})

	setCalls := 0
	var gotName, gotVal string
	bootCleanupRegSet = func(name, value string) error {
		setCalls++
		gotName = name
		gotVal = value
		return nil
	}
	delCalls := 0
	var gotDelName string
	bootCleanupRegDelete = func(name string) error {
		delCalls++
		gotDelName = name
		return nil
	}

	// Test binaries must NOT write to HKCU RunOnce
	bootCleanupExePath = func() (string, error) { return `C:\Temp\GoSingBox.test.exe`, nil }
	if err := registerSystemProxyBootCleanup(20808); err != nil {
		t.Fatalf("registerSystemProxyBootCleanup(test bin) failed: %v", err)
	}
	if setCalls != 0 {
		t.Fatalf("expected test binary to skip RunOnce registration, got %d calls", setCalls)
	}

	// Production binary writes exact RunOnce entry
	bootCleanupExePath = func() (string, error) { return `C:\SanTian\SanTian-Singbox.exe`, nil }
	if err := registerSystemProxyBootCleanup(20808); err != nil {
		t.Fatalf("registerSystemProxyBootCleanup failed: %v", err)
	}
	if setCalls != 1 || gotName != bootProxyCleanupRunOnceValue {
		t.Fatalf("unexpected RunOnce set: calls=%d name=%q", setCalls, gotName)
	}
	wantVal := `"C:\SanTian\SanTian-Singbox.exe" --boot-cleanup-proxy --port=20808`
	if gotVal != wantVal {
		t.Fatalf("RunOnce value = %q, want %q", gotVal, wantVal)
	}

	if err := unregisterSystemProxyBootCleanup(); err != nil {
		t.Fatalf("unregisterSystemProxyBootCleanup failed: %v", err)
	}
	if delCalls != 1 || gotDelName != bootProxyCleanupRunOnceValue {
		t.Fatalf("unexpected RunOnce delete: calls=%d name=%q", delCalls, gotDelName)
	}
}

func TestExecuteBootProxyCleanupBehavior(t *testing.T) {
	origOwned := bootCleanupOwned
	origOpen := bootCleanupPortOpen
	origDisable := bootCleanupDisable
	t.Cleanup(func() {
		bootCleanupOwned = origOwned
		bootCleanupPortOpen = origOpen
		bootCleanupDisable = origDisable
	})

	disableCalls := 0
	bootCleanupDisable = func() error {
		disableCalls++
		return nil
	}

	// Case 1: Proxy does NOT point to our port (disabled or owned by another proxy tool) -> no-op
	bootCleanupOwned = func(port int) (bool, error) { return false, nil }
	bootCleanupPortOpen = func(port int) bool { return false }
	if err := executeBootProxyCleanup(20808); err != nil {
		t.Fatalf("case 1 failed: %v", err)
	}
	if disableCalls != 0 {
		t.Fatalf("expected 0 disable calls when proxy not owned, got %d", disableCalls)
	}

	// Case 2: Proxy points to our port AND port is already listening (app already started) -> no-op
	bootCleanupOwned = func(port int) (bool, error) { return true, nil }
	bootCleanupPortOpen = func(port int) bool { return true }
	if err := executeBootProxyCleanup(20808); err != nil {
		t.Fatalf("case 2 failed: %v", err)
	}
	if disableCalls != 0 {
		t.Fatalf("expected 0 disable calls when port is actively listening, got %d", disableCalls)
	}

	// Case 3: Sudden power loss residual — Proxy points to our port AND port is dead -> disables proxy cleanly
	bootCleanupOwned = func(port int) (bool, error) { return true, nil }
	bootCleanupPortOpen = func(port int) bool { return false }
	if err := executeBootProxyCleanup(20808); err != nil {
		t.Fatalf("case 3 failed: %v", err)
	}
	if disableCalls != 1 {
		t.Fatalf("expected 1 disable call for stale residual proxy, got %d", disableCalls)
	}
}
