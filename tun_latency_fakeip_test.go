package main

import (
	"path/filepath"
	"testing"
)

func TestNodeServerDomainsBypassFakeIP(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	state.Nodes = []Node{
		{
			ID:      "node-1",
			Name:    "Test Node 1",
			Type:    "shadowsocks",
			Address: "ss.tiktokstudio.io",
			Port:    33407,
		},
		{
			ID:      "node-2",
			Name:    "Test Node 2",
			Type:    "vmess",
			Address: "144.56.19.142",
			Port:    443,
			Outbound: map[string]interface{}{
				"server": "hk.server.net",
			},
		},
	}

	domains := extractNodeServerDomains(state.Nodes)
	if !stringSliceContains(domains, "ss.tiktokstudio.io") {
		t.Fatalf("extractNodeServerDomains 应包含 ss.tiktokstudio.io，实际：%v", domains)
	}
	if !stringSliceContains(domains, "hk.server.net") {
		t.Fatalf("extractNodeServerDomains 应包含 hk.server.net，实际：%v", domains)
	}
	if stringSliceContains(domains, "144.56.19.142") {
		t.Fatalf("extractNodeServerDomains 不应包含 IP 地址，实际：%v", domains)
	}

	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成配置失败：%v", err)
	}

	dns, ok := config["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 dns 配置")
	}
	rules, _ := dns["rules"].([]map[string]interface{})

	fakeIPIndex := -1
	nodeDomainRuleIndex := -1
	for idx, rule := range rules {
		if rule["server"] == "fakeip" && rule["query_type"] != nil {
			fakeIPIndex = idx
		}
		if rule["server"] == "china" && rule["domain"] != nil {
			if doms, ok := rule["domain"].([]string); ok && stringSliceContains(doms, "ss.tiktokstudio.io") {
				nodeDomainRuleIndex = idx
			}
		}
	}

	if nodeDomainRuleIndex == -1 {
		t.Fatalf("未找到节点域名解析规则 (china server)")
	}
	if fakeIPIndex == -1 {
		t.Fatalf("未找到 fakeip 规则")
	}
	if nodeDomainRuleIndex >= fakeIPIndex {
		t.Fatalf("节点服务器域名必须在 fakeip 之前由 china DNS 解析真实 IP，实际 nodeDomainRuleIndex=%d, fakeIPIndex=%d", nodeDomainRuleIndex, fakeIPIndex)
	}
}

func TestTunCacheFileConfig(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	cachePath := filepath.Join(t.TempDir(), "cache.db")

	config, err := BuildSingBoxConfig(state, state.Nodes[0], cachePath)
	if err != nil {
		t.Fatalf("生成 TUN 配置失败：%v", err)
	}

	exp, ok := config["experimental"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 experimental 配置：%v", config)
	}

	cacheFile, ok := exp["cache_file"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 cache_file 配置：%v", exp)
	}

	if cacheFile["enabled"] != true {
		t.Fatalf("cache_file 应启用，实际：%v", cacheFile["enabled"])
	}

	if cacheFile["store_fakeip"] != true {
		t.Fatalf("TUN 模式下 cache_file.store_fakeip 必须为 true，以避免重启后丢 FakeIP 导致 missing fakeip record：%v", cacheFile["store_fakeip"])
	}

	if cacheFile["path"] != cachePath {
		t.Fatalf("cache_file.path 应为 %q，实际：%v", cachePath, cacheFile["path"])
	}
}

func TestTunBlocksQUICPort443(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true

	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 配置失败：%v", err)
	}

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 route 配置：%v", config)
	}

	rules, _ := route["rules"].([]map[string]interface{})
	foundQUICBlock := false
	for _, rule := range rules {
		if rule["outbound"] != "block" {
			continue
		}
		inbounds := configStringValues(rule["inbound"])
		if !stringSliceContains(inbounds, "tun-in") {
			continue
		}
		if rule["network"] != "udp" {
			continue
		}
		ports, _ := rule["port"].([]int)
		for _, p := range ports {
			if p == 443 {
				foundQUICBlock = true
				break
			}
		}
	}

	if !foundQUICBlock {
		t.Fatal("TUN 稳定性规则必须阻断 UDP 443 (QUIC)，防止浏览器因代理 UDP 延迟/丢包导致 3 秒超时卡顿")
	}
}

