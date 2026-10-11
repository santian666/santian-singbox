package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	ruleSourceBuiltin      = "builtin"
	ruleSourceLoyalsoldier = "loyalsoldier"
	ruleSourceV2rayN       = "v2rayn"
	ruleSourceSagerNet     = "sagernet"
	ruleSourceMetaCube     = "metacubex"
	clashAPIAddress        = "127.0.0.1:20990"
	clashAPIPort           = 20990
	tunInterfaceName       = "SanTian"
	defaultProxyMapPort    = 52001
	startupPortTimeout     = 30 * time.Second
	tunConnectivityTimeout = 6 * time.Second
	coreVersionTimeout     = 3 * time.Second
	coreCandidateTimeout   = 20 * time.Second
	configCheckTimeout     = 15 * time.Second
	commandOutputTimeout   = 30 * time.Second
)

const (
	loyalsoldierRuleVersion     = "202606042304"
	loyalsoldierRuleDownloadURL = "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/download/" + loyalsoldierRuleVersion
	ruleVersionCheckTimeout     = 5 * time.Second
	ruleFileDownloadTimeout     = 20 * time.Second
	maxRuleDownloadBytes        = 64 * 1024 * 1024
	maxReleaseMetadataBytes     = 4 * 1024 * 1024
	maxCoreDownloadBytes        = 256 * 1024 * 1024
	localProxyDownloadAttempts  = 2
	downloadRetryDelay          = 500 * time.Millisecond
)

var localMetadataIPCIDRs = []string{"169.254.169.254/32"}

var tunConnectivityProbeURLs = []string{
	"https://cp.cloudflare.com/generate_204",
	"https://www.google.com/generate_204",
	"http://www.msftconnecttest.com/connecttest.txt",
}

