//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	killSwitchMu                 sync.Mutex
	killSwitchActive             bool
	killSwitchRulesVerifiedClean bool

	validFirewallPolicyRE = regexp.MustCompile(`^(?i)(BlockInbound|AllowInbound),(BlockOutbound|AllowOutbound)$`)
	validFirewallProfiles = map[string]bool{
		"domain":  true,
		"private": true,
		"public":  true,
	}

	runFirewallCommand = func(name string, args ...string) (string, error) {
		return runCommandOutput(name, args...)
	}
	checkAdminPrivileges = hasAdminPrivileges
)

func systemBinaryPath(exeName string) string {
	sysRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	return filepath.Join(sysRoot, "System32", exeName)
}

func netshExePath() string {
	return systemBinaryPath("netsh.exe")
}

func sanitizeFirewallPolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	if validFirewallPolicyRE.MatchString(policy) {
		return policy
	}
	return "blockinbound,allowoutbound"
}

func parseFirewallPolicy(output string) string {
	re := regexp.MustCompile(`(?i)(BlockInbound|AllowInbound)\s*,\s*(BlockOutbound|AllowOutbound)`)
	m := re.FindStringSubmatch(output)
	if len(m) >= 3 {
		return m[1] + "," + m[2]
	}
	return ""
}

func queryFirewallPolicy(profile string) string {
	if !validFirewallProfiles[strings.ToLower(strings.TrimSpace(profile))] {
		return "BlockInbound,AllowOutbound"
	}
	out, err := runFirewallCommand(netshExePath(), "advfirewall", "show", profile+"profile", "firewallpolicy")
	if err != nil {
		return "BlockInbound,AllowOutbound"
	}
	policy := parseFirewallPolicy(out)
	if policy == "" {
		return "BlockInbound,AllowOutbound"
	}
	return policy
}

func restoreProfilePolicy(profile, policy string, logger *Logger) error {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if !validFirewallProfiles[profile] {
		return fmt.Errorf("无效防火墙配置档：%q", profile)
	}
	rawPolicy := strings.TrimSpace(policy)
	safePolicy := sanitizeFirewallPolicy(rawPolicy)
	if rawPolicy != "" && !validFirewallPolicyRE.MatchString(rawPolicy) && logger != nil {
		logger.Warn("检测到异常防火墙策略值，已按安全默认值回退：%s", profile)
	}
	if _, err := runFirewallCommand(netshExePath(), "advfirewall", "set", profile+"profile", "firewallpolicy", safePolicy); err != nil {
		if logger != nil {
			logger.Error("恢复 %s 防火墙策略失败: %v", profile, err)
		}
		return fmt.Errorf("恢复 %s 防火墙策略失败: %w", profile, err)
	}
	return nil
}

