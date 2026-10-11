package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var delayTestURLs = []string{
	"https://www.google.com/generate_204",
	"https://www.gstatic.com/generate_204",
	"http://www.msftconnecttest.com/connecttest.txt",
	"https://cp.cloudflare.com/generate_204",
}

const delayTestSampleCount = 2

const delayTestTimeout = 3 * time.Second

const delayCoreCheckTimeout = 8 * time.Second

const fastDelayTestSampleCount = 1

const fastDelayTestTimeout = 2500 * time.Millisecond

const fastDelayCoreCheckTimeout = 8 * time.Second

const fastDelayPortWaitTimeout = 2 * time.Second

const tcpPingResolveTimeout = 2 * time.Second

const tcpPingConnectTimeout = 2 * time.Second

const defaultSpeedTestTrafficMB = 20

const defaultSpeedTestDurationSec = 3

const speedTestRequestExtraTimeout = 3 * time.Second

const speedProgressInterval = 500 * time.Millisecond

type delayTestResult struct {
	delay int
	err   error
}

type delayRuntimeOptions struct {
	timeout      time.Duration
	sampleCount  int
	checkTimeout time.Duration
	portWait     time.Duration
}

type speedProgressFunc func(SpeedProgress)

type speedSampleFunc func(float64)

type speedTestConfig struct {
	trafficMB   int
	durationSec int
	bytes       int
	duration    time.Duration
}

var countryNames = map[string]string{
	"HK": "香港",
	"TW": "台湾",
	"MO": "澳门",
	"JP": "日本",
	"SG": "新加坡",
	"US": "美国",
	"KR": "韩国",
	"GB": "英国",
	"UK": "英国",
	"DE": "德国",
	"FR": "法国",
	"IT": "意大利",
	"ES": "西班牙",
	"CH": "瑞士",
	"SE": "瑞典",
	"NO": "挪威",
	"FI": "芬兰",
	"DK": "丹麦",
	"IE": "爱尔兰",
	"PL": "波兰",
	"PT": "葡萄牙",
	"CA": "加拿大",
	"AU": "澳大利亚",
	"IN": "印度",
	"MY": "马来西亚",
	"TH": "泰国",
	"VN": "越南",
	"PH": "菲律宾",
	"ID": "印度尼西亚",
	"AE": "阿联酋",
	"RU": "俄罗斯",
	"UA": "乌克兰",
	"CN": "中国",
	"NL": "荷兰",
	"TR": "土耳其",
	"BR": "巴西",
	"MX": "墨西哥",
	"AR": "阿根廷",
	"CL": "智利",
	"ZA": "南非",
}

var cloudflareColoCountryCodes = map[string]string{
	"HKG": "HK",
	"TPE": "TW",
	"NRT": "JP",
	"KIX": "JP",
	"SIN": "SG",
	"ICN": "KR",
	"LAX": "US",
	"SJC": "US",
	"SEA": "US",
	"DFW": "US",
	"ORD": "US",
	"JFK": "US",
	"IAD": "US",
	"LHR": "GB",
	"FRA": "DE",
	"CDG": "FR",
	"AMS": "NL",
	"SYD": "AU",
	"MEL": "AU",
	"YYZ": "CA",
	"YVR": "CA",
}

const maxCountryCacheEntries = 2048

var countryCache = struct {
	sync.RWMutex
	values map[string]string
}{values: make(map[string]string)}

type tcpPingResolveCall struct {
	done chan struct{}
	ip   string
	err  error
}

type tcpPingDNSCacheItem struct {
	ip        string
	expiresAt time.Time
}

var tcpPingDNSCache = struct {
	sync.RWMutex
	items map[string]tcpPingDNSCacheItem
}{items: make(map[string]tcpPingDNSCacheItem)}

var tcpPingResolveState = struct {
	sync.Mutex
	calls map[string]*tcpPingResolveCall
}{calls: make(map[string]*tcpPingResolveCall)}

var lookupTCPPingIP = net.DefaultResolver.LookupIPAddr

var temporaryDelayCoreSlots = make(chan struct{}, temporaryCoreDelayTestMaxConcurrency)

type geoResponse struct {
	IP          string
	Country     string
	CountryCode string
}

func defaultSpeedTestConfig() speedTestConfig {
	return speedTestConfig{
		trafficMB:   defaultSpeedTestTrafficMB,
		durationSec: defaultSpeedTestDurationSec,
		bytes:       defaultSpeedTestTrafficMB * 1000 * 1000,
		duration:    time.Duration(defaultSpeedTestDurationSec) * time.Second,
	}
}

func delayRuntimeFromOptions(options SpeedTestOptions) delayRuntimeOptions {
	opts := delayRuntimeOptions{
		timeout:      delayTestTimeout,
		sampleCount:  delayTestSampleCount,
		checkTimeout: delayCoreCheckTimeout,
		portWait:     5 * time.Second,
	}
	if options.FastDelay {
		opts = delayRuntimeOptions{
			timeout:      fastDelayTestTimeout,
			sampleCount:  fastDelayTestSampleCount,
			checkTimeout: fastDelayCoreCheckTimeout,
			portWait:     fastDelayPortWaitTimeout,
		}
	}
	if options.CustomTimeout > 0 {
		opts.timeout = options.CustomTimeout
		if opts.checkTimeout > options.CustomTimeout+2*time.Second {
			opts.checkTimeout = options.CustomTimeout + 2*time.Second
		}
	}
	return opts
}

