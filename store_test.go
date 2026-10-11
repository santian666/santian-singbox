package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadKeepsLastMode(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	settings := store.Snapshot().Settings
	settings.Mode = "rule"
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot().Settings.Mode; got != "rule" {
		t.Fatalf("expected manual mode switch to keep rule before restart, got %q", got)
	}

	reloadedStore, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloadedStore.Snapshot().Settings.Mode; got != "rule" {
		t.Fatalf("expected startup mode to keep the last saved mode, got %q", got)
	}
}

func TestStoreUpdateSettingsKeepsDirectDomainsOutsideRuleMode(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	settings := store.Snapshot().Settings
	settings.Mode = "rule"
	settings.DirectDomains = "example.com"
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	settings = store.Snapshot().Settings
	settings.Mode = "global"
	settings.DirectDomains = ""
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	if got := store.Snapshot().Settings.DirectDomains; got != "example.com" {
		t.Fatalf("expected direct domains to be kept outside rule mode, got %q", got)
	}
}

func TestStoreUpdateSettingsCanClearDirectDomainsInRuleMode(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	settings := store.Snapshot().Settings
	settings.Mode = "rule"
	settings.DirectDomains = "example.com"
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	settings = store.Snapshot().Settings
	settings.Mode = "rule"
	settings.DirectDomains = ""
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	if got := store.Snapshot().Settings.DirectDomains; got != "" {
		t.Fatalf("expected direct domains to be cleared in rule mode, got %q", got)
	}
}

func TestStoreThemeUpdateSurvivesStaleSettingsWrite(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	staleSettings := store.Snapshot().Settings
	if err := store.UpdateTheme("dark"); err != nil {
		t.Fatal(err)
	}
	staleSettings.Mode = "rule"
	if err := store.UpdateSettings(staleSettings); err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot().Settings.Theme; got != "dark" {
		t.Fatalf("stale settings write overwrote theme: %q", got)
	}
}

func TestStoreMoveNodesToGroup(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	group := NodeGroup{ID: "target", Name: "目标组", Source: "manual"}
	if err := store.ReplaceNodes([]Node{
		{ID: "n1", Name: "节点1", GroupID: defaultGroupID},
		{ID: "n2", Name: "节点2", GroupID: defaultGroupID},
	}, ""); err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if err := store.ReplaceGroupsAndNodes(append(state.Groups, group), state.Nodes, state.SelectedID); err != nil {
		t.Fatal(err)
	}

	if err := store.MoveNodesToGroup([]string{"n1"}, group.ID); err != nil {
		t.Fatal(err)
	}
	state = store.Snapshot()
	for _, node := range state.Nodes {
		if node.ID == "n1" && node.GroupID != group.ID {
			t.Fatalf("expected n1 moved to %q, got %q", group.ID, node.GroupID)
		}
		if node.ID == "n2" && node.GroupID != defaultGroupID {
			t.Fatalf("expected n2 kept in default group, got %q", node.GroupID)
		}
	}
	if err := store.MoveNodesToGroup([]string{"n1"}, "missing"); err == nil {
		t.Fatal("expected moving to missing group to fail")
	}
}

func TestStoreTrafficIsBufferedUntilFlush(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceNodes([]Node{{ID: "n1", Name: "节点1", GroupID: defaultGroupID}}, "n1"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTraffic("n1", defaultGroupID, 100, 200); err != nil {
		t.Fatal(err)
	}
	afterAdd, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterAdd) {
		t.Fatal("流量累计不应立即重写状态文件")
	}
	if got := store.Snapshot().Nodes[0].DownloadTotal; got != 200 {
		t.Fatalf("内存流量累计错误：%d", got)
	}
	if err := store.FlushTraffic(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	state := reloaded.Snapshot()
	if state.Nodes[0].UploadTotal != 100 || state.Nodes[0].DownloadTotal != 200 {
		t.Fatalf("刷新后流量未持久化：%+v", state.Nodes[0])
	}
}

func TestStoreSelectionUsesSmallIndependentState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceNodes([]Node{{ID: "n1", Name: "节点1", GroupID: defaultGroupID}}, ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SelectNode("n1"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("选择节点不应重写完整状态文件")
	}
	reloaded, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Snapshot().SelectedID; got != "n1" {
		t.Fatalf("独立选择状态未恢复：%q", got)
	}
}

func TestWriteFileAtomicReadOnlyFallback(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test_atomic.dat")
	initial := []byte("initial data")
	if err := writeFileAtomic(tempFile, initial, 0644); err != nil {
		t.Fatalf("Initial write failed: %v", err)
	}

	// Make file read-only
	if err := os.Chmod(tempFile, 0444); err != nil {
		t.Fatalf("Failed to make file read-only: %v", err)
	}

	updated := []byte("updated data after readonly")
	if err := writeFileAtomic(tempFile, updated, 0644); err != nil {
		t.Fatalf("writeFileAtomic failed on read-only file: %v", err)
	}

	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(data, updated) {
		t.Fatalf("Expected %s, got %s", string(updated), string(data))
	}
}

