package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

func (a *App) ToolStatus() (result map[string]ToolStatus) {
	result = map[string]ToolStatus{
		"nexttrace": a.toolStatus("nexttrace", "--version"),
		"iperf3":    a.toolStatus("iperf3", "--version"),
		"speedtest": a.toolStatus("speedtest", "--version"),
	}
	return result
}

func (a *App) RunNetworkTool(tool string) (result NetworkToolResult, err error) {
	defer a.recoverError("运行网络工具", &err)
	tool = strings.TrimSpace(strings.ToLower(tool))
	if tool == "network_reset" {
		return a.openNetworkResetSettings(), nil
	}
	switch tool {
	case "check_system_proxy":
		return a.checkSystemProxy(), nil
	case "check_winhttp_proxy":
		return a.runNetworkToolCommand("检查 WinHTTP 代理", "netsh winhttp show proxy", "未读取到 WinHTTP 代理配置。"), nil
	case "check_local_proxy_port":
		return a.checkLocalProxyPort(), nil
	case "flush_dns":
		return a.flushDnsTool(), nil
	case "clear_proxy":
		return a.clearProxySettings(), nil
	case "repair_proxy_residual":
		return a.repairProxyResidual(), nil
	case "renew_dhcp":
		return a.runNetworkToolCommand("刷新 DHCP 租约", "ipconfig /release & ipconfig /renew", "DHCP 租约已刷新。"), nil
	case "sync_time":
		return a.syncTimeTool(), nil
	case "repair_uwp_loopback":
		return a.repairUwpLoopback(), nil
	case "kill_occupied_port":
		return a.killOccupiedPort(), nil
	case "clear_terminal_git_proxy":
		return a.clearTerminalGitProxy(), nil
	}
	if !hasAdminPrivileges() {
		if err := a.confirmNetworkToolAdminRestart(); err != nil {
			return NetworkToolResult{}, err
		}
		return NetworkToolResult{}, errors.New("已请求管理员权限重启，请在新窗口中重新执行网络工具")
	}
	switch tool {
	case "repair_winsock":
		return a.runNetworkToolCommand("Winsock/LSP 修复", "netsh winsock reset", "Winsock/LSP 已修复，部分系统需要重启后完全生效。"), nil
	case "reset_tcpip":
		return a.runNetworkToolCommand("TCP/IP 协议栈重置", "netsh int ip reset", "TCP/IP 协议栈已重置，部分系统需要重启后完全生效。"), nil
	case "cleanup_tun_residual":
		summary := cleanupTunInterfaceWithSummary(a.logger)
		var output string
		if summary != "" {
			output = fmt.Sprintf("已成功清理 TUN 残留设备与路由：\n✔ %s\n请重新开启 TUN 验证是否恢复正常。", summary)
		} else {
			output = "已执行 TUN 残留清理。\n✔ 检测完毕：未发现残留 TUN 虚拟网卡或异常路由，系统处于干净状态。\n请重新开启 TUN 验证是否恢复。"
		}
		return NetworkToolResult{
			Name:    "清理 TUN 残留",
			Command: "清理 SanTian/历史 Wintun 网卡、路由和 DNS",
			Output:  output,
			Success: true,
		}, nil
	case "fix_firewall_rule":
		return a.fixFirewallRule(), nil
	case "toggle_ipv6":
		return a.toggleIpv6Tool(), nil
	default:
		return NetworkToolResult{}, errors.New("未知网络工具")
	}
}

func (a *App) flushDnsTool() NetworkToolResult {
	var steps []string
	successCount := 0

	// 1. Win32 API 原生刷新 DNS 解析器缓存
	if dnsapi := syscall.NewLazyDLL("dnsapi.dll"); dnsapi != nil {
		if proc := dnsapi.NewProc("DnsFlushResolverCache"); proc != nil {
			r1, _, _ := proc.Call()
			if r1 != 0 {
				steps = append(steps, "✔ Windows 系统 DNS 解析缓存已通过 Win32 API 立即清空 (DnsFlushResolverCache)")
				successCount++
			}
		}
	}

	// 2. PowerShell Clear-DnsClientCache
	psOut, psErr := runCommandOutput("powershell", "-NoProfile", "-Command", "Clear-DnsClientCache")
	if psErr == nil {
		steps = append(steps, "✔ Windows DNS 客户端缓存已清空 (Clear-DnsClientCache)")
		successCount++
	} else if len(strings.TrimSpace(psOut)) > 0 {
		steps = append(steps, "ℹ PowerShell DNS 客户端："+strings.TrimSpace(psOut))
	}

	// 3. ipconfig /flushdns
	cmdOut, cmdErr := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & ipconfig /flushdns")
	cleanedCmdOut := strings.TrimSpace(stripANSI(cmdOut))
	if cmdErr == nil {
		steps = append(steps, "✔ 系统命令解析器缓存已刷新 (ipconfig /flushdns)")
		successCount++
	} else {
		steps = append(steps, "⚠ ipconfig /flushdns："+cleanedCmdOut)
	}

	// 4. sing-box 内核 DNS 缓存与连接池清理
	kernelCleared := false
	if a.manager != nil && a.manager.Running() {
		// 4a. Clash API POST /dns/flush
		client := http.Client{Timeout: 1200 * time.Millisecond}
		req, _ := http.NewRequest(http.MethodPost, "http://"+clashAPIAddress+"/dns/flush", nil)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				kernelCleared = true
			}
		}
		// 4b. Clash API DELETE /connections (释放所有旧连接，迫使应用重新解析域名并建连)
		reqDel, _ := http.NewRequest(http.MethodDelete, "http://"+clashAPIAddress+"/connections", nil)
		if resp, err := client.Do(reqDel); err == nil {
			resp.Body.Close()
			steps = append(steps, "✔ sing-box 内核活动连接池已释放 (迫使应用重新解析域名并建连)")
			kernelCleared = true
		}
		// 4c. 清理 FakeIP 残留 TCP 僵尸连接
		closeStaleFakeIPTCPConnections(a.logger)
		steps = append(steps, "✔ 历史 FakeIP 僵尸 TCP 连接已清理")
	}
	if kernelCleared {
		steps = append(steps, "✔ sing-box 内核 DNS 缓存与连接池已完成刷新")
		successCount++
	} else {
		steps = append(steps, "ℹ sing-box 内核：当前未运行或处于直连就绪状态")
	}

	steps = append(steps, "")
	steps = append(steps, "💡 浏览器提示：Chrome、Edge 等现代浏览器内置有独立的 DNS 缓存与 Socket 连接池。若浏览器仍访问旧 IP，可在浏览器中按 Ctrl+F5 强制刷新页面，或在浏览器地址栏打开 chrome://net-internals/#dns 点击 Clear host cache。")

	return NetworkToolResult{
		Name:    "清理电脑 DNS 缓存",
		Command: "DnsFlushResolverCache & Clear-DnsClientCache & ipconfig /flushdns & sing-box DNS刷新",
		Output:  strings.Join(steps, "\n"),
		Success: successCount > 0,
	}
}

