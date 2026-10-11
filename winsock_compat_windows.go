//go:build windows

package main

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func detectIncompatibleWinsockProvider() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "netsh", "winsock", "show", "catalog")
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	return incompatibleWinsockProvider(string(output)), nil
}

func showIncompatibleWinsockProvider(provider string) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return
	}
	message := "检测到 " + provider + "。\r\n\r\n" +
		"该 Astrill 模块会注入 SanTian，并可能造成原生崩溃和孤立代理进程。\r\n\r\n" +
		"请先正常卸载 Astrill/AstrillTun，以管理员身份执行 netsh winsock reset，重启电脑后再运行 SanTian。\r\n" +
		"不要直接删除 ASProxy64.dll。"
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("SanTian 网络兼容性阻断")
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SYSTEMMODAL)
}