func normalizeSpeedTestOptions(options SpeedTestOptions) (speedTestConfig, error) {
	if options.TrafficMB == 0 {
		options.TrafficMB = defaultSpeedTestTrafficMB
	}
	if options.DurationSec == 0 {
		options.DurationSec = defaultSpeedTestDurationSec
	}
	if options.TrafficMB != 20 && options.TrafficMB != 50 && options.TrafficMB != 100 && options.TrafficMB != 200 {
		return speedTestConfig{}, errors.New("测速流量只能选择 20M、50M、100M、200M")
	}
	if options.DurationSec != 3 && options.DurationSec != 5 && options.DurationSec != 8 {
		return speedTestConfig{}, errors.New("测速时间只能选择 3s、5s、8s")
	}
	return speedTestConfig{
		trafficMB:   options.TrafficMB,
		durationSec: options.DurationSec,
		bytes:       options.TrafficMB * 1000 * 1000,
		duration:    time.Duration(options.DurationSec) * time.Second,
	}, nil
}

var clashAPITransport = &http.Transport{
	Proxy: nil,
	DialContext: (&net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:        128,
	MaxIdleConnsPerHost: 64,
	IdleConnTimeout:     60 * time.Second,
}

func (m *SingBoxManager) testNodeDelayViaClashAPI(tag string, testURL string, timeout time.Duration) (int, error) {
	if !m.Running() {
		return 0, errors.New("sing-box 内核未运行")
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return 0, errors.New("节点 tag 不能为空")
	}
	if timeout <= 0 {
		timeout = delayTestTimeout
	}
	targets := []string{"https://cp.cloudflare.com/generate_204", "https://www.google.com/generate_204"}
	if strings.TrimSpace(testURL) != "" {
		targets = []string{strings.TrimSpace(testURL)}
	}
	timeoutMS := int(timeout.Milliseconds())
	if timeoutMS <= 0 {
		timeoutMS = 3000
	}
	escapedTag := url.PathEscape(strings.ReplaceAll(tag, "/", "%2F"))
	var lastErr error
	for _, target := range targets {
		apiURL := fmt.Sprintf("http://%s/proxies/%s/delay?url=%s&timeout=%d",
			clashAPIAddress,
			escapedTag,
			url.QueryEscape(target),
			timeoutMS,
		)
		client := http.Client{Transport: clashAPITransport, Timeout: timeout + 1200*time.Millisecond}
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			if isClashAPIDialError(err) {
				return 0, err
			}
			lastErr = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		var res struct {
			Delay   int    `json:"delay"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			lastErr = fmt.Errorf("解析 Clash API 延迟响应失败：%w", err)
			continue
		}
		if resp.StatusCode != http.StatusOK || (res.Message != "" && res.Delay == 0) {
			if res.Message != "" {
				lastErr = errors.New(res.Message)
			} else {
				lastErr = fmt.Errorf("Clash API 延迟测试返回 HTTP %d", resp.StatusCode)
			}
			if isClashAPINotFoundError(lastErr) {
				return 0, lastErr
			}
			msg := strings.ToLower(lastErr.Error())
			// 若为节点超时、连接被拒或握手异常，说明出站节点本身断连，无需等待后续测试目标，直接退出
			if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") || strings.Contains(msg, "refused") || strings.Contains(msg, "reset") || strings.Contains(msg, "handshake") || strings.Contains(msg, "delay test") {
				return 0, lastErr
			}
			continue
		}
		if res.Delay <= 0 {
			res.Delay = 1
		}
		return res.Delay, nil
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("延迟测试超时或失败")
}

func (m *SingBoxManager) TestNode(node Node) (int, error) {
	return m.TestNodeWithOptions(node, SpeedTestOptions{})
}

func (m *SingBoxManager) TestNodeWithOptions(node Node, options SpeedTestOptions) (int, error) {
	result, err := m.TestNodeMetricWithProgressAndOptions(node, "delay", nil, options)
	if err != nil {
		return 0, err
	}
	return result.DelayMS, nil
}

func (m *SingBoxManager) TestNodeMetric(node Node, metric string) (SpeedResult, error) {
	return m.TestNodeMetricWithProgress(node, metric, nil)
}

func (m *SingBoxManager) TestNodeMetricWithProgress(node Node, metric string, progress speedProgressFunc) (SpeedResult, error) {
	return m.TestNodeMetricWithProgressAndOptions(node, metric, progress, SpeedTestOptions{})
}

func (m *SingBoxManager) TestNodeMetricWithProgressAndOptions(node Node, metric string, progress speedProgressFunc, options SpeedTestOptions) (SpeedResult, error) {
	if node.ID == "" {
		return SpeedResult{}, errors.New("节点无效")
	}
	result := SpeedResult{
		ID:          node.ID,
		Name:        node.Name,
		Type:        node.Type,
		Address:     node.Address,
		Port:        node.Port,
		Country:     node.Country,
		PingAddress: node.Address,
	}
	if metric == "ping" {
		// TCP 延迟测试严格采用本地物理网络直连，确保测量真实网络时延
		ping, err := testNodePing(node)
		if err != nil {
			return SpeedResult{}, err
		}
		result.PingMS = ping
		result.DelayMS = ping
		return result, nil
	}
	if metric != "delay" && metric != "url" && metric != "ip" && metric != "speed" {
		return SpeedResult{}, errors.New("测速类型无效")
	}
	speedConfig := defaultSpeedTestConfig()
	delayRuntime := delayRuntimeFromOptions(options)
	if metric == "speed" {
		var err error
		speedConfig, err = normalizeSpeedTestOptions(options)
		if err != nil {
			return SpeedResult{}, err
		}
	}
	if (metric == "delay" || metric == "url") && m.Running() && !options.UseSystemProxy && m.RuntimeHasOutbound(nodeOutboundTag(node)) {
		delay, err := m.testNodeDelayViaClashAPI(nodeOutboundTag(node), "", delayRuntime.timeout)
		if err == nil {
			result.URLDelayMS = delay
			result.DelayMS = delay
			return result, nil
		}
		if !isClashAPIDialError(err) && !isClashAPINotFoundError(err) {
			return SpeedResult{}, err
		}
	}
	if !options.PreferCore {
		upstreamPort := 0
		if options.UseSystemProxy {
			upstreamPort = m.getSOCKS5TestProxyPort()
		}
		client, ok, err := buildNativeProxyClientWithUpstreamTimeout(node, upstreamPort, nativeProxyTimeout(options))
		if err != nil {
			return SpeedResult{}, err
		}
		if ok {
			return testNodeMetricWithClient(result, client, metric, progress, speedConfig, delayRuntime)
		}
	}
	temporaryDelayCoreSlots <- struct{}{}
	defer func() { <-temporaryDelayCoreSlots }()
	delayCorePath := m.corePath
	if _, err := os.Stat(delayCorePath); err != nil {
		return SpeedResult{}, fmt.Errorf("未找到延迟测试内核：%s", delayCorePath)
	}
	port, err := pickFreePort()
	if err != nil {
		return SpeedResult{}, err
	}
	upstreamPort := 0
	if options.UseSystemProxy {
		upstreamPort = m.getSOCKS5TestProxyPort()
	}
	configPath := filepath.Join(m.rootDir, "data", "delay-"+node.ID+".json")
	config := buildDelayConfigWithUpstream(node, port, upstreamPort)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return SpeedResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return SpeedResult{}, err
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return SpeedResult{}, err
	}
	defer os.Remove(configPath)

	checkCtx, checkCancel := context.WithTimeout(context.Background(), delayRuntime.checkTimeout)
	defer checkCancel()
	check := exec.CommandContext(checkCtx, delayCorePath, "check", "-c", configPath)
	check.Dir = m.rootDir
	check.Env = directNetworkEnv(os.Environ())
	hideCommandWindow(check)
	if out, err := check.CombinedOutput(); err != nil {
		if checkCtx.Err() == context.DeadlineExceeded {
			return SpeedResult{}, errors.New("节点配置检查超时")
		}
		return SpeedResult{}, fmt.Errorf("节点配置检查失败：%s", trimForLog(string(out)))
	}

	runArgs := []string{"run", "-c", configPath}
	cmd := exec.Command(delayCorePath, runArgs...)
	cmd.Dir = m.rootDir
	cmd.Env = directNetworkEnv(os.Environ())
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		return SpeedResult{}, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if err := waitPort("127.0.0.1", port, delayRuntime.portWait); err != nil {
		return SpeedResult{}, fmt.Errorf("测试代理端口未就绪：%w", err)
	}

	client, err := buildLocalSOCKS5Client(port, delayRuntime.timeout)
	if err != nil {
		return SpeedResult{}, err
	}
	switch metric {
	case "delay", "url":
		return testNodeMetricWithClient(result, client, metric, progress, speedConfig, delayRuntime)
	case "ip":
		return testNodeMetricWithClient(result, client, metric, progress, speedConfig, delayRuntime)
	case "speed":
		return testNodeMetricWithClient(result, client, metric, progress, speedConfig, delayRuntime)
	default:
		return SpeedResult{}, errors.New("测速类型无效")
	}
}

func testNodeMetricWithClient(result SpeedResult, client *http.Client, metric string, progress speedProgressFunc, speedConfig speedTestConfig, delayRuntime delayRuntimeOptions) (SpeedResult, error) {
	switch metric {
	case "delay", "url":
		delayClient := *client
		delayClient.Timeout = delayRuntime.timeout
		delay, err := testDelay(&delayClient, delayRuntime)
		if err != nil {
			return SpeedResult{}, err
		}
		result.URLDelayMS = delay
		result.DelayMS = delay
	case "ip":
		ip, country, err := testOutboundInfo(client)
		if err != nil {
			return SpeedResult{}, err
		}
		result.OutboundIP = ip
		result.Country = firstNonEmpty(country, result.Country)
	case "speed":
		speedClient := *client
		speedClient.Timeout = speedConfig.duration + speedTestRequestExtraTimeout
		testLabel := fmt.Sprintf("%dM / %ds", speedConfig.trafficMB, speedConfig.durationSec)
		emitSpeedProgress(progress, result, "download", fmt.Sprintf("正在测试下载速度（%s）", testLabel), 0, 0)
		download, err := testDownload(&speedClient, speedConfig, func(mbps float64) {
			emitSpeedProgress(progress, result, "download", fmt.Sprintf("下载 %.2f Mbps（%s）", mbps, testLabel), mbps, 0)
		})
		if err != nil {
			return SpeedResult{}, fmt.Errorf("下载测速失败：%w", err)
		}
		result.DownloadMbps = download
		emitSpeedProgress(progress, result, "upload", fmt.Sprintf("下载 %.2f Mbps，正在测试上传（%s）", download, testLabel), download, 0)
		upload, err := testUpload(&speedClient, speedConfig, func(mbps float64) {
			emitSpeedProgress(progress, result, "upload", fmt.Sprintf("下载 %.2f Mbps，上传 %.2f Mbps（%s）", download, mbps, testLabel), download, mbps)
		})
		if err != nil {
			return SpeedResult{}, fmt.Errorf("上传测速失败：%w", err)
		}
		result.UploadMbps = upload
		emitSpeedProgress(progress, result, "done", fmt.Sprintf("下载 %.2f Mbps，上传 %.2f Mbps", download, upload), download, upload)
	default:
		return SpeedResult{}, errors.New("测速类型无效")
	}
	return result, nil
}

func emitSpeedProgress(progress speedProgressFunc, result SpeedResult, phase string, status string, download float64, upload float64) {
	if progress == nil {
		return
	}
	progress(SpeedProgress{
		ID:           result.ID,
		Raw:          "",
		Name:         result.Name,
		Phase:        phase,
		Status:       status,
		DownloadMbps: download,
		UploadMbps:   upload,
	})
}

func testNodePing(node Node) (int, error) {
	return testNodePingWithUpstream(node, 0)
}

func testNodePingWithUpstream(node Node, upstreamPort int) (int, error) {
	if node.Port > 0 {
		return testTCPAddressWithUpstream(node.Address, node.Port, upstreamPort)
	}
	if upstreamPort > 0 {
		return testTCPAddressWithUpstream(node.Address, 80, upstreamPort)
	}
	return testPingAddress(node.Address)
}

func testTCPAddress(address string, port int) (int, error) {
	return testTCPAddressWithUpstream(address, port, 0)
}

func testTCPAddressWithUpstream(address string, port int, upstreamPort int) (int, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0, errors.New("TCP 测试地址为空")
	}
	if port <= 0 || port > 65535 {
		return 0, errors.New("TCP 测试端口无效")
	}
	if upstreamPort > 0 {
		if !localProxyReady(upstreamPort) {
			return 0, fmt.Errorf("代理测试需要先启动本地代理端口：127.0.0.1:%d", upstreamPort)
		}
		dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(upstreamPort)), nil, &net.Dialer{Timeout: tcpPingConnectTimeout})
		if err != nil {
			return 0, fmt.Errorf("连接本地代理失败：%w", err)
		}
		type dialRes struct {
			conn net.Conn
			err  error
		}
		ch := make(chan dialRes, 1)
		start := time.Now()
		go func() {
			c, e := dialer.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
			ch <- dialRes{conn: c, err: e}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				return 0, fmt.Errorf("通过代理测试 TCP 失败：%w", r.err)
			}
			_ = r.conn.Close()
			delay := int(time.Since(start).Milliseconds())
			if delay < 1 {
				delay = 1
			}
			return delay, nil
		case <-time.After(tcpPingConnectTimeout):
			go func() {
				if r := <-ch; r.conn != nil {
					_ = r.conn.Close()
				}
			}()
			return 0, errors.New("通过代理测试 TCP 超时")
		}
	}
	resolvedAddress, err := resolveTCPPingAddress(address)
	if err != nil {
		return 0, fmt.Errorf("TCP 地址解析失败：%w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), tcpPingConnectTimeout)
	defer cancel()
	dialer := newDirectPhysicalDialerForTarget(net.ParseIP(resolvedAddress), tcpPingConnectTimeout)
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(resolvedAddress, strconv.Itoa(port)))
	if err != nil {
		return 0, fmt.Errorf("TCP 连接失败：%w", err)
	}
	_ = conn.Close()
	delay := int(time.Since(start).Milliseconds())
	if delay < 1 {
		delay = 1
	}
	return delay, nil
}

func resolveTCPPingAddress(address string) (string, error) {
	host := strings.Trim(strings.TrimSpace(address), "[]")
	if ip := net.ParseIP(host); ip != nil {
		if isFakeIPv4(ip) {
			return "", fmt.Errorf("目标地址属于 TUN FakeIP 保留段，无法用于本地测试：%s", host)
		}
		return ip.String(), nil
	}
	key := strings.ToLower(host)

	// 1. 优先读取 5 分钟 DNS 解析缓存，大幅提升批量节点测速效率（同域名仅需解析一次）
	tcpPingDNSCache.RLock()
	if cached, ok := tcpPingDNSCache.items[key]; ok && time.Now().Before(cached.expiresAt) {
		tcpPingDNSCache.RUnlock()
		return cached.ip, nil
	}
	tcpPingDNSCache.RUnlock()

	tcpPingResolveState.Lock()
	if call, ok := tcpPingResolveState.calls[key]; ok {
		tcpPingResolveState.Unlock()
		<-call.done
		return call.ip, call.err
	}
	call := &tcpPingResolveCall{done: make(chan struct{})}
	tcpPingResolveState.calls[key] = call
	tcpPingResolveState.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), tcpPingResolveTimeout)
	resolved, err := resolveDirectPhysicalIP(ctx, host)
	cancel()
	call.ip = resolved
	call.err = err

	// 2. 解析成功后写入缓存
	if err == nil && resolved != "" {
		tcpPingDNSCache.Lock()
		tcpPingDNSCache.items[key] = tcpPingDNSCacheItem{
			ip:        resolved,
			expiresAt: time.Now().Add(5 * time.Minute),
		}
		tcpPingDNSCache.Unlock()
	}

	tcpPingResolveState.Lock()
	delete(tcpPingResolveState.calls, key)
	close(call.done)
	tcpPingResolveState.Unlock()
	return call.ip, call.err
}

func preferredTCPPingIP(addresses []net.IPAddr) (string, error) {
	for _, address := range addresses {
		if ipv4 := address.IP.To4(); ipv4 != nil && !isFakeIPv4(ipv4) {
			return ipv4.String(), nil
		}
	}
	for _, address := range addresses {
		if address.IP != nil && !isFakeIPv4(address.IP) {
			return address.IP.String(), nil
		}
	}
	return "", errors.New("域名没有可用 IP 地址（或被 TUN FakeIP 拦截）")
}

func testPingAddress(address string) (int, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0, errors.New("Ping 地址为空")
	}
	if resolved, err := resolveTCPPingAddress(address); err == nil && resolved != "" {
		address = resolved
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", "1000", address)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", address)
	}
	hideCommandWindow(cmd)
	cmd.Env = directNetworkEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	text := string(out)
	if ctx.Err() == context.DeadlineExceeded {
		return 0, errors.New("Ping 超时")
	}
	if delay, ok := parsePingDelay(text); ok {
		return delay, nil
	}
	if err != nil {
		return 0, fmt.Errorf("Ping 失败：%s", trimForLog(text))
	}
	return 0, errors.New("未解析到 Ping 延迟")
}

func parsePingDelay(output string) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:time|时间)[=<]\s*(\d+)\s*ms`),
		regexp.MustCompile(`(?i)(?:average|平均)\s*=\s*(\d+)\s*ms`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(output)
		if len(match) < 2 {
			continue
		}
		delay, err := strconv.Atoi(match[1])
		if err == nil {
			if delay < 1 {
				delay = 1
			}
			return delay, true
		}
	}
	return 0, false
}

func lookupIPCountry(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return ""
	}
	cacheKey := ip.String()
	if cached, ok := loadCountryCache(cacheKey); ok {
		return cached
	}
	client := http.Client{Timeout: 1500 * time.Millisecond}
	country := lookupIPCountryWithClient(&client, cacheKey)
	if country != "" {
		storeCountryCache(cacheKey, country)
	}
	return country
}

func loadCountryCache(key string) (string, bool) {
	countryCache.RLock()
	value, ok := countryCache.values[key]
	countryCache.RUnlock()
	return value, ok
}

func storeCountryCache(key, value string) {
	countryCache.Lock()
	if _, exists := countryCache.values[key]; !exists && len(countryCache.values) >= maxCountryCacheEntries {
		clear(countryCache.values)
	}
	countryCache.values[key] = value
	countryCache.Unlock()
}

func lookupIPCountryWithClient(client *http.Client, ip string) string {
	escapedIP := url.PathEscape(ip)
	services := []struct {
		endpoint string
		decode   func(io.Reader) (geoResponse, error)
	}{
		{"https://ipwho.is/" + escapedIP, decodeIPWhoisGeo},
		{"https://ipapi.co/" + escapedIP + "/json/", decodeIPAPICoGeo},
		{"http://ip-api.com/json/" + escapedIP + "?fields=status,country,countryCode", decodeIPAPIGeo},
	}
	for _, service := range services {
		data, err := requestGeo(client, service.endpoint, service.decode)
		if err != nil {
			continue
		}
		if country := geoCountryName(data.CountryCode, data.Country); country != "" {
			return country
		}
	}
	return ""
}

func buildNativeProxyClient(node Node) (*http.Client, bool, error) {
	return buildNativeProxyClientWithUpstream(node, 0)
}

func (m *SingBoxManager) buildNativeProxyClient(node Node) (*http.Client, bool, error) {
	return buildNativeProxyClientWithUpstream(node, 0)
}

func buildNativeProxyClientWithUpstream(node Node, upstreamPort int) (*http.Client, bool, error) {
	return buildNativeProxyClientWithUpstreamTimeout(node, upstreamPort, 4*time.Second)
}

func nativeProxyTimeout(options SpeedTestOptions) time.Duration {
	if options.CustomTimeout > 0 {
		return options.CustomTimeout
	}
	if options.FastDelay {
		return fastDelayTestTimeout
	}
	return 4 * time.Second
}

func buildNativeProxyClientWithUpstreamTimeout(node Node, upstreamPort int, timeout time.Duration) (*http.Client, bool, error) {
	if timeout <= 0 {
		timeout = fastDelayTestTimeout
	}
	proxyType := strings.ToLower(firstNonEmpty(stringFromAny(node.Outbound["type"]), node.Type))
	switch proxyType {
	case "socks", "socks5":
		dialer, err := buildSOCKS5Dialer(node, upstreamPort, timeout)
		if err != nil {
			return nil, true, err
		}
		return &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext:           dialContextFromProxy(dialer),
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
			},
		}, true, nil
	case "http", "https":
		proxyURL, err := buildHTTPProxyURL(node)
		if err != nil {
			return nil, true, err
		}
		if upstreamPort > 0 {
			if !localProxyReady(upstreamPort) {
				return nil, true, fmt.Errorf("HTTP 代理测试需要先启动本地代理端口：127.0.0.1:%d", upstreamPort)
			}
			upstreamDialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(upstreamPort)), nil, &net.Dialer{Timeout: timeout})
			if err != nil {
				return nil, true, err
			}
			return &http.Client{
				Timeout: timeout,
				Transport: &http.Transport{
					Proxy:                 http.ProxyURL(proxyURL),
					DialContext:           dialContextFromProxy(upstreamDialer),
					DisableKeepAlives:     true,
					TLSHandshakeTimeout:   timeout,
					ResponseHeaderTimeout: timeout,
				},
			}, true, nil
		}
		return &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyURL(proxyURL),
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
			},
		}, true, nil
	default:
		return nil, false, nil
	}
}

