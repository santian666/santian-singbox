//go:build windows

package main

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestKillSwitchSanitizesInjectedOriginalPolicyAndNoCmdShell(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	root := t.TempDir()
	stateFile := killSwitchStatePath(root)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0700); err != nil {
		t.Fatal(err)
	}

	maliciousState := `{
  "active": true,
  "originalPolicy": {
    "domain": "BlockInbound,AllowOutbound & calc.exe",
    "private": "AllowInbound,AllowOutbound | whoami",
    "public": "$(evil)"
  }
}`
	if err := os.WriteFile(stateFile, []byte(maliciousState), 0600); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	runFirewallCommand = func(name string, args ...string) (string, error) {
		entry := append([]string{name}, args...)
		calls = append(calls, entry)
		return "", nil
	}

	if err := disableKillSwitch(root, nil); err != nil {
		t.Fatalf("disableKillSwitch returned unexpected error: %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("expected netsh calls during disableKillSwitch")
	}

	restoredPolicies := 0
	for _, call := range calls {
		exeBase := strings.ToLower(filepath.Base(call[0]))
		if exeBase == "cmd.exe" || exeBase == "cmd" {
			t.Fatalf("killswitch must NEVER invoke cmd.exe, got: %v", call)
		}
		if exeBase != "netsh.exe" {
			t.Fatalf("expected direct netsh.exe invocation, got: %v", call)
		}
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "calc.exe") || strings.Contains(joined, "whoami") || strings.Contains(joined, "&") || strings.Contains(joined, "|") {
			t.Fatalf("injected shell payload leaked into netsh args: %v", call)
		}
		if len(call) >= 6 && call[2] == "set" && call[4] == "firewallpolicy" {
			restoredPolicies++
			if !strings.EqualFold(call[5], "blockinbound,allowoutbound") {
				t.Fatalf("tampered policy should fall back to blockinbound,allowoutbound, got %q", call[5])
			}
		}
	}
	if restoredPolicies != 3 {
		t.Fatalf("expected 3 profile firewallpolicy restorations, got %d", restoredPolicies)
	}
}

func TestKillSwitchRejectsEmptyCorePath(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	root := t.TempDir()
	called := 0
	runFirewallCommand = func(name string, args ...string) (string, error) {
		called++
		return "", nil
	}

	if err := enableKillSwitch(root, nil, "", `C:\mock\SanTian-Singbox.exe`); err == nil {
		t.Fatal("expected error when corePath is empty")
	}
	if called != 0 {
		t.Fatalf("expected 0 firewall commands when corePath missing, got %d", called)
	}
}

func TestKillSwitchCleanDisableShortCircuitsZeroSubprocesses(t *testing.T) {
	defer resetFirewallMocks()

	checkAdminPrivileges = func() bool { return true }
	root := t.TempDir()
	calls := 0
	runFirewallCommand = func(name string, args ...string) (string, error) {
		calls++
		// Simulate no leftover firewall rules found
		return "No rules match the specified criteria.", errors.New("not found")
	}

	// First call probes existing rules once and marks verified clean.
	if err := disableKillSwitch(root, nil); err != nil {
		t.Fatalf("first disableKillSwitch failed: %v", err)
	}
	firstCalls := calls
	if firstCalls == 0 {
		t.Fatal("expected initial probe on first disableKillSwitch")
	}

	// Subsequent periodic health-check calls must short-circuit with zero subprocesses.
	for i := 0; i < 10; i++ {
		if err := disableKillSwitch(root, nil); err != nil {
			t.Fatalf("subsequent disableKillSwitch #%d failed: %v", i, err)
		}
	}
	if calls != firstCalls {
		t.Fatalf("expected 0 extra subprocesses after clean cache, got %d extra calls", calls-firstCalls)
	}
	if isKillSwitchActive() {
		t.Fatal("expected killSwitchActive to be false after clean disable")
	}
}