var tunRouteExcludeIPCIDRs = []string{
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"255.255.255.255/32",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

var tunBlockIPCIDRs = []string{
	"0.0.0.0/8",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"255.255.255.255/32",
	"ff00::/8",
}

type ruleSetDef struct {
	Tag      string
	URL      string
	FileName string
}

type ruleSetBackup struct {
	path   string
	exists bool
	data   []byte
}

type dnsProbeCandidate struct {
	Type   string
	Server string
}

type autoDNSPlan struct {
	China  dnsProbeCandidate
	Remote dnsProbeCandidate
}

var rulePresetDefs = map[string][]ruleSetDef{
	ruleSourceLoyalsoldier: {
		{Tag: "loyalsoldier-proxy", URL: loyalsoldierRuleDownloadURL + "/proxy-list.txt", FileName: "loyalsoldier-proxy.json"},
		{Tag: "loyalsoldier-direct", URL: loyalsoldierRuleDownloadURL + "/direct-list.txt", FileName: "loyalsoldier-direct.json"},
		{Tag: "loyalsoldier-china", URL: loyalsoldierRuleDownloadURL + "/china-list.txt", FileName: "loyalsoldier-china.json"},
		{Tag: "geoip-cn", URL: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt", FileName: "geoip-cn.json"},
	},
}

var chinaDNSCandidates = []dnsProbeCandidate{
	{Type: "udp", Server: "223.5.5.5"},
	{Type: "udp", Server: "119.29.29.29"},
	{Type: "udp", Server: "180.76.76.76"},
}

var remoteDNSCandidates = []dnsProbeCandidate{
	{Type: "https", Server: "1.1.1.1"},
	{Type: "https", Server: "1.0.0.1"},
}

func RulePresets() []RulePreset {
	return []RulePreset{
		{ID: ruleSourceLoyalsoldier, Name: "Loyalsoldier", Description: "使用 v2ray-rules-dat " + loyalsoldierRuleVersion + " 规则"},
		{ID: ruleSourceBuiltin, Name: "内置简单规则", Description: "不拉取远程规则，仅使用内置直连规则"},
	}
}

func normalizeSettings(settings *Settings) {
	if settings.Mode != "rule" && settings.Mode != "global" {
		settings.Mode = "rule"
	}
	enforceProxyEntryExclusivity(settings)
	if settings.MixedPort == 0 {
		settings.MixedPort = 20808
	}
	if settings.ProxyMapStartPort == 0 {
		settings.ProxyMapStartPort = defaultProxyMapPort
	}
	settings.ProxyMapGroupIDs = normalizeProxyMapGroupIDs(settings.ProxyMapGroupIDs, settings.ProxyMapGroupID)
	if len(settings.ProxyMapGroupIDs) > 0 {
		settings.ProxyMapGroupID = settings.ProxyMapGroupIDs[0]
	} else {
		settings.ProxyMapGroupID = ""
	}
	if settings.RuleSource == "" {
		settings.RuleSource = ruleSourceLoyalsoldier
	}
	if settings.RuleSource == ruleSourceV2rayN || settings.RuleSource == ruleSourceSagerNet || settings.RuleSource == ruleSourceMetaCube {
		settings.RuleSource = ruleSourceLoyalsoldier
	}
	if _, ok := rulePresetDefs[settings.RuleSource]; !ok && settings.RuleSource != ruleSourceBuiltin {
		settings.RuleSource = ruleSourceLoyalsoldier
	}
	if settings.RuleUpdateInterval == "" {
		settings.RuleUpdateInterval = "168h"
	}
	if settings.CoreLogLevel != "" && settings.CoreLogLevel != "debug" && settings.CoreLogLevel != "info" && settings.CoreLogLevel != "warn" && settings.CoreLogLevel != "error" {
		settings.CoreLogLevel = ""
	}
	if settings.Theme != "" && settings.Theme != "dark" {
		settings.Theme = "light"
	}
	settings.AppProxyTargets = normalizeAppProxyTargets(settings.AppProxyTargets)
	settings.ExternalProxyEndpoints = normalizeExternalProxyEndpoints(settings.ExternalProxyEndpoints)
	settings.DirectDomains = strings.Join(normalizeDirectDomains(settings.DirectDomains), "\n")
}

func enforceProxyEntryExclusivity(settings *Settings) {
	if settings == nil {
		return
	}
	if settings.Tun {
		settings.SystemProxy = false
		settings.AppProxy = false
		return
	}
	if settings.AppProxy {
		settings.SystemProxy = false
		settings.Tun = false
	}
}

func enforceProxyEntryTransition(current Settings, next *Settings) {
	if next == nil {
		return
	}
	switch {
	case next.SystemProxy && !current.SystemProxy:
		next.Tun = false
		next.AppProxy = false
	case next.Tun && !current.Tun:
		next.SystemProxy = false
		next.AppProxy = false
	case next.AppProxy && !current.AppProxy:
		next.SystemProxy = false
		next.Tun = false
	default:
		enforceProxyEntryExclusivity(next)
	}
}

func appProxyActive(settings Settings) bool {
	return settings.AppProxy && len(enabledAppProxyTargets(settings.AppProxyTargets)) > 0
}

func effectiveTun(settings Settings) bool {
	return settings.Tun || appProxyActive(settings)
}

func appProxyOnlyTun(settings Settings) bool {
	return appProxyActive(settings) && !settings.Tun
}

type SingBoxManager struct {
	mu                  sync.Mutex
	rootDir             string
	corePath            string
	cachePath           string
	coreVer             string
	latestCoreVer       string
	latestCoreCheckedAt time.Time
	latestCoreCheckErr  string
	latestCoreChecking  bool
	ruleVersionChecker  func(string) (string, error)
	config              string
	cmd                 *exec.Cmd
	processJob          uintptr
	cancel              chan struct{}
	done                chan error
	tunActive           bool
	logger              *Logger
	traffic             trafficCache
	socks5TestProxyPort int
	lastMixedPort       int
	proxyMapStartPort   int
	proxyMapPortCount   int
	runtimeOutboundTags map[string]struct{}
	blockLogCount       int
	lastBlockLogAt      time.Time
	recentBlockedTargets map[string]time.Time
	remoteLogCount      int
	lastRemoteLogAt     time.Time
	localCloseLogCount     int
	lastLocalCloseLogAt    time.Time
	missingFakeIPLogCount  int
	lastMissingFakeIPLogAt time.Time
}

type TrafficSnapshot struct {
	UploadRate    int64 `json:"uploadRate"`
	DownloadRate  int64 `json:"downloadRate"`
	UploadTotal   int64 `json:"uploadTotal"`
	DownloadTotal int64 `json:"downloadTotal"`
}

type trafficCache struct {
	snapshot TrafficSnapshot
	lastTime time.Time
}

func NewSingBoxManager(rootDir, corePath, config, cachePath string, logger *Logger) *SingBoxManager {
	return &SingBoxManager{
		rootDir:   rootDir,
		corePath:  corePath,
		cachePath: cachePath,
		config:    config,
		logger:    logger,
	}
}

func (m *SingBoxManager) SetSOCKS5TestProxyPort(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.socks5TestProxyPort = port
}

func (m *SingBoxManager) getSOCKS5TestProxyPort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.socks5TestProxyPort
}

func (m *SingBoxManager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

func (m *SingBoxManager) CurrentPID() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

func (m *SingBoxManager) RuntimeHasOutbound(tag string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.runtimeOutboundTags[strings.TrimSpace(tag)]
	return ok
}

func (m *SingBoxManager) HealthCheck(settings Settings) error {
	normalizeSettings(&settings)
	m.mu.Lock()
	running := m.cmd != nil && m.cmd.Process != nil
	proxyMapStartPort := m.proxyMapStartPort
	proxyMapPortCount := m.proxyMapPortCount
	m.mu.Unlock()
	if !running {
		return errors.New("sing-box 内核未运行")
	}
	if !tcpPortOpen("127.0.0.1", settings.MixedPort, 800*time.Millisecond) {
		return fmt.Errorf("本地代理端口 %d 未监听", settings.MixedPort)
	}
	if err := m.pingClashAPI(); err != nil {
		return err
	}
	if settings.ProxyMap {
		if proxyMapPortCount <= 0 {
			return errors.New("代理映射运行端口未初始化")
		}
		ports := sequentialPorts(proxyMapStartPort, proxyMapPortCount)
		if port := firstUnboundTCPPort("127.0.0.1", ports); port != 0 {
			return fmt.Errorf("代理映射端口 %d 未监听", port)
		}
	}
	return nil
}

func (m *SingBoxManager) pingClashAPI() error {
	client := http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get("http://" + clashAPIAddress + "/configs")
	if err != nil {
		return fmt.Errorf("内核控制接口不可用：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("内核控制接口异常：HTTP %d", resp.StatusCode)
	}
	return nil
}

func (m *SingBoxManager) TrafficSnapshot() (TrafficSnapshot, error) {
	total, err := m.fetchTrafficTotal()
	if err != nil {
		return TrafficSnapshot{}, err
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	last := m.traffic.snapshot
	elapsed := now.Sub(m.traffic.lastTime).Seconds()
	snapshot := total
	if !m.traffic.lastTime.IsZero() && elapsed > 0 {
		uploadDelta := total.UploadTotal - last.UploadTotal
		downloadDelta := total.DownloadTotal - last.DownloadTotal
		if uploadDelta > 0 {
			snapshot.UploadRate = int64(float64(uploadDelta) / elapsed)
		}
		if downloadDelta > 0 {
			snapshot.DownloadRate = int64(float64(downloadDelta) / elapsed)
		}
	}
	m.traffic = trafficCache{snapshot: snapshot, lastTime: now}
	return snapshot, nil
}

func (m *SingBoxManager) selectOutbound(tag string, fast bool) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("节点出站标识为空")
	}
	payload, err := json.Marshal(map[string]string{"name": tag})
	if err != nil {
		return err
	}
	req, err := newClashAPIRequest(http.MethodPut, "http://"+clashAPIAddress+"/proxies/proxy", payload)
	if err != nil {
		return err
	}
	if !fast {
		return doClashAPIOnce(req, 1500*time.Millisecond)
	}
	return doClashAPIOnce(req, 700*time.Millisecond)
}

func (m *SingBoxManager) CurrentOutbound() (string, error) {
	return m.currentOutbound(true)
}

func (m *SingBoxManager) currentOutbound(fast bool) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "http://"+clashAPIAddress+"/proxies/proxy", nil)
	if err != nil {
		return "", err
	}
	var data []byte
	if fast {
		data, err = doClashAPIBytesOnce(req, 700*time.Millisecond)
	} else {
		data, err = doClashAPIBytesOnce(req, 1500*time.Millisecond)
	}
	if err != nil {
		return "", err
	}
	var result struct {
		Now string `json:"now"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Now), nil
}

func (m *SingBoxManager) SelectOutboundVerified(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("节点出站标识为空")
	}
	fastErr := m.selectOutbound(tag, true)
	if outboundSelectionMatches(tag, m.currentOutbound, 80*time.Millisecond, true) {
		return nil
	}
	reliableErr := m.selectOutbound(tag, false)
	if outboundSelectionMatches(tag, m.currentOutbound, 150*time.Millisecond, true) ||
		outboundSelectionMatches(tag, m.currentOutbound, 300*time.Millisecond, false) {
		return nil
	}
	if fastErr == nil {
		fastErr = errors.New("selector 未更新为目标节点")
	}
	if reliableErr == nil {
		reliableErr = errors.New("selector 重试后仍未更新为目标节点")
	}
	return fmt.Errorf("节点切换未确认成功：快速确认失败：%v；稳定重试失败：%w", fastErr, reliableErr)
}

func outboundSelectionMatches(tag string, current func(bool) (string, error), wait time.Duration, fast bool) bool {
	if wait > 0 {
		time.Sleep(wait)
	}
	selected, err := current(fast)
	return err == nil && strings.TrimSpace(selected) == strings.TrimSpace(tag)
}

func (m *SingBoxManager) SelectOutboundReliable(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("节点出站标识为空")
	}
	return confirmOutboundSelection(
		tag,
		func() error { return m.selectOutbound(tag, false) },
		func() (string, error) { return m.currentOutbound(false) },
		2500*time.Millisecond,
		150*time.Millisecond,
	)
}

func confirmOutboundSelection(
	tag string,
	selectOutbound func() error,
	currentOutbound func() (string, error),
	confirmTimeout time.Duration,
	pollInterval time.Duration,
) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("节点出站标识为空")
	}
	if confirmTimeout <= 0 {
		confirmTimeout = time.Second
	}
	if pollInterval <= 0 {
		pollInterval = 50 * time.Millisecond
	}
	// 命令返回 EOF 或超时时也可能已被内核接收，因此仍需读取 selector 状态确认。
	lastErr := selectOutbound()
	deadline := time.Now().Add(confirmTimeout)
	for {
		current, err := currentOutbound()
		if err == nil {
			current = strings.TrimSpace(current)
			if current == tag {
				return nil
			}
			lastErr = fmt.Errorf("selector 当前节点仍为 %s", current)
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		remaining := time.Until(deadline)
		pause := pollInterval
		if remaining < pause {
			pause = remaining
		}
		if pause > 0 {
			time.Sleep(pause)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("selector 未返回当前节点")
	}
	return lastErr
}

func (m *SingBoxManager) SetMode(mode string) error {
	wanted := clashMode(mode)
	payload, err := json.Marshal(map[string]string{"mode": wanted})
	if err != nil {
		return err
	}
	req, err := newClashAPIRequest(http.MethodPatch, "http://"+clashAPIAddress+"/configs", payload)
	if err != nil {
		return err
	}
	applyErr := doClashAPIFast(req)
	confirmErr := confirmModeSelection(wanted, m.currentMode, 1200*time.Millisecond, 80*time.Millisecond)
	if confirmErr == nil {
		return nil
	}
	if applyErr != nil {
		return fmt.Errorf("模式切换请求失败：%v；回读确认失败：%w", applyErr, confirmErr)
	}
	return fmt.Errorf("模式切换未确认生效：%w", confirmErr)
}

func (m *SingBoxManager) currentMode() (string, error) {
	req, err := http.NewRequest(http.MethodGet, "http://"+clashAPIAddress+"/configs", nil)
	if err != nil {
		return "", err
	}
	data, err := doClashAPIBytesOnce(req, 500*time.Millisecond)
	if err != nil {
		return "", err
	}
	var result struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(result.Mode)), nil
}

func confirmModeSelection(wanted string, current func() (string, error), timeout, pollInterval time.Duration) error {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	if wanted == "" {
		return errors.New("目标模式为空")
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	if pollInterval <= 0 {
		pollInterval = 50 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		actual, err := current()
		if err == nil {
			actual = strings.ToLower(strings.TrimSpace(actual))
			if actual == wanted {
				return nil
			}
			lastErr = fmt.Errorf("当前模式仍为 %s", firstNonEmpty(actual, "未知"))
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}
	return lastErr
}

func (m *SingBoxManager) CloseConnections() error {
	req, err := http.NewRequest(http.MethodDelete, "http://"+clashAPIAddress+"/connections", nil)
	if err != nil {
		return err
	}
	return doClashAPIFast(req)
}

func newClashAPIRequest(method string, url string, payload []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func doClashAPI(req *http.Request) error {
	_, err := doClashAPIBytes(req)
	return err
}

func doClashAPIFast(req *http.Request) error {
	_, err := doClashAPIBytesWithRetry(req, 1200*time.Millisecond, 500*time.Millisecond, 60*time.Millisecond)
	return err
}

func doClashAPIBytes(req *http.Request) ([]byte, error) {
	return doClashAPIBytesWithRetry(req, 3*time.Second, 1500*time.Millisecond, 150*time.Millisecond)
}

func doClashAPIBytesFast(req *http.Request) ([]byte, error) {
	return doClashAPIBytesWithRetry(req, 1200*time.Millisecond, 500*time.Millisecond, 60*time.Millisecond)
}

func doClashAPIOnce(req *http.Request, timeout time.Duration) error {
	_, err := doClashAPIBytesOnce(req, timeout)
	return err
}

func doClashAPIBytesOnce(req *http.Request, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	tryReq := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		tryReq.Body = body
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(tryReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limit := int64(1 << 20)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limit = 512
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, nil
	}
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}
	return nil, fmt.Errorf("Clash API 返回 %d：%s", resp.StatusCode, message)
}

func doClashAPIBytesWithRetry(req *http.Request, totalTimeout time.Duration, requestTimeout time.Duration, retryDelay time.Duration) ([]byte, error) {
	deadline := time.Now().Add(totalTimeout)
	for {
		ctx := req.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		tryReq := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			tryReq.Body = body
		}
		client := http.Client{Timeout: requestTimeout}
		resp, err := client.Do(tryReq)
		if err == nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				_ = resp.Body.Close()
				return data, nil
			}
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			message := strings.TrimSpace(string(data))
			if message == "" {
				message = resp.Status
			}
			err := fmt.Errorf("Clash API 返回 %d：%s", resp.StatusCode, message)
			if resp.StatusCode >= 500 && time.Now().Before(deadline) {
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}
		if time.Now().After(deadline) || !isRetryableClashAPIError(err) {
			return nil, err
		}
		time.Sleep(retryDelay)
	}
}

func isRetryableClashAPIError(err error) bool {
	return isRetryableNetworkError(err)
}

func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "actively refused") ||
		strings.Contains(message, "connectex") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "forcibly closed") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.TrimSpace(message) == "eof"
}

func (m *SingBoxManager) CoreVersion() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.coreVer != "" {
		return m.coreVer
	}

	if _, err := os.Stat(m.corePath); err != nil {
		return "未安装"
	}
	ctx, cancel := context.WithTimeout(context.Background(), coreVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.corePath, "version")
	cmd.Dir = m.rootDir
	hideCommandWindow(cmd)
	out, err := cmd.Output()
	version := "未知"
	if err != nil {
		return version
	}
	version = strings.TrimSpace(strings.Split(string(out), "\n")[0])
	m.coreVer = version
	return version
}

func (m *SingBoxManager) CoreUpdateStatus() (latest string, available bool) {
	current := normalizeCoreVersionTag(m.CoreVersion())
	if current == "" {
		return "", false
	}

	m.mu.Lock()
	latest = m.latestCoreVer
	checkedAt := m.latestCoreCheckedAt
	shouldCheck := !m.latestCoreChecking && (latest == "" || time.Since(checkedAt) >= time.Hour)
	if shouldCheck {
		m.latestCoreChecking = true
	}
	m.mu.Unlock()

	if shouldCheck {
		go m.refreshCoreUpdateStatus()
	}
	return latest, normalizeCoreVersionTag(latest) != current
}

func (m *SingBoxManager) refreshCoreUpdateStatus() {
	m.mu.Lock()
	proxyPort := m.socks5TestProxyPort
	m.mu.Unlock()

	_, latest, err := latestCoreAsset(proxyPort)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latestCoreCheckedAt = time.Now()
	m.latestCoreChecking = false
	if err != nil {
		m.latestCoreCheckErr = err.Error()
		return
	}
	m.latestCoreVer = latest
	m.latestCoreCheckErr = ""
}

type stagedFilePair struct {
	targetPath string
	stagedPath string
}

type coreUpdateCandidate struct {
	version     string
	stagedPath  string
	stagedFiles []stagedFilePair
}

func (candidate *coreUpdateCandidate) Cleanup() {
	if candidate == nil {
		return
	}
	if candidate.stagedPath != "" {
		_ = os.Remove(candidate.stagedPath)
	}
	for _, pair := range candidate.stagedFiles {
		if pair.stagedPath != "" {
			_ = os.Remove(pair.stagedPath)
		}
	}
}

func validateCoreCandidate(executable, rootDir, configPath, expectedVersion string, checkConfig bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), coreCandidateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "version")
	cmd.Dir = rootDir
	hideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("新内核版本检查超时（%d 秒）", int(coreCandidateTimeout/time.Second))
		}
		return fmt.Errorf("新内核无法运行：%w", err)
	}
	actualVersion := normalizeCoreVersionTag(string(out))
	if actualVersion == "" || actualVersion != normalizeCoreVersionTag(expectedVersion) {
		return fmt.Errorf("新内核版本不匹配：期望 %s，实际 %s", expectedVersion, strings.TrimSpace(string(out)))
	}
	if checkConfig {
		if err := checkCoreConfigExecutable(executable, rootDir, configPath); err != nil {
			return fmt.Errorf("新内核与当前配置不兼容：%w", err)
		}
	}
	return nil
}

func (m *SingBoxManager) PrepareCoreUpdate() (string, *coreUpdateCandidate, error) {
	m.mu.Lock()
	proxyPort := m.socks5TestProxyPort
	m.mu.Unlock()

	asset, version, err := latestCoreAsset(proxyPort)
	if err != nil {
		return "", nil, err
	}
	if current := normalizeCoreVersionTag(m.CoreVersion()); current != "" && current == normalizeCoreVersionTag(version) {
		if m.logger != nil {
			m.logger.Info("sing-box 内核已是最新版本：%s", version)
		}
		return version, nil, nil
	}
	tmpDir, err := os.MkdirTemp("", "go-singbox-core-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "sing-box.zip")
	if err := downloadCoreAsset(asset, version, zipPath, proxyPort, m.logger); err != nil {
		return "", nil, fmt.Errorf("下载 sing-box %s 失败：%w", version, err)
	}
	extracted, err := extractCore(zipPath, tmpDir)
	if err != nil {
		return "", nil, err
	}
	binDir := filepath.Dir(m.corePath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", nil, err
	}
	stagedPath := m.corePath + ".new"
	_ = os.Remove(stagedPath)
	if err := copyFile(extracted.coreExe, stagedPath); err != nil {
		return "", nil, fmt.Errorf("准备新内核失败：%w", err)
	}
	stagedPairs := []stagedFilePair{
		{
			targetPath: m.corePath,
			stagedPath: stagedPath,
		},
	}
	for _, comp := range extracted.companionFiles {
		compTarget := filepath.Join(binDir, filepath.Base(comp))
		compStaged := compTarget + ".new"
		_ = os.Remove(compStaged)
		if err := copyFile(comp, compStaged); err != nil {
			_ = os.Remove(stagedPath)
			for _, p := range stagedPairs {
				_ = os.Remove(p.stagedPath)
			}
			return "", nil, fmt.Errorf("准备新内核配套组件失败：%w", err)
		}
		stagedPairs = append(stagedPairs, stagedFilePair{
			targetPath: compTarget,
			stagedPath: compStaged,
		})
	}
	candidate := &coreUpdateCandidate{
		version:     version,
		stagedPath:  stagedPath,
		stagedFiles: stagedPairs,
	}
	if err := validateCoreCandidate(stagedPath, m.rootDir, m.config, version, m.Running()); err != nil {
		candidate.Cleanup()
		return "", nil, err
	}
	if m.logger != nil {
		m.logger.Info("sing-box %s 已下载并校验，当前代理保持运行", version)
	}
	return version, candidate, nil
}

func (m *SingBoxManager) ApplyCoreUpdate(candidate *coreUpdateCandidate, state PersistState) error {
	if candidate == nil || (candidate.stagedPath == "" && len(candidate.stagedFiles) == 0) {
		return errors.New("待安装的 sing-box 内核无效")
	}
	normalizeSettings(&state.Settings)
	m.mu.Lock()
	defer m.mu.Unlock()

	wasRunning := m.cmd != nil && m.cmd.Process != nil
	cleanupTun := wasRunning && effectiveTun(state.Settings)
	if wasRunning {
		if m.logger != nil {
			m.logger.Info("新内核已准备完成，正在快速切换 sing-box")
		}
		if err := m.stopLocked(false, cleanupTun, true); err != nil {
			return fmt.Errorf("替换内核前停止 sing-box 失败：%w", err)
		}
	}

	pairs := candidate.stagedFiles
	if len(pairs) == 0 && candidate.stagedPath != "" {
		pairs = []stagedFilePair{{targetPath: m.corePath, stagedPath: candidate.stagedPath}}
	}
	swapped, err := swapCoreFiles(pairs)
	if err != nil {
		if wasRunning {
			if startErr := m.startLocked(state); startErr != nil {
				_ = m.stopLocked(true, true, true)
				return errors.Join(fmt.Errorf("替换 sing-box 组件失败：%w", err), fmt.Errorf("恢复旧内核代理失败：%w", startErr))
			}
		}
		return fmt.Errorf("替换 sing-box 组件失败：%w", err)
	}
	candidate.stagedPath = ""
	candidate.stagedFiles = nil
	m.coreVer = ""

	if wasRunning {
		if err := m.startLocked(state); err != nil {
			_ = m.stopLocked(false, cleanupTun, true)
			rollbackErr := rollbackCoreFiles(swapped)
			m.coreVer = ""
			if rollbackErr != nil {
				_ = m.stopLocked(true, true, true)
				return errors.Join(fmt.Errorf("新内核启动失败：%w", err), fmt.Errorf("恢复旧内核文件失败：%w", rollbackErr))
			}
			if restoreErr := m.startLocked(state); restoreErr != nil {
				_ = m.stopLocked(true, true, true)
				return errors.Join(fmt.Errorf("新内核启动失败：%w", err), fmt.Errorf("旧内核恢复启动失败：%w", restoreErr))
			}
			return fmt.Errorf("新内核启动失败，已恢复旧内核和代理：%w", err)
		}
	}
	cleanupCoreBackups(swapped)
	m.coreVer = ""
	m.latestCoreVer = candidate.version
	m.latestCoreCheckedAt = time.Now()
	m.latestCoreCheckErr = ""
	if m.logger != nil {
		m.logger.Info("sing-box 内核已更新到 %s", candidate.version)
	}
	return nil
}

func (m *SingBoxManager) Start(state PersistState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked(state)
}

func (m *SingBoxManager) startLocked(state PersistState) error {
	normalizeSettings(&state.Settings)
	if m.cmd != nil && m.cmd.Process != nil {
		return errors.New("sing-box 已在运行")
	}
	node, ok := findSelectedNode(state.Nodes, state.SelectedID)
	if !ok {
		if len(state.Nodes) == 0 {
			return errors.New("请先导入并选择一个代理节点")
		}
		return errors.New("请先选择节点")
	}
	if _, err := os.Stat(m.corePath); err != nil {
		return fmt.Errorf("未找到 sing-box 内核：%s", m.corePath)
	}
	proxyMapNodes := proxyMapNodesForSettings(state.Nodes, state.Settings)
	if err := validateProxyMapSettings(state.Settings, len(proxyMapNodes)); err != nil {
		return err
	}
	if err := validateProxyMapPorts(state.Settings, len(proxyMapNodes)); err != nil {
		return err
	}
	dnsPlan := defaultAutoDNSPlan()
	if err := m.ensureRuleSets(state.Settings); err != nil {
		return err
	}
	config, err := buildSingBoxConfig(state, node, m.cachePath, true, dnsPlan)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.config), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.cachePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.config, data, 0644); err != nil {
		return err
	}
	externalProxyCount := 0
	for _, endpoint := range state.Settings.ExternalProxyEndpoints {
		if endpoint.Enabled {
			externalProxyCount++
		}
	}
	if externalProxyCount > 0 {
		m.logger.Info("运行配置已写入：外部代理入口直连 %d 个", externalProxyCount)
	}
	m.lastMixedPort = state.Settings.MixedPort
	m.proxyMapStartPort = 0
	m.proxyMapPortCount = 0
	if state.Settings.ProxyMap && len(proxyMapNodes) > 0 {
		m.proxyMapStartPort = state.Settings.ProxyMapStartPort
		m.proxyMapPortCount = len(proxyMapNodes)
	}
	if err := m.checkConfig(); err != nil {
		if repaired, repairErr := m.tryAutoRepairOutboundConfig(err, node); repairErr == nil && repaired {
			if m.logger != nil {
				m.logger.Info("内核配置包含异常节点，已自动隔离修复并成功通过校验")
			}
		} else {
			return err
		}
	}
	if !isTCPPortFree("127.0.0.1", state.Settings.MixedPort) {
		return fmt.Errorf("本地代理端口 %d 已被占用，请在运行设置中修改端口或关闭占用该端口的程序", state.Settings.MixedPort)
	}
	if !isTCPPortFree("127.0.0.1", clashAPIPort) {
		return fmt.Errorf("内核控制端口 %d 已被占用，请停止残留 sing-box 后重试", clashAPIPort)
	}

	cmd := exec.Command(m.corePath, "run", "-c", m.config)
	cmd.Dir = m.rootDir
	hideCommandWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	processJob, err := createKillOnCloseJob(cmd.Process)
	if err != nil {
		processJob = 0
		m.logger.Error("无法建立 sing-box Job Object 保护，将由独立 watchdog 兜底：%v", err)
	}
	cancel := make(chan struct{})
	done := make(chan error, 1)
	m.cmd = cmd
	m.processJob = processJob
	m.cancel = cancel
	m.done = done
	m.traffic = trafficCache{}
	safeGo(m.logger, "读取内核标准输出", func() {
		m.pipeLog(cancel, stdout)
	})
	safeGo(m.logger, "读取内核错误输出", func() {
		m.pipeLog(cancel, stderr)
	})
	safeGo(m.logger, "监听内核退出", func() {
		m.watchExit(cmd, done)
	})

	if err := waitPortOrProcessExit("127.0.0.1", state.Settings.MixedPort, startupPortTimeout, done); err != nil {
		reason := m.stopLocked(true, true, true)
		if reason != nil {
			return fmt.Errorf("sing-box 启动后已退出：%w", reason)
		}
		return fmt.Errorf("sing-box 未能监听本地代理端口 %d，请检查端口是否被占用、节点配置是否可用或内核初始化是否超时：%w", state.Settings.MixedPort, err)
	}
	if effectiveTun(state.Settings) && runtime.GOOS == "windows" {
		if err := waitTunReady(15 * time.Second); err != nil {
			_ = m.stopLocked(true, true, true)
			return fmt.Errorf("TUN 未能真实接管系统网络：%w", err)
		}
		if err := verifyLocalProxyConnectivity(state.Settings.MixedPort, tunConnectivityTimeout); err != nil {
			_ = m.stopLocked(true, true, true)
			return fmt.Errorf("TUN 已接管路由，但当前节点无法建立可用网络，已自动清理 TUN：%w", err)
		}
		closeStaleFakeIPTCPConnections(m.logger)
		flushWindowsDNSCache(m.logger)
		m.logger.Info("TUN 已接管系统网络：%s", tunInterfaceName)
	}
	if state.Settings.ProxyMap && len(proxyMapNodes) > 0 {
		ports := sequentialPorts(state.Settings.ProxyMapStartPort, len(proxyMapNodes))
		port, err := waitPortsOrProcessExit("127.0.0.1", ports, startupPortTimeout, done)
		if err != nil {
			_ = m.stopLocked(true, true, true)
			return fmt.Errorf("代理映射端口 %d 未能监听：%w", port, err)
		}
	}
	if err := m.ValidateRuntimeSettings(state.Settings); err != nil {
		_ = m.stopLocked(true, true, true)
		return fmt.Errorf("运行配置校验失败：%w", err)
	}
	if externalProxyCount > 0 {
		m.logger.Info("运行配置已回读验证：外部代理入口直连规则已生效")
	}
	if state.Settings.SystemProxy && runtime.GOOS == "windows" {
		if err := EnableSystemProxy(state.Settings.MixedPort); err != nil {
			_ = m.stopLocked(true, true, true)
			return err
		}
		m.logger.Info("系统代理已开启：127.0.0.1:%d", state.Settings.MixedPort)
	}
	m.tunActive = effectiveTun(state.Settings)
	m.runtimeOutboundTags = runtimeOutboundTagSet(state.Nodes)
	m.logger.Info("sing-box 已启动，本地混合代理端口：%d", state.Settings.MixedPort)
	return nil
}

func (m *SingBoxManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked(true, m.tunActive, m.tunActive)
}

func (m *SingBoxManager) StopWithoutSystemProxyCleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked(false, m.tunActive, m.tunActive)
}

func (m *SingBoxManager) Restart(state PersistState, cleanupTun bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	normalizeSettings(&state.Settings)
	if effectiveTun(state.Settings) {
		cleanupTun = true
	}
	if err := m.stopLocked(false, cleanupTun, cleanupTun); err != nil {
		return err
	}
	waitTCPPortFree("127.0.0.1", state.Settings.MixedPort, 3*time.Second)
	waitTCPPortFree("127.0.0.1", clashAPIPort, 3*time.Second)
	return m.startLocked(state)
}

func (m *SingBoxManager) CleanupStale() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return nil
	}
	pids, err := staleSingBoxPIDsByPath(m.corePath)
	if err != nil {
		return fmt.Errorf("查找残留 sing-box 失败：%w", err)
	}
	stopped := make([]string, 0, len(pids))
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := process.Kill(); err == nil {
			stopped = append(stopped, strconv.Itoa(pid))
			continue
		}
		if _, err := runCommandOutput("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F"); err == nil {
			stopped = append(stopped, strconv.Itoa(pid))
		}
	}
	if len(stopped) > 0 {
		if m.logger != nil {
			m.logger.Info("已清理残留 sing-box 进程：%s", strings.Join(stopped, ","))
		}
		waitProcessPathGone(m.corePath, 3*time.Second)
	}
	remaining, err := staleSingBoxPIDsByPath(m.corePath)
	if err != nil {
		return fmt.Errorf("复查残留 sing-box 失败：%w", err)
	}
	if len(remaining) > 0 {
		ids := make([]string, 0, len(remaining))
		for _, pid := range remaining {
			ids = append(ids, strconv.Itoa(pid))
		}
		return fmt.Errorf("仍有 sing-box 进程未退出：%s", strings.Join(ids, ","))
	}
	return nil
}

func staleSingBoxPIDsByPath(corePath string) ([]int, error) {
	target := normalizeAppProxyPath(corePath)
	if target == "" {
		return nil, nil
	}
	processes, err := listProcesses()
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0)
	for _, item := range processes {
		if item.PID <= 0 || !strings.EqualFold(item.Name, "sing-box.exe") {
			continue
		}
		if strings.EqualFold(normalizeAppProxyPath(item.Path), target) {
			pids = append(pids, item.PID)
		}
	}
	return pids, nil
}

func (m *SingBoxManager) stopLocked(disableSystemProxy bool, cleanupTun bool, waitProcessGone bool) error {
	m.proxyMapStartPort = 0
	m.proxyMapPortCount = 0
	m.runtimeOutboundTags = nil
	m.tunActive = false
	if m.cancel != nil {
		defer close(m.cancel)
		m.cancel = nil
	}
	if m.cmd == nil || m.cmd.Process == nil {
		m.traffic = trafficCache{}
		if runtime.GOOS == "windows" {
			if cleanupTun {
				cleanupTunInterface(m.logger)
			}
			if disableSystemProxy {
				m.disableSystemProxyIfOwnedLocked()
			}
		}
		return nil
	}
	cmd := m.cmd
	processJob := m.processJob
	done := m.done
	m.cmd = nil
	m.processJob = 0
	m.done = nil
	m.traffic = trafficCache{}
	err := terminateProcess(cmd)
	if err != nil && strings.Contains(err.Error(), "process already finished") {
		err = nil
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			m.logger.Error("等待 sing-box 退出超时，继续清理")
		}
	}
	closeKillOnCloseJob(processJob)
	if runtime.GOOS == "windows" && waitProcessGone {
		waitProcessPathGone(m.corePath, 3*time.Second)
	}
	if runtime.GOOS == "windows" {
		if cleanupTun {
			cleanupTunInterface(m.logger)
		}
		if disableSystemProxy {
			m.disableSystemProxyIfOwnedLocked()
		}
	}
	m.logger.Info("sing-box 已停止")
	return err
}

func runtimeOutboundTagSet(nodes []Node) map[string]struct{} {
	tags := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if tag := nodeOutboundTag(node); tag != "" {
			tags[tag] = struct{}{}
		}
	}
	return tags
}

func (m *SingBoxManager) disableSystemProxyIfOwnedLocked() {
	if m.lastMixedPort <= 0 {
		return
	}
	ok, err := systemProxyEnabledForPort(m.lastMixedPort)
	if err != nil {
		if m.logger != nil {
			m.logger.Error("读取系统代理失败：%v", err)
		}
		return
	}
	if !ok {
		return
	}
	if err := DisableSystemProxy(); err != nil {
		if m.logger != nil {
			m.logger.Error("关闭系统代理失败：%v", err)
		}
	}
}

func (m *SingBoxManager) checkConfig() error {
	return checkCoreConfigExecutable(m.corePath, m.rootDir, m.config)
}

func checkCoreConfigExecutable(executable, rootDir, configPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), configCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "check", "-c", configPath)
	cmd.Dir = rootDir
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("sing-box 配置检查超时")
		}
		return fmt.Errorf("sing-box 配置检查失败：%s", strings.TrimSpace(string(out)))
	}
	return nil
}

var outboundErrRegex = regexp.MustCompile(`initialize outbound\[(\d+)\]`)

func (m *SingBoxManager) tryAutoRepairOutboundConfig(initialErr error, selected Node) (bool, error) {
	if initialErr == nil {
		return false, nil
	}
	currentErr := initialErr
	repairedAny := false
	selectedTag := nodeOutboundTag(selected)

	for attempt := 0; attempt < 10; attempt++ {
		match := outboundErrRegex.FindStringSubmatch(currentErr.Error())
		if len(match) != 2 {
			break
		}
		failedIdx, err := strconv.Atoi(match[1])
		if err != nil {
			break
		}

		data, err := os.ReadFile(m.config)
		if err != nil {
			break
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal(data, &cfg); err != nil {
			break
		}
		outbounds, ok := cfg["outbounds"].([]interface{})
		if !ok || failedIdx < 0 || failedIdx >= len(outbounds) {
			break
		}
		badOutbound, ok := outbounds[failedIdx].(map[string]interface{})
		if !ok {
			break
		}
		badTag := stringFromAny(badOutbound["tag"])
		if badTag == selectedTag || badTag == "proxy" || badTag == "direct" || badTag == "block" {
			break
		}

		newOutbounds := append(outbounds[:failedIdx], outbounds[failedIdx+1:]...)
		cfg["outbounds"] = newOutbounds

		for _, ob := range newOutbounds {
			if obMap, ok := ob.(map[string]interface{}); ok && obMap["type"] == "selector" {
				if tags, ok := obMap["outbounds"].([]interface{}); ok {
					var newTags []interface{}
					for _, t := range tags {
						if stringFromAny(t) != badTag {
							newTags = append(newTags, t)
						}
					}
					obMap["outbounds"] = newTags
				}
			}
		}

		newData, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			break
		}
		if err := atomicWriteFile(m.config, newData, 0644); err != nil {
			break
		}

		repairedAny = true
		if m.logger != nil {
			m.logger.Info("节点 [%s] 导致内核校验失败 (%v)，已自动从内核运行时配置中排除", badTag, currentErr)
		}

		currentErr = m.checkConfig()
		if currentErr == nil {
			return true, nil
		}
	}

	return repairedAny, currentErr
}

func (m *SingBoxManager) pipeLog(cancel <-chan struct{}, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-cancel:
			return
		default:
			m.logSingboxLine(scanner.Text())
		}
	}
}

func formatSingboxLogLine(line string) string {
	return strings.TrimSpace(stripANSI(line))
}

func (m *SingBoxManager) logSingboxLine(line string) {
	text := formatSingboxLogLine(line)
	if text == "" {
		return
	}
	if isSingboxErrorLog(text) {
		m.logger.Error("内核：%s", text)
		return
	}
	m.logger.Info("内核：%s", text)
}

func isSingboxErrorLog(text string) bool {
	if isSingboxBlockLog(text) {
		return false
	}
	if isSingboxLocalClientCloseLog(text) {
		return false
	}
	if isSingboxRemoteNodeOpenConnectionError(text) {
		return false
	}
	if isSingboxMissingFakeIPLog(text) {
		return false
	}
	upper := strings.ToUpper(text)
	return strings.Contains(upper, " ERROR ") || strings.Contains(upper, " FATAL") || strings.HasPrefix(upper, "ERROR ") || strings.HasPrefix(upper, "FATAL")
}

func isSingboxMissingFakeIPLog(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "missing fakeip record")
}

func isSingboxLocalClientCloseLog(text string) bool {
	lower := strings.ToLower(text)
	isAbortOrReset := strings.Contains(lower, "forcibly closed by the remote host") ||
		strings.Contains(lower, "aborted by the software in your host machine") ||
		strings.Contains(lower, "connection reset by peer")
	if !isAbortOrReset {
		return false
	}

	loopback := strings.Contains(lower, " 127.0.0.1:") &&
		(strings.Contains(lower, "->127.0.0.1:") || strings.Contains(lower, "→127.0.0.1:") || strings.Contains(lower, "<-127.0.0.1:"))
	tunLocal := (strings.Contains(lower, " 172.19.0.1:") &&
		(strings.Contains(lower, "->172.19.0.2:") || strings.Contains(lower, "→172.19.0.2:"))) ||
		(strings.Contains(lower, " 172.19.0.2:") &&
			(strings.Contains(lower, "->172.19.0.1:") || strings.Contains(lower, "→172.19.0.1:")))

	if strings.Contains(lower, "connection upload closed: raw-read tcp") {
		return loopback || tunLocal
	}

	if strings.Contains(lower, "inbound/") && (strings.Contains(lower, "process connection from 127.0.0.1:") || strings.Contains(lower, "inbound connection from 127.0.0.1:")) {
		return loopback || strings.Contains(lower, "from 127.0.0.1:")
	}

	return false
}

func isSingboxBlockLog(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "using outbound/block[block]") && strings.Contains(lower, "operation not permitted")
}

func extractBlockTarget(text string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "open connection to ")
	if idx != -1 {
		rest := text[idx+len("open connection to "):]
		if usingIdx := strings.Index(strings.ToLower(rest), " using outbound/block"); usingIdx != -1 {
			return strings.TrimSpace(rest[:usingIdx])
		}
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

func (m *SingBoxManager) shouldLogBlockTarget(target string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recentBlockedTargets == nil {
		m.recentBlockedTargets = make(map[string]time.Time)
	}
	now := time.Now()
	if len(m.recentBlockedTargets) > 100 {
		for k, t := range m.recentBlockedTargets {
			if now.Sub(t) > time.Minute {
				delete(m.recentBlockedTargets, k)
			}
		}
	}
	if last, ok := m.recentBlockedTargets[target]; ok && now.Sub(last) < 3*time.Second {
		return false
	}
	m.recentBlockedTargets[target] = now
	return true
}

func isSingboxRemoteNodeOpenConnectionError(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "connection download closed: remote: open connection to ") &&
		strings.Contains(lower, " using outbound/") &&
		(strings.Contains(lower, "lookup ") || strings.Contains(lower, "dial tcp ") || strings.Contains(lower, "i/o timeout"))
}

func (m *SingBoxManager) shouldLogBlockSummary() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.blockLogCount++
	if m.lastBlockLogAt.IsZero() || now.Sub(m.lastBlockLogAt) >= 30*time.Second {
		count := m.blockLogCount
		m.blockLogCount = 0
		m.lastBlockLogAt = now
		return count, true
	}
	return 0, false
}

func (m *SingBoxManager) shouldLogRemoteNodeSummary() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.remoteLogCount++
	if m.lastRemoteLogAt.IsZero() || now.Sub(m.lastRemoteLogAt) >= 30*time.Second {
		count := m.remoteLogCount
		m.remoteLogCount = 0
		m.lastRemoteLogAt = now
		return count, true
	}
	return 0, false
}

func (m *SingBoxManager) shouldLogLocalClientCloseSummary() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.localCloseLogCount++
	if m.lastLocalCloseLogAt.IsZero() || now.Sub(m.lastLocalCloseLogAt) >= 30*time.Second {
		count := m.localCloseLogCount
		m.localCloseLogCount = 0
		m.lastLocalCloseLogAt = now
		return count, true
	}
	return 0, false
}

func (m *SingBoxManager) shouldLogMissingFakeIPSummary() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.missingFakeIPLogCount++
	if m.lastMissingFakeIPLogAt.IsZero() || now.Sub(m.lastMissingFakeIPLogAt) >= 30*time.Second {
		count := m.missingFakeIPLogCount
		m.missingFakeIPLogCount = 0
		m.lastMissingFakeIPLogAt = now
		return count, true
	}
	return 0, false
}

func (m *SingBoxManager) watchExit(cmd *exec.Cmd, done chan<- error) {
	err := cmd.Wait()
	if done != nil {
		done <- err
		close(done)
	}
	m.mu.Lock()
	expected := m.cmd != cmd
	processJob := uintptr(0)
	if m.cmd == cmd {
		m.cmd = nil
		processJob = m.processJob
		m.processJob = 0
		m.done = nil
		m.proxyMapStartPort = 0
		m.proxyMapPortCount = 0
	}
	m.mu.Unlock()
	closeKillOnCloseJob(processJob)
	if err != nil && !expected {
		m.logger.Error("sing-box 已退出：%v", err)
	}
}

func BuildSingBoxConfig(state PersistState, selected Node, cachePath string) (map[string]interface{}, error) {
	return buildSingBoxConfig(state, selected, cachePath, true, defaultAutoDNSPlan())
}

func BuildSingBoxStartupConfig(state PersistState, selected Node, cachePath string) (map[string]interface{}, error) {
	return buildSingBoxConfig(state, selected, cachePath, false, defaultAutoDNSPlan())
}

func buildSingBoxStartupConfigWithDNSPlan(state PersistState, selected Node, cachePath string, dnsPlan autoDNSPlan) (map[string]interface{}, error) {
	return buildSingBoxConfig(state, selected, cachePath, false, dnsPlan)
}

func buildSingBoxConfig(state PersistState, selected Node, cachePath string, includeRemoteRuleSets bool, dnsPlan autoDNSPlan) (map[string]interface{}, error) {
	settings := state.Settings
	normalizeSettings(&settings)
	if selected.ID == "" {
		return nil, errors.New("未选择代理节点")
	}
	if settings.MixedPort == clashAPIPort {
		return nil, fmt.Errorf("本地端口不能使用内核控制端口 %d", clashAPIPort)
	}
	proxyMapNodes := proxyMapNodesForSettings(state.Nodes, settings)
	if err := validateProxyMapSettings(settings, len(proxyMapNodes)); err != nil {
		return nil, err
	}

	inbounds := []map[string]interface{}{
		{
			"type":             "mixed",
			"tag":              "mixed-in",
			"listen":           "127.0.0.1",
			"listen_port":      settings.MixedPort,
			"set_system_proxy": false,
		},
	}
	if effectiveTun(settings) {
		routeExclude := append([]string(nil), tunRouteExcludeIPCIDRs...)
		for _, nodeIP := range extractNodeServerIPs(state.Nodes) {
			routeExclude = append(routeExclude, nodeIP)
		}
		tunInbound := map[string]interface{}{
			"type":                     "tun",
			"tag":                      "tun-in",
			"interface_name":           tunInterfaceName,
			"address":                  []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			"mtu":                      1500,
			"auto_route":               true,
			"strict_route":             true,
			"endpoint_independent_nat": true,
			"stack":                    "mixed",
			"dns_mode":                 "native",
			"route_exclude_address":    routeExclude,
		}
		inbounds = append(inbounds, tunInbound)
	}
	if settings.ProxyMap {
		for index := range proxyMapNodes {
			inbounds = append(inbounds, map[string]interface{}{
				"type":        "mixed",
				"tag":         proxyMapInboundTag(index),
				"listen":      "127.0.0.1",
				"listen_port": proxyMapPort(settings, index),
			})
		}
	}

	outbounds := []map[string]interface{}{
		{"type": "direct", "tag": "direct"},
		{"type": "block", "tag": "block"},
	}
	endpoints := []map[string]interface{}{}
	selectorTags := make([]string, 0, len(state.Nodes))
	selectorTagSet := map[string]bool{}
	for _, node := range state.Nodes {
		tag := nodeOutboundTag(node)
		if tag == "" || selectorTagSet[tag] {
			continue
		}
		selectorTagSet[tag] = true
		selectorTags = append(selectorTags, tag)
		proxy := sanitizeOutboundForSingBox(node.Outbound)
		proxy["tag"] = tag
		if endpoint := wireGuardEndpointFromOutbound(proxy); endpoint != nil {
			endpoints = append(endpoints, endpoint)
		} else {
			outbounds = append(outbounds, proxy)
		}
	}
	selectedTag := nodeOutboundTag(selected)
	if selectedTag == "" {
		return nil, errors.New("所选节点缺少 ID")
	}
	if !selectorTagSet[selectedTag] {
		selectorTags = append(selectorTags, selectedTag)
		proxy := sanitizeOutboundForSingBox(selected.Outbound)
		proxy["tag"] = selectedTag
		if endpoint := wireGuardEndpointFromOutbound(proxy); endpoint != nil {
			endpoints = append(endpoints, endpoint)
		} else {
			outbounds = append(outbounds, proxy)
		}
	}
	if settings.ProxyMap {
		for index, node := range proxyMapNodes {
			proxy := sanitizeOutboundForSingBox(node.Outbound)
			proxy["tag"] = proxyMapOutboundTag(index)
			if endpoint := wireGuardEndpointFromOutbound(proxy); endpoint != nil {
				endpoints = append(endpoints, endpoint)
			} else {
				outbounds = append(outbounds, proxy)
			}
		}
	}
	outbounds = append([]map[string]interface{}{
		{
			"type":                        "selector",
			"tag":                         "proxy",
			"outbounds":                   selectorTags,
			"default":                     selectedTag,
			"interrupt_exist_connections": true,
		},
	}, outbounds...)

	final := "proxy"
	defaultResolver := "local"
	route := map[string]interface{}{
		"auto_detect_interface": true,
		"default_domain_resolver": map[string]interface{}{
			"server":   defaultResolver,
			"strategy": "prefer_ipv4",
		},
		"rules": buildRouteRules(settings, state.Nodes, includeRemoteRuleSets),
		"final": final,
	}
	if includeRemoteRuleSets {
		if ruleSets := buildRuleSets(settings, cachePath); len(ruleSets) > 0 {
			route["rule_set"] = ruleSets
		}
	}

	storeFakeIP := effectiveTun(settings) || settings.FakeDNS
	cacheFileConfig := map[string]interface{}{
		"enabled":      true,
		"store_fakeip": storeFakeIP,
	}
	if strings.TrimSpace(cachePath) != "" {
		cacheFileConfig["path"] = cachePath
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"level":     coreLogLevel(settings),
			"timestamp": true,
		},
		"dns":       buildDNSConfig(settings, final, dnsPlan, includeRemoteRuleSets, state.Nodes),
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
		"experimental": map[string]interface{}{
			"cache_file": cacheFileConfig,
			"clash_api": map[string]interface{}{
				"external_controller": clashAPIAddress,
				"default_mode":        clashMode(settings.Mode),
			},
		},
	}
	if len(endpoints) > 0 {
		config["endpoints"] = endpoints
	}
	return config, nil
}

func coreLogLevel(settings Settings) string {
	level := strings.TrimSpace(strings.ToLower(settings.CoreLogLevel))
	switch level {
	case "debug", "info", "warn", "error":
		return level
	}
	return "info"
}

func configModeSummary(state PersistState) ConfigMode {
	settings := state.Settings
	normalizeSettings(&settings)
	summary := ConfigMode{Mode: settings.Mode, Final: "proxy"}
	if settings.Mode == "rule" {
		summary.HasDirect = true
		summary.DirectHits = []string{"内置直连规则"}
		if _, ok := rulePresetDefs[settings.RuleSource]; ok {
			summary.RuleSets = []string{"loyalsoldier-proxy", "loyalsoldier-direct", "loyalsoldier-china", "geoip-cn"}
		}
	}
	return summary
}

func (m *SingBoxManager) RuntimeConfigMode(fallbackMode string) (ConfigMode, error) {
	config, err := m.readRuntimeConfig()
	if err != nil {
		return ConfigMode{}, err
	}
	return configModeFromConfig(config, fallbackMode), nil
}

func (m *SingBoxManager) ValidateRuntimeSettings(settings Settings) error {
	normalizeSettings(&settings)
	config, err := m.readRuntimeConfig()
	if err != nil {
		return err
	}
	summary := configModeFromConfigDetails(config, settings.Mode, settings.ExternalProxyEndpoints, appProxyActive(settings))
	if settings.Mode == "global" && (summary.HasDirect || len(summary.RuleSets) > 0) {
		return fmt.Errorf("全局模式未真实生效：运行配置仍包含直连规则或规则集")
	}
	if settings.Mode == "global" && hasGlobalModeBlockRule(config) {
		return fmt.Errorf("全局模式未真实生效：运行配置仍包含普通拦截规则")
	}
	if settings.Mode == "rule" && !summary.HasDirect {
		return fmt.Errorf("规则模式未真实生效：运行配置缺少直连分流规则")
	}
	wantTun := effectiveTun(settings)
	if hasTunInbound(config) != wantTun {
		return fmt.Errorf("TUN 状态未真实生效：期望 %v，运行配置为 %v", wantTun, hasTunInbound(config))
	}
	if appProxyActive(settings) && !hasAppProxyRouteRule(config, settings) {
		return fmt.Errorf("应用接管未真实生效：运行配置缺少进程路由规则")
	}
	if !hasExternalProxyRoutes(config, settings.ExternalProxyEndpoints) {
		return fmt.Errorf("外部代理入口直连未真实生效：运行配置缺少端点规则")
	}
	return nil
}

func (m *SingBoxManager) readRuntimeConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile(m.config)
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func configModeFromConfig(config map[string]interface{}, fallbackMode string) ConfigMode {
	return configModeFromConfigWithExternalEndpoints(config, fallbackMode, nil)
}

func configModeFromConfigWithExternalEndpoints(config map[string]interface{}, fallbackMode string, externalProxyEndpoints []ExternalProxyEndpoint) ConfigMode {
	return configModeFromConfigDetails(config, fallbackMode, externalProxyEndpoints, false)
}

func configModeFromConfigDetails(config map[string]interface{}, fallbackMode string, externalProxyEndpoints []ExternalProxyEndpoint, allowAppProxyFallback bool) ConfigMode {
	if fallbackMode != "rule" && fallbackMode != "global" {
		fallbackMode = modeFromRuntimeConfig(config)
	}
	summary := ConfigMode{Mode: fallbackMode}
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return summary
	}
	if final, ok := route["final"].(string); ok {
		summary.Final = final
	}
	for _, rule := range configRules(route["rules"]) {
		if !ruleAppliesToMode(rule, fallbackMode) {
			continue
		}
		summary.RuleCount++
		outbound, _ := rule["outbound"].(string)
		if outbound == "direct" &&
			!isTunStabilityDirectRule(rule) &&
			!isSavedExternalProxyDirectRule(rule, externalProxyEndpoints) &&
			!(allowAppProxyFallback && isAppProxyFallbackDirectRule(rule)) {
			summary.HasDirect = true
			summary.DirectHits = append(summary.DirectHits, routeRuleLabel(rule))
		}
	}
	for _, item := range configRules(route["rule_set"]) {
		if fallbackMode == "global" {
			continue
		}
		if tag, ok := item["tag"].(string); ok {
			summary.RuleSets = append(summary.RuleSets, tag)
		}
	}
	if summary.Mode != "rule" && summary.Mode != "global" {
		if summary.HasDirect || len(summary.RuleSets) > 0 {
			summary.Mode = "rule"
		} else {
			summary.Mode = "global"
		}
	}
	return summary
}

func modeFromRuntimeConfig(config map[string]interface{}) string {
	experimental, ok := config["experimental"].(map[string]interface{})
	if !ok {
		return ""
	}
	clashAPI, ok := experimental["clash_api"].(map[string]interface{})
	if !ok {
		return ""
	}
	return appModeFromClashMode(stringFromConfig(clashAPI["default_mode"]))
}

func appModeFromClashMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "global") {
		return "global"
	}
	if strings.EqualFold(strings.TrimSpace(mode), "rule") {
		return "rule"
	}
	return ""
}

func stringFromConfig(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func ruleAppliesToMode(rule map[string]interface{}, mode string) bool {
	if mode != "rule" && mode != "global" {
		return true
	}
	value, ok := rule["clash_mode"].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return true
	}
	return strings.EqualFold(value, mode)
}

func hasTunInbound(config map[string]interface{}) bool {
	for _, inbound := range configRules(config["inbounds"]) {
		if inbound["type"] == "tun" {
			return true
		}
	}
	return false
}

func hasAppProxyRouteRule(config map[string]interface{}, settings Settings) bool {
	expected := appProxyRouteRule(settings)
	if expected == nil {
		return false
	}
	expectedPaths := configStringValues(expected["process_path"])
	expectedPatterns := configStringValues(expected["process_path_regex"])
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return false
	}
	for _, rule := range configRules(route["rules"]) {
		if rule["action"] == "route" && rule["outbound"] == "proxy" &&
			stringSlicesEqual(configStringValues(rule["process_path"]), expectedPaths) &&
			stringSlicesEqual(configStringValues(rule["process_path_regex"]), expectedPatterns) {
			return true
		}
	}
	return false
}

func hasGlobalModeBlockRule(config map[string]interface{}) bool {
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return false
	}
	for _, rule := range configRules(route["rules"]) {
		if rule["outbound"] != "block" || !ruleAppliesToMode(rule, "global") {
			continue
		}
		if isTunStabilityBlockRule(rule) || isLocalMetadataBlockRule(rule) {
			continue
		}
		return true
	}
	return false
}

func isTunStabilityBlockRule(rule map[string]interface{}) bool {
	inbounds := configStringValues(rule["inbound"])
	if !stringSliceContains(inbounds, "tun-in") {
		return false
	}
	return rule["port"] != nil || rule["ip_cidr"] != nil || rule["source_ip_cidr"] != nil
}

func isLocalMetadataBlockRule(rule map[string]interface{}) bool {
	if rule["outbound"] != "block" {
		return false
	}
	return stringSlicesEqual(configStringValues(rule["ip_cidr"]), localMetadataIPCIDRs)
}

func isTunStabilityDirectRule(rule map[string]interface{}) bool {
	if rule["outbound"] != "direct" {
		return false
	}
	inbounds := configStringValues(rule["inbound"])
	if !stringSliceContains(inbounds, "tun-in") {
		return false
	}
	return rule["process_path_regex"] != nil || rule["process_name"] != nil
}

func isAppProxyFallbackDirectRule(rule map[string]interface{}) bool {
	return len(rule) == 3 &&
		rule["action"] == "route" &&
		rule["outbound"] == "direct" &&
		stringSlicesEqual(configStringValues(rule["inbound"]), []string{"tun-in"})
}

func configStringValues(value interface{}) []string {
	switch items := value.(type) {
	case string:
		if items == "" {
			return nil
		}
		return []string{items}
	case []string:
		return items
	case []interface{}:
		result := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func configRules(value interface{}) []map[string]interface{} {
	switch items := value.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if rule, ok := item.(map[string]interface{}); ok {
				result = append(result, rule)
			}
		}
		return result
	default:
		return nil
	}
}

func routeRuleLabel(rule map[string]interface{}) string {
	for _, key := range []string{"rule_set", "domain", "domain_suffix", "domain_keyword", "ip_is_private"} {
		if value, ok := rule[key]; ok {
			return fmt.Sprintf("%s=%v", key, value)
		}
	}
	return "direct"
}

func (m *SingBoxManager) fetchTrafficTotal() (TrafficSnapshot, error) {
	client := http.Client{Timeout: 1200 * time.Millisecond}
	req, err := http.NewRequest(http.MethodGet, "http://"+clashAPIAddress+"/connections", nil)
	if err != nil {
		return TrafficSnapshot{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return TrafficSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TrafficSnapshot{}, fmt.Errorf("读取流量失败：HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(resp.Body)
	var total TrafficSnapshot
	if err := readTrafficTotals(decoder, &total); err != nil {
		return TrafficSnapshot{}, err
	}
	return total, nil
}

func readTrafficTotals(decoder *json.Decoder, total *TrafficSnapshot) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return errors.New("读取流量失败：响应格式无效")
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, _ := keyToken.(string)
		switch key {
		case "uploadTotal":
			if err := decoder.Decode(&total.UploadTotal); err != nil {
				return err
			}
		case "downloadTotal":
			if err := decoder.Decode(&total.DownloadTotal); err != nil {
				return err
			}
		case "connections":
			if err := readTrafficConnections(decoder, total); err != nil {
				return err
			}
		default:
			var skip interface{}
			if err := decoder.Decode(&skip); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

func readTrafficConnections(decoder *json.Decoder, total *TrafficSnapshot) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return errors.New("读取流量失败：连接列表格式无效")
	}
	sumConnections := total.UploadTotal == 0 && total.DownloadTotal == 0
	for decoder.More() {
		var conn struct {
			Upload   int64 `json:"upload"`
			Download int64 `json:"download"`
		}
		if err := decoder.Decode(&conn); err != nil {
			return err
		}
		if sumConnections {
			total.UploadTotal += conn.Upload
			total.DownloadTotal += conn.Download
		}
	}
	_, err = decoder.Token()
	return err
}

func (m *SingBoxManager) ensureRuleSets(settings Settings) error {
	normalizeSettings(&settings)
	defs := rulePresetDefs[settings.RuleSource]
	if len(defs) == 0 {
		return nil
	}
	ruleDir := ruleSetDirFromCachePath(m.cachePath)
	missing := make([]string, 0)
	for _, def := range defs {
		target := filepath.Join(ruleDir, def.FileName)
		info, err := os.Stat(target)
		if err == nil && info.Size() > 0 {
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, def.FileName)
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("本地规则缓存缺失：%s，请先点击“更新规则”", strings.Join(missing, "、"))
}

func (m *SingBoxManager) UpdateRuleSets(settings Settings) (int, error) {
	targetVersion, err := m.ruleUpdateTargetVersion(settings)
	if err != nil {
		return 0, err
	}
	if m.localRuleSetsCurrent(settings, targetVersion) {
		return 0, m.writeRuleSetVersion(settings, targetVersion)
	}
	return m.updateRuleSets(settings, targetVersion)
}

func (m *SingBoxManager) UpdateRuleSetsIfNeeded(settings Settings) (RuleUpdateResult, error) {
	normalizeSettings(&settings)
	targetVersion, err := m.ruleUpdateTargetVersion(settings)
	if err != nil {
		return RuleUpdateResult{Version: ruleSourceVersion(settings.RuleSource)}, err
	}
	defs := rulePresetDefsForVersion(settings.RuleSource, targetVersion)
	if len(defs) == 0 {
		return RuleUpdateResult{Latest: true}, nil
	}
	if m.localRuleSetsCurrent(settings, targetVersion) {
		if err := m.writeRuleSetVersion(settings, targetVersion); err != nil {
			return RuleUpdateResult{Version: targetVersion}, err
		}
		if m.logger != nil {
			m.logger.Info("规则已是最新版本：%s", targetVersion)
		}
		return RuleUpdateResult{Latest: true, Version: targetVersion}, nil
	}
	count, err := m.updateRuleSets(settings, targetVersion)
	if err != nil {
		return RuleUpdateResult{Version: targetVersion}, err
	}
	if err := m.writeRuleSetVersion(settings, targetVersion); err != nil {
		return RuleUpdateResult{Count: count, Version: targetVersion}, err
	}
	return RuleUpdateResult{Count: count, Latest: count == 0, Version: targetVersion}, nil
}

func (m *SingBoxManager) RuleSetBackups(settings Settings) ([]ruleSetBackup, error) {
	normalizeSettings(&settings)
	defs := rulePresetDefs[settings.RuleSource]
	backups := make([]ruleSetBackup, 0, len(defs))
	for _, def := range defs {
		target := ruleSetLocalPath(m.cachePath, def.FileName)
		item := ruleSetBackup{path: target}
		data, err := os.ReadFile(target)
		if err == nil {
			item.exists = true
			item.data = append([]byte(nil), data...)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		backups = append(backups, item)
	}
	return backups, nil
}

func (m *SingBoxManager) RestoreRuleSetBackups(backups []ruleSetBackup) error {
	for _, item := range backups {
		if strings.TrimSpace(item.path) == "" {
			continue
		}
		if item.exists {
			if err := os.MkdirAll(filepath.Dir(item.path), 0755); err != nil {
				return err
			}
			if err := atomicWriteFile(item.path, item.data, 0644); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(item.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (m *SingBoxManager) updateRuleSets(settings Settings, version string) (int, error) {
	normalizeSettings(&settings)
	m.mu.Lock()
	proxyPort := m.socks5TestProxyPort
	m.mu.Unlock()
	defs := rulePresetDefsForVersion(settings.RuleSource, version)
	if len(defs) == 0 {
		return 0, nil
	}
	ruleDir := ruleSetDirFromCachePath(m.cachePath)
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		return 0, err
	}
	staged := make([]stagedRuleSetFile, 0, len(defs))
	defer func() {
		for _, item := range staged {
			if item.tempPath != "" {
				_ = os.Remove(item.tempPath)
			}
		}
	}()
	for _, def := range defs {
		target := filepath.Join(ruleDir, def.FileName)
		if _, err := os.Stat(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		if m.logger != nil {
			m.logger.Info("正在更新规则源：%s", def.URL)
		}
		body, err := httpGetBytesFromOptions(coreNetworkOptions("规则源", def.URL, proxyPort), ruleFileDownloadTimeout, maxRuleDownloadBytes)
		if err != nil {
			if _, statErr := os.Stat(target); statErr == nil {
				if m.logger != nil {
					m.logger.Error("规则源手动更新失败，当前仍保留本地缓存：%v", err)
				}
			}
			return 0, fmt.Errorf("下载 Loyalsoldier 规则失败：%w", err)
		}
		var data []byte
		if def.Tag == "geoip-cn" {
			data, err = convertLoyalsoldierIPList(body)
		} else {
			data, err = convertLoyalsoldierDomainList(body)
		}
		if err != nil {
			return 0, fmt.Errorf("转换 Loyalsoldier 规则失败：%w", err)
		}
		tempPath, err := writeRuleSetStage(ruleDir, def.FileName, data)
		if err != nil {
			return 0, err
		}
		staged = append(staged, stagedRuleSetFile{targetPath: target, tempPath: tempPath})
	}
	if err := commitStagedRuleSets(staged); err != nil {
		return 0, err
	}
	return len(staged), nil
}

type stagedRuleSetFile struct {
	targetPath    string
	tempPath      string
	backupPath    string
	originalMoved bool
	installed     bool
}

func writeRuleSetStage(dir, name string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, filepath.Base(name)+"-update-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0644); err != nil {
		return "", err
	}
	cleanup = false
	return path, nil
}

func commitStagedRuleSets(files []stagedRuleSetFile) error {
	for index := range files {
		item := &files[index]
		item.backupPath = item.targetPath + ".update.bak"
		if _, err := os.Stat(item.backupPath); err == nil {
			if _, targetErr := os.Stat(item.targetPath); errors.Is(targetErr, os.ErrNotExist) {
				if err := os.Rename(item.backupPath, item.targetPath); err != nil {
					return rollbackRuleSetCommit(files, err)
				}
			} else if targetErr != nil {
				return rollbackRuleSetCommit(files, targetErr)
			} else if err := os.Remove(item.backupPath); err != nil {
				return rollbackRuleSetCommit(files, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollbackRuleSetCommit(files, err)
		}
		if _, err := os.Stat(item.targetPath); err == nil {
			if err := os.Rename(item.targetPath, item.backupPath); err != nil {
				return rollbackRuleSetCommit(files, err)
			}
			item.originalMoved = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollbackRuleSetCommit(files, err)
		}
		if err := os.Rename(item.tempPath, item.targetPath); err != nil {
			return rollbackRuleSetCommit(files, err)
		}
		item.tempPath = ""
		item.installed = true
	}
	for index := range files {
		if files[index].originalMoved {
			_ = os.Remove(files[index].backupPath)
		}
	}
	return nil
}

func rollbackRuleSetCommit(files []stagedRuleSetFile, commitErr error) error {
	var rollbackErr error
	for index := len(files) - 1; index >= 0; index-- {
		item := files[index]
		if item.installed {
			if err := os.Remove(item.targetPath); err != nil && !errors.Is(err, os.ErrNotExist) && rollbackErr == nil {
				rollbackErr = err
			}
		}
		if item.originalMoved {
			if err := os.Rename(item.backupPath, item.targetPath); err != nil && rollbackErr == nil {
				rollbackErr = err
			}
		}
	}
	if rollbackErr != nil {
		return fmt.Errorf("提交规则文件失败：%v；恢复旧规则失败：%w", commitErr, rollbackErr)
	}
	return fmt.Errorf("提交规则文件失败，旧规则已恢复：%w", commitErr)
}

func (m *SingBoxManager) localRuleSetsCurrent(settings Settings, version string) bool {
	normalizeSettings(&settings)
	defs := rulePresetDefs[settings.RuleSource]
	if len(defs) == 0 {
		return true
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	cachedVersion := strings.TrimSpace(m.readRuleSetVersion(settings))
	ruleDir := ruleSetDirFromCachePath(m.cachePath)
	for _, def := range defs {
		info, err := os.Stat(filepath.Join(ruleDir, def.FileName))
		if err != nil || info.Size() <= 0 {
			return false
		}
	}
	return cachedVersion == version || cachedVersion == "" && version == ruleSourceVersion(settings.RuleSource)
}

func (m *SingBoxManager) readRuleSetVersion(settings Settings) string {
	data, err := os.ReadFile(ruleSetVersionPath(m.cachePath, settings.RuleSource))
	if err != nil {
		return ""
	}
	var item struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return ""
	}
	return item.Version
}

func (m *SingBoxManager) writeRuleSetVersion(settings Settings, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}
	data, err := json.MarshalIndent(map[string]string{"version": version}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(ruleSetVersionPath(m.cachePath, settings.RuleSource), data, 0644)
}

func ruleSourceVersion(source string) string {
	if source == ruleSourceLoyalsoldier {
		return loyalsoldierRuleVersion
	}
	return ""
}

func (m *SingBoxManager) ruleUpdateTargetVersion(settings Settings) (string, error) {
	current := ruleSourceVersion(settings.RuleSource)
	if settings.RuleSource != ruleSourceLoyalsoldier {
		return current, nil
	}

	latest, err := m.checkLatestRuleVersion(settings.RuleSource)
	if err != nil {
		return "", err
	}
	latest = normalizeRuleVersionTag(latest)
	if latest == "" {
		return current, nil
	}
	return latest, nil
}

func (m *SingBoxManager) checkLatestRuleVersion(source string) (string, error) {
	if m.ruleVersionChecker != nil {
		return m.ruleVersionChecker(source)
	}
	m.mu.Lock()
	proxyPort := m.socks5TestProxyPort
	m.mu.Unlock()
	return latestRuleVersion(source, proxyPort)
}

func rulePresetDefsForVersion(source string, version string) []ruleSetDef {
	if source != ruleSourceLoyalsoldier {
		return rulePresetDefs[source]
	}
	version = normalizeRuleVersionTag(version)
	if version == "" {
		version = loyalsoldierRuleVersion
	}
	baseURL := "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/download/" + version
	return []ruleSetDef{
		{Tag: "loyalsoldier-direct", URL: baseURL + "/direct-list.txt", FileName: "loyalsoldier-direct.json"},
		{Tag: "loyalsoldier-china", URL: baseURL + "/china-list.txt", FileName: "loyalsoldier-china.json"},
		{Tag: "geoip-cn", URL: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt", FileName: "geoip-cn.json"},
		{Tag: "loyalsoldier-proxy", URL: baseURL + "/proxy-list.txt", FileName: "loyalsoldier-proxy.json"},
	}
}

func latestRuleVersion(source string, proxyPort int) (string, error) {
	if source != ruleSourceLoyalsoldier {
		return "", nil
	}
	const releaseURL = "https://api.github.com/repos/Loyalsoldier/v2ray-rules-dat/releases/latest"
	body, err := httpGetBytesFromOptions(coreNetworkOptions("规则版本地址", releaseURL, proxyPort), ruleVersionCheckTimeout, maxReleaseMetadataBytes)
	if err != nil {
		return "", fmt.Errorf("检测规则源版本失败：%w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	version := normalizeRuleVersionTag(release.TagName)
	if version == "" {
		return "", fmt.Errorf("规则源版本号无效：%s", release.TagName)
	}
	return version, nil
}

func normalizeRuleVersionTag(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "tag/")
	version = strings.TrimSpace(version)
	if index := strings.IndexAny(version, " \r\n\t"); index >= 0 {
		version = version[:index]
	}
	return version
}

func ruleSetVersionPath(cachePath string, source string) string {
	return filepath.Join(ruleSetDirFromCachePath(cachePath), strings.TrimSpace(source)+"-version.json")
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		_ = os.Chmod(dir, 0777)
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode()&0200 == 0 {
		_ = os.Chmod(path, 0666)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-tmp-*")
	if err == nil {
		tmpPath := tmp.Name()
		cleanup := true
		defer func() {
			if cleanup {
				_ = os.Remove(tmpPath)
			}
		}()
		if _, err := tmp.Write(data); err == nil {
			_ = tmp.Sync()
			if err := tmp.Close(); err == nil {
				_ = os.Chmod(tmpPath, perm)
				if err := replaceFileWithRetry(tmpPath, path, 3); err == nil {
					cleanup = false
					return nil
				}
			}
		} else {
			_ = tmp.Close()
		}
	}
	_ = os.Chmod(path, 0666)
	return os.WriteFile(path, data, perm)
}

func convertLoyalsoldierDomainList(data []byte) ([]byte, error) {
	var domains, domainSuffixes, domainRegexes []string
	seenDomain := map[string]bool{}
	seenSuffix := map[string]bool{}
	seenRegex := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	add := func(items *[]string, seen map[string]bool, value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		*items = append(*items, value)
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "full:"):
			add(&domains, seenDomain, strings.TrimPrefix(line, "full:"))
		case strings.HasPrefix(line, "regexp:"):
			add(&domainRegexes, seenRegex, strings.TrimPrefix(line, "regexp:"))
		default:
			add(&domainSuffixes, seenSuffix, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	rule := map[string]interface{}{}
	if len(domains) > 0 {
		rule["domain"] = domains
	}
	if len(domainSuffixes) > 0 {
		rule["domain_suffix"] = domainSuffixes
	}
	if len(domainRegexes) > 0 {
		rule["domain_regex"] = domainRegexes
	}
	source := map[string]interface{}{
		"version": 3,
		"rules":   []map[string]interface{}{rule},
	}
	return json.MarshalIndent(source, "", "  ")
}

func convertLoyalsoldierIPList(data []byte) ([]byte, error) {
	var ipCIDRs []string
	seenCIDR := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if _, _, err := net.ParseCIDR(line); err == nil {
			if !seenCIDR[line] {
				seenCIDR[line] = true
				ipCIDRs = append(ipCIDRs, line)
			}
		} else if ip := net.ParseIP(line); ip != nil {
			cidr := line + "/32"
			if ip.To4() == nil {
				cidr = line + "/128"
			}
			if !seenCIDR[cidr] {
				seenCIDR[cidr] = true
				ipCIDRs = append(ipCIDRs, cidr)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	rule := map[string]interface{}{}
	if len(ipCIDRs) > 0 {
		rule["ip_cidr"] = ipCIDRs
	}
	source := map[string]interface{}{
		"version": 3,
		"rules":   []map[string]interface{}{rule},
	}
	return json.MarshalIndent(source, "", "  ")
}

func ruleSetDirFromCachePath(cachePath string) string {
	if strings.TrimSpace(cachePath) == "" {
		return filepath.Join("data", "gui", "rules")
	}
	return filepath.Join(filepath.Dir(cachePath), "rules")
}

func ruleSetLocalPath(cachePath string, fileName string) string {
	return filepath.Join(ruleSetDirFromCachePath(cachePath), fileName)
}

func buildRuleSets(settings Settings, cachePath string) []map[string]interface{} {
	defs := rulePresetDefs[settings.RuleSource]
	if len(defs) == 0 {
		return nil
	}
	ruleSets := make([]map[string]interface{}, 0, len(defs))
	for _, def := range defs {
		ruleSets = append(ruleSets, map[string]interface{}{
			"type":   "local",
			"tag":    def.Tag,
			"format": "source",
			"path":   ruleSetLocalPath(cachePath, def.FileName),
		})
	}
	return ruleSets
}

func buildRouteRules(settings Settings, nodes []Node, includeRemoteRuleSets bool) []map[string]interface{} {
	rules := []map[string]interface{}{
		{"action": "sniff"},
		{"port": []int{53}, "action": "hijack-dns"},
		{"protocol": "dns", "action": "hijack-dns"},
	}
	rules = append(rules, buildLocalMetadataBlockRouteRule())
	rules = append(rules, buildExternalProxyDirectRules(settings)...)
	if effectiveTun(settings) {
		rules = append(rules, buildTunStabilityRouteRules()...)
	}
	if settings.AppProxy {
		if appRule := appProxyRouteRule(settings); len(appRule) > 0 {
			rules = append(rules, appRule)
		}
		if appProxyOnlyTun(settings) {
			rules = append(rules, map[string]interface{}{
				"inbound":  []string{"tun-in"},
				"action":   "route",
				"outbound": "direct",
			})
		}
	}
	if settings.ProxyMap {
		proxyMapNodes := proxyMapNodesForSettings(nodes, settings)
		for index := range proxyMapNodes {
			rules = append(rules, map[string]interface{}{
				"inbound":  []string{proxyMapInboundTag(index)},
				"action":   "route",
				"outbound": proxyMapOutboundTag(index),
			})
		}
	}
	rules = append(rules,
		map[string]interface{}{"clash_mode": "global", "action": "route", "outbound": "proxy"},
		map[string]interface{}{"clash_mode": "direct", "action": "route", "outbound": "direct"},
	)
	domains, ipCIDRs := parseDirectEntries(settings.DirectDomains)
	if len(domains) > 0 {
		rules = append(rules, map[string]interface{}{
			"domain":        domains,
			"domain_suffix": domains,
			"clash_mode":    "rule",
			"action":        "route",
			"outbound":      "direct",
		})
	}
	if len(ipCIDRs) > 0 {
		rules = append(rules, map[string]interface{}{
			"ip_cidr":    ipCIDRs,
			"clash_mode": "rule",
			"action":     "route",
			"outbound":   "direct",
		})
	}
	if includeRemoteRuleSets {
		if _, ok := rulePresetDefs[settings.RuleSource]; ok {
			rules = append(rules, map[string]interface{}{"rule_set": "loyalsoldier-proxy", "clash_mode": "rule", "action": "route", "outbound": "proxy"})
			rules = append(rules, map[string]interface{}{"rule_set": "loyalsoldier-direct", "clash_mode": "rule", "action": "route", "outbound": "direct"})
			rules = append(rules, map[string]interface{}{"rule_set": "loyalsoldier-china", "clash_mode": "rule", "action": "route", "outbound": "direct"})
			rules = append(rules, map[string]interface{}{"rule_set": "geoip-cn", "clash_mode": "rule", "action": "route", "outbound": "direct"})
		}
	}
	rules = append(rules,
		map[string]interface{}{"ip_is_private": true, "clash_mode": "rule", "action": "route", "outbound": "direct"},
		map[string]interface{}{"domain_suffix": []string{"cn", "localhost"}, "clash_mode": "rule", "action": "route", "outbound": "direct"},
		map[string]interface{}{"domain_keyword": []string{
			"baidu", "bilibili", "qq", "taobao", "tmall", "jd", "alicdn", "aliyun",
			"weixin", "wechat", "douyin", "bytedance", "zhihu", "netease",
		}, "clash_mode": "rule", "action": "route", "outbound": "direct"},
	)
	return rules
}

func buildLocalMetadataBlockRouteRule() map[string]interface{} {
	return map[string]interface{}{
		"ip_cidr":  append([]string(nil), localMetadataIPCIDRs...),
		"action":   "route",
		"outbound": "block",
	}
}

func buildTunStabilityRouteRules() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"inbound":            []string{"tun-in"},
			"process_path_regex": tunBypassProcessPathRegex(),
			"action":             "route",
			"outbound":           "direct",
		},
		{
			"inbound":      []string{"tun-in"},
			"process_name": tunBypassProcessNames(),
			"action":       "route",
			"outbound":     "direct",
		},
		{
			"inbound":  []string{"tun-in"},
			"network":  "udp",
			"port":     []int{135, 137, 138, 139, 443, 1900, 5353},
			"action":   "route",
			"outbound": "block",
		},
		{
			"inbound":  []string{"tun-in"},
			"ip_cidr":  append([]string(nil), tunBlockIPCIDRs...),
			"action":   "route",
			"outbound": "block",
		},
		{
			"inbound":        []string{"tun-in"},
			"source_ip_cidr": append([]string(nil), tunBlockIPCIDRs...),
			"action":         "route",
			"outbound":       "block",
		},
		{
			"inbound":  []string{"tun-in"},
			"ip_cidr":  []string{"198.18.0.0/15"},
			"action":   "route",
			"outbound": "block",
		},
	}
}

func tunBypassProcessPathRegex() []string {
	regexes := []string{
		`(?i)\\bin\\sing-box\.exe$`,
		`(?i)\\system32\\ping\.exe$`,
		`(?i)\\SanTian-Singbox(\.exe)?$`,
		`(?i)\\SanTian-Singbox-dev(\.exe)?$`,
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		quoted := regexp.QuoteMeta(exe)
		pattern := "(?i)^" + quoted + "$"
		if !stringSliceContains(regexes, pattern) {
			regexes = append(regexes, pattern)
		}
	}
	return regexes
}

func tunBypassProcessNames() []string {
	names := []string{
		"sing-box.exe",
		"sing-box",
		"ping.exe",
		"ping",
		"SanTian-Singbox.exe",
		"SanTian-Singbox",
		"SanTian-Singbox-dev.exe",
		"SanTian-Singbox-dev",
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		base := filepath.Base(exe)
		if base != "" && !stringSliceContains(names, base) {
			names = append(names, base)
		}
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		if noExt != "" && !stringSliceContains(names, noExt) {
			names = append(names, noExt)
		}
	}
	return names
}

func appProxyRouteRule(settings Settings) map[string]interface{} {
	paths := make([]string, 0, len(settings.AppProxyTargets))
	folderRegex := make([]string, 0)
	seen := map[string]bool{}
	add := func(path string) {
		path = normalizeAppProxyPath(path)
		if path == "" {
			return
		}
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		paths = append(paths, path)
	}
	for _, target := range settings.AppProxyTargets {
		if target.Enabled && target.Type == "app" {
			add(target.Path)
		}
		if target.Enabled && target.Type == "folder" {
			if pattern := appProxyFolderRegex(target.Path); pattern != "" {
				folderRegex = append(folderRegex, pattern)
			}
		}
	}
	if len(paths) == 0 && len(folderRegex) == 0 {
		return nil
	}
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "proxy",
	}
	if len(paths) > 0 {
		rule["process_path"] = paths
	}
	if len(folderRegex) > 0 {
		rule["process_path_regex"] = folderRegex
	}
	return rule
}

func appProxyFolderRegex(folder string) string {
	folder = normalizeAppProxyPath(folder)
	if folder == "" {
		return ""
	}
	prefix := regexp.QuoteMeta(strings.TrimRight(folder, `\/`) + string(os.PathSeparator))
	return "(?i)^" + prefix + ".*\\.exe$"
}

func nodeOutboundTag(node Node) string {
	return strings.TrimSpace(node.ID)
}

func clashMode(mode string) string {
	if mode == "global" {
		return "global"
	}
	return "rule"
}

func proxyMapPort(settings Settings, index int) int {
	return settings.ProxyMapStartPort + index
}

func proxyMapInboundTag(index int) string {
	return fmt.Sprintf("proxy-map-in-%d", index+1)
}

func proxyMapOutboundTag(index int) string {
	return fmt.Sprintf("proxy-map-out-%d", index+1)
}

func proxyMapNodesForSettings(nodes []Node, settings Settings) []Node {
	return nodes
}

func normalizeProxyMapGroupIDs(ids []string, legacyID string) []string {
	result := make([]string, 0, len(ids)+1)
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	legacyID = strings.TrimSpace(legacyID)
	if legacyID != "" && !seen[legacyID] {
		result = append(result, legacyID)
	}
	return result
}

func proxyMapGroupIDSet(settings Settings) map[string]bool {
	ids := normalizeProxyMapGroupIDs(settings.ProxyMapGroupIDs, settings.ProxyMapGroupID)
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result
}

func validateProxyMapSettings(settings Settings, nodeCount int) error {
	if !settings.ProxyMap {
		return nil
	}
	if settings.ProxyMapStartPort < 1024 || settings.ProxyMapStartPort > 65535 {
		return fmt.Errorf("代理映射起始端口 %d 无效，范围必须是 1024-65535", settings.ProxyMapStartPort)
	}
	if nodeCount == 0 {
		return nil
	}
	lastPort := settings.ProxyMapStartPort + nodeCount - 1
	if lastPort > 65535 {
		return fmt.Errorf("代理映射端口范围 %d-%d 超出 65535，请调低起始端口或减少节点数量", settings.ProxyMapStartPort, lastPort)
	}
	if settings.MixedPort >= settings.ProxyMapStartPort && settings.MixedPort <= lastPort {
		return fmt.Errorf("代理映射端口范围 %d-%d 与本地端口 %d 冲突", settings.ProxyMapStartPort, lastPort, settings.MixedPort)
	}
	if clashAPIPort >= settings.ProxyMapStartPort && clashAPIPort <= lastPort {
		return fmt.Errorf("代理映射端口范围 %d-%d 与内核控制端口 %d 冲突", settings.ProxyMapStartPort, lastPort, clashAPIPort)
	}
	return nil
}

func validateProxyMapPorts(settings Settings, nodeCount int) error {
	if !settings.ProxyMap || nodeCount == 0 {
		return nil
	}
	for index := 0; index < nodeCount; index++ {
		port := proxyMapPort(settings, index)
		if !isTCPPortFree("127.0.0.1", port) {
			return fmt.Errorf("代理映射端口 %d 已被占用，请修改起始端口或关闭占用程序", port)
		}
	}
	return nil
}

func defaultAutoDNSPlan() autoDNSPlan {
	return autoDNSPlan{
		China:  chinaDNSCandidates[0],
		Remote: remoteDNSCandidates[0],
	}
}

func normalizeAutoDNSPlan(plan autoDNSPlan) autoDNSPlan {
	if plan.China.Type == "" || plan.China.Server == "" {
		plan.China = chinaDNSCandidates[0]
	}
	if plan.Remote.Type == "" || plan.Remote.Server == "" {
		plan.Remote = remoteDNSCandidates[0]
	}
	return plan
}

func resolveAutoDNSPlan(logger *Logger) autoDNSPlan {
	plan := defaultAutoDNSPlan()
	china, chinaMS, ok := probeBestChinaDNS(chinaDNSCandidates)
	if ok {
		plan.China = china
	}
	remote, remoteMS, ok := probeBestRemoteDNS(remoteDNSCandidates)
	if ok {
		plan.Remote = remote
	}
	if logger != nil {
		logger.Info("自动 DNS：国内=%s(%dms)，海外=%s(%dms)", plan.China.Server, chinaMS, plan.Remote.Server, remoteMS)
	}
	return plan
}

func probeBestChinaDNS(candidates []dnsProbeCandidate) (dnsProbeCandidate, int64, bool) {
	return probeBestDNS(candidates, func(candidate dnsProbeCandidate) bool {
		return probeUDPDNS(candidate.Server, "www.baidu.com") == nil
	})
}

func probeBestRemoteDNS(candidates []dnsProbeCandidate) (dnsProbeCandidate, int64, bool) {
	if len(candidates) == 0 {
		return dnsProbeCandidate{}, 0, false
	}
	return candidates[0], 0, true
}

func probeBestDNS(candidates []dnsProbeCandidate, probe func(dnsProbeCandidate) bool) (dnsProbeCandidate, int64, bool) {
	type result struct {
		candidate dnsProbeCandidate
		elapsedMS int64
		ok        bool
	}
	results := make(chan result, len(candidates))
	for _, candidate := range candidates {
		candidate := candidate
		go func() {
			start := time.Now()
			ok := probe(candidate)
			elapsed := time.Since(start).Milliseconds()
			if elapsed < 1 {
				elapsed = 1
			}
			results <- result{candidate: candidate, elapsedMS: elapsed, ok: ok}
		}()
	}
	var best dnsProbeCandidate
	var bestMS int64
	for range candidates {
		result := <-results
		if !result.ok {
			continue
		}
		if best.Server == "" || result.elapsedMS < bestMS {
			best = result.candidate
			bestMS = result.elapsedMS
		}
	}
	return best, bestMS, best.Server != ""
}

func probeUDPDNS(server string, domain string) error {
	resolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 700 * time.Millisecond}
			return dialer.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	_, err := resolver.LookupHost(ctx, domain)
	return err
}

func probeHTTPSDNS(server string, domain string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Millisecond)
	defer cancel()
	url := fmt.Sprintf("https://%s/dns-query?name=%s&type=A", server, domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/dns-json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	resp, err := (&http.Client{Timeout: 1800 * time.Millisecond, Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DoH 返回状态：%d", resp.StatusCode)
	}
	return nil
}

func buildDNSConfig(settings Settings, final string, dnsPlan autoDNSPlan, includeRemoteRuleSets bool, nodes []Node) map[string]interface{} {
	dnsPlan = normalizeAutoDNSPlan(dnsPlan)
	dnsFinal := "remote"
	if !includeRemoteRuleSets {
		dnsFinal = "local"
	}
	strategy := "prefer_ipv4"
	if effectiveTun(settings) {
		strategy = "ipv4_only"
		settings.FakeDNS = true
	}
	servers := []map[string]interface{}{
		{"tag": "local", "type": "local"},
		{"tag": "china", "type": dnsPlan.China.Type, "server": dnsPlan.China.Server},
	}
	if dnsFinal == "remote" {
		servers = append(servers, map[string]interface{}{"tag": "remote", "type": dnsPlan.Remote.Type, "server": dnsPlan.Remote.Server, "detour": final})
	}
	if settings.FakeDNS {
		servers = append(servers, map[string]interface{}{
			"tag":         "fakeip",
			"type":        "fakeip",
			"inet4_range": "198.18.0.0/15",
		})
	}

	dnsRules := []map[string]interface{}{}

	dnsRules = append(dnsRules, map[string]interface{}{"clash_mode": "direct", "action": "route", "server": "local"})

	if settings.FakeDNS {
		dnsRules = append(dnsRules, map[string]interface{}{
			"query_type": []string{"HTTPS"},
			"action":     "reject",
		})
	}

	if nodeDomains := extractNodeServerDomains(nodes); len(nodeDomains) > 0 {
		dnsRules = append(dnsRules, map[string]interface{}{
			"domain":        nodeDomains,
			"domain_suffix": nodeDomains,
			"action":        "route",
			"server":        "china",
		})
	}

	if directDomains, _ := parseDirectEntries(settings.DirectDomains); len(directDomains) > 0 {
		dnsRules = append(dnsRules, map[string]interface{}{
			"domain":        directDomains,
			"domain_suffix": directDomains,
			"clash_mode":    "rule",
			"action":        "route",
			"server":        "china",
		})
	}

	if includeRemoteRuleSets {
		if _, ok := rulePresetDefs[settings.RuleSource]; ok {
			if settings.FakeDNS {
				fakeProxyRule := map[string]interface{}{
					"rule_set":   "loyalsoldier-proxy",
					"clash_mode": "rule",
					"action":     "route",
					"server":     "fakeip",
				}
				if effectiveTun(settings) {
					fakeProxyRule["inbound"] = []string{"tun-in"}
				}
				dnsRules = append(dnsRules, fakeProxyRule)
				dnsRules = append(dnsRules, map[string]interface{}{
					"rule_set":   "loyalsoldier-proxy",
					"clash_mode": "rule",
					"action":     "route",
					"server":     "remote",
				})
			} else {
				dnsRules = append(dnsRules, map[string]interface{}{"rule_set": "loyalsoldier-proxy", "clash_mode": "rule", "action": "route", "server": "remote"})
			}
			dnsRules = append(dnsRules,
				map[string]interface{}{"rule_set": "loyalsoldier-direct", "clash_mode": "rule", "action": "route", "server": "china"},
				map[string]interface{}{"rule_set": "loyalsoldier-china", "clash_mode": "rule", "action": "route", "server": "china"},
			)
		}
	}

	dnsRules = append(dnsRules,
		map[string]interface{}{"domain_suffix": []string{"cn"}, "clash_mode": "rule", "action": "route", "server": "china"},
		map[string]interface{}{"domain_keyword": []string{
			"baidu", "bilibili", "qq", "taobao", "tmall", "jd", "alicdn", "aliyun",
			"weixin", "wechat", "douyin", "bytedance", "zhihu", "netease",
		}, "clash_mode": "rule", "action": "route", "server": "china"},
	)

	if settings.FakeDNS {
		fakeIPRule := map[string]interface{}{"query_type": []string{"A", "AAAA"}, "action": "route", "server": "fakeip"}
		if effectiveTun(settings) {
			fakeIPRule["inbound"] = []string{"tun-in"}
		}
		dnsRules = append(dnsRules, fakeIPRule)
	}
	return map[string]interface{}{
		"servers":  servers,
		"rules":    dnsRules,
		"final":    dnsFinal,
		"strategy": strategy,
	}
}

func extractNodeServerDomains(nodes []Node) []string {
	seen := make(map[string]struct{})
	var domains []string
	for _, n := range nodes {
		for _, raw := range []string{n.Address, stringFromAny(n.Outbound["server"])} {
			addr := strings.TrimSpace(raw)
			if addr == "" {
				continue
			}
			if host, _, err := net.SplitHostPort(addr); err == nil {
				addr = host
			}
			addr = strings.Trim(addr, "[]")
			if net.ParseIP(addr) != nil {
				continue
			}
			addr = strings.ToLower(addr)
			if _, ok := seen[addr]; !ok {
				seen[addr] = struct{}{}
				domains = append(domains, addr)
			}
		}
	}
	return domains
}

func extractNodeServerIPs(nodes []Node) []string {
	seen := make(map[string]struct{})
	var ips []string
	for _, n := range nodes {
		for _, raw := range []string{n.Address, stringFromAny(n.Outbound["server"])} {
			addr := strings.TrimSpace(raw)
			if addr == "" {
				continue
			}
			if host, _, err := net.SplitHostPort(addr); err == nil {
				addr = host
			}
			addr = strings.Trim(addr, "[]")
			parsed := net.ParseIP(addr)
			if parsed == nil {
				continue
			}
			var cidr string
			if parsed.To4() != nil {
				cidr = addr + "/32"
			} else {
				cidr = addr + "/128"
			}
			if _, ok := seen[cidr]; !ok {
				seen[cidr] = struct{}{}
				ips = append(ips, cidr)
			}
		}
	}
	return ips
}

func normalizeDirectDomainItem(item string) string {
	item = strings.TrimSpace(strings.ToLower(item))
	if item == "" {
		return ""
	}
	item = strings.TrimPrefix(item, "http://")
	item = strings.TrimPrefix(item, "https://")

	// IPv6 bracketed format: [::1]:8080 or [::1]
	if strings.HasPrefix(item, "[") {
		if end := strings.Index(item, "]"); end > 0 {
			v6 := item[1:end]
			if ip := net.ParseIP(v6); ip != nil {
				return ip.String()
			}
		}
	}

	// CIDR format: 192.168.1.0/24 or 2001:db8::/32
	if strings.Contains(item, "/") {
		if _, ipNet, err := net.ParseCIDR(item); err == nil {
			return ipNet.String()
		}
		// If not a valid CIDR, strip path/query
		if i := strings.IndexAny(item, "/?#"); i >= 0 {
			item = item[:i]
		}
	} else if i := strings.IndexAny(item, "?#"); i >= 0 {
		item = item[:i]
	}

	// Single colon host:port or ip:port (avoid splitting IPv6 addresses with multiple colons)
	if strings.Count(item, ":") == 1 {
		if host, _, err := net.SplitHostPort(item); err == nil {
			item = host
		}
	}

	item = strings.TrimPrefix(item, "*.")
	item = strings.TrimPrefix(item, ".")
	item = strings.TrimSuffix(item, ".")

	if item == "" {
		return ""
	}

	// Validate as IP
	if ip := net.ParseIP(item); ip != nil {
		return ip.String()
	}

	// Validate as CIDR
	if _, ipNet, err := net.ParseCIDR(item); err == nil {
		return ipNet.String()
	}

	// Localhost
	if item == "localhost" {
		return item
	}

	// Validate as domain
	if len(item) <= 253 && !strings.Contains(item, "..") && strings.Contains(item, ".") {
		valid := true
		for _, part := range strings.Split(item, ".") {
			if part == "" || len(part) > 63 {
				valid = false
				break
			}
			for i, r := range part {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
					valid = false
					break
				}
				if (i == 0 || i == len(part)-1) && (r == '-' || r == '_') {
					valid = false
					break
				}
			}
			if !valid {
				break
			}
		}
		if valid {
			return item
		}
	}

	return ""
}

func normalizeDirectDomains(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	results := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, raw := range fields {
		norm := normalizeDirectDomainItem(raw)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		results = append(results, norm)
	}
	return results
}

func parseDirectEntries(input string) (domains []string, ipCIDRs []string) {
	items := normalizeDirectDomains(input)
	domainSeen := map[string]bool{}
	cidrSeen := map[string]bool{}

	for _, item := range items {
		if _, ipNet, err := net.ParseCIDR(item); err == nil {
			cidr := ipNet.String()
			if !cidrSeen[cidr] {
				cidrSeen[cidr] = true
				ipCIDRs = append(ipCIDRs, cidr)
			}
			continue
		}
		if ip := net.ParseIP(item); ip != nil {
			var cidr string
			if ip.To4() != nil {
				cidr = ip.String() + "/32"
			} else {
				cidr = ip.String() + "/128"
			}
			if !cidrSeen[cidr] {
				cidrSeen[cidr] = true
				ipCIDRs = append(ipCIDRs, cidr)
			}
			continue
		}
		if !domainSeen[item] {
			domainSeen[item] = true
			domains = append(domains, item)
		}
	}
	return domains, ipCIDRs
}

func findSelectedNode(nodes []Node, selectedID string) (Node, bool) {
	if strings.TrimSpace(selectedID) == "" {
		return Node{}, false
	}
	for _, node := range nodes {
		if node.ID == selectedID {
			return node, true
		}
	}
	return Node{}, false
}

func waitPort(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(120 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("超时")
	}
	return lastErr
}

func waitPortOrProcessExit(host string, port int, timeout time.Duration, done <-chan error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			if err == nil {
				return errors.New("sing-box 启动后已退出")
			}
			return err
		default:
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(120 * time.Millisecond)
	}
	select {
	case err := <-done:
		if err == nil {
			return errors.New("sing-box 启动后已退出")
		}
		return err
	default:
	}
	if lastErr == nil {
		lastErr = errors.New("超时")
	}
	return lastErr
}

func waitPortsOrProcessExit(host string, ports []int, timeout time.Duration, done <-chan error) (int, error) {
	if len(ports) == 0 {
		return 0, nil
	}
	pending := append([]int(nil), ports...)
	deadline := time.Now().Add(timeout)
	for len(pending) > 0 {
		next := pending[:0]
		for _, port := range pending {
			select {
			case err := <-done:
				if err == nil {
					err = errors.New("sing-box 启动后已退出")
				}
				return port, err
			default:
			}
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return port, errors.New("超时")
			}
			probeTimeout := 100 * time.Millisecond
			if remaining < probeTimeout {
				probeTimeout = remaining
			}
			if !tcpPortOpen(host, port, probeTimeout) {
				next = append(next, port)
			}
		}
		if len(next) == 0 {
			return 0, nil
		}
		pending = next
		if remaining := time.Until(deadline); remaining > 0 {
			pause := 120 * time.Millisecond
			if remaining < pause {
				pause = remaining
			}
			time.Sleep(pause)
		}
	}
	return 0, nil
}

func sequentialPorts(startPort, count int) []int {
	if startPort <= 0 || count <= 0 {
		return nil
	}
	ports := make([]int, count)
	for index := range ports {
		ports[index] = startPort + index
	}
	return ports
}

func firstUnboundTCPPort(host string, ports []int) int {
	for _, port := range ports {
		if isTCPPortFree(host, port) {
			return port
		}
	}
	return 0
}

func waitProcessPathGone(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pids, err := staleSingBoxPIDsByPath(path)
		if err == nil && len(pids) == 0 {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func terminateProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		if err := cmd.Process.Kill(); err == nil {
			return nil
		}
		if cmd.Process.Pid > 0 {
			if _, err := runCommandOutput("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F"); err == nil {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(syscall.SIGTERM)
}

func findAvailableMixedPort(preferred int) (int, error) {
	if preferred < 1024 || preferred > 65535 {
		preferred = 20808
	}
	for port := preferred; port <= 65535; port++ {
		if isTCPPortFree("127.0.0.1", port) {
			return port, nil
		}
	}
	for port := 1024; port < preferred; port++ {
		if isTCPPortFree("127.0.0.1", port) {
			return port, nil
		}
	}
	return 0, errors.New("未找到可用本地代理端口，请关闭占用端口的程序后重试")
}

func isTCPPortFree(host string, port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func tcpPortOpen(host string, port int, timeout time.Duration) bool {
	if port < 1 || port > 65535 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func verifyLocalProxyConnectivity(port int, timeout time.Duration) error {
	return verifyLocalProxyConnectivityURLs(port, timeout, tunConnectivityProbeURLs)
}

func verifyLocalProxyConnectivityURLs(port int, timeout time.Duration, probeURLs []string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("本地代理端口 %d 无效", port)
	}
	if timeout <= 0 {
		timeout = tunConnectivityTimeout
	}
	if len(probeURLs) == 0 {
		return errors.New("没有可用的网络验证地址")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	dialTimeout := timeout
	if dialTimeout > 3*time.Second {
		dialTimeout = 3 * time.Second
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
		TLSHandshakeTimeout: dialTimeout,
		DisableKeepAlives:   true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	type probeResult struct {
		url string
		err error
	}
	results := make(chan probeResult, len(probeURLs))
	for _, rawURL := range probeURLs {
		rawURL := strings.TrimSpace(rawURL)
		go func() {
			if rawURL == "" {
				results <- probeResult{err: errors.New("验证地址为空")}
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "SanTian-Singbox/TUN-Check")
				var resp *http.Response
				resp, err = client.Do(req)
				if resp != nil {
					_ = resp.Body.Close()
					if err == nil && (resp.StatusCode == http.StatusProxyAuthRequired || resp.StatusCode >= 500) {
						err = fmt.Errorf("HTTP %d", resp.StatusCode)
					}
				}
			}
			results <- probeResult{url: rawURL, err: err}
		}()
	}

	failures := make([]string, 0, len(probeURLs))
	for range probeURLs {
		select {
		case result := <-results:
			if result.err == nil {
				cancel()
				return nil
			}
			failures = append(failures, fmt.Sprintf("%s：%v", result.url, result.err))
		case <-ctx.Done():
			return fmt.Errorf("代理出网验证超时：%w", ctx.Err())
		}
	}
	return fmt.Errorf("代理无法访问验证地址：%s", strings.Join(failures, "；"))
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func wireGuardEndpointFromOutbound(outbound map[string]interface{}) map[string]interface{} {
	if strings.ToLower(stringFromAny(outbound["type"])) != "wireguard" {
		return nil
	}
	tag := stringFromAny(outbound["tag"])
	endpoint := map[string]interface{}{
		"type":        "wireguard",
		"tag":         tag,
		"address":     stringListFromAny(outbound["local_address"]),
		"private_key": stringFromAny(outbound["private_key"]),
		"peers": []map[string]interface{}{
			{
				"address":     stringFromAny(outbound["server"]),
				"port":        intFromAny(outbound["server_port"]),
				"public_key":  stringFromAny(outbound["peer_public_key"]),
				"allowed_ips": []string{"0.0.0.0/0", "::/0"},
			},
		},
	}
	if mtu := intFromAny(outbound["mtu"]); mtu > 0 {
		endpoint["mtu"] = mtu
	}
	if preSharedKey := stringFromAny(outbound["pre_shared_key"]); preSharedKey != "" {
		endpoint["peers"].([]map[string]interface{})[0]["pre_shared_key"] = preSharedKey
	}
	if reserved := intListFromAny(outbound["reserved"]); len(reserved) == 3 {
		endpoint["peers"].([]map[string]interface{})[0]["reserved"] = reserved
	}
	return endpoint
}

func normalizeECHConfigToPEMLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if strings.Contains(raw, "-----BEGIN ECH CONFIGS-----") {
		block, _ := pem.Decode([]byte(raw))
		if block != nil && block.Type == "ECH CONFIGS" {
			lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
			var cleaned []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					cleaned = append(cleaned, line)
				}
			}
			if len(cleaned) > 0 {
				return cleaned
			}
		}
	}

	cleanB64 := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == ',' || r == '"' || r == '\'' {
			return -1
		}
		return r
	}, raw)
	if cleanB64 == "" {
		return nil
	}

	var decoded []byte
	var err error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err = enc.DecodeString(cleanB64)
		if err == nil && len(decoded) > 0 {
			break
		}
	}
	if err != nil || len(decoded) < 20 {
		return nil
	}

	b64Str := base64.StdEncoding.EncodeToString(decoded)
	pemStr := "-----BEGIN ECH CONFIGS-----\n" + b64Str + "\n-----END ECH CONFIGS-----"
	block, _ := pem.Decode([]byte(pemStr))
	if block != nil && block.Type == "ECH CONFIGS" {
		return []string{
			"-----BEGIN ECH CONFIGS-----",
			b64Str,
			"-----END ECH CONFIGS-----",
		}
	}
	return nil
}

func sanitizeOutboundForSingBox(outbound map[string]interface{}) map[string]interface{} {
	outbound = cloneMap(outbound)
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok || tls == nil {
		return outbound
	}

	ech, ok := tls["ech"].(map[string]interface{})
	if !ok || ech == nil {
		return outbound
	}

	enabledVal := stringFromAny(ech["enabled"])
	if isExplicitFalse(enabledVal) || (!boolFromAny(ech["enabled"]) && !isExplicitTrue(enabledVal)) {
		delete(tls, "ech")
		return outbound
	}

	ech["enabled"] = true

	if rawConfig := ech["config"]; rawConfig != nil {
		lines := stringListFromAny(rawConfig)
		joined := strings.TrimSpace(strings.Join(lines, "\n"))
		if joined == "" || isExplicitFalse(joined) || isExplicitTrue(joined) {
			delete(ech, "config")
		} else {
			pemLines := normalizeECHConfigToPEMLines(joined)
			if len(pemLines) > 0 {
				ech["config"] = pemLines
			} else {
				delete(ech, "config")
			}
		}
	}

	return outbound
}

func stringListFromAny(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if text := stringFromAny(item); text != "" {
				items = append(items, text)
			}
		}
		return items
	case string:
		return splitCSV(v)
	default:
		return nil
	}
}

func intListFromAny(value interface{}) []int {
	switch v := value.(type) {
	case []int:
		return v
	case []interface{}:
		items := make([]int, 0, len(v))
		for _, item := range v {
			if n := intFromAny(item); n >= 0 {
				items = append(items, n)
			}
		}
		return items
	case string:
		return parseReserved(v)
	default:
		return nil
	}
}

func runCommandOutput(name string, args ...string) (string, error) {
	return runCommandOutputWithEnv(nil, name, args...)
}

func runCommandOutputWithEnv(env map[string]string, name string, args ...string) (string, error) {
	return runCommandOutputWithEnvTimeout(commandOutputTimeout, env, name, args...)
}

func runCommandOutputWithEnvTimeout(timeout time.Duration, env map[string]string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideCommandWindow(cmd)
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err := cmd.Run()
	output := strings.TrimSpace(buffer.String())
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("命令执行超时（%s）", timeout)
	}
	return output, err
}

func hideCommandWindow(cmd *exec.Cmd) {
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}
}

func latestCoreAsset(proxyPort int) (string, string, error) {
	body, err := fetchCoreRelease(proxyPort)
	if err != nil {
		return "", "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", "", err
	}
	target := runtime.GOOS + "-" + runtime.GOARCH
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".zip") && strings.Contains(name, target) && !strings.Contains(name, "legacy") {
			return asset.URL, release.TagName, nil
		}
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".zip") && strings.Contains(name, target) {
			return asset.URL, release.TagName, nil
		}
	}
	return "", "", fmt.Errorf("未找到适配 %s 的 sing-box 内核包", target)
}

func fetchCoreRelease(proxyPort int) ([]byte, error) {
	const releaseURL = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
	body, err := httpGetBytesFromOptions(coreNetworkOptions("GitHub API", releaseURL, proxyPort), 45*time.Second, maxReleaseMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("获取最新内核版本失败：%w", err)
	}
	return body, nil
}

func normalizeCoreVersionTag(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	version = strings.TrimPrefix(version, "sing-box version")
	version = strings.TrimSpace(version)
	if index := strings.IndexAny(version, " \r\n\t"); index >= 0 {
		version = version[:index]
	}
	version = strings.TrimPrefix(version, "v")
	if version == "" || version == "未知" || version == "未安装" {
		return ""
	}
	return version
}

func downloadCoreAsset(assetURL, version, path string, proxyPort int, logger *Logger) error {
	var lastErr error
	for _, option := range coreDownloadOptions(assetURL, version, proxyPort) {
		attempts := 1
		if option.proxyURL != "" {
			attempts = localProxyDownloadAttempts
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			if logger != nil {
				if attempt == 1 {
					logger.Info("正在下载 sing-box 内核：%s", option.label)
				} else {
					logger.Info("正在重试 sing-box 内核下载：%s（%d/%d）", option.label, attempt, attempts)
				}
			}
			err := downloadFile(option.url, path, option.proxyURL)
			if err == nil {
				return nil
			}
			lastErr = err
			_ = os.Remove(path)
			if attempt < attempts && isRetryableNetworkError(err) {
				time.Sleep(downloadRetryDelay)
				continue
			}
			if logger != nil {
				logger.Error("sing-box 内核下载失败（%s）：%v", option.label, err)
			}
			break
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("没有可用下载地址")
}

type coreDownloadOption struct {
	label    string
	url      string
	proxyURL string
}

func coreNetworkOptions(label, rawURL string, proxyPort int) []coreDownloadOption {
	result := []coreDownloadOption{{label: label, url: rawURL}}
	if proxyPort > 0 {
		proxyOption := coreDownloadOption{
			label:    "本地代理 " + label,
			url:      rawURL,
			proxyURL: fmt.Sprintf("http://127.0.0.1:%d", proxyPort),
		}
		if !isTCPPortFree("127.0.0.1", proxyPort) {
			return []coreDownloadOption{proxyOption, result[0]}
		}
		result = append(result, proxyOption)
	}
	return result
}

func coreDownloadOptions(assetURL, version string, proxyPort int) []coreDownloadOption {
	fileName := coreAssetFileName(assetURL)
	directURL := ""
	if fileName != "" && strings.TrimSpace(version) != "" {
		directURL = "https://github.com/SagerNet/sing-box/releases/download/" + strings.TrimSpace(version) + "/" + fileName
	}
	proxyURL := ""
	if proxyPort > 0 {
		proxyURL = fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	}
	baseOptions := []coreDownloadOption{{label: "GitHub 资产地址", url: assetURL}}
	if directURL != "" && directURL != assetURL {
		baseOptions = append(baseOptions, coreDownloadOption{label: "GitHub 直链地址", url: directURL})
	}
	result := append([]coreDownloadOption{}, baseOptions...)
	if proxyURL != "" {
		proxyOptions := []coreDownloadOption{{label: "本地代理 GitHub 资产地址", url: assetURL, proxyURL: proxyURL}}
		if directURL != "" && directURL != assetURL {
			proxyOptions = append(proxyOptions, coreDownloadOption{label: "本地代理 GitHub 直链地址", url: directURL, proxyURL: proxyURL})
		}
		if !isTCPPortFree("127.0.0.1", proxyPort) {
			return append(proxyOptions, baseOptions...)
		}
		result = append(result, proxyOptions...)
	}
	return result
}

func coreAssetFileName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func downloadFile(rawURL, filePath, proxyURL string) error {
	resp, closeTransport, err := openHTTPDownload(rawURL, proxyURL, 45*time.Second)
	if err != nil {
		return err
	}
	defer closeTransport()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxCoreDownloadBytes {
		return fmt.Errorf("下载内容超过 %d MB 限制", maxCoreDownloadBytes/(1024*1024))
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	removeOnError := true
	defer func() {
		_ = file.Close()
		if removeOnError {
			_ = os.Remove(filePath)
		}
	}()
	written, err := io.Copy(file, io.LimitReader(resp.Body, maxCoreDownloadBytes+1))
	if err != nil {
		return err
	}
	if written > maxCoreDownloadBytes {
		return fmt.Errorf("下载内容超过 %d MB 限制", maxCoreDownloadBytes/(1024*1024))
	}
	if err := file.Close(); err != nil {
		return err
	}
	removeOnError = false
	return nil
}

func httpGetBytes(rawURL, proxyURL string, timeout time.Duration, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("下载大小限制无效")
	}
	resp, closeTransport, err := openHTTPDownload(rawURL, proxyURL, timeout)
	if err != nil {
		return nil, err
	}
	defer closeTransport()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("下载内容超过 %d MB 限制", maxBytes/(1024*1024))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("下载内容超过 %d MB 限制", maxBytes/(1024*1024))
	}
	return body, nil
}

func httpGetBytesFromOptions(options []coreDownloadOption, timeout time.Duration, maxBytes int64) ([]byte, error) {
	var lastErr error
	hasProxy := false
	for _, option := range options {
		attempts := 1
		if option.proxyURL != "" {
			hasProxy = true
			attempts = localProxyDownloadAttempts
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			body, err := httpGetBytes(option.url, option.proxyURL, timeout, maxBytes)
			if err == nil {
				return body, nil
			}
			lastErr = err
			if attempt < attempts && isRetryableNetworkError(err) {
				time.Sleep(downloadRetryDelay)
				continue
			}
			break
		}
	}
	if lastErr == nil {
		return nil, errors.New("没有可用下载地址")
	}
	if hasProxy {
		return nil, fmt.Errorf("本地代理与直连下载均失败，最后错误：%w", lastErr)
	}
	return nil, lastErr
}

func openHTTPDownload(rawURL, proxyURL string, timeout time.Duration) (*http.Response, func(), error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "GoSingBox")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Timeout: timeout, Transport: transport}
	if strings.TrimSpace(proxyURL) != "" {
		proxyParsed, err := url.Parse(proxyURL)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, nil, err
		}
		transport.Proxy = http.ProxyURL(proxyParsed)
	}
	resp, err := client.Do(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, nil, err
	}
	return resp, transport.CloseIdleConnections, nil
}

type extractedCoreResult struct {
	coreExe        string
	companionFiles []string
}

func extractCore(zipPath, dstDir string) (extractedCoreResult, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return extractedCoreResult{}, err
	}
	defer reader.Close()

	var result extractedCoreResult
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(file.Name)
		isCore := base == exeName("sing-box")
		isDll := strings.HasSuffix(strings.ToLower(base), ".dll")
		if !isCore && !isDll {
			continue
		}
		if file.UncompressedSize64 > maxCoreDownloadBytes {
			return extractedCoreResult{}, errors.New("压缩包中的组件超过大小限制")
		}
		src, err := file.Open()
		if err != nil {
			return extractedCoreResult{}, err
		}
		target := filepath.Join(dstDir, base)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			_ = src.Close()
			return extractedCoreResult{}, err
		}
		written, err := io.Copy(out, io.LimitReader(src, maxCoreDownloadBytes+1))
		_ = src.Close()
		if err != nil {
			_ = out.Close()
			return extractedCoreResult{}, err
		}
		if written > maxCoreDownloadBytes {
			_ = out.Close()
			_ = os.Remove(target)
			return extractedCoreResult{}, errors.New("压缩包中的组件超过大小限制")
		}
		if err := out.Close(); err != nil {
			return extractedCoreResult{}, err
		}
		if isCore {
			result.coreExe = target
		} else if isDll {
			result.companionFiles = append(result.companionFiles, target)
		}
	}
	if result.coreExe == "" {
		return extractedCoreResult{}, errors.New("压缩包中未找到 sing-box 内核")
	}
	return result, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

type swappedFileInfo struct {
	targetPath string
	backupPath string
	hadOld     bool
}

func swapCoreFiles(pairs []stagedFilePair) ([]swappedFileInfo, error) {
	swapped := make([]swappedFileInfo, 0, len(pairs))
	for _, pair := range pairs {
		if pair.stagedPath == "" {
			continue
		}
		if _, err := os.Stat(pair.stagedPath); err != nil {
			_ = rollbackCoreFiles(swapped)
			return nil, fmt.Errorf("读取待安装组件失败：%w", err)
		}
		backupPath := pair.targetPath + ".bak"
		_ = os.Remove(backupPath)
		hadOld := false
		if _, err := os.Stat(pair.targetPath); err == nil {
			hadOld = true
			if err := os.Rename(pair.targetPath, backupPath); err != nil {
				_ = rollbackCoreFiles(swapped)
				return nil, fmt.Errorf("备份组件 %s 失败：%w", filepath.Base(pair.targetPath), err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = rollbackCoreFiles(swapped)
			return nil, fmt.Errorf("检查组件 %s 失败：%w", filepath.Base(pair.targetPath), err)
		}
		if err := os.Rename(pair.stagedPath, pair.targetPath); err != nil {
			if hadOld {
				_ = os.Rename(backupPath, pair.targetPath)
			}
			_ = rollbackCoreFiles(swapped)
			return nil, fmt.Errorf("替换组件 %s 失败：%w", filepath.Base(pair.targetPath), err)
		}
		swapped = append(swapped, swappedFileInfo{
			targetPath: pair.targetPath,
			backupPath: backupPath,
			hadOld:     hadOld,
		})
	}
	return swapped, nil
}

func rollbackCoreFiles(swapped []swappedFileInfo) error {
	var errs []error
	for i := len(swapped) - 1; i >= 0; i-- {
		info := swapped[i]
		_ = os.Remove(info.targetPath)
		if info.hadOld {
			if err := os.Rename(info.backupPath, info.targetPath); err != nil {
				errs = append(errs, fmt.Errorf("恢复原组件 %s 失败：%w", filepath.Base(info.targetPath), err))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func cleanupCoreBackups(swapped []swappedFileInfo) {
	for _, info := range swapped {
		if info.backupPath != "" {
			_ = os.Remove(info.backupPath)
		}
	}
}

func swapCoreExecutable(corePath, stagedPath string) (string, bool, error) {
	swapped, err := swapCoreFiles([]stagedFilePair{{targetPath: corePath, stagedPath: stagedPath}})
	if err != nil {
		return "", false, err
	}
	if len(swapped) > 0 {
		return swapped[0].backupPath, swapped[0].hadOld, nil
	}
	return "", false, nil
}

func rollbackCoreExecutable(corePath, backupPath string, hadOld bool) error {
	return rollbackCoreFiles([]swappedFileInfo{{targetPath: corePath, backupPath: backupPath, hadOld: hadOld}})
}
