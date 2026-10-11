//go:build windows

package main

import "testing"

func TestParseProxyWatchdogOptions(t *testing.T) {
	options, err := parseProxyWatchdogOptions([]string{
		proxyWatchdogArg,
		"--parent-pid=1234",
		`--root=C:\SanTian`,
		`--state=C:\SanTian\data\gui\state.dat`,
		`--core=C:\SanTian\bin\sing-box.exe`,
		`--gui=C:\SanTian\SanTian-Singbox.exe`,
	})
	if err != nil {
		t.Fatalf("解析 watchdog 参数失败：%v", err)
	}
	if options.ParentPID != 1234 || options.CorePath == "" || options.StatePath == "" {
		t.Fatalf("watchdog 参数错误：%+v", options)
	}
}

func TestParseProxyWatchdogOptionsRejectsMissingPath(t *testing.T) {
	if _, err := parseProxyWatchdogOptions([]string{proxyWatchdogArg, "--parent-pid=1234"}); err == nil {
		t.Fatal("缺少路径时应拒绝启动 watchdog")
	}
}

func TestProxyWatchdogConsumesAdminHandoffOnce(t *testing.T) {
	root := t.TempDir()
	if err := markProxyWatchdogAdminHandoff(root, 4321); err != nil {
		t.Fatal(err)
	}
	ok, err := consumeProxyWatchdogAdminHandoff(root, 4321)
	if err != nil || !ok {
		t.Fatalf("watchdog 应识别管理员交接标记，ok=%v err=%v", ok, err)
	}
	ok, err = consumeProxyWatchdogAdminHandoff(root, 4321)
	if err != nil || ok {
		t.Fatalf("管理员交接标记只能消费一次，ok=%v err=%v", ok, err)
	}
}