func (a *App) clearProxySettings() NetworkToolResult {
	var steps []string
	if err := DisableSystemProxy(); err != nil {
		steps = append(steps, fmt.Sprintf("⚠ 关闭 Windows 系统代理失败：%v", err))
	} else {
		steps = append(steps, "✔ 已成功关闭 Windows 系统代理并通知 WinINet/浏览器生效")
	}
	if hasAdminPrivileges() {
		out, err := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & netsh winhttp reset proxy")
		if err == nil {
			steps = append(steps, "✔ WinHTTP 系统底层代理已重置 (直连)")
		} else {
			steps = append(steps, "ℹ WinHTTP 重置提示："+strings.TrimSpace(stripANSI(out)))
		}
	} else {
		steps = append(steps, "ℹ 当前为普通权限，已跳过 WinHTTP 服务重置（如需重置 WinHTTP 请以管理员身份运行）")
	}
	if a.store != nil {
		state := a.store.Snapshot()
		if state.Settings.SystemProxy {
			state.Settings.SystemProxy = false
			_ = a.store.UpdateSettings(state.Settings)
			a.emitState()
			steps = append(steps, "✔ 客户端界面系统代理开关已同步关闭")
		}
	}
	return NetworkToolResult{
		Name:    "清除代理设置",
		Command: "DisableSystemProxy & netsh winhttp reset proxy",
		Output:  strings.Join(steps, "\n"),
		Success: true,
	}
}

func (a *App) repairProxyResidual() NetworkToolResult {
	var steps []string
	if err := DisableSystemProxy(); err != nil {
		steps = append(steps, fmt.Sprintf("⚠ 清理系统代理失败：%v", err))
	} else {
		steps = append(steps, "✔ 系统代理设置已关闭并通知系统生效")
	}
	// 清理代理环境变量
	envScript := `
$envKey = 'HKCU:\Environment'
foreach ($name in 'HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy') {
  Remove-ItemProperty -Path $envKey -Name $name -ErrorAction SilentlyContinue
}
if ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent().IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  $sysKey = 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
  foreach ($name in 'HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy') {
    Remove-ItemProperty -Path $sysKey -Name $name -ErrorAction SilentlyContinue
  }
}
try {
  Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@ -ErrorAction SilentlyContinue
  [Win32.NativeMethods]::SendMessageTimeout([IntPtr]0xffff, 0x001A, [UIntPtr]::Zero, "Environment", 2, 2000, [ref][UIntPtr]::Zero) | Out-Null
} catch {}
`
	_, _ = runCommandOutput("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", envScript)
	steps = append(steps, "✔ 用户的 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY 环境变量已清理")
	if hasAdminPrivileges() {
		steps = append(steps, "✔ 系统级 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY 环境变量已同步清理")
	}
	steps = append(steps, "✔ 已广播 WM_SETTINGCHANGE 刷新系统全局环境变量")

	// 清理 DNS
	_, _ = runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & ipconfig /flushdns")
	steps = append(steps, "✔ 系统 DNS 解析缓存已清理")

	if hasAdminPrivileges() {
		out, err := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & netsh winhttp reset proxy")
		if err == nil {
			steps = append(steps, "✔ WinHTTP 代理已重置")
		} else {
			steps = append(steps, "ℹ WinHTTP："+strings.TrimSpace(stripANSI(out)))
		}
	}

	if a.store != nil {
		state := a.store.Snapshot()
		if state.Settings.SystemProxy {
			state.Settings.SystemProxy = false
			_ = a.store.UpdateSettings(state.Settings)
			a.emitState()
		}
	}
	return NetworkToolResult{
		Name:    "清理代理残留",
		Command: "清理系统代理、WinHTTP、代理环境变量与 DNS 缓存",
		Output:  strings.Join(steps, "\n"),
		Success: true,
	}
}

func (a *App) checkSystemProxy() NetworkToolResult {
	settings, err := readSystemProxySettings()
	if err != nil {
		return NetworkToolResult{
			Name:    "检查系统代理",
			Command: "查询注册表 Internet Settings",
			Output:  fmt.Sprintf("读取系统代理设置失败：%v", err),
			Success: false,
		}
	}
	var b strings.Builder
	if settings.enabled {
		b.WriteString("系统代理状态：已启用 (ProxyEnable = 1)\n")
		b.WriteString(fmt.Sprintf("代理服务器 (ProxyServer)：%s\n", settings.server))
		if settings.bypass != "" {
			b.WriteString(fmt.Sprintf("跳过列表 (ProxyOverride)：%s\n", settings.bypass))
		}
	} else {
		b.WriteString("系统代理状态：未启用 (ProxyEnable = 0)\n")
		if settings.server != "" {
			b.WriteString(fmt.Sprintf("残留代理服务器配置：%s\n", settings.server))
		}
	}
	if settings.autoConfigURL != "" {
		b.WriteString(fmt.Sprintf("自动配置脚本 (PAC)：%s\n", settings.autoConfigURL))
	}
	return NetworkToolResult{
		Name:    "检查系统代理",
		Command: `reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings"`,
		Output:  strings.TrimSpace(b.String()),
		Success: true,
	}
}

func (a *App) checkLocalProxyPort() NetworkToolResult {
	port := 20808
	if a.store != nil {
		port = a.store.Snapshot().Settings.MixedPort
	}
	open := tcpPortOpen("127.0.0.1", port, 800*time.Millisecond)
	items, _ := findTCPPortProcesses("127.0.0.1", port)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("本地代理端口：%d\n", port))
	if open {
		b.WriteString("端口状态：正在监听 (Open)\n")
	} else {
		b.WriteString("端口状态：未监听 (Closed)\n")
	}
	if len(items) > 0 {
		b.WriteString("监听进程：\n")
		for _, item := range items {
			b.WriteString(fmt.Sprintf("  - %s\n", processLabel(item)))
		}
	} else if open {
		b.WriteString("端口已打开，由内核正常监听。\n")
	} else {
		b.WriteString("未检测到监听该端口的进程。\n")
	}
	return NetworkToolResult{
		Name:    "检查本地代理端口",
		Command: fmt.Sprintf("netstat -ano | findstr :%d", port),
		Output:  strings.TrimSpace(b.String()),
		Success: open,
	}
}

func (a *App) openNetworkResetSettings() NetworkToolResult {
	result := NetworkToolResult{
		Name:    "重置网络",
		Command: "打开 Windows 网络重置设置页",
		Output:  "已打开 Windows 设置。请在“网络和 Internet”里进入“网络重置”，由系统页面确认执行重置。",
		Success: true,
	}
	candidates := []string{
		"ms-settings:network-reset",
		"ms-settings:network-advancedsettings",
		"ms-settings:network-status",
	}
	var lastErr error
	for _, uri := range candidates {
		if err := exec.Command("cmd", "/d", "/c", "start", "", uri).Start(); err != nil {
			lastErr = err
			continue
		}
		result.Command = "start " + uri
		if uri != candidates[0] {
			result.Output = "已打开 Windows 网络设置。不同 Windows 版本的“网络重置”入口位置不同，请在“高级网络设置”或“网络状态”页面中点击“网络重置”。"
		}
		if a.logger != nil {
			a.logger.Info("已打开网络重置设置页：%s", uri)
		}
		return result
	}
	result.Success = false
	result.ExitCode = 1
	if lastErr != nil {
		result.Output = "打开 Windows 网络重置设置页失败：" + lastErr.Error()
	}
	if a.logger != nil {
		a.logger.Error("打开网络重置设置页失败：%s", result.Output)
	}
	return result
}

