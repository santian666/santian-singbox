package main

import (
	"path/filepath"
	"testing"
)

func TestOrphanedForeignSanTianCores(t *testing.T) {
	base := filepath.FromSlash("/apps")
	current := filepath.Join(base, "new", "bin", "sing-box.exe")
	oldCopy := filepath.Join(base, "old", "bin", "sing-box.exe")
	liveCopy := filepath.Join(base, "live", "bin", "sing-box.exe")
	otherApp := filepath.Join(base, "v2rayN", "bin", "sing-box.exe")
	notInBin := filepath.Join(base, "old", "sing-box.exe")

	sanTianDirs := map[string]bool{
		normalizeAppProxyPath(filepath.Join(base, "new")):  true,
		normalizeAppProxyPath(filepath.Join(base, "old")):  true,
		normalizeAppProxyPath(filepath.Join(base, "live")): true,
	}
	isSanTian := func(dir string) bool { return sanTianDirs[normalizeAppProxyPath(dir)] }

	processes := []processInfo{
		{PID: 100, ParentPID: 1, Name: "SanTian-Singbox.exe", Path: filepath.Join(base, "new", "SanTian-Singbox.exe")},
		{PID: 101, ParentPID: 100, Name: "sing-box.exe", Path: current}, // 本程序自己的内核：不结束
		{PID: 200, ParentPID: 999, Name: "sing-box.exe", Path: oldCopy}, // 旧副本遗留、父进程已退出：结束
		{PID: 300, ParentPID: 1, Name: "SanTian-Singbox.exe", Path: filepath.Join(base, "live", "SanTian-Singbox.exe")},
		{PID: 301, ParentPID: 300, Name: "sing-box.exe", Path: liveCopy}, // 另一个仍在运行的实例：不结束
		{PID: 400, ParentPID: 999, Name: "sing-box.exe", Path: otherApp}, // 其他软件的 sing-box：不结束
		{PID: 500, ParentPID: 999, Name: "sing-box.exe", Path: notInBin}, // 不在 bin 目录：不结束
		{PID: 600, ParentPID: 999, Name: "sing-box.exe", Path: ""},       // 路径不可读：不结束
		{PID: 700, ParentPID: 999, Name: "chrome.exe", Path: oldCopy},    // 不是 sing-box：不结束
	}

	got := orphanedForeignSanTianCores(current, processes, isSanTian)
	if len(got) != 1 || got[0].PID != 200 {
		t.Fatalf("只应选中旧副本遗留的内核 PID 200，实际：%+v", got)
	}
	if got := orphanedForeignSanTianCores(current, processes, nil); len(got) != 0 {
		t.Fatalf("没有目录判断函数时不应选中任何进程，实际：%+v", got)
	}
}
