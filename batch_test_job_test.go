package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 测试辅助：假内核与事件记录器
// ---------------------------------------------------------------------------

type fakeBatchCore struct {
	ports   []int
	closed  int32
	onClose func()
}

func (c *fakeBatchCore) Port(offset int) int {
	if offset < 0 || offset >= len(c.ports) {
		return 0
	}
	return c.ports[offset]
}

func (c *fakeBatchCore) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) && c.onClose != nil {
		c.onClose()
	}
	return nil
}

type batchEventRecorder struct {
	mu      sync.Mutex
	start   time.Time
	results []BatchTestResultEvent
	times   []time.Duration
	done    []BatchTestDoneEvent
}

func newBatchEventRecorder() *batchEventRecorder {
	return &batchEventRecorder{start: time.Now()}
}

func (r *batchEventRecorder) onResult(event BatchTestResultEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, event)
	r.times = append(r.times, time.Since(r.start))
}

func (r *batchEventRecorder) onDone(event BatchTestDoneEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = append(r.done, event)
}

func (r *batchEventRecorder) snapshot() ([]BatchTestResultEvent, []time.Duration, []BatchTestDoneEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]BatchTestResultEvent(nil), r.results...), append([]time.Duration(nil), r.times...), append([]BatchTestDoneEvent(nil), r.done...)
}

func makeBatchItems(count int) []batchNodeTestItem {
	items := make([]batchNodeTestItem, count)
	for i := range items {
		node := Node{
			ID:       fmt.Sprintf("node-%d", i),
			Name:     fmt.Sprintf("节点-%d", i),
			Type:     "vmess",
			Address:  "1.2.3.4",
			Port:     443,
			Outbound: map[string]interface{}{"type": "vmess"},
		}
		items[i] = batchNodeTestItem{index: i, node: node, options: SpeedTestOptions{PreferCore: true}}
	}
	return items
}

// fakeCoreDeps 返回一组使用假内核的依赖：节点名 → 端口 = 10000 + 原始序号，便于 delay 假函数识别节点。
func fakeCoreDeps(rec *batchEventRecorder, delayFn func(port int) (int, error)) (batchTestDeps, *int32, *int32) {
	var opened, maxOpen int32
	var current int32
	deps := batchTestDeps{
		workers: 64,
		startCore: func(ctx context.Context, nodes []Node) (tempCoreInstance, error) {
			ports := make([]int, len(nodes))
			for i, node := range nodes {
				var idx int
				fmt.Sscanf(node.ID, "node-%d", &idx)
				ports[i] = 10000 + idx
			}
			atomic.AddInt32(&opened, 1)
			now := atomic.AddInt32(&current, 1)
			for {
				prev := atomic.LoadInt32(&maxOpen)
				if now <= prev || atomic.CompareAndSwapInt32(&maxOpen, prev, now) {
					break
				}
			}
			return &fakeBatchCore{ports: ports, onClose: func() { atomic.AddInt32(&current, -1) }}, nil
		},
		clientForPort: func(port int) (*http.Client, error) {
			// 用 Timeout 字段携带端口，让 delay 假函数知道是哪个节点
			return &http.Client{Timeout: time.Duration(port)}, nil
		},
		delay: func(ctx context.Context, client *http.Client, base SpeedResult) (SpeedResult, error) {
			ms, err := delayFn(int(client.Timeout))
			if err != nil {
				return base, err
			}
			base.URLDelayMS = ms
			base.DelayMS = ms
			return base, nil
		},
		outboundInfo: func(ctx context.Context, client *http.Client) (string, string, error) {
			return fmt.Sprintf("8.8.%d.%d", int(client.Timeout)/256%256, int(client.Timeout)%256), "US", nil
		},
		emitResult: rec.onResult,
		emitDone:   rec.onDone,
	}
	return deps, &opened, &maxOpen
}

// ---------------------------------------------------------------------------
// 1. 无屏障：一个慢节点不能拖住其他节点的结果
// ---------------------------------------------------------------------------

func TestBatchJobNoBarrierSlowNodeDoesNotBlockOthers(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) {
		if port == 10000 {
			time.Sleep(1500 * time.Millisecond)
			return 1500, nil
		}
		time.Sleep(10 * time.Millisecond)
		return 10, nil
	})
	runner := newBatchTestJobRunnerWithDeps("job-1", 200, makeBatchItems(200), "delay", false, 0, deps)
	runner.Run()

	results, times, done := rec.snapshot()
	if len(done) != 1 || done[0].Cancelled || done[0].OK != 200 {
		t.Fatalf("done 事件不正确：%+v", done)
	}
	fastLate := 0
	for i, event := range results {
		if event.Index != 0 && times[i] > time.Second {
			fastLate++
		}
	}
	if fastLate > 0 {
		t.Fatalf("有 %d 个普通节点的结果超过 1 秒才发出，说明存在批次屏障", fastLate)
	}
}

