package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const batchDelayCoreCheckTimeout = 15 * time.Second

const batchDelayPortWaitConcurrency = 32

const temporaryCoreDelayTestBatchSize = 300

type nodeDelayTestResult struct {
	result SpeedResult
	err    error
}

func (m *SingBoxManager) testNodesDelayViaClashAPIConcurrently(nodes []Node, options SpeedTestOptions, concurrency int) map[string]nodeDelayTestResult {
	results := make(map[string]nodeDelayTestResult, len(nodes))
	if len(nodes) == 0 {
		return results
	}
	delayRuntime := delayRuntimeFromOptions(options)
	if concurrency < 1 || concurrency > len(nodes) {
		concurrency = len(nodes)
	}
	jobs := make(chan Node)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range jobs {
				delay, err := m.testNodeDelayViaClashAPI(nodeOutboundTag(node), "", delayRuntime.timeout)
				base := SpeedResult{
					ID:          node.ID,
					Name:        node.Name,
					Type:        node.Type,
					Address:     node.Address,
					Port:        node.Port,
					Country:     node.Country,
					PingAddress: node.Address,
				}
				if err == nil {
					base.URLDelayMS = delay
					base.DelayMS = delay
					mu.Lock()
					results[node.ID] = nodeDelayTestResult{result: base}
					mu.Unlock()
				} else {
					mu.Lock()
					results[node.ID] = nodeDelayTestResult{result: base, err: err}
					mu.Unlock()
				}
			}
		}()
	}
	for _, node := range nodes {
		jobs <- node
	}
	close(jobs)
	wg.Wait()
	return results
}

func (m *SingBoxManager) TestNodesDelay(nodes []Node, options SpeedTestOptions) map[string]nodeDelayTestResult {
	return m.TestNodesMetric(nodes, "delay", options)
}

func (m *SingBoxManager) TestNodesMetric(nodes []Node, metric string, options SpeedTestOptions) map[string]nodeDelayTestResult {
	results := make(map[string]nodeDelayTestResult, len(nodes))
	if len(nodes) == 0 {
		return results
	}
	if metric != "delay" && metric != "url" && metric != "ip" {
		err := errors.New("批量节点检测类型无效")
		for _, node := range nodes {
			results[node.ID] = nodeDelayTestResult{err: err}
		}
		return results
	}

	var pendingNodes []Node
	if (metric == "delay" || metric == "url") && m.Running() && !options.UseSystemProxy {
		var clashNodes []Node
		for _, node := range nodes {
			if m.RuntimeHasOutbound(nodeOutboundTag(node)) {
				clashNodes = append(clashNodes, node)
			} else {
				pendingNodes = append(pendingNodes, node)
			}
		}
		if len(clashNodes) > 0 {
			concurrency := batchNodeTestConcurrency(len(clashNodes))
			clashResults := m.testNodesDelayViaClashAPIConcurrently(clashNodes, options, concurrency)
			for _, node := range clashNodes {
				r, ok := clashResults[node.ID]
				if !ok || (r.err != nil && (isClashAPIDialError(r.err) || isClashAPINotFoundError(r.err))) {
					pendingNodes = append(pendingNodes, node)
					delete(clashResults, node.ID)
				}
			}
			mergeNodeDelayTestResults(results, clashResults)
		}
	} else {
		pendingNodes = nodes
	}

	if len(pendingNodes) == 0 {
		return results
	}

	coreNodes := make([]Node, 0, len(pendingNodes))
	nativeNodes := make([]Node, 0)
	for _, node := range pendingNodes {
		if usesNativeProxyClient(node) {
			nativeNodes = append(nativeNodes, node)
		} else {
			coreNodes = append(coreNodes, node)
		}
	}

	var nativeResults map[string]nodeDelayTestResult
	var coreResults map[string]nodeDelayTestResult
	var coreErr error
	var wg sync.WaitGroup
	if len(nativeNodes) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			concurrency := batchNodeTestConcurrency(len(nativeNodes))
			nativeResults = m.testNodesDelayConcurrently(nativeNodes, metric, options, concurrency)
		}()
	}
	if len(coreNodes) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			concurrency := batchNodeTestConcurrency(len(coreNodes))
			coreResults, coreErr = m.testNodesDelayWithTemporaryCoreBatches(coreNodes, metric, options, concurrency)
		}()
	}
	wg.Wait()

	for id, item := range nativeResults {
		results[id] = item
	}
	if coreErr != nil {
		if m.logger != nil {
			m.logger.Error("批量真连接测试内核启动失败，改用兼容模式：%v", coreErr)
		}
		concurrency := temporaryCoreDelayTestMaxConcurrency
		coreResults = m.testNodesDelayConcurrently(coreNodes, metric, options, concurrency)
	}
	for id, item := range coreResults {
		results[id] = item
	}
	return results
}

