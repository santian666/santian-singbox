package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReserveDelayTestPortsUnique(t *testing.T) {
	ports, release, err := reserveDelayTestPorts(32)
	if err != nil {
		t.Fatalf("reserve ports: %v", err)
	}
	defer release()
	if len(ports) != 32 {
		t.Fatalf("ports count = %d", len(ports))
	}
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port <= 0 {
			t.Fatalf("invalid port: %d", port)
		}
		if _, exists := seen[port]; exists {
			t.Fatalf("duplicate port: %d", port)
		}
		seen[port] = struct{}{}
	}
}

func TestBuildBatchDelayConfigRoutesEachNode(t *testing.T) {
	nodes := []Node{
		{ID: "node-a", Outbound: map[string]interface{}{"type": "direct"}},
		{ID: "node-b", Outbound: map[string]interface{}{"type": "direct"}},
	}
	config := buildBatchDelayConfig(nodes, []int{31001, 31002})
	inbounds := config["inbounds"].([]map[string]interface{})
	if len(inbounds) != 2 {
		t.Fatalf("inbounds count = %d", len(inbounds))
	}
	outbounds := config["outbounds"].([]map[string]interface{})
	if len(outbounds) != 3 {
		t.Fatalf("outbounds count = %d", len(outbounds))
	}
	route := config["route"].(map[string]interface{})
	rules := route["rules"].([]map[string]interface{})
	if len(rules) != 2 {
		t.Fatalf("rules count = %d", len(rules))
	}
	for index, rule := range rules {
		wantInbound := "delay-in-" + strconv.Itoa(index)
		wantOutbound := "delay-out-" + strconv.Itoa(index)
		inbound := rule["inbound"].([]string)
		if len(inbound) != 1 || inbound[0] != wantInbound {
			t.Fatalf("rule %d inbound = %v", index, inbound)
		}
		if rule["outbound"] != wantOutbound {
			t.Fatalf("rule %d outbound = %v", index, rule["outbound"])
		}
	}
}

func TestTemporaryCoreBatchDelayRunsConcurrently(t *testing.T) {
	corePath, err := filepath.Abs(filepath.Join("bin", exeName("sing-box")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("sing-box test core is unavailable")
	}

	var active int32
	var maximum int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maximum)
			if current <= previous || atomic.CompareAndSwapInt32(&maximum, previous, current) {
				break
			}
		}
		time.Sleep(120 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	originalURLs := delayTestURLs
	delayTestURLs = []string{server.URL}
	defer func() { delayTestURLs = originalURLs }()

	nodes := make([]Node, 32)
	for index := range nodes {
		nodes[index] = Node{
			ID:       "node-" + strconv.Itoa(index),
			Name:     "node " + strconv.Itoa(index),
			Outbound: map[string]interface{}{"type": "direct"},
		}
	}
	manager := &SingBoxManager{rootDir: t.TempDir(), corePath: corePath}
	results, err := manager.testNodesDelayWithTemporaryCore(nodes, SpeedTestOptions{DirectOnly: true, FastDelay: true})
	if err != nil {
		t.Fatalf("batch delay test: %v", err)
	}
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil || item.result.DelayMS <= 0 {
			t.Fatalf("node %s result = %+v", node.ID, item)
		}
	}
	if atomic.LoadInt32(&maximum) < 2 {
		t.Fatalf("requests did not run concurrently, max active = %d", maximum)
	}
	if maximum > int32(batchNodeTestConcurrency(len(nodes))) {
		t.Fatalf("requests exceeded concurrency limit, max active = %d", maximum)
	}
}

