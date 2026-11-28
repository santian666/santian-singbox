package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

var directDNSFallbackCandidates = []string{"223.5.5.5", "119.29.29.29", "1.1.1.1", "8.8.8.8", "9.9.9.9"}

type networkFingerprint struct {
	IfIndex uint32
	LocalIP string
	DNS     string
}

func (fp networkFingerprint) Equal(other networkFingerprint) bool {
	return fp.IfIndex == other.IfIndex && fp.LocalIP == other.LocalIP && fp.DNS == other.DNS
}

type directDNSProbeResult struct {
	latency time.Duration
	ok      bool
}

type directDNSCacheEntry struct {
	ifIndex   uint32
	server    string
	source    string
	expiresAt time.Time
}

var (
	directDNSCacheMu sync.Mutex
	directDNSCache   directDNSCacheEntry

	timeNowFunc                = time.Now
	getDefaultRouteIfIndexFunc = func() uint32 {
		return getDefaultRouteIfIndex()
	}
	detectPhysicalDNSServersFunc  = detectPhysicalDNSServers
	probeDirectUDPDNSFunc         = probeDirectUDPDNS
	currentNetworkFingerprintFunc = currentNetworkFingerprint
)

func invalidateDirectDNSCache() {
	directDNSCacheMu.Lock()
	directDNSCache = directDNSCacheEntry{}
	directDNSCacheMu.Unlock()
}

func isTunSubnetIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// 172.19.0.0/16 is SanTian TUN, 198.18.0.0/15 is FakeIP
	return (ip4[0] == 172 && ip4[1] == 19) || (ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19))
}

func isFakeIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
}

func filterDirectDNSServers(ips []string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, s := range ips {
		parsed := net.ParseIP(strings.TrimSpace(s))
		if parsed == nil {
			continue
		}
		ip4 := parsed.To4()
		if ip4 == nil {
			continue
		}
		// 去掉：0.0.0.0/8、127.0.0.0/8、169.254.0.0/16、172.19.0.0/16、198.18.0.0/15、组播和广播地址
		if ip4[0] == 0 {
			continue
		}
		if ip4[0] == 127 || ip4.IsLoopback() {
			continue
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			continue
		}
		if isTunSubnetIP(ip4) || isFakeIPv4(ip4) {
			continue
		}
		if ip4.IsMulticast() || (ip4[0] >= 224 && ip4[0] <= 239) {
			continue
		}
		if ip4.Equal(net.IPv4bcast) || (ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255) {
			continue
		}

		str := ip4.String()
		if seen[str] {
			continue
		}
		seen[str] = true
		result = append(result, str)
		if len(result) == 3 {
			break
		}
	}
	return result
}

func chooseDirectDNS(
	primary []string,
	secondary []string,
	fallbacks []string,
	probe func(server string) (time.Duration, bool),
	totalTimeout time.Duration,
) (server string, source string) {
	if totalTimeout <= 0 {
		totalTimeout = 1200 * time.Millisecond
	}

	// 收集所有需要探测的非重复 IP
	var allServers []string
	seen := make(map[string]bool)
	for _, list := range [][]string{primary, secondary, fallbacks} {
		for _, s := range list {
			if s != "" && !seen[s] {
				seen[s] = true
				allServers = append(allServers, s)
			}
		}
	}

	if len(allServers) == 0 {
		if len(fallbacks) > 0 {
			return fallbacks[0], "default"
		}
		return "223.5.5.5", "default"
	}

	type probeItem struct {
		server  string
		latency time.Duration
		ok      bool
	}

	resCh := make(chan probeItem, len(allServers))
	for _, srv := range allServers {
		srv := srv
		go func() {
			lat, ok := probe(srv)
			resCh <- probeItem{server: srv, latency: lat, ok: ok}
		}()
	}

	results := make(map[string]directDNSProbeResult, len(allServers))
	timer := time.NewTimer(totalTimeout)
	defer timer.Stop()

	remaining := len(allServers)
	for remaining > 0 {
		select {
		case item := <-resCh:
			results[item.server] = directDNSProbeResult{latency: item.latency, ok: item.ok}
			remaining--
		case <-timer.C:
			remaining = 0
		}
	}

	findBestIn := func(list []string) (string, bool) {
		var bestSrv string
		var bestLat time.Duration = 1<<63 - 1
		found := false
		for _, s := range list {
			if res, ok := results[s]; ok && res.ok {
				if !found || res.latency < bestLat {
					bestLat = res.latency
					bestSrv = s
					found = true
				}
			}
		}
		return bestSrv, found
	}

	if best, ok := findBestIn(primary); ok {
		return best, "physical"
	}
	if best, ok := findBestIn(secondary); ok {
		return best, "physical-secondary"
	}
	if best, ok := findBestIn(fallbacks); ok {
		return best, "fallback"
	}
	if len(fallbacks) > 0 {
		return fallbacks[0], "default"
	}
	return "223.5.5.5", "default"
}