func isSOCKS5Node(node Node) bool {
	proxyType := strings.ToLower(firstNonEmpty(stringFromAny(node.Outbound["type"]), node.Type))
	return proxyType == "socks" || proxyType == "socks5"
}

func usesNativeProxyClient(node Node) bool {
	proxyType := strings.ToLower(firstNonEmpty(stringFromAny(node.Outbound["type"]), node.Type))
	return proxyType == "socks" || proxyType == "socks5" || proxyType == "http" || proxyType == "https"
}

func buildSOCKS5Dialer(node Node, upstreamPort int, timeout time.Duration) (proxy.Dialer, error) {
	if timeout <= 0 {
		timeout = fastDelayTestTimeout
	}
	host := strings.TrimSpace(firstNonEmpty(stringFromAny(node.Outbound["server"]), node.Address))
	port := intFromAny(node.Outbound["server_port"])
	if port == 0 {
		port = node.Port
	}
	if host == "" || port <= 0 || port > 65535 {
		return nil, errors.New("SOCKS5 节点地址或端口无效")
	}
	username := stringFromAny(node.Outbound["username"])
	password := stringFromAny(node.Outbound["password"])
	var auth *proxy.Auth
	if username != "" || password != "" {
		auth = &proxy.Auth{User: username, Password: password}
	}
	if upstreamPort > 0 {
		if !localProxyReady(upstreamPort) {
			return nil, fmt.Errorf("SOCKS5 测试需要先启动本地系统代理端口：127.0.0.1:%d", upstreamPort)
		}
		upstream, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(upstreamPort)), nil, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, err
		}
		return proxy.SOCKS5("tcp", net.JoinHostPort(host, strconv.Itoa(port)), auth, upstream)
	}
	return proxy.SOCKS5("tcp", net.JoinHostPort(host, strconv.Itoa(port)), auth, &net.Dialer{Timeout: timeout})
}