func TestTemporaryCoreDelaySplitsLargeInput(t *testing.T) {
	corePath, err := filepath.Abs(filepath.Join("bin", exeName("sing-box")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("sing-box test core is unavailable")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	originalURLs := delayTestURLs
	delayTestURLs = []string{server.URL}
	defer func() { delayTestURLs = originalURLs }()

	nodes := make([]Node, temporaryCoreDelayTestBatchSize+1)
	for index := range nodes {
		nodes[index] = Node{
			ID:       "large-node-" + strconv.Itoa(index),
			Name:     "large node " + strconv.Itoa(index),
			Outbound: map[string]interface{}{"type": "direct"},
		}
	}
	manager := &SingBoxManager{rootDir: t.TempDir(), corePath: corePath}
	results, err := manager.testNodesDelayWithTemporaryCore(nodes, SpeedTestOptions{DirectOnly: true, FastDelay: true})
	if err != nil {
		t.Fatalf("large batch delay test: %v", err)
	}
	if len(results) != len(nodes) {
		t.Fatalf("results count = %d, want %d", len(results), len(nodes))
	}
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil || item.result.DelayMS <= 0 {
			t.Fatalf("node %s result = %+v", node.ID, item)
		}
	}
}

func TestBuildBatchDelayConfigWithUpstreamDetour(t *testing.T) {
	nodes := []Node{
		{ID: "node-1", Outbound: map[string]interface{}{"type": "shadowsocks", "tag": "n1"}},
		{ID: "node-2", Outbound: map[string]interface{}{"type": "vmess", "tag": "n2"}},
	}
	cfg := buildBatchDelayConfigWithUpstream(nodes, []int{32001, 32002}, 20808)
	outbounds, ok := cfg["outbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("outbounds not found in config")
	}
	hasUpstream := false
	for _, ob := range outbounds {
		if ob["tag"] == "upstream" && ob["type"] == "socks" && ob["server_port"] == 20808 {
			hasUpstream = true
		}
		if ob["tag"] == "delay-out-0" || ob["tag"] == "delay-out-1" {
			if ob["detour"] != "upstream" {
				t.Fatalf("expected node detour to upstream, got %v", ob["detour"])
			}
		}
	}
	if !hasUpstream {
		t.Fatal("expected upstream socks outbound in outbounds")
	}
}

func TestBuildDelayConfigWithUpstreamDetour(t *testing.T) {
	node := Node{ID: "node-x", Outbound: map[string]interface{}{"type": "trojan", "server": "1.2.3.4"}}
	cfg := buildDelayConfigWithUpstream(node, 32005, 20808)
	outbounds, ok := cfg["outbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("outbounds not found in config")
	}
	hasUpstream := false
	for _, ob := range outbounds {
		if ob["tag"] == "upstream" {
			hasUpstream = true
		}
		if ob["tag"] == "selected" {
			if ob["detour"] != "upstream" {
				t.Fatalf("expected node detour to upstream, got %v", ob["detour"])
			}
		}
	}
	if !hasUpstream {
		t.Fatal("expected upstream socks outbound in single node delay config")
	}
}

func TestLiveClashAPIDelay(t *testing.T) {
	if !tcpPortOpen("127.0.0.1", clashAPIPort, 300*time.Millisecond) {
		t.Skip("sing-box clash api is not running on 20990")
	}
	manager := &SingBoxManager{}
	manager.cmd = &exec.Cmd{Process: &os.Process{Pid: 1}}

	delay, err := manager.testNodeDelayViaClashAPI("node-e7f9fd9e47d2", "", 3*time.Second)
	if err != nil {
		t.Logf("Clash API test node returned: %v (node may be offline)", err)
	} else {
		t.Logf("Clash API test node delay = %d ms", delay)
		if delay <= 0 {
			t.Fatalf("expected delay > 0, got %d", delay)
		}
	}
}

func TestBuildBatchDelayConfigWithDirectNodeDetourExclusion(t *testing.T) {
	nodes := []Node{
		{ID: "node-1", Outbound: map[string]interface{}{"type": "shadowsocks", "tag": "n1"}},
		{ID: "node-2", Outbound: map[string]interface{}{"type": "direct", "tag": "n2"}},
		{ID: "node-3", Outbound: map[string]interface{}{"type": "block", "tag": "n3"}},
	}
	cfg := buildBatchDelayConfigWithUpstream(nodes, []int{32001, 32002, 32003}, 20808)
	outbounds, ok := cfg["outbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("outbounds not found in config")
	}
	for _, ob := range outbounds {
		tag := ob["tag"]
		if tag == "delay-out-0" {
			if ob["detour"] != "upstream" {
				t.Fatalf("expected shadowsocks node detour to upstream, got %v", ob["detour"])
			}
		}
		if tag == "delay-out-1" || tag == "delay-out-2" {
			if ob["detour"] != nil {
				t.Fatalf("expected direct/block node to have no detour, got %v for %s", ob["detour"], tag)
			}
		}
	}
}

func TestBuildBatchDelayConfigOnlyDirectNodesNoUpstream(t *testing.T) {
	nodes := []Node{
		{ID: "node-1", Outbound: map[string]interface{}{"type": "direct", "tag": "n1"}},
		{ID: "node-2", Outbound: map[string]interface{}{"type": "block", "tag": "n2"}},
	}
	cfg := buildBatchDelayConfigWithUpstream(nodes, []int{32001, 32002}, 20808)
	outbounds, ok := cfg["outbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("outbounds not found in config")
	}
	for _, ob := range outbounds {
		if ob["tag"] == "upstream" {
			t.Fatal("expected no upstream socks outbound when all nodes are direct/block")
		}
	}
}

func TestNodesMetricSkipsClashAPIWhenUseSystemProxy(t *testing.T) {
	node := Node{
		ID:       "node-clash-test",
		Name:     "Test Node",
		Type:     "shadowsocks",
		Address:  "127.0.0.1",
		Port:     1080,
		Outbound: map[string]interface{}{"type": "shadowsocks", "tag": "node-clash-test"},
	}
	manager := &SingBoxManager{
		runtimeOutboundTags: map[string]struct{}{
			"node-clash-test": {},
		},
		cmd: &exec.Cmd{Process: &os.Process{Pid: 999999}},
	}
	manager.SetSOCKS5TestProxyPort(20808)

	// When UseSystemProxy is true, Clash API should be skipped.
	// Since port 20808 is not listening, native/upstream test will immediately report dial error to 127.0.0.1:20808,
	// rather than trying to hit Clash API (20990).
	results := manager.TestNodesMetric([]Node{node}, "delay", SpeedTestOptions{UseSystemProxy: true})
	res, ok := results[node.ID]
	if !ok {
		t.Fatalf("expected result for node, got none")
	}
	if res.err == nil {
		t.Fatalf("expected upstream connection error when proxy port 20808 is closed, got nil")
	}
	if strings.Contains(res.err.Error(), "Clash API") {
		t.Fatalf("expected Clash API to be skipped, got error mentioning Clash API: %v", res.err)
	}
}

func TestBatchDelayPartialFailureNoRetry(t *testing.T) {
	corePath, err := filepath.Abs(filepath.Join("bin", exeName("sing-box")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("sing-box test core is unavailable")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	originalURLs := delayTestURLs
	delayTestURLs = []string{server.URL}
	defer func() { delayTestURLs = originalURLs }()

	// 3 good nodes (direct outbound), 2 dead nodes (pointing to an unreachable blackhole address)
	nodes := []Node{
		{ID: "node-ok-1", Name: "ok 1", Outbound: map[string]interface{}{"type": "direct"}},
		{ID: "node-ok-2", Name: "ok 2", Outbound: map[string]interface{}{"type": "direct"}},
		{ID: "node-ok-3", Name: "ok 3", Outbound: map[string]interface{}{"type": "direct"}},
		{ID: "node-dead-1", Name: "dead 1", Outbound: map[string]interface{}{"type": "socks", "server": "192.0.2.1", "server_port": 59999}},
		{ID: "node-dead-2", Name: "dead 2", Outbound: map[string]interface{}{"type": "socks", "server": "192.0.2.2", "server_port": 59999}},
	}

	manager := &SingBoxManager{rootDir: t.TempDir(), corePath: corePath}
	start := time.Now()
	results := manager.TestNodesMetric(nodes, "delay", SpeedTestOptions{DirectOnly: true, FastDelay: true})
	elapsed := time.Since(start)

	okCount := 0
	deadCount := 0
	for _, n := range nodes {
		item := results[n.ID]
		if item.err == nil && item.result.DelayMS > 0 {
			okCount++
		} else {
			deadCount++
		}
	}
	if okCount != 3 || deadCount != 2 {
		t.Fatalf("expected 3 ok and 2 dead, got %d ok and %d dead", okCount, deadCount)
	}

	// Because partial failure does not trigger a second temporary core retry,
	// elapsed time should be approximately fastDelayTestTimeout (~2.5s) plus startup overhead,
	// strictly less than 5.5s (whereas a retry would take > 6s).
	t.Logf("Batch test with 3 ok and 2 dead finished in %v", elapsed)
	if elapsed > 5500*time.Millisecond {
		t.Fatalf("batch test took %v, indicating redundant retry occurred", elapsed)
	}
}