func resolveDirectDNSWithConfig(
	nowFunc func() time.Time,
	getIfIndex func() uint32,
	detectDNS func() (primary, secondary []string),
	probe func(server string) (time.Duration, bool),
	fallbacks []string,
	totalTimeout time.Duration,
) (string, string) {
	now := nowFunc()
	currentIfIndex := getIfIndex()

	directDNSCacheMu.Lock()
	if directDNSCache.server != "" && directDNSCache.ifIndex == currentIfIndex && now.Before(directDNSCache.expiresAt) {
		srv, src := directDNSCache.server, directDNSCache.source
		directDNSCacheMu.Unlock()
		return srv, src
	}
	directDNSCacheMu.Unlock()

	var primary, secondary []string
	if detectDNS != nil {
		primary, secondary = detectDNS()
	}
	srv, src := chooseDirectDNS(primary, secondary, fallbacks, probe, totalTimeout)

	if src != "default" {
		directDNSCacheMu.Lock()
		directDNSCache = directDNSCacheEntry{
			ifIndex:   currentIfIndex,
			server:    srv,
			source:    src,
			expiresAt: now.Add(60 * time.Second),
		}
		directDNSCacheMu.Unlock()
	}

	return srv, src
}

func resolveDirectDNS(logger *Logger) (string, string) {
	return resolveDirectDNSWithConfig(
		timeNowFunc,
		getDefaultRouteIfIndexFunc,
		detectPhysicalDNSServersFunc,
		probeDirectUDPDNSFunc,
		directDNSFallbackCandidates,
		1200*time.Millisecond,
	)
}

func resolveDirectDNSPlan(logger *Logger) autoDNSPlan {
	server, source := resolveDirectDNS(logger)
	sourceDesc := "默认"
	switch source {
	case "physical":
		sourceDesc = "网卡"
	case "physical-secondary":
		sourceDesc = "其他网卡"
	case "fallback":
		sourceDesc = "公共候选测速"
	case "default":
		sourceDesc = "默认"
	}
	if logger != nil {
		logger.Info("直连 DNS：%s（来源：%s）", server, sourceDesc)
	}
	return autoDNSPlan{
		Direct: dnsProbeCandidate{
			Type:   "udp",
			Server: server,
		},
		Remote:       remoteDNSCandidates[0],
		DirectSource: source,
	}
}

type directDNSAction int

const (
	directDNSKeep directDNSAction = iota
	directDNSRestart
)

func decideDirectDNSAction(running string, runningReachable bool, resolved string, resolvedSource string) directDNSAction {
	if runningReachable {
		return directDNSKeep
	}
	if resolvedSource == "default" {
		return directDNSKeep
	}
	if resolved == running {
		return directDNSKeep
	}
	return directDNSRestart
}

const (
	// 当前直连 DNS 连续探测失败多少次（每 30 秒一次）才触发重新选择。
	// 路由器 DNS 偶尔丢一个 UDP 包是正常现象，阈值不能太低。
	directDNSFailThreshold = 3
	// 因直连 DNS 触发的内核重启，每小时最多次数；超过后暂停自动切换，
	// 直到下一次网络指纹变化或系统唤醒。
	directDNSMaxRestartsPerHour = 3
)

type directDNSHealthChecker struct {
	mu                     sync.Mutex
	hasRecordedFingerprint bool
	lastFingerprint        networkFingerprint
	directDNSCycleCount    int
	directDNSFailCount     int
	forceReselect          bool
	lastNoRestartLogAt     map[string]time.Time
	checkCycleInterval     int
	restartHistory         []time.Time
	autoSwitchPaused       bool

	// Dependencies:
	getFingerprint  func() networkFingerprint
	probeDNS        func(server string) (time.Duration, bool)
	invalidateCache func()
	resolvePlan     func() (server string, source string)
	restart         func() error
	runningDNS      func() string
	now             func() time.Time
	sleep           func(time.Duration)
	tryLockProxyOp  func() bool
	unlockProxyOp   func()
	getLastRestart  func() time.Time
	setLastRestart  func(time.Time)
	logInfo         func(format string, args ...any)
	logError        func(format string, args ...any)

	lastRestartAt time.Time
}

