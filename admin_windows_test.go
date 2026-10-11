//go:build windows

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveAdminRestartExePrefersStableBuildOverDevExe(t *testing.T) {
	dir := t.TempDir()
	devExe := filepath.Join(dir, "SanTian-Singbox-dev.exe")
	stableExe := filepath.Join(dir, "SanTian-Singbox.exe")
	if err := os.WriteFile(devExe, []byte("dev"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableExe, []byte("stable"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveAdminRestartExe(dir, devExe)
	if got != stableExe {
		t.Fatalf("管理员重启应优先稳定 exe，got %q want %q", got, stableExe)
	}
}

func TestResolveAdminRestartExeFallsBackToCurrentExe(t *testing.T) {
	dir := t.TempDir()
	devExe := filepath.Join(dir, "SanTian-Singbox-dev.exe")
	if err := os.WriteFile(devExe, []byte("dev"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveAdminRestartExe(dir, devExe)
	if got != devExe {
		t.Fatalf("没有稳定 exe 时应回退当前 exe，got %q want %q", got, devExe)
	}
}

func TestAdminRestartScriptWaitsForParentBeforeStart(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "SanTian-Singbox.exe")
	script, err := writeAdminRestartScript(root, exe, adminRestartArg+" "+adminTunHandoffArg, 4321)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	waitIndex := strings.Index(content, "Get-Process -Id 4321")
	startIndex := strings.Index(content, `start ""`)
	if waitIndex < 0 || startIndex < 0 || waitIndex > startIndex {
		t.Fatalf("管理员实例必须等待旧进程退出后再启动：%s", content)
	}
	if !strings.Contains(content, adminTunHandoffArg) {
		t.Fatalf("管理员 TUN 交接参数未写入启动脚本：%s", content)
	}
}

func TestTunReadyMessageIncludesRouteDetail(t *testing.T) {
	got := tunReadyMessage("MISSING_ROUTE:198.18.0.0/15,0.0.0.0/1")
	want := "TUN 默认路由未生效，当前路由：198.18.0.0/15,0.0.0.0/1"
	if got != want {
		t.Fatalf("路由错误应保留实际路由，got %q want %q", got, want)
	}
}

func TestTunReadyPollIntervalStartsFast(t *testing.T) {
	if got := tunReadyPollInterval(12 * time.Second); got != 300*time.Millisecond {
		t.Fatalf("TUN 初始等待应兼顾启动速度与 PowerShell 开销，got %v", got)
	}
	if got := tunReadyPollInterval(7 * time.Second); got != 500*time.Millisecond {
		t.Fatalf("TUN 中段等待应降低轮询开销，got %v", got)
	}
	if got := tunReadyPollInterval(3 * time.Second); got != 800*time.Millisecond {
		t.Fatalf("TUN 末段等待应降低轮询频率，got %v", got)
	}
}

func TestTunIPv4RoutesReadySupportsWindowsSplitRoutes(t *testing.T) {
	routes := []tunIPv4Route{
		{interfaceIndex: 12, destination: net.IPv4(0, 0, 0, 0), prefixLength: 1},
		{interfaceIndex: 12, destination: net.IPv4(128, 0, 0, 0), prefixLength: 1},
	}
	if ok, _ := tunIPv4RoutesReady(routes, 12); !ok {
		t.Fatal("Win10/Win11 的 IPv4 拆分默认路由应判定为 TUN 已就绪")
	}
}

func TestTunIPv4RoutesReadySupportsExcludedNetworkSegments(t *testing.T) {
	routes := []tunIPv4Route{
		{interfaceIndex: 8, destination: net.IPv4(128, 0, 0, 0), prefixLength: 2},
		{interfaceIndex: 8, destination: net.IPv4(192, 0, 0, 0), prefixLength: 3},
		{interfaceIndex: 8, destination: net.IPv4(224, 0, 0, 0), prefixLength: 4},
		{interfaceIndex: 8, destination: net.IPv4(64, 0, 0, 0), prefixLength: 2},
		{interfaceIndex: 8, destination: net.IPv4(32, 0, 0, 0), prefixLength: 3},
	}
	if ok, _ := tunIPv4RoutesReady(routes, 8); !ok {
		t.Fatal("排除局域网后生成的多段公网路由应判定为 TUN 已就绪")
	}
}

func TestTunIPv4RoutesReadyRejectsOtherAdapter(t *testing.T) {
	routes := []tunIPv4Route{{interfaceIndex: 7, destination: net.IPv4zero, prefixLength: 0}}
	if ok, _ := tunIPv4RoutesReady(routes, 8); ok {
		t.Fatal("其他网卡的默认路由不能误判为 SanTian TUN 已就绪")
	}
}

func TestWindowsNetworkAPIsAvailable(t *testing.T) {
	if _, err := windowsIPv4Routes(); err != nil {
		t.Fatalf("Windows 原生路由查询不可用：%v", err)
	}
	adapter, err := findWindowsAdapter("__SanTianMissingAdapter__")
	if err != nil {
		t.Fatalf("Windows 原生网卡查询不可用：%v", err)
	}
	if adapter != nil {
		t.Fatal("不存在的测试网卡不应被识别")
	}
}

func TestCloseStaleFakeIPTCPConnections(t *testing.T) {
	// Should execute safely without panic
	closeStaleFakeIPTCPConnections(nil)
}
