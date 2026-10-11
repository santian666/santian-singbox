package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsArchiveTempDir(t *testing.T) {
	rarDir := filepath.Join(os.TempDir(), "Rar$EXa48096.28652", "bin")
	if !isArchiveTempDir(rarDir) {
		t.Fatalf("应识别 WinRAR 临时解压目录：%s", rarDir)
	}

	normalDir := filepath.Join(os.TempDir(), "go-singbox-normal")
	if isArchiveTempDir(normalDir) {
		t.Fatalf("不应把普通临时目录识别为压缩包临时目录：%s", normalDir)
	}
}