func TestDNSRulesOrderPrioritizesDomesticDirectOverFakeIP(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	state.Settings.DirectDomains = "mytest.example.com\n1.1.1.1\n192.168.1.0/24"

	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 配置失败：%v", err)
	}

	dns, ok := config["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 dns 配置：%v", config)
	}

	rules, _ := dns["rules"].([]map[string]interface{})

	fakeIPIndex := -1
	chinaIndex := -1
	directDomainIndex := -1
	cnSuffixIndex := -1

	httpsRejectIndex := -1
	for idx, rule := range rules {
		if stringSliceContains(configStringValues(rule["query_type"]), "HTTPS") && rule["action"] == "reject" {
			httpsRejectIndex = idx
		}
		if rule["server"] == "fakeip" && rule["query_type"] != nil {
			fakeIPIndex = idx
		}
		if rule["server"] == "china" {
			if rule["domain"] != nil {
				directDomainIndex = idx
			} else if rule["rule_set"] == "loyalsoldier-china" {
				chinaIndex = idx
			} else if stringSliceContains(configStringValues(rule["domain_suffix"]), "cn") {
				cnSuffixIndex = idx
			}
		}
	}

	if httpsRejectIndex == -1 {
		t.Fatalf("TUN FakeDNS 必须包含立即拒绝 Type 65 (HTTPS) DNS 查询的规则，防止浏览器首包卡顿 2-5 秒：%v", rules)
	}
	if fakeIPIndex == -1 {
		t.Fatalf("未找到 TUN fakeip 兜底规则：%v", rules)
	}
	if directDomainIndex == -1 || directDomainIndex >= fakeIPIndex {
		t.Fatalf("用户直连域名必须在 fakeip 规则之前由 china DNS 解析真实 IP，实际 directDomainIndex=%d, fakeIPIndex=%d", directDomainIndex, fakeIPIndex)
	}
	if chinaIndex == -1 || chinaIndex >= fakeIPIndex {
		t.Fatalf("国内规则集 loyalsoldier-china 必须在 fakeip 规则之前，避免国内网站被赋予 FakeIP，实际 chinaIndex=%d, fakeIPIndex=%d", chinaIndex, fakeIPIndex)
	}
	if cnSuffixIndex == -1 || cnSuffixIndex >= fakeIPIndex {
		t.Fatalf("国内 .cn 域名规则必须在 fakeip 规则之前，实际 cnSuffixIndex=%d, fakeIPIndex=%d", cnSuffixIndex, fakeIPIndex)
	}

	// 验证路由规则同时包含 domain、ip_cidr、geoip-cn 和 53 端口直劫持
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 route 配置")
	}
	routeRules, _ := route["rules"].([]map[string]interface{})
	var foundDirectDomain, foundDirectIP, foundGeoIPCN, foundPort53Hijack bool
	for _, r := range routeRules {
		if r["action"] == "hijack-dns" {
			ports, _ := r["port"].([]int)
			for _, p := range ports {
				if p == 53 {
					foundPort53Hijack = true
				}
			}
		}
		if r["outbound"] == "direct" {
			if doms, ok := r["domain"].([]string); ok && stringSliceContains(doms, "mytest.example.com") {
				foundDirectDomain = true
			}
			if cidrs, ok := r["ip_cidr"].([]string); ok && stringSliceContains(cidrs, "1.1.1.1/32") && stringSliceContains(cidrs, "192.168.1.0/24") {
				foundDirectIP = true
			}
			if r["rule_set"] == "geoip-cn" {
				foundGeoIPCN = true
			}
		}
	}
	if !foundPort53Hijack {
		t.Fatalf("路由规则中必须显式劫持端口 53 DNS 请求，防止 DNS 泄漏")
	}
	if !foundDirectDomain {
		t.Fatalf("路由规则中缺少自定义直连域名")
	}
	if !foundDirectIP {
		t.Fatalf("路由规则中缺少自定义直连 IP/CIDR")
	}
	if !foundGeoIPCN {
		t.Fatalf("路由规则中必须包含 geoip-cn 中国 IP 直连规则，防止国内流量被误代理")
	}
}

func TestNodeServerIPsExcludedFromTunRoute(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	state.Nodes = []Node{
		{
			ID:      "node-ip-1",
			Name:    "IP Node 1",
			Type:    "anytls",
			Address: "209.248.57.144",
			Port:    20080,
		},
		{
			ID:      "node-domain-1",
			Name:    "Domain Node 1",
			Type:    "shadowsocks",
			Address: "ss.tiktokstudio.io",
			Port:    33407,
		},
	}

	ips := extractNodeServerIPs(state.Nodes)
	if !stringSliceContains(ips, "209.248.57.144/32") {
		t.Fatalf("extractNodeServerIPs 应包含 209.248.57.144/32，实际：%v", ips)
	}

	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成配置失败：%v", err)
	}

	inbounds, ok := config["inbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("缺少 inbounds 配置")
	}

	var foundExclude bool
	for _, in := range inbounds {
		if in["tag"] == "tun-in" {
			excludes := configStringValues(in["route_exclude_address"])
			if stringSliceContains(excludes, "209.248.57.144/32") {
				foundExclude = true
				break
			}
		}
	}
	if !foundExclude {
		t.Fatalf("TUN 入站 route_exclude_address 必须包含节点服务器 IP 209.248.57.144/32，以避开内核路由拦截")
	}
}

