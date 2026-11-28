package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// 批量测试任务（T04）
//
// 设计要点：
//   - 一个全局 worker 池，没有批次屏障：测完一个节点立刻推送结果并取下一个；
//   - 需要临时内核的节点每 300 个一组、一组一个内核，最多同时运行 2 个内核，
//     当前组快测完时预先启动下一组；
//   - WithIP 时同一个 worker 在延迟成功后立刻用同一个端口查出口 IP，不再有第二轮；
//   - 取消时立即关闭所有临时内核，不再推送结果，最后推送一次 done{cancelled:true}；
//   - 临时内核因某个节点配置无效而启动失败时，定位并剔除该节点后重试（最多 3 次）。
// ---------------------------------------------------------------------------

const (
	batchCoreMaxConcurrent    = 2  // 同时运行的临时内核数量上限
	batchCorePrefetchRemain   = 50 // 当前组剩余节点数不超过该值时，预先启动下一组内核
	batchCoreConfigRetryLimit = 3  // 剔除坏节点后重试启动的次数上限
	batchCompatConcurrency    = temporaryCoreDelayTestMaxConcurrency
	batchCoreCloseWait        = 2 * time.Second
	batchCoreStderrLimit      = 64 * 1024
	batchErrorSummaryMaxRunes = 160
)

var failingConfigEntryRegexp = regexp.MustCompile(`(?i)\b(outbound|endpoint)s?\[(\d+)\]`)

// parseFailingConfigEntry 从 sing-box 的错误输出中解析出出错条目的类型（"outbound" / "endpoint"）和序号。
// 兼容 "decode config at x.json: outbounds[3].uuid: ..." 与 "initialize outbound[3]: ..." 两种格式。
func parseFailingConfigEntry(output string) (string, int, bool) {
	matches := failingConfigEntryRegexp.FindStringSubmatch(output)
	if len(matches) < 3 {
		return "", -1, false
	}
	index, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", -1, false
	}
	return strings.ToLower(matches[1]), index, true
}

// parseFailingOutboundIndex 只返回 outbound 的序号（保留给旧调用方和测试使用）。
func parseFailingOutboundIndex(output string) (int, bool) {
	kind, index, ok := parseFailingConfigEntry(output)
	if !ok || kind != "outbound" {
		return -1, false
	}
	return index, true
}