func restorePolicies(policy KillSwitchProfilePolicy, logger *Logger) error {
	if policy.Domain == "" && policy.Private == "" && policy.Public == "" {
		if logger != nil {
			logger.Warn("Kill Switch 状态文件损坏或字段缺失，回退使用默认策略 blockinbound,allowoutbound 恢复网络")
		}
		_, err := runFirewallCommand(netshExePath(), "advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,allowoutbound")
		return err
	}
	var firstErr error
	for _, p := range []struct {
		name string
		pol  string
	}{
		{"domain", policy.Domain},
		{"private", policy.Private},
		{"public", policy.Public},
	} {
		if err := restoreProfilePolicy(p.name, p.pol, logger); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func hasRule(suffix string) bool {
	ruleName := killSwitchRulePrefix + suffix
	out, err := runFirewallCommand(netshExePath(), "advfirewall", "firewall", "show", "rule", "name="+ruleName)
	return err == nil && strings.Contains(out, ruleName)
}

func hasCoreAppLANRules() bool {
	return hasRule("Core") && hasRule("App") && hasRule("LAN")
}

func hasAnyKillSwitchRule() bool {
	return hasRule("Core") || hasRule("App") || hasRule("LAN") || hasRule("DHCP")
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

	corePath = strings.TrimSpace(corePath)
	if corePath == "" {
		if logger != nil {
			logger.Error("缺少代理核心路径，中止启用 Windows 防火墙阻断以防锁死核心出网")
		}
		return errors.New("缺少代理核心路径")
	}
	appPath = strings.TrimSpace(appPath)
	if appPath == "" {
		appPath, _ = os.Executable()
		appPath = strings.TrimSpace(appPath)
	}
	if appPath == "" {
		return errors.New("缺少主程序路径")
	}

	killSwitchRulesVerifiedClean = false
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

	// 幂等性检查：已激活（killSwitchActive == true，或者状态文件 active=true 且 Core/App/LAN 三条规则都存在）→ 直接返回 nil，不重复操作
	if killSwitchActive || (alreadyActive && hasCoreAppLANRules()) {
		killSwitchActive = true
		return nil
	}

	if !alreadyActive {
		hasOldRules := hasAnyKillSwitchRule()
		warned := false
		sanitize := func(policy string) string {
			parts := strings.Split(policy, ",")
			if len(parts) == 2 && strings.EqualFold(parts[1], "BlockOutbound") && hasOldRules {
				if !warned && logger != nil {
					logger.Warn("检测到旧版 Kill Switch 残留，原始出站策略按放行处理")
					warned = true
				}
				return parts[0] + ",AllowOutbound"
			}
			return policy
		}

		state = KillSwitchState{
			Active:    true,
			CreatedAt: time.Now().Format(time.RFC3339),
			PID:       os.Getpid(),
			OriginalPolicy: KillSwitchProfilePolicy{
				Domain:  sanitize(queryFirewallPolicy("domain")),
				Private: sanitize(queryFirewallPolicy("private")),
				Public:  sanitize(queryFirewallPolicy("public")),
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
	if err := atomicWriteFile(stateFile, stateBytes, 0600); err != nil {
		if logger != nil {
			logger.Error("写入 Kill Switch 状态文件失败，中止阻断以防整机断网: %v", err)
		}
		return err
	}

	// 2. 先清理旧的规则，防止重复堆叠
	_ = removeKillSwitchRules()

	cleanupOnFailure := func(policyChanged bool) {
		_ = removeKillSwitchRules()
		if alreadyActive || policyChanged {
			if rerr := restorePolicies(state.OriginalPolicy, logger); rerr != nil {
				if logger != nil {
					logger.Error("回滚时恢复防火墙策略失败: %v，保留状态文件以供自检恢复", rerr)
				}
				return
			}
		}
		_ = os.Remove(stateFile)
	}

	// 3. 添加白名单放行规则（直接调用 System32\netsh.exe 参数切片，彻底杜绝 cmd /c 注入）
	if _, err := runFirewallCommand(netshExePath(),
		"advfirewall", "firewall", "add", "rule",
		"name="+killSwitchRulePrefix+"Core",
		"dir=out", "action=allow", "program="+corePath, "enable=yes", "profile=any",
	); err != nil {
		cleanupOnFailure(false)
		if logger != nil {
			logger.Error("添加核心放行规则失败: %v，已回滚", err)
		}
		return fmt.Errorf("添加核心放行规则失败: %w", err)
	}

	if _, err := runFirewallCommand(netshExePath(),
		"advfirewall", "firewall", "add", "rule",
		"name="+killSwitchRulePrefix+"App",
		"dir=out", "action=allow", "program="+appPath, "enable=yes", "profile=any",
	); err != nil {
		cleanupOnFailure(false)
		if logger != nil {
			logger.Error("添加主程序放行规则失败: %v，已回滚", err)
		}
		return fmt.Errorf("添加主程序放行规则失败: %w", err)
	}

	if _, err := runFirewallCommand(netshExePath(),
		"advfirewall", "firewall", "add", "rule",
		"name="+killSwitchRulePrefix+"LAN",
		"dir=out", "action=allow",
		"remoteip=127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,::1,fe80::/10",
		"enable=yes", "profile=any",
	); err != nil {
		cleanupOnFailure(false)
		if logger != nil {
			logger.Error("添加局域网放行规则失败: %v，已回滚", err)
		}
		return fmt.Errorf("添加局域网放行规则失败: %w", err)
	}

	if _, err := runFirewallCommand(netshExePath(),
		"advfirewall", "firewall", "add", "rule",
		"name="+killSwitchRulePrefix+"DHCP",
		"dir=out", "action=allow", "protocol=udp", "localport=68", "remoteport=67",
		"enable=yes", "profile=any",
	); err != nil {
		if logger != nil {
			logger.Warn("添加 DHCP 放行规则失败（不阻断流程）: %v", err)
		}
	}

	// 4. 将防火墙默认出站策略设为阻断 (BlockOutbound)
	if _, err := runFirewallCommand(netshExePath(),
		"advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,blockoutbound",
	); err != nil {
		cleanupOnFailure(true)
		if logger != nil {
			logger.Error("设置防火墙出站阻断失败: %v，已回滚", err)
		}
		return err
	}

	killSwitchActive = true
	if logger != nil {
		if stateOut, stateErr := runFirewallCommand(netshExePath(), "advfirewall", "show", "allprofiles", "state"); stateErr == nil && isAnyFirewallProfileOff(stateOut) {
			logger.Warn("【防泄露保护警告】检测到 Windows 防火墙存在已关闭的配置档（State OFF），Kill Switch 出站阻断可能不会生效，建议在系统设置中开启 Windows 防火墙！")
		}
		logger.Info("【防泄露保护 (Kill Switch)】已激活：除代理核心与局域网外，所有直连出站已被防火墙安全阻断！")
	}
	return nil
}

func isAnyFirewallProfileOff(output string) bool {
	re := regexp.MustCompile(`(?im)^\s*(?:State|状态)\s+(?:OFF|关闭)\s*$`)
	return re.MatchString(output)
}

func disableKillSwitch(root string, logger *Logger) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()

	stateFile := killSwitchStatePath(root)
	stateExists := fileExists(stateFile)

	// 内存短路：已核验无残留规则、当前未激活且无状态文件时直接返回，避免 10s 健康巡检空跑 netsh 子进程
	if !stateExists && !killSwitchActive && killSwitchRulesVerifiedClean {
		return nil
	}

	rulesExist := hasAnyKillSwitchRule()

	// 只有状态文件存在、或者能检测到 SanTianKillSwitch_* 规则时才执行恢复；否则同步清除内存状态并直接返回
	if !stateExists && !rulesExist {
		killSwitchActive = false
		killSwitchRulesVerifiedClean = true
		return nil
	}

	if !checkAdminPrivileges() {
		return errors.New("缺少管理员权限")
	}

	// 恢复各 profile 的防火墙策略
	var restoreErr error
	if stateExists {
		var state KillSwitchState
		data, err := os.ReadFile(stateFile)
		if err == nil {
			err = json.Unmarshal(data, &state)
		}
		if err != nil {
			restoreErr = restorePolicies(KillSwitchProfilePolicy{}, logger)
		} else {
			restoreErr = restorePolicies(state.OriginalPolicy, logger)
		}
	} else {
		// 没有状态文件但检测到了规则，回退默认策略
		if logger != nil {
			logger.Warn("未检测到 Kill Switch 状态文件但存在残留规则，回退使用默认策略 blockinbound,allowoutbound 恢复网络")
		}
		restoreErr = restorePolicies(KillSwitchProfilePolicy{}, logger)
	}

	if restoreErr != nil {
		if logger != nil {
			logger.Error("解除 Kill Switch 恢复防火墙策略失败: %v，保留状态文件与放行规则以防彻底断网", restoreErr)
		}
		return restoreErr
	}

	// 全部恢复成功后，才删除规则和状态文件，并把 killSwitchActive 置为 false
	_ = removeKillSwitchRules()
	_ = os.Remove(stateFile)

	wasActive := killSwitchActive
	killSwitchActive = false
	killSwitchRulesVerifiedClean = true
	if wasActive && logger != nil {
		logger.Info("【防泄露保护 (Kill Switch)】已解除，系统网络恢复正常直连。")
	}
	return nil
}

func removeKillSwitchRules() error {
	rules := []string{"Core", "App", "LAN", "DHCP", "DNS"}
	for _, r := range rules {
		ruleName := killSwitchRulePrefix + r
		_, _ = runFirewallCommand(netshExePath(), "advfirewall", "firewall", "delete", "rule", "name="+ruleName)
	}
	return nil
}

func isKillSwitchActive() bool {
	killSwitchMu.Lock()
	defer killSwitchMu.Unlock()
	return killSwitchActive
}
