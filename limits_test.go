package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestManualNodeInputRejectsOversizeContent(t *testing.T) {
	text := strings.Repeat("x", maxManualNodeInputBytes+1)
	if err := validateManualNodeInput(text); err == nil {
		t.Fatal("超大节点内容未被拒绝")
	}
}

func TestParseTestNodesRejectsTooManyNodes(t *testing.T) {
	lines := make([]string, maxBatchTestNodes+1)
	for index := range lines {
		lines[index] = fmt.Sprintf("socks5://127.0.0.1:%d#node-%d", 10000+index, index)
	}
	if _, err := (&App{}).ParseTestNodes(strings.Join(lines, "\n")); err == nil || !strings.Contains(err.Error(), "最多支持") {
		t.Fatalf("过多批量测试节点未被拒绝：%v", err)
	}
}

func TestTestRawNodesRejectsTooManyNodesBeforeStarting(t *testing.T) {
	if _, err := (&App{}).TestRawNodes(make([]string, maxBatchTestNodes+1), "ping", SpeedTestOptions{}); err == nil || !strings.Contains(err.Error(), "最多支持") {
		t.Fatalf("过多批量测试任务未被拒绝：%v", err)
	}
}

func TestStoredNodesRejectsTooManyNodesBeforeStarting(t *testing.T) {
	ids := make([]string, maxBatchTestNodes+1)
	for index := range ids {
		ids[index] = fmt.Sprintf("node-%d", index)
	}
	app := &App{store: &Store{}, manager: &SingBoxManager{}}
	if _, err := app.TestNodesWithMode(ids, "tcp"); err == nil || !strings.Contains(err.Error(), "最多支持") {
		t.Fatalf("过多已保存节点测试任务未被拒绝：%v", err)
	}
}
