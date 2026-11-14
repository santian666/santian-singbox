package main

import (
	"path/filepath"
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

// 6. T03-3 决策函数单测
func TestDecideDirectDNSAction(t *testing.T) {
	// 6.1 runningReachable == true → Keep (即使有新解析的更优物理网卡 DNS，也绝不重启)
	if act := decideDirectDNSAction("192.168.1.1", true, "10.0.0.1", "physical"); act != directDNSKeep {
		t.Fatalf("runningReachable 为 true 时必须为 Keep，实际: %v", act)
	}

	// 6.2 resolvedSource == "default" → Keep (断网时不重启)
	if act := decideDirectDNSAction("192.168.1.1", false, "223.5.5.5", "default"); act != directDNSKeep {
		t.Fatalf("resolvedSource 为 default 时必须为 Keep，实际: %v", act)
	}

	// 6.3 resolved == running → Keep (解析相同不重启)
	if act := decideDirectDNSAction("192.168.1.1", false, "192.168.1.1", "physical"); act != directDNSKeep {
		t.Fatalf("resolved 与 running 相同时必须为 Keep，实际: %v", act)
	}

	// 6.4 running 不可用且 source 为 physical → Restart
	if act := decideDirectDNSAction("192.168.1.1", false, "10.0.0.1", "physical"); act != directDNSRestart {
		t.Fatalf("running 不可用且 source 为 physical 时必须为 Restart，实际: %v", act)
	}

	// 6.5 running 不可用且 source 为 physical-secondary → Restart
	if act := decideDirectDNSAction("192.168.1.1", false, "10.0.0.2", "physical-secondary"); act != directDNSRestart {
		t.Fatalf("running 不可用且 source 为 physical-secondary 时必须为 Restart，实际: %v", act)
	}

	// 6.6 running 不可用且 source 为 fallback → Restart
	if act := decideDirectDNSAction("192.168.1.1", false, "119.29.29.29", "fallback"); act != directDNSRestart {
		t.Fatalf("running 不可用且 source 为 fallback 时必须为 Restart，实际: %v", act)
	}

	// 6.7 running 为空且 resolved 可用 → Restart
	if act := decideDirectDNSAction("", false, "192.168.1.1", "physical"); act != directDNSRestart {
		t.Fatalf("running 为空且 resolved 可用时必须为 Restart，实际: %v", act)
	}
}

// 7. T03-2 指纹比较单测
func TestNetworkFingerprintComparison(t *testing.T) {
	fpBase := networkFingerprint{
		IfIndex: 12,
		LocalIP: "192.168.1.100",
		DNS:     "192.168.1.1,192.168.1.2",
	}

	// 完全相同
	fpSame := networkFingerprint{
		IfIndex: 12,
		LocalIP: "192.168.1.100",
		DNS:     "192.168.1.1,192.168.1.2",
	}
	if !fpBase.Equal(fpSame) || fpBase != fpSame {
		t.Fatal("完全相同的指纹判定为变化")
	}

	// IfIndex 改变
	fpDiffIfIndex := fpBase
	fpDiffIfIndex.IfIndex = 15
	if fpBase.Equal(fpDiffIfIndex) || fpBase == fpDiffIfIndex {
		t.Fatal("IfIndex 改变应判定为变化")
	}

	// LocalIP 改变
	fpDiffIP := fpBase
	fpDiffIP.LocalIP = "192.168.1.101"
	if fpBase.Equal(fpDiffIP) || fpBase == fpDiffIP {
		t.Fatal("LocalIP 改变应判定为变化")
	}

	// DNS 改变
	fpDiffDNS := fpBase
	fpDiffDNS.DNS = "192.168.1.1,8.8.8.8"
	if fpBase.Equal(fpDiffDNS) || fpBase == fpDiffDNS {
		t.Fatal("DNS 改变应判定为变化")
	}
}

// 8. T03-2 缓存清空单测
func TestInvalidateDirectDNSCache(t *testing.T) {
	now := time.Now()
	directDNSCacheMu.Lock()
	directDNSCache = directDNSCacheEntry{
		ifIndex:   10,
		server:    "192.168.1.1",
		source:    "physical",
		expiresAt: now.Add(60 * time.Second),
	}
	directDNSCacheMu.Unlock()

	invalidateDirectDNSCache()

	directDNSCacheMu.Lock()
	clearedServer := directDNSCache.server
	directDNSCacheMu.Unlock()

	if clearedServer != "" {
		t.Fatalf("invalidateDirectDNSCache 后缓存 server 应为空，实际: %s", clearedServer)
	}
}

// 9. T03-4 健康检查接入逻辑单测
func TestDirectDNSHealthCheckerScenarios(t *testing.T) {
	baseTime := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	// 9.1 指纹从 A 变为 B，resolve 返回新地址 → restart 被调用 1 次
	t.Run("FingerprintChangedRestartsOnce", func(t *testing.T) {
		fpA := networkFingerprint{IfIndex: 10, LocalIP: "192.168.1.10", DNS: "192.168.1.1"}
		fpB := networkFingerprint{IfIndex: 20, LocalIP: "10.0.0.10", DNS: "10.0.0.1"}

		currentFP := fpA
		var restartCount int32
		var sleptDuration time.Duration

		checker := &directDNSHealthChecker{
			getFingerprint: func() networkFingerprint { return currentFP },
			probeDNS: func(server string) (time.Duration, bool) {
				if server == "192.168.1.1" {
					return 0, false
				}
				return 10 * time.Millisecond, true
			},
			resolvePlan: func() (string, string) {
				return "10.0.0.1", "physical"
			},
			restart: func() error {
				atomic.AddInt32(&restartCount, 1)
				return nil
			},
			runningDNS: func() string { return "192.168.1.1" },
			now:        func() time.Time { return baseTime },
			sleep: func(d time.Duration) {
				sleptDuration = d
			},
		}

		// 第一次执行：只记录指纹，不动作
		restarted := checker.CheckHealth()
		if restarted || atomic.LoadInt32(&restartCount) != 0 {
			t.Fatalf("第一次记录指纹不应重启，实际: restarted=%v, count=%d", restarted, restartCount)
		}

		// 网络切换为 B
		currentFP = fpB
		restarted = checker.CheckHealth()
		if !restarted || atomic.LoadInt32(&restartCount) != 1 {
			t.Fatalf("指纹从 A 变 B 且 resolve 新地址时应重启 1 次，实际: restarted=%v, count=%d", restarted, restartCount)
		}
		if sleptDuration != 3*time.Second {
			t.Fatalf("指纹变化后应等待 3 秒稳定，实际: %v", sleptDuration)
		}
	})

	// 9.2 指纹不变，探测连续失败 1 次 → 不 resolve；连续失败 2 次 → resolve
	t.Run("ProbeFailuresTriggerResolve", func(t *testing.T) {
		fp := networkFingerprint{IfIndex: 10, LocalIP: "192.168.1.10", DNS: "192.168.1.1"}
		var resolveCount int32
		var restartCount int32

		checker := &directDNSHealthChecker{
			checkCycleInterval: 1,
			getFingerprint:     func() networkFingerprint { return fp },
			probeDNS: func(server string) (time.Duration, bool) {
				return 0, false
			},
			resolvePlan: func() (string, string) {
				atomic.AddInt32(&resolveCount, 1)
				return "119.29.29.29", "fallback"
			},
			restart: func() error {
				atomic.AddInt32(&restartCount, 1)
				return nil
			},
			runningDNS: func() string { return "192.168.1.1" },
			now:        func() time.Time { return baseTime },
		}

		checker.CheckHealth()
		if atomic.LoadInt32(&resolveCount) != 0 {
			t.Fatalf("连续失败 1 次不应触发 resolve，实际 resolveCount=%d", resolveCount)
		}

		checker.CheckHealth()
		if atomic.LoadInt32(&resolveCount) != 1 {
			t.Fatalf("连续失败 2 次必须触发 resolve，实际 resolveCount=%d", resolveCount)
		}
		if atomic.LoadInt32(&restartCount) != 1 {
			t.Fatalf("resolve 返回有效 fallback 应重启 1 次，实际 restartCount=%d", restartCount)
		}
	})

	// 9.3 resolve 返回 default → 不 restart
	t.Run("ResolveDefaultDoesNotRestart", func(t *testing.T) {
		fpA := networkFingerprint{IfIndex: 10, LocalIP: "192.168.1.10", DNS: "192.168.1.1"}
		fpB := networkFingerprint{IfIndex: 20, LocalIP: "10.0.0.10", DNS: "10.0.0.1"}

		currentFP := fpA
		var restartCount int32

		checker := &directDNSHealthChecker{
			getFingerprint: func() networkFingerprint { return currentFP },
			probeDNS: func(server string) (time.Duration, bool) {
				return 0, false
			},
			resolvePlan: func() (string, string) {
				return "223.5.5.5", "default"
			},
			restart: func() error {
				atomic.AddInt32(&restartCount, 1)
				return nil
			},
			runningDNS: func() string { return "192.168.1.1" },
			now:        func() time.Time { return baseTime },
		}

		checker.CheckHealth()
		currentFP = fpB
		restarted := checker.CheckHealth()
		if restarted || atomic.LoadInt32(&restartCount) != 0 {
			t.Fatalf("resolve 返回 default 时断网，绝不应重启内核，实际: restarted=%v, restartCount=%d", restarted, restartCount)
		}
	})

	// 9.4 距离上次重启不足 20 秒 → 不 restart
	t.Run("ThrottledUnder20Seconds", func(t *testing.T) {
		fpA := networkFingerprint{IfIndex: 10, LocalIP: "192.168.1.10", DNS: "192.168.1.1"}
		fpB := networkFingerprint{IfIndex: 20, LocalIP: "10.0.0.10", DNS: "10.0.0.1"}

		currentFP := fpA
		var restartCount int32
		lastRestart := baseTime.Add(-10 * time.Second)

		checker := &directDNSHealthChecker{
			getFingerprint: func() networkFingerprint { return currentFP },
			probeDNS: func(server string) (time.Duration, bool) {
				return 0, false
			},
			resolvePlan: func() (string, string) {
				return "10.0.0.1", "physical"
			},
			restart: func() error {
				atomic.AddInt32(&restartCount, 1)
				return nil
			},
			runningDNS:     func() string { return "192.168.1.1" },
			now:            func() time.Time { return baseTime },
			getLastRestart: func() time.Time { return lastRestart },
		}

		checker.CheckHealth()
		currentFP = fpB
		restarted := checker.CheckHealth()
		if restarted || atomic.LoadInt32(&restartCount) != 0 {
			t.Fatalf("距离上次重启不足 20 秒时不应重启，实际: restarted=%v, restartCount=%d", restarted, restartCount)
		}
	})

	// 9.5 唤醒后的第一次检查 → 即使指纹未变也会 resolve
	t.Run("ResumeForcesResolveEvenIfFingerprintUnchanged", func(t *testing.T) {
		fp := networkFingerprint{IfIndex: 10, LocalIP: "192.168.1.10", DNS: "192.168.1.1"}
		var resolveCount int32

		checker := &directDNSHealthChecker{
			checkCycleInterval: 3,
			getFingerprint:     func() networkFingerprint { return fp },
			probeDNS: func(server string) (time.Duration, bool) {
				return 10 * time.Millisecond, true
			},
			resolvePlan: func() (string, string) {
				atomic.AddInt32(&resolveCount, 1)
				return "192.168.1.1", "physical"
			},
			runningDNS: func() string { return "192.168.1.1" },
			now:        func() time.Time { return baseTime },
		}

		checker.CheckHealth()
		if atomic.LoadInt32(&resolveCount) != 0 {
			t.Fatalf("初始指纹不应触发 resolve，实际: %d", resolveCount)
		}

		checker.SetForceReselect()

		checker.CheckHealth()
		if atomic.LoadInt32(&resolveCount) != 1 {
			t.Fatalf("唤醒后第一次检查即使指纹未变，也必须触发 resolve，实际 resolveCount=%d", resolveCount)
		}
	})

	// 9.6 同一未重启原因 5 分钟内只记一次
	t.Run("NoRestartLoggingThrottled5Minutes", func(t *testing.T) {
		var loggedReasons []string
		currTime := baseTime

		checker := &directDNSHealthChecker{
			now: func() time.Time { return currTime },
			logInfo: func(format string, args ...any) {
				loggedReasons = append(loggedReasons, format)
			},
		}

		reason := "网络可能未连通，暂不切换"
		checker.logNoRestartThrottled(reason, currTime)
		if len(loggedReasons) != 1 {
			t.Fatalf("第一次应记录日志，实际条数: %d", len(loggedReasons))
		}

		currTime = currTime.Add(1 * time.Minute)
		checker.logNoRestartThrottled(reason, currTime)
		if len(loggedReasons) != 1 {
			t.Fatalf("5 分钟内相同原因不应重复记录，实际条数: %d", len(loggedReasons))
		}

		currTime = currTime.Add(5 * time.Minute)
		checker.logNoRestartThrottled(reason, currTime)
		if len(loggedReasons) != 2 {
			t.Fatalf("5 分钟后应允许记录相同原因，实际条数: %d", len(loggedReasons))
		}
	})
}

// 10. T03-1 / Section 3-4 RunningDirectDNS() 测试
func TestSingBoxManagerRunningDirectDNS(t *testing.T) {
	manager := NewSingBoxManager(t.TempDir(), "sing-box.exe", "sing-box.json", filepath.Join(t.TempDir(), "cache.db"), nil)

	// 初始状态为空
	if got := manager.RunningDirectDNS(); got != "" {
		t.Fatalf("初始 RunningDirectDNS 必须为空，实际: %s", got)
	}

	// 模拟启动成功赋值
	manager.mu.Lock()
	manager.runningDirectDNS = "192.168.1.1"
	manager.mu.Unlock()

	if got := manager.RunningDirectDNS(); got != "192.168.1.1" {
		t.Fatalf("启动后 RunningDirectDNS 必须返回本次使用的 plan，实际: %s", got)
	}

	// 模拟停止清空
	manager.mu.Lock()
	manager.stopLocked(false, false, false)
	manager.mu.Unlock()

	if got := manager.RunningDirectDNS(); got != "" {
		t.Fatalf("停止后 RunningDirectDNS 必须清空为空字符串，实际: %s", got)
	}
}
