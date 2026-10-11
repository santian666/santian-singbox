package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestFetchSubscriptionRejectsOversizedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(maxSubscriptionBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := FetchSubscription(context.Background(), server.URL, 0); err == nil {
		t.Fatal("超出大小限制的订阅不应继续读取或解析")
	}
}

func TestFetchSubscriptionRetriesTransientProxyFailureBeforeDirect(t *testing.T) {
	var attempts atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte("subscription-data"))
	}))
	defer proxy.Close()

	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	body, _, mode, err := fetchSubscriptionWithFallback(context.Background(), "http://subscription.invalid/list", port)
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || mode != "proxy" || body != "subscription-data" {
		t.Fatalf("订阅代理重试结果异常：attempts=%d mode=%s body=%q", attempts.Load(), mode, body)
	}
}

func TestUpdateSubscriptionGroupReplacesOnlyTargetGroup(t *testing.T) {
	body := `proxies:
  - name: new-a
    type: trojan
    server: new-a.example.com
    port: 443
    password: pass-a
  - name: new-b
    type: trojan
    server: new-b.example.com
    port: 443
    password: pass-b
proxy-groups:
  - name: Group A
    type: select
    proxies:
      - new-a
  - name: Group B
    type: select
    proxies:
      - new-b
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	groupAID := groupIDFromName(server.URL + "|Group A")
	groupBID := groupIDFromName(server.URL + "|Group B")
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "Default", Source: "manual"},
		{ID: groupAID, Name: "Group A", Source: "subscription", URL: server.URL},
		{ID: groupBID, Name: "Group B", Source: "subscription", URL: server.URL},
	}
	nodes := []Node{
		{ID: "old-a", Name: "old-a", Type: "trojan", Address: "old-a.example.com", Port: 443, GroupID: groupAID},
		{ID: "old-b", Name: "old-b", Type: "trojan", Address: "old-b.example.com", Port: 443, GroupID: groupBID},
	}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, "old-a"); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}

	result, err := app.UpdateSubscriptionGroup(groupAID)
	if err != nil {
		t.Fatal(err)
	}
	if result["updated"].(int) != 1 {
		t.Fatalf("updated = %v, want 1", result["updated"])
	}
	state := store.Snapshot()
	var groupANodes, groupBNodes []Node
	for _, node := range state.Nodes {
		switch node.GroupID {
		case groupAID:
			groupANodes = append(groupANodes, node)
		case groupBID:
			groupBNodes = append(groupBNodes, node)
		}
	}
	if len(groupANodes) != 1 || groupANodes[0].Name != "new-a" {
		t.Fatalf("group A nodes = %#v, want only new-a", groupANodes)
	}
	if len(groupBNodes) != 1 || groupBNodes[0].ID != "old-b" {
		t.Fatalf("group B nodes = %#v, want old-b untouched", groupBNodes)
	}
	if state.SelectedID != groupANodes[0].ID {
		t.Fatalf("selected = %q, want %q", state.SelectedID, groupANodes[0].ID)
	}
}

func TestUpdateSubscriptionGroupRejectsManualGroup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}
	if _, err := app.UpdateSubscriptionGroup(defaultGroupID); err == nil {
		t.Fatal("expected manual group update to fail")
	}
}

func TestUpdateSubscriptionGroupWithURLSavesNewURL(t *testing.T) {
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies: []`))
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies:
  - name: changed-url-node
    type: trojan
    server: changed.example.com
    port: 443
    password: pass-a
`))
	}))
	defer newServer.Close()

	groupID := "sub-group"
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "Default", Source: "manual"},
		{ID: groupID, Name: "Sub", Source: "subscription", URL: oldServer.URL},
	}
	nodes := []Node{{ID: "old", Name: "old", Type: "trojan", Address: "old.example.com", Port: 443, GroupID: groupID}}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, ""); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}

	if _, err := app.UpdateSubscriptionGroupWithURL(groupID, newServer.URL); err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if state.Groups[1].URL != newServer.URL {
		t.Fatalf("订阅链接未保存，实际：%s", state.Groups[1].URL)
	}
	if len(state.Nodes) != 1 || state.Nodes[0].Name != "changed-url-node" {
		t.Fatalf("节点未使用新订阅更新：%#v", state.Nodes)
	}
}

func TestUpdateSubscriptionGroupDoesNotAutoSelectWhenEmpty(t *testing.T) {
	body := `proxies:
  - name: new-a
    type: trojan
    server: new-a.example.com
    port: 443
    password: pass-a
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	groupID := groupIDFromName(server.URL + "|Group A")
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "Default", Source: "manual"},
		{ID: groupID, Name: "Group A", Source: "subscription", URL: server.URL},
	}
	nodes := []Node{
		{ID: "old-a", Name: "old-a", Type: "trojan", Address: "old-a.example.com", Port: 443, GroupID: groupID},
	}
	if err := store.ReplaceGroupsAndNodes(groups, nodes, ""); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}

	if _, err := app.UpdateSubscriptionGroup(groupID); err != nil {
		t.Fatal(err)
	}
	if selected := store.Snapshot().SelectedID; selected != "" {
		t.Fatalf("空选中状态更新订阅后不应自动选中节点：%s", selected)
	}
}

func TestImportSubscriptionDoesNotAutoSelectFirstNode(t *testing.T) {
	body := `proxies:
  - name: first-sub-node
    type: trojan
    server: first.example.com
    port: 443
    password: pass-a
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, logger: NewLogger(filepath.Join(t.TempDir(), "app.log"))}
	if _, err := app.ImportSubscription(server.URL); err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if len(state.Nodes) != 1 {
		t.Fatalf("订阅导入节点数量错误：%d", len(state.Nodes))
	}
	if state.SelectedID != "" {
		t.Fatalf("首次订阅导入后不应自动选中节点：%s", state.SelectedID)
	}
}