func buildHTTPProxyURL(node Node) (*url.URL, error) {
	host := strings.TrimSpace(firstNonEmpty(stringFromAny(node.Outbound["server"]), node.Address))
	port := intFromAny(node.Outbound["server_port"])
	if port == 0 {
		port = node.Port
	}
	if host == "" || port <= 0 || port > 65535 {
		return nil, errors.New("HTTP 代理节点地址或端口无效")
	}
	scheme := "http"
	if strings.EqualFold(node.Type, "https") {
		scheme = "https"
	}
	if tlsConfig, ok := node.Outbound["tls"].(map[string]interface{}); ok && tlsConfig["enabled"] == true {
		scheme = "https"
	}
	proxyURL := &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(port))}
	username := stringFromAny(node.Outbound["username"])
	password := stringFromAny(node.Outbound["password"])
	if username != "" || password != "" {
		proxyURL.User = url.UserPassword(username, password)
	}
	return proxyURL, nil
}

func dialContextFromProxy(dialer proxy.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		type dialResult struct {
			conn net.Conn
			err  error
		}
		done := make(chan dialResult, 1)
		go func() {
			conn, err := dialer.Dial(network, address)
			select {
			case done <- dialResult{conn: conn, err: err}:
			case <-ctx.Done():
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-done:
			return result.conn, result.err
		}
	}
}

