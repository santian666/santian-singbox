package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

const livePressureMixedPort = 21808

func TestLiveCoreCandidateValidation(t *testing.T) {
	corePath := os.Getenv("SANTIAN_LIVE_CORE")
	if corePath == "" {
		t.Skip("未设置现场内核路径")
	}
	output, err := exec.Command(corePath, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	version := normalizeCoreVersionTag(string(output))
	if version == "" {
		t.Fatalf("无法识别当前内核版本：%s", output)
	}
	candidate := filepath.Join(t.TempDir(), "sing-box.exe.new")
	if err := copyFile(corePath, candidate); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := validateCoreCandidate(candidate, filepath.Dir(corePath), "", version, false); err != nil {
		t.Fatal(err)
	}
	t.Logf("新内核候选文件验收通过：版本=%s 耗时=%s", version, time.Since(started))
}

func TestLivePrepareCoreUpdate(t *testing.T) {
	corePath := os.Getenv("SANTIAN_LIVE_CORE_UPDATE")
	if corePath == "" {
		t.Skip("未设置现场内核更新测试路径")
	}
	root := t.TempDir()
	testCorePath := filepath.Join(root, "bin", exeName("sing-box"))
	if err := os.MkdirAll(filepath.Dir(testCorePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(corePath, testCorePath); err != nil {
		t.Fatal(err)
	}
	logger := NewLogger(filepath.Join(root, "core-update.log"))
	defer logger.Close()
	manager := NewSingBoxManager(root, testCorePath, filepath.Join(root, "sing-box.json"), filepath.Join(root, "cache.db"), logger)
	manager.SetSOCKS5TestProxyPort(20808)
	started := time.Now()
	version, candidate, err := manager.PrepareCoreUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if candidate != nil {
		defer candidate.Cleanup()
	}
	t.Logf("内核更新准备链路通过：目标版本=%s 需要更新=%v 耗时=%s", version, candidate != nil, time.Since(started))
}

func TestLivePressureSystemProxyChangeLatency(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_SYSTEM_PROXY") != "1" {
		t.Skip("未启用现场系统代理切换测试")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:21808")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	defer DisableSystemProxy()

	started := time.Now()
	if err := EnableSystemProxy(21808); err != nil {
		t.Fatal(err)
	}
	t.Logf("开启系统代理耗时：%s", time.Since(started))
	started = time.Now()
	if err := DisableSystemProxy(); err != nil {
		t.Fatal(err)
	}
	t.Logf("关闭系统代理耗时：%s", time.Since(started))
}

type livePressureFixture struct {
	state    PersistState
	root     string
	corePath string
	cache    string
}

func loadLivePressureFixture(t *testing.T) livePressureFixture {
	t.Helper()
	statePath := os.Getenv("SANTIAN_LIVE_STATE")
	root := os.Getenv("SANTIAN_LIVE_ROOT")
	cache := os.Getenv("SANTIAN_LIVE_CACHE")
	if statePath == "" || root == "" || cache == "" {
		t.Skip("未设置现场压力测试环境")
	}
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if len(state.Nodes) < 3000 {
		t.Fatalf("现场节点不足 3000 个：%d", len(state.Nodes))
	}
	if !stateHasSelectedNode(state) {
		state.SelectedID = state.Nodes[0].ID
	}
	return livePressureFixture{
		state:    state,
		root:     root,
		corePath: filepath.Join(root, "bin", exeName("sing-box")),
		cache:    cache,
	}
}

func (fixture livePressureFixture) settings(mode string, systemProxy, tun bool) PersistState {
	state := cloneState(fixture.state)
	settings := state.Settings
	settings.Mode = mode
	settings.SystemProxy = systemProxy
	settings.Tun = tun
	settings.AppProxy = false
	settings.ProxyMap = false
	settings.MixedPort = livePressureMixedPort
	settings.CoreLogLevel = "warn"
	normalizeSettings(&settings)
	state.Settings = settings
	return state
}

func TestLivePressureConfigMatrix(t *testing.T) {
	fixture := loadLivePressureFixture(t)
	selected, ok := findSelectedNode(fixture.state.Nodes, fixture.state.SelectedID)
	if !ok {
		t.Fatal("没有有效选择节点")
	}
	cases := []struct {
		name        string
		mode        string
		systemProxy bool
		tun         bool
	}{
		{name: "system-global", mode: "global", systemProxy: true},
		{name: "system-rule", mode: "rule", systemProxy: true},
		{name: "tun-global", mode: "global", tun: true},
		{name: "tun-rule", mode: "rule", tun: true},
	}
	started := time.Now()
	for iteration := 0; iteration < 10; iteration++ {
		for _, testCase := range cases {
			state := fixture.settings(testCase.mode, testCase.systemProxy, testCase.tun)
			config, err := BuildSingBoxConfig(state, selected, fixture.cache)
			if err != nil {
				t.Fatalf("%s build: %v", testCase.name, err)
			}
			if _, err := json.Marshal(config); err != nil {
				t.Fatalf("%s marshal: %v", testCase.name, err)
			}
		}
	}
	t.Logf("3091 节点四种配置生成 40 次耗时：%s", time.Since(started))

	for _, testCase := range cases {
		state := fixture.settings(testCase.mode, testCase.systemProxy, testCase.tun)
		config, err := BuildSingBoxConfig(state, selected, fixture.cache)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), testCase.name+".json")
		data, _ := json.Marshal(config)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, fixture.corePath, "check", "-c", path)
		cmd.Dir = fixture.root
		cmd.Env = directNetworkEnv(os.Environ())
		hideCommandWindow(cmd)
		output, checkErr := cmd.CombinedOutput()
		cancel()
		if checkErr != nil {
			t.Fatalf("%s check: %v: %s", testCase.name, checkErr, trimForLog(string(output)))
		}
	}
}