// ---------------------------------------------------------------------------
// 2. WithIP：成功节点依次收到 delay 和 ip；失败节点只有 delay（带 error）
// ---------------------------------------------------------------------------

func TestBatchJobWithIPSequencing(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) {
		if (port-10000)%2 == 1 {
			return 0, errors.New("连接超时")
		}
		return 50, nil
	})
	runner := newBatchTestJobRunnerWithDeps("job-ip", 20, makeBatchItems(20), "delay", true, 0, deps)
	runner.Run()

	results, _, done := rec.snapshot()
	phases := make(map[int][]string)
	for _, event := range results {
		phase := event.Phase
		if event.Error != "" {
			phase += "!"
		}
		phases[event.Index] = append(phases[event.Index], phase)
	}
	for i := 0; i < 20; i++ {
		got := strings.Join(phases[i], ",")
		want := "delay,ip"
		if i%2 == 1 {
			want = "delay!"
		}
		if got != want {
			t.Fatalf("节点 %d 的事件顺序 = %q，期望 %q", i, got, want)
		}
	}
	if done[0].OK != 10 || done[0].Failed != 10 {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}

// ---------------------------------------------------------------------------
// 3. 取消：之后不再有 result 事件；done{cancelled:true}；内核被关闭
// ---------------------------------------------------------------------------

func TestBatchJobCancelStopsEventsAndClosesCores(t *testing.T) {
	rec := newBatchEventRecorder()
	release := make(chan struct{})
	deps, opened, _ := fakeCoreDeps(rec, func(port int) (int, error) {
		if port < 10010 {
			return 5, nil
		}
		<-release
		return 5, nil
	})
	var closedCount int32
	startCore := deps.startCore
	deps.startCore = func(ctx context.Context, nodes []Node) (tempCoreInstance, error) {
		core, err := startCore(ctx, nodes)
		if err != nil {
			return nil, err
		}
		fake := core.(*fakeBatchCore)
		previous := fake.onClose
		fake.onClose = func() {
			atomic.AddInt32(&closedCount, 1)
			previous()
		}
		return fake, nil
	}
	runner := newBatchTestJobRunnerWithDeps("job-cancel", 100, makeBatchItems(100), "delay", false, 0, deps)
	finished := make(chan struct{})
	go func() {
		runner.Run()
		close(finished)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		results, _, _ := rec.snapshot()
		if len(results) >= 10 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	runner.Cancel()
	resultsAtCancel, _, _ := rec.snapshot()
	countAtCancel := len(resultsAtCancel)
	close(release) // 放行被卡住的 worker，它们的结果必须被丢弃

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("取消后任务没有在 3 秒内结束")
	}
	results, _, done := rec.snapshot()
	if len(results) != countAtCancel {
		t.Fatalf("取消后仍有 result 事件：取消时 %d 个，结束时 %d 个", countAtCancel, len(results))
	}
	if len(done) != 1 || !done[0].Cancelled {
		t.Fatalf("应收到一次 done{cancelled:true}，实际：%+v", done)
	}
	if atomic.LoadInt32(opened) == 0 || atomic.LoadInt32(&closedCount) != atomic.LoadInt32(opened) {
		t.Fatalf("所有临时内核都应被关闭：打开 %d 个，关闭 %d 个", atomic.LoadInt32(opened), atomic.LoadInt32(&closedCount))
	}
}

// ---------------------------------------------------------------------------
// 4. 最多同时 2 个临时内核，并且全部在结束时关闭
// ---------------------------------------------------------------------------

func TestBatchJobAtMostTwoCoresAndAllClosed(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, opened, maxOpen := fakeCoreDeps(rec, func(port int) (int, error) {
		time.Sleep(time.Millisecond)
		return 20, nil
	})
	total := temporaryCoreDelayTestBatchSize*2 + 100 // 3 组
	runner := newBatchTestJobRunnerWithDeps("job-cores", total, makeBatchItems(total), "delay", false, 0, deps)
	runner.Run()

	results, _, done := rec.snapshot()
	if len(results) != total || done[0].OK != total {
		t.Fatalf("结果数量 = %d，done = %+v，期望 %d", len(results), done, total)
	}
	if got := atomic.LoadInt32(opened); got != 3 {
		t.Fatalf("应启动 3 个临时内核，实际 %d", got)
	}
	if got := atomic.LoadInt32(maxOpen); got > batchCoreMaxConcurrent {
		t.Fatalf("同时运行的临时内核最多 %d 个，实际 %d", batchCoreMaxConcurrent, got)
	}
	runner.coresMu.Lock()
	remaining := len(runner.activeCores)
	runner.coresMu.Unlock()
	if remaining != 0 {
		t.Fatalf("任务结束后仍有 %d 个临时内核未关闭", remaining)
	}
}