func testDelay(client *http.Client, options delayRuntimeOptions) (int, error) {
	if len(delayTestURLs) == 0 {
		return 0, errors.New("URL 延迟测试地址为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	results := make(chan delayTestResult, len(delayTestURLs))
	for _, target := range delayTestURLs {
		target := target
		go func() {
			delay, err := requestBestDelay(ctx, client, target, options.sampleCount)
			results <- delayTestResult{delay: delay, err: err}
		}()
	}
	var lastErr error
	for range delayTestURLs {
		var result delayTestResult
		select {
		case result = <-results:
		case <-ctx.Done():
			return 0, fmt.Errorf("URL 延迟测试超时：%w", ctx.Err())
		}
		if result.err == nil {
			cancel()
			return result.delay, nil
		}
		lastErr = result.err
	}
	return 0, fmt.Errorf("所有 URL 延迟测试地址失败：%w", lastErr)
}

func requestBestDelay(ctx context.Context, client *http.Client, target string, sampleCount int) (int, error) {
	best := 0
	var lastErr error
	if sampleCount < 1 {
		sampleCount = 1
	}
	for i := 0; i < sampleCount; i++ {
		sampleCtx, cancel := delaySampleContext(ctx, sampleCount-i)
		delay, err := requestDelay(sampleCtx, client, target)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if best == 0 || delay < best {
			best = delay
		}
	}
	if best > 0 {
		return best, nil
	}
	return 0, lastErr
}

func delaySampleContext(parent context.Context, remainingSamples int) (context.Context, context.CancelFunc) {
	if remainingSamples < 1 {
		remainingSamples = 1
	}
	deadline, ok := parent.Deadline()
	if !ok {
		return context.WithCancel(parent)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, remaining/time.Duration(remainingSamples))
}

func requestDelay(ctx context.Context, client *http.Client, target string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	delay := int(time.Since(start).Milliseconds())
	_ = resp.Body.Close()
	if delay < 1 {
		delay = 1
	}
	return delay, nil
}

func buildLocalSOCKS5Client(port int, timeout time.Duration) (*http.Client, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("本地 SOCKS5 端口 %d 无效", port)
	}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("创建本地 SOCKS5 客户端失败：%w", err)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dialContextFromProxy(dialer),
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
		},
	}, nil
}

