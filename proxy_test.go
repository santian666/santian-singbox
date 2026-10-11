package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestReadWinINetConnectionProxy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 支持 WinINet")
	}
	if _, err := readWinINetConnectionProxy(); err != nil {
		t.Fatalf("读取 WinINet 默认连接代理失败：%v", err)
	}
}

func TestSystemProxyBypassIncludesLocalNetworks(t *testing.T) {
	bypass := systemProxyBypass()
	for _, item := range []string{"<local>", "localhost", "127.*", "10.*", "172.16.*", "172.31.*", "192.168.*", "169.254.*"} {
		if !strings.Contains(bypass, item) {
			t.Fatalf("系统代理绕过列表缺少 %s：%s", item, bypass)
		}
	}
}

func TestSystemProxyServerUsesCompatibleSingleEndpoint(t *testing.T) {
	if server := systemProxyServer(20808); server != "127.0.0.1:20808" {
		t.Fatalf("系统代理应使用兼容单地址格式：%s", server)
	}
}

func TestSystemProxySettingsMatchRequiresCompleteConfiguration(t *testing.T) {
	want := systemProxySettings{
		enabled: true,
		server:  systemProxyServer(20808),
		bypass:  systemProxyBypass(),
	}
	if !systemProxySettingsMatch(want, 20808) {
		t.Fatal("完整系统代理配置应通过校验")
	}
	want.bypass = "*"
	if systemProxySettingsMatch(want, 20808) {
		t.Fatal("错误绕过列表不应通过校验")
	}
	want.bypass = systemProxyBypass()
	want.autoConfigURL = "http://127.0.0.1/proxy.pac"
	if systemProxySettingsMatch(want, 20808) {
		t.Fatal("残留 PAC 地址不应通过校验")
	}
}

func TestWinINetProxySettingsMatchRequiresDefaultConnection(t *testing.T) {
	want := winINetProxySettings{
		flags:  proxyTypeDirect | proxyTypeProxy,
		server: systemProxyServer(20808),
		bypass: systemProxyBypass(),
	}
	if !winINetProxySettingsMatch(want, 20808) {
		t.Fatal("完整 WinINet 默认连接配置应通过校验")
	}
	want.flags = proxyTypeDirect
	if winINetProxySettingsMatch(want, 20808) {
		t.Fatal("WinINet PROXY 标志关闭时不应通过校验")
	}
	want.flags = proxyTypeDirect | proxyTypeProxy
	want.server = ""
	if winINetProxySettingsMatch(want, 20808) {
		t.Fatal("WinINet 默认连接地址为空时不应通过校验")
	}
}

func TestSystemProxyConfigurationIssueReportsBothClearedLayers(t *testing.T) {
	issue := systemProxyConfigurationIssue(systemProxySettings{}, winINetProxySettings{flags: proxyTypeDirect}, 20808)
	for _, want := range []string{
		"注册表 ProxyEnable=0",
		"注册表 ProxyServer 为空或不匹配",
		"WinINet 默认连接 PROXY 标志关闭",
		"WinINet 默认连接代理地址为空或不匹配",
	} {
		if !strings.Contains(issue, want) {
			t.Fatalf("双层代理被清空时缺少诊断 %q：%s", want, issue)
		}
	}
}

func TestSystemProxyConfigurationIssueAcceptsExactTwoLayerConfiguration(t *testing.T) {
	registrySettings := systemProxySettings{
		enabled: true,
		server:  systemProxyServer(20808),
		bypass:  systemProxyBypass(),
	}
	connectionSettings := winINetProxySettings{
		flags:  proxyTypeDirect | proxyTypeProxy,
		server: systemProxyServer(20808),
		bypass: systemProxyBypass(),
	}
	if issue := systemProxyConfigurationIssue(registrySettings, connectionSettings, 20808); issue != "" {
		t.Fatalf("完整双层系统代理配置不应报错：%s", issue)
	}
}

func TestSystemProxyRASHelperArgsRoundTrip(t *testing.T) {
	wantFlags := uint32(proxyTypeDirect | proxyTypeProxy)
	wantServer := "http=127.0.0.1:20808;socks=127.0.0.1:20808"
	wantBypass := "<local>;127.*"
	args := buildSystemProxyRASHelperArgs(wantFlags, wantServer, wantBypass)
	flags, server, bypass, err := parseSystemProxyRASHelperArgs(args)
	if err != nil {
		t.Fatalf("解析 RAS 辅助进程参数失败：%v", err)
	}
	if flags != wantFlags || server != wantServer || bypass != wantBypass {
		t.Fatalf("RAS 辅助进程参数往返不一致：flags=%d server=%s bypass=%s", flags, server, bypass)
	}
}
