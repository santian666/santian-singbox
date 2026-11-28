package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrayIconBadgeVariantsAreValidICO(t *testing.T) {
	idle := trayIconForBadge(trayIconKindIdle)
	if len(idle) == 0 {
		t.Fatal("idle icon should not be empty")
	}
	for _, kind := range []string{trayIconKindRunning, trayIconKindDegraded, trayIconKindFault} {
		data := trayIconForBadge(kind)
		if len(data) < 22 {
			t.Fatalf("tray icon variant %s too short: %d", kind, len(data))
		}
		if data[0] != 0 || data[1] != 0 || data[2] != 1 || data[3] != 0 {
			t.Fatalf("tray icon variant %s invalid ICO header: %v", kind, data[:6])
		}
		if string(data) == string(idle) {
			t.Fatalf("tray icon variant %s was not composited differently from idle", kind)
		}
	}
}

func TestPreemptBackgroundChecksCancelsActiveProbeContext(t *testing.T) {
	app := &App{}
	ctx, cleanup := app.registerProbeContext()
	defer cleanup()

	select {
	case <-ctx.Done():
		t.Fatal("probe context should not be canceled initially")
	default:
	}

	started := time.Now()
	app.preemptBackgroundChecks()
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("preemptBackgroundChecks did not cancel active probe context promptly")
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("preemption took too long: %s", elapsed)
	}
}

func TestStressConcurrentStatePreemptionAndSelectNode(t *testing.T) {
	root := t.TempDir()
	logger := NewLogger(filepath.Join(root, "logs", "stress.log"))
	defer logger.Close()

	store, err := NewStore(filepath.Join(root, "data", "gui", "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := make([]Node, 0, 10)
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("node-%d", i)
		nodes = append(nodes, Node{
			ID:      id,
			Name:    fmt.Sprintf("测试节点-%02d", i),
			Type:    "vless",
			Address: "203.0.113.10",
			Port:    443 + i,
			GroupID: defaultGroupID,
			Outbound: map[string]interface{}{
				"type":        "vless",
				"server":      "203.0.113.10",
				"server_port": 443 + i,
				"uuid":        "11111111-1111-1111-1111-111111111111",
			},
		})
	}
	if err := store.ReplaceNodes(nodes, nodes[0].ID); err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot().Settings
	settings.SystemProxy = false
	settings.Tun = false
	settings.ProxyMap = false
	settings.AppProxy = false
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	app := &App{
		root:    root,
		store:   store,
		manager: NewSingBoxManager(root, filepath.Join(root, "sing-box.exe"), filepath.Join(root, "sing-box.json"), filepath.Join(root, "cache.db"), logger),
		logger:  logger,
	}

	const workers = 50
	const opsPerWorker = 15 // 750 total ops (>= 500)
	var stateCalls int64
	var totalStateNS int64
	var errs int64

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				switch (workerID + i) % 4 {
				case 0:
					start := time.Now()
					st := app.State()
					atomic.AddInt64(&totalStateNS, time.Since(start).Nanoseconds())
					atomic.AddInt64(&stateCalls, 1)
					if len(st.Nodes) != 10 {
						atomic.AddInt64(&errs, 1)
					}
				case 1:
					nodeID := nodes[(workerID+i)%len(nodes)].ID
					if selErr := app.SelectNode(nodeID); selErr != nil {
						atomic.AddInt64(&errs, 1)
					}
				case 2:
					ctx, cleanup := app.registerProbeContext()
					app.preemptBackgroundChecks()
					<-ctx.Done()
					cleanup()
				case 3:
					switch (workerID + i) % 3 {
					case 0:
						app.setNetworkHealth("degraded", "延迟偏高 (1450ms)", "测试降级", false)
					case 1:
						app.setNetworkHealth("node_down", "节点不可用", "测试故障", true)
					default:
						app.clearNetworkHealth()
					}
				}
			}
		}()
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(15 * time.Second):
		t.Fatal("50-goroutine stress test timed out (possible deadlock)")
	}

	if errs > 0 {
		t.Fatalf("stress test observed %d unexpected errors", errs)
	}
	calls := atomic.LoadInt64(&stateCalls)
	if calls == 0 {
		t.Fatal("no State() calls recorded")
	}
	avgState := time.Duration(atomic.LoadInt64(&totalStateNS) / calls)
	t.Logf("concurrent State() avg latency across %d calls = %s", calls, avgState)
	if avgState >= 2*time.Millisecond {
		t.Fatalf("average State() latency %s exceeded 2ms budget", avgState)
	}
}

