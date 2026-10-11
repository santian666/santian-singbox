package main

import (
	"path/filepath"
	"strings"
)

var embeddedBinNames = []string{
	"sing-box.exe",
	"nexttrace.exe",
	"iperf3.exe",
	"speedtest.exe",
	"cygwin1.dll",
	"WinDivert.dll",
	"WinDivert64.sys",
}

func ensureEmbeddedBins(root string, logger *Logger) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	binDir := filepath.Join(root, "bin")
	for _, name := range embeddedBinNames {
		target := filepath.Join(binDir, name)
		if !fileExists(target) {
			if logger != nil {
				logger.Error("运行组件未就绪：%s", target)
			}
		}
	}
	return nil
}
