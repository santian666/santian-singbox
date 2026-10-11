package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const autoStartName = "SanTian-Singbox"
const runRegistryPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func syncAutoStart(enabled bool) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("获取程序路径失败：%w", err)
		}
		if strings.HasSuffix(strings.ToLower(exe), "-test.exe") {
			return nil
		}
		value := fmt.Sprintf(`"%s"`, exe)
		if _, err := runCommandOutput("reg", "add", runRegistryPath, "/v", autoStartName, "/t", "REG_SZ", "/d", value, "/f"); err != nil {
			return fmt.Errorf("设置开机自启动失败：%w", err)
		}
		return nil
	}
	if _, err := runCommandOutput("reg", "query", runRegistryPath, "/v", autoStartName); err != nil {
		return nil
	}
	if _, err := runCommandOutput("reg", "delete", runRegistryPath, "/v", autoStartName, "/f"); err != nil {
		return fmt.Errorf("关闭开机自启动失败：%w", err)
	}
	return nil
}
