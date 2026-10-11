package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCoreWithCompanionDlls(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test-core.zip")

	// Create test zip with sing-box.exe and libcronet.dll
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip: %v", err)
	}
	w := zip.NewWriter(zipFile)

	f1, err := w.Create("sing-box-1.14.3-windows-amd64/sing-box.exe")
	if err != nil {
		t.Fatalf("failed to create file entry: %v", err)
	}
	_, _ = f1.Write([]byte("fake-sing-box-binary"))

	f2, err := w.Create("sing-box-1.14.3-windows-amd64/libcronet.dll")
	if err != nil {
		t.Fatalf("failed to create file entry: %v", err)
	}
	_, _ = f2.Write([]byte("fake-libcronet-dll"))

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	_ = zipFile.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("failed to create destDir: %v", err)
	}

	result, err := extractCore(zipPath, destDir)
	if err != nil {
		t.Fatalf("extractCore failed: %v", err)
	}

	if filepath.Base(result.coreExe) != "sing-box.exe" {
		t.Fatalf("expected coreExe sing-box.exe, got %s", result.coreExe)
	}
	if len(result.companionFiles) != 1 || filepath.Base(result.companionFiles[0]) != "libcronet.dll" {
		t.Fatalf("expected companionFiles to contain libcronet.dll, got %v", result.companionFiles)
	}

	data1, _ := os.ReadFile(result.coreExe)
	if string(data1) != "fake-sing-box-binary" {
		t.Fatalf("unexpected content in coreExe: %s", string(data1))
	}
	data2, _ := os.ReadFile(result.companionFiles[0])
	if string(data2) != "fake-libcronet-dll" {
		t.Fatalf("unexpected content in companion file: %s", string(data2))
	}
}

func TestSwapAndRollbackCoreFiles(t *testing.T) {
	tmpDir := t.TempDir()

	exeTarget := filepath.Join(tmpDir, "sing-box.exe")
	dllTarget := filepath.Join(tmpDir, "libcronet.dll")

	_ = os.WriteFile(exeTarget, []byte("old-exe"), 0755)
	_ = os.WriteFile(dllTarget, []byte("old-dll"), 0644)

	exeStaged := filepath.Join(tmpDir, "sing-box.exe.new")
	dllStaged := filepath.Join(tmpDir, "libcronet.dll.new")

	_ = os.WriteFile(exeStaged, []byte("new-exe"), 0755)
	_ = os.WriteFile(dllStaged, []byte("new-dll"), 0644)

	pairs := []stagedFilePair{
		{targetPath: exeTarget, stagedPath: exeStaged},
		{targetPath: dllTarget, stagedPath: dllStaged},
	}

	swapped, err := swapCoreFiles(pairs)
	if err != nil {
		t.Fatalf("swapCoreFiles failed: %v", err)
	}

	// Verify target files are updated
	curExe, _ := os.ReadFile(exeTarget)
	if string(curExe) != "new-exe" {
		t.Fatalf("exe not updated: %s", string(curExe))
	}
	curDll, _ := os.ReadFile(dllTarget)
	if string(curDll) != "new-dll" {
		t.Fatalf("dll not updated: %s", string(curDll))
	}

	// Verify rollback restores old files
	if err := rollbackCoreFiles(swapped); err != nil {
		t.Fatalf("rollbackCoreFiles failed: %v", err)
	}

	oldExe, _ := os.ReadFile(exeTarget)
	if string(oldExe) != "old-exe" {
		t.Fatalf("exe not rolled back: %s", string(oldExe))
	}
	oldDll, _ := os.ReadFile(dllTarget)
	if string(oldDll) != "old-dll" {
		t.Fatalf("dll not rolled back: %s", string(oldDll))
	}
}

func TestTunConfigHasNativeDnsMode(t *testing.T) {
	state := PersistState{
		Settings: Settings{
			Mode:      "rule",
			Tun:       true,
			MixedPort: 20808,
		},
		Nodes: []Node{
			{
				ID:      "test-1",
				Name:    "Test Node",
				Type:    "shadowsocks",
				Address: "1.2.3.4",
				Port:    8388,
				Outbound: map[string]interface{}{
					"type":        "shadowsocks",
					"server":      "1.2.3.4",
					"server_port": 8388,
					"method":      "aes-128-gcm",
					"password":    "test",
				},
			},
		},
	}
	cfg, err := BuildSingBoxConfig(state, state.Nodes[0], "")
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}
	inbounds, ok := cfg["inbounds"].([]map[string]interface{})
	if !ok {
		t.Fatalf("invalid inbounds format")
	}
	foundTun := false
	for _, in := range inbounds {
		if in["type"] == "tun" {
			foundTun = true
			if in["dns_mode"] != "native" {
				t.Fatalf("expected tun dns_mode to be 'native', got: %v", in["dns_mode"])
			}
		}
	}
	if !foundTun {
		t.Fatalf("tun inbound not found in config")
	}
}
