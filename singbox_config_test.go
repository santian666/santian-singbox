package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildSingBoxConfigGlobalHasNoDirectRules(t *testing.T) {
	state := testConfigState("global")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成全局配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if route["final"] != "proxy" {
		t.Fatalf("全局模式 final 应为 proxy，实际：%v", route["final"])
	}
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "direct" && ruleAppliesToMode(rule, "global") {
			t.Fatalf("全局模式不应包含直连分流规则：%v", rule)
		}
	}
	dns := config["dns"].(map[string]interface{})
	for _, rule := range dns["rules"].([]map[string]interface{}) {
		if rule["server"] == "local" && ruleAppliesToMode(rule, "global") {
			t.Fatalf("全局模式 DNS 不应包含本地分流规则：%v", rule)
		}
	}
}

func TestConfigModeSummaryUsesLightweightSettingsOnly(t *testing.T) {
	state := testConfigState("rule")
	state.SelectedID = ""
	state.Nodes = nil
	summary := configModeSummary(state)
	if summary.Mode != "rule" || !summary.HasDirect {
		t.Fatalf("规则模式摘要应轻量返回规则状态：%+v", summary)
	}

	state.Settings.Mode = "global"
	summary = configModeSummary(state)
	if summary.Mode != "global" || summary.HasDirect || len(summary.RuleSets) > 0 {
		t.Fatalf("全局模式摘要不应包含规则集或直连命中：%+v", summary)
	}
}

func TestBuildSingBoxConfigRuleHasDirectRules(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成规则配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; !ok {
		t.Fatal("规则模式应包含规则集")
	}
	foundDirect := false
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "direct" {
			foundDirect = true
			break
		}
	}
	if !foundDirect {
		t.Fatal("规则模式应包含直连分流规则")
	}
	if config["log"].(map[string]interface{})["level"] != "info" {
		t.Fatalf("非 TUN 默认内核日志应保持 info，实际：%v", config["log"])
	}
}

func TestBuildSingBoxConfigDoesNotDirectImportedNodeServers(t *testing.T) {
	tests := []struct {
		name        string
		systemProxy bool
		tun         bool
	}{
		{name: "系统代理全局", systemProxy: true},
		{name: "TUN 全局", tun: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := testConfigState("global")
			state.Settings.SystemProxy = test.systemProxy
			state.Settings.Tun = test.tun
			state.Nodes[0].Address = "144.56.19.142"
			state.Nodes[0].Port = 7778
			state.Nodes[0].Outbound["server"] = "144.56.19.142"
			state.Nodes[0].Outbound["server_port"] = 7778
			config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
			if err != nil {
				t.Fatalf("生成全局配置失败：%v", err)
			}
			rules := config["route"].(map[string]interface{})["rules"].([]map[string]interface{})
			for _, rule := range rules {
				if rule["outbound"] != "direct" || rule["port"] != 7778 {
					continue
				}
				if cidrs, ok := rule["ip_cidr"].([]string); ok && stringSliceContains(cidrs, "144.56.19.142/32") {
					t.Fatalf("已导入节点地址不得被普通流量无条件直连：%v", rule)
				}
			}
			if test.tun && !hasDirectRouteForProcessName(config, "sing-box.exe") {
				t.Fatal("TUN 防回环应仅保留 sing-box 进程直连规则")
			}
		})
	}
}

func TestBuildSingBoxStartupConfigSkipsRuleSets(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxStartupConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成启动配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; ok {
		t.Fatal("启动配置不应包含规则集，避免代理入口启动前被规则加载阻塞")
	}
	foundCNDirect := false
	foundRuleSetRemoteResolve := false
	foundRemoteResolve := false
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "direct" && rule["rule_set"] == nil {
			foundCNDirect = true
		}
		if rule["action"] == "resolve" {
			switch rule["server"] {
			case "remote":
				foundRemoteResolve = true
				if rule["rule_set"] != nil {
					foundRuleSetRemoteResolve = true
				}
			}
		}
	}
	if !foundCNDirect {
		t.Fatal("启动配置应保留内置直连规则")
	}
	if foundRemoteResolve {
		t.Fatalf("启动配置不应依赖远程 DNS 解析，避免代理入口启动闭环：%v", route["rules"])
	}
	if foundRuleSetRemoteResolve {
		t.Fatal("启动配置不应引用规则集做远程解析，避免规则加载阻塞启动")
	}
	resolver := route["default_domain_resolver"].(map[string]interface{})
	if resolver["server"] != "local" {
		t.Fatalf("启动配置默认解析应使用本地 DNS，实际：%v", resolver)
	}
	dns := config["dns"].(map[string]interface{})
	if dns["final"] != "local" {
		t.Fatalf("启动配置 DNS 默认应使用本地解析，实际：%v", dns["final"])
	}
	if !hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "china") {
		t.Fatal("规则模式应包含国内 DNS 服务器用于中国站点解析")
	}
	if hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "remote") {
		t.Fatalf("启动配置不应初始化远程 DNS：%v", dns["servers"])
	}
	foundChinaDNS := false
	foundRemoteDNS := false
	for _, rule := range dns["rules"].([]map[string]interface{}) {
		if rule["rule_set"] != nil && rule["server"] == "china" {
			foundChinaDNS = true
		}
		if rule["rule_set"] != nil && rule["server"] == "remote" {
			foundRemoteDNS = true
		}
	}
	if foundChinaDNS || foundRemoteDNS {
		t.Fatalf("启动 DNS 不应引用规则集：%v", dns["rules"])
	}
}

func TestBuildSingBoxConfigIncludesRuleSets(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成完整配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; !ok {
		t.Fatal("完整配置应包含规则集")
	}
	for _, ruleSet := range route["rule_set"].([]map[string]interface{}) {
		if ruleSet["type"] != "local" || ruleSet["format"] != "source" {
			t.Fatalf("规则集应使用本地 source 文件：%v", ruleSet)
		}
	}
	dns := config["dns"].(map[string]interface{})
	if dns["final"] != "remote" {
		t.Fatalf("完整规则模式 DNS 默认应使用远程解析，实际：%v", dns["final"])
	}
}

func TestRuleSourceUsesLoyalsoldierRulesets(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成完整配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	ruleSets := route["rule_set"].([]map[string]interface{})
	wantFiles := map[string]string{
		"loyalsoldier-direct": "loyalsoldier-direct.json",
		"loyalsoldier-china":  "loyalsoldier-china.json",
		"geoip-cn":            "geoip-cn.json",
		"loyalsoldier-proxy":  "loyalsoldier-proxy.json",
	}
	for _, ruleSet := range ruleSets {
		tag, _ := ruleSet["tag"].(string)
		if tag == "loyalsoldier-reject" {
			t.Fatalf("规则模式不应加载 reject 黑名单规则，避免 TikTok 等网站被禁用：%v", ruleSet)
		}
		rulePath, _ := ruleSet["path"].(string)
		if wantFiles[tag] == "" {
			t.Fatalf("规则集 tag 不应变化，避免路由或 DNS 引用失配：%v", ruleSet)
		}
		if !strings.HasSuffix(filepath.ToSlash(rulePath), "/"+wantFiles[tag]) {
			t.Fatalf("%s 应使用 Loyalsoldier 本地规则文件，实际：%s", tag, rulePath)
		}
		if ruleSet["type"] != "local" || ruleSet["format"] != "source" {
			t.Fatalf("规则集应使用本地 source 文件：%v", ruleSet)
		}
		delete(wantFiles, tag)
	}
	if len(wantFiles) > 0 {
		t.Fatalf("缺少规则集：%v", wantFiles)
	}
	assertReferencedRuleSetsExist(t, route, config["dns"].(map[string]interface{}))
}

func TestLoyalsoldierRejectRulesetIsNotLoadedByDefault(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成完整配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	for _, ruleSet := range route["rule_set"].([]map[string]interface{}) {
		if ruleSet["tag"] == "loyalsoldier-reject" {
			t.Fatalf("规则模式不应加载 reject 黑名单规则集，避免 TikTok 等网站被禁用：%v", ruleSet)
		}
	}
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["rule_set"] == "loyalsoldier-reject" {
			t.Fatalf("默认路由不应引用 reject 黑名单规则集，避免 ads.tiktok.com 等可访问站点被阻断：%v", rule)
		}
	}
}

func TestTikTokIsNotBlockedInProxyModes(t *testing.T) {
	cases := []struct {
		name string
		mode string
		tun  bool
	}{
		{name: "系统代理全局", mode: "global"},
		{name: "系统代理规则", mode: "rule"},
		{name: "TUN 全局", mode: "global", tun: true},
		{name: "TUN 规则", mode: "rule", tun: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			state := testConfigState(item.mode)
			state.Settings.Tun = item.tun
			config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
			if err != nil {
				t.Fatalf("生成配置失败：%v", err)
			}
			route := config["route"].(map[string]interface{})
			for _, ruleSet := range configRules(route["rule_set"]) {
				if ruleSet["tag"] == "loyalsoldier-reject" {
					t.Fatalf("%s 不应加载 TikTok 所在的 reject 黑名单规则集：%v", item.name, ruleSet)
				}
			}
			for _, rule := range configRules(route["rules"]) {
				if rule["rule_set"] == "loyalsoldier-reject" {
					t.Fatalf("%s 不应引用 TikTok 所在的 reject 黑名单规则集：%v", item.name, rule)
				}
				if rule["outbound"] == "block" && !isTunStabilityBlockRule(rule) && !isLocalMetadataBlockRule(rule) {
					t.Fatalf("%s 不应包含普通网站拉黑规则：%v", item.name, rule)
				}
			}
		})
	}
}

func TestNormalizeSettingsMigratesLegacyRuleSources(t *testing.T) {
	for _, source := range []string{"", ruleSourceV2rayN, ruleSourceSagerNet, ruleSourceMetaCube, "unknown"} {
		settings := Settings{Mode: "rule", RuleSource: source}
		normalizeSettings(&settings)
		if settings.RuleSource != ruleSourceLoyalsoldier {
			t.Fatalf("历史或未知规则源应迁移到 Loyalsoldier，输入 %q 实际 %q", source, settings.RuleSource)
		}
	}
	settings := Settings{Mode: "rule", RuleSource: ruleSourceBuiltin}
	normalizeSettings(&settings)
	if settings.RuleSource != ruleSourceBuiltin {
		t.Fatalf("内置规则源不应被迁移，实际：%s", settings.RuleSource)
	}
}

func TestNormalizeSettingsKeepsProxyEntrancesExclusive(t *testing.T) {
	settings := Settings{Mode: "rule", SystemProxy: true, Tun: true}
	normalizeSettings(&settings)
	if settings.SystemProxy || !settings.Tun || settings.AppProxy {
		t.Fatalf("系统代理和 TUN 同时开启时应只保留 TUN：%+v", settings)
	}

	settings = Settings{Mode: "rule", SystemProxy: true, Tun: false}
	normalizeSettings(&settings)
	if !settings.SystemProxy || settings.Tun || settings.AppProxy {
		t.Fatalf("仅开启系统代理时不应被关闭：%+v", settings)
	}

	settings = Settings{Mode: "rule", SystemProxy: true, AppProxy: true}
	normalizeSettings(&settings)
	if settings.SystemProxy || settings.Tun || !settings.AppProxy {
		t.Fatalf("应用接管应与系统代理互斥：%+v", settings)
	}

	current := Settings{Mode: "rule", AppProxy: true}
	next := current
	next.SystemProxy = true
	enforceProxyEntryTransition(current, &next)
	if !next.SystemProxy || next.Tun || next.AppProxy {
		t.Fatalf("切换到系统代理时应关闭应用接管：%+v", next)
	}
}

func TestProxyEntranceSwitchRequiresTunCleanupRestart(t *testing.T) {
	current := testConfigState("rule").Settings
	current.SystemProxy = true
	current.Tun = false
	next := current
	next.SystemProxy = false
	next.Tun = true
	if !needsRestartForRunningSettings(current, next) {
		t.Fatal("系统代理切换到 TUN 应重启内核")
	}
	if !shouldCleanupTunOnRunningRestart(current, next) {
		t.Fatal("系统代理切换到 TUN 应清理旧 TUN 残留")
	}

	current = next
	next.SystemProxy = true
	next.Tun = false
	normalizeSettings(&next)
	if !needsRestartForRunningSettings(current, next) {
		t.Fatal("TUN 切换到系统代理应重启内核")
	}
	if !shouldCleanupTunOnRunningRestart(current, next) {
		t.Fatal("TUN 切换到系统代理应清理旧 TUN 网卡")
	}
}

func TestDisableStoredProxyEntrancesClearsTunAfterFailedStartup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("创建配置失败：%v", err)
	}
	settings := testConfigState("rule").Settings
	settings.Tun = true
	settings.SystemProxy = false
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	if err := app.disableStoredProxyEntrances("测试"); err != nil {
		t.Fatalf("关闭代理入口失败：%v", err)
	}
	next := store.Snapshot().Settings
	if next.Tun || next.SystemProxy || next.ProxyMap || next.AppProxy {
		t.Fatalf("启动失败回滚应关闭全部代理入口：%+v", next)
	}
	if next.Mode != "rule" {
		t.Fatalf("关闭代理入口不应重置上次使用模式：%+v", next)
	}
}

func TestDisableStoredProxyEntrancesKeepsProxyMapGroups(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("创建配置失败：%v", err)
	}
	settings := testConfigState("rule").Settings
	settings.ProxyMap = true
	settings.ProxyMapGroupIDs = []string{"default"}
	settings.ProxyMapGroupID = "default"
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	if err := app.disableStoredProxyEntrances("测试"); err != nil {
		t.Fatalf("关闭代理入口失败：%v", err)
	}
	next := store.Snapshot().Settings
	if next.ProxyMap {
		t.Fatalf("关闭软件应关闭代理映射入口：%+v", next)
	}
	if len(next.ProxyMapGroupIDs) != 1 || next.ProxyMapGroupIDs[0] != "default" || next.ProxyMapGroupID != "default" {
		t.Fatalf("关闭软件不应清除已选择的映射分组：%+v", next)
	}
}

