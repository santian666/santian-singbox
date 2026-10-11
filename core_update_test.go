package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPGetBytesRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 33)))
	}))
	defer server.Close()

	if _, err := httpGetBytes(server.URL, "", time.Second, 32); err == nil {
		t.Fatal("下载内容超过限制时应返回错误")
	}
}

func TestCoreCandidateTimeoutAllowsWindowsSecurityScan(t *testing.T) {
	if coreCandidateTimeout < 15*time.Second {
		t.Fatalf("新内核首次运行校验超时过短：%s", coreCandidateTimeout)
	}
	if coreVersionTimeout >= coreCandidateTimeout {
		t.Fatalf("日常版本查询不应使用新内核验收的长超时：query=%s candidate=%s", coreVersionTimeout, coreCandidateTimeout)
	}
}

func TestDownloadCoreAssetRetriesTransientLocalProxyFailure(t *testing.T) {
	var attempts atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte("core-data"))
	}))
	defer proxy.Close()
	proxyPort := proxy.Listener.Addr().(*net.TCPAddr).Port
	target := filepath.Join(t.TempDir(), "sing-box.zip")

	if err := downloadCoreAsset("http://download.invalid/sing-box.zip", "v1.2.3", target, proxyPort, nil); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("本地代理瞬时断开后未重试：attempts=%d", attempts.Load())
	}
	assertFileContent(t, target, "core-data")
}

func TestHTTPGetBytesFromOptionsUsesLocalProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("rule-data"))
	}))
	defer proxy.Close()
	body, err := httpGetBytesFromOptions([]coreDownloadOption{{
		label:    "本地代理规则源",
		url:      "http://download.invalid/rule.txt",
		proxyURL: proxy.URL,
	}}, time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "rule-data" {
		t.Fatalf("本地代理规则下载内容错误：%q", body)
	}
}

func TestSwapCoreExecutableCanRollback(t *testing.T) {
	dir := t.TempDir()
	corePath := filepath.Join(dir, "sing-box.exe")
	stagedPath := corePath + ".new"
	if err := os.WriteFile(corePath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}

	backupPath, hadOld, err := swapCoreExecutable(corePath, stagedPath)
	if err != nil {
		t.Fatalf("替换内核失败：%v", err)
	}
	if !hadOld {
		t.Fatal("应识别已有旧内核")
	}
	assertFileContent(t, corePath, "new")
	assertFileContent(t, backupPath, "old")

	if err := rollbackCoreExecutable(corePath, backupPath, hadOld); err != nil {
		t.Fatalf("回滚内核失败：%v", err)
	}
	assertFileContent(t, corePath, "old")
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("回滚后不应残留备份文件：%v", err)
	}
}

func TestRulePresetUsesLoyalsoldierSource(t *testing.T) {
	presets := RulePresets()
	if len(presets) == 0 || presets[0].ID != ruleSourceLoyalsoldier || presets[0].Name != "Loyalsoldier" {
		t.Fatalf("默认规则源应显示为 Loyalsoldier：%+v", presets)
	}
	for _, def := range rulePresetDefs[ruleSourceLoyalsoldier] {
		if !strings.Contains(def.URL, "Loyalsoldier/v2ray-rules-dat") && !strings.Contains(def.URL, "Loyalsoldier/geoip") {
			t.Fatalf("规则下载地址不是 Loyalsoldier 仓库：%s", def.URL)
		}
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("文件内容错误：期望 %q，实际 %q", expected, string(data))
	}
}