func newDirectDNSHealthChecker(a *App) *directDNSHealthChecker {
	return &directDNSHealthChecker{
		getFingerprint:  currentNetworkFingerprintFunc,
		probeDNS:        probeDirectUDPDNSFunc,
		invalidateCache: invalidateDirectDNSCache,
		resolvePlan: func() (string, string) {
			plan := resolveDirectDNSPlanFunc(a.logger)
			return plan.Direct.Server, plan.DirectSource
		},
		restart: func() error {
			return a.restartForDirectDNS()
		},
		runningDNS: func() string {
			if a == nil || a.manager == nil {
				return ""
			}
			return a.manager.RunningDirectDNS()
		},
		now:   time.Now,
		sleep: time.Sleep,
		tryLockProxyOp: func() bool {
			if a == nil {
				return true
			}
			return a.proxyOpMu.TryLock()
		},
		unlockProxyOp: func() {
			if a != nil {
				a.proxyOpMu.Unlock()
			}
		},
		getLastRestart: func() time.Time {
			if a == nil {
				return time.Time{}
			}
			a.healthMu.Lock()
			defer a.healthMu.Unlock()
			return a.lastHealthRestart
		},
		setLastRestart: func(t time.Time) {
			if a != nil {
				a.healthMu.Lock()
				a.lastHealthRestart = t
				a.healthMu.Unlock()
			}
		},
		logInfo: func(format string, args ...any) {
			if a != nil && a.logger != nil {
				a.logger.Info(format, args...)
			}
		},
		logError: func(format string, args ...any) {
			if a != nil && a.logger != nil {
				a.logger.Error(format, args...)
			}
		},
	}
}

// AutoSwitchPaused 返回是否因频繁切换而暂停了自动切换（供测试与诊断使用）。
func (c *directDNSHealthChecker) AutoSwitchPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.autoSwitchPaused
}

// recentRestartCountLocked 清理一小时以前的重启记录并返回剩余次数。调用方必须持有 c.mu。
func (c *directDNSHealthChecker) recentRestartCountLocked(now time.Time) int {
	kept := c.restartHistory[:0]
	for _, t := range c.restartHistory {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	c.restartHistory = kept
	return len(c.restartHistory)
}

func (c *directDNSHealthChecker) SetForceReselect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forceReselect = true
}

func (c *directDNSHealthChecker) HasForceReselect() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.forceReselect
}

func (c *directDNSHealthChecker) LastFingerprint() networkFingerprint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastFingerprint
}

func (c *directDNSHealthChecker) lastRestartTime() time.Time {
	if c.getLastRestart != nil {
		return c.getLastRestart()
	}
	return c.lastRestartAt
}

func (c *directDNSHealthChecker) updateLastRestart(t time.Time) {
	if c.setLastRestart != nil {
		c.setLastRestart(t)
	} else {
		c.lastRestartAt = t
	}
}

func (c *directDNSHealthChecker) logNoRestartThrottled(reason string, now time.Time) {
	c.mu.Lock()
	if c.lastNoRestartLogAt == nil {
		c.lastNoRestartLogAt = make(map[string]time.Time)
	}
	last, ok := c.lastNoRestartLogAt[reason]
	if ok && now.Sub(last) < 5*time.Minute {
		c.mu.Unlock()
		return
	}
	c.lastNoRestartLogAt[reason] = now
	c.mu.Unlock()

	if c.logInfo != nil {
		c.logInfo("直连 DNS 暂不切换：%s", reason)
	}
}

