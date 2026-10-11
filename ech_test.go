package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEchParsingAndSanitization(t *testing.T) {
	// 1. ech=false / ech=0 should NOT enable ech
	vmessFalse := map[string]interface{}{
		"v":    "2",
		"ps":   "vmess-ech-false",
		"add":  "example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"net":  "ws",
		"tls":  "tls",
		"ech":  false,
	}
	rawJSON, _ := json.Marshal(vmessFalse)
	raw := "vmess://" + base64.StdEncoding.EncodeToString(rawJSON)
	nodes, errs := ParseNodes(raw)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Parse VMess failed: %v", errs)
	}
	tls, _ := nodes[0].Outbound["tls"].(map[string]interface{})
	if _, hasEch := tls["ech"]; hasEch {
		t.Fatalf("ech:false should not create tls.ech: %v", tls["ech"])
	}

	// 2. VLESS with ech=0
	vless0 := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&ech=0#test-vless"
	nodes, errs = ParseNodes(vless0)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Parse VLESS failed: %v", errs)
	}
	tls, _ = nodes[0].Outbound["tls"].(map[string]interface{})
	if _, hasEch := tls["ech"]; hasEch {
		t.Fatalf("ech=0 should not create tls.ech: %v", tls["ech"])
	}

	// 3. VLESS with ech=1 (no config, DNS fallback)
	vless1 := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&ech=1#test-vless"
	nodes, errs = ParseNodes(vless1)
	if len(errs) > 0 || len(nodes) != 1 {
		t.Fatalf("Parse VLESS failed: %v", errs)
	}
	tls, _ = nodes[0].Outbound["tls"].(map[string]interface{})
	ech, ok := tls["ech"].(map[string]interface{})
	if !ok || ech["enabled"] != true {
		t.Fatalf("ech=1 should enable ech: %v", tls)
	}
	if ech["config"] != nil {
		t.Fatalf("ech=1 should not put config: %v", ech["config"])
	}

	// Verify sing-box check passes with empty config
	sanitized := sanitizeOutboundForSingBox(nodes[0].Outbound)
	state := PersistState{
		SelectedID: nodes[0].ID,
		Settings:   defaultSettings(),
		Nodes:      []Node{nodes[0]},
	}
	cfg, err := BuildSingBoxConfig(state, nodes[0], "")
	if err != nil {
		t.Fatalf("Build config failed: %v", err)
	}
	tmpFile := filepath.Join(t.TempDir(), "test-singbox-ech1.json")
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(tmpFile, cfgBytes, 0644)
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", tmpFile)
	if err != nil {
		t.Fatalf("sing-box check failed on ech=1: %s (%v)", out, err)
	}

	// 4. Invalid config (e.g. "ech-a\nech-b") is cleaned by sanitizeOutboundForSingBox
	badNode := Node{
		ID:   "bad-node",
		Name: "Bad ECH",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"server":      "example.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"ech": map[string]interface{}{
					"enabled": true,
					"config":  []string{"ech-a", "ech-b"},
				},
			},
		},
	}
	sanitizedBad := sanitizeOutboundForSingBox(badNode.Outbound)
	tlsBad, _ := sanitizedBad["tls"].(map[string]interface{})
	echBad, _ := tlsBad["ech"].(map[string]interface{})
	if echBad["config"] != nil {
		t.Fatalf("invalid ech config should be deleted by sanitizeOutboundForSingBox: %v", echBad["config"])
	}
	_ = sanitized
}