func TestBackupAndStoreSanitizeMaliciousEntityIDs(t *testing.T) {
	xssGroupID := `"><img src=x onerror=alert(1)>`
	traversalNodeID := `..\..\evil\pwn`
	xssNodeID := `"><svg onload=alert(2)>`

	rawBackup := map[string]interface{}{
		"app":     "SanTian-Singbox",
		"version": backupFileVersion,
		"groups": []map[string]interface{}{
			{"id": xssGroupID, "name": "EvilGroup", "source": "manual"},
		},
		"nodes": []map[string]interface{}{
			{
				"id":      traversalNodeID,
				"groupId": xssGroupID,
				"name":    "NodeA",
				"type":    "socks",
				"address": "1.2.3.4",
				"port":    1080,
				"raw":     "socks5://1.2.3.4:1080#NodeA",
				"outbound": map[string]interface{}{
					"type":        "socks",
					"server":      "1.2.3.4",
					"server_port": 1080,
				},
			},
			{
				"id":      xssNodeID,
				"groupId": xssGroupID,
				"name":    "NodeB",
				"type":    "socks",
				"address": "1.2.3.5",
				"port":    1080,
				"raw":     "socks5://1.2.3.5:1080#NodeB",
				"outbound": map[string]interface{}{
					"type":        "socks",
					"server":      "1.2.3.5",
					"server_port": 1080,
				},
			},
		},
		"selectedId": traversalNodeID,
	}
	encoded, err := json.Marshal(rawBackup)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := parseNodeBackup(encoded)
	if err != nil {
		t.Fatalf("parseNodeBackup failed: %v", err)
	}
	if len(parsed.Groups) == 0 || len(parsed.Nodes) != 2 {
		t.Fatalf("unexpected parsed counts: groups=%d nodes=%d", len(parsed.Groups), len(parsed.Nodes))
	}
	for _, g := range parsed.Groups {
		if !isSafeEntityID(g.ID) {
			t.Fatalf("unsanitized group ID survived backup import: %q", g.ID)
		}
	}
	groupIDs := map[string]bool{}
	for _, g := range parsed.Groups {
		groupIDs[g.ID] = true
	}
	nodeIDs := map[string]bool{}
	for _, n := range parsed.Nodes {
		if !isSafeEntityID(n.ID) {
			t.Fatalf("unsanitized node ID survived backup import: %q", n.ID)
		}
		if !groupIDs[n.GroupID] {
			t.Fatalf("node GroupID %q lost group mapping after sanitization", n.GroupID)
		}
		nodeIDs[n.ID] = true
	}
	if !isSafeEntityID(parsed.SelectedID) || !nodeIDs[parsed.SelectedID] {
		t.Fatalf("SelectedID %q invalid or unmapped after sanitization", parsed.SelectedID)
	}

	// Also verify Store.normalizeLocked sanitizes direct state tampering.
	storePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	s := store.Snapshot()
	s.Groups = append(s.Groups, NodeGroup{ID: xssGroupID, Name: "Evil2", Source: "manual"})
	s.Nodes = []Node{
		{
			ID:      traversalNodeID,
			GroupID: xssGroupID,
			Name:    "NodeA",
			Type:    "socks",
			Address: "1.2.3.4",
			Port:    1080,
			Outbound: map[string]interface{}{
				"type":        "socks",
				"server":      "1.2.3.4",
				"server_port": 1080,
			},
		},
	}
	s.Settings.ProxyMapGroupID = xssGroupID
	s.Settings.ProxyMapGroupIDs = []string{xssGroupID}
	if err := store.ReplaceGroupsAndNodes(s.Groups, s.Nodes, traversalNodeID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSettings(s.Settings); err != nil {
		t.Fatal(err)
	}
	snap := store.Snapshot()
	for _, g := range snap.Groups {
		if !isSafeEntityID(g.ID) {
			t.Fatalf("store kept unsafe group ID: %q", g.ID)
		}
	}
	for _, n := range snap.Nodes {
		if !isSafeEntityID(n.ID) || !isSafeEntityID(n.GroupID) {
			t.Fatalf("store kept unsafe node/group ID: %q / %q", n.ID, n.GroupID)
		}
	}
	if !isSafeEntityID(snap.SelectedID) {
		t.Fatalf("store kept unsafe SelectedID: %q", snap.SelectedID)
	}
}

func TestClashAPIConfigHasRandomSecretNoWildcardCORSAndPassesSingBoxCheck(t *testing.T) {
	secret := newRandomClashSecret()
	if len(secret) != 64 {
		t.Fatalf("expected 64-char hex Clash API secret, got len=%d (%q)", len(secret), secret)
	}
	secret2 := newRandomClashSecret()
	if secret == secret2 {
		t.Fatal("expected random Clash API secret to differ across calls")
	}

	node := Node{
		ID:      "node-1",
		Name:    "TestNode",
		Type:    "socks",
		Address: "203.0.113.10",
		Port:    1080,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "203.0.113.10",
			"server_port": 1080,
		},
	}
	settings := defaultSettings()
	settings.Mode = "global"
	settings.RuleSource = ruleSourceBuiltin
	settings.SystemProxy = true
	settings.Tun = false

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.db")
	cfg, err := BuildSingBoxConfig(PersistState{
		SelectedID: node.ID,
		Nodes:      []Node{node},
		Settings:   settings,
	}, node, cachePath)
	if err != nil {
		t.Fatalf("buildSingBoxConfig failed: %v", err)
	}

	exp, ok := cfg["experimental"].(map[string]interface{})
	if !ok {
		t.Fatal("missing experimental section in sing-box config")
	}
	clashAPI, ok := exp["clash_api"].(map[string]interface{})
	if !ok {
		t.Fatal("missing experimental.clash_api section")
	}
	clashAPI["secret"] = secret

	origins, ok := clashAPI["access_control_allow_origin"].([]string)
	if !ok || len(origins) != 0 {
		t.Fatalf("expected empty access_control_allow_origin (no wildcard CORS), got %#v", clashAPI["access_control_allow_origin"])
	}
	if clashAPI["secret"] != secret {
		t.Fatal("secret not set on clash_api")
	}

	// Verify against real sing-box binary if present in bin/sing-box.exe
	realCore := filepath.Join(".", "bin", "sing-box.exe")
	if fileExists(realCore) {
		cfgPath := filepath.Join(tmpDir, "sing-box-clash-auth.json")
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(realCore, "check", "-c", cfgPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("real sing-box check rejected hardened clash_api config: %v\n%s", err, string(out))
		}
	}
}

