package main

import (
	"archive/zip"
	"os"
	"path/filepath"
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
