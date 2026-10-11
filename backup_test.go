package main

import (
	"encoding/json"
	"testing"
)

func TestParseNodeBackupAcceptsValidBackup(t *testing.T) {
	data, err := json.Marshal(NodeBackupFile{
		App:     "SanTian-Singbox",
		Version: backupFileVersion,
		Groups:  []NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}},
		Nodes:   []Node{{ID: "node-1", Name: "测试节点", GroupID: defaultGroupID, Raw: "socks5://example"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := parseNodeBackup(data)
	if err != nil {
		t.Fatalf("parseNodeBackup() error = %v", err)
	}
	if len(backup.Groups) != 1 || len(backup.Nodes) != 1 {
		t.Fatalf("backup counts = groups %d, nodes %d", len(backup.Groups), len(backup.Nodes))
	}
}

func TestParseNodeBackupKeepsExternalProxyEndpoints(t *testing.T) {
	data, err := json.Marshal(NodeBackupFile{
		App:     "SanTian-Singbox",
		Version: backupFileVersion,
		Groups:  []NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}},
		Nodes:   []Node{{ID: "node-1", Name: "测试节点", GroupID: defaultGroupID, Raw: "socks5://example"}},
		ExternalProxyEndpoints: []ExternalProxyEndpoint{
			{Host: "203.0.113.8", Port: 7778, Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := parseNodeBackup(data)
	if err != nil {
		t.Fatalf("parseNodeBackup() error = %v", err)
	}
	if len(backup.ExternalProxyEndpoints) != 1 || backup.ExternalProxyEndpoints[0].Port != 7778 {
		t.Fatalf("外部代理入口未保留：%+v", backup.ExternalProxyEndpoints)
	}
}

func TestParseNodeBackupAcceptsVersionOneWithoutExternalProxyEndpoints(t *testing.T) {
	data, err := json.Marshal(NodeBackupFile{
		App:     "SanTian-Singbox",
		Version: 1,
		Groups:  []NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}},
		Nodes:   []Node{{ID: "node-1", Name: "测试节点", GroupID: defaultGroupID, Raw: "socks5://example"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseNodeBackup(data); err != nil {
		t.Fatalf("旧版备份应保持兼容：%v", err)
	}
}

func TestVersionOneBackupKeepsCurrentExternalProxyEndpoints(t *testing.T) {
	settings := defaultSettings()
	settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{{Host: "203.0.113.8", Port: 7778, Enabled: true}}
	got, restored := settingsWithBackupExternalProxyEndpoints(settings, NodeBackupFile{Version: 1})
	if restored || len(got.ExternalProxyEndpoints) != 1 {
		t.Fatalf("旧版备份不应清空当前外部代理入口：%+v", got.ExternalProxyEndpoints)
	}
}

func TestVersionTwoBackupRestoresExternalProxyEndpoints(t *testing.T) {
	settings := defaultSettings()
	settings.ExternalProxyEndpoints = []ExternalProxyEndpoint{{Host: "203.0.113.8", Port: 7778, Enabled: true}}
	backup := NodeBackupFile{
		Version:                2,
		ExternalProxyEndpoints: []ExternalProxyEndpoint{{Host: "198.51.100.9", Port: 1080, Enabled: true}},
	}
	got, restored := settingsWithBackupExternalProxyEndpoints(settings, backup)
	if !restored || len(got.ExternalProxyEndpoints) != 1 || got.ExternalProxyEndpoints[0].Host != "198.51.100.9" {
		t.Fatalf("新版备份未恢复外部代理入口：%+v", got.ExternalProxyEndpoints)
	}
}

func TestParseNodeBackupRejectsOtherApp(t *testing.T) {
	data, err := json.Marshal(NodeBackupFile{
		App:     "Other",
		Version: backupFileVersion,
		Groups:  []NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}},
		Nodes:   []Node{{ID: "node-1", Name: "测试节点", GroupID: defaultGroupID, Raw: "socks5://example"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseNodeBackup(data); err == nil {
		t.Fatal("parseNodeBackup() error = nil, want error")
	}
}