// parseFailingOutboundErrorSummary 提取给用户看的错误摘要，去掉 "outbounds[3]" 这类对用户无意义的前缀。
func parseFailingOutboundErrorSummary(output string) string {
	for _, line := range strings.Split(stripANSI(output), "\n") {
		line = strings.TrimSpace(line)
		loc := failingConfigEntryRegexp.FindStringIndex(line)
		if loc == nil {
			continue
		}
		rest := strings.TrimLeft(line[loc[1]:], ".: ")
		if rest == "" {
			rest = line
		}
		return truncateRunes(rest, batchErrorSummaryMaxRunes)
	}
	return "配置校验失败"
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

// ---------------------------------------------------------------------------
// 对外类型
// ---------------------------------------------------------------------------

type BatchTestOptions struct {
	JobID            string `json:"jobId"`  // 可选：前端生成的任务 ID，便于前端在调用返回前就能识别事件
	Metric           string `json:"metric"` // "delay" 或 "ping"
	WithIP           bool   `json:"withIP"` // 延迟成功后，同一节点立即顺带查询出口 IP
	SpeedTestOptions        // 复用现有选项（超时、是否经系统代理等）
}

type BatchTestStartResult struct {
	JobID string `json:"jobId"`
	Total int    `json:"total"`
}

type BatchTestResultEvent struct {
	JobID  string      `json:"jobId"`
	Index  int         `json:"index"`
	Phase  string      `json:"phase"` // "delay" 或 "ip"
	Result SpeedResult `json:"result"`
	Error  string      `json:"error"`
}

type BatchTestDoneEvent struct {
	JobID     string `json:"jobId"`
	Total     int    `json:"total"`
	OK        int    `json:"ok"`
	Failed    int    `json:"failed"`
	Cancelled bool   `json:"cancelled"`
}

// ---------------------------------------------------------------------------
// 内部类型
// ---------------------------------------------------------------------------

type batchNodeTestItem struct {
	index   int
	raw     string
	node    Node
	options SpeedTestOptions // 已经按当前设置修正过的选项（PreferCore 等）
}

// tempCoreInstance 是一个已启动、端口已就绪的临时 sing-box 内核。
type tempCoreInstance interface {
	Port(offset int) int
	Close() error
}

// batchCoreConfigError 表示临时内核因配置无效而启动失败，Output 为 sing-box 的错误输出。
type batchCoreConfigError struct {
	Output string
	Owners batchConfigOwners
}

func (e *batchCoreConfigError) Error() string {
	return "临时内核配置无效：" + trimForLog(e.Output)
}

// batchTestDeps 汇集任务对外部世界的全部依赖，测试时整体替换为假实现。
type batchTestDeps struct {
	workers       int
	ping          func(ctx context.Context, item batchNodeTestItem) (SpeedResult, error)
	nativeClient  func(item batchNodeTestItem) (*http.Client, bool, error)
	startCore     func(ctx context.Context, nodes []Node) (tempCoreInstance, error)
	clientForPort func(port int) (*http.Client, error)
	delay         func(ctx context.Context, client *http.Client, base SpeedResult) (SpeedResult, error)
	outboundInfo  func(ctx context.Context, client *http.Client) (string, string, error)
	compat        func(ctx context.Context, item batchNodeTestItem, metric string) (SpeedResult, error)
	emitResult    func(event BatchTestResultEvent)
	emitDone      func(event BatchTestDoneEvent)
	logInfo       func(format string, args ...any)
	logError      func(format string, args ...any)
}

type batchTestJobRunner struct {
	jobID  string
	total  int
	items  []batchNodeTestItem
	metric string
	withIP bool
	deps   batchTestDeps

	ctx       context.Context
	cancel    context.CancelFunc
	cancelled int32

	okCount     int32
	failedCount int32

	coresMu     sync.Mutex
	activeCores map[*batchCoreGroup]struct{}
}

func newBatchTestJobRunnerWithDeps(jobID string, total int, items []batchNodeTestItem, metric string, withIP bool, initialFailed int, deps batchTestDeps) *batchTestJobRunner {
	ctx, cancel := context.WithCancel(context.Background())
	metric = strings.ToLower(strings.TrimSpace(metric))
	if metric != "ping" && metric != "both" {
		metric = "delay"
	}
	if deps.workers <= 0 {
		deps.workers = batchNodeTestConcurrency(len(items))
	}
	return &batchTestJobRunner{
		jobID:       jobID,
		total:       total,
		items:       items,
		metric:      metric,
		withIP:      withIP,
		deps:        deps,
		ctx:         ctx,
		cancel:      cancel,
		failedCount: int32(initialFailed),
		activeCores: make(map[*batchCoreGroup]struct{}),
	}
}

// newBatchTestJobRunner 组装生产环境依赖。
func newBatchTestJobRunner(a *App, jobID string, total int, items []batchNodeTestItem, options BatchTestOptions, initialFailed int) *batchTestJobRunner {
	jobOptions := options.SpeedTestOptions
	if len(items) > 0 {
		jobOptions = items[0].options
	}
	delayRuntime := delayRuntimeFromOptions(jobOptions)
	var manager *SingBoxManager
	var logger *Logger
	if a != nil {
		manager = a.manager
		logger = a.logger
	}

	var directDNSOnce sync.Once
	directDNS := ""
	resolveDirectDNS := func() string {
		directDNSOnce.Do(func() {
			if resolveDirectDNSPlanFunc != nil {
				directDNS = resolveDirectDNSPlanFunc(logger).Direct.Server
			}
		})
		return directDNS
	}

	deps := batchTestDeps{
		ping: func(ctx context.Context, item batchNodeTestItem) (SpeedResult, error) {
			base := batchBaseResult(item.node)
			ping, err := testNodePing(item.node)
			if err != nil {
				base.PingMS = -1
				base.DelayMS = -1
				return base, err
			}
			base.PingMS = ping
			base.DelayMS = ping
			return base, nil
		},
		nativeClient: func(item batchNodeTestItem) (*http.Client, bool, error) {
			upstreamPort := 0
			if item.options.UseSystemProxy && manager != nil {
				upstreamPort = manager.getSOCKS5TestProxyPort()
			}
			return buildNativeProxyClientWithUpstreamTimeout(item.node, upstreamPort, nativeProxyTimeout(item.options))
		},
		startCore: func(ctx context.Context, nodes []Node) (tempCoreInstance, error) {
			return startBatchTempCore(ctx, manager, nodes, jobOptions, resolveDirectDNS())
		},
		clientForPort: func(port int) (*http.Client, error) {
			return buildLocalSOCKS5Client(port, delayRuntime.timeout)
		},
		delay: func(ctx context.Context, client *http.Client, base SpeedResult) (SpeedResult, error) {
			return testNodeMetricWithClient(base, client, "delay", nil, defaultSpeedTestConfig(), delayRuntime)
		},
		outboundInfo: func(ctx context.Context, client *http.Client) (string, string, error) {
			return testOutboundInfoContext(ctx, client)
		},
		compat: func(ctx context.Context, item batchNodeTestItem, metric string) (SpeedResult, error) {
			if manager == nil {
				return SpeedResult{}, errors.New("内核管理器未初始化")
			}
			return manager.TestNodeMetricWithProgressAndOptions(item.node, metric, nil, item.options)
		},
		emitResult: func(event BatchTestResultEvent) {
			if a != nil {
				a.emitBatchTestResult(event)
			}
		},
		emitDone: func(event BatchTestDoneEvent) {
			if a != nil {
				a.emitBatchTestDone(event)
			}
		},
		logInfo: func(format string, args ...any) {
			if logger != nil {
				logger.Info(format, args...)
			}
		},
		logError: func(format string, args ...any) {
			if logger != nil {
				logger.Error(format, args...)
			}
		},
	}
	return newBatchTestJobRunnerWithDeps(jobID, total, items, options.Metric, options.WithIP, initialFailed, deps)
}

func batchBaseResult(node Node) SpeedResult {
	return SpeedResult{
		ID:          node.ID,
		Name:        node.Name,
		Type:        node.Type,
		Address:     node.Address,
		Port:        node.Port,
		Country:     node.Country,
		PingAddress: node.Address,
	}
}

// Cancel 取消任务：停止派发、立即关闭所有临时内核，之后不再推送结果事件。
func (r *batchTestJobRunner) Cancel() {
	if atomic.CompareAndSwapInt32(&r.cancelled, 0, 1) {
		r.cancel()
		r.closeAllCores()
	}
}

func (r *batchTestJobRunner) stopped() bool {
	return atomic.LoadInt32(&r.cancelled) == 1 || r.ctx.Err() != nil
}

func (r *batchTestJobRunner) emit(event BatchTestResultEvent) {
	if r.stopped() || r.deps.emitResult == nil {
		return
	}
	event.JobID = r.jobID
	r.deps.emitResult(event)
}

func (r *batchTestJobRunner) logInfo(format string, args ...any) {
	if r.deps.logInfo != nil {
		r.deps.logInfo(format, args...)
	}
}

func (r *batchTestJobRunner) logError(format string, args ...any) {
	if r.deps.logError != nil {
		r.deps.logError(format, args...)
	}
}

// reportPrimaryFailure 推送某个节点第一阶段（ping 或 delay）的失败结果。
// ipOnly 阶段（ping 之后补查 IP）失败只推送 ip 事件，不计入失败数。
func (r *batchTestJobRunner) reportPrimaryFailure(item batchNodeTestItem, ipOnly bool, message string) {
	if r.stopped() {
		return
	}
	if ipOnly {
		result := batchBaseResult(item.node)
		result.OutboundIP = "未知"
		r.emit(BatchTestResultEvent{Index: item.index, Phase: "ip", Result: result, Error: message})
		return
	}
	atomic.AddInt32(&r.failedCount, 1)
	result := batchBaseResult(item.node)
	if r.metric == "ping" {
		result.PingMS = -1
	} else {
		result.URLDelayMS = -1
	}
	result.DelayMS = -1
	r.emit(BatchTestResultEvent{Index: item.index, Phase: "delay", Result: result, Error: message})
}

func (r *batchTestJobRunner) reportPrimarySuccess(item batchNodeTestItem, result SpeedResult) {
	if r.stopped() {
		return
	}
	atomic.AddInt32(&r.okCount, 1)
	r.emit(BatchTestResultEvent{Index: item.index, Phase: "delay", Result: result})
}

func (r *batchTestJobRunner) reportIP(item batchNodeTestItem, result SpeedResult, ip, country string, err error) {
	if r.stopped() {
		return
	}
	if err != nil {
		result.OutboundIP = "未知"
		r.emit(BatchTestResultEvent{Index: item.index, Phase: "ip", Result: result, Error: err.Error()})
		return
	}
	result.OutboundIP = ip
	if country != "" {
		result.Country = country
	}
	r.emit(BatchTestResultEvent{Index: item.index, Phase: "ip", Result: result})
}

// Run 执行任务，返回时一定已推送 done 事件。
func (r *batchTestJobRunner) Run() {
	defer func() {
		if r.deps.emitDone != nil {
			r.deps.emitDone(BatchTestDoneEvent{
				JobID:     r.jobID,
				Total:     r.total,
				OK:        int(atomic.LoadInt32(&r.okCount)),
				Failed:    int(atomic.LoadInt32(&r.failedCount)),
				Cancelled: r.stopped(),
			})
		}
		r.cancel()
		r.closeAllCores()
	}()
	if len(r.items) == 0 || r.stopped() {
		return
	}
	if r.metric == "ping" {
		succeeded := r.runPingStage()
		if r.withIP && len(succeeded) > 0 && !r.stopped() {
			r.runProxyStage(succeeded, true)
		}
		return
	}
	if r.metric == "both" {
		r.runPingStageForBoth()
		if !r.stopped() {
			r.runProxyStage(r.items, false)
		}
		return
	}
	r.runProxyStage(r.items, false)
}

// runPingStageForBoth 在同时测试 TCP 与 URL 延迟时先并发执行 TCP Ping，仅推送 ping 事件，由后续 URL 阶段统计总成败与出口 IP。
func (r *batchTestJobRunner) runPingStageForBoth() {
	jobs := make(chan batchNodeTestItem, len(r.items))
	for _, item := range r.items {
		jobs <- item
	}
	close(jobs)

	var wg sync.WaitGroup
	workers := r.deps.workers
	if workers > len(r.items) {
		workers = len(r.items)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if r.stopped() {
					return
				}
				result, err := r.deps.ping(r.ctx, item)
				if r.stopped() {
					return
				}
				if err != nil {
					failRes := batchBaseResult(item.node)
					failRes.PingMS = -1
					failRes.DelayMS = -1
					r.emit(BatchTestResultEvent{Index: item.index, Phase: "ping", Result: failRes, Error: err.Error()})
					continue
				}
				r.emit(BatchTestResultEvent{Index: item.index, Phase: "ping", Result: result})
			}
		}()
	}
	wg.Wait()
}