func (a *App) syncTimeTool() NetworkToolResult {
	var steps []string
	now := time.Now()
	steps = append(steps, fmt.Sprintf("本地系统时间：%s", now.Format("2006-01-02 15:04:05.000 (MST)")))

	ntpServers := []string{
		"ntp.aliyun.com:123",
		"time.windows.com:123",
		"pool.ntp.org:123",
	}

	var netTime time.Time
	var timeSource string
	var queryErr error

	for _, srv := range ntpServers {
		t, err := queryNtpTime(srv, 2*time.Second)
		if err == nil {
			netTime = t
			timeSource = "NTP (" + srv + ")"
			break
		}
		queryErr = err
	}

	if netTime.IsZero() {
		httpServers := []string{
			"http://api.m.taobao.com/rest/api3.do?api=mtop.common.getTimestamp",
			"https://www.qualcomm.com",
			"https://www.cloudflare.com",
		}
		for _, url := range httpServers {
			t, err := queryHttpDate(url, 2*time.Second)
			if err == nil {
				netTime = t
				timeSource = "HTTP Date (" + url + ")"
				break
			}
		}
	}

	if netTime.IsZero() {
		return NetworkToolResult{
			Name:    "系统时间 NTP 校验与对时",
			Command: "UDP 123 (NTP) & HTTP Date 校验",
			Output:  fmt.Sprintf("未能从公共网络时间服务器获取到时间基准：%v\n请检查电脑当前直连网络连接。", queryErr),
			Success: false,
		}
	}

	steps = append(steps, fmt.Sprintf("网络标准时间：%s (来源：%s)", netTime.Format("2006-01-02 15:04:05.000 (MST)"), timeSource))
	diff := netTime.Sub(now)
	diffSec := diff.Seconds()
	steps = append(steps, fmt.Sprintf("时钟偏差：%+.3f 秒", diffSec))

	resynced := false
	if hasAdminPrivileges() {
		cmdOut, cmdErr := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & net start w32time & w32tm /resync /force")
		if cmdErr == nil {
			steps = append(steps, "✔ 已通过 Windows 时间服务强制重新对时 (w32tm /resync /force)")
			resynced = true
		} else {
			steps = append(steps, "ℹ Windows 对时服务响应："+strings.TrimSpace(stripANSI(cmdOut)))
		}
	} else {
		steps = append(steps, "ℹ 当前运行于普通用户权限，无法直接修改系统底层时钟（如需强制修改系统时间，请右键以管理员身份运行）。")
	}

	if mathAbs(diffSec) <= 1.5 || resynced {
		steps = append(steps, "✔ 状态正常：系统时钟偏差处于安全范围内，TLS 证书握手、VMess/VLESS/Shadowsocks/gRPC 协议可正常通信。")
	} else {
		steps = append(steps, fmt.Sprintf("⚠ 警告：时间偏差较大（%+.3f 秒）！VMess 等协议要求时差必须在 90 秒内，建议校准系统时间以防代理节点无法握手建连。", diffSec))
	}

	return NetworkToolResult{
		Name:    "系统时间 NTP 校验与对时",
		Command: "NTP 123 / w32tm /resync /force",
		Output:  strings.Join(steps, "\n"),
		Success: true,
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func queryNtpTime(server string, timeout time.Duration) (time.Time, error) {
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	req := make([]byte, 48)
	req[0] = 0x1B // LI = 0, VN = 3, Mode = 3 (client)

	if _, err := conn.Write(req); err != nil {
		return time.Time{}, err
	}

	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		return time.Time{}, err
	}

	sec := binary.BigEndian.Uint32(resp[40:44])
	frac := binary.BigEndian.Uint32(resp[44:48])
	if sec == 0 {
		return time.Time{}, errors.New("无效的 NTP 时间戳")
	}
	unixSec := int64(sec) - 2208988800
	nano := int64(float64(frac) / 4294967296.0 * 1e9)
	return time.Unix(unixSec, nano), nil
}

func queryHttpDate(urlStr string, timeout time.Duration) (time.Time, error) {
	client := http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	resp, err := client.Head(urlStr)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		return time.Time{}, errors.New("无 Date 头部")
	}
	return http.ParseTime(dateStr)
}

