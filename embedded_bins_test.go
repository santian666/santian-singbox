package main

import (
	"path/filepath"
	"testing"
)

func TestEmbeddedBinsAvailable(t *testing.T) {
	for _, name := range embeddedBinNames {
		target := filepath.Join("bin", name)
		if !fileExists(target) {
			t.Fatalf("运行组件文件缺失：%s", target)
		}
	}
}