func (m *SingBoxManager) testNodesDelayConcurrently(nodes []Node, metric string, options SpeedTestOptions, concurrency int) map[string]nodeDelayTestResult {
	results := make(map[string]nodeDelayTestResult, len(nodes))
	if len(nodes) == 0 {
		return results
	}
	if concurrency < 1 || concurrency > len(nodes) {
		concurrency = len(nodes)
	}
	jobs := make(chan Node)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range jobs {
				result, err := m.TestNodeMetricWithProgressAndOptions(node, metric, nil, options)
				mu.Lock()
				results[node.ID] = nodeDelayTestResult{result: result, err: err}
				mu.Unlock()
			}
		}()
	}
	for _, node := range nodes {
		jobs <- node
	}
	close(jobs)
	wg.Wait()
	return results
}

func (m *SingBoxManager) testNodesDelayWithTemporaryCore(nodes []Node, options SpeedTestOptions) (map[string]nodeDelayTestResult, error) {
	return m.testNodesDelayWithTemporaryCoreBatches(nodes, "delay", options, batchNodeTestConcurrency(len(nodes)))
}

func (m *SingBoxManager) testNodesDelayWithTemporaryCoreBatches(nodes []Node, metric string, options SpeedTestOptions, concurrency int) (map[string]nodeDelayTestResult, error) {
	results := make(map[string]nodeDelayTestResult, len(nodes))
	for start := 0; start < len(nodes); start += temporaryCoreDelayTestBatchSize {
		end := start + temporaryCoreDelayTestBatchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch, err := m.testNodesDelayWithTemporaryCoreConcurrency(nodes[start:end], metric, options, concurrency)
		if err != nil {
			return nil, fmt.Errorf("第 %d 批临时内核启动失败：%w", start/temporaryCoreDelayTestBatchSize+1, err)
		}
		mergeNodeDelayTestResults(results, batch)
	}
	return results, nil
}

func (m *SingBoxManager) testNodesDelayWithTemporaryCoreConcurrency(nodes []Node, metric string, options SpeedTestOptions, concurrency int) (map[string]nodeDelayTestResult, error) {
	results := make(map[string]nodeDelayTestResult, len(nodes))
	if len(nodes) == 0 {
		return results, nil
	}
	if _, err := os.Stat(m.corePath); err != nil {
		return nil, fmt.Errorf("未找到延迟测试内核：%s", m.corePath)
	}

	ports, releasePorts, err := reserveDelayTestPorts(len(nodes))
	if err != nil {
		return nil, err
	}
	defer releasePorts()

	dataDir := filepath.Join(m.rootDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	configFile, err := os.CreateTemp(dataDir, "delay-batch-*.json")
	if err != nil {
		return nil, err
	}
	configPath := configFile.Name()
	defer os.Remove(configPath)
	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	upstreamPort := 0
	if options.UseSystemProxy {
		upstreamPort = m.getSOCKS5TestProxyPort()
	}
	if err := encoder.Encode(buildBatchDelayConfigWithUpstream(nodes, ports, upstreamPort)); err != nil {
		_ = configFile.Close()
		return nil, err
	}
	if err := configFile.Close(); err != nil {
		return nil, err
	}

	releasePorts()
	cmd := exec.Command(m.corePath, "run", "-c", configPath)
	cmd.Dir = m.rootDir
	cmd.Env = directNetworkEnv(os.Environ())
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		diagCtx, diagCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer diagCancel()
		check := exec.CommandContext(diagCtx, m.corePath, "check", "-c", configPath)
		check.Dir = m.rootDir
		check.Env = directNetworkEnv(os.Environ())
		hideCommandWindow(check)
		if out, checkErr := check.CombinedOutput(); checkErr != nil {
			return nil, fmt.Errorf("批量节点测试内核启动失败：%w (%s)", err, trimForLog(string(out)))
		}
		return nil, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	delayRuntime := delayRuntimeFromOptions(options)
	if port, err := waitDelayTestPorts("127.0.0.1", ports, delayRuntime.portWait); err != nil {
		diagCtx, diagCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer diagCancel()
		check := exec.CommandContext(diagCtx, m.corePath, "check", "-c", configPath)
		check.Dir = m.rootDir
		check.Env = directNetworkEnv(os.Environ())
		hideCommandWindow(check)
		if out, checkErr := check.CombinedOutput(); checkErr != nil {
			return nil, fmt.Errorf("批量测试代理端口 %d 未就绪且配置检查异常：%w (%s)", port, err, trimForLog(string(out)))
		}
		return nil, fmt.Errorf("批量测试代理端口 %d 未就绪：%w", port, err)
	}

	type delayJob struct {
		index int
		node  Node
	}
	jobs := make(chan delayJob)
	workerCount := concurrency
	if workerCount < 1 || workerCount > len(nodes) {
		workerCount = len(nodes)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				node := job.node
				base := SpeedResult{
					ID:          node.ID,
					Name:        node.Name,
					Type:        node.Type,
					Address:     node.Address,
					Port:        node.Port,
					Country:     node.Country,
					PingAddress: node.Address,
				}
				client, err := buildLocalSOCKS5Client(ports[job.index], delayRuntime.timeout)
				if err != nil {
					mu.Lock()
					results[node.ID] = nodeDelayTestResult{err: err}
					mu.Unlock()
					continue
				}
				result, err := testNodeMetricWithClient(base, client, metric, nil, defaultSpeedTestConfig(), delayRuntime)
				mu.Lock()
				results[node.ID] = nodeDelayTestResult{result: result, err: err}
				mu.Unlock()
			}
		}()
	}
	for index, node := range nodes {
		jobs <- delayJob{index: index, node: node}
	}
	close(jobs)
	wg.Wait()
	return results, nil
}

