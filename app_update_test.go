package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"6.0.80", "6.0.79", 1},
		{"v6.0.80", "6.0.79", 1},
		{"6.0.79", "v6.0.79", 0},
		{"6.0.78", "6.0.79", -1},
		{"6.1.0", "6.0.99", 1},
		{"7.0.0", "6.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"1.0.1", "1.0", 1},
	}

	for _, tt := range tests {
		got := compareSemver(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("compareSemver(%q, %q) = %d, expected %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestExtractZipArchive(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "sample.zip")

	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip: %v", err)
	}
	w := zip.NewWriter(zipFile)

	f1, err := w.Create("SanTian-Singbox-v6.0.80/SanTian-Singbox.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = f1.Write([]byte("fake-new-app-exe"))

	f2, err := w.Create("SanTian-Singbox-v6.0.80/bin/sing-box.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = f2.Write([]byte("fake-new-sing-box"))

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	_ = zipFile.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractZipArchive(zipPath, destDir); err != nil {
		t.Fatalf("extractZipArchive failed: %v", err)
	}

	located := locateExtractedAppDir(destDir, "SanTian-Singbox.exe")
	expectedSub := filepath.Join(destDir, "SanTian-Singbox-v6.0.80")
	if located != expectedSub {
		t.Fatalf("locateExtractedAppDir = %s, expected %s", located, expectedSub)
	}

	content, err := os.ReadFile(filepath.Join(located, "SanTian-Singbox.exe"))
	if err != nil || string(content) != "fake-new-app-exe" {
		t.Fatalf("extracted content mismatch: %v, %s", err, string(content))
	}
}

func TestPerformAtomicInPlaceUpdate(t *testing.T) {
	tmpAppRoot := t.TempDir()
	tmpStaging := t.TempDir()

	exePath := filepath.Join(tmpAppRoot, "SanTian-Singbox.exe")
	if err := os.WriteFile(exePath, []byte("old-exe"), 0755); err != nil {
		t.Fatal(err)
	}

	newExePath := filepath.Join(tmpStaging, "SanTian-Singbox.exe")
	if err := os.WriteFile(newExePath, []byte("new-exe"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := performAtomicInPlaceUpdate(tmpAppRoot, tmpStaging, exePath); err != nil {
		t.Fatalf("performAtomicInPlaceUpdate failed: %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil || string(data) != "new-exe" {
		t.Fatalf("exe content mismatch: %v, %s", err, string(data))
	}

	oldData, err := os.ReadFile(exePath + ".old")
	if err != nil || string(oldData) != "old-exe" {
		t.Fatalf("old exe content mismatch: %v, %s", err, string(oldData))
	}
}

func TestIsValidUpdateZip(t *testing.T) {
	tmpDir := t.TempDir()
	validZipPath := filepath.Join(tmpDir, "valid.zip")

	// 1. Create a valid zip with SanTian-Singbox.exe
	f, err := os.Create(validZipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	entry, err := zw.Create("SanTian-Singbox.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("binary-data"))
	_ = zw.Close()
	_ = f.Close()

	fi, err := os.Stat(validZipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !isValidUpdateZip(validZipPath, fi.Size()) {
		t.Errorf("isValidUpdateZip failed for valid zip")
	}
	if !isValidUpdateZip(validZipPath, 0) {
		t.Errorf("isValidUpdateZip failed for valid zip with expectedSize=0")
	}
	if isValidUpdateZip(validZipPath, fi.Size()+100) {
		t.Errorf("isValidUpdateZip should fail when expected size mismatches")
	}

	// 2. Corrupt zip
	corruptZipPath := filepath.Join(tmpDir, "corrupt.zip")
	_ = os.WriteFile(corruptZipPath, []byte("not-a-zip"), 0644)
	if isValidUpdateZip(corruptZipPath, 0) {
		t.Errorf("isValidUpdateZip should fail for corrupt zip")
	}
}

func TestCleanupOldUpdateArtifactsPreservesPendingUpdate(t *testing.T) {
	tmpRoot := t.TempDir()
	updatesDir := filepath.Join(tmpRoot, "data", "updates")
	_ = os.MkdirAll(filepath.Join(updatesDir, "staging"), 0755)

	zipPath := filepath.Join(updatesDir, "update.zip")
	_ = os.WriteFile(zipPath, []byte("cached-zip-content"), 0644)
	_ = os.WriteFile(filepath.Join(updatesDir, "staging", "temp.txt"), []byte("staging"), 0644)
	_ = os.WriteFile(filepath.Join(updatesDir, "target_version.txt"), []byte("v99.9.9"), 0644)

	// AppVersion is e.g. "0.0.1" which is < "99.9.9"
	cleanupOldUpdateArtifacts(tmpRoot)

	// update.zip and target_version.txt MUST be preserved!
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		t.Errorf("cleanupOldUpdateArtifacts should NOT have deleted update.zip for pending update")
	}
	// staging directory MUST be cleaned!
	if _, err := os.Stat(filepath.Join(updatesDir, "staging", "temp.txt")); !os.IsNotExist(err) {
		t.Errorf("cleanupOldUpdateArtifacts should have cleaned temporary staging files")
	}
}

func TestCheckUpdateRequiresProxyPort(t *testing.T) {
	mgr := newAppUpdateManager(nil)
	info, err := mgr.CheckUpdate(0)
	if err == nil || err.Error() != "proxy_not_running" {
		t.Fatalf("expected proxy_not_running error when proxyPort is 0, got info=%v, err=%v", info, err)
	}
}

func TestDownloadUpdateAssetRequiresProxyPort(t *testing.T) {
	err := downloadUpdateAsset("https://example.com/test.zip", "test.zip", 0, 100, nil)
	if err == nil {
		t.Fatalf("expected error when proxyPort is 0, got nil")
	}
}

func TestAppUpdateRequiresRunningProxy(t *testing.T) {
	app := &App{
		appUpdater: newAppUpdateManager(nil),
	}
	// Proxy not running, force = true (manual click)
	_, err := app.CheckAppUpdate(true)
	if err == nil || !strings.Contains(err.Error(), "请先开启代理") {
		t.Fatalf("expected error containing 请先开启代理, got: %v", err)
	}

	// Proxy not running, force = false (auto check)
	_, err = app.CheckAppUpdate(false)
	if err == nil || err.Error() != "proxy_not_running" {
		t.Fatalf("expected proxy_not_running error, got: %v", err)
	}

	// ApplyAppUpdate requires proxy
	_, err = app.ApplyAppUpdate()
	if err == nil || !strings.Contains(err.Error(), "请先开启代理") {
		t.Fatalf("expected ApplyAppUpdate to require proxy, got: %v", err)
	}
}

