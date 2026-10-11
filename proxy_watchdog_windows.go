//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const proxyWatchdogArg = "--proxy-watchdog"

type proxyWatchdogOptions struct {
	ParentPID int
	Root      string
	StatePath string
	CorePath  string
	GUIPath   string
}

func (a *App) startProxyWatchdog() error {
	if a == nil || a.store == nil || a.manager == nil {
		return errors.New("watchdog 组件未初始化")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(
		exe,
		proxyWatchdogArg,
		"--parent-pid="+strconv.Itoa(os.Getpid()),
		"--root="+a.root,
		"--state="+a.store.path,
		"--core="+a.manager.corePath,
		"--gui="+exe,
	)
	hideCommandWindow(cmd)
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func runProxyWatchdogFromArgs() (bool, int) {
	if !hasArg(proxyWatchdogArg) {
		return false, 0
	}
	options, err := parseProxyWatchdogOptions(os.Args[1:])
	if err != nil {
		return true, 2
	}
	if err := waitParentAndCleanupProxy(options); err != nil {
		return true, 1
	}
	return true, 0
}

func parseProxyWatchdogOptions(args []string) (proxyWatchdogOptions, error) {
	var options proxyWatchdogOptions
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--parent-pid="):
			options.ParentPID, _ = strconv.Atoi(strings.TrimPrefix(arg, "--parent-pid="))
		case strings.HasPrefix(arg, "--root="):
			options.Root = strings.TrimSpace(strings.TrimPrefix(arg, "--root="))
		case strings.HasPrefix(arg, "--state="):
			options.StatePath = strings.TrimSpace(strings.TrimPrefix(arg, "--state="))
		case strings.HasPrefix(arg, "--core="):
			options.CorePath = strings.TrimSpace(strings.TrimPrefix(arg, "--core="))
		case strings.HasPrefix(arg, "--gui="):
			options.GUIPath = strings.TrimSpace(strings.TrimPrefix(arg, "--gui="))
		}
	}
	if options.ParentPID <= 0 || options.Root == "" || options.StatePath == "" || options.CorePath == "" || options.GUIPath == "" {
		return proxyWatchdogOptions{}, errors.New("watchdog 参数不完整")
	}
	options.Root = filepath.Clean(options.Root)
	options.StatePath = filepath.Clean(options.StatePath)
	options.CorePath = filepath.Clean(options.CorePath)
	options.GUIPath = filepath.Clean(options.GUIPath)
	return options, nil
}

func waitParentAndCleanupProxy(options proxyWatchdogOptions) error {
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(options.ParentPID))
	if err == nil {
		_, waitErr := windows.WaitForSingleObject(parent, windows.INFINITE)
		windows.CloseHandle(parent)
		if waitErr != nil {
			return waitErr
		}
	}
	return cleanupProxyAfterParentExit(options)
}

func cleanupProxyAfterParentExit(options proxyWatchdogOptions) error {
	logger := NewLogger(filepath.Join(options.Root, "logs", "gui", "watchdog.log"))
	defer logger.Close()
	handoff, handoffErr := consumeProxyWatchdogAdminHandoff(options.Root, options.ParentPID)
	if handoffErr != nil {
		logger.Error("读取管理员 TUN 交接标记失败，继续执行异常退出清理：%v", handoffErr)
	} else if handoff {
		logger.Info("检测到管理员 TUN 交接标记，跳过旧实例异常退出清理")
		return nil
	}
	store, storeErr := NewStore(options.StatePath)
	settings := defaultSettings()
	if storeErr == nil {
		settings = store.Snapshot().Settings
		normalizeSettings(&settings)
	}
	wasEnabled := shouldRunProxy(settings)
	if wasEnabled {
		logger.Error("检测到 GUI 已结束但代理入口仍开启，开始清理异常退出残留")
	}

	var result error
	if err := killWatchedSingBoxChildren(options); err != nil {
		result = errors.Join(result, err)
	}
	newGUIRunning, err := otherGUIProcessRunning(options.GUIPath, options.ParentPID, os.Getpid())
	if err != nil {
		result = errors.Join(result, fmt.Errorf("检查新 GUI 进程失败：%w", err))
	} else if newGUIRunning {
		logger.Info("检测到同路径的新 GUI 已启动，跳过系统代理、TUN 和状态清理")
		return result
	}
	if owned, err := systemProxyEnabledForPort(settings.MixedPort); err != nil {
		result = errors.Join(result, fmt.Errorf("读取系统代理失败：%w", err))
	} else if owned {
		if err := DisableSystemProxy(); err != nil {
			result = errors.Join(result, fmt.Errorf("关闭残留系统代理失败：%w", err))
		} else {
			logger.Info("已关闭异常退出后残留的系统代理：127.0.0.1:%d", settings.MixedPort)
		}
	}
	if effectiveTun(settings) {
		cleanupTunInterface(logger)
	}
	if storeErr != nil {
		result = errors.Join(result, fmt.Errorf("读取代理状态失败：%w", storeErr))
	} else {
		app := &App{store: store, logger: logger}
		if err := app.disableStoredProxyEntrances("异常退出恢复"); err != nil {
			result = errors.Join(result, err)
		}
	}
	if result == nil && wasEnabled {
		logger.Info("异常退出残留已清理完成")
	}
	return result
}

func proxyWatchdogAdminHandoffPath(root string, parentPID int) string {
	return filepath.Join(root, "data", "gui", "admin-handoff-"+strconv.Itoa(parentPID)+".flag")
}

func markProxyWatchdogAdminHandoff(root string, parentPID int) error {
	if parentPID <= 0 {
		return errors.New("管理员 TUN 交接进程号无效")
	}
	path := proxyWatchdogAdminHandoffPath(root, parentPID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("tun-handoff\n"), 0600)
}

func consumeProxyWatchdogAdminHandoff(root string, parentPID int) (bool, error) {
	path := proxyWatchdogAdminHandoffPath(root, parentPID)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func killWatchedSingBoxChildren(options proxyWatchdogOptions) error {
	processes, err := listProcesses()
	if err != nil {
		return err
	}
	var result error
	for _, item := range processes {
		if item.PID <= 0 || item.ParentPID != options.ParentPID || !sameExecutablePath(item.Path, options.CorePath) {
			continue
		}
		process, err := os.FindProcess(item.PID)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := process.Kill(); err != nil {
			result = errors.Join(result, fmt.Errorf("结束旧 sing-box 进程 %d 失败：%w", item.PID, err))
		}
	}
	return result
}

func otherGUIProcessRunning(guiPath string, excludedPIDs ...int) (bool, error) {
	excluded := make(map[int]bool, len(excludedPIDs))
	for _, pid := range excludedPIDs {
		excluded[pid] = true
	}
	processes, err := listProcesses()
	if err != nil {
		return false, err
	}
	for _, item := range processes {
		if item.PID > 0 && !excluded[item.PID] && sameExecutablePath(item.Path, guiPath) {
			return true, nil
		}
	}
	return false, nil
}