func TestNormalizeCoreVersionTag(t *testing.T) {
	cases := map[string]string{
		"sing-box version 1.13.12": "1.13.12",
		"v1.13.12":                 "1.13.12",
		" 1.13.12 ":                "1.13.12",
		"未知":                       "",
		"未安装":                      "",
	}
	for input, want := range cases {
		if got := normalizeCoreVersionTag(input); got != want {
			t.Fatalf("版本号标准化错误：输入 %q，期望 %q，实际 %q", input, want, got)
		}
	}
}

func TestBuildSingBoxConfigKreaUsesProxyDNSAndRoute(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成完整配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	resolver := route["default_domain_resolver"].(map[string]interface{})
	if resolver["server"] != "local" {
		t.Fatalf("代理入口域名必须本地解析，避免远程 DNS 走代理形成闭环，实际：%v", resolver)
	}
	if hasDirectDomainRule(route["rules"].([]map[string]interface{}), "krea.ai") {
		t.Fatalf("krea.ai 不应命中内置直连规则：%v", route["rules"])
	}
	dns := config["dns"].(map[string]interface{})
	if dns["final"] != "remote" {
		t.Fatalf("krea.ai 这类未知海外域名 DNS 应默认远程解析，实际：%v", dns["final"])
	}
}

func TestBuildSingBoxConfigRuleAppliesCustomDirectDomains(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.DirectDomains = "example.com"
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成完整配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if !hasDirectDomainRule(route["rules"].([]map[string]interface{}), "example.com") {
		t.Fatalf("规则模式必须把自定义直连域名写入路由规则：%v", route["rules"])
	}
	dns := config["dns"].(map[string]interface{})
	if !hasDNSRuleForDomainServer(dns["rules"].([]map[string]interface{}), "example.com", "china") {
		t.Fatalf("规则模式必须把自定义直连域名写入 DNS 直连规则：%v", dns["rules"])
	}
}

func TestBuildSingBoxConfigAvoidsRemoteDNSBootstrapLoop(t *testing.T) {
	state := testConfigState("rule")
	state.Nodes[0].Address = "ss.tiktokbusiness.io"
	state.Nodes[0].Outbound["server"] = "ss.tiktokbusiness.io"
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成域名节点配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	resolver := route["default_domain_resolver"].(map[string]interface{})
	if resolver["server"] == "remote" {
		t.Fatalf("代理节点是域名时，默认解析器不能使用远程 DNS：%v", resolver)
	}
	dns := config["dns"].(map[string]interface{})
	remoteDNS := findConfigItem(dns["servers"].([]map[string]interface{}), "tag", "remote")
	if remoteDNS["detour"] == "proxy" && resolver["server"] == "remote" {
		t.Fatalf("不能生成 remote DNS 经 proxy 且默认解析也用 remote 的闭环配置：remote=%v resolver=%v", remoteDNS, resolver)
	}
}

func TestBuildSingBoxStartupConfigKreaUsesLocalDNS(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxStartupConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成启动配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	resolver := route["default_domain_resolver"].(map[string]interface{})
	if resolver["server"] != "local" {
		t.Fatalf("启动阶段未知域名应先本地解析，实际：%v", resolver)
	}
	dns := config["dns"].(map[string]interface{})
	if dns["final"] != "local" {
		t.Fatalf("启动阶段 DNS 应默认本地解析，实际：%v", dns["final"])
	}
}

func TestBuildSingBoxConfigTunInbound(t *testing.T) {
	state := testConfigState("global")
	state.Settings.Tun = true
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 配置失败：%v", err)
	}
	inbounds := config["inbounds"].([]map[string]interface{})
	foundTun := false
	for _, inbound := range inbounds {
		if inbound["type"] != "tun" {
			continue
		}
		foundTun = true
		if inbound["interface_name"] != "SanTian" {
			t.Fatalf("TUN 网卡名应使用 SanTian，实际：%v", inbound["interface_name"])
		}
		if inbound["auto_route"] != true {
			t.Fatalf("TUN 应开启 auto_route：%v", inbound)
		}
		if inbound["strict_route"] != true {
			t.Fatalf("TUN 应开启 strict_route：%v", inbound)
		}
		if inbound["endpoint_independent_nat"] != true {
			t.Fatalf("TUN 应开启 endpoint_independent_nat：%v", inbound)
		}
		if mtu := intFromAny(inbound["mtu"]); mtu != 1500 {
			t.Fatalf("TUN MTU 应使用通用值 1500，实际：%d", mtu)
		}
		excludes, _ := inbound["route_exclude_address"].([]string)
		for _, cidr := range []string{"169.254.0.0/16", "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4"} {
			if !stringSliceContains(excludes, cidr) {
				t.Fatalf("TUN 应排除无意义或本地网段 %s，实际：%v", cidr, excludes)
			}
		}
	}
	if !foundTun {
		t.Fatal("开启 TUN 后应生成 tun inbound")
	}
	logConfig := config["log"].(map[string]interface{})
	if logConfig["level"] != "info" {
		t.Fatalf("TUN 默认内核日志应为 info，实际：%v", logConfig["level"])
	}
	state.Settings.CoreLogLevel = "warn"
	debugConfig, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 配置失败：%v", err)
	}
	if debugConfig["log"].(map[string]interface{})["level"] != "warn" {
		t.Fatalf("允许切换日志级别为 warn，实际：%v", debugConfig["log"])
	}
	dns := config["dns"].(map[string]interface{})
	if dns["strategy"] != "ipv4_only" {
		t.Fatalf("TUN 仅配置 IPv4 地址时 DNS 应使用 ipv4_only，实际：%v", dns["strategy"])
	}
	foundTunFakeIP := false
	for _, rule := range dns["rules"].([]map[string]interface{}) {
		if rule["server"] == "fakeip" {
			inbounds, _ := rule["inbound"].([]string)
			if stringSliceContains(inbounds, "tun-in") {
				foundTunFakeIP = true
			}
		}
	}
	if !foundTunFakeIP {
		t.Fatalf("TUN FakeIP DNS 规则应只绑定 tun-in，避免影响系统代理：%v", dns["rules"])
	}
	if !hasTunStabilityBlockRule(config) {
		t.Fatalf("TUN 应包含广播和组播阻断规则：%v", config["route"])
	}
}

func TestBuildSingBoxConfigTunGlobalKeepsInactiveRuleRules(t *testing.T) {
	state := testConfigState("global")
	state.Settings.Tun = true
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 全局配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if len(configRules(route["rule_set"])) == 0 {
		t.Fatalf("完整运行配置应预载规则集，以便通过 Clash API 快速切换规则模式：%v", route)
	}
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if _, ok := rule["rule_set"]; ok {
			if ruleAppliesToMode(rule, "global") || !ruleAppliesToMode(rule, "rule") {
				t.Fatalf("全局配置中的规则集规则必须只在 rule 模式生效：%v", rule)
			}
		}
	}
	dns := config["dns"].(map[string]interface{})
	for _, rule := range dns["rules"].([]map[string]interface{}) {
		if _, ok := rule["rule_set"]; ok {
			if ruleAppliesToMode(rule, "global") || !ruleAppliesToMode(rule, "rule") {
				t.Fatalf("全局配置中的 DNS 规则集规则必须只在 rule 模式生效：%v", rule)
			}
		}
	}
	summary := configModeFromConfig(config, "global")
	if summary.HasDirect || len(summary.RuleSets) > 0 {
		t.Fatalf("预载规则不应被识别为全局模式残留：%+v", summary)
	}
	if dns["final"] == "remote" && !hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "remote") {
		t.Fatalf("TUN 全局模式 DNS final 使用 remote 时必须创建 remote server：%v", dns)
	}
	assertDNSRuleServersExist(t, dns)
}

func TestNormalizeSettingsKeepsSystemProxyAndTunExclusive(t *testing.T) {
	settings := Settings{Mode: "rule", SystemProxy: true, Tun: true}
	normalizeSettings(&settings)
	if settings.SystemProxy {
		t.Fatalf("TUN 和系统代理应保持二选一，开启 TUN 时必须关闭系统代理：%+v", settings)
	}
	if !settings.Tun {
		t.Fatalf("开启 TUN 后不应被互斥逻辑关闭：%+v", settings)
	}
}

func TestBuildSingBoxConfigDNSReferencesExistingServers(t *testing.T) {
	cases := []struct {
		name string
		mode string
		tun  bool
	}{
		{name: "系统代理规则", mode: "rule"},
		{name: "系统代理全局", mode: "global"},
		{name: "TUN规则", mode: "rule", tun: true},
		{name: "TUN全局", mode: "global", tun: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := testConfigState(tc.mode)
			state.Settings.Tun = tc.tun
			state.Settings.SystemProxy = !tc.tun
			config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
			if err != nil {
				t.Fatalf("生成配置失败：%v", err)
			}
			assertDNSRuleServersExist(t, config["dns"].(map[string]interface{}))
		})
	}
}

func TestBuildSingBoxStartupConfigTunUsesFakeIPWithoutRemoteRuleSets(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.Tun = true
	config, err := BuildSingBoxStartupConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUN 启动配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("TUN 启动配置不应初始化规则集：%v", route["rule_set"])
	}
	dns := config["dns"].(map[string]interface{})
	if !hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "fakeip") {
		t.Fatalf("TUN 启动配置应启用 fakeip，避免本地 DNS 污染：%v", dns["servers"])
	}
}

func TestBuildSingBoxStartupConfigSystemProxyUsesLocalDNSWithoutTun(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.SystemProxy = true
	state.Settings.Tun = false
	config, err := BuildSingBoxStartupConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成系统代理启动配置失败：%v", err)
	}
	if hasTunInbound(config) {
		t.Fatalf("仅开启系统代理不应生成 TUN 入站：%v", config["inbounds"])
	}
	inbounds := config["inbounds"].([]map[string]interface{})
	if !hasConfigItem(inbounds, "tag", "mixed-in") {
		t.Fatalf("系统代理启动配置应保留 mixed-in 入站：%v", inbounds)
	}
	route := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("系统代理启动配置不应初始化规则集：%v", route["rule_set"])
	}
	resolver := route["default_domain_resolver"].(map[string]interface{})
	if resolver["server"] != "local" {
		t.Fatalf("系统代理启动配置默认解析应使用本地 DNS，实际：%v", resolver)
	}
	dns := config["dns"].(map[string]interface{})
	if dns["final"] != "local" {
		t.Fatalf("系统代理启动配置 DNS 默认应使用本地解析，实际：%v", dns["final"])
	}
	if hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "remote") {
		t.Fatalf("系统代理启动配置不应初始化远程 DNS：%v", dns["servers"])
	}
	if hasConfigItem(dns["servers"].([]map[string]interface{}), "tag", "fakeip") {
		t.Fatalf("仅系统代理不应启用 fakeip：%v", dns["servers"])
	}
}