func testOutboundInfo(client *http.Client) (string, string, error) {
	ip, country, err := testCloudflareTraceGeo(client)
	if err == nil && country != "" {
		return ip, country, nil
	}
	ip, country, err = testOutboundGeo(client)
	if err == nil {
		return ip, country, nil
	}
	ip, err = testOutboundIP(client)
	if err != nil {
		return "", "", err
	}
	return ip, lookupIPCountry(ip), nil
}

func testCloudflareTraceGeo(client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://cp.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("Cloudflare Trace 返回状态：%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", "", err
	}
	values := parseTraceValues(string(data))
	ip := strings.TrimSpace(values["ip"])
	if net.ParseIP(ip) == nil {
		return "", "", errors.New("Cloudflare Trace 返回 IP 无效")
	}
	return ip, traceCountryName(values["colo"], values["loc"]), nil
}

func parseTraceValues(text string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	return values
}

func traceCountryName(colo string, loc string) string {
	if country := geoCountryName(loc, ""); country != "" {
		return country
	}
	if code := cloudflareColoCountryCodes[strings.ToUpper(strings.TrimSpace(colo))]; code != "" {
		return geoCountryName(code, "")
	}
	return ""
}

func testOutboundGeo(client *http.Client) (string, string, error) {
	services := []struct {
		endpoint string
		decode   func(io.Reader) (geoResponse, error)
	}{
		{"https://ipwho.is/", decodeIPWhoisGeo},
		{"https://ipapi.co/json/", decodeIPAPICoGeo},
		{"http://ip-api.com/json/?fields=status,country,countryCode,query", decodeIPAPIGeo},
	}
	var lastErr error
	for _, service := range services {
		data, err := requestGeo(client, service.endpoint, service.decode)
		if err != nil {
			lastErr = err
			continue
		}
		ip := strings.TrimSpace(data.IP)
		country := geoCountryName(data.CountryCode, data.Country)
		if net.ParseIP(ip) == nil || country == "" {
			lastErr = errors.New("出站地理位置返回格式无效")
			continue
		}
		return ip, country, nil
	}
	if lastErr == nil {
		lastErr = errors.New("出站地理位置查询失败")
	}
	return "", "", lastErr
}