func TestConfigMatrixValidationWithRealSingBoxCore(t *testing.T) {
	corePath := filepath.Join(".", "bin", exeName("sing-box"))
	if !fileExists(corePath) {
		t.Skipf("sing-box binary not found at %s", corePath)
	}

	modes := []string{"rule", "global"}
	entrances := []struct {
		name string
		sys  bool
		tun  bool
		app  bool
	}{
		{name: "SystemProxy", sys: true},
		{name: "TUN", tun: true},
		{name: "AppProxy", app: true},
	}

	tmpDir := t.TempDir()
	for _, mode := range modes {
		for _, ent := range entrances {
			for _, proxyMapDetour := range []bool{false, true} {
				caseName := fmt.Sprintf("%s_%s_Detour%v", mode, ent.name, proxyMapDetour)
				t.Run(caseName, func(t *testing.T) {
					state := testConfigState(mode)
					state.Settings.RuleSource = ruleSourceBuiltin
					state.Settings.SystemProxy = ent.sys
					state.Settings.Tun = ent.tun
					state.Settings.AppProxy = ent.app
					if ent.app {
						state.Settings.AppProxyTargets = []AppProxyTarget{
							{Type: "app", Path: `C:\Program Files\AdsPower\AdsPower.exe`, Enabled: true},
						}
					}
					state.Settings.ProxyMap = true
					state.Settings.ProxyMapDetour = proxyMapDetour
					state.Settings.ProxyMapStartPort = 52001
					state.Settings.ProxyMapGroupIDs = []string{"g1"}
					state.Settings.ProxyMapGroupID = "g1"
					state.Groups = []NodeGroup{
						{ID: defaultGroupID, Name: "默认", Source: "manual"},
						{ID: "g1", Name: "指纹组", Source: "manual"},
					}
					selected := Node{
						ID:      "sel-vless",
						Name:    "中转节点-HK",
						Type:    "vless",
						Address: "hk.example.com",
						Port:    443,
						GroupID: defaultGroupID,
						Outbound: map[string]interface{}{
							"type":        "vless",
							"server":      "hk.example.com",
							"server_port": 443,
							"uuid":        "11111111-1111-1111-1111-111111111111",
							"tls": map[string]interface{}{
								"enabled":     true,
								"server_name": "hk.example.com",
							},
						},
					}
					resSocks := Node{
						ID:      "res-socks",
						Name:    "住宅Socks-US",
						Type:    "socks",
						Address: "198.51.100.10",
						Port:    1080,
						GroupID: "g1",
						Outbound: map[string]interface{}{
							"type":        "socks",
							"server":      "198.51.100.10",
							"server_port": 1080,
						},
					}
					resHTTP := Node{
						ID:      "res-http",
						Name:    "住宅HTTP-US",
						Type:    "http",
						Address: "198.51.100.11",
						Port:    8080,
						GroupID: "g1",
						Outbound: map[string]interface{}{
							"type":        "http",
							"server":      "198.51.100.11",
							"server_port": 8080,
						},
					}
					state.Nodes = []Node{selected, resSocks, resHTTP}
					state.SelectedID = selected.ID

					cfg, err := buildSingBoxConfig(state, selected, filepath.Join(tmpDir, caseName+".db"), false, defaultAutoDNSPlan())
					if err != nil {
						t.Fatalf("buildSingBoxConfig failed: %v", err)
					}
					if err := validateGeneratedConfig(cfg, state.Settings); err != nil {
						t.Fatalf("validateGeneratedConfig failed: %v", err)
					}
					data, err := json.MarshalIndent(cfg, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					cfgPath := filepath.Join(tmpDir, caseName+".json")
					if err := os.WriteFile(cfgPath, data, 0644); err != nil {
						t.Fatal(err)
					}
					cmd := exec.Command(corePath, "check", "-c", cfgPath)
					if out, checkErr := cmd.CombinedOutput(); checkErr != nil {
						t.Fatalf("sing-box check failed (%v): %s", checkErr, string(out))
					}
				})
			}
		}
	}

	t.Run("RuleWithCompiledSRS_TUN_ProxyMapDetour", func(t *testing.T) {
		srsRoot := t.TempDir()
		cachePath := filepath.Join(srsRoot, "cache.db")
		ruleDir := ruleSetDirFromCachePath(cachePath)
		if err := os.MkdirAll(ruleDir, 0755); err != nil {
			t.Fatal(err)
		}
		sampleRules := map[string]string{
			"loyalsoldier-proxy.json":  `{"version":3,"rules":[{"domain_suffix":["google.com"]}]}`,
			"loyalsoldier-direct.json": `{"version":3,"rules":[{"domain_suffix":["qq.com"]}]}`,
			"loyalsoldier-china.json":  `{"version":3,"rules":[{"domain_suffix":["cn"]}]}`,
			"geoip-cn.json":            `{"version":3,"rules":[{"ip_cidr":["114.114.114.114/32"]}]}`,
		}
		for name, body := range sampleRules {
			if err := os.WriteFile(filepath.Join(ruleDir, name), []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}
		state := testConfigState("rule")
		state.Settings.RuleSource = ruleSourceLoyalsoldier
		state.Settings.Tun = true
		state.Settings.ProxyMap = true
		state.Settings.ProxyMapDetour = true
		state.Settings.ProxyMapStartPort = 52001
		selected := Node{
			ID:      "sel-vless",
			Name:    "中转节点-HK",
			Type:    "vless",
			Address: "hk.example.com",
			Port:    443,
			GroupID: defaultGroupID,
			Outbound: map[string]interface{}{
				"type":        "vless",
				"server":      "hk.example.com",
				"server_port": 443,
				"uuid":        "11111111-1111-1111-1111-111111111111",
				"tls": map[string]interface{}{
					"enabled":     true,
					"server_name": "hk.example.com",
				},
			},
		}
		resSocks := Node{
			ID:      "res-socks",
			Name:    "住宅Socks-US",
			Type:    "socks",
			Address: "198.51.100.10",
			Port:    1080,
			GroupID: defaultGroupID,
			Outbound: map[string]interface{}{
				"type":        "socks",
				"server":      "198.51.100.10",
				"server_port": 1080,
			},
		}
		state.Nodes = []Node{selected, resSocks}
		state.SelectedID = selected.ID

		mgr := NewSingBoxManager(srsRoot, corePath, filepath.Join(srsRoot, "sing-box.json"), cachePath, nil)
		mgr.ensureCompiledRuleSets(state.Settings)

		cfg, err := BuildSingBoxConfig(state, selected, cachePath)
		if err != nil {
			t.Fatalf("BuildSingBoxConfig with .srs failed: %v", err)
		}
		if err := validateGeneratedConfig(cfg, state.Settings); err != nil {
			t.Fatalf("validateGeneratedConfig with .srs failed: %v", err)
		}
		ruleSets := cfg["route"].(map[string]interface{})["rule_set"].([]map[string]interface{})
		for _, rs := range ruleSets {
			if rs["format"] != "binary" {
				t.Fatalf("expected compiled binary .srs rule-set format, got %v", rs)
			}
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		cfgPath := filepath.Join(srsRoot, "srs-full.json")
		if err := os.WriteFile(cfgPath, data, 0644); err != nil {
			t.Fatal(err)
		}
		if out, checkErr := exec.Command(corePath, "check", "-c", cfgPath).CombinedOutput(); checkErr != nil {
			t.Fatalf("sing-box check for .srs full config failed (%v): %s", checkErr, string(out))
		}
	})
}

func TestSingBoxCtrlBreakGracefulStopRealBinary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CTRL_BREAK_EVENT test")
	}
	corePath, err := filepath.Abs(filepath.Join(".", "bin", exeName("sing-box")))
	if err != nil || !fileExists(corePath) {
		t.Skip("sing-box.exe not found")
	}

	tmpDir := t.TempDir()
	for cycle := 1; cycle <= 3; cycle++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()

		cfg := map[string]interface{}{
			"log": map[string]interface{}{"level": "warn"},
			"inbounds": []map[string]interface{}{
				{
					"type":        "mixed",
					"tag":         "mixed-in",
					"listen":      "127.0.0.1",
					"listen_port": port,
				},
			},
			"outbounds": []map[string]interface{}{
				{"type": "direct", "tag": "direct"},
			},
		}
		cfgBytes, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		cfgPath := filepath.Join(tmpDir, fmt.Sprintf("cycle-%d.json", cycle))
		if err := os.WriteFile(cfgPath, cfgBytes, 0644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(corePath, "run", "-c", cfgPath)
		configureSingBoxCmd(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatalf("cycle %d failed to start sing-box: %v", cycle, err)
		}

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		// Wait briefly until mixed-in port is actually listening so sing-box has finished init
		deadline := time.Now().Add(3 * time.Second)
		listening := false
		for time.Now().Before(deadline) {
			if !isTCPPortFree("127.0.0.1", port) {
				listening = true
				break
			}
			time.Sleep(40 * time.Millisecond)
		}
		if !listening {
			_ = cmd.Process.Kill()
			<-done
			t.Fatalf("cycle %d sing-box did not listen on %d", cycle, port)
		}

		stopStart := time.Now()
		if err := gracefulStopSingBox(cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
			<-done
			t.Fatalf("cycle %d gracefulStopSingBox error: %v", cycle, err)
		}
		select {
		case <-done:
			elapsed := time.Since(stopStart)
			t.Logf("cycle %d stopped gracefully in %s", cycle, elapsed)
			if elapsed > 2*time.Second {
				t.Fatalf("cycle %d graceful stop exceeded 2s (%s)", cycle, elapsed)
			}
		case <-time.After(2500 * time.Millisecond):
			_ = cmd.Process.Kill()
			<-done
			t.Fatalf("cycle %d sing-box did not exit within 2s after CTRL_BREAK_EVENT", cycle)
		}
	}
}

func TestBatchTestBothMetricEmitsPingAndDelay(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) {
		return 88, nil
	})
	deps.ping = func(ctx context.Context, item batchNodeTestItem) (SpeedResult, error) {
		res := batchBaseResult(item.node)
		res.PingMS = 23
		res.DelayMS = 23
		return res, nil
	}
	items := []batchNodeTestItem{
		{index: 0, node: Node{ID: "n0", Name: "node0", Outbound: map[string]interface{}{"type": "vless"}}},
		{index: 1, node: Node{ID: "n1", Name: "node1", Outbound: map[string]interface{}{"type": "trojan"}}},
	}
	runner := newBatchTestJobRunnerWithDeps("job-both", len(items), items, "both", true, 0, deps)
	runner.Run()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.done) != 1 {
		t.Fatalf("expected 1 done event, got %d", len(rec.done))
	}
	if rec.done[0].Total != 2 || rec.done[0].OK != 2 || rec.done[0].Failed != 0 {
		t.Fatalf("unexpected done counts: %+v", rec.done[0])
	}
	phases := map[string]int{}
	for _, ev := range rec.results {
		phases[ev.Phase]++
	}
	if phases["ping"] != 2 || phases["delay"] != 2 || phases["ip"] != 2 {
		t.Fatalf("expected 2 ping, 2 delay, 2 ip events, got %+v", phases)
	}
}

