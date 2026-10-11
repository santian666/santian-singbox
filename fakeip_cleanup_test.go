package main

import (
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

func TestTunStabilityRouteRulesBlocksOrphanedFakeIP(t *testing.T) {
	rules := buildTunStabilityRouteRules()
	found := false
	for _, rule := range rules {
		if action, _ := rule["action"].(string); action != "route" {
			continue
		}
		if outbound, _ := rule["outbound"].(string); outbound != "block" {
			continue
		}
		if ipCIDRs, ok := rule["ip_cidr"].([]string); ok {
			for _, cidr := range ipCIDRs {
				if cidr == "198.18.0.0/15" {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatalf("buildTunStabilityRouteRules 必须包含 198.18.0.0/15 的 block 规则，以防止未反查 FakeIP 孤儿包外泄或挂起")
	}
}
