//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	bootProxyCleanupArg          = "--boot-cleanup-proxy"
	bootProxyCleanupRunOnceKey   = `Software\Microsoft\Windows\CurrentVersion\RunOnce`
	bootProxyCleanupRunOnceValue = "SanTian-Singbox-ProxyCleanup"
)

var (
	bootCleanupExePath = os.Executable
	bootCleanupRegSet  = func(name, value string) error {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, bootProxyCleanupRunOnceKey, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer key.Close()
		return key.SetStringValue(name, value)
	}
	bootCleanupRegDelete = func(name string) error {
		key, err := registry.OpenKey(registry.CURRENT_USER, bootProxyCleanupRunOnceKey, registry.SET_VALUE)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				return nil
			}
			return err
		}
		defer key.Close()
		if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	bootCleanupOwned = func(port int) (bool, error) {
		return systemProxyEnabledForPort(port)
	}
	bootCleanupPortOpen = func(port int) bool {
		return tcpPortOpen("127.0.0.1", port, 250*time.Millisecond)
	}
	bootCleanupDisable = DisableSystemProxy
)

func isBootCleanupTestBinary(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".test.exe") ||
		strings.HasSuffix(lower, "-test.exe") ||
		strings.HasSuffix(lower, ".test")
}

func buildBootProxyCleanupCommand(exe string, port int) string {
	return fmt.Sprintf(`"%s" %s --port=%d`, filepath.Clean(strings.TrimSpace(exe)), bootProxyCleanupArg, port)
}

func parseBootProxyCleanupArgs(args []string) (int, bool, error) {
	hasFlag := false
	port := 0
	for _, arg := range args {
		switch {
		case arg == bootProxyCleanupArg:
			hasFlag = true
		case strings.HasPrefix(arg, "--port="):
			parsed, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--port=")))
			if err != nil {
				return 0, true, fmt.Errorf("无效端口参数：%w", err)
			}
			port = parsed
		}
	}
	if !hasFlag {
		return 0, false, nil
	}
	if port <= 0 || port > 65535 {
		return 0, true, fmt.Errorf("系统代理开机自清端口无效：%d", port)
	}
	return port, true, nil
}

func registerSystemProxyBootCleanup(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("系统代理开机自清端口无效：%d", port)
	}
	exe, err := bootCleanupExePath()
	if err != nil {
		return err
	}
	exe = strings.TrimSpace(exe)
	if exe == "" || isBootCleanupTestBinary(exe) {
		return nil
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return bootCleanupRegSet(bootProxyCleanupRunOnceValue, buildBootProxyCleanupCommand(exe, port))
}

func unregisterSystemProxyBootCleanup() error {
	return bootCleanupRegDelete(bootProxyCleanupRunOnceValue)
}

func runBootProxyCleanupFromArgs() (bool, int) {
	port, handled, err := parseBootProxyCleanupArgs(os.Args[1:])
	if !handled {
		return false, 0
	}
	if err != nil {
		return true, 2
	}
	if err := executeBootProxyCleanup(port); err != nil {
		return true, 1
	}
	return true, 0
}

// executeBootProxyCleanup clears stale Windows system proxy left behind by a
// sudden power loss / hard reboot, ONLY when:
//  1. Windows system proxy still points to our port (127.0.0.1:<port>), AND
//  2. Nothing is currently listening on 127.0.0.1:<port> (app was not already started).
func executeBootProxyCleanup(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("系统代理开机自清端口无效：%d", port)
	}
	owned, err := bootCleanupOwned(port)
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if bootCleanupPortOpen(port) {
		return nil
	}
	return bootCleanupDisable()
}