// runPingStage 并发执行 TCP Ping，返回成功的节点（用于后续补查出口 IP）。
func (r *batchTestJobRunner) runPingStage() []batchNodeTestItem {
	jobs := make(chan batchNodeTestItem, len(r.items))
	for _, item := range r.items {
		jobs <- item
	}
	close(jobs)

	var mu sync.Mutex
	succeeded := make([]batchNodeTestItem, 0, len(r.items))
	var wg sync.WaitGroup
	workers := r.deps.workers
	if workers > len(r.items) {
		workers = len(r.items)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if r.stopped() {
					return
				}
				result, err := r.deps.ping(r.ctx, item)
				if r.stopped() {
					return
				}
				if err != nil {
					r.reportPrimaryFailure(item, false, err.Error())
					continue
				}
				r.reportPrimarySuccess(item, result)
				mu.Lock()
				succeeded = append(succeeded, item)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return succeeded
}

// ---------------------------------------------------------------------------
// 经代理（原生客户端或临时内核）的测试阶段
// ---------------------------------------------------------------------------

type batchProxyJob struct {
	item   batchNodeTestItem
	client *http.Client // 原生客户端（http/https 节点）
	group  *batchCoreGroup
	offset int
}

// batchCoreGroup 跟踪一个临时内核及其剩余待测节点数。
type batchCoreGroup struct {
	core      tempCoreInstance
	remaining int32
	prefetch  int32

	nearlyDone     chan struct{}
	nearlyDoneOnce sync.Once
	finished       chan struct{}
	finishOnce     sync.Once
	onFinish       func()
}

func newBatchCoreGroup(core tempCoreInstance, count int, prefetch int, onFinish func()) *batchCoreGroup {
	g := &batchCoreGroup{
		core:       core,
		remaining:  int32(count),
		prefetch:   int32(prefetch),
		nearlyDone: make(chan struct{}),
		finished:   make(chan struct{}),
		onFinish:   onFinish,
	}
	if count <= prefetch {
		g.nearlyDoneOnce.Do(func() { close(g.nearlyDone) })
	}
	if count <= 0 {
		g.finish()
	}
	return g
}

func (g *batchCoreGroup) done() {
	remaining := atomic.AddInt32(&g.remaining, -1)
	if remaining <= g.prefetch {
		g.nearlyDoneOnce.Do(func() { close(g.nearlyDone) })
	}
	if remaining <= 0 {
		g.finish()
	}
}

func (g *batchCoreGroup) finish() {
	g.finishOnce.Do(func() {
		g.nearlyDoneOnce.Do(func() { close(g.nearlyDone) })
		if g.core != nil {
			_ = g.core.Close()
		}
		if g.onFinish != nil {
			g.onFinish()
		}
		close(g.finished)
	})
}

func (r *batchTestJobRunner) trackCore(g *batchCoreGroup) {
	r.coresMu.Lock()
	defer r.coresMu.Unlock()
	r.activeCores[g] = struct{}{}
}

func (r *batchTestJobRunner) untrackCore(g *batchCoreGroup) {
	r.coresMu.Lock()
	defer r.coresMu.Unlock()
	delete(r.activeCores, g)
}

func (r *batchTestJobRunner) closeAllCores() {
	r.coresMu.Lock()
	groups := make([]*batchCoreGroup, 0, len(r.activeCores))
	for g := range r.activeCores {
		groups = append(groups, g)
	}
	r.coresMu.Unlock()
	for _, g := range groups {
		g.finish()
	}
}

// runProxyStage 执行 URL 延迟（ipOnly=false）或只查出口 IP（ipOnly=true，用于 ping 之后）。
func (r *batchTestJobRunner) runProxyStage(items []batchNodeTestItem, ipOnly bool) {
	queue := make(chan batchProxyJob, len(items))

	var workersWG sync.WaitGroup
	workers := r.deps.workers
	if workers > len(items) {
		workers = len(items)
	}
	for i := 0; i < workers; i++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for {
				select {
				case <-r.ctx.Done():
					return
				case job, ok := <-queue:
					if !ok {
						return
					}
					r.processProxyJob(job, ipOnly)
					if job.group != nil {
						job.group.done()
					}
				}
			}
		}()
	}

	// 原生客户端节点直接入队；其他节点交给临时内核
	coreItems := make([]batchNodeTestItem, 0, len(items))
	for _, item := range items {
		if r.stopped() {
			break
		}
		if item.options.PreferCore || r.deps.nativeClient == nil {
			coreItems = append(coreItems, item)
			continue
		}
		client, ok, err := r.deps.nativeClient(item)
		if err != nil {
			r.reportPrimaryFailure(item, ipOnly, err.Error())
			continue
		}
		if ok && client != nil {
			queue <- batchProxyJob{item: item, client: client}
			continue
		}
		coreItems = append(coreItems, item)
	}

	var compatWG sync.WaitGroup
	compatSlots := make(chan struct{}, batchCompatConcurrency)
	runCompat := func(pending []batchNodeTestItem) {
		for _, item := range pending {
			item := item
			compatWG.Add(1)
			go func() {
				defer compatWG.Done()
				select {
				case compatSlots <- struct{}{}:
				case <-r.ctx.Done():
					return
				}
				defer func() { <-compatSlots }()
				r.processCompatItem(item, ipOnly)
			}()
		}
	}

	coreSlots := make(chan struct{}, batchCoreMaxConcurrent)
	groups := make([]*batchCoreGroup, 0)
	chunkSize := temporaryCoreDelayTestBatchSize
	for start := 0; start < len(coreItems); start += chunkSize {
		if r.stopped() {
			break
		}
		end := start + chunkSize
		if end > len(coreItems) {
			end = len(coreItems)
		}
		select {
		case coreSlots <- struct{}{}:
		case <-r.ctx.Done():
		}
		if r.stopped() {
			break
		}

		core, active, err := r.startGroupWithRetry(coreItems[start:end], ipOnly)
		if err != nil {
			<-coreSlots
			if r.stopped() {
				break
			}
			r.logError("批量测试第 %d 组临时内核启动失败，该组改用兼容模式：%v", start/chunkSize+1, err)
			runCompat(active)
			continue
		}
		if core == nil {
			// 该组节点全部因配置无效被剔除
			<-coreSlots
			continue
		}

		var group *batchCoreGroup
		group = newBatchCoreGroup(core, len(active), batchCorePrefetchRemain, func() {
			r.untrackCore(group)
			<-coreSlots
		})
		r.trackCore(group)
		if r.stopped() {
			group.finish()
			break
		}
		groups = append(groups, group)
		for offset, item := range active {
			queue <- batchProxyJob{item: item, group: group, offset: offset}
		}

		// 还有下一组时，等当前组快测完再启动下一组内核
		if end < len(coreItems) {
			select {
			case <-group.nearlyDone:
			case <-r.ctx.Done():
			}
		}
	}

	// 等待所有内核组完成
	for _, group := range groups {
		select {
		case <-group.finished:
		case <-r.ctx.Done():
		}
	}
	close(queue)
	workersWG.Wait()
	compatWG.Wait()
}

