package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeGroupID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", defaultGroupID},
		{"   ", defaultGroupID},
		{"default", "default"},
		{"group-1", "group-1"},
		{"  group-2  ", "group-2"},
	}

	for _, tt := range tests {
		got := normalizeGroupID(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeGroupID(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestFilterAndRankCandidatesForAutoFailover_StrictSameGroup(t *testing.T) {
	groups := []NodeGroup{
		{ID: "group-A", Name: "香港专线"},
		{ID: "group-B", Name: "日本专线"},
	}

	nodes := []Node{
		{ID: "n1", Name: "香港-1", GroupID: "group-A", DelayMS: -1}, // 断网当前节点
		{ID: "n2", Name: "香港-2", GroupID: "group-A", DelayMS: 200},
		{ID: "n3", Name: "香港-3", GroupID: "group-A", DelayMS: 150},
		{ID: "n4", Name: "日本-1", GroupID: "group-B", DelayMS: 30}, // 延迟极低但属于其他分组！
		{ID: "n5", Name: "日本-2", GroupID: "group-B", DelayMS: 40},
	}

	candidates, groupID, groupName, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if groupID != "group-A" {
		t.Errorf("expected groupID 'group-A', got %q", groupID)
	}
	if groupName != "香港专线" {
		t.Errorf("expected groupName '香港专线', got %q", groupName)
	}

	// 候选必须严格且只包含同分组 group-A 节点
	if len(candidates) != 2 {
		t.Fatalf("expected exactly 2 candidates from group-A, got %d", len(candidates))
	}

	for _, c := range candidates {
		if c.GroupID != "group-A" {
			t.Errorf("candidate %s belongs to %s, violation: candidate must strictly be in group-A", c.Name, c.GroupID)
		}
		if c.ID == "n1" {
			t.Errorf("candidate must not include the current failing node n1")
		}
	}

	// 历史延迟升序：香港-3 (150ms) 必须排在 香港-2 (200ms) 前面
	if candidates[0].ID != "n3" || candidates[1].ID != "n2" {
		t.Errorf("candidates order incorrect: got [%s, %s], expected [n3, n2]", candidates[0].ID, candidates[1].ID)
	}
}

func TestFilterAndRankCandidatesForAutoFailover_NoCrossGroupFallback(t *testing.T) {
	// 当同分组下没有其他节点时，绝不切换到其他分组，必须返回同分组错误
	groups := []NodeGroup{
		{ID: "solo-group", Name: "单节点分组"},
		{ID: "other-group", Name: "其他可用分组"},
	}

	nodes := []Node{
		{ID: "solo-1", Name: "孤立节点", GroupID: "solo-group"},
		{ID: "other-1", Name: "其他节点1", GroupID: "other-group", DelayMS: 50},
		{ID: "other-2", Name: "其他节点2", GroupID: "other-group", DelayMS: 60},
	}

	candidates, _, _, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "solo-1")
	if err == nil {
		t.Fatalf("expected error when no candidates in same group, but got %d candidates: %v", len(candidates), candidates)
	}

	if !strings.Contains(err.Error(), "单节点分组") || !strings.Contains(err.Error(), "只在同分组内切换") {
		t.Errorf("error message should explain restricted to same group, got: %v", err)
	}
}

func TestFilterAndRankCandidatesForAutoFailover_HistoricalSorting(t *testing.T) {
	groups := []NodeGroup{
		{ID: "g1", Name: "测试组"},
	}

	nodes := []Node{
		{ID: "curr", Name: "当前", GroupID: "g1"},
		{ID: "n_high", Name: "高延迟", GroupID: "g1", DelayMS: 800},
		{ID: "n_failed", Name: "失败节点", GroupID: "g1", DelayMS: -1},
		{ID: "n_lowest", Name: "最低延迟", GroupID: "g1", DelayMS: 50},
		{ID: "n_untested", Name: "未测节点", GroupID: "g1", DelayMS: 0},
		{ID: "n_medium", Name: "中延迟", GroupID: "g1", DelayMS: 180},
	}

	candidates, _, _, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "curr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedOrder := []string{"n_lowest", "n_medium", "n_high", "n_untested", "n_failed"}
	if len(candidates) != len(expectedOrder) {
		t.Fatalf("expected %d candidates, got %d", len(expectedOrder), len(candidates))
	}

	for i, expID := range expectedOrder {
		if candidates[i].ID != expID {
			t.Errorf("candidate at index %d: expected %s, got %s", i, expID, candidates[i].ID)
		}
	}
}

func TestFilterAndRankCandidatesForAutoFailover_DefaultGroupNormalization(t *testing.T) {
	groups := []NodeGroup{
		{ID: defaultGroupID, Name: "默认"},
	}

	nodes := []Node{
		{ID: "n1", Name: "默认节点1", GroupID: ""}, // empty GroupID should normalize to defaultGroupID
		{ID: "n2", Name: "默认节点2", GroupID: "default"},
		{ID: "n3", Name: "默认节点3", GroupID: "   "},
		{ID: "other", Name: "其他节点", GroupID: "custom"},
	}

	candidates, groupID, groupName, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if groupID != defaultGroupID {
		t.Errorf("expected groupID 'default', got %q", groupID)
	}
	if groupName != "默认" {
		t.Errorf("expected groupName '默认', got %q", groupName)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates in default group, got %d", len(candidates))
	}

	for _, c := range candidates {
		if c.ID != "n2" && c.ID != "n3" {
			t.Errorf("unexpected candidate %s", c.ID)
		}
	}
}

func TestFilterAndRankCandidatesForAutoFailover_CapAt16(t *testing.T) {
	groups := []NodeGroup{{ID: "big-group", Name: "大分组"}}
	nodes := []Node{{ID: "curr", GroupID: "big-group"}}
	for i := 1; i <= 25; i++ {
		nodes = append(nodes, Node{
			ID:      fmt.Sprintf("n%d", i),
			Name:    fmt.Sprintf("Node-%d", i),
			GroupID: "big-group",
			DelayMS: i * 10,
		})
	}

	candidates, _, _, err := filterAndRankCandidatesForAutoFailover(nodes, groups, "curr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidates) != 16 {
		t.Fatalf("expected candidates to be capped at 16, got %d", len(candidates))
	}

	// Should be the lowest 16 (10ms to 160ms)
	if candidates[0].DelayMS != 10 || candidates[15].DelayMS != 160 {
		t.Errorf("expected top 16 lowest delay candidates, got [%d...%d]", candidates[0].DelayMS, candidates[15].DelayMS)
	}
}
