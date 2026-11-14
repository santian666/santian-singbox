package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

var directDNSFallbackCandidates = []string{"223.5.5.5", "119.29.29.29", "1.1.1.1", "8.8.8.8", "9.9.9.9"}

type directDNSProbeResult struct {
	server  string
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

func probeBest(candidates []string, probe func(server string) (time.Duration, bool)) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}
	ch := make(chan directDNSProbeResult, len(candidates))
	for _, c := range candidates {
		go func(srv string) {
			lat, ok := probe(srv)
			ch <- directDNSProbeResult{server: srv, latency: lat, ok: ok}
		}(c)
	}

	var bestServer string
	var bestLatency time.Duration = 1<<63 - 1
	found := false

	for i := 0; i < len(candidates); i++ {
		res := <-ch
		if res.ok {
			if !found || res.latency < bestLatency {
				bestLatency = res.latency
				bestServer = res.server
				found = true
			}
		}
	}
	return bestServer, found
}

func chooseDirectDNS(physical []string, fallbacks []string, probe func(server string) (time.Duration, bool)) (server string, source string) {
	if len(physical) > 0 {
		if best, ok := probeBest(physical, probe); ok {
			return best, "physical"
		}
	}
	if len(fallbacks) > 0 {
		if best, ok := probeBest(fallbacks, probe); ok {
			return best, "fallback"
		}
		return fallbacks[0], "default"
	}
	return "223.5.5.5", "default"
}

func resolveDirectDNSWithConfig(
	nowFunc func() time.Time,
	getIfIndex func() uint32,
	detectDNS func() []string,
	probe func(server string) (time.Duration, bool),
	fallbacks []string,
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

	var physical []string
	if detectDNS != nil {
		physical = detectDNS()
	}
	srv, src := chooseDirectDNS(physical, fallbacks, probe)

	directDNSCacheMu.Lock()
	directDNSCache = directDNSCacheEntry{
		ifIndex:   currentIfIndex,
		server:    srv,
		source:    src,
		expiresAt: now.Add(60 * time.Second),
	}
	directDNSCacheMu.Unlock()

	return srv, src
}

func resolveDirectDNS(logger *Logger) (string, string) {
	return resolveDirectDNSWithConfig(
		timeNowFunc,
		getDefaultRouteIfIndexFunc,
		detectPhysicalDNSServersFunc,
		probeDirectUDPDNSFunc,
		directDNSFallbackCandidates,
	)
}

func resolveDirectDNSPlan(logger *Logger) autoDNSPlan {
	server, source := resolveDirectDNS(logger)
	sourceDesc := "默认"
	switch source {
	case "physical":
		sourceDesc = "网卡"
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
