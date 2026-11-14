//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	killSwitchMu     sync.Mutex
	killSwitchActive bool

	runFirewallCommand = func(name string, args ...string) (string, error) {
		return runCommandOutput(name, args...)
	}
	checkAdminPrivileges = hasAdminPrivileges
)

func parseFirewallPolicy(output string) string {
	re := regexp.MustCompile(`(?i)(BlockInbound|AllowInbound)\s*,\s*(BlockOutbound|AllowOutbound)`)
	m := re.FindStringSubmatch(output)
	if len(m) >= 3 {
		return m[1] + "," + m[2]
	}
	return ""
}

func queryFirewallPolicy(profile string) string {
	cmd := fmt.Sprintf("netsh advfirewall show %sprofile firewallpolicy", profile)
	out, err := runFirewallCommand("cmd", "/c", cmd)
	if err != nil {
		return "BlockInbound,AllowOutbound"
	}
	policy := parseFirewallPolicy(out)
	if policy == "" {
		return "BlockInbound,AllowOutbound"
	}
	return policy
}

func restoreProfilePolicy(profile, policy string, logger *Logger) {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		policy = "blockinbound,allowoutbound"
	}
	cmd := fmt.Sprintf("netsh advfirewall set %sprofile firewallpolicy %s", profile, policy)
	if _, err := runFirewallCommand("cmd", "/c", cmd); err != nil && logger != nil {
		logger.Error("恢复 %s 防火墙策略失败: %v", profile, err)
	}
}

func hasAnyKillSwitchRule() bool {
	cmd := fmt.Sprintf(`netsh advfirewall firewall show rule name="%sCore"`, killSwitchRulePrefix)
	out, err := runFirewallCommand("cmd", "/c", cmd)
	if err == nil && strings.Contains(out, killSwitchRulePrefix) {
		return true
	}
	cmdLAN := fmt.Sprintf(`netsh advfirewall firewall show rule name="%sLAN"`, killSwitchRulePrefix)
	outLAN, errLAN := runFirewallCommand("cmd", "/c", cmdLAN)
	if errLAN == nil && strings.Contains(outLAN, killSwitchRulePrefix) {
		return true
	}
	return false
}