// ---------------------------------------------------------------------------
// 5. 解析 sing-box 错误输出
// ---------------------------------------------------------------------------

func TestParseFailingConfigEntry(t *testing.T) {
	cases := []struct {
		output  string
		kind    string
		index   int
		ok      bool
		summary string
	}{
		{
			output:  "FATAL[0000] decode config at C:\\data\\delay-batch-1.json: outbounds[3].uuid: invalid UUID length: 5",
			kind:    "outbound",
			index:   3,
			ok:      true,
			summary: "uuid: invalid UUID length: 5",
		},
		{
			output:  "+0800 2026-11-15 00:10:00 FATAL[0000] start service: initialize outbound[12]: parse reality public key: illegal base64 data at input byte 4",
			kind:    "outbound",
			index:   12,
			ok:      true,
			summary: "parse reality public key: illegal base64 data at input byte 4",
		},
		{
			output:  "FATAL[0000] start service: initialize endpoint[0]: parse private key: wrong key length",
			kind:    "endpoint",
			index:   0,
			ok:      true,
			summary: "parse private key: wrong key length",
		},
		{
			// 真实样例：sing-box 1.14.4 对私钥错误的 WireGuard endpoint 执行 check 的原始输出（含颜色控制字符）
			output:  "\x1b[31mFATAL\x1b[0m[0000] initialize endpoint[0]: decode private key: illegal base64 data at input byte 4",
			kind:    "endpoint",
			index:   0,
			ok:      true,
			summary: "decode private key: illegal base64 data at input byte 4",
		},
		{
			output: "FATAL[0000] start service: listen tcp 127.0.0.1:31001: bind: address already in use",
			ok:     false,
		},
	}
	for _, tc := range cases {
		kind, index, ok := parseFailingConfigEntry(tc.output)
		if ok != tc.ok || (ok && (kind != tc.kind || index != tc.index)) {
			t.Fatalf("parseFailingConfigEntry(%q) = (%q, %d, %v)，期望 (%q, %d, %v)", tc.output, kind, index, ok, tc.kind, tc.index, tc.ok)
		}
		if tc.ok {
			if summary := parseFailingOutboundErrorSummary(tc.output); summary != tc.summary {
				t.Fatalf("摘要 = %q，期望 %q", summary, tc.summary)
			}
		}
	}
	if idx, ok := parseFailingOutboundIndex("initialize endpoint[2]: x"); ok || idx != -1 {
		t.Fatalf("endpoint 错误不应被当作 outbound 序号：%d %v", idx, ok)
	}
}

// ---------------------------------------------------------------------------
// 6. 坏节点隔离：剔除出错节点后重试，其他节点正常出结果
// ---------------------------------------------------------------------------

func TestBatchJobIsolatesBadNodeAndRetries(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) { return 30, nil })
	badIDs := map[string]bool{"node-3": true, "node-7": true}
	var attempts int32
	startCore := deps.startCore
	deps.startCore = func(ctx context.Context, nodes []Node) (tempCoreInstance, error) {
		atomic.AddInt32(&attempts, 1)
		owners := batchConfigOwners{Outbounds: []int{-1}}
		for i := range nodes {
			owners.Outbounds = append(owners.Outbounds, i)
		}
		for i, node := range nodes {
			if badIDs[node.ID] {
				return nil, &batchCoreConfigError{
					Output: fmt.Sprintf("FATAL[0000] decode config at x.json: outbounds[%d].uuid: invalid UUID length: 5", i+1),
					Owners: owners,
				}
			}
		}
		return startCore(ctx, nodes)
	}
	runner := newBatchTestJobRunnerWithDeps("job-bad", 10, makeBatchItems(10), "delay", false, 0, deps)
	runner.Run()

	results, _, done := rec.snapshot()
	byIndex := make(map[int]BatchTestResultEvent)
	for _, event := range results {
		byIndex[event.Index] = event
	}
	for i := 0; i < 10; i++ {
		event, ok := byIndex[i]
		if !ok {
			t.Fatalf("节点 %d 没有结果", i)
		}
		isBad := i == 3 || i == 7
		if isBad && !strings.Contains(event.Error, "节点配置无效") {
			t.Fatalf("坏节点 %d 应标记为配置无效，实际：%+v", i, event)
		}
		if !isBad && event.Error != "" {
			t.Fatalf("正常节点 %d 不应失败，实际：%+v", i, event)
		}
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("应尝试启动 3 次（剔除 2 个坏节点），实际 %d 次", got)
	}
	if done[0].OK != 8 || done[0].Failed != 2 {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}