func TestLivePressureTCPAllNodes(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_NETWORK") != "1" {
		t.Skip("未启用现场网络压力测试")
	}
	fixture := loadLivePressureFixture(t)
	logger := NewLogger(filepath.Join(t.TempDir(), "tcp.log"))
	defer logger.Close()
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "tcp.json"), fixture.cache, logger)
	app := &App{manager: manager, logger: logger}
	started := time.Now()
	results := app.testNodeListBatch(fixture.state.Nodes, "tcp", batchNodeTestConcurrency(len(fixture.state.Nodes)))
	success := 0
	for _, result := range results {
		if result.err == nil && result.metric.DelayMS > 0 {
			success++
		}
	}
	t.Logf("TCP 批测完成：总数=%d 成功=%d 失败=%d 耗时=%s", len(fixture.state.Nodes), success, len(fixture.state.Nodes)-success, time.Since(started))
	if success == 0 {
		t.Fatal("所有节点 TCP 批测均失败")
	}
}

func TestLivePressureRealDelay(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_NETWORK") != "1" {
		t.Skip("未启用现场网络压力测试")
	}
	fixture := loadLivePressureFixture(t)
	limit := 900
	if len(fixture.state.Nodes) < limit {
		limit = len(fixture.state.Nodes)
	}
	logger := NewLogger(filepath.Join(t.TempDir(), "real-delay.log"))
	defer logger.Close()
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "real-delay.json"), fixture.cache, logger)
	started := time.Now()
	results := manager.TestNodesDelay(fixture.state.Nodes[:limit], SpeedTestOptions{DirectOnly: true, FastDelay: true})
	success := 0
	for _, result := range results {
		if result.err == nil && result.result.DelayMS > 0 {
			success++
		}
	}
	t.Logf("真连接分批测试完成：总数=%d 成功=%d 失败=%d 耗时=%s", limit, success, limit-success, time.Since(started))
	if success == 0 {
		t.Fatal("所有节点真连接测试均失败")
	}
}

func TestLivePressureStoreOperations(t *testing.T) {
	fixture := loadLivePressureFixture(t)
	started := time.Now()
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceGroupsAndNodes(fixture.state.Groups, fixture.state.Nodes, fixture.state.SelectedID); err != nil {
		t.Fatal(err)
	}

	groupIDs := make([]string, 0, len(fixture.state.Groups))
	for index := len(fixture.state.Groups) - 1; index >= 0; index-- {
		groupIDs = append(groupIDs, fixture.state.Groups[index].ID)
	}
	if err := store.ReorderGroups(groupIDs); err != nil {
		t.Fatal(err)
	}
	largestGroup := ""
	largestIDs := []string{}
	groupNodes := make(map[string][]string)
	for _, node := range fixture.state.Nodes {
		groupNodes[node.GroupID] = append(groupNodes[node.GroupID], node.ID)
	}
	for groupID, ids := range groupNodes {
		if len(ids) > len(largestIDs) {
			largestGroup = groupID
			largestIDs = ids
		}
	}
	for left, right := 0, len(largestIDs)-1; left < right; left, right = left+1, right-1 {
		largestIDs[left], largestIDs[right] = largestIDs[right], largestIDs[left]
	}
	if err := store.ReorderNodes(largestGroup, largestIDs); err != nil {
		t.Fatal(err)
	}

	updates := make(map[string]NodeDelayUpdate, len(fixture.state.Nodes))
	for index, node := range fixture.state.Nodes {
		updates[node.ID] = NodeDelayUpdate{DelayMS: index%300 + 1, LastTest: "pressure"}
	}
	if err := store.UpdateNodeDelays(updates); err != nil {
		t.Fatal(err)
	}

	links := make([]exportLinkRow, 0, len(fixture.state.Nodes))
	for _, node := range fixture.state.Nodes {
		link, err := nodeShareLink(node)
		if err != nil {
			t.Fatalf("生成分享链接失败：%s: %v", node.Name, err)
		}
		links = append(links, exportLinkRow{Name: node.Name, Link: link})
	}
	qrLimit := 500
	if len(links) < qrLimit {
		qrLimit = len(links)
	}
	if err := writeGroupLinksXLSXWithQR(filepath.Join(t.TempDir(), "pressure.xlsx"), links[:qrLimit]); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				if len(store.Snapshot().Nodes) != len(fixture.state.Nodes) {
					t.Errorf("并发快照节点数量不一致")
					return
				}
			}
		}()
	}
	wait.Wait()
	if len(store.Snapshot().Nodes) != len(fixture.state.Nodes) {
		t.Fatal("压力操作后节点数量发生变化")
	}
	t.Logf("3091 节点保存、分组排序、最大组排序、批量更新、全部分享、500 二维码和 1600 次并发快照耗时=%s", time.Since(started))
}

