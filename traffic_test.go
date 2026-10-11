package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadTrafficTotalsPrefersSummaryTotals(t *testing.T) {
	var total TrafficSnapshot
	body := `{"uploadTotal":1000,"downloadTotal":2000,"connections":[{"upload":1,"download":2}]}`
	if err := readTrafficTotals(json.NewDecoder(strings.NewReader(body)), &total); err != nil {
		t.Fatalf("解析流量失败：%v", err)
	}
	if total.UploadTotal != 1000 || total.DownloadTotal != 2000 {
		t.Fatalf("应优先使用总流量字段，实际：%+v", total)
	}
}

func TestReadTrafficTotalsFallsBackToConnectionSum(t *testing.T) {
	var total TrafficSnapshot
	body := `{"connections":[{"upload":100,"download":200},{"upload":3,"download":4}]}`
	if err := readTrafficTotals(json.NewDecoder(strings.NewReader(body)), &total); err != nil {
		t.Fatalf("解析流量失败：%v", err)
	}
	if total.UploadTotal != 103 || total.DownloadTotal != 204 {
		t.Fatalf("缺少总流量字段时应累加连接列表，实际：%+v", total)
	}
}
