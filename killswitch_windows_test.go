//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetFirewallMocks() {
	runFirewallCommand = func(name string, args ...string) (string, error) {
		return runCommandOutput(name, args...)
	}
	checkAdminPrivileges = hasAdminPrivileges
}

// 1. enable 前读取策略并写状态文件；状态文件写入失败 → 不执行 blockoutbound
func TestKillSwitchEnableStateWriteFailureAbortsBlock(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	var executedCommands []string
	var mu sync.Mutex

	runFirewallCommand = func(name string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		full := strings.Join(append([]string{name}, args...), " ")
		executedCommands = append(executedCommands, full)
		if strings.Contains(full, "show") && strings.Contains(full, "firewallpolicy") {
			return "BlockInbound,AllowOutbound", nil
		}
		return "", nil
	}

	// 传入非法根目录路径，触发 atomicWriteFile 创建目录/文件失败
	invalidRoot := "Z:\\NonExistentPath_InvalidRoot_123456\x00invalid"
	err := enableKillSwitch(invalidRoot, nil, "C:\\mock\\sing-box.exe", "C:\\mock\\app.exe")
	if err == nil {
		t.Fatal("状态文件写入失败时 enableKillSwitch 应当返回错误")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range executedCommands {
		if strings.Contains(cmd, "blockoutbound") {
			t.Fatalf("状态文件写入失败时不应执行 blockoutbound，实际执行了：%s", cmd)
		}
	}
}

// 2. 已有 active=true 状态文件时再次 enable，originalPolicy 不被覆盖
func TestKillSwitchEnableDoesNotOverwriteOriginalPolicy(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	tempDir := t.TempDir()

	initialState := KillSwitchState{
		Active:    true,
		CreatedAt: "2026-01-01T00:00:00Z",
		PID:       1111,
		OriginalPolicy: KillSwitchProfilePolicy{
			Domain:  "AllowInbound,AllowOutbound",
			Private: "BlockInbound,CustomPrivate",
			Public:  "BlockInbound,CustomPublic",
		},
	}
	statePath := killSwitchStatePath(tempDir)
	stateBytes, _ := json.MarshalIndent(initialState, "", "  ")
	_ = os.MkdirAll(filepath.Dir(statePath), 0755)
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("写入测试初始状态文件失败: %v", err)
	}

	runFirewallCommand = func(name string, args ...string) (string, error) {
		full := strings.Join(append([]string{name}, args...), " ")
		// 模拟此时策略已经被设置成了 BlockOutbound
		if strings.Contains(full, "show") && strings.Contains(full, "firewallpolicy") {
			return "BlockInbound,BlockOutbound", nil
		}
		return "", nil
	}

	if err := enableKillSwitch(tempDir, nil, "C:\\mock\\sing-box.exe", "C:\\mock\\app.exe"); err != nil {
		t.Fatalf("再次 enableKillSwitch 失败: %v", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("读取状态文件失败: %v", err)
	}
	var loadedState KillSwitchState
	if err := json.Unmarshal(data, &loadedState); err != nil {
		t.Fatalf("反序列化状态文件失败: %v", err)
	}

	if loadedState.OriginalPolicy.Domain != "AllowInbound,AllowOutbound" ||
		loadedState.OriginalPolicy.Private != "BlockInbound,CustomPrivate" ||
		loadedState.OriginalPolicy.Public != "BlockInbound,CustomPublic" {
		t.Fatalf("二次 enable 不应覆盖已有的原始策略，实际：%+v", loadedState.OriginalPolicy)
	}
}

// 3. Core 规则失败 → 不执行 blockoutbound，返回 error 并清理状态
func TestKillSwitchEnableCoreRuleFailureAbortsAndCleansUp(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	tempDir := t.TempDir()

	var executedCommands []string
	var mu sync.Mutex

	runFirewallCommand = func(name string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		full := strings.Join(append([]string{name}, args...), " ")
		executedCommands = append(executedCommands, full)
		if strings.Contains(full, "SanTianKillSwitch_Core") && strings.Contains(full, "add rule") {
			return "", errors.New("mock firewall core rule failure")
		}
		return "", nil
	}

	err := enableKillSwitch(tempDir, nil, "C:\\mock\\sing-box.exe", "C:\\mock\\app.exe")
	if err == nil {
		t.Fatal("Core 规则失败时 enableKillSwitch 应当返回错误")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range executedCommands {
		if strings.Contains(cmd, "blockoutbound") {
			t.Fatalf("Core 规则失败时不应设置 blockoutbound，实际执行了：%s", cmd)
		}
	}

	statePath := killSwitchStatePath(tempDir)
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("Core 规则失败后状态文件应当被删除清理，实际仍存在：%s", statePath)
	}
}

// 4. disable 在没有状态文件、没有规则时不执行任何 set ... firewallpolicy 命令
func TestKillSwitchDisableNoOpWhenNoStateAndNoRules(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	tempDir := t.TempDir()

	var executedCommands []string
	var mu sync.Mutex

	runFirewallCommand = func(name string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		full := strings.Join(append([]string{name}, args...), " ")
		executedCommands = append(executedCommands, full)
		if strings.Contains(full, "show rule") {
			return "No rules match the specified criteria.", errors.New("rule not found")
		}
		return "", nil
	}

	if err := disableKillSwitch(tempDir, nil); err != nil {
		t.Fatalf("无状态文件和规则时 disableKillSwitch 不应报错：%v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range executedCommands {
		if strings.Contains(cmd, "firewallpolicy") && strings.Contains(cmd, "set") {
			t.Fatalf("无状态与规则时不应执行任何 set firewallpolicy，实际执行了：%s", cmd)
		}
	}
}

// 5. disable 按状态文件恢复各 profile 的原始策略；状态文件损坏 → 回退默认值
func TestKillSwitchDisableRestoresProfilesOrFallsBack(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }

	// A: 正常按状态文件恢复各 profile
	tempDirA := t.TempDir()
	stateA := KillSwitchState{
		Active: true,
		OriginalPolicy: KillSwitchProfilePolicy{
			Domain:  "AllowInbound,AllowOutbound",
			Private: "BlockInbound,AllowOutbound",
			Public:  "BlockInbound,BlockOutbound",
		},
	}
	statePathA := killSwitchStatePath(tempDirA)
	_ = os.MkdirAll(filepath.Dir(statePathA), 0755)
	bytesA, _ := json.Marshal(stateA)
	_ = os.WriteFile(statePathA, bytesA, 0644)

	var cmdsA []string
	var muA sync.Mutex
	runFirewallCommand = func(name string, args ...string) (string, error) {
		muA.Lock()
		defer muA.Unlock()
		cmdsA = append(cmdsA, strings.Join(append([]string{name}, args...), " "))
		return "", nil
	}

	if err := disableKillSwitch(tempDirA, nil); err != nil {
		t.Fatalf("disableKillSwitch A 失败：%v", err)
	}

	hasDomain := false
	hasPrivate := false
	hasPublic := false
	muA.Lock()
	for _, c := range cmdsA {
		if strings.Contains(c, "set domainprofile firewallpolicy AllowInbound,AllowOutbound") {
			hasDomain = true
		}
		if strings.Contains(c, "set privateprofile firewallpolicy BlockInbound,AllowOutbound") {
			hasPrivate = true
		}
		if strings.Contains(c, "set publicprofile firewallpolicy BlockInbound,BlockOutbound") {
			hasPublic = true
		}
	}
	muA.Unlock()

	if !hasDomain || !hasPrivate || !hasPublic {
		t.Fatalf("应按各 profile 恢复原始策略，实际执行的命令：%v", cmdsA)
	}

	// B: 状态文件损坏 → 回退 allprofiles blockinbound,allowoutbound
	tempDirB := t.TempDir()
	statePathB := killSwitchStatePath(tempDirB)
	_ = os.MkdirAll(filepath.Dir(statePathB), 0755)
	_ = os.WriteFile(statePathB, []byte("{invalid-json"), 0644)

	var cmdsB []string
	var muB sync.Mutex
	runFirewallCommand = func(name string, args ...string) (string, error) {
		muB.Lock()
		defer muB.Unlock()
		cmdsB = append(cmdsB, strings.Join(append([]string{name}, args...), " "))
		return "", nil
	}

	if err := disableKillSwitch(tempDirB, nil); err != nil {
		t.Fatalf("损坏状态文件下 disableKillSwitch 不应失败：%v", err)
	}

	hasFallback := false
	muB.Lock()
	for _, c := range cmdsB {
		if strings.Contains(c, "set allprofiles firewallpolicy blockinbound,allowoutbound") {
			hasFallback = true
			break
		}
	}
	muB.Unlock()

	if !hasFallback {
		t.Fatalf("损坏状态文件时应回退到 allprofiles blockinbound,allowoutbound，实际命令：%v", cmdsB)
	}
}

// 6. 生成的命令中不再包含 remoteport=53 的全局放行
func TestKillSwitchEnableDoesNotContainGlobalDNSAllow(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	tempDir := t.TempDir()

	var executedCommands []string
	var mu sync.Mutex

	runFirewallCommand = func(name string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		full := strings.Join(append([]string{name}, args...), " ")
		executedCommands = append(executedCommands, full)
		return "", nil
	}

	if err := enableKillSwitch(tempDir, nil, "C:\\mock\\sing-box.exe", "C:\\mock\\app.exe"); err != nil {
		t.Fatalf("enableKillSwitch 失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range executedCommands {
		if strings.Contains(cmd, "remoteport=53") {
			t.Fatalf("不得再生成针对所有目标放行 remoteport=53 的规则，发现命令：%s", cmd)
		}
	}
}

// 7. shouldApplyFirewallKillSwitch：TUN 开 → false；系统代理 + KillSwitch → true；熔断 + 非 TUN → true；熔断 + TUN → false
func TestShouldApplyFirewallKillSwitchConditions(t *testing.T) {
	// 1. TUN 开 → false
	sTun := Settings{Tun: true, KillSwitch: true}
	if shouldApplyFirewallKillSwitch(sTun, false) {
		t.Fatal("TUN 开启时 shouldApplyFirewallKillSwitch 必须返回 false")
	}

	// 2. 系统代理 + KillSwitch → true
	sSys := Settings{SystemProxy: true, KillSwitch: true}
	if !shouldApplyFirewallKillSwitch(sSys, false) {
		t.Fatal("系统代理开启且 KillSwitch 为 true 时应当返回 true")
	}

	// 3. 熔断 + 非 TUN → true
	sDirect := Settings{SystemProxy: false, Tun: false, KillSwitch: false}
	if !shouldApplyFirewallKillSwitch(sDirect, true) {
		t.Fatal("非 TUN 模式触发智能熔断时应当返回 true")
	}

	// 4. 熔断 + TUN → false
	if shouldApplyFirewallKillSwitch(sTun, true) {
		t.Fatal("TUN 模式下即便触发智能熔断也必须返回 false（由 strict_route 防泄露）")
	}
}

// 8. buildDNSConfig：TUN + 直连规则 server == "china"；TUN + includeRemoteRuleSets=false 时 final == "china"；非 TUN 时两者仍为 "local"
func TestBuildDNSConfigTunUsesChinaAndNonTunUsesLocal(t *testing.T) {
	// A: TUN 开启时
	sTun := Settings{Tun: true, Mode: "rule"}
	nodes := []Node{{ID: "n1", Name: "Node 1"}}

	// 8.1 TUN + includeRemoteRuleSets=false
	dnsConfigTunStartup := buildDNSConfig(sTun, "proxy", defaultAutoDNSPlan(), false, nodes)
	if dnsConfigTunStartup["final"] != "china" {
		t.Fatalf("TUN 开启且 includeRemoteRuleSets=false 时，dns.final 应为 china，实际：%v", dnsConfigTunStartup["final"])
	}

	// 8.2 TUN + clash_mode: direct 规则
	dnsRulesTun := dnsConfigTunStartup["rules"].([]map[string]interface{})
	var directRuleServerTun string
	for _, r := range dnsRulesTun {
		if r["clash_mode"] == "direct" {
			directRuleServerTun = r["server"].(string)
			break
		}
	}
	if directRuleServerTun != "china" {
		t.Fatalf("TUN 模式下 clash_mode: direct 的 DNS server 应为 china，实际：%v", directRuleServerTun)
	}

	// B: 非 TUN 模式（系统代理）
	sSys := Settings{SystemProxy: true, Mode: "rule"}

	// 8.3 非 TUN + includeRemoteRuleSets=false 时 final == "local"
	dnsConfigSysStartup := buildDNSConfig(sSys, "proxy", defaultAutoDNSPlan(), false, nodes)
	if dnsConfigSysStartup["final"] != "local" {
		t.Fatalf("非 TUN 且 includeRemoteRuleSets=false 时，dns.final 应为 local，实际：%v", dnsConfigSysStartup["final"])
	}

	// 8.4 非 TUN + clash_mode: direct 规则 server == "local"
	dnsRulesSys := dnsConfigSysStartup["rules"].([]map[string]interface{})
	var directRuleServerSys string
	for _, r := range dnsRulesSys {
		if r["clash_mode"] == "direct" {
			directRuleServerSys = r["server"].(string)
			break
		}
	}
	if directRuleServerSys != "local" {
		t.Fatalf("非 TUN 模式下 clash_mode: direct 的 DNS server 应为 local，实际：%v", directRuleServerSys)
	}
}