func TestDPAPIEncryptDecryptRoundTripAndLegacyGSB1Compat(t *testing.T) {
	plain := []byte(`{"selectedId":"node-abc","secret":"trojan://my-super-secret@example.com:443"}`)

	sealed, err := encryptBytes(plain)
	if err != nil {
		t.Fatalf("encryptBytes failed: %v", err)
	}
	if !strings.HasPrefix(string(sealed), dpapiEncryptedPrefix) {
		t.Fatalf("expected Windows encryptBytes to use DPAPI prefix %q, got %q", dpapiEncryptedPrefix, string(sealed[:min(12, len(sealed))]))
	}
	opened, err := decryptBytes(sealed)
	if err != nil {
		t.Fatalf("decryptBytes(GSB2) failed: %v", err)
	}
	if string(opened) != string(plain) {
		t.Fatalf("DPAPI round-trip mismatch: got %q, want %q", string(opened), string(plain))
	}

	// Construct legacy GSB1 payload and ensure backward-compatible decryption still succeeds.
	block, err := aes.NewCipher(localKey())
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	legacyCipher := gcm.Seal(nonce, nonce, plain, nil)
	legacyData := []byte(encryptedPrefix + base64.StdEncoding.EncodeToString(legacyCipher))

	legacyOpened, err := decryptBytes(legacyData)
	if err != nil {
		t.Fatalf("decryptBytes(legacy GSB1) failed: %v", err)
	}
	if string(legacyOpened) != string(plain) {
		t.Fatalf("legacy GSB1 round-trip mismatch: got %q, want %q", string(legacyOpened), string(plain))
	}
}

func TestAppUpdateURLAllowlistAndSymlinkExtractionGuard(t *testing.T) {
	allowed := []string{
		"https://github.com/santian666/santian-singbox/releases/download/v7.0.31/SanTian-Singbox.zip",
		"https://objects.githubusercontent.com/github-production-release-asset/12345",
		"https://release-assets.githubusercontent.com/some-asset",
	}
	for _, u := range allowed {
		if err := validateTrustedUpdateURL(u); err != nil {
			t.Errorf("expected trusted URL %q to be allowed, got error: %v", u, err)
		}
	}

	blocked := []string{
		"http://github.com/santian666/santian-singbox/releases/download/v7.0.31/SanTian-Singbox.zip",
		"https://evil.example.com/SanTian-Singbox.zip",
		"https://github.com.evil.com/SanTian-Singbox.zip",
		"file:///C:/Windows/System32/cmd.exe",
	}
	for _, u := range blocked {
		if err := validateTrustedUpdateURL(u); err == nil {
			t.Errorf("expected untrusted URL %q to be rejected", u)
		}
	}

	// Build a ZIP with a symlink entry and verify extractZipArchive rejects it.
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "symlink.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	hdr := &zip.FileHeader{Name: "SanTian-Singbox/evil-link"}
	hdr.SetMode(os.ModeSymlink | 0777)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`C:\Windows\System32\cmd.exe`))
	_ = zw.Close()
	_ = f.Close()

	if err := extractZipArchive(zipPath, filepath.Join(tmpDir, "out")); err == nil {
		t.Fatal("expected extractZipArchive to reject symlink entry")
	}
}

func TestNetworkTargetValidationBlocksFlagInjectionAndSupportsIPv6(t *testing.T) {
	badTargets := []string{
		"--output=C:\\evil.txt",
		"-4",
		"-c",
		"1.1.1.1;calc.exe",
		"1.1.1.1 & whoami",
		"host with space",
	}
	for _, bad := range badTargets {
		if err := validateMTRTarget(bad); err == nil {
			t.Errorf("validateMTRTarget(%q) should fail", bad)
		}
		if _, _, err := parseIperfTarget(bad); err == nil {
			t.Errorf("parseIperfTarget(%q) should fail", bad)
		}
	}

	if err := validateMTRTarget("1.1.1.1"); err != nil {
		t.Errorf("validateMTRTarget(1.1.1.1) failed: %v", err)
	}
	if err := validateMTRTarget("2001:4860:4860::8888"); err != nil {
		t.Errorf("validateMTRTarget(IPv6) failed: %v", err)
	}
	if err := validateMTRTarget("speed.cloudflare.com"); err != nil {
		t.Errorf("validateMTRTarget(hostname) failed: %v", err)
	}

	host, port, err := parseIperfTarget("[2001:db8::1]:5202")
	if err != nil || host != "2001:db8::1" || port != 5202 {
		t.Fatalf("parseIperfTarget([2001:db8::1]:5202) = (%q, %d, %v)", host, port, err)
	}
	host, port, err = parseIperfTarget("2001:db8::1")
	if err != nil || host != "2001:db8::1" || port != 0 {
		t.Fatalf("parseIperfTarget(2001:db8::1) = (%q, %d, %v)", host, port, err)
	}
	host, port, err = parseIperfTarget("iperf.example.com:5205")
	if err != nil || host != "iperf.example.com" || port != 5205 {
		t.Fatalf("parseIperfTarget(host:port) = (%q, %d, %v)", host, port, err)
	}

	if _, err := testPingAddress("-n"); err == nil {
		t.Fatal("testPingAddress(-n) should reject leading dash flag injection")
	}
}

