package main

import (
	"strings"
	"testing"
	"time"
)

func TestSingboxMissingFakeIPLogClassification(t *testing.T) {
	line := "+0800 2026-09-21 18:07:37 ERROR [1372636300 0ms] router: missing fakeip record, try enable `experimental.cache_file`"
	if !isSingboxMissingFakeIPLog(line) {
		t.Fatalf("isSingboxMissingFakeIPLog 应判定为 true，实际为 false")
	}

	if isSingboxErrorLog(line) {
		t.Fatalf("isSingboxErrorLog 对 missing fakeip record 应返回 false，避免被判定为红色错误，实际为 true")
	}
}

func TestSingboxMissingFakeIPSummaryRateLimiter(t *testing.T) {
	mgr := &SingBoxManager{}

	// 第一次发生时应立即触发汇总输出
	count, ok := mgr.shouldLogMissingFakeIPSummary()
	if !ok || count != 1 {
		t.Fatalf("首次检测应触发汇总：count=%d, ok=%v", count, ok)
	}

	// 紧接着的调用应被限流（静默累加）
	count, ok = mgr.shouldLogMissingFakeIPSummary()
	if ok {
		t.Fatalf("紧随其后的调用应被限流，实际 ok=%v, count=%d", ok, count)
	}

	// 模拟30秒窗口之后，应输出累加计数
	mgr.mu.Lock()
	mgr.lastMissingFakeIPLogAt = time.Now().Add(-31 * time.Second)
	mgr.mu.Unlock()

	count, ok = mgr.shouldLogMissingFakeIPSummary()
	if !ok || count != 2 {
		t.Fatalf("30秒窗口后应输出累加总数 2，实际 count=%d, ok=%v", count, ok)
	}
}

func TestTunStabilityRouteRulesNeverBlocksFakeIP(t *testing.T) {
	rules := buildTunStabilityRouteRules()
	for _, rule := range rules {
		action, _ := rule["action"].(string)
		outbound, _ := rule["outbound"].(string)
		if action == "reject" || outbound == "block" {
			for _, cidr := range configStringValues(rule["ip_cidr"]) {
				if isFakeIPCIDROverlap(cidr) {
					t.Fatalf("buildTunStabilityRouteRules 严禁包含 FakeIP (%s) 的 block 规则，否则会导致 TUN 下所有正常域名解析被拦截: %v", cidr, rule)
				}
			}
			for _, cidr := range configStringValues(rule["source_ip_cidr"]) {
				if isFakeIPCIDROverlap(cidr) {
					t.Fatalf("buildTunStabilityRouteRules 严禁包含 FakeIP 源地址 (%s) 的 block 规则: %v", cidr, rule)
				}
			}
		}
	}
}

func TestTunStabilityRouteRulesBlocksBroadcastMulticastLoopbackLinkLocal(t *testing.T) {
	rules := buildTunStabilityRouteRules()
	var blockedIPs []string
	for _, rule := range rules {
		if (rule["action"] == "reject" || rule["outbound"] == "block") && rule["inbound"] != nil {
			inbounds := configStringValues(rule["inbound"])
			if stringSliceContains(inbounds, "tun-in") {
				blockedIPs = append(blockedIPs, configStringValues(rule["ip_cidr"])...)
			}
		}
	}

	requiredCIDRs := []string{
		"0.0.0.0/8",          // 广播
		"127.0.0.0/8",        // 回环 IPv4
		"::1/128",            // 回环 IPv6
		"169.254.0.0/16",     // 链路本地与元数据 IPv4
		"fe80::/10",          // 链路本地 IPv6
		"224.0.0.0/4",        // 组播 IPv4
		"240.0.0.0/4",        // 保留 IPv4
		"255.255.255.255/32", // 广播 IPv4
		"ff00::/8",           // 组播 IPv6
	}

	for _, required := range requiredCIDRs {
		if !stringSliceContains(blockedIPs, required) {
			t.Errorf("TUN 稳定性规则必须拦截 %s，实际已拦截列表: %v", required, blockedIPs)
		}
	}
}

func TestFakeDNSProhibitsBlockingFakeIPInConfig(t *testing.T) {
	// 测试 TUN 模式（自动开启 FakeDNS）和普通模式显式开启 FakeDNS
	modes := []string{"rule", "global"}
	for _, mode := range modes {
		state := testConfigState(mode)
		state.Settings.Tun = true
		state.Settings.FakeDNS = true

		config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
		if err != nil {
			t.Fatalf("生成配置失败 (mode=%s): %v", mode, err)
		}

		route, ok := config["route"].(map[string]interface{})
		if !ok {
			t.Fatalf("缺少 route 配置 (mode=%s)", mode)
		}
		routeRules, _ := route["rules"].([]map[string]interface{})

		for idx, rule := range routeRules {
			action, _ := rule["action"].(string)
			outbound, _ := rule["outbound"].(string)
			if action == "reject" || outbound == "block" {
				for _, cidr := range configStringValues(rule["ip_cidr"]) {
					if isFakeIPCIDROverlap(cidr) {
						t.Fatalf("FakeDNS 开启时 (mode=%s)，路由规则 #%d 严禁拦截 FakeIP: cidr=%s, rule=%v", mode, idx, cidr, rule)
					}
				}
			}
		}
	}
}

func TestValidateGeneratedConfigRejectsFakeIPBlock(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	state.Settings.FakeDNS = true

	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("BuildSingBoxConfig 失败: %v", err)
	}

	// 注入非法的 198.18.0.0/15 -> block 规则
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	badRule := map[string]interface{}{
		"inbound":  []string{"tun-in"},
		"ip_cidr":  []string{"198.18.0.0/15"},
		"action":   "route",
		"outbound": "block",
	}
	route["rules"] = append([]map[string]interface{}{badRule}, rules...)

	// 校验必须拦截并报错
	err = validateGeneratedConfig(config, state.Settings)
	if err == nil {
		t.Fatalf("validateGeneratedConfig 必须拒绝 FakeDNS 开启时存在 198.18.0.0/15 -> block 规则，但未报错")
	}
	if !strings.Contains(err.Error(), "不得存在拦截 FakeIP 的 block 规则") {
		t.Fatalf("期望错误包含 '不得存在拦截 FakeIP 的 block 规则'，实际错误: %v", err)
	}
}