func enableKillSwitch(root string, logger *Logger, corePath string, appPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()

	if !checkAdminPrivileges() {
		if logger != nil {
			logger.Warn("当前非管理员权限，无法启用 Windows 防火墙防泄露拦截 (Kill Switch)")
		}
		return errors.New("缺少管理员权限")
	}

	stateFile := killSwitchStatePath(root)

	// 1. 读取并保存原始策略，防止二次开启把“已阻断”覆盖为原始策略
	var state KillSwitchState
	alreadyActive := false
	if data, err := os.ReadFile(stateFile); err == nil {
		if json.Unmarshal(data, &state) == nil && state.Active &&
			(state.OriginalPolicy.Domain != "" || state.OriginalPolicy.Private != "" || state.OriginalPolicy.Public != "") {
			alreadyActive = true
		}
	}

	if !alreadyActive {
		state = KillSwitchState{
			Active:    true,
			CreatedAt: time.Now().Format(time.RFC3339),
			PID:       os.Getpid(),
			OriginalPolicy: KillSwitchProfilePolicy{
				Domain:  queryFirewallPolicy("domain"),
				Private: queryFirewallPolicy("private"),
				Public:  queryFirewallPolicy("public"),
			},
		}
	} else {
		state.PID = os.Getpid()
		state.CreatedAt = time.Now().Format(time.RFC3339)
	}

	stateBytes, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWriteFile(stateFile, stateBytes, 0644); err != nil {
		if logger != nil {
			logger.Error("写入 Kill Switch 状态文件失败，中止阻断以防整机断网: %v", err)
		}
		return err
	}

	// 2. 先清理旧的规则，防止重复堆叠
	_ = removeKillSwitchRules()

	cleanupOnFailure := func() {
		_ = removeKillSwitchRules()
		_ = os.Remove(stateFile)
	}

	// 3. 添加白名单放行规则（Core, App, LAN 必须成功，任意失败即回滚；DHCP 失败记 Warn 不阻断）
	// 放行核心 sing-box.exe
	if corePath != "" {
		cmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sCore" dir=out action=allow program="%s" enable=yes profile=any`, killSwitchRulePrefix, corePath)
		if _, err := runFirewallCommand("cmd", "/c", cmd); err != nil {
			cleanupOnFailure()
			if logger != nil {
				logger.Error("添加核心放行规则失败: %v，已回滚", err)
			}
			return fmt.Errorf("添加核心放行规则失败: %w", err)
		}
	}

	// 放行主程序 SanTian-Singbox.exe
	if appPath == "" {
		appPath, _ = os.Executable()
	}
	if appPath != "" {
		cmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sApp" dir=out action=allow program="%s" enable=yes profile=any`, killSwitchRulePrefix, appPath)
		if _, err := runFirewallCommand("cmd", "/c", cmd); err != nil {
			cleanupOnFailure()
			if logger != nil {
				logger.Error("添加主程序放行规则失败: %v，已回滚", err)
			}
			return fmt.Errorf("添加主程序放行规则失败: %w", err)
		}
	}

	// 放行局域网 & 回环
	lanCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sLAN" dir=out action=allow remoteip=127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,::1,fe80::/10 enable=yes profile=any`, killSwitchRulePrefix)
	if _, err := runFirewallCommand("cmd", "/c", lanCmd); err != nil {
		cleanupOnFailure()
		if logger != nil {
			logger.Error("添加局域网放行规则失败: %v，已回滚", err)
		}
		return fmt.Errorf("添加局域网放行规则失败: %w", err)
	}

	// 放行 DHCP (UDP 67/68) - 失败只记 Warn，不中断流程
	dhcpCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%sDHCP" dir=out action=allow protocol=udp localport=68 remoteport=67 enable=yes profile=any`, killSwitchRulePrefix)
	if _, err := runFirewallCommand("cmd", "/c", dhcpCmd); err != nil {
		if logger != nil {
			logger.Warn("添加 DHCP 放行规则失败（不阻断流程）: %v", err)
		}
	}

	// 4. 将防火墙默认出站策略设为阻断 (BlockOutbound)
	policyCmd := "netsh advfirewall set allprofiles firewallpolicy blockinbound,blockoutbound"
	if _, err := runFirewallCommand("cmd", "/c", policyCmd); err != nil {
		cleanupOnFailure()
		if logger != nil {
			logger.Error("设置防火墙出站阻断失败: %v，已回滚", err)
		}
		return err
	}

	killSwitchActive = true
	if logger != nil {
		logger.Info("【防泄露保护 (Kill Switch)】已激活：除代理核心与局域网外，所有直连出站已被防火墙安全阻断！")
	}
	return nil
}

func disableKillSwitch(root string, logger *Logger) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()

	stateFile := killSwitchStatePath(root)
	stateExists := fileExists(stateFile)
	rulesExist := hasAnyKillSwitchRule()

	// 只有状态文件存在、或者能检测到 SanTianKillSwitch_* 规则时才执行恢复；否则什么都不做、直接返回
	if !stateExists && !rulesExist {
		return nil
	}

	if !checkAdminPrivileges() {
		return errors.New("缺少管理员权限")
	}

	// 恢复各 profile 的防火墙策略
	if stateExists {
		var state KillSwitchState
		data, err := os.ReadFile(stateFile)
		if err == nil {
			err = json.Unmarshal(data, &state)
		}
		if err != nil || (state.OriginalPolicy.Domain == "" && state.OriginalPolicy.Private == "" && state.OriginalPolicy.Public == "") {
			if logger != nil {
				logger.Warn("Kill Switch 状态文件损坏或字段缺失，回退使用默认策略 blockinbound,allowoutbound 恢复网络")
			}
			_, _ = runFirewallCommand("cmd", "/c", "netsh advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound")
		} else {
			restoreProfilePolicy("domain", state.OriginalPolicy.Domain, logger)
			restoreProfilePolicy("private", state.OriginalPolicy.Private, logger)
			restoreProfilePolicy("public", state.OriginalPolicy.Public, logger)
		}
	} else {
		// 没有状态文件但检测到了规则，回退默认策略
		if logger != nil {
			logger.Warn("未检测到 Kill Switch 状态文件但存在残留规则，回退使用默认策略 blockinbound,allowoutbound 恢复网络")
		}
		_, _ = runFirewallCommand("cmd", "/c", "netsh advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound")
	}

	// 删除临时规则与状态文件
	_ = removeKillSwitchRules()
	_ = os.Remove(stateFile)

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
		_, _ = runFirewallCommand("cmd", "/c", cmd)
	}
	return nil
}

func isKillSwitchActive() bool {
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()
	return killSwitchActive
}
