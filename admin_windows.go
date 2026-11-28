//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

func isRunAsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

var (
	adminPrivOnce  sync.Once
	adminPrivValue bool
)

const waitParentPIDArgPrefix = "--wait-pid="

func hasAdminPrivileges() bool {
	adminPrivOnce.Do(func() {
		adminPrivValue = windows.GetCurrentProcessToken().IsElevated()
	})
	return adminPrivValue
}

func buildAdminRestartParams(args string, parentPID int) string {
	parts := make([]string, 0, 2)
	if parentPID > 0 {
		parts = append(parts, waitParentPIDArgPrefix+strconv.Itoa(parentPID))
	}
	if trimmed := strings.TrimSpace(args); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

func waitParentPIDFromArgs() {
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, waitParentPIDArgPrefix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, waitParentPIDArgPrefix)))
		if err != nil || pid <= 0 || pid == os.Getpid() {
			return
		}
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			return
		}
		_, _ = windows.WaitForSingleObject(handle, 45*1000)
		_ = windows.CloseHandle(handle)
		return
	}
}

func restartAsAdmin(args string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root := detectAppRoot()
	exe = resolveAdminRestartExe(root, exe)
	if !fileExists(exe) {
		return fmt.Errorf("管理员重启文件不存在：%s", exe)
	}
	// 清理历史上遗留的不安全批处理脚本
	_ = os.Remove(filepath.Join(root, "data", "gui", "admin-restart.cmd"))

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(buildAdminRestartParams(args, os.Getpid()))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, params, cwd, windows.SW_SHOWNORMAL)
}

func resolveAdminRestartExe(root, exe string) string {
	exeDir := filepath.Dir(exe)
	base := filepath.Base(exe)
	withoutDev := strings.Replace(base, "-dev.exe", ".exe", 1)
	candidates := []string{
		filepath.Join(exeDir, withoutDev),
		filepath.Join(exeDir, "SanTian-Singbox.exe"),
		filepath.Join(root, withoutDev),
		filepath.Join(root, "SanTian-Singbox.exe"),
		exe,
		filepath.Join(exeDir, "go-singbox-gui-dev.exe"),
		filepath.Join(root, "go-singbox-gui-dev.exe"),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return exe
}