func TestMultiStreamSpeedtestExcludesHandshakeLatency(t *testing.T) {
	chunk := make([]byte, 512*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate 250ms proxy/TLS/TTFB handshake latency before first payload byte.
		time.Sleep(250 * time.Millisecond)
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			for i := 0; i < 8; i++ {
				_, _ = w.Write(chunk)
			}
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 8 * time.Second}
	dlMbps, err := measureMultiStreamDownload(client, srv.URL, 1500*time.Millisecond, 100*1000*1000, nil)
	if err != nil {
		t.Fatalf("measureMultiStreamDownload failed: %v", err)
	}
	if dlMbps <= 0 {
		t.Fatalf("expected positive download Mbps, got %v", dlMbps)
	}
	ulMbps, err := measureMultiStreamUpload(client, srv.URL, 1500*time.Millisecond, 100*1000*1000, nil)
	if err != nil {
		t.Fatalf("measureMultiStreamUpload failed: %v", err)
	}
	if ulMbps <= 0 {
		t.Fatalf("expected positive upload Mbps, got %v", ulMbps)
	}
	t.Logf("multi-stream localhost speedtest (with 250ms TTFB): download=%.2f Mbps, upload=%.2f Mbps", dlMbps, ulMbps)
}

func TestEnsureCacheDBSelectedTagAndSyncSelectedDefault(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.db")
	configPath := filepath.Join(dir, "sing-box.json")
	if err := os.WriteFile(cachePath, []byte("stale-bbolt-cache"), 0644); err != nil {
		t.Fatal(err)
	}
	m := &SingBoxManager{cachePath: cachePath, config: configPath}

	// First run / upgrade without marker should remove stale cache.db and write marker.
	m.ensureCacheDBSelectedTag("node-sg-1")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale cache.db to be removed on upgrade/mismatch, got err=%v", err)
	}

	// Recreate cache.db and verify same tag preserves it.
	if err := os.WriteFile(cachePath, []byte("fresh-fakeip-cache"), 0644); err != nil {
		t.Fatal(err)
	}
	m.ensureCacheDBSelectedTag("node-sg-1")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache.db to be preserved when selected tag matches, got err=%v", err)
	}

	// Changing target tag offline must remove cache.db so old selection cannot override default.
	m.ensureCacheDBSelectedTag("node-jp-2")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache.db to be removed when target tag changes, got err=%v", err)
	}

	// SyncSelectedDefault updates proxy selector default in sing-box.json without deleting active cache.db.
	if err := os.WriteFile(cachePath, []byte("running-bbolt-cache"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]interface{}{
		"outbounds": []interface{}{
			map[string]interface{}{"type": "selector", "tag": "proxy", "default": "node-sg-1"},
		},
	}
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	m.SyncSelectedDefault("node-jp-2")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("SyncSelectedDefault must not remove running cache.db, got err=%v", err)
	}
	updatedRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var updatedCfg map[string]interface{}
	if err := json.Unmarshal(updatedRaw, &updatedCfg); err != nil {
		t.Fatal(err)
	}
	obs := updatedCfg["outbounds"].([]interface{})
	if got := obs[0].(map[string]interface{})["default"]; got != "node-jp-2" {
		t.Fatalf("expected proxy default to sync to node-jp-2, got %v", got)
	}
}