func requestGeo(client *http.Client, endpoint string, decode func(io.Reader) (geoResponse, error)) (geoResponse, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return geoResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return geoResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return geoResponse{}, fmt.Errorf("地理位置查询返回状态：%d", resp.StatusCode)
	}
	return decode(io.LimitReader(resp.Body, 2048))
}

func decodeIPWhoisGeo(reader io.Reader) (geoResponse, error) {
	var data struct {
		Success     bool   `json:"success"`
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return geoResponse{}, err
	}
	if !data.Success {
		if data.Message != "" {
			return geoResponse{}, errors.New(data.Message)
		}
		return geoResponse{}, errors.New("ipwho.is 查询失败")
	}
	return geoResponse{IP: data.IP, Country: data.Country, CountryCode: data.CountryCode}, nil
}

func decodeIPAPICoGeo(reader io.Reader) (geoResponse, error) {
	var data struct {
		IP          string `json:"ip"`
		Country     string `json:"country_name"`
		CountryCode string `json:"country_code"`
		Error       bool   `json:"error"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return geoResponse{}, err
	}
	if data.Error {
		if data.Reason != "" {
			return geoResponse{}, errors.New(data.Reason)
		}
		return geoResponse{}, errors.New("ipapi.co 查询失败")
	}
	return geoResponse{IP: data.IP, Country: data.Country, CountryCode: data.CountryCode}, nil
}

func decodeIPAPIGeo(reader io.Reader) (geoResponse, error) {
	var data struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		Query       string `json:"query"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return geoResponse{}, err
	}
	if data.Status != "success" {
		if data.Message != "" {
			return geoResponse{}, errors.New(data.Message)
		}
		return geoResponse{}, errors.New("ip-api 查询失败")
	}
	return geoResponse{IP: data.Query, Country: data.Country, CountryCode: data.CountryCode}, nil
}

func geoCountryName(code string, name string) string {
	if country := countryNames[strings.ToUpper(strings.TrimSpace(code))]; country != "" {
		return country
	}
	name = strings.TrimSpace(name)
	switch strings.ToLower(name) {
	case "hong kong":
		return "香港"
	case "taiwan":
		return "台湾"
	case "japan":
		return "日本"
	case "singapore":
		return "新加坡"
	case "united states":
		return "美国"
	case "south korea":
		return "韩国"
	case "united kingdom":
		return "英国"
	case "germany":
		return "德国"
	case "france":
		return "法国"
	case "canada":
		return "加拿大"
	case "australia":
		return "澳大利亚"
	case "india":
		return "印度"
	case "russia":
		return "俄罗斯"
	case "china":
		return "中国"
	}
	return name
}