// 6b. 重试用尽后，该组剩余节点改用兼容模式
func TestBatchJobFallsBackToCompatAfterRetryLimit(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) { return 30, nil })
	deps.startCore = func(ctx context.Context, nodes []Node) (tempCoreInstance, error) {
		owners := batchConfigOwners{Outbounds: []int{-1}}
		for i := range nodes {
			owners.Outbounds = append(owners.Outbounds, i)
		}
		return nil, &batchCoreConfigError{Output: "FATAL[0000] decode config: outbounds[1].uuid: bad", Owners: owners}
	}
	var compatCalls int32
	deps.compat = func(ctx context.Context, item batchNodeTestItem, metric string) (SpeedResult, error) {
		atomic.AddInt32(&compatCalls, 1)
		result := batchBaseResult(item.node)
		result.URLDelayMS = 99
		result.DelayMS = 99
		return result, nil
	}
	runner := newBatchTestJobRunnerWithDeps("job-compat", 10, makeBatchItems(10), "delay", false, 0, deps)
	runner.Run()

	_, _, done := rec.snapshot()
	// 每次剔除第 1 个节点，共剔除 batchCoreConfigRetryLimit 个，其余走兼容模式
	wantCompat := int32(10 - batchCoreConfigRetryLimit)
	if got := atomic.LoadInt32(&compatCalls); got != wantCompat {
		t.Fatalf("兼容模式调用次数 = %d，期望 %d", got, wantCompat)
	}
	if done[0].OK != int(wantCompat) || done[0].Failed != batchCoreConfigRetryLimit {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}

// ---------------------------------------------------------------------------
// 7. Ping + 出口 IP：只对 Ping 成功的节点补查 IP
// ---------------------------------------------------------------------------

func TestBatchJobPingThenIPForSuccessfulNodes(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) {
		t.Errorf("ping 模式不应进行 URL 延迟测试")
		return 0, nil
	})
	deps.ping = func(ctx context.Context, item batchNodeTestItem) (SpeedResult, error) {
		result := batchBaseResult(item.node)
		if item.index%3 == 0 {
			result.PingMS = -1
			return result, errors.New("ping 超时")
		}
		result.PingMS = 40
		result.DelayMS = 40
		return result, nil
	}
	runner := newBatchTestJobRunnerWithDeps("job-ping", 9, makeBatchItems(9), "ping", true, 0, deps)
	runner.Run()

	results, _, done := rec.snapshot()
	ipEvents := 0
	for _, event := range results {
		if event.Phase == "ip" {
			ipEvents++
			if event.Index%3 == 0 {
				t.Fatalf("Ping 失败的节点 %d 不应补查 IP", event.Index)
			}
			if event.Result.OutboundIP == "" {
				t.Fatalf("节点 %d 的 IP 事件缺少出口 IP", event.Index)
			}
		}
	}
	if ipEvents != 6 {
		t.Fatalf("应有 6 个 IP 事件，实际 %d", ipEvents)
	}
	if done[0].OK != 6 || done[0].Failed != 3 {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}

// ---------------------------------------------------------------------------
// 8. 原生客户端节点（PreferCore=false）不启动临时内核
// ---------------------------------------------------------------------------

func TestBatchJobNativeNodesSkipTemporaryCore(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, opened, _ := fakeCoreDeps(rec, func(port int) (int, error) { return 15, nil })
	deps.nativeClient = func(item batchNodeTestItem) (*http.Client, bool, error) {
		return &http.Client{Timeout: time.Duration(20000 + item.index)}, true, nil
	}
	items := makeBatchItems(5)
	for i := range items {
		items[i].options.PreferCore = false
	}
	runner := newBatchTestJobRunnerWithDeps("job-native", 5, items, "delay", false, 0, deps)
	runner.Run()

	_, _, done := rec.snapshot()
	if got := atomic.LoadInt32(opened); got != 0 {
		t.Fatalf("原生客户端节点不应启动临时内核，实际启动 %d 个", got)
	}
	if done[0].OK != 5 {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}

// ---------------------------------------------------------------------------
// 9. 初始失败数（解析失败的节点）计入 done
// ---------------------------------------------------------------------------

func TestBatchJobInitialFailedCountedInDone(t *testing.T) {
	rec := newBatchEventRecorder()
	deps, _, _ := fakeCoreDeps(rec, func(port int) (int, error) { return 15, nil })
	runner := newBatchTestJobRunnerWithDeps("job-init", 7, makeBatchItems(5), "delay", false, 2, deps)
	runner.Run()
	_, _, done := rec.snapshot()
	if done[0].Total != 7 || done[0].OK != 5 || done[0].Failed != 2 {
		t.Fatalf("done 计数不正确：%+v", done[0])
	}
}