func TestCanonicalRegionCodeAndIndependentExitProbeContext(t *testing.T) {
	if got := detectNodeRegion(Node{Name: "SG-85.149.212.77-新加坡"}); got != "SG" {
		t.Fatalf("expected SG, got %q", got)
	}
	if got := canonicalRegionCode("新加坡"); got != "SG" {
		t.Fatalf("expected 新加坡 -> SG, got %q", got)
	}
	if got := canonicalRegionCode("英国"); got != "GB" || detectNodeRegion(Node{Name: "UK-London-01"}) != "GB" {
		t.Fatalf("expected 英国 and UK-London-01 -> GB, got %q / %q", got, detectNodeRegion(Node{Name: "UK-London-01"}))
	}
	if canonicalRegionCode("澳大利亚") != "AU" || canonicalRegionCode("澳大利亚") == detectNodeRegion(Node{Name: "SG-新加坡"}) {
		t.Fatalf("expected AU != SG")
	}

	app := &App{}
	exitCtx, exitCancel := app.registerExitProbeContext()
	defer exitCancel()
	healthCtx, healthCancel := app.registerProbeContext()
	defer healthCancel()

	// Health probe registration must NOT cancel an active exit IP probe.
	if exitCtx.Err() != nil {
		t.Fatalf("registerProbeContext must not cancel exit probe context: %v", exitCtx.Err())
	}
	// Preempting background checks must cancel BOTH health and exit probes.
	app.preemptBackgroundChecks()
	if healthCtx.Err() == nil || exitCtx.Err() == nil {
		t.Fatalf("preemptBackgroundChecks must cancel both probes: health=%v exit=%v", healthCtx.Err(), exitCtx.Err())
	}
}