func TestLogRedactionAndUTF8SafeTrimForLog(t *testing.T) {
	samples := []struct {
		in     string
		secret string
	}{
		{"Authorization: Bearer super_secret_token_12345", "super_secret_token_12345"},
		{"GET https://sub.example.com/api?token=my_sub_secret&flag=1", "my_sub_secret"},
		{`{"password":"my_node_password","server":"1.2.3.4"}`, "my_node_password"},
		{"failed to parse trojan://TopSecretPass@example.com:443#HK", "TopSecretPass"},
	}
	for _, tc := range samples {
		got := redactSensitiveLogText(tc.in)
		if strings.Contains(got, tc.secret) {
			t.Errorf("redactSensitiveLogText leaked secret %q in %q", tc.secret, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("redactSensitiveLogText did not insert [REDACTED] for %q -> %q", tc.in, got)
		}
	}

	longChinese := "节点名称包含超长中文描述测试截断不会破坏多字节字符编码边界验证一二三四五六七八九十零壹贰叁肆伍陆柒捌玖拾"
	trimmed := trimForLog(longChinese)
	if !utf8.ValidString(trimmed) {
		t.Fatalf("trimForLog produced invalid UTF-8: %q", trimmed)
	}
	secretLine := "trojan://SuperSecretPassword12345@example.com:443#HK"
	trimmedSecret := trimForLog(secretLine)
	if strings.Contains(trimmedSecret, "SuperSecretPassword12345") {
		t.Fatalf("trimForLog leaked credential: %q", trimmedSecret)
	}
}

func TestSubscriptionSSRFGuardAndParserHardening(t *testing.T) {
	ssrfBlocked := []string{
		"http://127.0.0.1/sub",
		"http://localhost/sub",
		"https://192.168.1.1/sub",
		"https://10.0.0.1/sub",
		"https://169.254.169.254/latest/meta-data/",
		"file:///C:/Windows/win.ini",
	}
	for _, u := range ssrfBlocked {
		if _, err := validateSubscriptionURL(u); err == nil {
			t.Errorf("validateSubscriptionURL(%q) should block SSRF/non-HTTP(S)", u)
		}
	}
	if _, err := validateSubscriptionURL("https://sub.example.com/api?token=abc"); err != nil {
		t.Errorf("validateSubscriptionURL should allow public HTTPS URL, got: %v", err)
	}

	sanitized := sanitizeSubscriptionErr(&url.Error{
		Op:  "Get",
		URL: "https://sub.example.com/link?token=TOP_SECRET_TOKEN",
		Err: errors.New("connection refused"),
	})
	if strings.Contains(sanitized.Error(), "TOP_SECRET_TOKEN") {
		t.Fatalf("sanitizeSubscriptionErr leaked query token: %v", sanitized)
	}

	// Uppercase scheme parsing (SS://, HY2://)
	ssRaw := "SS://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pass123")) + "@203.0.113.10:8388#UpperSS"
	hy2Raw := "HY2://pass123@203.0.113.11:443?sni=example.com#UpperHY2"
	nodes, errs := ParseNodes(ssRaw + "\n" + hy2Raw)
	if len(nodes) != 2 || len(errs) != 0 {
		t.Fatalf("expected 2 uppercase scheme nodes with 0 errors, got nodes=%d errs=%v", len(nodes), errs)
	}

	// Out-of-range port rejection in JSON outbound & Clash HTTP
	if _, err := parseJSONOutbound(`{"type":"socks","server":"203.0.113.10","server_port":70000}`); err == nil {
		t.Fatal("expected parseJSONOutbound to reject port 70000")
	}
	if _, err := parseJSONOutbound(`{"type":"socks","server":"203.0.113.10","server_port":0}`); err == nil {
		t.Fatal("expected parseJSONOutbound to reject port 0")
	}
	if _, err := parseClashHTTP(map[string]interface{}{"type": "http", "server": "203.0.113.10", "port": 70000}); err == nil {
		t.Fatal("expected parseClashHTTP to reject port 70000")
	}
	// IPv6 host formatting in Clash SOCKS / standard URLs
	socksURL := buildClashSOCKS(map[string]interface{}{"type": "socks5", "server": "2001:db8::1", "port": 1080, "name": "v6"})
	if !strings.Contains(socksURL, "[2001:db8::1]:1080") {
		t.Fatalf("expected Clash IPv6 SOCKS link to bracket IPv6 host, got %q", socksURL)
	}
}