func TestLivePressureProxyModes(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_SYSTEM") != "1" {
		t.Skip("未启用现场系统代理压力测试")
	}
	fixture := loadLivePressureFixture(t)
	logger := NewLogger(filepath.Join(t.TempDir(), "proxy-modes.log"))
	defer logger.Close()
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "proxy-modes.json"), fixture.cache, logger)
	defer func() {
		_ = manager.Stop()
		if runtime.GOOS == "windows" {
			_ = DisableSystemProxy()
			cleanupTunInterface(logger)
		}
	}()

	state := fixture.settings("global", true, false)
	if err := manager.Start(state); err != nil {
		t.Fatal(err)
	}
	if issue, err := systemProxyConfigurationIssueForPort(livePressureMixedPort); err != nil || issue != "" {
		t.Fatalf("系统代理入口异常：issue=%s err=%v", issue, err)
	}
	if err := verifyLocalProxyConnectivity(livePressureMixedPort, 12*time.Second); err != nil {
		t.Fatalf("系统代理全局模式无法出网：%v", err)
	}

	working := make([]Node, 0, 20)
	for _, node := range state.Nodes {
		if node.DelayMS > 0 {
			working = append(working, node)
			if len(working) == 20 {
				break
			}
		}
	}
	if len(working) == 0 {
		working = append(working, state.Nodes[:20]...)
	}
	started := time.Now()
	for iteration := 0; iteration < 3; iteration++ {
		for _, node := range working {
			if err := manager.SelectOutboundVerified(nodeOutboundTag(node)); err != nil {
				t.Fatalf("快速切换节点失败：%s: %v", node.Name, err)
			}
		}
	}
	t.Logf("Clash API 已确认切换 %d 次，耗时=%s", len(working)*3, time.Since(started))

	if err := manager.SetMode("rule"); err != nil {
		t.Fatal(err)
	}
	state.Settings.Mode = "rule"
	if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
		t.Fatal(err)
	}
	if err := verifyLocalProxyConnectivity(livePressureMixedPort, 12*time.Second); err != nil {
		t.Fatalf("系统代理规则模式无法出网：%v", err)
	}
	if err := manager.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	state.Settings.Mode = "global"
	if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if issue, _ := systemProxyConfigurationIssueForPort(livePressureMixedPort); issue == "" {
		t.Fatal("停止后系统代理仍指向测试端口")
	}

	mapState := fixture.settings("global", false, false)
	groupCounts := make(map[string]int)
	for _, node := range mapState.Nodes {
		groupCounts[node.GroupID]++
	}
	groupIDs := make([]string, 0, len(groupCounts))
	for id, count := range groupCounts {
		if count >= 250 && count <= 300 {
			groupIDs = append(groupIDs, id)
		}
	}
	sort.Strings(groupIDs)
	if len(groupIDs) == 0 {
		t.Fatal("没有适合压力测试的 250-300 节点分组")
	}
	mapState.Settings.ProxyMap = true
	mapState.Settings.ProxyMapGroupIDs = []string{groupIDs[0]}
	mapState.Settings.ProxyMapStartPort = 52001
	if err := manager.Start(mapState); err != nil {
		t.Fatal(err)
	}
	mapCount := len(proxyMapNodesForSettings(mapState.Nodes, mapState.Settings))
	for _, port := range []int{52001, 52001 + mapCount/2, 52001 + mapCount - 1} {
		if !tcpPortOpen("127.0.0.1", port, time.Second) {
			t.Fatalf("代理映射端口未监听：%d", port)
		}
	}
	t.Logf("代理映射启动成功：分组=%s 端口数=%d", groupIDs[0], mapCount)
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestLivePressureTunModes(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_TUN") != "1" {
		t.Skip("未启用现场 TUN 压力测试")
	}
	if !hasAdminPrivileges() {
		t.Skip("现场 TUN 压力测试需要管理员权限")
	}
	fixture := loadLivePressureFixture(t)
	logPath := os.Getenv("SANTIAN_LIVE_RESULT")
	if logPath == "" {
		logPath = filepath.Join(t.TempDir(), "tun-modes.log")
	}
	logger := NewLogger(logPath)
	defer logger.Close()
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "tun-modes.json"), fixture.cache, logger)
	defer func() {
		_ = manager.Stop()
		cleanupTunInterface(logger)
		_ = DisableSystemProxy()
	}()

	state := fixture.settings("global", false, true)
	if err := manager.Start(state); err != nil {
		t.Fatal(err)
	}
	if code, ok := tunReadyState(); !ok {
		t.Fatalf("TUN 全局模式未接管：%s", tunReadyMessage(code))
	}
	if err := verifyLocalProxyConnectivity(livePressureMixedPort, 12*time.Second); err != nil {
		t.Fatalf("TUN 全局模式无法出网：%v", err)
	}
	logger.Info("TUN 全局模式启动和出网验证通过")
	for iteration := 0; iteration < 5; iteration++ {
		if err := manager.SetMode("rule"); err != nil {
			t.Fatal(err)
		}
		state.Settings.Mode = "rule"
		if err := manager.CloseConnections(); err != nil {
			t.Fatal(err)
		}
		if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
			t.Fatal(err)
		}
		if err := verifyLocalProxyConnectivity(livePressureMixedPort, 12*time.Second); err != nil {
			t.Fatalf("第 %d 轮 TUN 规则模式无法出网：%v", iteration+1, err)
		}
		if err := manager.SetMode("global"); err != nil {
			t.Fatal(err)
		}
		state.Settings.Mode = "global"
		if err := manager.CloseConnections(); err != nil {
			t.Fatal(err)
		}
		if err := manager.ValidateRuntimeSettings(state.Settings); err != nil {
			t.Fatal(err)
		}
		if err := verifyLocalProxyConnectivity(livePressureMixedPort, 12*time.Second); err != nil {
			t.Fatalf("第 %d 轮 TUN 全局模式无法出网：%v", iteration+1, err)
		}
	}
	logger.Info("TUN 全局/规则模式往返切换 5 轮通过")
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, ok := tunReadyState(); ok {
		t.Fatal("停止后 TUN 路由仍然存在")
	}
	if tcpPortOpen("127.0.0.1", livePressureMixedPort, 200*time.Millisecond) {
		t.Fatal("停止后测试代理端口仍在监听")
	}
	logger.Info("TUN 压力测试完成，端口、路由和网卡残留检查通过")
}

