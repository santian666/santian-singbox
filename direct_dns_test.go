package main

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// 1. filterDirectDNSServers 纯函数测试
func TestFilterDirectDNSServers(t *testing.T) {
	input := []string{
		"192.168.1.1", "127.0.0.1", "172.19.0.2", "198.18.0.5",
		"0.0.0.0", "169.254.1.1", "fe80::1", "192.168.1.1",
		"10.0.0.1", "8.8.8.8", "1.1.1.1",
	}
	expected := []string{"192.168.1.1", "10.0.0.1", "8.8.8.8"}
	actual := filterDirectDNSServers(input)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("filterDirectDNSServers 结果不符合预期。\n期望: %v\n实际: %v", expected, actual)
	}
}

// 2. chooseDirectDNS 探测与回退选择逻辑测试
func TestChooseDirectDNS(t *testing.T) {
	fallbacks := []string{"223.5.5.5", "119.29.29.29", "1.1.1.1"}

	// 2.1 物理 DNS 有两个可用 → 选更快的那个，source 为 physical
	latencies := map[string]time.Duration{
		"192.168.1.1": 50 * time.Millisecond,
		"192.168.1.2": 20 * time.Millisecond,
	}
	probeFunc1 := func(server string) (time.Duration, bool) {
		if lat, ok := latencies[server]; ok {
			return lat, true
		}
		return 0, false
	}
	srv, src := chooseDirectDNS([]string{"192.168.1.1", "192.168.1.2"}, fallbacks, probeFunc1)
	if srv != "192.168.1.2" || src != "physical" {
		t.Fatalf("物理 DNS 可用时应选延迟最低的 physical，实际: %s, %s", srv, src)
	}

	// 2.2 物理 DNS 全部不可用 → 从 fallbacks 中选最快的，source 为 fallback
	fallbackLatencies := map[string]time.Duration{
		"223.5.5.5":    80 * time.Millisecond,
		"119.29.29.29": 30 * time.Millisecond,
		"1.1.1.1":      60 * time.Millisecond,
	}
	probeFunc2 := func(server string) (time.Duration, bool) {
		if lat, ok := fallbackLatencies[server]; ok {
			return lat, true
		}
		return 0, false
	}
	srv, src = chooseDirectDNS([]string{"192.168.1.1"}, fallbacks, probeFunc2)
	if srv != "119.29.29.29" || src != "fallback" {
		t.Fatalf("物理 DNS 全部不可用时应选 fallback 中延迟最低的，实际: %s, %s", srv, src)
	}

	// 2.3 physical 为空 → 直接走 fallbacks
	srv, src = chooseDirectDNS(nil, fallbacks, probeFunc2)
	if srv != "119.29.29.29" || src != "fallback" {
		t.Fatalf("physical 为空时应走 fallback，实际: %s, %s", srv, src)
	}

	// 2.4 全部不可用 → fallbacks[0]，source 为 default
	probeFuncFail := func(server string) (time.Duration, bool) {
		return 0, false
	}
	srv, src = chooseDirectDNS([]string{"192.168.1.1"}, fallbacks, probeFuncFail)
	if srv != "223.5.5.5" || src != "default" {
		t.Fatalf("全部探测失败时应返回 fallbacks[0] 且 source 为 default，实际: %s, %s", srv, src)
	}
}

// 3. 缓存测试：同一 ifIndex 在 60 秒内第二次调用不会触发 probe；ifIndex 变化后会重新 probe
func TestDirectDNSCache(t *testing.T) {
	// 重置缓存
	directDNSCacheMu.Lock()
	directDNSCache = directDNSCacheEntry{}
	directDNSCacheMu.Unlock()

	var probeCount int32
	mockProbe := func(server string) (time.Duration, bool) {
		atomic.AddInt32(&probeCount, 1)
		return 10 * time.Millisecond, true
	}

	currentTime := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	currentIfIndex := uint32(10)
	mockDetectDNS := func() []string {
		return []string{"192.168.1.1"}
	}
	fallbacks := []string{"223.5.5.5"}

	// 第一次调用：应触发 probe
	srv1, src1 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
	)
	if srv1 != "192.168.1.1" || src1 != "physical" {
		t.Fatalf("第一次调用解析结果异常: %s, %s", srv1, src1)
	}
	if count := atomic.LoadInt32(&probeCount); count != 1 {
		t.Fatalf("第一次调用应执行 1 次探测，实际: %d", count)
	}

	// 第二次调用（同一 ifIndex，30 秒后，仍在 60 秒有效期内）：不应触发 probe
	currentTime = currentTime.Add(30 * time.Second)
	srv2, src2 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
	)
	if srv2 != srv1 || src2 != src1 {
		t.Fatalf("缓存命中时结果应与第一次一致: %s, %s", srv2, src2)
	}
	if count := atomic.LoadInt32(&probeCount); count != 1 {
		t.Fatalf("缓存有效期内第二次调用不应增加探测次数，实际: %d", count)
	}

	// 第三次调用（时间不变，但 ifIndex 改变为 20）：网卡变化，应重新触发 probe
	currentIfIndex = 20
	srv3, src3 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
	)
	if srv3 != "192.168.1.1" || src3 != "physical" {
		t.Fatalf("网卡变化后解析结果异常: %s, %s", srv3, src3)
	}
	if count := atomic.LoadInt32(&probeCount); count != 2 {
		t.Fatalf("网卡变化后应重新触发探测，探测次数应为 2，实际: %d", count)
	}

	// 第四次调用（同一 ifIndex 20，但时间前进 61 秒，缓存已过期）：应重新触发 probe
	currentTime = currentTime.Add(61 * time.Second)
	resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
	)
	if count := atomic.LoadInt32(&probeCount); count != 3 {
		t.Fatalf("缓存过期后应重新触发探测，探测次数应为 3，实际: %d", count)
	}
}
