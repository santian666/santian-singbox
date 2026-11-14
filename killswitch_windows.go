package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

var (
	killSwitchMu     sync.Mutex
	killSwitchActive bool
)

const killSwitchRulePrefix = "SanTianKillSwitch_"

func enableKillSwitch(logger *Logger, corePath string, appPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()

	if !hasAdminPrivileges() {
		if logger != nil {
			logger.Warn("当前非管理员权限，无法启用 Windows 防火墙防泄露拦截 (Kill Switch)")
		}
		return nil
	}

	// 1. 先清理旧的规则，防止重复堆叠
	_ = removeKillSwitchRules()

	// 2. 添加白名单放行规则
	// 放行核心 sing-box.exe
	if corePath != "" && fileExists(corePath) {
		cmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sCore" dir=out action=allow program="%s" enable=yes profile=any`, killSwitchRulePrefix, corePath)
		_, _ = runCommandOutput("cmd", "/c", cmd)
	}
	// 放行主程序 SanTian-Singbox.exe
	if appPath == "" {
		appPath, _ = os.Executable()
	}
	if appPath != "" && fileExists(appPath) {
		cmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sApp" dir=out action=allow program="%s" enable=yes profile=any`, killSwitchRulePrefix, appPath)
		_, _ = runCommandOutput("cmd", "/c", cmd)
	}
	// 放行局域网 & 回环
	lanCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sLAN" dir=out action=allow remoteip=127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,::1,fe80::/10 enable=yes profile=any`, killSwitchRulePrefix)
	_, _ = runCommandOutput("cmd", "/c", lanCmd)

	// 放行 DHCP (UDP 67/68)
	dhcpCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sDHCP" dir=out action=allow protocol=udp localport=68 remoteport=67 enable=yes profile=any`, killSwitchRulePrefix)
	_, _ = runCommandOutput("cmd", "/c", dhcpCmd)

	// 放行本地 DNS (UDP 53)
	dnsCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sDNS" dir=out action=allow protocol=udp remoteport=53 enable=yes profile=any`, killSwitchRulePrefix)
	_, _ = runCommandOutput("cmd", "/c", dnsCmd)

	// 3. 将防火墙默认出站策略设为阻断 (BlockOutbound)
	policyCmd := "netsh advfirewall set allprofiles firewallpolicy blockinbound,blockoutbound"
	if _, err := runCommandOutput("cmd", "/c", policyCmd); err != nil {
		if logger != nil {
			logger.Error("设置防火墙出站阻断失败: %v", err)
		}
		return err
	}

	killSwitchActive = true
	if logger != nil {
		logger.Info("【防泄露保护 (Kill Switch)】已激活：除代理核心与局域网外，所有直连出站已被防火墙安全阻断！")
	}
	return nil
}

func disableKillSwitch(logger *Logger) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()

	if !hasAdminPrivileges() {
		return nil
	}

	// 1. 恢复防火墙默认出站策略为放行 (AllowOutbound)
	policyCmd := "netsh advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound"
	_, _ = runCommandOutput("cmd", "/c", policyCmd)

	// 2. 清理临时白名单规则
	_ = removeKillSwitchRules()

	wasActive := killSwitchActive
	killSwitchActive = false
	if wasActive && logger != nil {
		logger.Info("【防泄露保护 (Kill Switch)】已解除，系统网络恢复正常直连。")
	}
	return nil
}

func removeKillSwitchRules() error {
	rules := []string{"Core", "App", "LAN", "DHCP", "DNS"}
	for _, r := range rules {
		ruleName := killSwitchRulePrefix + r
		cmd := fmt.Sprintf(`netsh advfirewall firewall delete rule name="%s"`, ruleName)
		_, _ = runCommandOutput("cmd", "/c", cmd)
	}
	return nil
}

func isKillSwitchActive() bool {
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()
	return killSwitchActive
}
