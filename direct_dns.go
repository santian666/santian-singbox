package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

var directDNSFallbackCandidates = []string{"223.5.5.5", "119.29.29.29", "1.1.1.1", "8.8.8.8", "9.9.9.9"}

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
	detectPhysicalDNSServersFunc = detectPhysicalDNSServers
	probeDirectUDPDNSFunc        = probeDirectUDPDNS
)

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
		Remote: remoteDNSCandidates[0],
	}
}