func TestCloneStateAndCloneMapDeepCopyNoConcurrentMapPanic(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	node := Node{
		ID:      "node-deep",
		GroupID: defaultGroupID,
		Name:    "DeepNode",
		Type:    "vless",
		Address: "203.0.113.20",
		Port:    443,
		Outbound: map[string]interface{}{
			"type":        "vless",
			"server":      "203.0.113.20",
			"server_port": 443,
			"tls": map[string]interface{}{
				"enabled": true,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
			},
		},
	}
	if err := store.ReplaceGroupsAndNodes(store.Snapshot().Groups, []Node{node}, node.ID); err != nil {
		t.Fatal(err)
	}

	// Mutating nested map on Snapshot must NOT affect stored state.
	snap := store.Snapshot()
	snapTLS := snap.Nodes[0].Outbound["tls"].(map[string]interface{})
	snapUTLS := snapTLS["utls"].(map[string]interface{})
	snapUTLS["fingerprint"] = "mutated"

	fresh := store.Snapshot()
	freshUTLS := fresh.Nodes[0].Outbound["tls"].(map[string]interface{})["utls"].(map[string]interface{})
	if freshUTLS["fingerprint"] != "chrome" {
		t.Fatalf("Snapshot shallow-copied nested Outbound map: got %v, want chrome", freshUTLS["fingerprint"])
	}

	// Stress concurrent Snapshot mutations + Store writes to verify zero concurrent map panic.
	var wg sync.WaitGroup
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				s := store.Snapshot()
				if len(s.Nodes) > 0 {
					tlsMap := s.Nodes[0].Outbound["tls"].(map[string]interface{})
					tlsMap["server_name"] = "mutated.example.com"
					tlsMap["utls"].(map[string]interface{})["fingerprint"] = "firefox"
				}
				if id == 0 && i%5 == 0 {
					_ = store.UpdateSettings(s.Settings)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestProxyServerMatchesLocalPortNoPrefixCollision(t *testing.T) {
	if proxyServerMatchesLocalPort("127.0.0.1:7890", 789) {
		t.Fatal("port 789 must NOT prefix-match 127.0.0.1:7890")
	}
	if proxyServerMatchesLocalPort("http=127.0.0.1:208080;https=127.0.0.1:208080", 20808) {
		t.Fatal("port 20808 must NOT prefix-match 208080")
	}
	if !proxyServerMatchesLocalPort("127.0.0.1:20808", 20808) {
		t.Fatal("expected exact match for 127.0.0.1:20808")
	}
	if !proxyServerMatchesLocalPort("http=127.0.0.1:20808;https=127.0.0.1:20808", 20808) {
		t.Fatal("expected scheme-prefixed match for 127.0.0.1:20808")
	}
	if !proxyServerMatchesLocalPort("localhost:20808", 20808) {
		t.Fatal("expected exact match for localhost:20808")
	}
	if proxyServerMatchesLocalPort("127.0.0.1:20808", 0) || proxyServerMatchesLocalPort("127.0.0.1:20808", 70000) {
		t.Fatal("invalid port bounds should return false")
	}
}

type failingCloseWFPBackend struct {
	*fakeWFPBackend
	closeErr error
}

func (f *failingCloseWFPBackend) EngineClose(engine uintptr) error {
	f.fakeWFPBackend.closeCalls++
	if f.closeErr != nil {
		return f.closeErr
	}
	return f.fakeWFPBackend.EngineClose(engine)
}

func TestWFPV4MaskRejectsIPv4MappedIPv6AndPreservesEngineOnCloseError(t *testing.T) {
	if _, err := parseWFPV4AddrMask("::ffff:10.0.0.0/104"); err == nil {
		t.Fatal("parseWFPV4AddrMask must reject IPv4-mapped IPv6 CIDR")
	}
	if _, err := parseWFPV6AddrMask("::ffff:10.0.0.0/104"); err == nil {
		t.Fatal("parseWFPV6AddrMask must reject IPv4-mapped IPv6 CIDR")
	}

	origBackend := activeWFPBackend
	origAdmin := checkAdminPrivileges
	defer func() {
		wfpKS.mu.Lock()
		wfpKS.active = false
		wfpKS.engine = 0
		wfpKS.mu.Unlock()
		activeWFPBackend = origBackend
		checkAdminPrivileges = origAdmin
	}()

	checkAdminPrivileges = func() bool { return true }
	mock := &failingCloseWFPBackend{
		fakeWFPBackend: &fakeWFPBackend{},
		closeErr:       errors.New("simulated EngineClose failure"),
	}
	activeWFPBackend = mock

	if err := enableWFPKillSwitch(nil, `C:\mock\sing-box.exe`, `C:\mock\SanTian-Singbox.exe`); err != nil {
		t.Fatalf("enableWFPKillSwitch failed: %v", err)
	}
	if !isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch active after enable")
	}

	// First disable fails in EngineClose: engine handle & active state MUST be preserved so retry works!
	if err := disableWFPKillSwitch(nil); err == nil {
		t.Fatal("expected disableWFPKillSwitch to return EngineClose error")
	}
	if !isWFPKillSwitchActive() {
		t.Fatal("WFP engine handle must remain preserved when EngineClose fails")
	}

	// Second disable succeeds once EngineClose recovers.
	mock.closeErr = nil
	if err := disableWFPKillSwitch(nil); err != nil {
		t.Fatalf("retry disableWFPKillSwitch failed: %v", err)
	}
	if isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch inactive after successful close")
	}
}