func TestLivePressureAppTransitions(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_TUN") != "1" {
		t.Skip("未启用现场 TUN 压力测试")
	}
	if !hasAdminPrivileges() {
		t.Skip("现场代理状态机压力测试需要管理员权限")
	}
	fixture := loadLivePressureFixture(t)
	logPath := os.Getenv("SANTIAN_LIVE_TRANSITION_RESULT")
	if logPath == "" {
		logPath = filepath.Join(t.TempDir(), "app-transitions.log")
	}
	logger := NewLogger(logPath)
	defer logger.Close()
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceGroupsAndNodes(fixture.state.Groups, fixture.state.Nodes, fixture.state.SelectedID); err != nil {
		t.Fatal(err)
	}
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "app-transitions.json"), fixture.cache, logger)
	app := &App{root: fixture.root, store: store, manager: manager, logger: logger, startupReady: true}
	defer func() {
		_ = manager.Stop()
		cleanupTunInterface(logger)
		_ = DisableSystemProxy()
	}()

	settings := fixture.settings("global", true, false).Settings
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("开启系统代理全局模式失败：%v", err)
	}
	if issue, err := systemProxyConfigurationIssueForPort(livePressureMixedPort); err != nil || issue != "" {
		t.Fatalf("系统代理入口异常：issue=%s err=%v", issue, err)
	}
	settings.Mode = "rule"
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("系统代理切换规则模式失败：%v", err)
	}

	settings.SystemProxy = false
	settings.Tun = true
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("系统代理切换 TUN 失败：%v", err)
	}
	if code, ok := tunReadyState(); !ok {
		t.Fatalf("切换后 TUN 未接管：%s", tunReadyMessage(code))
	}
	settings.Mode = "global"
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("TUN 切换全局模式失败：%v", err)
	}

	settings.SystemProxy = true
	settings.Tun = false
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("TUN 切回系统代理失败：%v", err)
	}
	if issue, err := systemProxyConfigurationIssueForPort(livePressureMixedPort); err != nil || issue != "" {
		t.Fatalf("切回后系统代理入口异常：issue=%s err=%v", issue, err)
	}
	if _, ok := tunReadyState(); ok {
		t.Fatal("切回系统代理后仍有 TUN 路由")
	}

	settings.SystemProxy = false
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("关闭代理入口失败：%v", err)
	}
	if manager.Running() {
		t.Fatal("关闭代理入口后 sing-box 仍在运行")
	}
	if tcpPortOpen("127.0.0.1", livePressureMixedPort, 200*time.Millisecond) {
		t.Fatal("关闭代理入口后端口仍在监听")
	}
	logger.Info("App.SaveSettings 系统代理/规则/TUN/全局/系统代理/关闭完整状态机通过")
}