// startGroupWithRetry 启动一组节点的临时内核；遇到配置无效时剔除出错节点并重试。
// 返回：内核（全部被剔除时为 nil）、实际在内核中的节点、错误（非配置类错误，或重试用尽）。
func (r *batchTestJobRunner) startGroupWithRetry(items []batchNodeTestItem, ipOnly bool) (tempCoreInstance, []batchNodeTestItem, error) {
	active := append([]batchNodeTestItem(nil), items...)
	for attempt := 0; ; attempt++ {
		if r.stopped() {
			return nil, nil, r.ctx.Err()
		}
		if len(active) == 0 {
			return nil, nil, nil
		}
		nodes := make([]Node, len(active))
		for i, item := range active {
			nodes[i] = item.node
		}
		core, err := r.deps.startCore(r.ctx, nodes)
		if err == nil {
			return core, active, nil
		}
		if r.stopped() {
			return nil, nil, r.ctx.Err()
		}
		var configErr *batchCoreConfigError
		if !errors.As(err, &configErr) || attempt >= batchCoreConfigRetryLimit {
			return nil, active, err
		}
		kind, entryIndex, ok := parseFailingConfigEntry(configErr.Output)
		if !ok {
			return nil, active, err
		}
		pos := configErr.Owners.Owner(kind, entryIndex)
		if pos < 0 || pos >= len(active) {
			return nil, active, err
		}
		bad := active[pos]
		summary := parseFailingOutboundErrorSummary(configErr.Output)
		r.logError("批量测试：节点配置无效，已跳过：%s，%s", bad.node.Name, summary)
		r.reportPrimaryFailure(bad, ipOnly, "节点配置无效："+summary)
		active = append(active[:pos], active[pos+1:]...)
	}
}