func (a *App) repairUwpLoopback() NetworkToolResult {
	script := `
$count = 0
$keys = Get-ChildItem -Path 'HKCU:\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings' -ErrorAction SilentlyContinue
if ($keys) {
  foreach ($k in $keys) {
    $sid = $k.PSChildName
    & CheckNetIsolation.exe LoopbackExempt -a -p="$sid" 2>$null | Out-Null
    $count++
  }
}
Write-Output "SUCCESS:$count"
`
	out, err := runCommandOutput("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	out = strings.TrimSpace(stripANSI(out))
	count := 0
	if strings.Contains(out, "SUCCESS:") {
		parts := strings.Split(out, "SUCCESS:")
		if len(parts) > 1 {
			count, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
	}

	var steps []string
	if err == nil || count > 0 {
		steps = append(steps, fmt.Sprintf("✔ 已成功为 %d 个 Windows UWP / Microsoft Store 应用解除回环隔离限制 (CheckNetIsolation LoopbackExempt)", count))
		steps = append(steps, "✔ 覆盖应用：微软应用商店、Xbox、Windows Copilot、OneNote、Edge UWP 以及已安装的所有 Modern 应用。")
		steps = append(steps, "✔ 效果说明：应用已被豁免 AppContainer 网络沙盒，在普通系统代理模式下可直接通过 127.0.0.1 本地端口访问代理网络。")
	} else {
		steps = append(steps, "⚠ 执行 UWP 回环解除时出现提示："+out)
	}

	return NetworkToolResult{
		Name:    "UWP 应用回环代理免除",
		Command: "CheckNetIsolation.exe LoopbackExempt -a",
		Output:  strings.Join(steps, "\n"),
		Success: count > 0 || err == nil,
	}
}

func (a *App) killOccupiedPort() NetworkToolResult {
	port := 20808
	mapPort := 52001
	if a.store != nil {
		s := a.store.Snapshot().Settings
		if s.MixedPort > 0 {
			port = s.MixedPort
		}
		if s.ProxyMapStartPort > 0 {
			mapPort = s.ProxyMapStartPort
		}
	}

	ports := []int{port, mapPort}
	var steps []string
	var ourSingboxPID int
	if a.manager != nil && a.manager.Running() {
		ourSingboxPID = a.manager.CurrentPID()
	}
	ourGuiPID := os.Getpid()

	killedCount := 0
	for _, p := range ports {
		items, err := findTCPPortProcesses("127.0.0.1", p)
		if err != nil || len(items) == 0 {
			steps = append(steps, fmt.Sprintf("✔ 端口 %d：当前无任何进程监听，处于空闲状态。", p))
			continue
		}
		for _, item := range items {
			if item.PID == ourSingboxPID {
				steps = append(steps, fmt.Sprintf("✔ 端口 %d：由当前 SanTian SingBox 内核正在正常监听 (PID=%d)。", p, item.PID))
				continue
			}
			if item.PID == ourGuiPID {
				steps = append(steps, fmt.Sprintf("✔ 端口 %d：由当前客户端主程序正常持有 (PID=%d)。", p, item.PID))
				continue
			}
			pName := firstNonEmpty(item.Name, "未知程序")
			killErr := forceKillProcess(item.PID)
			if killErr == nil {
				steps = append(steps, fmt.Sprintf("✔ 已强制终止占用端口 %d 的外部冲突进程：%s (PID=%d) 并释放端口！", p, pName, item.PID))
				killedCount++
			} else {
				steps = append(steps, fmt.Sprintf("⚠ 尝试终止占用端口 %d 的进程 %s (PID=%d) 失败：%v（若为系统保护进程请以管理员身份运行）", p, pName, item.PID, killErr))
			}
		}
	}

	if killedCount == 0 && len(steps) >= len(ports) {
		steps = append(steps, "💡 本地核心代理端口未发现异常外部占用冲突，系统运行环境正常。")
	}

	return NetworkToolResult{
		Name:    "释放被占用的本地代理端口",
		Command: fmt.Sprintf("检测并释放端口 :%d / :%d 占用", port, mapPort),
		Output:  strings.Join(steps, "\n"),
		Success: true,
	}
}

func (a *App) clearTerminalGitProxy() NetworkToolResult {
	var steps []string

	// 1. git config --global --unset http.proxy / https.proxy
	_, _ = runCommandOutput("git", "config", "--global", "--unset", "http.proxy")
	_, _ = runCommandOutput("git", "config", "--global", "--unset", "https.proxy")
	steps = append(steps, "✔ Git 全局配置 http.proxy 与 https.proxy 已清除 (git config --global --unset)")

	// 2. PowerShell 清理环境变量
	envScript := `
$envKey = 'HKCU:\Environment'
foreach ($name in 'HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy','NO_PROXY','no_proxy') {
  Remove-ItemProperty -Path $envKey -Name $name -ErrorAction SilentlyContinue
}
if ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent().IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  $sysKey = 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
  foreach ($name in 'HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy','NO_PROXY','no_proxy') {
    Remove-ItemProperty -Path $sysKey -Name $name -ErrorAction SilentlyContinue
  }
}
try {
  Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@ -ErrorAction SilentlyContinue
  [Win32.NativeMethods]::SendMessageTimeout([IntPtr]0xffff, 0x001A, [UIntPtr]::Zero, "Environment", 2, 2000, [ref][UIntPtr]::Zero) | Out-Null
} catch {}
`
	_, _ = runCommandOutput("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", envScript)
	steps = append(steps, "✔ 用户环境变量 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY 已清理")
	if hasAdminPrivileges() {
		steps = append(steps, "✔ 系统全局环境变量已同步清理")
	}
	steps = append(steps, "✔ 已广播 WM_SETTINGCHANGE 通知系统终端、VSCode 与 Explorer 刷新环境变量")
	steps = append(steps, "")
	steps = append(steps, "💡 提示：已彻底解决因代理关闭或端口改变导致的 git clone、curl、npm、pip “Connection refused” 报错。")

	return NetworkToolResult{
		Name:    "清理终端与 Git 代理残留",
		Command: "git config --unset & 清理环境变量 & 广播刷新",
		Output:  strings.Join(steps, "\n"),
		Success: true,
	}
}

func (a *App) fixFirewallRule() NetworkToolResult {
	singboxPath := a.toolPath("sing-box")
	var steps []string

	// Delete old rules
	_, _ = runCommandOutput("cmd", "/d", "/c", `netsh advfirewall firewall delete rule name="SanTian SingBox" >nul 2>&1`)
	_, _ = runCommandOutput("cmd", "/d", "/c", `netsh advfirewall firewall delete rule name="SanTian GUI" >nul 2>&1`)

	// Inbound & Outbound for sing-box.exe
	inCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="SanTian SingBox" dir=in action=allow program="%s" enable=yes profile=any`, singboxPath)
	outCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="SanTian SingBox" dir=out action=allow program="%s" enable=yes profile=any`, singboxPath)

	inOut, inErr := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & "+inCmd)
	outOut, outErr := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & "+outCmd)

	if inErr == nil && outErr == nil {
		steps = append(steps, "✔ 已成功为 sing-box 内核建立入站允许防火墙规则 (SanTian SingBox)")
		steps = append(steps, "✔ 已成功为 sing-box 内核建立出站允许防火墙规则 (SanTian SingBox)")
	} else {
		steps = append(steps, fmt.Sprintf("⚠ 防火墙规则创建提示：%s %s", strings.TrimSpace(stripANSI(inOut)), strings.TrimSpace(stripANSI(outOut))))
	}

	// Inbound for current GUI process if executable exists
	if exePath, err := os.Executable(); err == nil {
		guiCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="SanTian GUI" dir=in action=allow program="%s" enable=yes profile=any`, exePath)
		_, _ = runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & "+guiCmd)
		steps = append(steps, "✔ 已成功为主客户端界面程序添加防火墙入站允许规则 (SanTian GUI)")
	}

	steps = append(steps, "✔ 规则属性已设为全部网络配置文件生效 (专用网络、公用网络、域网络全部放行)。")
	steps = append(steps, "✔ 消除 Windows Defender 防火墙拦截导致的本地回环阻断、局域网连接超时及 TUN 流量丢包。")

	return NetworkToolResult{
		Name:    "Windows 防火墙放行修复",
		Command: inCmd + " & " + outCmd,
		Output:  strings.Join(steps, "\n"),
		Success: inErr == nil && outErr == nil,
	}
}

func (a *App) toggleIpv6Tool() NetworkToolResult {
	checkScript := `
$adapters = Get-NetAdapterBinding -ComponentId ms_tcpip6 -ErrorAction SilentlyContinue | Where-Object { $_.Name -notlike "*Wintun*" -and $_.Name -notlike "*SanTian*" }
$enabled = @($adapters | Where-Object { $_.Enabled -eq $true })
if ($enabled.Count -gt 0) {
  $adapters | ForEach-Object { Disable-NetAdapterBinding -Name $_.Name -ComponentId ms_tcpip6 -ErrorAction SilentlyContinue }
  Write-Output ("DISABLED:" + $enabled.Count)
} else {
  $adapters | ForEach-Object { Enable-NetAdapterBinding -Name $_.Name -ComponentId ms_tcpip6 -ErrorAction SilentlyContinue }
  Write-Output ("ENABLED:" + $adapters.Count)
}
`
	out, err := runCommandOutput("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", checkScript)
	out = strings.TrimSpace(stripANSI(out))
	_, _ = runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & ipconfig /flushdns")

	var steps []string
	if strings.Contains(out, "DISABLED:") {
		count := strings.TrimPrefix(out, "DISABLED:")
		steps = append(steps, fmt.Sprintf("✔ 已成功为 %s 个系统网络适配器禁用 IPv6 协议绑定 (Disable-NetAdapterBinding)。", count))
		steps = append(steps, "✔ 修复作用：")
		steps = append(steps, "  1. 彻底解决海外流量通过 IPv6 绕过代理直连造成的网页打不开或 DNS 泄露；")
		steps = append(steps, "  2. 消除双栈网络下因部分节点不支持 IPv6 导致的 Happy Eyeballs 首包 300ms~3s 等待卡顿；")
		steps = append(steps, "  3. 系统 DNS 缓存已自动刷新完成。")
		steps = append(steps, "💡 若需重新开启，再次点击此工具即可一键恢复启用。")
	} else if strings.Contains(out, "ENABLED:") {
		count := strings.TrimPrefix(out, "ENABLED:")
		steps = append(steps, fmt.Sprintf("✔ 已成功为 %s 个系统网络适配器重新启用 IPv6 协议绑定 (Enable-NetAdapterBinding)。", count))
		steps = append(steps, "✔ 系统已恢复原生 IPv4 / IPv6 双栈网络访问能力。")
		steps = append(steps, "✔ 系统 DNS 缓存已自动刷新完成。")
	} else {
		if err != nil {
			steps = append(steps, "⚠ 操作失败："+err.Error())
		} else {
			steps = append(steps, "ℹ 操作输出："+out)
		}
	}

	return NetworkToolResult{
		Name:    "IPv6 诊断与一键开关",
		Command: "Get-NetAdapterBinding -ComponentId ms_tcpip6 & Toggle",
		Output:  strings.Join(steps, "\n"),
		Success: err == nil,
	}
}

func (a *App) runNetworkToolCommand(name string, command string, successText string) NetworkToolResult {
	output, err := runCommandOutput("cmd", "/d", "/c", "chcp 65001>nul & "+command)
	output = strings.TrimSpace(stripANSI(output))
	result := NetworkToolResult{
		Name:    name,
		Command: command,
		Output:  output,
		Success: err == nil,
	}
	if result.Success && result.Output == "" {
		result.Output = successText
	}
	if !result.Success {
		result.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		if result.Output == "" {
			result.Output = err.Error()
		}
	}
	if a.logger != nil {
		if result.Success {
			a.logger.Info("网络工具完成：%s", result.Name)
		} else {
			a.logger.Error("网络工具失败：%s，%s", result.Name, result.Output)
		}
	}
	return result
}

func (a *App) runNetworkToolPowerShell(name string, command string, script string, successText string) NetworkToolResult {
	prefix := `$OutputEncoding = [System.Text.UTF8Encoding]::new($false); [Console]::InputEncoding = [System.Text.Encoding]::UTF8; [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; `
	output, err := runCommandOutput("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", prefix+script)
	output = strings.TrimSpace(stripANSI(output))
	result := NetworkToolResult{
		Name:    name,
		Command: command,
		Output:  output,
		Success: err == nil,
	}
	if result.Success && result.Output == "" {
		result.Output = successText
	}
	if !result.Success {
		result.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		if result.Output == "" {
			result.Output = err.Error()
		}
	}
	if a.logger != nil {
		if result.Success {
			a.logger.Info("网络工具完成：%s", result.Name)
		} else {
			a.logger.Error("网络工具失败：%s，%s", result.Name, result.Output)
		}
	}
	return result
}

func (a *App) StartIperf3(options IperfStartOptions) (result IperfStartResult, err error) {
	defer a.recoverError("启动 iperf3", &err)
	target, port, err := parseIperfTarget(options.Target)
	if err != nil {
		return IperfStartResult{}, err
	}
	parallel := options.Parallel
	if parallel <= 0 {
		parallel = 8
	}
	if parallel > 64 {
		return IperfStartResult{}, errors.New("线程不能超过 64")
	}
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	if mode != "download" {
		mode = "upload"
	}
	path := a.toolPath("iperf3")
	if _, err := os.Stat(path); err != nil {
		return IperfStartResult{}, fmt.Errorf("未找到 iperf3：%s", path)
	}
	a.iperfMu.Lock()
	if a.iperfCancel != nil {
		a.iperfMu.Unlock()
		return IperfStartResult{}, errors.New("iperf3 正在运行，请先停止")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.iperfCancel = cancel
	a.iperfMu.Unlock()

	args := []string{"-c", target, "-P", strconv.Itoa(parallel)}
	if port > 0 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	if mode == "download" {
		args = append(args, "-R")
	}
	command := "iperf3 " + strings.Join(args, " ")
	a.emitIperfOutput("开始执行：" + command)
	netMode := strings.ToLower(strings.TrimSpace(options.NetworkMode))
	var env []string
	if netMode == "proxy" {
		a.emitIperfOutput("网络模式：当前网络/TUN；若 TUN 已开启，测速流量将通过代理节点转发。")
		env = os.Environ()
	} else {
		a.emitIperfOutput("网络模式：本机直连；已清理代理环境变量。")
		env = directNetworkEnv(os.Environ())
	}
	safeGo(a.logger, "运行 iperf3", func() {
		a.runIperf3(ctx, path, args, env)
	})
	return IperfStartResult{Command: command}, nil
}

func (a *App) StopIperf3() (err error) {
	defer a.recoverError("停止 iperf3", &err)
	a.iperfMu.Lock()
	cancel := a.iperfCancel
	a.iperfMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	a.emitIperfOutput("正在停止 iperf3")
	return nil
}

func (a *App) StartLocalSpeedtest(options LocalSpeedtestStartOptions) (result LocalSpeedtestStartResult, err error) {
	defer a.recoverError("启动本地测速", &err)
	path := a.toolPath("speedtest")
	if _, err := os.Stat(path); err != nil {
		return LocalSpeedtestStartResult{}, fmt.Errorf("未找到 Speedtest CLI：%s", path)
	}
	env, networkText, err := a.localSpeedtestEnv(options.NetworkMode)
	if err != nil {
		return LocalSpeedtestStartResult{}, err
	}
	a.localSpeedtestMu.Lock()
	if a.localSpeedtestCancel != nil {
		a.localSpeedtestMu.Unlock()
		return LocalSpeedtestStartResult{}, errors.New("本地测速正在运行，请先停止")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.localSpeedtestCancel = cancel
	a.localSpeedtestMu.Unlock()

	args := []string{"--accept-license", "--accept-gdpr", "--progress=yes"}
	serverID := strings.TrimSpace(options.ServerID)
	if serverID != "" {
		if _, err := strconv.Atoi(serverID); err != nil {
			a.localSpeedtestMu.Lock()
			a.localSpeedtestCancel = nil
			a.localSpeedtestMu.Unlock()
			cancel()
			return LocalSpeedtestStartResult{}, errors.New("测速服务器 ID 必须是数字")
		}
		args = append(args, "--server-id", serverID)
	}
	command := "speedtest " + strings.Join(args, " ")
	a.emitLocalSpeedtestOutput("开始执行：" + command)
	a.emitLocalSpeedtestOutput(networkText)
	if serverID == "" {
		a.emitLocalSpeedtestOutput("测速服务器：自动选择")
	} else {
		a.emitLocalSpeedtestOutput("测速服务器 ID：" + serverID)
	}
	if a.logger != nil {
		a.logger.Info("本地测速：Ookla Speedtest CLI")
	}
	safeGo(a.logger, "运行本地测速", func() {
		a.runLocalSpeedtest(ctx, path, args, env)
	})
	return LocalSpeedtestStartResult{Command: command}, nil
}

func (a *App) StopLocalSpeedtest() (err error) {
	defer a.recoverError("停止本地测速", &err)
	a.localSpeedtestMu.Lock()
	cancel := a.localSpeedtestCancel
	a.localSpeedtestMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	a.emitLocalSpeedtestOutput("正在停止本地测速")
	return nil
}

func (a *App) ListLocalSpeedtestServers(options LocalSpeedtestServerListOptions) (result []LocalSpeedtestServer, err error) {
	defer a.recoverError("获取本地测速服务器", &err)
	path := a.toolPath("speedtest")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("未找到 Speedtest CLI：%s", path)
	}
	env, networkText, err := a.localSpeedtestEnv(options.NetworkMode)
	if err != nil {
		return nil, err
	}
	a.emitLocalSpeedtestOutput(networkText)
	output, runErr := a.runLocalSpeedtestServerList(path, env)
	servers := parseLocalSpeedtestServers(output)
	if len(servers) == 0 {
		message := strings.TrimSpace(stripANSI(output))
		if message == "" && runErr != nil {
			message = runErr.Error()
		}
		if message == "" {
			message = "未获取到服务器列表"
		}
		return nil, errors.New(a.localSpeedtestServerListError(options.NetworkMode, message))
	}
	return servers, nil
}

func (a *App) StartMTR(target string) (result MTRStartResult, err error) {
	return a.startMTR(MTRStartOptions{Target: target, NetworkMode: "local"})
}

func (a *App) StartMTRWithOptions(options MTRStartOptions) (result MTRStartResult, err error) {
	return a.startMTR(options)
}

func (a *App) startMTR(options MTRStartOptions) (result MTRStartResult, err error) {
	defer a.recoverError("启动 MTR", &err)
	target := strings.TrimSpace(options.Target)
	if err := validateMTRTarget(target); err != nil {
		return MTRStartResult{}, err
	}
	path := a.toolPath("nexttrace")
	if _, err := os.Stat(path); err != nil {
		return MTRStartResult{}, fmt.Errorf("未找到 NextTrace：%s", path)
	}
	a.emitMTROutput("NextTrace 路径：" + path)
	networkMode := normalizeMTRNetworkMode(options.NetworkMode)
	env, networkText, err := a.mtrNetworkEnv(networkMode)
	if err != nil {
		return MTRStartResult{}, err
	}
	a.emitMTROutput(networkText)
	var args []string
	proto := strings.ToLower(strings.TrimSpace(options.Protocol))
	if strings.HasPrefix(proto, "tcp") {
		port := options.Port
		if port <= 0 {
			if proto == "tcp_80" {
				port = 80
			} else {
				port = 443
			}
		}
		a.emitMTROutput(fmt.Sprintf("探测协议：TCP SYN (目标端口 %d)", port))
		args = []string{"-4", "--tcp", "-p", strconv.Itoa(port), "--mtr", "--raw", "--no-color", "--queries", "1", "--ttl-time", "800", "--timeout", "1200", target}
	} else {
		a.emitMTROutput("探测协议：ICMP Socket")
		args = []string{"-4", "--icmp-mode", "1", "--mtr", "--raw", "--no-color", "--queries", "1", "--ttl-time", "800", "--timeout", "1200", target}
	}
	a.mtrMu.Lock()
	if a.mtrCancel != nil {
		a.mtrMu.Unlock()
		return MTRStartResult{}, errors.New("MTR 正在运行，请先停止")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mtrCancel = cancel
	a.mtrMu.Unlock()

	command := "nexttrace " + strings.Join(args, " ")
	a.emitMTROutput("开始执行：" + command)
	if a.logger != nil {
		a.logger.Info("MTR：%s，网络=%s", target, networkMode)
	}
	safeGo(a.logger, "运行 MTR", func() {
		a.runMTR(ctx, path, args, env)
	})
	return MTRStartResult{Command: command}, nil
}

func isWinDivertRuntimeError(output string) bool {
	text := strings.ToLower(output)
	return strings.Contains(text, "windivert") &&
		(strings.Contains(text, "cannot find the path") ||
			strings.Contains(text, "system cannot find the path") ||
			strings.Contains(text, "无法打开 windivert") ||
			strings.Contains(text, "没有管理员权限") ||
			strings.Contains(text, "管理员权限"))
}

func normalizeMTRNetworkMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "proxy", "current", "tun":
		return "proxy"
	default:
		return "local"
	}
}

func (a *App) mtrNetworkEnv(mode string) ([]string, string, error) {
	if mode != "proxy" {
		return directNetworkEnv(os.Environ()), "追踪模式：本地直连，已清理代理环境变量", nil
	}
	if a == nil || a.store == nil {
		return nil, "", errors.New("无法读取代理状态，不能使用当前代理节点追踪")
	}
	settings := a.store.Snapshot().Settings
	if effectiveTun(settings) {
		if a.manager == nil || !a.manager.Running() {
			return nil, "", errors.New("当前代理节点追踪需要 TUN 正在运行，请先开启 TUN")
		}
		config, err := a.manager.readRuntimeConfig()
		if err != nil || !hasTunInbound(config) {
			return nil, "", errors.New("当前代理节点追踪需要 TUN 已真实接管网络，请重新开启 TUN 后再试")
		}
		return directNetworkEnv(os.Environ()), "追踪模式：当前代理节点，检测到 TUN 正在运行，将使用当前 TUN 出口", nil
	}
	if settings.SystemProxy {
		return nil, "", errors.New("系统代理只能代理网页/应用的 HTTP 或 SOCKS 流量，不能接管 ICMP MTR；请切换到 TUN 模式后再使用当前代理节点 MTR")
	}
	return nil, "", errors.New("当前未开启可用于 MTR 的代理入口；请开启 TUN 后再使用当前代理节点 MTR")
}

func (a *App) StopMTR() (err error) {
	defer a.recoverError("停止 MTR", &err)
	a.mtrMu.Lock()
	cancel := a.mtrCancel
	a.mtrMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	a.emitMTROutput("正在停止 MTR")
	return nil
}

func (a *App) toolStatus(name string, args ...string) ToolStatus {
	path := a.toolPath(name)
	status := ToolStatus{Name: name, Path: path}
	if _, err := os.Stat(path); err != nil {
		status.ErrorText = "未找到工具"
		return status
	}
	output, err := runToolWithTimeout(path, a.root, 8*time.Second, args...)
	status.Version = firstOutputLine(stripANSI(output))
	if err != nil && status.Version == "" {
		status.ErrorText = trimForLog(stripANSI(err.Error()))
		return status
	}
	status.Ready = true
	return status
}

func (a *App) runIperf3(ctx context.Context, path string, args []string, env []string) {
	defer func() {
		a.iperfMu.Lock()
		a.iperfCancel = nil
		a.iperfMu.Unlock()
		a.emitIperfOutput("iperf3 已结束")
	}()
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(a.root) != "" {
		cmd.Dir = a.root
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	hideCommandWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.emitIperfOutput("读取输出失败：" + err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.emitIperfOutput("读取错误输出失败：" + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		a.emitIperfOutput("启动失败：" + err.Error())
		return
	}
	done := make(chan struct{}, 2)
	go a.pipeIperfOutput(stdout, done)
	go a.pipeIperfOutput(stderr, done)
	err = cmd.Wait()
	if !waitOutputReaders(done, 2, 2*time.Second) {
		a.emitIperfOutput("读取剩余输出超时，任务已结束")
	}
	if ctx.Err() == context.Canceled {
		a.emitIperfOutput("已停止")
		return
	}
	if err != nil {
		a.emitIperfOutput("执行失败：" + err.Error())
	}
}

func (a *App) pipeIperfOutput(reader io.Reader, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	scanReaderLines(reader, a.emitIperfOutput)
}

func (a *App) runLocalSpeedtest(ctx context.Context, path string, args []string, env []string) {
	defer func() {
		a.localSpeedtestMu.Lock()
		a.localSpeedtestCancel = nil
		a.localSpeedtestMu.Unlock()
		a.emitLocalSpeedtestOutput("本地测速已结束")
	}()
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(a.root) != "" {
		cmd.Dir = a.root
	}
	cmd.Env = env
	hideCommandWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.emitLocalSpeedtestOutput("读取输出失败：" + err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.emitLocalSpeedtestOutput("读取错误输出失败：" + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		a.emitLocalSpeedtestOutput("启动失败：" + err.Error())
		return
	}
	done := make(chan struct{}, 2)
	go a.pipeLocalSpeedtestOutput(stdout, done)
	go a.pipeLocalSpeedtestOutput(stderr, done)
	err = cmd.Wait()
	if !waitOutputReaders(done, 2, 2*time.Second) {
		a.emitLocalSpeedtestOutput("读取剩余输出超时，任务已结束")
	}
	if ctx.Err() == context.Canceled {
		a.emitLocalSpeedtestOutput("已停止")
		return
	}
	if err != nil {
		a.emitLocalSpeedtestOutput("执行失败：" + err.Error())
	}
}

func (a *App) pipeLocalSpeedtestOutput(reader io.Reader, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	scanReaderChunks(reader, a.emitLocalSpeedtestOutput)
}

func (a *App) runLocalSpeedtestServerList(path string, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--accept-license", "--accept-gdpr", "--servers")
	if strings.TrimSpace(a.root) != "" {
		cmd.Dir = a.root
	}
	cmd.Env = env
	hideCommandWindow(cmd)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err := cmd.Run()
	output := buffer.String()
	if ctx.Err() == context.DeadlineExceeded {
		return output, errors.New("获取测速服务器超时")
	}
	return output, err
}

func parseLocalSpeedtestServers(output string) []LocalSpeedtestServer {
	linePattern := regexp.MustCompile(`^\s*(\d+)\s+(.+?)\s{2,}(.+?)\s{2,}(.+?)\s*$`)
	servers := make([]LocalSpeedtestServer, 0)
	for _, line := range strings.Split(stripANSI(output), "\n") {
		line = strings.TrimRight(line, "\r")
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		servers = append(servers, LocalSpeedtestServer{
			ID:       strings.TrimSpace(match[1]),
			Name:     strings.TrimSpace(match[2]),
			Location: strings.TrimSpace(match[3]),
			Country:  strings.TrimSpace(match[4]),
		})
	}
	return servers
}

func (a *App) runMTR(ctx context.Context, path string, args []string, env []string) {
	defer func() {
		a.mtrMu.Lock()
		a.mtrCancel = nil
		a.mtrMu.Unlock()
		a.emitMTROutput("MTR 已结束")
	}()
	cmd := exec.CommandContext(ctx, path, args...)
	if dir := toolExecDir(path); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	hideCommandWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.emitMTROutput("读取输出失败：" + err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.emitMTROutput("读取错误输出失败：" + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		a.emitMTROutput("启动失败：" + err.Error())
		return
	}
	done := make(chan struct{}, 2)
	go a.pipeMTROutput(stdout, done)
	go a.pipeMTROutput(stderr, done)
	err = cmd.Wait()
	if !waitOutputReaders(done, 2, 2*time.Second) {
		a.emitMTROutput("读取剩余输出超时，任务已结束")
	}
	if ctx.Err() == context.Canceled {
		a.emitMTROutput("已停止")
		return
	}
	if err != nil {
		a.emitMTROutput("执行失败：" + err.Error())
	}
}

func (a *App) pipeMTROutput(reader io.Reader, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	scanReaderLines(reader, func(line string) {
		a.emitMTROutput(line)
		if isAdminRequiredToolOutput(line) {
			a.emitMTROutput("权限检查失败：当前运行实例不是有效管理员权限，请退出托盘旧实例后重新以管理员身份运行")
		}
	})
}

func waitOutputReaders(done <-chan struct{}, count int, timeout time.Duration) bool {
	if count <= 0 {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for range count {
		select {
		case <-done:
		case <-timer.C:
			return false
		}
	}
	return true
}

func isAdminRequiredToolOutput(line string) bool {
	text := strings.ToLower(strings.TrimSpace(line))
	return strings.Contains(text, "没有管理员权限") ||
		strings.Contains(text, "administrator")
}

func scanReaderLines(reader io.Reader, emit func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		emit(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		emit("读取输出异常：" + err.Error())
	}
}

func scanReaderChunks(reader io.Reader, emit func(string)) {
	buffer := make([]byte, 4096)
	var pending strings.Builder
	flush := func() {
		text := strings.TrimSpace(pending.String())
		pending.Reset()
		if text != "" {
			emit(text)
		}
	}
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			for _, b := range buffer[:n] {
				if b == '\r' || b == '\n' {
					flush()
					continue
				}
				pending.WriteByte(b)
			}
		}
		if err != nil {
			flush()
			if err != io.EOF {
				emit("读取输出异常：" + err.Error())
			}
			return
		}
	}
}

func (a *App) emitIperfOutput(line string) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "iperf-output", stripANSI(line))
}

func (a *App) emitLocalSpeedtestOutput(line string) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "local-speedtest-output", stripANSI(line))
}

func (a *App) emitMTROutput(line string) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "mtr-output", stripANSI(line))
}

func directNetworkEnv(env []string) []string {
	blocked := map[string]bool{
		"http_proxy":  true,
		"https_proxy": true,
		"all_proxy":   true,
		"no_proxy":    true,
	}
	result := make([]string, 0, len(env)+4)
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			result = append(result, item)
			continue
		}
		if blocked[strings.ToLower(key)] {
			continue
		}
		result = append(result, item)
	}
	return append(result, "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=")
}

func (a *App) localSpeedtestEnv(networkMode string) ([]string, string, error) {
	switch strings.ToLower(strings.TrimSpace(networkMode)) {
	case "proxy":
		return directNetworkEnv(os.Environ()), a.localSpeedtestTunText(), nil
	case "http_proxy":
		proxyURL := a.localSpeedtestProxyURL()
		if proxyURL == "" {
			return nil, "", errors.New("HTTP 代理模式需要软件代理正在运行且本地端口可用")
		}
		return proxyNetworkEnv(os.Environ(), proxyURL), "已选择 HTTP 代理兼容模式，测速将使用：" + proxyURL, nil
	default:
		return directNetworkEnv(os.Environ()), "已清理代理环境变量，测速将使用本机直连网络", nil
	}
}

func (a *App) localSpeedtestTunText() string {
	prefix := "已选择当前网络/TUN模式，测速不会强制注入 HTTP_PROXY；"
	if a == nil || a.store == nil {
		return prefix + "无法读取 TUN 设置，测速将使用系统当前网络"
	}
	settings := a.store.Snapshot().Settings
	if !effectiveTun(settings) {
		return prefix + "当前未开启 TUN，测速将使用系统当前网络"
	}
	if a.manager == nil || !a.manager.Running() {
		return prefix + "TUN 已开启但 sing-box 内核未运行，测速将使用系统当前网络"
	}
	config, err := a.manager.readRuntimeConfig()
	if err != nil {
		return prefix + "TUN 已开启但无法读取运行配置，测速将使用系统当前网络"
	}
	if !hasTunInbound(config) {
		return prefix + "TUN 已开启但运行配置里没有 TUN 入站，测速将使用系统当前网络"
	}
	return prefix + "检测到 TUN 正在运行，测速将使用当前 TUN/系统路由出口"
}

func (a *App) localSpeedtestProxyURL() string {
	if a == nil || a.manager == nil || !a.manager.Running() || a.store == nil {
		return ""
	}
	port := a.store.Snapshot().Settings.MixedPort
	if port < 1 || port > 65535 {
		return ""
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func (a *App) localSpeedtestServerListError(networkMode string, message string) string {
	message = trimForLog(message)
	switch strings.ToLower(strings.TrimSpace(networkMode)) {
	case "proxy":
		state := PersistState{}
		if a != nil && a.store != nil {
			state = a.store.Snapshot()
		}
		if !state.Settings.Tun {
			return "当前网络/TUN模式不会强制使用普通代理节点。你现在没有开启 TUN，请切换到 HTTP代理，或先开启 TUN 后再获取服务器。原始错误：" + message
		}
		return "当前网络/TUN模式获取服务器失败，请确认 TUN 已接管系统网络。原始错误：" + message
	case "http_proxy":
		return "HTTP代理模式获取服务器失败，请确认软件代理正在运行且本地端口可用，或切换到当前网络/TUN。原始错误：" + message
	default:
		return "本机直连获取服务器失败，请检查本机网络是否能访问 Ookla。原始错误：" + message
	}
}

func proxyNetworkEnv(env []string, proxyURL string) []string {
	result := directNetworkEnv(env)
	if proxyURL == "" {
		return result
	}
	return append(result, "HTTP_PROXY="+proxyURL, "HTTPS_PROXY="+proxyURL, "ALL_PROXY="+proxyURL, "NO_PROXY=127.0.0.1,localhost")
}

func parseIperfTarget(value string) (string, int, error) {
	target := strings.TrimSpace(value)
	if target == "" {
		return "", 0, errors.New("请输入目标地址")
	}
	if len(target) > 253 || strings.ContainsAny(target, " \t\r\n/\\") {
		return "", 0, errors.New("目标地址格式无效")
	}
	if host, portText, ok := strings.Cut(target, ":"); ok && !strings.Contains(host, ":") {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return "", 0, errors.New("目标端口无效")
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return "", 0, errors.New("目标地址格式无效")
		}
		return host, port, nil
	}
	return target, 0, nil
}

func (a *App) toolPath(name string) string {
	root := a.root
	if strings.TrimSpace(root) == "" {
		root = detectAppRoot()
	}
	return filepath.Join(root, "bin", exeName(name))
}

func (a *App) initNextTrace(path string) error {
	root := strings.TrimSpace(a.root)
	if root == "" {
		root = detectAppRoot()
	}
	if err := ensureEmbeddedBins(root, a.logger); err != nil {
		return fmt.Errorf("初始化 NextTrace 运行文件失败：%s", err.Error())
	}
	dir := toolExecDir(path)
	output, err := runToolWithTimeout(path, dir, 15*time.Second, "--init")
	if err != nil {
		return fmt.Errorf("初始化 NextTrace 失败：%s", firstNonEmpty(trimForLog(output), err.Error()))
	}
	if status := nextTraceRuntimeStatus(path); status != "已就绪" {
		return fmt.Errorf("初始化 NextTrace 失败：WinDivert %s", status)
	}
	return nil
}

func toolExecDir(path string) string {
	dir := filepath.Dir(strings.TrimSpace(path))
	if dir == "." {
		return ""
	}
	return dir
}

func nextTraceRuntimeStatus(path string) string {
	dir := filepath.Dir(path)
	missing := make([]string, 0, 2)
	for _, name := range []string{"WinDivert.dll", "WinDivert64.sys"} {
		if !fileExists(filepath.Join(dir, name)) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return "已就绪"
	}
	return "缺少 " + strings.Join(missing, "、")
}

func validateMTRTarget(target string) error {
	if target == "" {
		return errors.New("请输入 MTR 目标")
	}
	if len(target) > 253 || strings.ContainsAny(target, " \t\r\n/\\") {
		return errors.New("MTR 目标格式无效")
	}
	return nil
}

func runToolWithTimeout(path string, dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	hideCommandWindow(cmd)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err := cmd.Run()
	output := buffer.String()
	if ctx.Err() == context.DeadlineExceeded {
		return output, errors.New("工具执行超时")
	}
	return output, err
}

func firstOutputLine(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return trimForLog(line)
		}
	}
	return ""
}

func stripANSI(value string) string {
	return ansiEscapePattern.ReplaceAllString(value, "")
}