func TestProxyLayerSixHardeningFixes(t *testing.T) {
	// 1. TUN + non-IPv6 node adds predefined NOERROR AAAA rule & blocks domestic DoH IPs on port 443.
	wgIPv6 := Node{
		ID:      "wg-v6",
		Name:    "WG-IPv6",
		Type:    "wireguard",
		Address: "198.51.100.9:51820",
		Outbound: map[string]interface{}{
			"type":            "wireguard",
			"server":          "198.51.100.9",
			"server_port":     51820,
			"local_address":   []string{"10.0.0.2/32", "fd00::2/128"},
			"private_key":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"peer_public_key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		},
	}
	vlessNode := Node{
		ID:      "vless-1",
		Name:    "SG-VLESS",
		Type:    "vless",
		Address: "edge.example.com:443",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"server":      "edge.example.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "edge.example.com",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "",
				},
			},
		},
	}
	plainSocks := Node{
		ID:      "socks-1",
		Name:    "US-Socks",
		Type:    "socks",
		Address: "198.51.100.10:1080",
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "198.51.100.10",
			"server_port": 1080,
		},
	}
	state := PersistState{
		SelectedID: vlessNode.ID,
		Nodes:      []Node{vlessNode, plainSocks, wgIPv6},
		Settings: Settings{
			Mode:              "rule",
			Tun:               true,
			ProxyMap:          true,
			ProxyMapStartPort: 52001,
			ProxyMapDetour:    true,
			MixedPort:         20808,
			RuleSource:        ruleSourceBuiltin,
		},
	}

	cfg, err := BuildSingBoxConfig(state, vlessNode, "")
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}
	dnsMap := cfg["dns"].(map[string]interface{})
	dnsRules := dnsMap["rules"].([]map[string]interface{})
	foundAAAANoError := false
	for _, r := range dnsRules {
		if r["action"] == "predefined" && r["rcode"] == "NOERROR" && stringSliceContains(configStringValues(r["query_type"]), "AAAA") {
			foundAAAANoError = true
		}
		if r["server"] == "direct-dns" && stringSliceContains(configStringValues(r["domain"]), "edge.example.com") {
			if _, hasSuffix := r["domain_suffix"]; hasSuffix {
				t.Fatalf("node server DNS rule must not use domain_suffix: %+v", r)
			}
		}
	}
	if !foundAAAANoError {
		t.Fatal("expected predefined NOERROR AAAA DNS rule in TUN mode for non-IPv6 node")
	}

	// Domestic DoH IP:443 reject rule in route.rules.
	routeRules := cfg["route"].(map[string]interface{})["rules"].([]map[string]interface{})
	foundDoHIPReject := false
	for _, r := range routeRules {
		if r["action"] == "reject" && stringSliceContains(configStringValues(r["ip_cidr"]), "223.5.5.5/32") {
			foundDoHIPReject = true
		}
	}
	if !foundDoHIPReject {
		t.Fatal("expected TUN route rule rejecting domestic DoH IP 223.5.5.5/32 on port 443")
	}

	// uTLS empty fingerprint filled with "chrome".
	obs := cfg["outbounds"].([]map[string]interface{})
	for _, ob := range obs {
		if ob["tag"] == nodeOutboundTag(vlessNode) {
			utls := ob["tls"].(map[string]interface{})["utls"].(map[string]interface{})
			if utls["fingerprint"] != "chrome" {
				t.Fatalf("expected empty uTLS fingerprint to default to chrome, got %v", utls["fingerprint"])
			}
		}
	}

	// WireGuard local_address IPv6 detection & endpoint fallback.
	if !nodeSupportsIPv6(wgIPv6) {
		t.Fatal("expected WireGuard node with IPv6 local_address to report nodeSupportsIPv6=true")
	}
	epFromAddr := wireGuardEndpointFromOutbound(map[string]interface{}{
		"type":            "wireguard",
		"tag":             "wg-new",
		"address":         []string{"10.0.0.3/32"},
		"private_key":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"peer_public_key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		"server":          "198.51.100.9",
		"server_port":     51820,
	})
	if len(stringListFromAny(epFromAddr["address"])) != 1 {
		t.Fatalf("expected wireGuardEndpointFromOutbound to fall back to address field, got %+v", epFromAddr)
	}

	// Self-loop guard: ProxyMap node matching selected main node must not detour through proxy,
	// and switching to/from a mapped node with ProxyMapDetour enabled must trigger reload.
	cfgPlainSelected, err := BuildSingBoxConfig(state, plainSocks, "")
	if err != nil {
		t.Fatalf("BuildSingBoxConfig with plain selected failed: %v", err)
	}
	for _, ob := range cfgPlainSelected["outbounds"].([]map[string]interface{}) {
		if ob["tag"] == proxyMapOutboundTag(1) && ob["detour"] != nil {
			t.Fatalf("ProxyMap node matching selected main proxy must not self-loop detour: %+v", ob)
		}
	}
	if !needsRuntimeReloadForNodeSwitch(state, vlessNode, plainSocks) {
		t.Fatal("switching main node to a mapped node with ProxyMapDetour must trigger reload to prevent self-loop")
	}
	if !needsRuntimeReloadForNodeSwitch(state, vlessNode, wgIPv6) {
		t.Fatal("switching between non-IPv6 and IPv6 node under TUN must trigger reload")
	}
}

func TestTunBlockRulesUseActionReject(t *testing.T) {
	if got := buildLocalMetadataBlockRouteRule()["action"]; got != "reject" {
		t.Fatalf("expected local metadata rule action=reject, got %v", got)
	}
	for _, rule := range buildTunStabilityRouteRules() {
		if rule["outbound"] == "block" {
			t.Fatalf("TUN stability rule must use action=reject instead of outbound=block: %+v", rule)
		}
	}
}