func TestBuildSingBoxConfigAppProxyRules(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.AppProxy = true
	state.Settings.AppProxyTargets = []AppProxyTarget{
		{Type: "app", Path: `C:\Tools\App\App.exe`, Enabled: true},
		{Type: "folder", Path: `C:\Games\Game`, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成应用代理配置失败：%v", err)
	}
	if state.Settings.Tun {
		t.Fatal("测试输入未显式开启普通 TUN")
	}
	if !hasTunInbound(config) {
		t.Fatal("应用接管应自动开启 TUN 入站")
	}
	route := config["route"].(map[string]interface{})
	var found map[string]interface{}
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "proxy" && rule["process_path"] != nil {
			found = rule
			break
		}
	}
	if found == nil {
		t.Fatalf("应生成进程路径代理规则：%v", route["rules"])
	}
	paths := found["process_path"].([]string)
	if len(paths) != 1 || !stringSliceContainsFold(paths, `C:\Tools\App\App.exe`) {
		t.Fatalf("进程路径规则只能使用已保存的应用路径：%v", paths)
	}
	patterns := found["process_path_regex"].([]string)
	if len(patterns) == 0 || !strings.Contains(strings.ToLower(patterns[0]), strings.ToLower(regexp.QuoteMeta(`C:\Games\Game\`))) {
		t.Fatalf("文件夹规则应生成目录正则：%v", patterns)
	}
	if !hasRouteOutbound(route["rules"].([]map[string]interface{}), "direct") {
		t.Fatal("仅开启应用接管时，非目标 TUN 流量应直连")
	}
}

func TestBuildSingBoxConfigProxyMap(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ProxyMap = true
	state.Settings.ProxyMapStartPort = 52001
	state.Nodes = append(state.Nodes, Node{
		ID:      "n2",
		Name:    "测试节点2",
		Type:    "socks",
		Address: "127.0.0.1",
		Port:    10002,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "127.0.0.1",
			"server_port": 10002,
		},
	})
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成代理映射配置失败：%v", err)
	}
	inbounds := config["inbounds"].([]map[string]interface{})
	outbounds := config["outbounds"].([]map[string]interface{})
	route := config["route"].(map[string]interface{})
	if !hasConfigItem(inbounds, "tag", "proxy-map-in-1") || !hasConfigItem(inbounds, "tag", "proxy-map-in-2") {
		t.Fatalf("应生成每个节点的映射入站：%v", inbounds)
	}
	if !hasConfigItem(outbounds, "tag", "proxy-map-out-1") || !hasConfigItem(outbounds, "tag", "proxy-map-out-2") {
		t.Fatalf("应生成每个节点的映射出站：%v", outbounds)
	}
	foundRule := false
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "proxy-map-out-1" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatalf("应生成映射路由规则：%v", route["rules"])
	}
}

func TestBuildSingBoxConfigProxyMapGroupsDoNotEnableMapping(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ProxyMap = false
	state.Settings.ProxyMapStartPort = 52001
	state.Settings.ProxyMapGroupIDs = []string{"default"}
	state.Settings.ProxyMapGroupID = "default"
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成未启用代理映射配置失败：%v", err)
	}
	inbounds := config["inbounds"].([]map[string]interface{})
	outbounds := config["outbounds"].([]map[string]interface{})
	if hasConfigItem(inbounds, "tag", "proxy-map-in-1") {
		t.Fatalf("仅选择映射分组不应生成映射入站：%v", inbounds)
	}
	if hasConfigItem(outbounds, "tag", "proxy-map-out-1") {
		t.Fatalf("仅选择映射分组不应生成映射出站：%v", outbounds)
	}
}
func TestBuildSingBoxConfigProxyMapGroup(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ProxyMap = true
	state.Settings.ProxyMapStartPort = 52001
	state.Settings.ProxyMapGroupIDs = []string{"g1", "g2"}
	state.Nodes[0].GroupID = defaultGroupID
	state.Groups = []NodeGroup{
		{ID: defaultGroupID, Name: "默认", Source: "manual"},
		{ID: "g1", Name: "分组1", Source: "manual"},
		{ID: "g2", Name: "分组2", Source: "manual"},
	}
	state.Nodes = append(state.Nodes,
		Node{
			ID:      "n2",
			Name:    "分组节点1",
			Type:    "socks",
			Address: "127.0.0.1",
			Port:    10002,
			GroupID: "g1",
			Outbound: map[string]interface{}{
				"type":        "socks",
				"server":      "127.0.0.1",
				"server_port": 10002,
			},
		},
		Node{
			ID:      "n3",
			Name:    "分组节点2",
			Type:    "socks",
			Address: "127.0.0.1",
			Port:    10003,
			GroupID: "g2",
			Outbound: map[string]interface{}{
				"type":        "socks",
				"server":      "127.0.0.1",
				"server_port": 10003,
			},
		},
	)
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成分组代理映射配置失败：%v", err)
	}
	inbounds := config["inbounds"].([]map[string]interface{})
	outbounds := config["outbounds"].([]map[string]interface{})
	route := config["route"].(map[string]interface{})
	if !hasConfigItem(inbounds, "tag", "proxy-map-in-1") ||
		!hasConfigItem(inbounds, "tag", "proxy-map-in-2") ||
		!hasConfigItem(inbounds, "tag", "proxy-map-in-3") {
		t.Fatalf("应为所有节点生成映射入站，分组只控制界面显示：%v", inbounds)
	}
	if inbound := findConfigItem(inbounds, "tag", "proxy-map-in-1"); inbound["listen_port"] != 52001 {
		t.Fatalf("第一个映射端口应从起始端口开始：%v", inbound)
	}
	if inbound := findConfigItem(inbounds, "tag", "proxy-map-in-2"); inbound["listen_port"] != 52002 {
		t.Fatalf("第二个映射端口应连续分配：%v", inbound)
	}
	if !hasConfigItem(outbounds, "tag", "proxy-map-out-1") ||
		!hasConfigItem(outbounds, "tag", "proxy-map-out-2") ||
		!hasConfigItem(outbounds, "tag", "proxy-map-out-3") {
		t.Fatalf("应为所有节点生成映射出站，分组只控制界面显示：%v", outbounds)
	}
	if outbound := findConfigItem(outbounds, "tag", "proxy-map-out-1"); outbound["server_port"] != 10001 {
		t.Fatalf("第一个映射出站应对应完整节点列表第一个节点：%v", outbound)
	}
	if outbound := findConfigItem(outbounds, "tag", "proxy-map-out-2"); outbound["server_port"] != 10002 {
		t.Fatalf("第二个映射出站应对应完整节点列表第二个节点：%v", outbound)
	}
	if !hasRouteOutbound(route["rules"].([]map[string]interface{}), "proxy-map-out-1") ||
		!hasRouteOutbound(route["rules"].([]map[string]interface{}), "proxy-map-out-2") ||
		!hasRouteOutbound(route["rules"].([]map[string]interface{}), "proxy-map-out-3") {
		t.Fatalf("应生成所有节点的映射路由规则：%v", route["rules"])
	}
}

func TestConfigModeFromMarshaledConfig(t *testing.T) {
	state := testConfigState("global")
	state.Settings.Tun = true
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成全局配置失败：%v", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("序列化配置失败：%v", err)
	}
	var runtimeConfig map[string]interface{}
	if err := json.Unmarshal(data, &runtimeConfig); err != nil {
		t.Fatalf("反序列化配置失败：%v", err)
	}
	summary := configModeFromConfig(runtimeConfig, "")
	if summary.Mode != "global" || len(summary.RuleSets) > 0 {
		t.Fatalf("运行配置应识别为真实全局模式：%+v", summary)
	}
	if summary.HasDirect {
		t.Fatalf("TUN 稳定性直连规则不应被识别为全局模式规则残留：%+v", summary)
	}
	if !hasTunInbound(runtimeConfig) {
		t.Fatal("运行配置应识别出 TUN 入站")
	}
	if !hasDirectRouteForProcessPathPattern(runtimeConfig, `(?i)\\bin\\sing-box\.exe$`) {
		t.Fatalf("TUN 模式应让测试工具进程直连：%+v", runtimeConfig["route"])
	}
	if !hasDirectRouteForProcessName(runtimeConfig, "sing-box.exe") {
		t.Fatalf("TUN 模式应按进程名让测试内核直连，避免测速被 TUN 回环：%+v", runtimeConfig["route"])
	}
	if hasGlobalModeBlockRule(runtimeConfig) {
		t.Fatalf("全局 TUN 模式不应包含普通网站拦截规则：%+v", runtimeConfig["route"])
	}
}

func TestValidateRuntimeSettingsAcceptsInactiveRuleRulesInGlobal(t *testing.T) {
	state := testConfigState("global")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成全局配置失败：%v", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("序列化全局配置失败：%v", err)
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "sing-box.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("写入运行配置失败：%v", err)
	}
	manager := NewSingBoxManager(root, filepath.Join(root, "sing-box.exe"), configPath, filepath.Join(root, "cache.db"), nil)
	if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
		t.Fatalf("全局模式不应把未激活的规则模式配置识别为残留：%v", err)
	}
}

func TestValidateRuntimeSettingsAcceptsAppProxyFallbackInGlobal(t *testing.T) {
	state := testConfigState("global")
	state.Settings.AppProxy = true
	state.Settings.AppProxyTargets = []AppProxyTarget{
		{Type: "folder", Path: `C:\Tools\BitBrowser`, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成应用接管全局配置失败：%v", err)
	}
	if summary := configModeFromConfigWithExternalEndpoints(config, "global", nil); !summary.HasDirect {
		t.Fatal("未明确启用应用接管时，不得豁免非目标应用直连规则")
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "sing-box.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	manager := NewSingBoxManager(root, filepath.Join(root, "sing-box.exe"), configPath, filepath.Join(root, "cache.db"), nil)
	if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
		t.Fatalf("应用接管的精确 TUN 兜底直连规则不应被误判为残留：%v", err)
	}
}

func TestValidateRuntimeSettingsRejectsStaleAppProxyRoute(t *testing.T) {
	state := testConfigState("global")
	state.Settings.AppProxy = true
	state.Settings.AppProxyTargets = []AppProxyTarget{
		{Type: "app", Path: `C:\Tools\Current\App.exe`, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成应用接管配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if rule["outbound"] == "proxy" && rule["process_path"] != nil {
			rule["process_path"] = []string{`C:\Tools\Old\App.exe`}
			break
		}
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "sing-box.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	manager := NewSingBoxManager(root, filepath.Join(root, "sing-box.exe"), configPath, filepath.Join(root, "cache.db"), nil)
	if err := manager.ValidateRuntimeSettings(state.Settings); err == nil {
		t.Fatal("旧应用路径残留不应被误判为当前应用接管规则已生效")
	}
}

func TestCountryCacheHasCapacityLimit(t *testing.T) {
	countryCache.Lock()
	clear(countryCache.values)
	countryCache.Unlock()
	t.Cleanup(func() {
		countryCache.Lock()
		clear(countryCache.values)
		countryCache.Unlock()
	})

	for index := 0; index < maxCountryCacheEntries+100; index++ {
		storeCountryCache(fmt.Sprintf("203.0.%d.%d", index/256, index%256), "测试")
	}
	countryCache.RLock()
	size := len(countryCache.values)
	countryCache.RUnlock()
	if size > maxCountryCacheEntries {
		t.Fatalf("国家缓存超过上限：%d > %d", size, maxCountryCacheEntries)
	}
}

func TestConfirmModeSelectionWaitsForAppliedMode(t *testing.T) {
	calls := 0
	err := confirmModeSelection("global", func() (string, error) {
		calls++
		if calls < 3 {
			return "rule", nil
		}
		return "GLOBAL", nil
	}, 100*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("模式回读最终匹配时应成功：%v", err)
	}
	if calls < 3 {
		t.Fatalf("模式确认未进行必要重试：%d", calls)
	}
}

func TestConfirmModeSelectionRejectsMismatch(t *testing.T) {
	err := confirmModeSelection("global", func() (string, error) {
		return "rule", nil
	}, 5*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("模式始终不匹配时应返回错误")
	}
}

func TestGlobalModeAllowsOnlyTunStabilityBlockRules(t *testing.T) {
	state := testConfigState("global")
	state.Settings.Tun = true
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成全局配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	hasTunBlock := false
	for _, rule := range rules {
		if rule["outbound"] != "block" {
			continue
		}
		if isTunStabilityBlockRule(rule) {
			hasTunBlock = true
			continue
		}
		if !isLocalMetadataBlockRule(rule) {
			t.Fatalf("全局模式只能保留 TUN 稳定性拦截规则，实际：%v", rule)
		}
	}
	if !hasTunBlock {
		t.Fatal("TUN 模式应保留必要的多播和广播拦截规则")
	}
}

func TestBuildSingBoxConfigBlocksLocalMetadataAddress(t *testing.T) {
	state := testConfigState("global")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成全局配置失败：%v", err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	metadataIndex := -1
	globalIndex := -1
	for index, rule := range rules {
		if isLocalMetadataBlockRule(rule) {
			metadataIndex = index
		}
		if rule["clash_mode"] == "global" && rule["outbound"] == "proxy" {
			globalIndex = index
		}
	}
	if metadataIndex < 0 {
		t.Fatalf("应拦截本地元数据地址，避免 169.254.169.254 走代理节点：%v", rules)
	}
	if globalIndex < 0 || metadataIndex > globalIndex {
		t.Fatalf("本地元数据地址拦截必须排在全局代理前：metadata=%d global=%d rules=%v", metadataIndex, globalIndex, rules)
	}
	if hasGlobalModeBlockRule(config) {
		t.Fatalf("本地元数据地址拦截不应被识别为普通全局拦截残留：%v", route)
	}
}

func TestConfigModeFromGeneratedRuleConfig(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成规则配置失败：%v", err)
	}
	summary := configModeFromConfig(config, "")
	if summary.Mode != "rule" || !summary.HasDirect || len(summary.RuleSets) == 0 {
		t.Fatalf("运行配置应识别为规则模式：%+v", summary)
	}
}

func TestBuildSingBoxConfigRejectsClashAPIPortConflict(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.MixedPort = clashAPIPort
	if _, err := BuildSingBoxConfig(state, state.Nodes[0], ""); err == nil {
		t.Fatalf("本地代理端口与内核控制端口冲突时应拒绝生成配置")
	}
}

func TestDelayURLPrefersV2rayNAddressWithFallbacks(t *testing.T) {
	if len(delayTestURLs) < 2 {
		t.Fatalf("URL 延迟测试应提供失败回退地址：%v", delayTestURLs)
	}
	if delayTestURLs[0] != "https://www.google.com/generate_204" {
		t.Fatalf("URL 延迟测试应优先使用 v2rayN 同款地址：%v", delayTestURLs)
	}
}

func TestTraceCountryPrefersLocationCode(t *testing.T) {
	if country := traceCountryName("LAX", "CN"); country != "中国" {
		t.Fatalf("Cloudflare Trace 应优先使用 loc 国家码，实际：%s", country)
	}
}

func TestGeoDecodersNormalizeCountryCode(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		decode func(reader io.Reader) (geoResponse, error)
	}{
		{name: "ipwho.is", raw: `{"success":true,"ip":"103.45.129.219","country":"China","country_code":"CN"}`, decode: decodeIPWhoisGeo},
		{name: "ipapi.co", raw: `{"ip":"103.45.129.219","country_name":"China","country_code":"CN"}`, decode: decodeIPAPICoGeo},
		{name: "ip-api", raw: `{"status":"success","query":"103.45.129.219","country":"China","countryCode":"CN"}`, decode: decodeIPAPIGeo},
	}
	for _, item := range cases {
		data, err := item.decode(strings.NewReader(item.raw))
		if err != nil {
			t.Fatalf("%s 解析失败：%v", item.name, err)
		}
		if country := geoCountryName(data.CountryCode, data.Country); country != "中国" {
			t.Fatalf("%s 国家识别错误：%s", item.name, country)
		}
	}
}

func TestExportCheckConfig(t *testing.T) {
	output := os.Getenv("GO_SINGBOX_EXPORT_CHECK_CONFIG")
	if output == "" {
		t.Skip("仅用于手工导出 sing-box check 配置")
	}
	state := testConfigState("global")
	state.Settings.Tun = true
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "data/gui/cache.db")
	if err != nil {
		t.Fatalf("生成检查配置失败：%v", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("序列化检查配置失败：%v", err)
	}
	if err := os.WriteFile(output, data, 0644); err != nil {
		t.Fatalf("写出检查配置失败：%v", err)
	}
}

func TestNeedsRestartForRunningSettings(t *testing.T) {
	current := testConfigState("rule").Settings
	next := current
	next.Mode = "global"
	if needsRestartForRunningSettings(current, next) {
		t.Fatal("运行中切换规则/全局应通过 Clash API 生效，不应重启代理")
	}
	next = current
	next.Tun = !current.Tun
	if !needsRestartForRunningSettings(current, next) {
		t.Fatal("运行中切换 TUN 应重启代理")
	}
	next = current
	next.SystemProxy = !current.SystemProxy
	if needsRestartForRunningSettings(current, next) {
		t.Fatal("仅切换系统代理不应重启代理")
	}
}

func TestSystemProxyModeSwitchUsesClashAPIWithoutRestart(t *testing.T) {
	current := testConfigState("rule").Settings
	current.SystemProxy = true
	current.Tun = false
	next := current
	next.Mode = "global"
	if !canApplySettingsWhileRunning(current, next) {
		t.Fatal("系统代理运行中应允许规则/全局模式切换")
	}
	if needsRestartForRunningSettings(current, next) {
		t.Fatal("系统代理规则切全局应通过 Clash API 生效，不应重启或触发规则残留校验")
	}

	current = next
	next.Mode = "rule"
	if !canApplySettingsWhileRunning(current, next) {
		t.Fatal("系统代理运行中应允许全局/规则模式切换")
	}
	if needsRestartForRunningSettings(current, next) {
		t.Fatal("系统代理全局切规则应通过 Clash API 生效，不应重启")
	}
}

func TestWaitPortReturnsWhenProcessExits(t *testing.T) {
	port, err := freeTestPort()
	if err != nil {
		t.Fatalf("获取测试端口失败：%v", err)
	}
	done := make(chan error, 1)
	done <- errors.New("进程已退出")
	close(done)

	start := time.Now()
	err = waitPortOrProcessExit("127.0.0.1", port, 5*time.Second, done)
	if err == nil || !strings.Contains(err.Error(), "进程已退出") {
		t.Fatalf("应返回进程退出错误，实际：%v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("进程退出后应快速返回，实际耗时：%v", elapsed)
	}
}

func TestBuildSingBoxConfigSelectorContainsAllNodes(t *testing.T) {
	state := testConfigState("rule")
	state.Nodes = append(state.Nodes, Node{
		ID:      "n2",
		Name:    "测试节点2",
		Type:    "socks",
		Address: "127.0.0.1",
		Port:    10002,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "127.0.0.1",
			"server_port": 10002,
		},
	})
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成 selector 配置失败：%v", err)
	}
	outbounds := config["outbounds"].([]map[string]interface{})
	selector := outbounds[0]
	if selector["type"] != "selector" || selector["tag"] != "proxy" {
		t.Fatalf("第一个出站应为 proxy selector：%v", selector)
	}
	all, ok := selector["outbounds"].([]string)
	if !ok {
		t.Fatalf("selector outbounds 类型错误：%T", selector["outbounds"])
	}
	if !stringSliceContains(all, "n1") || !stringSliceContains(all, "n2") {
		t.Fatalf("selector 应包含全部节点：%v", all)
	}
	if selector["default"] != "n1" {
		t.Fatalf("selector 默认节点应为 n1：%v", selector["default"])
	}
	if selector["interrupt_exist_connections"] != true {
		t.Fatalf("selector 切换应中断旧连接以立即生效：%v", selector)
	}
}

func TestParseSupportedProtocolLinksForConfig(t *testing.T) {
	nodes := testSupportedProtocolNodes(t)
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("各种协议节点应能生成 sing-box 配置：%v", err)
	}
	outbounds := config["outbounds"].([]map[string]interface{})
	selector := outbounds[0]
	if len(selector["outbounds"].([]string)) != len(nodes) {
		t.Fatalf("selector 应包含所有协议节点：%v", selector)
	}
}

func TestParseVLESSRealityUsesUTLSFingerprint(t *testing.T) {
	raw := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=reality&sni=stock.adobe.com&fp=firefox&pbk=test-public-key&sid=5fe902b31d0a9179&spx=%2Fpath&pqv=test-mldsa65&type=tcp&headerType=none&flow=xtls-rprx-vision&encryption=none#VLESS-Reality"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("VLESS Reality 解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("VLESS Reality 应解析出 1 个节点：%d", len(nodes))
	}
	tls, ok := nodes[0].Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("VLESS Reality 应生成 TLS 配置：%v", nodes[0].Outbound)
	}
	reality, ok := tls["reality"].(map[string]interface{})
	if !ok || reality["public_key"] != "test-public-key" || reality["short_id"] != "5fe902b31d0a9179" {
		t.Fatalf("Reality 配置错误：%v", tls["reality"])
	}
	if _, ok := reality["spider_x"]; ok {
		t.Fatalf("当前 sing-box 不支持 spider_x，不能写入运行配置：%v", reality)
	}
	if _, ok := reality["mldsa65_verify"]; ok {
		t.Fatalf("当前 sing-box 不支持 mldsa65_verify，不能写入运行配置：%v", reality)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["enabled"] != true || utls["fingerprint"] != "firefox" {
		t.Fatalf("Reality 必须带 uTLS 指纹：%v", tls["utls"])
	}
	delayConfig := buildDelayConfig(nodes[0], 20808)
	outbounds := delayConfig["outbounds"].([]map[string]interface{})
	delayTLS := outbounds[0]["tls"].(map[string]interface{})
	delayUTLS := delayTLS["utls"].(map[string]interface{})
	if delayUTLS["fingerprint"] != "firefox" {
		t.Fatalf("延迟/测速配置应保留 uTLS 指纹：%v", delayTLS)
	}
}

func TestParseVMessEditFieldsAffectOutbound(t *testing.T) {
	data := map[string]interface{}{
		"v":                             "2",
		"ps":                            "VMess-Edit",
		"add":                           "example.com",
		"port":                          "443",
		"id":                            "11111111-1111-1111-1111-111111111111",
		"aid":                           "0",
		"scy":                           "auto",
		"net":                           "ws",
		"tls":                           "tls",
		"sni":                           "vmess.example",
		"alpn":                          "h2",
		"fp":                            "chrome",
		"mux":                           true,
		"certificate":                   "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
		"certificate_public_key_sha256": "sha256-pin",
		"ech_config":                    "ech-a\nech-b",
	}
	rawJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("编码 VMess 测试链接失败：%v", err)
	}
	raw := "vmess://" + base64.StdEncoding.EncodeToString(rawJSON)
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("VMess 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("VMess 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["security"] != "auto" {
		t.Fatalf("VMess security 未写入 outbound：%v", outbound["security"])
	}
	mux, ok := outbound["multiplex"].(map[string]interface{})
	if !ok || mux["enabled"] != true {
		t.Fatalf("Mux 未写入 outbound：%v", outbound["multiplex"])
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || transport["type"] != "ws" {
		t.Fatalf("WS 传输未写入 outbound：%v", outbound["transport"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("VMess TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "vmess.example" {
		t.Fatalf("SNI 未写入 TLS：%v", tls)
	}
	if _, ok := tls["insecure"]; ok {
		t.Fatalf("固定证书存在时不应同时写入跳过证书验证：%v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("Fingerprint 未写入 uTLS：%v", tls["utls"])
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("ALPN 未写入 TLS：%v", tls["alpn"])
	}
	certs, ok := tls["certificate"].([]string)
	if !ok || len(certs) != 3 {
		t.Fatalf("固定证书未写入 TLS：%v", tls["certificate"])
	}
	pins, ok := tls["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	config, configOK := ech["config"].([]string)
	if !ok || !configOK || len(config) != 2 {
		t.Fatalf("ECHConfigList 未写入 TLS：%v", tls["ech"])
	}
}

func TestVMessEditFieldsPassSingBoxCheck(t *testing.T) {
	data := map[string]interface{}{
		"v":             "2",
		"ps":            "VMess-Edit",
		"add":           "example.com",
		"port":          "443",
		"id":            "11111111-1111-1111-1111-111111111111",
		"aid":           "0",
		"scy":           "auto",
		"net":           "ws",
		"tls":           "tls",
		"sni":           "vmess.example",
		"alpn":          "h2",
		"fp":            "chrome",
		"allowInsecure": true,
		"mux":           true,
	}
	rawJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("编码 VMess 测试链接失败：%v", err)
	}
	raw := "vmess://" + base64.StdEncoding.EncodeToString(rawJSON)
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("VMess 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 VMess 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "vmess-edit.json")
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, configJSON, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseVMessAllowInsecureWithoutPinnedCertificate(t *testing.T) {
	data := map[string]interface{}{
		"v":             "2",
		"ps":            "VMess-Insecure",
		"add":           "example.com",
		"port":          "443",
		"id":            "11111111-1111-1111-1111-111111111111",
		"aid":           "0",
		"scy":           "auto",
		"net":           "tcp",
		"tls":           "tls",
		"sni":           "vmess.example",
		"allowInsecure": true,
	}
	rawJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("编码 VMess 测试链接失败：%v", err)
	}
	nodes, errs := ParseNodes("vmess://" + base64.StdEncoding.EncodeToString(rawJSON))
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("VMess 跳过证书验证解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	tls, ok := nodes[0].Outbound["tls"].(map[string]interface{})
	if !ok || tls["insecure"] != true {
		t.Fatalf("未设置固定证书时，跳过证书验证应写入 TLS：%v", tls)
	}
}

func TestParseVLESSEditFieldsAffectOutbound(t *testing.T) {
	raw := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&sni=vless.example&fp=chrome&alpn=h2&allowInsecure=1&type=tcp&rawType=http&finalMask=%2Fmask&flow=xtls-rprx-vision&encryption=none&mux=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#VLESS-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("VLESS 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("VLESS 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["flow"] != "xtls-rprx-vision" {
		t.Fatalf("flow 未写入 outbound：%v", outbound["flow"])
	}
	mux, ok := outbound["multiplex"].(map[string]interface{})
	if !ok || mux["enabled"] != true {
		t.Fatalf("Mux 未写入 outbound：%v", outbound["multiplex"])
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || transport["type"] != "raw" {
		t.Fatalf("raw 传输未写入 outbound：%v", outbound["transport"])
	}
	headers, ok := transport["headers"].(map[string]interface{})
	if !ok || headers["type"] != "http" {
		t.Fatalf("raw http 伪装未写入 transport：%v", transport["headers"])
	}
	request, ok := headers["request"].(map[string]interface{})
	paths, pathOK := request["path"].([]string)
	if !ok || !pathOK || len(paths) != 1 || paths[0] != "/mask" {
		t.Fatalf("Finalmask 未写入 raw http request.path：%v", headers["request"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("VLESS TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "vless.example" || tls["insecure"] != true {
		t.Fatalf("SNI 或跳过证书验证未写入 TLS：%v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("Fingerprint 未写入 uTLS：%v", tls["utls"])
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("ALPN 未写入 TLS：%v", tls["alpn"])
	}
	certs, ok := tls["certificate"].([]string)
	if !ok || len(certs) != 3 {
		t.Fatalf("固定证书未写入 TLS：%v", tls["certificate"])
	}
	pins, ok := tls["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	config, configOK := ech["config"].([]string)
	if !ok || !configOK || len(config) != 2 {
		t.Fatalf("ECHConfigList 未写入 TLS：%v", tls["ech"])
	}
}

func TestParseTrojanEditFieldsAffectOutbound(t *testing.T) {
	raw := "trojan://pass@example.com:443?security=tls&sni=trojan.example&fp=chrome&alpn=h2&allowInsecure=1&mux=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#Trojan-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("Trojan 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("Trojan 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	mux, ok := outbound["multiplex"].(map[string]interface{})
	if !ok || mux["enabled"] != true {
		t.Fatalf("Mux 未写入 outbound：%v", outbound["multiplex"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("Trojan TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "trojan.example" || tls["insecure"] != true {
		t.Fatalf("SNI 或跳过证书验证未写入 TLS：%v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("Fingerprint 未写入 uTLS：%v", tls["utls"])
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("ALPN 未写入 TLS：%v", tls["alpn"])
	}
	certs, ok := tls["certificate"].([]string)
	if !ok || len(certs) != 3 {
		t.Fatalf("固定证书未写入 TLS：%v", tls["certificate"])
	}
	pins, ok := tls["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	config, configOK := ech["config"].([]string)
	if !ok || !configOK || len(config) != 2 {
		t.Fatalf("ECHConfigList 未写入 TLS：%v", tls["ech"])
	}
}

func TestTrojanEditFieldsPassSingBoxCheck(t *testing.T) {
	raw := "trojan://pass@example.com:443?security=tls&sni=trojan.example&fp=chrome&alpn=h2&allowInsecure=1&mux=1#Trojan-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Trojan 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 Trojan 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "trojan-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseHysteria2EditFieldsAffectOutbound(t *testing.T) {
	raw := "hysteria2://pass@example.com:443?sni=hysteria.example&fp=chrome&alpn=h3&obfs-password=obfs-pass&mport=1000-2000,3000,4000&hop=30s&upmbps=100&downmbps=200&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#Hysteria2-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("Hysteria2 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("Hysteria2 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	obfs, ok := outbound["obfs"].(map[string]interface{})
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "obfs-pass" {
		t.Fatalf("混淆密码未写入 obfs：%v", outbound["obfs"])
	}
	ports, ok := outbound["server_ports"].([]string)
	if !ok || len(ports) != 3 || ports[0] != "1000-2000" || ports[1] != "3000" || ports[2] != "4000" {
		t.Fatalf("跳跃端口范围未写入 server_ports：%v", outbound["server_ports"])
	}
	if outbound["hop_interval"] != "30s" || outbound["up_mbps"] != 100 || outbound["down_mbps"] != 200 {
		t.Fatalf("端口跳跃间隔或带宽未写入 outbound：%v", outbound)
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("Hysteria2 TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "hysteria.example" {
		t.Fatalf("SNI 未写入 TLS：%v", tls)
	}
	if _, ok := tls["insecure"]; ok {
		t.Fatalf("固定证书存在时不应同时写入跳过证书验证：%v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("Fingerprint 未写入 uTLS：%v", tls["utls"])
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h3" {
		t.Fatalf("ALPN 未写入 TLS：%v", tls["alpn"])
	}
	certs, ok := tls["certificate"].([]string)
	if !ok || len(certs) != 3 {
		t.Fatalf("固定证书未写入 TLS：%v", tls["certificate"])
	}
	pins, ok := tls["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	config, configOK := ech["config"].([]string)
	if !ok || !configOK || len(config) != 2 {
		t.Fatalf("ECHConfigList 未写入 TLS：%v", tls["ech"])
	}
}

func TestHysteria2EditFieldsPassSingBoxCheck(t *testing.T) {
	raw := "hysteria2://pass@example.com:443?sni=hysteria.example&fp=chrome&alpn=h3&obfs-password=obfs-pass&hop=30s&upmbps=100&downmbps=200&insecure=1#Hysteria2-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Hysteria2 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 Hysteria2 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "hysteria2-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseHysteria2AllowInsecureWithoutPinnedCertificate(t *testing.T) {
	nodes, errs := ParseNodes("hysteria2://pass@example.com:443?sni=hysteria.example&insecure=1#Hysteria2-Insecure")
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Hysteria2 跳过证书验证解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	tls, ok := nodes[0].Outbound["tls"].(map[string]interface{})
	if !ok || tls["insecure"] != true {
		t.Fatalf("未设置固定证书时，跳过证书验证应写入 TLS：%v", tls)
	}
}

func TestParseWireGuardEditFieldsAffectOutbound(t *testing.T) {
	raw := "wireguard://privkey@example.com:51820?public_key=peer-public&pre_shared_key=psk&reserved=1,2,3&address=10.0.0.2%2F32,fd00::2%2F128&mtu=1420#WireGuard-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("WireGuard 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("WireGuard 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["private_key"] != "privkey" || outbound["peer_public_key"] != "peer-public" {
		t.Fatalf("PrivateKey 或 PublicKey 未写入 outbound：%v", outbound)
	}
	if outbound["pre_shared_key"] != "psk" {
		t.Fatalf("PreSharedKey 未写入 outbound：%v", outbound["pre_shared_key"])
	}
	reserved, ok := outbound["reserved"].([]int)
	if !ok || len(reserved) != 3 || reserved[0] != 1 || reserved[1] != 2 || reserved[2] != 3 {
		t.Fatalf("Reserved 未写入 outbound：%v", outbound["reserved"])
	}
	localAddress, ok := outbound["local_address"].([]string)
	if !ok || len(localAddress) != 2 || localAddress[0] != "10.0.0.2/32" || localAddress[1] != "fd00::2/128" {
		t.Fatalf("Address 未写入 outbound：%v", outbound["local_address"])
	}
	if outbound["mtu"] != 1420 {
		t.Fatalf("MTU 未写入 outbound：%v", outbound["mtu"])
	}
	if _, ok := outbound["network"]; ok {
		t.Fatalf("WireGuard 编辑弹窗已移除 Network，不应写入 network：%v", outbound["network"])
	}
}

func TestWireGuardEditFieldsPassSingBoxCheck(t *testing.T) {
	raw := "wireguard://6MwTUKYBqtwMTgV77OHQbeq0BNtzSTe7VMllSB29ZVU%3D@162.159.192.1:2408?public_key=Gp3k%2BUPoWgGc3ESb5vs0%2B%2F1%2FUiGC5GazvJEgFINpkEE%3D&reserved=0,0,0&address=172.16.0.2%2F32&mtu=1280#WireGuard-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("WireGuard 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 WireGuard 编辑字段配置失败：%v", err)
	}
	endpoints, ok := config["endpoints"].([]map[string]interface{})
	if !ok || len(endpoints) != 1 {
		t.Fatalf("WireGuard 应生成 endpoint 配置：%v", config["endpoints"])
	}
	endpoint := endpoints[0]
	if endpoint["type"] != "wireguard" || endpoint["private_key"] == "" {
		t.Fatalf("WireGuard endpoint 基础字段错误：%v", endpoint)
	}
	addresses, ok := endpoint["address"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "172.16.0.2/32" {
		t.Fatalf("WireGuard Address 未写入 endpoint：%v", endpoint["address"])
	}
	peers, ok := endpoint["peers"].([]map[string]interface{})
	if !ok || len(peers) != 1 || peers[0]["public_key"] == "" || peers[0]["address"] != "162.159.192.1" {
		t.Fatalf("WireGuard peer 未写入 endpoint：%v", endpoint["peers"])
	}
	path := filepath.Join(t.TempDir(), "wireguard-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseSOCKSEditFieldsAffectOutbound(t *testing.T) {
	nodes, errs := ParseNodes("socks5://user:pass@example.com:1080#SOCKS-Edit")
	if len(errs) > 0 {
		t.Fatalf("SOCKS 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("SOCKS 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["type"] != "socks" || outbound["server"] != "example.com" || outbound["server_port"] != 1080 {
		t.Fatalf("SOCKS 地址端口未写入 outbound：%v", outbound)
	}
	if outbound["username"] != "user" || outbound["password"] != "pass" {
		t.Fatalf("SOCKS 用户名或密码未写入 outbound：%v", outbound)
	}
	if _, ok := outbound["transport"]; ok {
		t.Fatalf("当前 sing-box SOCKS 不支持 transport，不应写入：%v", outbound["transport"])
	}
	if _, ok := outbound["tls"]; ok {
		t.Fatalf("当前 sing-box SOCKS 不支持 TLS，不应写入：%v", outbound["tls"])
	}
}

func TestSOCKSEditFieldsPassSingBoxCheck(t *testing.T) {
	nodes, errs := ParseNodes("socks5://user:pass@example.com:1080#SOCKS-Edit")
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("SOCKS 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 SOCKS 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "socks-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseShadowsocksEditFieldsAffectOutbound(t *testing.T) {
	raw := "ss://YWVzLTEyOC1nY206cGFzcw@example.com:8388?udp_over_tcp=1&mux=1#SS-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("Shadowsocks 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("Shadowsocks 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	udpOverTCP, ok := outbound["udp_over_tcp"].(map[string]interface{})
	if !ok || udpOverTCP["enabled"] != true {
		t.Fatalf("UDP over TCP 未写入 outbound：%v", outbound["udp_over_tcp"])
	}
	mux, ok := outbound["multiplex"].(map[string]interface{})
	if !ok || mux["enabled"] != true {
		t.Fatalf("Mux 未写入 outbound：%v", outbound["multiplex"])
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 Shadowsocks 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "ss-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseNaiveEditFieldsAffectOutbound(t *testing.T) {
	raw := "naive://user:pass@example.com:443?sni=naive.example&quic=1&quic_congestion_control=bbr2&insecure_concurrency=4&udp_over_tcp=1&fp=chrome&alpn=h3&allowInsecure=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#Naive-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("Naive 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("Naive 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["username"] != "user" || outbound["password"] != "pass" {
		t.Fatalf("Naive 用户名或密码未写入 outbound：%v", outbound)
	}
	if outbound["quic"] != true || outbound["quic_congestion_control"] != "bbr2" {
		t.Fatalf("Naive QUIC 或拥塞控制算法未写入 outbound：%v", outbound)
	}
	if outbound["insecure_concurrency"] != 4 {
		t.Fatalf("Naive 不安全并发未写入 outbound：%v", outbound)
	}
	udpOverTCP, ok := outbound["udp_over_tcp"].(map[string]interface{})
	if !ok || udpOverTCP["enabled"] != true {
		t.Fatalf("Naive UDP over TCP 未写入 outbound：%v", outbound["udp_over_tcp"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("Naive TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "naive.example" {
		t.Fatalf("Naive SNI 未写入 TLS：%v", tls)
	}
	if _, ok := tls["insecure"]; ok {
		t.Fatalf("当前 sing-box Naive 不支持跳过证书验证，不应写入：%v", tls["insecure"])
	}
	if _, ok := tls["utls"]; ok {
		t.Fatalf("当前 sing-box Naive 不支持 Fingerprint，不应写入：%v", tls["utls"])
	}
	if _, ok := tls["alpn"]; ok {
		t.Fatalf("当前 sing-box Naive 不支持 ALPN，不应写入：%v", tls["alpn"])
	}
	if certs, ok := tls["certificate"].([]string); !ok || len(certs) == 0 {
		t.Fatalf("Naive 固定证书未写入 TLS：%v", tls["certificate"])
	}
	if pins, ok := tls["certificate_public_key_sha256"].([]string); !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("Naive 证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	if !ok || ech["enabled"] != true {
		t.Fatalf("Naive ECH 未写入 TLS：%v", tls["ech"])
	}
	echConfigs, ok := ech["config"].([]string)
	if !ok || len(echConfigs) != 2 || echConfigs[0] != "ech-a" || echConfigs[1] != "ech-b" {
		t.Fatalf("Naive ECHConfigList 未写入 TLS：%v", ech["config"])
	}
}

func TestNaiveEditFieldsPassSingBoxDecode(t *testing.T) {
	raw := "naive://user:pass@example.com:443?sni=naive.example&quic=1&quic_congestion_control=bbr2&insecure_concurrency=4&udp_over_tcp=1&certificate_public_key_sha256=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%3D#Naive-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Naive 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 Naive 编辑字段配置失败：%v", err)
	}
	outbounds, ok := config["outbounds"].([]map[string]interface{})
	if !ok || len(outbounds) == 0 {
		t.Fatalf("Naive 配置缺少 outbounds：%v", config["outbounds"])
	}
	var naive map[string]interface{}
	for _, outbound := range outbounds {
		if outbound["type"] == "naive" {
			naive = outbound
			break
		}
	}
	if naive == nil {
		t.Fatalf("生成配置中缺少 Naive outbound：%v", outbounds)
	}
	tls, ok := naive["tls"].(map[string]interface{})
	if !ok || tls["certificate_public_key_sha256"] == nil {
		t.Fatalf("Naive 可检查配置未写入证书指纹：%v", naive["tls"])
	}
}

func TestParseTUICEditFieldsAffectOutbound(t *testing.T) {
	raw := "tuic://11111111-1111-1111-1111-111111111111:pass@example.com:443?sni=tuic.example&congestion_control=bbr&fp=chrome&alpn=h3&allowInsecure=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#TUIC-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("TUIC 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("TUIC 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["uuid"] != "11111111-1111-1111-1111-111111111111" || outbound["password"] != "pass" {
		t.Fatalf("TUIC 用户 ID 或密码未写入 outbound：%v", outbound)
	}
	if outbound["congestion_control"] != "bbr" {
		t.Fatalf("TUIC 拥塞控制算法未写入 outbound：%v", outbound)
	}
	if _, ok := outbound["udp_relay_mode"]; ok {
		t.Fatalf("TUIC 图片中没有 UDP Relay Mode，不应写入：%v", outbound["udp_relay_mode"])
	}
	if _, ok := outbound["zero_rtt_handshake"]; ok {
		t.Fatalf("TUIC 图片中没有 0-RTT，不应写入：%v", outbound["zero_rtt_handshake"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("TUIC TLS 未写入 outbound：%v", outbound)
	}
	if tls["server_name"] != "tuic.example" || tls["insecure"] != true {
		t.Fatalf("TUIC SNI 或跳过证书验证未写入 TLS：%v", tls)
	}
	if alpn, ok := tls["alpn"].([]string); !ok || len(alpn) != 1 || alpn[0] != "h3" {
		t.Fatalf("TUIC ALPN 未写入 TLS：%v", tls["alpn"])
	}
	if utls, ok := tls["utls"].(map[string]interface{}); !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("TUIC Fingerprint 未写入 TLS：%v", tls["utls"])
	}
	if certs, ok := tls["certificate"].([]string); !ok || len(certs) == 0 {
		t.Fatalf("TUIC 固定证书未写入 TLS：%v", tls["certificate"])
	}
	if pins, ok := tls["certificate_public_key_sha256"].([]string); !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("TUIC 证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	if !ok || ech["enabled"] != true {
		t.Fatalf("TUIC ECH 未写入 TLS：%v", tls["ech"])
	}
	echConfigs, ok := ech["config"].([]string)
	if !ok || len(echConfigs) != 2 || echConfigs[0] != "ech-a" || echConfigs[1] != "ech-b" {
		t.Fatalf("TUIC ECHConfigList 未写入 TLS：%v", ech["config"])
	}
}

func TestTUICEditFieldsPassSingBoxCheck(t *testing.T) {
	raw := "tuic://11111111-1111-1111-1111-111111111111:pass@example.com:443?sni=tuic.example&congestion_control=bbr&fp=chrome&alpn=h3&certificate_public_key_sha256=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%3D#TUIC-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("TUIC 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 TUIC 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "tuic-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码 TUIC 编辑字段配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入 TUIC 编辑字段配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseAnyTLSEditFieldsAffectOutbound(t *testing.T) {
	raw := "anytls://pass@example.com:443?security=reality&sni=example.com&fp=chrome&pbk=test-public-key&sid=abcd&alpn=h3,h2&allowInsecure=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#AnyTLS-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("AnyTLS 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("AnyTLS 应解析出 1 个节点：%d", len(nodes))
	}
	tls, ok := nodes[0].Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("AnyTLS 应生成 TLS 配置：%v", nodes[0].Outbound)
	}
	if tls["server_name"] != "example.com" {
		t.Fatalf("SNI 未写入 TLS：%v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("Fingerprint 未写入 uTLS：%v", tls["utls"])
	}
	reality, ok := tls["reality"].(map[string]interface{})
	if !ok || reality["public_key"] != "test-public-key" || reality["short_id"] != "abcd" {
		t.Fatalf("Reality 字段未写入 TLS：%v", tls["reality"])
	}
	certs, ok := tls["certificate"].([]string)
	if !ok || len(certs) != 3 {
		t.Fatalf("固定证书应写入 certificate 数组：%v", tls["certificate"])
	}
	pins, ok := tls["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("证书指纹应写入 certificate_public_key_sha256：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	config, configOK := ech["config"].([]string)
	if !ok || !configOK || len(config) != 2 {
		t.Fatalf("ECHConfigList 未写入 TLS ECH 配置：%v", tls["ech"])
	}
}

func TestAnyTLSEditFieldsPassSingBoxCheck(t *testing.T) {
	if os.Getenv("GO_SINGBOX_CHECK_ANYTLS_EDIT") == "" {
		t.Skip("仅用于手工校验 AnyTLS 编辑字段生成的 sing-box 配置")
	}
	raw := "anytls://pass@example.com:443?security=tls&sni=example.com&fp=chrome&alpn=h2&certificate_public_key_sha256=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%3D#AnyTLS-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("AnyTLS 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 AnyTLS 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "anytls-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestParseHTTPEditFieldsAffectOutbound(t *testing.T) {
	raw := "https://user:pass@example.com:8443?sni=http.example&insecure=1&path=%2Fproxy&host=edge.example#HTTP-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("HTTP 编辑字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("HTTP 应解析出 1 个节点：%d", len(nodes))
	}
	outbound := nodes[0].Outbound
	if outbound["path"] != "/proxy" {
		t.Fatalf("HTTP path 未写入 outbound：%v", outbound)
	}
	headers, ok := outbound["headers"].(map[string]interface{})
	if !ok || headers["Host"] != "edge.example" {
		t.Fatalf("HTTP Host header 未写入 outbound：%v", outbound["headers"])
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok || tls["server_name"] != "http.example" || tls["insecure"] != true {
		t.Fatalf("HTTP TLS 字段未写入 outbound：%v", outbound["tls"])
	}
}

func TestParseHTTPEditTLSFieldsAffectOutbound(t *testing.T) {
	raw := "https://user:pass@example.com:8443?sni=http.example&fp=chrome&alpn=h2&allowInsecure=1&certificate=-----BEGIN%20CERTIFICATE-----%0Atest%0A-----END%20CERTIFICATE-----&certificate_public_key_sha256=sha256-pin&ech_config=ech-a%0Aech-b#HTTP-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 {
		t.Fatalf("HTTP 编辑 TLS 字段解析不应报错：%v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("HTTP 应解析出 1 个节点：%d", len(nodes))
	}
	tls, ok := nodes[0].Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("HTTP TLS 未写入 outbound：%v", nodes[0].Outbound)
	}
	if tls["server_name"] != "http.example" || tls["insecure"] != true {
		t.Fatalf("HTTP SNI 或跳过证书验证未写入 TLS：%v", tls)
	}
	if alpn, ok := tls["alpn"].([]string); !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("HTTP ALPN 未写入 TLS：%v", tls["alpn"])
	}
	if utls, ok := tls["utls"].(map[string]interface{}); !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("HTTP Fingerprint 未写入 TLS：%v", tls["utls"])
	}
	if certs, ok := tls["certificate"].([]string); !ok || len(certs) == 0 {
		t.Fatalf("HTTP 固定证书未写入 TLS：%v", tls["certificate"])
	}
	if pins, ok := tls["certificate_public_key_sha256"].([]string); !ok || len(pins) != 1 || pins[0] != "sha256-pin" {
		t.Fatalf("HTTP 证书指纹未写入 TLS：%v", tls["certificate_public_key_sha256"])
	}
	ech, ok := tls["ech"].(map[string]interface{})
	if !ok || ech["enabled"] != true {
		t.Fatalf("HTTP ECH 未写入 TLS：%v", tls["ech"])
	}
	echConfigs, ok := ech["config"].([]string)
	if !ok || len(echConfigs) != 2 || echConfigs[0] != "ech-a" || echConfigs[1] != "ech-b" {
		t.Fatalf("HTTP ECHConfigList 未写入 TLS：%v", ech["config"])
	}
	if _, ok := nodes[0].Outbound["transport"]; ok {
		t.Fatalf("当前 sing-box HTTP 不支持 transport，不应写入：%v", nodes[0].Outbound["transport"])
	}
}

func TestHTTPEditFieldsPassSingBoxCheck(t *testing.T) {
	raw := "https://user:pass@example.com:8443?sni=http.example&fp=chrome&alpn=h2&allowInsecure=1#HTTP-Edit"
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("HTTP 编辑字段解析失败：nodes=%d errs=%v", len(nodes), errs)
	}
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成 HTTP 编辑字段配置失败：%v", err)
	}
	path := filepath.Join(t.TempDir(), "http-edit.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("编码 HTTP 编辑字段配置失败：%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入 HTTP 编辑字段配置失败：%v", err)
	}
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", path)
	if err != nil {
		t.Fatalf("sing-box check 失败：%s：%v", out, err)
	}
}

func TestExportSupportedProtocolCheckConfig(t *testing.T) {
	output := os.Getenv("GO_SINGBOX_EXPORT_PROTOCOL_CHECK_CONFIG")
	if output == "" {
		t.Skip("仅用于手工导出各种协议 sing-box check 配置")
	}
	nodes := testSupportedProtocolNodes(t)
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings: Settings{
			Mode:               "global",
			MixedPort:          20808,
			RuleSource:         ruleSourceBuiltin,
			RuleUpdateInterval: "24h",
		},
		Nodes: nodes,
	}
	config, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("生成各种协议检查配置失败：%v", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("序列化各种协议检查配置失败：%v", err)
	}
	if err := os.WriteFile(output, data, 0644); err != nil {
		t.Fatalf("写出各种协议检查配置失败：%v", err)
	}
}

func TestParseClashSubscriptionAnyTLS(t *testing.T) {
	content := `
proxies:
  - name: 香港 AnyTLS
    type: anytls
    server: example.com
    port: 443
    password: b770312b-3683-4f6f-8487-dcca37cc6ff3
    sni: bing.com
    skip-cert-verify: true
proxy-groups:
  - name: 自动选择
    type: select
    proxies:
      - 香港 AnyTLS
`
	nodes, groups, errs := parseClashSubscription(content, "https://sub.example.com/a")
	if len(errs) > 0 {
		t.Fatalf("Clash AnyTLS 订阅不应报错：%v", errs)
	}
	if len(nodes) != 1 || nodes[0].Type != "anytls" {
		t.Fatalf("Clash AnyTLS 订阅解析错误：nodes=%+v groups=%+v", nodes, groups)
	}
	if nodes[0].GroupID == "" || len(groups) != 1 {
		t.Fatalf("Clash AnyTLS 订阅应保留分组：nodes=%+v groups=%+v", nodes, groups)
	}
}

func TestStoreSelectAndDeleteKeepsSelectedNodeValid(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	nodes := []Node{
		{ID: "n0", Name: "默认节点", GroupID: defaultGroupID},
		{ID: "n1", Name: "分组节点", GroupID: "g1"},
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "默认", Source: "manual"},
		{ID: "g1", Name: "分组", Source: "manual"},
	}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, "n1"); err != nil {
		t.Fatalf("写入节点失败：%v", err)
	}
	if err := store.SelectNode("missing"); err == nil {
		t.Fatal("选择不存在节点应报错")
	}
	if selected := store.Snapshot().SelectedID; selected != "n1" {
		t.Fatalf("选择失败后不应改变当前节点：%s", selected)
	}
	if err := store.DeleteGroup("g1"); err != nil {
		t.Fatalf("删除分组失败：%v", err)
	}
	state := store.Snapshot()
	if state.SelectedID != "" {
		t.Fatalf("删除当前节点所在分组后不应自动选中剩余节点：%s", state.SelectedID)
	}
	if len(state.Nodes) != 1 || state.Nodes[0].ID != "n0" {
		t.Fatalf("删除分组后节点列表错误：%+v", state.Nodes)
	}
	if err := store.DeleteGroup(defaultGroupID); err == nil {
		t.Fatal("默认分组不允许删除")
	}
}

func TestSpeedTestOptionsPreferCoreForSOCKS5(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store}

	socksNode := Node{
		Type:    "socks5",
		Address: "202.47.187.212",
		Port:    7778,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "202.47.187.212",
			"server_port": 7778,
			"version":     "5",
			"username":    "user",
			"password":    "pass",
		},
	}
	if got := app.speedTestOptionsForCurrentSettings(SpeedTestOptions{}, socksNode); !got.PreferCore {
		t.Fatal("SOCKS5 节点应使用 sing-box 临时内核检测，避免系统代理模式下原生检测链路不一致")
	}
	if got := app.speedTestOptionsForCurrentSettings(SpeedTestOptions{UseSystemProxy: true}, socksNode); got.PreferCore {
		t.Fatal("SK5 原始批量测试选择系统代理时，应走本地混合端口再连接上游 SK5")
	}
	if got := app.speedTestOptionsForCurrentSettings(SpeedTestOptions{DirectOnly: true}, socksNode); got.PreferCore {
		t.Fatal("节点列表 SOCKS5 测试要求本地直连时，不应强制走核心检测")
	}

	httpNode := Node{
		Type:    "http",
		Address: "127.0.0.1",
		Port:    8080,
		Outbound: map[string]interface{}{
			"type":        "http",
			"server":      "127.0.0.1",
			"server_port": 8080,
		},
	}
	if got := app.speedTestOptionsForCurrentSettings(SpeedTestOptions{}, httpNode); got.PreferCore {
		t.Fatal("未开启 TUN 时 HTTP 代理不应被 SOCKS5 修复逻辑强制走核心检测")
	}
}

func TestReorderGroupsKeepsDefaultFirst(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "默认", Source: "manual"},
		{ID: "g1", Name: "一组", Source: "manual"},
		{ID: "g2", Name: "二组", Source: "manual"},
		{ID: "g3", Name: "三组", Source: "subscription", URL: "https://example.com/sub"},
	}
	nodes := []Node{
		{ID: "n1", Name: "节点1", GroupID: "g1"},
		{ID: "n2", Name: "节点2", GroupID: "g2"},
	}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, ""); err != nil {
		t.Fatalf("写入分组失败：%v", err)
	}
	if err := store.ReorderGroups([]string{"g3", defaultGroupID, "g1"}); err != nil {
		t.Fatalf("分组排序失败：%v", err)
	}
	state := store.Snapshot()
	got := []string{}
	for _, group := range state.Groups {
		got = append(got, group.ID)
	}
	want := []string{defaultGroupID, "g3", "g1", "g2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("分组顺序错误：got %v want %v", got, want)
	}
	if state.Nodes[0].GroupID != "g1" || state.Nodes[1].GroupID != "g2" {
		t.Fatalf("分组排序不应改变节点归属：%+v", state.Nodes)
	}
}

func TestReorderNodesOnlyChangesCurrentGroup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "default", Source: "manual"},
		{ID: "g1", Name: "group1", Source: "manual"},
		{ID: "g2", Name: "group2", Source: "manual"},
	}
	nodes := []Node{
		{ID: "d1", Name: "default1", GroupID: defaultGroupID},
		{ID: "g1-a", Name: "group1-a", GroupID: "g1"},
		{ID: "g2-a", Name: "group2-a", GroupID: "g2"},
		{ID: "g1-b", Name: "group1-b", GroupID: "g1"},
		{ID: "g1-c", Name: "group1-c", GroupID: "g1"},
	}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, "g1-b"); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if err := store.ReorderNodes("g1", []string{"g1-c", "g1-a"}); err != nil {
		t.Fatalf("reorder nodes failed: %v", err)
	}
	state := store.Snapshot()
	got := []string{}
	for _, node := range state.Nodes {
		got = append(got, node.ID)
	}
	want := []string{"d1", "g1-c", "g2-a", "g1-a", "g1-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("node order mismatch: got %v want %v", got, want)
	}
	if state.SelectedID != "g1-b" {
		t.Fatalf("selected node changed: %s", state.SelectedID)
	}
}

func TestImportNodesDoesNotAutoSelectFirstNode(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	if _, err := app.ImportNodes("trojan://pass@example.com:443#first"); err != nil {
		t.Fatalf("导入节点失败：%v", err)
	}
	state := store.Snapshot()
	if len(state.Nodes) != 1 {
		t.Fatalf("导入节点数量错误：%d", len(state.Nodes))
	}
	if state.SelectedID != "" {
		t.Fatalf("首次导入后不应自动选中节点：%s", state.SelectedID)
	}
}

func TestImportNodesToGroupUsesTargetGroup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	group, err := app.CreateGroup("手动分组")
	if err != nil {
		t.Fatalf("创建分组失败：%v", err)
	}
	if _, err := app.ImportNodesToGroup("trojan://pass@example.com:443#target-group", group.ID); err != nil {
		t.Fatalf("导入节点到分组失败：%v", err)
	}
	state := store.Snapshot()
	if len(state.Nodes) != 1 {
		t.Fatalf("导入节点数量错误：%d", len(state.Nodes))
	}
	if state.Nodes[0].GroupID != group.ID {
		t.Fatalf("节点分组错误：got %s want %s", state.Nodes[0].GroupID, group.ID)
	}
}

func TestImportDuplicateNodeRequiresConfirmation(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	raw := "trojan://pass@example.com:443#duplicate-node"
	if _, err := app.ImportNodes(raw); err != nil {
		t.Fatalf("首次导入节点失败：%v", err)
	}

	result, err := app.ImportNodesToGroupWithDuplicates(raw, defaultGroupID, false)
	if err != nil {
		t.Fatalf("检测重复节点失败：%v", err)
	}
	if result["requiresConfirmation"] != true || result["duplicateCount"] != 1 {
		t.Fatalf("重复节点检测结果错误：%v", result)
	}
	if got := len(store.Snapshot().Nodes); got != 1 {
		t.Fatalf("确认前不应写入节点：%d", got)
	}

	result, err = app.ImportNodesToGroupWithDuplicates(raw, defaultGroupID, true)
	if err != nil {
		t.Fatalf("确认导入重复节点失败：%v", err)
	}
	state := store.Snapshot()
	if result["added"] != 1 || len(state.Nodes) != 2 {
		t.Fatalf("重复节点未完整导入：result=%v nodes=%d", result, len(state.Nodes))
	}
	if state.Nodes[0].ID == state.Nodes[1].ID || state.Nodes[0].Raw != state.Nodes[1].Raw {
		t.Fatalf("重复节点应保留相同链接并使用不同 ID：%+v", state.Nodes)
	}
}

func TestImportDuplicateNodeAcrossGroups(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	group, err := app.CreateGroup("重复节点分组")
	if err != nil {
		t.Fatalf("创建分组失败：%v", err)
	}
	raw := "trojan://pass@example.com:443#cross-group-duplicate"
	if _, err := app.ImportNodes(raw); err != nil {
		t.Fatalf("首次导入节点失败：%v", err)
	}
	result, err := app.ImportNodesToGroupWithDuplicates(raw, group.ID, false)
	if err != nil {
		t.Fatalf("跨分组检测重复节点失败：%v", err)
	}
	if result["requiresConfirmation"] != true || result["duplicateCount"] != 1 {
		t.Fatalf("跨分组重复节点检测结果错误：%v", result)
	}
	if _, err := app.ImportNodesToGroupWithDuplicates(raw, group.ID, true); err != nil {
		t.Fatalf("跨分组确认导入失败：%v", err)
	}
	state := store.Snapshot()
	if len(state.Nodes) != 2 || state.Nodes[1].GroupID != group.ID {
		t.Fatalf("跨分组重复节点导入位置错误：%+v", state.Nodes)
	}
}

func TestImportDuplicateNodesFromSameInput(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatalf("初始化存储失败：%v", err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	raw := "trojan://pass@example.com:443#same-input-duplicate"
	input := raw + "\n" + raw
	result, err := app.ImportNodesToGroupWithDuplicates(input, defaultGroupID, false)
	if err != nil {
		t.Fatalf("检测同批重复节点失败：%v", err)
	}
	if result["requiresConfirmation"] != true || result["duplicateCount"] != 1 {
		t.Fatalf("同批重复节点检测结果错误：%v", result)
	}
	if got := len(store.Snapshot().Nodes); got != 0 {
		t.Fatalf("确认前不应导入同批节点：%d", got)
	}
	if _, err := app.ImportNodesToGroupWithDuplicates(input, defaultGroupID, true); err != nil {
		t.Fatalf("确认导入同批重复节点失败：%v", err)
	}
	state := store.Snapshot()
	if len(state.Nodes) != 2 || state.Nodes[0].ID == state.Nodes[1].ID {
		t.Fatalf("同批重复节点未完整保留：%+v", state.Nodes)
	}
}

func TestStartRequiresManualSelectedNode(t *testing.T) {
	logger := NewLogger(filepath.Join(t.TempDir(), "app.log"))
	manager := NewSingBoxManager("", filepath.Join(t.TempDir(), "missing-sing-box.exe"), "", "", logger)
	err := manager.Start(PersistState{
		Settings: defaultSettings(),
		Nodes: []Node{
			{ID: "n1", Name: "node", Type: "trojan", Address: "example.com", Port: 443},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "请先选择节点") {
		t.Fatalf("未选择节点启动应提示先选择节点：%v", err)
	}
}

func TestNodeListAffectsRunningConfig(t *testing.T) {
	previous := testConfigState("rule")
	previous.Nodes = append(previous.Nodes, Node{
		ID:      "n2",
		Name:    "备用节点",
		Type:    "socks",
		Address: "127.0.0.1",
		Port:    10002,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "127.0.0.1",
			"server_port": 10002,
		},
	})
	next := previous
	next.Nodes = append([]Node{}, previous.Nodes...)
	next.Nodes = append(next.Nodes, Node{ID: "n3", Name: "新增节点"})
	if nodeListAffectsRunningConfig(previous, next) {
		t.Fatal("未启用代理映射时新增非当前节点不应触发重启")
	}
	next = previous
	next.SelectedID = "n2"
	if !nodeListAffectsRunningConfig(previous, next) {
		t.Fatal("当前选中节点变化应触发重启")
	}
	next = previous
	next.Settings.ProxyMap = true
	if !nodeListAffectsRunningConfig(previous, next) {
		t.Fatal("启用代理映射时节点列表变化应触发重启")
	}
	next = previous
	next.Nodes = append([]Node{}, previous.Nodes...)
	next.Nodes[0].Outbound = cloneMap(previous.Nodes[0].Outbound)
	next.Nodes[0].Outbound["server_port"] = 10003
	if !nodeListAffectsRunningConfig(previous, next) {
		t.Fatal("当前节点出站变化应触发重启")
	}
}

func TestCanApplySettingsWhileRunning(t *testing.T) {
	current := testConfigState("rule").Settings
	next := current
	next.Tun = !current.Tun
	if !canApplySettingsWhileRunning(current, next) {
		t.Fatal("运行中应允许切换 TUN")
	}
	next = current
	next.Mode = "global"
	if !canApplySettingsWhileRunning(current, next) {
		t.Fatal("运行中应允许切换规则/全局")
	}
	next = current
	next.MixedPort = 20809
	if canApplySettingsWhileRunning(current, next) {
		t.Fatal("运行中不应允许修改本地端口")
	}
}

func TestShouldRunProxy(t *testing.T) {
	settings := Settings{}
	if shouldRunProxy(settings) {
		t.Fatal("未开启代理入口时不应自动运行")
	}
	settings.SystemProxy = true
	if !shouldRunProxy(settings) {
		t.Fatal("开启系统代理时应自动运行")
	}
	settings.SystemProxy = false
	settings.Tun = true
	if !shouldRunProxy(settings) {
		t.Fatal("开启 TUN 时应自动运行")
	}
	settings.Tun = false
	settings.ProxyMap = true
	if !shouldRunProxy(settings) {
		t.Fatal("开启代理映射时应自动运行")
	}
	settings.ProxyMap = false
	settings.AppProxy = true
	if shouldRunProxy(settings) {
		t.Fatal("应用接管没有目标时不应自动运行")
	}
	settings.AppProxyTargets = []AppProxyTarget{{Type: "app", Path: `C:\Tools\App.exe`, Enabled: true}}
	if !shouldRunProxy(settings) {
		t.Fatal("应用接管有目标时应自动运行")
	}
}

func TestRetryableClashAPIError(t *testing.T) {
	if !isRetryableClashAPIError(errors.New("connectex: No connection could be made because the target machine actively refused it")) {
		t.Fatal("控制端口短暂拒绝连接应允许静默重试")
	}
	if isRetryableClashAPIError(errors.New("Clash API 返回 404：not found")) {
		t.Fatal("业务错误不应静默重试")
	}
}

func buildTestVMessLink() (string, error) {
	data := map[string]interface{}{
		"v":    "2",
		"ps":   "VMess",
		"add":  "example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"aid":  "0",
		"scy":  "auto",
		"net":  "ws",
		"tls":  "tls",
		"sni":  "example.com",
		"path": "/ws",
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
}

func testSupportedProtocolNodes(t *testing.T) []Node {
	t.Helper()
	vmessRaw, err := buildTestVMessLink()
	if err != nil {
		t.Fatalf("生成 VMess 测试链接失败：%v", err)
	}
	lines := []string{
		"ss://YWVzLTEyOC1nY206cGFzcw@example.com:8388#SS",
		vmessRaw,
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&sni=example.com&type=ws&path=%2Fws#VLESS",
		"trojan://pass@example.com:443?security=tls&sni=example.com#Trojan",
		"hysteria2://pass@example.com:443?sni=example.com&insecure=1#Hysteria2",
		"socks5://user:pass@example.com:1080#SOCKS5",
		"http://user:pass@example.com:8080#HTTP",
		"tuic://11111111-1111-1111-1111-111111111111:pass@example.com:443?sni=example.com&allowInsecure=1#TUIC",
		"anytls://b770312b-3683-4f6f-8487-dcca37cc6ff3@example.com:443?security=tls&sni=bing.com&insecure=1&type=tcp#AnyTLS",
	}
	nodes, errs := ParseNodes(strings.Join(lines, "\n"))
	if len(errs) > 0 {
		t.Fatalf("协议链接解析不应报错：%v", errs)
	}
	if len(nodes) != len(lines) {
		t.Fatalf("协议链接解析数量错误：got %d want %d", len(nodes), len(lines))
	}
	return nodes
}

func TestShouldCleanupTunOnRunningRestart(t *testing.T) {
	current := testConfigState("rule").Settings
	current.Tun = true
	next := current
	next.Mode = "global"
	if !shouldCleanupTunOnRunningRestart(current, next) {
		t.Fatal("TUN 保持开启时重启内核前应清理旧网卡")
	}
	next = current
	next.Tun = false
	if !shouldCleanupTunOnRunningRestart(current, next) {
		t.Fatal("关闭 TUN 时应清理网卡")
	}
}

func TestForceKillPortProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要测试端口占用进程强杀")
	}
	port, err := freeTestPort()
	if err != nil {
		t.Fatalf("获取测试端口失败：%v", err)
	}
	script := fmt.Sprintf(`$listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Parse("127.0.0.1"),%d); $listener.Start(); try { while($true){ Start-Sleep -Milliseconds 200 } } finally { $listener.Stop() }`, port)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试占用进程失败：%v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	defer func() {
		if !waitTCPPortFreeStrict("127.0.0.1", port, 500*time.Millisecond) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	if !waitTestPortBusy(port, 5*time.Second) {
		t.Fatalf("测试端口 %d 未进入占用状态", port)
	}
	items, err := findTCPPortProcesses("127.0.0.1", port)
	if err != nil {
		t.Fatalf("查询端口占用进程失败：%v", err)
	}
	if len(items) == 0 {
		t.Fatalf("未识别到测试端口 %d 的占用进程", port)
	}
	if err := forceKillProcess(items[0].PID); err != nil {
		t.Fatalf("强杀占用进程失败：%v", err)
	}
	if !waitTCPPortFreeStrict("127.0.0.1", port, 5*time.Second) {
		t.Fatalf("测试端口 %d 强杀后仍未释放", port)
	}
}

func TestForceKillSingBoxPortProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要测试 sing-box 端口占用进程强杀")
	}
	corePath := filepath.Join("bin", exeName("sing-box"))
	if _, err := os.Stat(corePath); err != nil {
		t.Skipf("未找到测试内核：%s", corePath)
	}
	port, err := freeTestPort()
	if err != nil {
		t.Fatalf("获取测试端口失败：%v", err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "sing-box-test.json")
	config := map[string]interface{}{
		"log": map[string]interface{}{"level": "error"},
		"inbounds": []map[string]interface{}{
			{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": port},
		},
		"outbounds": []map[string]interface{}{
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]interface{}{"final": "direct"},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("序列化测试配置失败：%v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("写入测试配置失败：%v", err)
	}

	cmd := exec.Command(corePath, "run", "-c", configPath)
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试 sing-box 失败：%v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	defer func() {
		if !waitTCPPortFreeStrict("127.0.0.1", port, 500*time.Millisecond) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	if !waitTestPortBusy(port, 10*time.Second) {
		t.Fatalf("测试 sing-box 未监听端口 %d", port)
	}
	items, err := findTCPPortProcesses("127.0.0.1", port)
	if err != nil {
		t.Fatalf("查询 sing-box 端口占用进程失败：%v", err)
	}
	if len(items) == 0 {
		t.Fatalf("未识别到 sing-box 端口 %d 的占用进程", port)
	}
	if err := forceKillProcess(items[0].PID); err != nil {
		t.Fatalf("强杀 sing-box 占用进程失败：%v", err)
	}
	if !waitTCPPortFreeStrict("127.0.0.1", port, 5*time.Second) {
		t.Fatalf("sing-box 端口 %d 强杀后仍未释放", port)
	}
}

func TestReleaseStartupPortKillsOwnSingBoxResidual(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要测试本软件残留内核清理")
	}
	corePath, err := filepath.Abs(filepath.Join("bin", exeName("sing-box")))
	if err != nil {
		t.Fatalf("解析测试内核路径失败：%v", err)
	}
	if _, err := os.Stat(corePath); err != nil {
		t.Skipf("未找到测试内核：%s", corePath)
	}
	port, err := freeTestPort()
	if err != nil {
		t.Fatalf("获取测试端口失败：%v", err)
	}
	configPath := filepath.Join(t.TempDir(), "sing-box-test.json")
	config := map[string]interface{}{
		"log": map[string]interface{}{"level": "error"},
		"inbounds": []map[string]interface{}{
			{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": port},
		},
		"outbounds": []map[string]interface{}{
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]interface{}{"final": "direct"},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("序列化测试配置失败：%v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("写入测试配置失败：%v", err)
	}

	cmd := exec.Command(corePath, "run", "-c", configPath)
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试 sing-box 失败：%v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	defer func() {
		if !waitTCPPortFreeStrict("127.0.0.1", port, 500*time.Millisecond) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	if !waitTestPortBusy(port, 10*time.Second) {
		t.Fatalf("测试 sing-box 未监听端口 %d", port)
	}
	logPath := filepath.Join(t.TempDir(), "app.log")
	logger := NewLogger(logPath)
	app := &App{
		logger:  logger,
		manager: NewSingBoxManager("", corePath, "", "", logger),
	}
	items, err := findTCPPortProcesses("127.0.0.1", port)
	if err != nil {
		t.Fatalf("查询测试端口占用失败：%v", err)
	}
	if err := app.releaseStartupPort("本地代理", port, items); err != nil {
		t.Fatalf("释放本软件残留内核占用端口失败：%v", err)
	}
	if !waitTCPPortFreeStrict("127.0.0.1", port, 5*time.Second) {
		t.Fatalf("本地代理端口 %d 清理后仍未释放", port)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("读取测试日志失败：%v", err)
	}
	if !strings.Contains(string(logData), "发现本软件残留内核占用本地代理端口") {
		t.Fatalf("日志未记录本软件残留内核清理：%s", string(logData))
	}
}

func TestReleaseStartupPortDoesNotKillExternalProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要测试外部端口占用保护")
	}
	port, err := freeTestPort()
	if err != nil {
		t.Fatalf("获取测试端口失败：%v", err)
	}
	script := fmt.Sprintf(`$listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Parse("127.0.0.1"),%d); $listener.Start(); try { while($true){ Start-Sleep -Milliseconds 200 } } finally { $listener.Stop() }`, port)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试占用进程失败：%v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	defer func() {
		if !waitTCPPortFreeStrict("127.0.0.1", port, 500*time.Millisecond) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	if !waitTestPortBusy(port, 5*time.Second) {
		t.Fatalf("测试端口 %d 未进入占用状态", port)
	}
	logPath := filepath.Join(t.TempDir(), "app.log")
	logger := NewLogger(logPath)
	app := &App{
		logger:  logger,
		manager: NewSingBoxManager("", filepath.Join(t.TempDir(), "missing-sing-box.exe"), "", "", logger),
	}
	items, err := findTCPPortProcesses("127.0.0.1", port)
	if err != nil {
		t.Fatalf("查询测试端口占用失败：%v", err)
	}
	err = app.releaseStartupPort("本地代理", port, items)
	if err == nil {
		t.Fatalf("外部进程占用端口时不应自动强杀并释放端口")
	}
	if !strings.Contains(err.Error(), "被其他程序占用") {
		t.Fatalf("外部端口占用错误提示不明确：%v", err)
	}
	if waitTCPPortFreeStrict("127.0.0.1", port, 500*time.Millisecond) {
		t.Fatalf("外部占用进程不应被本软件强制结束")
	}
	logData, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("读取测试日志失败：%v", readErr)
	}
	if strings.Contains(string(logData), "强制结束占用本地代理端口") {
		t.Fatalf("日志不应记录强杀外部占用进程：%s", string(logData))
	}
}

func freeTestPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitTestPortBusy(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isTCPPortFree("127.0.0.1", port) {
			return true
		}
		time.Sleep(120 * time.Millisecond)
	}
	return !isTCPPortFree("127.0.0.1", port)
}

func hasConfigItem(items []map[string]interface{}, key string, value interface{}) bool {
	for _, item := range items {
		if item[key] == value {
			return true
		}
	}
	return false
}

func findConfigItem(items []map[string]interface{}, key string, value interface{}) map[string]interface{} {
	for _, item := range items {
		if item[key] == value {
			return item
		}
	}
	return nil
}

func assertReferencedRuleSetsExist(t *testing.T, route map[string]interface{}, dns map[string]interface{}) {
	t.Helper()
	known := map[string]bool{}
	for _, item := range route["rule_set"].([]map[string]interface{}) {
		tag, _ := item["tag"].(string)
		if tag != "" {
			known[tag] = true
		}
	}
	checkRules := func(scope string, rules []map[string]interface{}) {
		t.Helper()
		for _, rule := range rules {
			if tag, ok := rule["rule_set"].(string); ok && !known[tag] {
				t.Fatalf("%s 引用了不存在的规则集 %q：%v", scope, tag, rule)
			}
			if tags, ok := rule["rule_set"].([]string); ok {
				for _, tag := range tags {
					if !known[tag] {
						t.Fatalf("%s 引用了不存在的规则集 %q：%v", scope, tag, rule)
					}
				}
			}
		}
	}
	checkRules("route.rules", route["rules"].([]map[string]interface{}))
	checkRules("dns.rules", dns["rules"].([]map[string]interface{}))
}

func assertDNSRuleServersExist(t *testing.T, dns map[string]interface{}) {
	t.Helper()
	known := map[string]bool{}
	for _, server := range dns["servers"].([]map[string]interface{}) {
		tag, _ := server["tag"].(string)
		if tag != "" {
			known[tag] = true
		}
	}
	if final, _ := dns["final"].(string); final != "" && !known[final] {
		t.Fatalf("DNS final 引用了不存在的服务器 %q：%v", final, dns)
	}
	for _, rule := range dns["rules"].([]map[string]interface{}) {
		server, _ := rule["server"].(string)
		if server != "" && !known[server] {
			t.Fatalf("DNS 规则引用了不存在的服务器 %q：%v", server, rule)
		}
	}
}

func hasRouteOutbound(rules []map[string]interface{}, outbound string) bool {
	for _, rule := range rules {
		if rule["outbound"] == outbound {
			return true
		}
	}
	return false
}

func hasDirectRouteForProcessPathPattern(config map[string]interface{}, pattern string) bool {
	route, _ := config["route"].(map[string]interface{})
	for _, rule := range configRules(route["rules"]) {
		if rule["outbound"] != "direct" {
			continue
		}
		if stringSliceContains(configStringValues(rule["process_path_regex"]), pattern) {
			return true
		}
	}
	return false
}

func hasDirectRouteForProcessName(config map[string]interface{}, name string) bool {
	route, _ := config["route"].(map[string]interface{})
	for _, rule := range configRules(route["rules"]) {
		if rule["outbound"] != "direct" {
			continue
		}
		if stringSliceContains(configStringValues(rule["process_name"]), name) {
			return true
		}
	}
	return false
}

func hasTunStabilityBlockRule(config map[string]interface{}) bool {
	route, _ := config["route"].(map[string]interface{})
	rules, _ := route["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if rule["outbound"] != "block" || rule["port"] == nil {
			continue
		}
		inbounds, _ := rule["inbound"].([]string)
		if !stringSliceContains(inbounds, "tun-in") {
			continue
		}
		ports, _ := rule["port"].([]int)
		if stringIntSliceContains(ports, 5353) {
			return true
		}
	}
	return false
}

func hasDirectDomainRule(rules []map[string]interface{}, domain string) bool {
	for _, rule := range rules {
		if rule["outbound"] != "direct" {
			continue
		}
		for _, key := range []string{"domain", "domain_suffix", "domain_keyword"} {
			if values, ok := rule[key].([]string); ok && stringSliceContains(values, domain) {
				return true
			}
		}
	}
	return false
}

func hasDNSRuleForDomainServer(rules []map[string]interface{}, domain string, server string) bool {
	for _, rule := range rules {
		if rule["server"] != server {
			continue
		}
		for _, key := range []string{"domain", "domain_suffix", "domain_keyword"} {
			if values, ok := rule[key].([]string); ok && stringSliceContains(values, domain) {
				return true
			}
		}
	}
	return false
}

func stringIntSliceContains(items []int, value int) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func stringSliceContainsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func testConfigState(mode string) PersistState {
	return PersistState{
		SelectedID: "n1",
		Settings: Settings{
			Mode:               mode,
			MixedPort:          20808,
			DirectDomains:      "ai-pixel.online",
			RuleSource:         ruleSourceLoyalsoldier,
			AutoUpdateRules:    true,
			RuleUpdateInterval: "24h",
		},
		Nodes: []Node{
			{
				ID:      "n1",
				Name:    "测试节点",
				Type:    "socks",
				Address: "127.0.0.1",
				Port:    10001,
				Outbound: map[string]interface{}{
					"type":        "socks",
					"server":      "127.0.0.1",
					"server_port": 10001,
				},
			},
		},
	}
}

func TestStopLockedClearsTunRuntimeState(t *testing.T) {
	manager := &SingBoxManager{tunActive: true}
	if err := manager.stopLocked(false, false, false); err != nil {
		t.Fatal(err)
	}
	if manager.tunActive {
		t.Fatal("停止后仍保留 TUN 运行状态")
	}
}

func TestCleanupStaleWaitsForManagerOperation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅验证 Windows 进程清理串行化")
	}
	manager := &SingBoxManager{corePath: filepath.Join(t.TempDir(), "sing-box.exe")}
	manager.mu.Lock()
	done := make(chan error, 1)
	go func() {
		done <- manager.CleanupStale()
	}()
	select {
	case err := <-done:
		manager.mu.Unlock()
		t.Fatalf("残留清理未等待管理器操作结束：%v", err)
	case <-time.After(100 * time.Millisecond):
	}
	manager.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("管理器操作结束后残留清理仍未完成")
	}
}

func TestDirectEntriesNormalizationAndClassification(t *testing.T) {
	input := `
		http://example.com:8080/path
		https://test.org?q=1
		*.sub.google.com
		1.1.1.1:443
		192.168.1.0/24
		10.0.0.1
		[2001:db8::1]:8080
		localhost
		invalid..host
	`
	domains, ipCIDRs := parseDirectEntries(input)

	wantDomains := []string{"example.com", "test.org", "sub.google.com", "localhost"}
	for _, wd := range wantDomains {
		if !stringSliceContains(domains, wd) {
			t.Errorf("domains 应包含 %q，实际 domains=%v", wd, domains)
		}
	}
	if stringSliceContains(domains, "invalid..host") {
		t.Errorf("domains 不应包含无效主机名 invalid..host")
	}

	wantCIDRs := []string{"1.1.1.1/32", "192.168.1.0/24", "10.0.0.1/32", "2001:db8::1/128"}
	for _, wc := range wantCIDRs {
		if !stringSliceContains(ipCIDRs, wc) {
			t.Errorf("ipCIDRs 应包含 %q，实际 ipCIDRs=%v", wc, ipCIDRs)
		}
	}
}

func TestDirectEntriesRouteRuleGeneratesIPCIDR(t *testing.T) {
	state := testConfigState("rule")
	state.Settings.DirectDomains = "example.com\n1.1.1.1\n192.168.1.0/24\n[2001:db8::1]:443"
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成配置失败：%v", err)
	}

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 route 配置：%v", config)
	}
	rules, _ := route["rules"].([]map[string]interface{})

	var foundDomainRule, foundCIDRRule bool
	for _, rule := range rules {
		if rule["outbound"] != "direct" || rule["clash_mode"] != "rule" {
			continue
		}
		if doms, hasDom := rule["domain"].([]string); hasDom {
			if stringSliceContains(doms, "example.com") {
				foundDomainRule = true
			}
		}
		if cidrs, hasCIDR := rule["ip_cidr"].([]string); hasCIDR {
			if stringSliceContains(cidrs, "1.1.1.1/32") &&
				stringSliceContains(cidrs, "192.168.1.0/24") &&
				stringSliceContains(cidrs, "2001:db8::1/128") {
				foundCIDRRule = true
			}
		}
	}
	if !foundDomainRule {
		t.Fatalf("路由规则中缺少自定义直连域名规则：%v", rules)
	}
	if !foundCIDRRule {
		t.Fatalf("路由规则中缺少自定义直连 IP CIDR 规则：%v", rules)
	}

	// 确保生成的完整配置能被 sing-box.exe 成功解析
	corePath := filepath.Join(".", "bin", "sing-box.exe")
	if _, statErr := os.Stat(corePath); statErr == nil {
		data, marshalErr := json.Marshal(config)
		if marshalErr != nil {
			t.Fatalf("序列化配置失败：%v", marshalErr)
		}
		tmpFile := filepath.Join(t.TempDir(), "test_config.json")
		if writeErr := os.WriteFile(tmpFile, data, 0644); writeErr != nil {
			t.Fatalf("写入临时配置文件失败：%v", writeErr)
		}
		cmd := exec.Command(corePath, "check", "-c", tmpFile)
		output, checkErr := cmd.CombinedOutput()
		if checkErr != nil {
			t.Fatalf("sing-box check 验证失败：%v，输出：%s", checkErr, string(output))
		}
	}
}

func TestRemoteRuleSetsPriorityProxyOverGeoIPAndChina(t *testing.T) {
	state := testConfigState("rule")
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成规则配置失败：%v", err)
	}

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 route 配置")
	}
	routeRules, _ := route["rules"].([]map[string]interface{})

	proxyIndex := -1
	geoIPCNIndex := -1
	chinaIndex := -1
	directIndex := -1

	for idx, r := range routeRules {
		rs, _ := r["rule_set"].(string)
		switch rs {
		case "loyalsoldier-proxy":
			proxyIndex = idx
		case "geoip-cn":
			geoIPCNIndex = idx
		case "loyalsoldier-china":
			chinaIndex = idx
		case "loyalsoldier-direct":
			directIndex = idx
		}
	}

	if proxyIndex == -1 {
		t.Fatalf("路由规则中缺少 loyalsoldier-proxy 规则：%v", routeRules)
	}
	if geoIPCNIndex == -1 {
		t.Fatalf("路由规则中缺少 geoip-cn 规则：%v", routeRules)
	}
	if chinaIndex == -1 {
		t.Fatalf("路由规则中缺少 loyalsoldier-china 规则：%v", routeRules)
	}
	if directIndex == -1 {
		t.Fatalf("路由规则中缺少 loyalsoldier-direct 规则：%v", routeRules)
	}

	if proxyIndex >= geoIPCNIndex {
		t.Fatalf("loyalsoldier-proxy (idx=%d) 必须置于 geoip-cn (idx=%d) 之前，防止 Google 等境外网站因包含中国 IP 段 (如 203.208.32.0/19) 被错误分流至直连导致证书错误 (ERR_CERT_COMMON_NAME_INVALID)", proxyIndex, geoIPCNIndex)
	}
	if proxyIndex >= chinaIndex {
		t.Fatalf("loyalsoldier-proxy (idx=%d) 必须置于 loyalsoldier-china (idx=%d) 之前", proxyIndex, chinaIndex)
	}
	if proxyIndex >= directIndex {
		t.Fatalf("loyalsoldier-proxy (idx=%d) 必须置于 loyalsoldier-direct (idx=%d) 之前", proxyIndex, directIndex)
	}

	dns, ok := config["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 dns 配置")
	}
	dnsRules, _ := dns["rules"].([]map[string]interface{})

	proxyDNSIndex := -1
	chinaDNSIndex := -1
	directDNSIndex := -1

	for idx, r := range dnsRules {
		rs, _ := r["rule_set"].(string)
		switch rs {
		case "loyalsoldier-proxy":
			proxyDNSIndex = idx
		case "loyalsoldier-china":
			chinaDNSIndex = idx
		case "loyalsoldier-direct":
			directDNSIndex = idx
		}
	}

	if proxyDNSIndex == -1 {
		t.Fatalf("DNS 规则中缺少 loyalsoldier-proxy 规则：%v", dnsRules)
	}
	if chinaDNSIndex == -1 {
		t.Fatalf("DNS 规则中缺少 loyalsoldier-china 规则：%v", dnsRules)
	}
	if directDNSIndex == -1 {
		t.Fatalf("DNS 规则中缺少 loyalsoldier-direct 规则：%v", dnsRules)
	}

	if proxyDNSIndex >= chinaDNSIndex {
		t.Fatalf("DNS loyalsoldier-proxy (idx=%d) 必须置于 loyalsoldier-china (idx=%d) 之前", proxyDNSIndex, chinaDNSIndex)
	}
	if proxyDNSIndex >= directDNSIndex {
		t.Fatalf("DNS loyalsoldier-proxy (idx=%d) 必须置于 loyalsoldier-direct (idx=%d) 之前", proxyDNSIndex, directDNSIndex)
	}
}