func TestEchRawBase64NormalizedToPEM(t *testing.T) {
	// Raw Base64 from sing-box ech-keypair
	rawB64 := "AEb+DQBCAAAgACAXoezbwEZIV/zjXS8gzS7ZqF+WxgcotLSdl3p85XuMcQAMAAEAAQABAAIAAQADAAtleGFtcGxlLmNvbQAA"
	node := Node{
		ID:   "b64-node",
		Name: "Base64 ECH",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"server":      "example.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"ech": map[string]interface{}{
					"enabled": true,
					"config":  []string{rawB64},
				},
			},
		},
	}
	sanitized := sanitizeOutboundForSingBox(node.Outbound)
	tls, _ := sanitized["tls"].(map[string]interface{})
	ech, _ := tls["ech"].(map[string]interface{})
	config, ok := ech["config"].([]string)
	if !ok || len(config) != 3 {
		t.Fatalf("raw base64 should be converted to 3-line PEM: %v", ech["config"])
	}
	if config[0] != "-----BEGIN ECH CONFIGS-----" || config[2] != "-----END ECH CONFIGS-----" {
		t.Fatalf("unexpected PEM boundaries: %v", config)
	}

	state := PersistState{
		SelectedID: node.ID,
		Settings:   defaultSettings(),
		Nodes:      []Node{node},
	}
	cfg, err := BuildSingBoxConfig(state, node, "")
	if err != nil {
		t.Fatalf("Build config failed: %v", err)
	}
	tmpFile := filepath.Join(t.TempDir(), "test-singbox-b64.json")
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(tmpFile, cfgBytes, 0644)
	out, err := runCommandOutput(filepath.Join("bin", exeName("sing-box")), "check", "-c", tmpFile)
	if err != nil {
		t.Fatalf("sing-box check failed on raw base64 converted PEM: %s (%v)", out, err)
	}
}

func TestAutoRepairOutboundConfig(t *testing.T) {
	goodNode := Node{
		ID:       "good-node",
		Name:     "Good Node",
		Outbound: map[string]interface{}{"type": "direct"},
	}
	badNode := Node{
		ID:   "bad-node",
		Name: "Broken Outbound",
		Outbound: map[string]interface{}{
			"type": "vless",
			"server": "1.1.1.1",
			"server_port": 443,
			"uuid": "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled": true,
				"server_name": "example.com",
				"ech": map[string]interface{}{
					"enabled": true,
					"config": []string{"-----BEGIN CERTIFICATE-----", "not-ech", "-----END CERTIFICATE-----"},
				},
			},
		},
	}
	settings := defaultSettings()
	settings.RuleSource = ruleSourceBuiltin
	state := PersistState{
		SelectedID: goodNode.ID,
		Settings:   settings,
		Nodes:      []Node{goodNode, badNode},
	}
	// Build a raw config where badNode still has its invalid ECH PEM to test tryAutoRepairOutboundConfig
	cfg, err := BuildSingBoxConfig(state, goodNode, "")
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}
	// Intentionally corrupt outbound 4 (badNode) to verify auto repair
	outbounds := cfg["outbounds"].([]map[string]interface{})
	outbounds[4]["tls"].(map[string]interface{})["ech"] = map[string]interface{}{
		"enabled": true,
		"config": []string{"-----BEGIN CERTIFICATE-----", "not-ech", "-----END CERTIFICATE-----"},
	}
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "sing-box.json")
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(configFile, cfgBytes, 0644)

	absCorePath, _ := filepath.Abs(filepath.Join("bin", exeName("sing-box")))
	mgr := &SingBoxManager{
		corePath: absCorePath,
		rootDir:  tmpDir,
		config:   configFile,
	}

	checkErr := mgr.checkConfig()
	if checkErr == nil {
		t.Fatal("expected checkConfig to fail on corrupted outbound")
	}

	repaired, repairErr := mgr.tryAutoRepairOutboundConfig(checkErr, goodNode)
	if repairErr != nil || !repaired {
		t.Fatalf("tryAutoRepairOutboundConfig failed: repaired=%v err=%v", repaired, repairErr)
	}

	// Verify repaired config passes checkConfig
	if err := mgr.checkConfig(); err != nil {
		t.Fatalf("checkConfig failed after auto-repair: %v", err)
	}
}