func TestCoreUpdateStatusNoFalsePositiveWhenLatestEmpty(t *testing.T) {
	mgr := &SingBoxManager{
		coreVer:                "sing-box version 1.14.6",
		latestCoreVer:          "",
		latestCoreCheckedAt:    time.Now(),
		coreVersionFailedUntil: time.Now().Add(time.Minute),
	}
	latest, available := mgr.CoreUpdateStatus()
	if latest != "" || available {
		t.Fatalf("expected (\"\", false) when latestCoreVer is empty, got (%q, %v)", latest, available)
	}

	mgr.latestCoreVer = "v1.14.6"
	latest, available = mgr.CoreUpdateStatus()
	if latest != "v1.14.6" || available {
		t.Fatalf("expected (\"v1.14.6\", false) when versions match, got (%q, %v)", latest, available)
	}

	mgr.latestCoreVer = "v1.15.0"
	latest, available = mgr.CoreUpdateStatus()
	if latest != "v1.15.0" || !available {
		t.Fatalf("expected (\"v1.15.0\", true) when newer version available, got (%q, %v)", latest, available)
	}
}

func TestDirectPhysicalUDPDialerUsesUDPAddr(t *testing.T) {
	dialer := newDirectPhysicalUDPDialer(2 * time.Second)
	if dialer == nil {
		t.Fatal("newDirectPhysicalUDPDialer returned nil")
	}
	if dialer.LocalAddr != nil {
		if _, ok := dialer.LocalAddr.(*net.UDPAddr); !ok {
			t.Fatalf("newDirectPhysicalUDPDialer.LocalAddr must be *net.UDPAddr, got %T", dialer.LocalAddr)
		}
	}
}

func TestNormalizeSettingsPortBounds(t *testing.T) {
	s := defaultSettings()
	s.MixedPort = -5
	s.ProxyMapStartPort = 70000
	normalizeSettings(&s)
	if s.MixedPort < 1 || s.MixedPort > 65535 {
		t.Fatalf("MixedPort out of bounds after normalize: %d", s.MixedPort)
	}
	if s.ProxyMapStartPort < 1 || s.ProxyMapStartPort > 65535 {
		t.Fatalf("ProxyMapStartPort out of bounds after normalize: %d", s.ProxyMapStartPort)
	}
}

func TestSingBoxManagerClashSecretDoesNotDeadlockUnderManagerLock(t *testing.T) {
	mgr := NewSingBoxManager(t.TempDir(), "", "", "", nil)

	// Simulate startLocked / HealthCheck / TrafficSnapshot holding m.mu while reading Clash API auth secret.
	done := make(chan string, 1)
	go func() {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		secret := mgr.clashSecret()
		req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:20990/configs", nil)
		mgr.applyClashAuth(req)
		done <- req.Header.Get("Authorization") + "|" + secret
	}()

	select {
	case got := <-done:
		parts := strings.SplitN(got, "|", 2)
		if len(parts) != 2 || len(parts[1]) != 64 || parts[0] != "Bearer "+parts[1] {
			t.Fatalf("unexpected clash auth header/secret: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: clashSecret()/applyClashAuth() blocked while SingBoxManager.mu was held!")
	}
}

func TestSingBoxManagerRealStartHealthCheckAndStopNoDeadlock(t *testing.T) {
	realCore := filepath.Join(".", "bin", "sing-box.exe")
	if !fileExists(realCore) {
		t.Skip("bin/sing-box.exe not present")
	}

	pickFreePort := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		return port
	}

	mixedPort := pickFreePort()
	testClashPort := pickFreePort()
	for testClashPort == mixedPort {
		testClashPort = pickFreePort()
	}

	origPort := clashAPIPort
	origAddr := clashAPIAddress
	clashAPIPort = testClashPort
	clashAPIAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(testClashPort))
	t.Cleanup(func() {
		clashAPIPort = origPort
		clashAPIAddress = origAddr
	})

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "data", "sing-box.json")
	cachePath := filepath.Join(tmpDir, "data", "cache.db")
	logger := NewLogger(filepath.Join(tmpDir, "test.log"))
	defer logger.Close()

	mgr := NewSingBoxManager(tmpDir, realCore, cfgPath, cachePath, logger)
	node := Node{
		ID:      "node-e2e-1",
		Name:    "E2E-Test",
		Type:    "socks",
		Address: "203.0.113.10",
		Port:    1080,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "203.0.113.10",
			"server_port": 1080,
		},
	}
	settings := defaultSettings()
	settings.Mode = "global"
	settings.RuleSource = ruleSourceBuiltin
	settings.SystemProxy = false
	settings.Tun = false
	settings.MixedPort = mixedPort

	state := PersistState{
		SelectedID: node.ID,
		Nodes:      []Node{node},
		Settings:   settings,
	}

	if err := mgr.Start(state); err != nil {
		t.Fatalf("real SingBoxManager.Start failed: %v", err)
	}
	defer func() {
		_ = mgr.Stop()
	}()

	if err := mgr.HealthCheck(settings); err != nil {
		t.Fatalf("real SingBoxManager.HealthCheck failed (Clash API Bearer auth?): %v", err)
	}
	if _, err := mgr.TrafficSnapshot(); err != nil {
		t.Fatalf("real SingBoxManager.TrafficSnapshot failed: %v", err)
	}
	if _, err := mgr.FetchConnections(); err != nil {
		t.Fatalf("real SingBoxManager.FetchConnections failed: %v", err)
	}
	if err := mgr.SetMode("rule"); err != nil {
		t.Fatalf("real SingBoxManager.SetMode failed: %v", err)
	}
	if err := mgr.CloseConnections(); err != nil {
		t.Fatalf("real SingBoxManager.CloseConnections failed: %v", err)
	}
	cur, err := mgr.CurrentOutbound()
	if err != nil || cur != nodeOutboundTag(node) {
		t.Fatalf("CurrentOutbound = (%q, %v), want %q", cur, err, nodeOutboundTag(node))
	}
	// Verify unauthorized request without Bearer token is actually rejected by sing-box!
	unauthClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := unauthClient.Get("http://" + clashAPIAddress + "/configs")
	if err != nil {
		t.Fatalf("unexpected transport error probing clash API: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated Clash API request to return 401, got %d", resp.StatusCode)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("real SingBoxManager.Stop failed: %v", err)
	}
}