func (r *batchTestJobRunner) processProxyJob(job batchProxyJob, ipOnly bool) {
	if r.stopped() {
		return
	}
	client := job.client
	if client == nil {
		if job.group == nil || job.group.core == nil || r.deps.clientForPort == nil {
			r.reportPrimaryFailure(job.item, ipOnly, "代理客户端不可用")
			return
		}
		var err error
		client, err = r.deps.clientForPort(job.group.core.Port(job.offset))
		if err != nil {
			r.reportPrimaryFailure(job.item, ipOnly, err.Error())
			return
		}
	}

	result := batchBaseResult(job.item.node)
	if !ipOnly {
		delayResult, err := r.deps.delay(r.ctx, client, result)
		if r.stopped() {
			return
		}
		if err != nil {
			r.reportPrimaryFailure(job.item, false, err.Error())
			return
		}
		r.reportPrimarySuccess(job.item, delayResult)
		if !r.withIP {
			return
		}
		result = delayResult
	}
	ip, country, err := r.deps.outboundInfo(r.ctx, client)
	if r.stopped() {
		return
	}
	r.reportIP(job.item, result, ip, country, err)
}

// processCompatItem 兼容模式：每个节点单独起内核测试（临时内核整组启动失败时使用）。
func (r *batchTestJobRunner) processCompatItem(item batchNodeTestItem, ipOnly bool) {
	if r.stopped() || r.deps.compat == nil {
		return
	}
	if !ipOnly {
		result, err := r.deps.compat(r.ctx, item, "delay")
		if r.stopped() {
			return
		}
		if err != nil {
			r.reportPrimaryFailure(item, false, err.Error())
			return
		}
		r.reportPrimarySuccess(item, result)
		if !r.withIP {
			return
		}
	}
	ipResult, err := r.deps.compat(r.ctx, item, "ip")
	if r.stopped() {
		return
	}
	r.reportIP(item, ipResult, ipResult.OutboundIP, ipResult.Country, err)
}