func (c *directDNSHealthChecker) CheckHealth() bool {
	c.mu.Lock()
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}

	force := c.forceReselect
	c.forceReselect = false

	var currentFP networkFingerprint
	if c.getFingerprint != nil {
		currentFP = c.getFingerprint()
	}

	shouldReselect := false
	needStabilizeSleep := false
	var oldFP networkFingerprint

	if !c.hasRecordedFingerprint {
		c.hasRecordedFingerprint = true
		c.lastFingerprint = currentFP
	} else if currentFP != c.lastFingerprint {
		oldFP = c.lastFingerprint
		c.lastFingerprint = currentFP
		shouldReselect = true
		needStabilizeSleep = true
	}

	if force {
		shouldReselect = true
	}

	// 网络变化或系统唤醒时解除“频繁切换暂停”，并清空重启记录。
	if shouldReselect && c.autoSwitchPaused {
		c.autoSwitchPaused = false
		c.restartHistory = nil
	}
	paused := c.autoSwitchPaused

	interval := c.checkCycleInterval
	if interval <= 0 {
		interval = 3
	}
	c.directDNSCycleCount++
	runPeriodicProbe := (c.directDNSCycleCount >= interval)
	if runPeriodicProbe {
		c.directDNSCycleCount = 0
	}
	c.mu.Unlock()

	if needStabilizeSleep {
		if c.logInfo != nil {
			c.logInfo("检测到网络变化：%d/%s → %d/%s", oldFP.IfIndex, oldFP.LocalIP, currentFP.IfIndex, currentFP.LocalIP)
		}
		if c.sleep != nil {
			c.sleep(3 * time.Second)
		}
	}

	var running string
	if c.runningDNS != nil {
		running = c.runningDNS()
	}

	periodicProbeRan := false
	periodicProbeOK := false
	if runPeriodicProbe && !paused && running != "" && c.probeDNS != nil {
		periodicProbeRan = true
		_, periodicProbeOK = c.probeDNS(running)
		c.mu.Lock()
		if periodicProbeOK {
			c.directDNSFailCount = 0
		} else {
			c.directDNSFailCount++
			if c.directDNSFailCount >= directDNSFailThreshold {
				c.directDNSFailCount = 0
				shouldReselect = true
			}
		}
		c.mu.Unlock()
	}

	if !shouldReselect {
		return false
	}

	// 重新选择
	runningReachable := false
	if periodicProbeRan && !periodicProbeOK {
		runningReachable = false
	} else if running != "" && c.probeDNS != nil {
		_, runningReachable = c.probeDNS(running)
	}

	// 当前直连 DNS 仍然可用：直接保持，不重新测速、不重启。
	if runningReachable {
		c.logNoRestartThrottled("当前直连 DNS 仍可用，暂不切换", now)
		c.mu.Lock()
		c.directDNSFailCount = 0
		c.mu.Unlock()
		return false
	}

	if c.invalidateCache != nil {
		c.invalidateCache()
	}

	var resolved string
	var resolvedSource string
	if c.resolvePlan != nil {
		resolved, resolvedSource = c.resolvePlan()
	}

	action := decideDirectDNSAction(running, runningReachable, resolved, resolvedSource)
	if action == directDNSKeep {
		reason := "直连 DNS 保持不变"
		if runningReachable {
			reason = "当前直连 DNS 仍可用，暂不切换"
		} else if resolvedSource == "default" {
			reason = "网络可能未连通，暂不切换"
		} else if resolved == running {
			reason = "新解析直连 DNS 与当前运行一致，暂不切换"
		}
		c.logNoRestartThrottled(reason, now)
		c.mu.Lock()
		c.directDNSFailCount = 0
		c.mu.Unlock()
		return false
	}

	// 需要重启时：遵守现有的 lastHealthRestart 20 秒间隔限制
	lastRestart := c.lastRestartTime()
	if !lastRestart.IsZero() && now.Sub(lastRestart) < 20*time.Second {
		c.logNoRestartThrottled("距离上次自动恢复过近，暂不切换", now)
		return false
	}

	// 每小时最多 directDNSMaxRestartsPerHour 次，超过后暂停自动切换
	c.mu.Lock()
	if c.recentRestartCountLocked(now) >= directDNSMaxRestartsPerHour {
		c.autoSwitchPaused = true
		c.directDNSFailCount = 0
		c.mu.Unlock()
		if c.logError != nil {
			c.logError("直连 DNS 频繁切换，已暂停自动切换，等待网络变化")
		}
		return false
	}
	c.mu.Unlock()

	// 持有 proxyOpMu（使用 TryLock）
	if c.tryLockProxyOp != nil {
		if !c.tryLockProxyOp() {
			c.mu.Lock()
			c.forceReselect = true
			c.mu.Unlock()
			return false
		}
		if c.unlockProxyOp != nil {
			defer c.unlockProxyOp()
		}
	}

	lastRestart = c.lastRestartTime()
	if !lastRestart.IsZero() && now.Sub(lastRestart) < 20*time.Second {
		c.logNoRestartThrottled("距离上次自动恢复过近，暂不切换", now)
		return false
	}

	if c.restart != nil {
		if err := c.restart(); err != nil {
			if c.logInfo != nil {
				c.logInfo("直连 DNS 切换重启失败：%v", err)
			}
			return false
		}
	}

	c.updateLastRestart(now)
	c.mu.Lock()
	c.directDNSFailCount = 0
	c.restartHistory = append(c.restartHistory, now)
	c.mu.Unlock()
	if c.logInfo != nil {
		c.logInfo("直连 DNS 已切换：%s → %s，已重启内核", running, resolved)
	}
	return true
}
