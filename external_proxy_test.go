package main

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

func TestDecodeExternalProxyProbeRequestAcceptsUTF8BOM(t *testing.T) {
	payload, err := json.Marshal(externalProxyProbeRequest{
		Path:     "本机直连",
		Endpoint: ExternalProxyEndpoint{Host: "127.0.0.1", Port: 7778, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload = append([]byte{0xef, 0xbb, 0xbf}, payload...)
	request, err := decodeExternalProxyProbeRequest(payload)
	if err != nil {
		t.Fatalf("带 UTF-8 BOM 的 helper 参数解析失败：%v", err)
	}
	if request.Endpoint.Port != 7778 || request.Path != "本机直连" {
		t.Fatalf("helper 参数解析错误：%+v", request)
	}
}

func TestExternalProxyRouteIsExactAndPrecedesGlobalProxy(t *testing.T) {
	state := testConfigState("global")
	state.Settings.SystemProxy = true
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "85.149.223.86", Port: 10171, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("生成外部代理入口配置失败：%v", err)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]map[string]interface{})
	directIndex := -1
	globalIndex := -1
	for index, rule := range rules {
		if rule["clash_mode"] == "global" && rule["outbound"] == "proxy" {
			globalIndex = index
		}
		if rule["outbound"] == "direct" && configPortContains(rule["port"], 10171) &&
			stringSliceContains(configStringValues(rule["ip_cidr"]), "85.149.223.86/32") {
			directIndex = index
		}
	}
	if directIndex < 0 {
		t.Fatalf("缺少外部 SOCKS5 精确直连规则：%v", rules)
	}
	if rules[directIndex]["network"] != "tcp" {
		t.Fatalf("外部 SOCKS5 入口只能生成 TCP 直连规则：%v", rules[directIndex])
	}
	if globalIndex < 0 || directIndex >= globalIndex {
		t.Fatalf("外部代理入口规则必须位于全局代理规则之前：direct=%d global=%d", directIndex, globalIndex)
	}
	if configPortContains(rules[directIndex]["port"], 10172) {
		t.Fatal("同 IP 的其他端口不得被外部代理入口规则直连")
	}
	if summary := configModeFromConfigWithExternalEndpoints(config, "global", state.Settings.ExternalProxyEndpoints); summary.HasDirect || summary.Mode != "global" {
		t.Fatalf("外部代理入口直连不得被误判为规则模式残留：%+v", summary)
	}
	if summary := configModeFromConfig(config, "global"); !summary.HasDirect {
		t.Fatal("未提供已保存端点时，不得笼统豁免外部形态的 direct 规则")
	}
}

func TestExternalProxyDomainIncludesResolvedIPv4AndIPv6(t *testing.T) {
	settings := Settings{ExternalProxyEndpoints: []ExternalProxyEndpoint{
		{
			Host:        "proxy.example.com",
			Port:        1080,
			Enabled:     true,
			ResolvedIPs: []string{"203.0.113.7", "2001:db8::7"},
		},
	}}
	rules := buildExternalProxyDirectRules(settings)
	if len(rules) != 2 {
		t.Fatalf("域名入口应生成域名规则和解析地址规则：%v", rules)
	}
	if !stringSliceContains(configStringValues(rules[0]["domain"]), "proxy.example.com") || !configPortContains(rules[0]["port"], 1080) {
		t.Fatalf("域名直连规则不完整：%v", rules[0])
	}
	cidrs := configStringValues(rules[1]["ip_cidr"])
	if !stringSliceContains(cidrs, "203.0.113.7/32") || !stringSliceContains(cidrs, "2001:db8::7/128") {
		t.Fatalf("域名解析地址规则缺少 IPv4/IPv6：%v", rules[1])
	}
}

func TestDisabledExternalProxyEndpointDoesNotChangeRouting(t *testing.T) {
	settings := Settings{ExternalProxyEndpoints: []ExternalProxyEndpoint{
		{Host: "85.149.223.86", Port: 10171, Enabled: false},
	}}
	if rules := buildExternalProxyDirectRules(settings); len(rules) != 0 {
		t.Fatalf("已停用的外部代理入口不应生成规则：%v", rules)
	}
}

func TestExternalProxyRoutesSurviveRuntimeJSON(t *testing.T) {
	state := testConfigState("global")
	state.Settings.Tun = true
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "85.149.223.86", Port: 10171, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var runtimeConfig map[string]interface{}
	if err := json.Unmarshal(data, &runtimeConfig); err != nil {
		t.Fatal(err)
	}
	if !hasExternalProxyRoutes(runtimeConfig, state.Settings.ExternalProxyEndpoints) {
		t.Fatal("运行配置 JSON 重新读取后应仍能确认外部代理入口规则")
	}
}

func TestExternalProxyRoutesMustPrecedeGlobalRule(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "proxy.example.com", Port: 1080, Enabled: true, ResolvedIPs: []string{"203.0.113.7"}},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatal(err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	globalIndex := -1
	for index, rule := range rules {
		if rule["clash_mode"] == "global" && rule["outbound"] == "proxy" {
			globalIndex = index
			break
		}
	}
	if globalIndex < 0 {
		t.Fatal("测试配置缺少全局代理规则")
	}
	externalRules := append([]map[string]interface{}(nil), rules[3:globalIndex]...)
	reordered := append([]map[string]interface{}(nil), rules[:3]...)
	reordered = append(reordered, rules[globalIndex])
	reordered = append(reordered, externalRules...)
	reordered = append(reordered, rules[globalIndex+1:]...)
	route["rules"] = reordered
	if hasExternalProxyRoutes(config, state.Settings.ExternalProxyEndpoints) {
		t.Fatal("位于全局代理规则之后的外部入口直连规则不应通过运行校验")
	}
}

func TestExternalProxyRoutesRequireResolvedAddresses(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "proxy.example.com", Port: 1080, Enabled: true, ResolvedIPs: []string{"203.0.113.7"}},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatal(err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	for index, rule := range rules {
		if stringSliceContains(configStringValues(rule["ip_cidr"]), "203.0.113.7/32") {
			rules = append(rules[:index], rules[index+1:]...)
			break
		}
	}
	route["rules"] = rules
	if hasExternalProxyRoutes(config, state.Settings.ExternalProxyEndpoints) {
		t.Fatal("域名入口缺少已解析地址规则时不应通过运行校验")
	}
}

func TestExternalProxyRoutesRequireTCP(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "203.0.113.7", Port: 1080, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatal(err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if configPortContains(rule["port"], 1080) && rule["outbound"] == "direct" {
			delete(rule, "network")
			break
		}
	}
	if hasExternalProxyRoutes(config, state.Settings.ExternalProxyEndpoints) {
		t.Fatal("缺少 TCP 限定的外部入口规则不应通过运行校验")
	}
}

func TestGlobalModeRejectsUnregisteredDirectRule(t *testing.T) {
	state := testConfigState("global")
	state.Settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{
		{Host: "203.0.113.7", Port: 1080, Enabled: true},
	}
	config, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatal(err)
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	unknown := externalProxyIPRule([]string{"198.51.100.9/32"}, []int{1081})
	rules = append(rules[:3], append([]map[string]interface{}{unknown}, rules[3:]...)...)
	route["rules"] = rules
	summary := configModeFromConfigWithExternalEndpoints(config, "global", state.Settings.ExternalProxyEndpoints)
	if !summary.HasDirect {
		t.Fatal("全局模式不得豁免未登记的 TCP direct 规则")
	}
}

func TestNormalizeExternalProxyEndpointsDeduplicatesExactEndpoint(t *testing.T) {
	endpoints := normalizeExternalProxyEndpoints([]ExternalProxyEndpoint{
		{Host: "Proxy.Example.com.", Port: 1080, Enabled: true},
		{Host: "proxy.example.com", Port: 1080, Enabled: true},
		{Host: "proxy.example.com", Port: 1081, Enabled: true},
	})
	if len(endpoints) != 2 {
		t.Fatalf("只应去重完全相同的主机和端口：%+v", endpoints)
	}
	if endpoints[0].Host != "proxy.example.com" {
		t.Fatalf("域名应规范化：%+v", endpoints[0])
	}
}

func TestSOCKS5EndpointProbeThroughLocalMixedInbound(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(conn, greeting); err != nil {
			done <- err
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			done <- err
			return
		}
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			done <- err
			return
		}
		addressLength := 0
		switch header[3] {
		case 1:
			addressLength = 4
		case 4:
			addressLength = 16
		case 3:
			length := []byte{0}
			if _, err := io.ReadFull(conn, length); err != nil {
				done <- err
				return
			}
			addressLength = int(length[0])
		}
		if _, err := io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
			done <- err
			return
		}
		if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
			done <- err
			return
		}
		remoteGreeting := make([]byte, 4)
		if _, err := io.ReadFull(conn, remoteGreeting); err != nil {
			done <- err
			return
		}
		_, err = conn.Write([]byte{5, 2})
		done <- err
	}()

	conn, err := dialSOCKS5Target(listener.Addr().String(), ExternalProxyEndpoint{
		Host: "85.149.223.86",
		Port: 10171,
	}, time.Second)
	if err != nil {
		t.Fatalf("通过本地 mixed 入站连接远程 SOCKS5 失败：%v", err)
	}
	defer conn.Close()
	auth, err := probeRemoteSOCKS5(conn)
	if err != nil {
		t.Fatalf("未读取到远程 SOCKS5 的真实方法响应：%v", err)
	}
	if auth != "用户名密码" {
		t.Fatalf("应识别用户名密码认证，实际：%s", auth)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