func testOutboundIP(client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("出站 IP 查询返回状态：%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(data))
	if net.ParseIP(ip) == nil {
		return "", errors.New("出站 IP 返回格式无效")
	}
	return ip, nil
}

func testDownload(client *http.Client, config speedTestConfig, sample speedSampleFunc) (float64, error) {
	targets := []string{
		fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", config.bytes),
		"https://cachefly.cachefly.net/50mb.test",
	}
	if config.trafficMB == 100 {
		targets = append(targets, "https://proof.ovh.net/files/100Mb.dat")
	}
	var lastErr error
	for _, target := range targets {
		speed, err := measureHTTP(client, http.MethodGet, target, nil, 0, config.duration, sample)
		if err == nil {
			return speed, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func testUpload(client *http.Client, config speedTestConfig, sample speedSampleFunc) (float64, error) {
	body := newTimedUploadReader(config.duration, config.bytes, sample)
	return measureHTTP(client, http.MethodPost, "https://speed.cloudflare.com/__up", body, 0, config.duration, sample)
}

func measureHTTP(client *http.Client, method, target string, body io.Reader, bytesCount int, duration time.Duration, sample speedSampleFunc) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = -1
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if upload, ok := body.(*timedUploadReader); ok && upload.BytesWritten() > 0 {
			return mbpsFromBytes(upload.BytesWritten(), time.Since(start)), nil
		}
		return 0, err
	}
	defer resp.Body.Close()
	if method == http.MethodGet {
		n, err := copyUntilDone(io.Discard, resp.Body, ctx, start, sample)
		if err != nil && n == 0 {
			return 0, err
		}
		bytesCount = int(n)
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		if upload, ok := body.(*timedUploadReader); ok {
			bytesCount = int(upload.BytesWritten())
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		if upload, ok := body.(*timedUploadReader); ok && upload.BytesWritten() > 0 {
			return mbpsFromBytes(upload.BytesWritten(), time.Since(start)), nil
		}
		return 0, fmt.Errorf("测速地址返回状态：%d", resp.StatusCode)
	}
	if bytesCount <= 0 {
		return 0, errors.New("测速未收到有效数据")
	}
	seconds := time.Since(start).Seconds()
	if seconds <= 0 {
		return 0, errors.New("测速时间无效")
	}
	return float64(bytesCount) * 8 / seconds / 1000 / 1000, nil
}

func copyUntilDone(dst io.Writer, src io.Reader, ctx context.Context, start time.Time, sample speedSampleFunc) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	lastSample := time.Now()
	for {
		select {
		case <-ctx.Done():
			return written, nil
		default:
		}
		n, err := src.Read(buf)
		if n > 0 {
			w, writeErr := dst.Write(buf[:n])
			written += int64(w)
			if writeErr != nil {
				return written, writeErr
			}
			if w != n {
				return written, io.ErrShortWrite
			}
			if sample != nil && time.Since(lastSample) >= speedProgressInterval {
				sample(mbpsFromBytes(written, time.Since(start)))
				lastSample = time.Now()
			}
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && written > 0 {
				return written, nil
			}
			if err == io.EOF {
				return written, nil
			}
			return written, err
		}
	}
}

func mbpsFromBytes(bytesCount int64, elapsed time.Duration) float64 {
	seconds := elapsed.Seconds()
	if bytesCount <= 0 || seconds <= 0 {
		return 0
	}
	return float64(bytesCount) * 8 / seconds / 1000 / 1000
}

type timedUploadReader struct {
	deadline   time.Time
	start      time.Time
	data       []byte
	limit      int64
	written    int64
	lastSample time.Time
	sample     speedSampleFunc
}

func newTimedUploadReader(duration time.Duration, limit int, sample speedSampleFunc) *timedUploadReader {
	now := time.Now()
	return &timedUploadReader{
		deadline:   now.Add(duration),
		start:      now,
		data:       make([]byte, 32*1024),
		limit:      int64(limit),
		lastSample: now,
		sample:     sample,
	}
}

func (r *timedUploadReader) Read(p []byte) (int, error) {
	if time.Now().After(r.deadline) || r.written >= r.limit {
		return 0, io.EOF
	}
	if remain := r.limit - r.written; int64(len(p)) > remain {
		p = p[:remain]
	}
	n := copy(p, r.data)
	r.written += int64(n)
	if r.sample != nil && time.Since(r.lastSample) >= speedProgressInterval {
		r.sample(mbpsFromBytes(r.written, time.Since(r.start)))
		r.lastSample = time.Now()
	}
	return n, nil
}

func (r *timedUploadReader) BytesWritten() int64 {
	if r == nil {
		return 0
	}
	return r.written
}

func buildDelayConfig(node Node, port int) map[string]interface{} {
	return buildDelayConfigWithUpstream(node, port, 0)
}

func outboundSupportsDetour(outbound map[string]interface{}) bool {
	if outbound == nil {
		return false
	}
	proxyType := strings.ToLower(strings.TrimSpace(stringFromAny(outbound["type"])))
	switch proxyType {
	case "", "direct", "block", "dns":
		return false
	default:
		return true
	}
}

func isClashAPIDialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "connectex") ||
		strings.Contains(msg, "dial tcp")
}

func isClashAPINotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

func buildDelayConfigWithUpstream(node Node, port int, upstreamPort int) map[string]interface{} {
	proxy := sanitizeOutboundForSingBox(node.Outbound)
	proxy["tag"] = "selected"
	outbounds := []map[string]interface{}{
		proxy,
		{"type": "direct", "tag": "direct"},
	}
	if upstreamPort > 0 && outboundSupportsDetour(proxy) {
		outbounds = append(outbounds, map[string]interface{}{
			"type":        "socks",
			"tag":         "upstream",
			"server":      "127.0.0.1",
			"server_port": upstreamPort,
		})
		proxy["detour"] = "upstream"
	}
	return map[string]interface{}{
		"log": map[string]interface{}{
			"level":     "error",
			"timestamp": true,
		},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{"tag": "local", "type": "local"},
			},
			"final": "local",
		},
		"inbounds": []map[string]interface{}{
			{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": port,
			},
		},
		"outbounds": outbounds,
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"default_domain_resolver": map[string]interface{}{
				"server":   "local",
				"strategy": "prefer_ipv4",
			},
			"rules": []map[string]interface{}{
				{"inbound": []string{"mixed-in"}, "action": "route", "outbound": "selected"},
			},
		},
	}
}

func pickFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
