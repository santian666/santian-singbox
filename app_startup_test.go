package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureProxyHealthSkipsBeforeStartupReady(t *testing.T) {
	logger := NewLogger(filepath.Join(t.TempDir(), "app.log"))
	defer logger.Close()
	app := &App{
		store: &Store{
			state: PersistState{
				SelectedID: "n1",
				Settings: Settings{
					Mode:      "global",
					Tun:       true,
					MixedPort: 20808,
				},
				Nodes: []Node{{
					ID:      "n1",
					Name:    "测试节点",
					Type:    "socks",
					Address: "127.0.0.1",
					Port:    10001,
					Outbound: map[string]interface{}{
						"type":        "socks",
						"server":      "127.0.0.1",
						"server_port": 10001,
					},
				}},
			},
		},
		manager: &SingBoxManager{},
		logger:  logger,
	}

	app.ensureProxyHealthAsync("状态刷新")
	time.Sleep(50 * time.Millisecond)

	app.healthMu.Lock()
	defer app.healthMu.Unlock()
	if app.startupReady {
		t.Fatal("测试前置条件错误：启动自检完成标记不应自动变为 true")
	}
	if app.healthChecking {
		t.Fatal("启动自检完成前不应启动代理健康检查，避免自动恢复代理抢跑")
	}
}

func TestShouldCleanupTunBeforeStartupRestoreSkipsAutoTunRestore(t *testing.T) {
	settings := Settings{Mode: "global", Tun: true, MixedPort: 20808}
	if shouldCleanupTunBeforeStartupRestore(true, true, settings, false) {
		t.Fatal("管理员重启后自动恢复 TUN 时不应先清理 TUN 网卡，避免重复重建设备导致生效慢")
	}
	if !shouldCleanupTunBeforeStartupRestore(true, true, settings, true) {
		t.Fatal("系统代理交接到管理员 TUN 时应由新实例清理一次残留，避免新旧实例并发操作网卡")
	}
	if !shouldCleanupTunBeforeStartupRestore(false, true, settings, false) {
		t.Fatal("普通启动时应清理上次残留 TUN")
	}
	settings.Tun = false
	settings.SystemProxy = true
	if !shouldCleanupTunBeforeStartupRestore(true, true, settings, false) {
		t.Fatal("自动恢复非 TUN 入口时仍应清理残留 TUN")
	}
}

func TestAdminTunHandoffPreservesProxySettingsOnExit(t *testing.T) {
	app := &App{}
	if app.shouldPreserveProxySettingsOnExit() {
		t.Fatal("普通退出不应保留代理入口设置")
	}
	app.markAdminTunHandoffExit()
	if !app.shouldPreserveProxySettingsOnExit() {
		t.Fatal("管理员 TUN 交接退出必须保留待启动设置")
	}
}

func TestAdminTunHandoffCleanupKeepsStoredTunEnabled(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot().Settings
	settings.Tun = true
	settings.SystemProxy = false
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}
	app.markAdminTunHandoffExit()
	app.cleanupAllProxyEffects("管理员 TUN 交接测试")
	stored := store.Snapshot().Settings
	if !stored.Tun || stored.SystemProxy {
		t.Fatalf("交接退出后必须保留 TUN=true、系统代理=false，got tun=%v systemProxy=%v", stored.Tun, stored.SystemProxy)
	}
}

func TestValidateSelectedNodeForSettings(t *testing.T) {
	state := PersistState{
		SelectedID: "node-1",
		Nodes:      []Node{{ID: "node-1"}},
	}
	if err := validateSelectedNodeForSettings(state, Settings{Mode: "global", SystemProxy: true}); err != nil {
		t.Fatalf("有效节点应允许开启系统代理：%v", err)
	}
	if err := validateSelectedNodeForSettings(PersistState{}, Settings{Mode: "global"}); err != nil {
		t.Fatalf("仅切换代理模式不应要求选择节点：%v", err)
	}

	for _, settings := range []Settings{
		{Mode: "global", SystemProxy: true},
		{Mode: "global", Tun: true},
		{Mode: "global", ProxyMap: true},
	} {
		if err := validateSelectedNodeForSettings(PersistState{}, settings); err == nil || err.Error() != "请先选择有效节点" {
			t.Fatalf("没有有效节点时必须拒绝开启代理入口，settings=%+v err=%v", settings, err)
		}
	}

	stale := PersistState{SelectedID: "missing", Nodes: []Node{{ID: "node-1"}}}
	if err := validateSelectedNodeForSettings(stale, Settings{Mode: "global", SystemProxy: true}); err == nil {
		t.Fatal("已选择节点不存在时必须拒绝开启系统代理")
	}
}