func TestLivePressureTrayCommands(t *testing.T) {
	if os.Getenv("SANTIAN_LIVE_TUN") != "1" {
		t.Skip("未启用现场托盘压力测试")
	}
	if !hasAdminPrivileges() {
		t.Skip("现场托盘压力测试需要管理员权限")
	}
	fixture := loadLivePressureFixture(t)
	logPath := os.Getenv("SANTIAN_LIVE_TRAY_RESULT")
	if logPath == "" {
		logPath = filepath.Join(t.TempDir(), "tray-pressure.log")
	}
	logger := NewLogger(logPath)
	defer logger.Close()
	store, err := NewStore(filepath.Join(t.TempDir(), "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceGroupsAndNodes(fixture.state.Groups, fixture.state.Nodes, fixture.state.SelectedID); err != nil {
		t.Fatal(err)
	}
	manager := NewSingBoxManager(fixture.root, fixture.corePath, filepath.Join(t.TempDir(), "tray-pressure.json"), fixture.cache, logger)
	app := &App{root: fixture.root, store: store, manager: manager, logger: logger, startupReady: true}
	defer func() {
		_ = manager.Stop()
		cleanupTunInterface(logger)
		_ = DisableSystemProxy()
	}()

	app.trayApplySettings(trayProxySystem)
	state := store.Snapshot()
	if !manager.Running() || !state.Settings.SystemProxy || state.Settings.Tun {
		t.Fatalf("托盘系统代理未正确启动：running=%v settings=%+v", manager.Running(), state.Settings)
	}
	for iteration := 0; iteration < 10; iteration++ {
		app.trayApplySettings(trayProxyRule)
		app.trayApplySettings(trayProxyGlobal)
	}
	if err := manager.ValidateRuntimeSettings(store.Snapshot().Settings); err != nil {
		t.Fatal(err)
	}

	app.trayApplySettings(trayProxyTun)
	state = store.Snapshot()
	if !manager.Running() || !state.Settings.Tun || state.Settings.SystemProxy {
		t.Fatalf("托盘 TUN 未正确启动：running=%v settings=%+v", manager.Running(), state.Settings)
	}
	if code, ok := tunReadyState(); !ok {
		t.Fatalf("托盘 TUN 未接管：%s", tunReadyMessage(code))
	}
	for iteration := 0; iteration < 10; iteration++ {
		app.trayApplySettings(trayProxyRule)
		app.trayApplySettings(trayProxyGlobal)
	}

	app.trayApplySettings(trayProxySystem)
	state = store.Snapshot()
	if !manager.Running() || !state.Settings.SystemProxy || state.Settings.Tun {
		t.Fatalf("托盘切回系统代理失败：running=%v settings=%+v", manager.Running(), state.Settings)
	}
	app.trayStop()
	if manager.Running() || shouldRunProxy(store.Snapshot().Settings) {
		t.Fatal("托盘停止后代理入口仍在运行")
	}
	app.trayStart()
	state = store.Snapshot()
	if !manager.Running() || !state.Settings.SystemProxy || state.Settings.Tun {
		t.Fatalf("托盘恢复上次代理模式失败：running=%v settings=%+v", manager.Running(), state.Settings)
	}
	app.trayStop()
	logger.Info("3091 节点托盘系统代理/TUN/全局/规则/停止/恢复压力测试通过")
}
