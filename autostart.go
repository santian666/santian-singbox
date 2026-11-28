package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartName        = "SanTian-Singbox"
	autoStartRunKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
)

func syncAutoStart(enabled bool) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("获取程序路径失败：%w", err)
		}
		if isBootCleanupTestBinary(exe) {
			return nil
		}
		key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartRunKeyPath, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("设置开机自启动失败：%w", err)
		}
		defer key.Close()
		value := fmt.Sprintf(`"%s" %s`, strings.TrimSpace(exe), adminRestartArg)
		if err := key.SetStringValue(autoStartName, value); err != nil {
			return fmt.Errorf("设置开机自启动失败：%w", err)
		}
		return nil
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartRunKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("关闭开机自启动失败：%w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(autoStartName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("关闭开机自启动失败：%w", err)
	}
	return nil
}
