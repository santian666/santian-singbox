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

// 2. chooseDirectDNS 探测与优先级选择逻辑测试
func TestChooseDirectDNS(t *testing.T) {
	fallbacks := []string{"223.5.5.5", "119.29.29.29", "1.1.1.1"}

	// 2.1 primary 可用但比 secondary 慢 → 仍然选 primary，source 为 physical
	latencies1 := map[string]time.Duration{
		"192.168.1.1": 80 * time.Millisecond, // primary (较慢)
		"10.0.0.1":    20 * time.Millisecond, // secondary (较快)
		"223.5.5.5":   30 * time.Millisecond, // fallback
	}
	probe1 := func(server string) (time.Duration, bool) {
		if lat, ok := latencies1[server]; ok {
			return lat, true
		}
		return 0, false
	}
	srv, src := chooseDirectDNS([]string{"192.168.1.1"}, []string{"10.0.0.1"}, fallbacks, probe1, 500*time.Millisecond)
	if srv != "192.168.1.1" || src != "physical" {
		t.Fatalf("primary 可用但较慢时，必须优先选 primary (physical)，实际: %s, %s", srv, src)
	}

	// 2.2 primary 全部不可用、secondary 可用 → 选 secondary，source 为 physical-secondary
	latencies2 := map[string]time.Duration{
		"10.0.0.1":  50 * time.Millisecond,
		"10.0.0.2":  20 * time.Millisecond,
		"223.5.5.5": 10 * time.Millisecond,
	}
	probe2 := func(server string) (time.Duration, bool) {
		if lat, ok := latencies2[server]; ok {
			return lat, true
		}
		return 0, false
	}
	srv, src = chooseDirectDNS([]string{"192.168.1.1"}, []string{"10.0.0.1", "10.0.0.2"}, fallbacks, probe2, 500*time.Millisecond)
	if srv != "10.0.0.2" || src != "physical-secondary" {
		t.Fatalf("primary 不可用时应选 secondary 中最快的，实际: %s, %s", srv, src)
	}

	// 2.3 primary 和 secondary 都不可用 → fallback 中最快的，source 为 fallback
	latencies3 := map[string]time.Duration{
		"223.5.5.5":    80 * time.Millisecond,
		"119.29.29.29": 30 * time.Millisecond,
		"1.1.1.1":      60 * time.Millisecond,
	}
	probe3 := func(server string) (time.Duration, bool) {
		if lat, ok := latencies3[server]; ok {
			return lat, true
		}
		return 0, false
	}
	srv, src = chooseDirectDNS([]string{"192.168.1.1"}, []string{"10.0.0.1"}, fallbacks, probe3, 500*time.Millisecond)
	if srv != "119.29.29.29" || src != "fallback" {
		t.Fatalf("网卡全部不可用时应选 fallback 中最快的，实际: %s, %s", srv, src)
	}

	// 2.4 全部不可用 → fallbacks[0]，source 为 default
	probeFail := func(server string) (time.Duration, bool) {
		return 0, false
	}
	srv, src = chooseDirectDNS([]string{"192.168.1.1"}, []string{"10.0.0.1"}, fallbacks, probeFail, 500*time.Millisecond)
	if srv != "223.5.5.5" || src != "default" {
		t.Fatalf("全部不可用时应返回 fallbacks[0] 且 source 为 default，实际: %s, %s", srv, src)
	}

	// 2.5 同一地址同时出现在 primary、secondary 和 fallbacks 中 → probe 只被调用一次
	var callCount int32
	sharedProbe := func(server string) (time.Duration, bool) {
		if server == "8.8.8.8" {
			atomic.AddInt32(&callCount, 1)
		}
		return 20 * time.Millisecond, true
	}
	chooseDirectDNS(
		[]string{"8.8.8.8"},
		[]string{"8.8.8.8", "10.0.0.1"},
		[]string{"8.8.8.8", "223.5.5.5"},
		sharedProbe,
		500*time.Millisecond,
	)
	if count := atomic.LoadInt32(&callCount); count != 1 {
		t.Fatalf("重复地址 8.8.8.8 的 probe 必须仅被调用 1 次，实际调用了 %d 次", count)
	}
}