func failedNodeDelayTests(nodes []Node, results map[string]nodeDelayTestResult) []Node {
	failed := make([]Node, 0)
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil {
			failed = append(failed, node)
		}
	}
	return failed
}

func mergeNodeDelayTestResults(target, source map[string]nodeDelayTestResult) {
	for id, item := range source {
		target[id] = item
	}
}

func reducedDelayRetryConcurrency(concurrency int) int {
	concurrency /= 2
	if concurrency < 1 {
		return 1
	}
	return concurrency
}

func (m *SingBoxManager) logDelayRetry(count, concurrency int) {
	if m.logger != nil {
		m.logger.Info("批量节点检测首次失败 %d 个，降低并发至 %d 路重试", count, concurrency)
	}
}

func waitDelayTestPorts(host string, ports []int, timeout time.Duration) (int, error) {
	if len(ports) == 0 {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	jobs := make(chan int)
	type portResult struct {
		port int
		err  error
	}
	results := make(chan portResult, len(ports))
	workerCount := batchDelayPortWaitConcurrency
	if workerCount > len(ports) {
		workerCount = len(ports)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				results <- portResult{port: port, err: waitDelayTestPort(ctx, host, port)}
			}
		}()
	}
	go func() {
		for _, port := range ports {
			jobs <- port
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for result := range results {
		if result.err != nil {
			return result.port, result.err
		}
	}
	return 0, nil
}

func waitDelayTestPort(ctx context.Context, host string, port int) error {
	for {
		if tcpPortOpen(host, port, 150*time.Millisecond) {
			return nil
		}
		timer := time.NewTimer(60 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func reserveDelayTestPorts(count int) ([]int, func(), error) {
	listeners := make([]net.Listener, 0, count)
	ports := make([]int, 0, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for _, item := range listeners {
				_ = item.Close()
			}
			return nil, nil, err
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			for _, listener := range listeners {
				_ = listener.Close()
			}
		})
	}
	return ports, release, nil
}

func buildBatchDelayConfig(nodes []Node, ports []int) map[string]interface{} {
	return buildBatchDelayConfigWithUpstream(nodes, ports, 0)
}

func buildBatchDelayConfigWithUpstream(nodes []Node, ports []int, upstreamPort int) map[string]interface{} {
	inbounds := make([]map[string]interface{}, 0, len(nodes))
	outbounds := []map[string]interface{}{{"type": "direct", "tag": "direct"}}
	hasDetourOutbound := false
	for _, node := range nodes {
		if outboundSupportsDetour(node.Outbound) {
			hasDetourOutbound = true
			break
		}
	}
	if upstreamPort > 0 && hasDetourOutbound {
		outbounds = append(outbounds, map[string]interface{}{
			"type":        "socks",
			"tag":         "upstream",
			"server":      "127.0.0.1",
			"server_port": upstreamPort,
		})
	}
	endpoints := make([]map[string]interface{}, 0)
	rules := make([]map[string]interface{}, 0, len(nodes))
	for index, node := range nodes {
		inboundTag := fmt.Sprintf("delay-in-%d", index)
		outboundTag := fmt.Sprintf("delay-out-%d", index)
		inbounds = append(inbounds, map[string]interface{}{
			"type":        "mixed",
			"tag":         inboundTag,
			"listen":      "127.0.0.1",
			"listen_port": ports[index],
		})
		outbound := sanitizeOutboundForSingBox(node.Outbound)
		outbound["tag"] = outboundTag
		if upstreamPort > 0 && outboundSupportsDetour(outbound) {
			outbound["detour"] = "upstream"
		}
		if endpoint := wireGuardEndpointFromOutbound(outbound); endpoint != nil {
			endpoints = append(endpoints, endpoint)
		} else {
			outbounds = append(outbounds, outbound)
		}
		rules = append(rules, map[string]interface{}{
			"inbound":  []string{inboundTag},
			"action":   "route",
			"outbound": outboundTag,
		})
	}
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"level":     "error",
			"timestamp": true,
		},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{{"tag": "local", "type": "local"}},
			"final":   "local",
		},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"default_domain_resolver": map[string]interface{}{
				"server":   "local",
				"strategy": "prefer_ipv4",
			},
			"rules": rules,
			"final": "direct",
		},
	}
	if len(endpoints) > 0 {
		config["endpoints"] = endpoints
	}
	return config
}