func TestAppProxyMixedAppAndFolderUsesSeparateRulesAndPassesSingBoxCheck(t *testing.T) {
	node := Node{
		ID:      "node-appproxy-1",
		Name:    "AppProxyNode",
		Type:    "socks",
		Address: "203.0.113.10",
		Port:    1080,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "203.0.113.10",
			"server_port": 1080,
		},
	}
	settings := defaultSettings()
	settings.Mode = "rule"
	settings.RuleSource = ruleSourceBuiltin
	settings.SystemProxy = false
	settings.Tun = true
	settings.AppProxy = true
	settings.AppProxyTargets = []AppProxyTarget{
		{ID: "t1", Type: "app", Path: `C:\Users\Alice\Desktop\tool.exe`, Enabled: true},
		{ID: "t2", Type: "folder", Path: `D:\Games`, Enabled: true},
		{ID: "t3", Type: "app", Path: `C:\Program Files\AdsPower\AdsPower.exe`, Enabled: true},
	}

	tmpDir := t.TempDir()
	cfg, err := BuildSingBoxConfig(PersistState{
		SelectedID: node.ID,
		Nodes:      []Node{node},
		Settings:   settings,
	}, node, filepath.Join(tmpDir, "cache.db"))
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}
	if !hasAppProxyRouteRule(cfg, settings) {
		t.Fatal("hasAppProxyRouteRule returned false for valid multi-rule AppProxy config")
	}

	route := cfg["route"].(map[string]interface{})
	rules := configRules(route["rules"])
	foundPathRule := false
	foundRegexRule := false
	for _, r := range rules {
		if r["action"] != "route" || r["outbound"] != "proxy" {
			continue
		}
		p := configStringValues(r["process_path"])
		rx := configStringValues(r["process_path_regex"])
		if len(p) > 0 && len(rx) > 0 {
			t.Fatalf("route rule MUST NOT combine process_path and process_path_regex (sing-box ANDs them): %#v", r)
		}
		if len(p) > 0 {
			foundPathRule = true
		}
		if len(rx) > 0 {
			foundRegexRule = true
		}
	}
	if !foundPathRule || !foundRegexRule {
		t.Fatalf("expected separate process_path and process_path_regex rules: path=%v regex=%v", foundPathRule, foundRegexRule)
	}

	realCore := filepath.Join(".", "bin", "sing-box.exe")
	if fileExists(realCore) {
		cfgPath := filepath.Join(tmpDir, "sing-box-appproxy.json")
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(realCore, "check", "-c", cfgPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("real sing-box check rejected split AppProxy config: %v\n%s", err, string(out))
		}
	}
}

func TestSingBoxManagerStatusReadsDoNotBlockOnLifecycleLock(t *testing.T) {
	mgr := NewSingBoxManager(t.TempDir(), "", "", "", nil)
	mgr.stateMu.Lock()
	mgr.runningDirectDNS = "192.168.1.1"
	mgr.hasCachedConfigMode = true
	mgr.cachedConfigMode = ConfigMode{Mode: "rule"}
	mgr.cachedTunActive = true
	mgr.stateMu.Unlock()

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = mgr.Running()
		_ = mgr.CurrentPID()
		if dns := mgr.RunningDirectDNS(); dns != "192.168.1.1" {
			t.Errorf("RunningDirectDNS = %q, want 192.168.1.1", dns)
		}
		mode, tun, ok := mgr.CachedRuntimeInfo()
		if !ok || !tun || mode.Mode != "rule" {
			t.Errorf("CachedRuntimeInfo = (%+v, %v, %v)", mode, tun, ok)
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SingBoxManager read-only status methods blocked while lifecycle m.mu was held")
	}
}