// 3. 缓存测试：同一 ifIndex 在 60 秒内第二次调用不会触发 probe；ifIndex 变化后会重新 probe
func TestDirectDNSCache(t *testing.T) {
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
	mockDetectDNS := func() (primary, secondary []string) {
		return []string{"192.168.1.1"}, nil
	}
	fallbacks := []string{"223.5.5.5"}

	// 第一次调用：应触发 probe
	srv1, src1 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
		500*time.Millisecond,
	)
	if srv1 != "192.168.1.1" || src1 != "physical" {
		t.Fatalf("第一次调用解析结果异常: %s, %s", srv1, src1)
	}
	if count := atomic.LoadInt32(&probeCount); count != 2 { // 192.168.1.1 + 223.5.5.5 (并发探测)
		t.Fatalf("第一次调用应执行 2 次探测 (primary + fallback)，实际: %d", count)
	}

	// 第二次调用（同一 ifIndex，30 秒后，仍在 60 秒有效期内）：不应触发 probe
	currentTime = currentTime.Add(30 * time.Second)
	srv2, src2 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
		500*time.Millisecond,
	)
	if srv2 != srv1 || src2 != src1 {
		t.Fatalf("缓存命中时结果应与第一次一致: %s, %s", srv2, src2)
	}
	if count := atomic.LoadInt32(&probeCount); count != 2 {
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
		500*time.Millisecond,
	)
	if srv3 != "192.168.1.1" || src3 != "physical" {
		t.Fatalf("网卡变化后解析结果异常: %s, %s", srv3, src3)
	}
	if count := atomic.LoadInt32(&probeCount); count != 4 {
		t.Fatalf("网卡变化后应重新触发探测，探测次数应增加 2 到 4，实际: %d", count)
	}

	// 第四次调用（同一 ifIndex 20，但时间前进 61 秒，缓存已过期）：应重新触发 probe
	currentTime = currentTime.Add(61 * time.Second)
	resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		mockProbe,
		fallbacks,
		500*time.Millisecond,
	)
	if count := atomic.LoadInt32(&probeCount); count != 6 {
		t.Fatalf("缓存过期后应重新触发探测，探测次数应增加 2 到 6，实际: %d", count)
	}
}

// 4. R3 新增：全部探测失败 (default) 后，第二次调用会再次触发 probe（不写缓存）
func TestDirectDNSCacheDefaultNotCached(t *testing.T) {
	directDNSCacheMu.Lock()
	directDNSCache = directDNSCacheEntry{}
	directDNSCacheMu.Unlock()

	var probeCount int32
	failingProbe := func(server string) (time.Duration, bool) {
		atomic.AddInt32(&probeCount, 1)
		return 0, false
	}

	currentTime := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	currentIfIndex := uint32(10)
	mockDetectDNS := func() (primary, secondary []string) {
		return []string{"192.168.1.1"}, nil
	}
	fallbacks := []string{"223.5.5.5"}

	// 第一次调用全部失败，返回 default
	srv1, src1 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		failingProbe,
		fallbacks,
		200*time.Millisecond,
	)
	if srv1 != "223.5.5.5" || src1 != "default" {
		t.Fatalf("全失败时应返回 default，实际: %s, %s", srv1, src1)
	}
	count1 := atomic.LoadInt32(&probeCount)
	if count1 == 0 {
		t.Fatal("第一次调用应当执行探测")
	}

	// 第二次调用（同一 ifIndex，仅过去 5 秒）：由于 default 不缓存，必须再次触发 probe
	currentTime = currentTime.Add(5 * time.Second)
	srv2, src2 := resolveDirectDNSWithConfig(
		func() time.Time { return currentTime },
		func() uint32 { return currentIfIndex },
		mockDetectDNS,
		failingProbe,
		fallbacks,
		200*time.Millisecond,
	)
	if srv2 != "223.5.5.5" || src2 != "default" {
		t.Fatalf("第二次调用依然返回 default，实际: %s, %s", srv2, src2)
	}
	count2 := atomic.LoadInt32(&probeCount)
	if count2 <= count1 {
		t.Fatalf("default 结果不应写入缓存，第二次调用必须重新触发探测 (count1=%d, count2=%d)", count1, count2)
	}
}

// 5. R2 新增：probe 中有卡住超过总超时的地址时，chooseDirectDNS 依然在超时限制内返回
func TestChooseDirectDNSTimeoutEnforced(t *testing.T) {
	hangingProbe := func(server string) (time.Duration, bool) {
		if server == "1.1.1.1" {
			time.Sleep(2 * time.Second) // 模拟卡住
			return 10 * time.Millisecond, true
		}
		if server == "223.5.5.5" {
			return 20 * time.Millisecond, true
		}
		return 0, false
	}

	// 设置超时为 100ms
	timeout := 100 * time.Millisecond
	start := time.Now()
	srv, src := chooseDirectDNS(
		[]string{"1.1.1.1"}, // 卡住的 primary
		nil,
		[]string{"223.5.5.5"}, // 可用的 fallback
		hangingProbe,
		timeout,
	)
	elapsed := time.Since(start)

	// 耗时应在 100ms 左右，远小于 2 秒
	if elapsed > 500*time.Millisecond {
		t.Fatalf("探测超时未生效，耗时过长: %v (设定超时: %v)", elapsed, timeout)
	}
	// primary 卡住未在 100ms 内返回，结果应落入 fallback
	if srv != "223.5.5.5" || src != "fallback" {
		t.Fatalf("超时后未及时返回的 primary 应被放弃并降级到 fallback，实际: %s, %s", srv, src)
	}
}