// ---------------------------------------------------------------------------
// 真实的临时内核
// ---------------------------------------------------------------------------

type lockedLimitedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (b *lockedLimitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *lockedLimitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type batchCoreProcess struct {
	cmd        *exec.Cmd
	exited     chan struct{}
	configPath string
	ports      []int
	closeOnce  sync.Once
}

func (p *batchCoreProcess) Port(offset int) int {
	if p == nil || offset < 0 || offset >= len(p.ports) {
		return 0
	}
	return p.ports[offset]
}

func (p *batchCoreProcess) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			select {
			case <-p.exited:
			case <-time.After(batchCoreCloseWait):
			}
		}
		if p.configPath != "" {
			_ = os.Remove(p.configPath)
		}
	})
	return nil
}

// startBatchTempCore 写入配置并启动一个临时 sing-box 内核，等待全部端口就绪。
// 内核在端口就绪前退出时，返回 *batchCoreConfigError（可定位出错节点）或普通错误。
func startBatchTempCore(ctx context.Context, m *SingBoxManager, nodes []Node, options SpeedTestOptions, directDNS string) (tempCoreInstance, error) {
	if len(nodes) == 0 {
		return nil, errors.New("节点列表为空")
	}
	if m == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	if _, err := os.Stat(m.corePath); err != nil {
		return nil, fmt.Errorf("未找到测试内核：%s", m.corePath)
	}
	ports, releasePorts, err := reserveDelayTestPorts(len(nodes))
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(m.rootDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		releasePorts()
		return nil, err
	}
	configFile, err := os.CreateTemp(dataDir, "delay-batch-*.json")
	if err != nil {
		releasePorts()
		return nil, err
	}
	configPath := configFile.Name()
	upstreamPort := 0
	if options.UseSystemProxy {
		upstreamPort = m.getSOCKS5TestProxyPort()
	}
	config, owners := buildBatchDelayConfigWithOwners(nodes, ports, upstreamPort, directDNS)
	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		_ = configFile.Close()
		_ = os.Remove(configPath)
		releasePorts()
		return nil, err
	}
	if err := configFile.Close(); err != nil {
		_ = os.Remove(configPath)
		releasePorts()
		return nil, err
	}

	stderr := &lockedLimitedBuffer{limit: batchCoreStderrLimit}
	releasePorts()
	cmd := exec.Command(m.corePath, "run", "-c", configPath)
	cmd.Dir = m.rootDir
	cmd.Env = directNetworkEnv(os.Environ())
	cmd.Stderr = stderr
	// 进程被结束后，最多再等 500ms 让 stderr 管道收尾，避免 Wait 被子进程持有的管道卡住
	cmd.WaitDelay = 500 * time.Millisecond
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(configPath)
		return nil, err
	}
	process := &batchCoreProcess{cmd: cmd, exited: make(chan struct{}), configPath: configPath, ports: ports}
	go func() {
		_ = cmd.Wait()
		close(process.exited)
	}()

	portWait := delayRuntimeFromOptions(options).portWait
	portsReady := make(chan error, 1)
	go func() {
		_, err := waitDelayTestPortsContext(ctx, "127.0.0.1", ports, portWait)
		portsReady <- err
	}()

	select {
	case err := <-portsReady:
		if err != nil {
			_ = process.Close()
			return nil, fmt.Errorf("批量测试代理端口未就绪：%w (%s)", err, trimForLog(stderr.String()))
		}
		return process, nil
	case <-process.exited:
		_ = process.Close()
		output := stderr.String()
		if _, _, ok := parseFailingConfigEntry(output); ok {
			return nil, &batchCoreConfigError{Output: output, Owners: owners}
		}
		return nil, fmt.Errorf("临时内核异常退出：%s", trimForLog(output))
	case <-ctx.Done():
		_ = process.Close()
		return nil, ctx.Err()
	}
}