func TestEnsureProxyHealthDoesNotEnableKillSwitchWhenProxyOpHeld(t *testing.T) {
	defer resetFirewallMocks()
	origBackend := activeWFPBackend
	defer func() {
		wfpKS.mu.Lock()
		wfpKS.active = false
		wfpKS.engine = 0
		wfpKS.mu.Unlock()
		activeWFPBackend = origBackend
	}()

	checkAdminPrivileges = func() bool { return true }
	mockWFP := &fakeWFPBackend{}
	activeWFPBackend = mockWFP
	runFirewallCommand = func(name string, args ...string) (string, error) {
		return "No rules match", errors.New("not found")
	}

	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	node := Node{ID: "node-1", GroupID: defaultGroupID, Name: "N1", Type: "socks", Address: "203.0.113.10", Port: 1080}
	if err := store.ReplaceGroupsAndNodes(store.Snapshot().Groups, []Node{node}, node.ID); err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot().Settings
	settings.Tun = true
	settings.CircuitBreaker = true
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	app := &App{
		root:    tmpDir,
		store:   store,
		manager: NewSingBoxManager(tmpDir, `C:\mock\sing-box.exe`, "", "", nil),
		logger:  NewLogger(filepath.Join(tmpDir, "test.log")),
	}
	defer app.logger.Close()

	// Hold proxyOpMu to simulate active Restart/Stop in progress.
	app.proxyOpMu.Lock()
	app.ensureProxyHealth("定时自检")
	app.proxyOpMu.Unlock()

	if isWFPKillSwitchActive() || mockWFP.openCalls != 0 {
		t.Fatalf("ensureProxyHealth must NOT enable WFP Kill Switch while proxyOpMu is held (openCalls=%d)", mockWFP.openCalls)
	}
	status, _, _, _ := app.currentNetworkHealth()
	if status == "core_down" {
		t.Fatal("ensureProxyHealth must NOT set core_down alert while proxyOpMu is held")
	}
}

func TestTryAutoRepairOutboundConfigCleansProxyMapDNSDetourAndTracksRemovedTags(t *testing.T) {
	realCore := filepath.Join(".", "bin", "sing-box.exe")
	if !fileExists(realCore) {
		t.Skip("bin/sing-box.exe not present")
	}

	goodNode := Node{
		ID:      "node-pm-good",
		GroupID: defaultGroupID,
		Name:    "GoodPM",
		Type:    "socks",
		Address: "203.0.113.10",
		Port:    1080,
		Outbound: map[string]interface{}{
			"type":        "socks",
			"server":      "203.0.113.10",
			"server_port": 1080,
		},
	}
	badNode := Node{
		ID:      "node-pm-bad",
		GroupID: defaultGroupID,
		Name:    "BadPM",
		Type:    "vless",
		Address: "203.0.113.11",
		Port:    443,
		Outbound: map[string]interface{}{
			"type":        "vless",
			"server":      "203.0.113.11",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
			},
		},
	}

	settings := defaultSettings()
	settings.Mode = "rule"
	settings.RuleSource = ruleSourceBuiltin
	settings.SystemProxy = false
	settings.Tun = false
	settings.ProxyMap = true
	settings.ProxyMapGroupID = defaultGroupID
	settings.ProxyMapGroupIDs = []string{defaultGroupID}

	tmpDir := t.TempDir()
	cfg, err := BuildSingBoxConfig(PersistState{
		SelectedID: goodNode.ID,
		Nodes:      []Node{goodNode, badNode},
		Settings:   settings,
	}, goodNode, filepath.Join(tmpDir, "cache.db"))
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}

	// Corrupt the proxy-map outbound for badNode ("proxy-map-out-1") with an invalid ECH PEM.
	corrupted := false
	badPMTag := proxyMapOutboundTag(1)
	for _, ob := range cfg["outbounds"].([]map[string]interface{}) {
		if ob["tag"] == badPMTag {
			ob["tls"].(map[string]interface{})["ech"] = map[string]interface{}{
				"enabled": true,
				"config":  []string{"-----BEGIN CERTIFICATE-----", "broken", "-----END CERTIFICATE-----"},
			}
			corrupted = true
			break
		}
	}
	if !corrupted {
		t.Fatalf("failed to find %s in generated outbounds", badPMTag)
	}

	cfgPath := filepath.Join(tmpDir, "sing-box-pm-repair.json")
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, raw, 0600); err != nil {
		t.Fatal(err)
	}

	absCore, _ := filepath.Abs(realCore)
	mgr := NewSingBoxManager(tmpDir, absCore, cfgPath, filepath.Join(tmpDir, "cache.db"), nil)
	checkErr := mgr.checkConfig()
	if checkErr == nil {
		t.Fatal("expected initial checkConfig to fail on corrupted ProxyMap outbound")
	}

	repaired, repairErr := mgr.tryAutoRepairOutboundConfig(checkErr, goodNode)
	if !repaired || repairErr != nil {
		t.Fatalf("tryAutoRepairOutboundConfig failed to repair ProxyMap outbound + DNS detour: repaired=%v err=%v", repaired, repairErr)
	}
	if err := mgr.checkConfig(); err != nil {
		t.Fatalf("repaired ProxyMap config failed checkConfig: %v", err)
	}
	foundTracked := false
	for _, tag := range mgr.repairedRemovedTags {
		if tag == badPMTag {
			foundTracked = true
			break
		}
	}
	if !foundTracked {
		t.Fatalf("expected repairedRemovedTags to include %q, got %v", badPMTag, mgr.repairedRemovedTags)
	}
}
