package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func benchmarkNodes(count int) []Node {
	nodes := make([]Node, count)
	for index := range nodes {
		id := fmt.Sprintf("stress-%05d", index)
		host := fmt.Sprintf("192.0.2.%d", index%250+1)
		nodes[index] = Node{
			ID:      id,
			Name:    "压力测试节点 " + id,
			Type:    "socks5",
			Address: host,
			Port:    10000 + index%50000,
			GroupID: defaultGroupID,
			Raw:     fmt.Sprintf("socks5://user:pass@%s:%d#%s", host, 10000+index%50000, id),
			Outbound: map[string]interface{}{
				"type":        "socks",
				"server":      host,
				"server_port": 10000 + index%50000,
				"username":    "user",
				"password":    "pass",
			},
		}
	}
	return nodes
}

func benchmarkState(count int) PersistState {
	nodes := benchmarkNodes(count)
	return PersistState{
		Nodes:      nodes,
		Groups:     defaultGroups(),
		SelectedID: nodes[0].ID,
		Settings:   defaultSettings(),
	}
}

func BenchmarkStoreSaveLoad5000(b *testing.B) {
	state := benchmarkState(5000)
	dir := b.TempDir()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		path := filepath.Join(dir, fmt.Sprintf("state-%d.dat", index))
		store, err := NewStore(path)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.ReplaceGroupsAndNodes(state.Groups, state.Nodes, state.SelectedID); err != nil {
			b.Fatal(err)
		}
		reloaded, err := NewStore(path)
		if err != nil {
			b.Fatal(err)
		}
		if len(reloaded.Snapshot().Nodes) != len(state.Nodes) {
			b.Fatal("节点数量不一致")
		}
	}
}

func BenchmarkBuildConfig5000(b *testing.B) {
	state := benchmarkState(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		config, err := BuildSingBoxStartupConfig(state, state.Nodes[0], "cache.db")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(config); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateSummary5000(b *testing.B) {
	nodes := benchmarkNodes(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result := nodesForState(nodes)
		if len(result) != len(nodes) {
			b.Fatal("节点数量不一致")
		}
	}
}

func BenchmarkShareLinks5000(b *testing.B) {
	nodes := benchmarkNodes(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		for _, node := range nodes {
			if _, err := nodeShareLink(node); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkExportXLSXWithQR1000(b *testing.B) {
	nodes := benchmarkNodes(1000)
	rows := make([]exportLinkRow, len(nodes))
	for index, node := range nodes {
		link, err := nodeShareLink(node)
		if err != nil {
			b.Fatal(err)
		}
		rows[index] = exportLinkRow{Name: node.Name, Link: link}
	}
	dir := b.TempDir()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := writeGroupLinksXLSXWithQR(filepath.Join(dir, fmt.Sprintf("nodes-%d.xlsx", index)), rows); err != nil {
			b.Fatal(err)
		}
	}
}
