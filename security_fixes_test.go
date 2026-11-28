package main

import (
	"strings"
	"testing"
)

func TestSTUNPortsBlockedInTunStabilityRules(t *testing.T) {
	rules := buildTunStabilityRouteRules()
	var stunBlocked bool
	expectedPorts := []int{3478, 3479, 5349, 19302, 19305}
	for _, rule := range rules {
		if (rule["action"] == "reject" || rule["outbound"] == "block") && rule["network"] == "udp" {
			ports, ok := rule["port"].([]int)
			if !ok {
				continue
			}
			matched := 0
			for _, ep := range expectedPorts {
				for _, p := range ports {
					if p == ep {
						matched++
						break
					}
				}
			}
			if matched == len(expectedPorts) {
				stunBlocked = true
				break
			}
		}
	}
	if !stunBlocked {
		t.Fatalf("TUN 稳定性规则必须拦截 WebRTC STUN/TURN UDP 端口 %v，防止指纹浏览器 IP 泄漏", expectedPorts)
	}
}

func TestKeywordsDoNotContainShortJDOrQQ(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("Build config failed: %v", err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})

	for _, rule := range rules {
		if kw, ok := rule["domain_keyword"].([]string); ok {
			for _, k := range kw {
				if k == "jd" || k == "qq" {
					t.Fatalf("规则路由中不得出现过宽关键词短语 %q，以防误杀 jdk.java.net 等海外域名", k)
				}
			}
		}
	}

	dns := config["dns"].(map[string]interface{})
	dnsRules := dns["rules"].([]map[string]interface{})
	for _, rule := range dnsRules {
		if kw, ok := rule["domain_keyword"].([]string); ok {
			for _, k := range kw {
				if k == "jd" || k == "qq" {
					t.Fatalf("DNS 规则中不得出现过宽关键词短语 %q，以防误杀海外域名解析", k)
				}
			}
		}
	}
}

func TestAutoFailoverLockedToSameRegion(t *testing.T) {
	nodes := []Node{
		{ID: "node-us-1", Name: "美国 01 节点", GroupID: "group1", DelayMS: 150},
		{ID: "node-us-2", Name: "美国 02 优质", GroupID: "group1", DelayMS: 120},
		{ID: "node-hk-1", Name: "香港 01 高速", GroupID: "group1", DelayMS: 30},
		{ID: "node-jp-1", Name: "日本 01 节点", GroupID: "group1", DelayMS: 60},
	}
	groups := []NodeGroup{{ID: "group1", Name: "主力组"}}

	candidates, _, _, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "node-us-1")
	if err != nil {
		t.Fatalf("筛选故障转移候选节点失败：%v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("应仅匹配同为美国地区的备用节点，实际匹配数量：%d", len(candidates))
	}
	if candidates[0].ID != "node-us-2" {
		t.Fatalf("应优选同地区的 node-us-2，实际得到：%s", candidates[0].ID)
	}
}

func TestAutoFailoverRejectsCrossRegionWhenNoSameRegion(t *testing.T) {
	nodes := []Node{
		{ID: "node-us-1", Name: "美国 01 节点", GroupID: "group1", DelayMS: 150},
		{ID: "node-hk-1", Name: "香港 01 高速", GroupID: "group1", DelayMS: 30},
	}
	groups := []NodeGroup{{ID: "group1", Name: "主力组"}}

	_, _, _, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "node-us-1")
	if err == nil {
		t.Fatal("没有同地区节点时应拒绝跨国切换以防账号风控，但未返回错误")
	}
	if !strings.Contains(err.Error(), "禁止跨国自动切换") {
		t.Fatalf("返回的错误信息不符合预期：%v", err)
	}
}

func TestAppProxyRouteRuleMatchesAppAndDirectory(t *testing.T) {
	settings := Settings{
		AppProxy: true,
		AppProxyTargets: []AppProxyTarget{
			{Type: "app", Path: `C:\Program Files\AdsPower\AdsPower.exe`, Enabled: true},
		},
	}
	rule := appProxyRouteRule(settings)
	if rule == nil {
		t.Fatal("应生成应用代理规则")
	}
	paths, ok := rule["process_path"].([]string)
	if !ok || len(paths) == 0 {
		t.Fatal("应包含进程绝对路径")
	}
	regexes, ok := rule["process_path_regex"].([]string)
	if !ok || len(regexes) == 0 {
		t.Fatal("应包含目录正则以自动覆盖子进程与内核进程")
	}

	portableSettings := Settings{
		AppProxy: true,
		AppProxyTargets: []AppProxyTarget{
			{Type: "app", Path: `C:\Users\test\Downloads\tool.exe`, Enabled: true},
		},
	}
	portableRule := appProxyRouteRule(portableSettings)
	if portableRule == nil {
		t.Fatal("应生成便携应用代理规则")
	}
	if _, ok := portableRule["process_path"].([]string); !ok {
		t.Fatal("便携应用应包含 process_path")
	}
	if _, hasRegex := portableRule["process_path_regex"]; hasRegex {
		t.Fatalf("位于 Downloads 等共享目录的应用不得生成整目录正则：%v", portableRule["process_path_regex"])
	}
}

func TestTunAndAppProxyCoexistence(t *testing.T) {
	settings := Settings{
		Tun:         true,
		AppProxy:    true,
		SystemProxy: true,
	}
	normalizeSettings(&settings)
	if !settings.Tun {
		t.Fatal("TUN 应当保持开启")
	}
	if !settings.AppProxy {
		t.Fatal("AppProxy 现已支持与 TUN 共存，不应被强制关闭")
	}
	if settings.SystemProxy {
		t.Fatal("开启 TUN 时系统代理应当被互斥关闭")
	}
}
