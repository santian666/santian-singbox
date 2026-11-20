package main

import (
	"encoding/json"
	"fmt"
	"net"
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
