package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const adminRestartArg = "--restart-proxy"
const adminTunHandoffArg = "--tun-handoff"
const groupNodeTestMaxConcurrency = 64

const temporaryCoreDelayTestMaxConcurrency = 6

const saveSettingsLockTimeout = 5 * time.Second
const forcedQuitTimeout = 12 * time.Second
const tunConnectivityCheckInterval = time.Minute
const tunConnectivityFailureCache = 2 * time.Second
const trafficSampleInterval = 2 * time.Second
const trafficPersistInterval = time.Minute
const maxManualNodeInputBytes = 16 * 1024 * 1024
const maxBatchTestNodes = 5000

var (
	errProxyOperationBusy  = errors.New("代理正在执行其他操作")
	errSystemProxyEntrance = errors.New("系统代理入口异常")
)

type App struct {
	ctx                         context.Context
	root                        string
	store                       *Store
	manager                     *SingBoxManager
	logger                      *Logger
	proxyOpMu                   sync.Mutex
	coreUpdateMu                sync.Mutex
	ruleUpdateMu                sync.Mutex
	proxyResumeMu               sync.Mutex
	proxyResume                 proxyEntrySnapshot
	proxyResumeValid            bool
	trayMu                      sync.Mutex
	trayItems                   *trayMenuItems
	healthMu                    sync.Mutex
	healthChecking              bool
	startupReady                bool
	lastHealthRestart           time.Time
	lastTunConnectivityCheck    time.Time
	lastTunConnectivityFailure  time.Time
	lastTunConnectivityError    string
	lastAutoFailover            time.Time
	circuitBreakerMu            sync.Mutex
	circuitBreakerActive        bool
	circuitBreakerText          string
	resumeCheckMu               sync.Mutex
	resumeCheckTimers           [2]*time.Timer
	tunStatusMu                 sync.Mutex
	tunStatusCachedAt           time.Time
	tunStatusConfig             ConfigMode
	tunStatusActive             bool
	tunStatusText               string
	quitMu                      sync.Mutex
	quiting                     bool
	allowClose                  bool
	preserveProxySettingsOnExit bool
	shutdownOnce                sync.Once
	autoStartOnLaunch           bool
	tunHandoffOnLaunch          bool
	trafficMu                   sync.Mutex
	traffic                     trafficTrack
	trafficSnapshot             TrafficSnapshot
	logEventMu                  sync.Mutex
	pendingLogEvents            []LogMessage
	iperfMu                     sync.Mutex
	iperfCancel                 context.CancelFunc
	localSpeedtestMu            sync.Mutex
	localSpeedtestCancel        context.CancelFunc
	mtrMu                       sync.Mutex
	mtrCancel                   context.CancelFunc
	appUpdater                  *AppUpdateManager
}

type proxyEntrySnapshot struct {
	SystemProxy bool
	Tun         bool
	ProxyMap    bool
	AppProxy    bool
}

type trafficTrack struct {
	NodeID        string
	GroupID       string
	UploadTotal   int64
	DownloadTotal int64
}

func NewApp() *App {
	root := detectAppRoot()
	logger := NewLogger(filepath.Join(root, "logs", "gui", "app.log"))
	logger.RotateIfLarge(2 * 1024 * 1024)
	if err := ensureEmbeddedBins(root, logger); err != nil {
		logger.Error("释放运行内核失败：%v", err)
	}
	statePath := filepath.Join(root, "data", "gui", "state.dat")
	legacyStatePath := filepath.Join(root, "data", "state.dat")
	if _, err := os.Stat(statePath); errors.Is(err, os.ErrNotExist) {
		if data, readErr := os.ReadFile(legacyStatePath); readErr == nil {
			_ = os.MkdirAll(filepath.Dir(statePath), 0755)
			if writeErr := os.WriteFile(statePath, data, 0644); writeErr == nil {
				_ = os.Remove(legacyStatePath)
			}
		}
	}
	store, err := NewStore(statePath)
	if err != nil {
		logger.Error("读取配置失败：%v", err)
		if backupErr := backupUnreadableState(statePath); backupErr != nil {
			logger.Error("备份异常配置失败：%v", backupErr)
		} else {
			logger.Info("已备份异常配置并重建默认配置")
		}
		store, err = NewStore(statePath)
		if err != nil {
			store = &Store{
				path:          statePath,
				selectionPath: filepath.Join(filepath.Dir(statePath), "selected.dat"),
				state:         PersistState{Settings: defaultSettings(), Groups: defaultGroups()},
			}
		}
	}
	manager := NewSingBoxManager(
		root,
		filepath.Join(root, "bin", exeName("sing-box")),
		filepath.Join(root, "data", "gui", "sing-box.json"),
		filepath.Join(root, "data", "gui", "cache.db"),
		logger,
	)
	manager.SetSOCKS5TestProxyPort(store.Snapshot().Settings.MixedPort)
	app := &App{
		root:               root,
		store:              store,
		manager:            manager,
		logger:             logger,
		autoStartOnLaunch:  hasRestartArg(),
		tunHandoffOnLaunch: hasArg(adminTunHandoffArg),
		appUpdater:         newAppUpdateManager(logger),
	}
	return app
}

func backupUnreadableState(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	backup := path + ".bad-" + time.Now().Format("20060102150405")
	return os.Rename(path, backup)
}

func (a *App) startup(ctx context.Context) {
	defer recoverPanic(a.logger, "启动初始化")
	a.ctx = ctx
	a.logger.SetListener(a.queueLogEvent)
	a.startLogEventWatcher()
	a.fitWindowToScreen()
	a.cleanupDelayTempFiles()
	if a.store != nil {
		state := a.store.Snapshot()
		if state.Settings.AutoStart {
			state.Settings.AutoStart = false
			if err := a.store.UpdateSettings(state.Settings); err != nil {
				a.logger.Error("%v", err)
			}
		}
		if err := syncAutoStart(false); err != nil {
			a.logger.Error("%v", err)
		}
	}
	a.startTray()
	if err := a.startProxyWatchdog(); err != nil {
		a.logger.Error("启动异常退出清理 watchdog 失败：%v", err)
	} else {
		a.logger.Info("异常退出清理 watchdog 已启动")
	}
	a.startQuitSignalWatcher()
	a.startShutdownSignalWatcher()
	a.startTrafficWatcher()
	a.startProxyHealthWatcher()
	safeGo(a.logger, "启动自检关闭代理入口", a.restoreStartupState)
	safeGo(a.logger, "启动检查软件更新", a.startupCheckAppUpdate)
	safeGo(a.logger, "清理历史更新文件", func() { cleanupOldUpdateArtifacts(a.root) })
}

func (a *App) fitWindowToScreen() {
	if a.ctx == nil {
		return
	}
	screens, err := wailsRuntime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		wailsRuntime.WindowCenter(a.ctx)
		return
	}
	screen := screens[0]
	for _, item := range screens {
		if item.IsCurrent || item.IsPrimary {
			screen = item
			break
		}
	}
	screenWidth, screenHeight := screen.Size.Width, screen.Size.Height
	if screenWidth <= 0 || screenHeight <= 0 {
		screenWidth, screenHeight = screen.Width, screen.Height
	}
	if screenWidth <= 0 || screenHeight <= 0 {
		wailsRuntime.WindowCenter(a.ctx)
		return
	}
	width := defaultWindowWidth
	height := defaultWindowHeight
	if maxWidth := screenWidth - 80; maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	if maxHeight := screenHeight - 80; maxHeight > 0 && height > maxHeight {
		height = maxHeight
	}
	if width < minWindowWidth {
		width = minWindowWidth
	}
	if height < minWindowHeight {
		height = minWindowHeight
	}
	wailsRuntime.WindowSetSize(a.ctx, width, height)
	wailsRuntime.WindowCenter(a.ctx)
}

func (a *App) cleanupDelayTempFiles() {
	if strings.TrimSpace(a.root) == "" {
		return
	}
	files, err := filepath.Glob(filepath.Join(a.root, "data", "delay-*.json"))
	if err != nil {
		return
	}
	for _, file := range files {
		if err := os.Remove(file); err == nil && a.logger != nil {
			a.logger.Info("已清理残留延迟测试配置：%s", filepath.Base(file))
		}
	}
}

func (a *App) restoreStartupState() {
	defer a.markStartupReady()
	if a.store == nil || a.manager == nil {
		return
	}
	state := a.store.Snapshot()
	settings := state.Settings
	normalizeSettings(&settings)
	wasEnabled := shouldRunProxy(settings)
	cleanupTun := shouldCleanupTunBeforeStartupRestore(a.autoStartOnLaunch, wasEnabled, settings, a.tunHandoffOnLaunch)
	a.cleanupStartupResiduals(settings, true, cleanupTun)
	if a.autoStartOnLaunch && wasEnabled {
		a.proxyOpMu.Lock()
		err := a.startLocked()
		a.proxyOpMu.Unlock()
		if err != nil {
			a.logger.Error("管理员重启后自动开启代理失败：%v", err)
			a.cleanupStartupResiduals(settings, true, true)
			if disableErr := a.disableStoredProxyEntrances("启动自检"); disableErr != nil {
				a.logger.Error("启动失败后关闭代理入口状态失败：%v", disableErr)
			}
			a.emitState()
			return
		}
		a.logger.Info("管理员重启后已自动开启代理入口")
		a.emitState()
		return
	}
	if err := a.disableStoredProxyEntrances("启动自检"); err != nil {
		a.logger.Error("启动自检关闭代理入口失败：%v", err)
	} else if wasEnabled {
		a.logger.Info("启动自检：已关闭上次残留的代理入口，启动后默认未开启")
	} else {
		a.logger.Info("启动自检：代理入口保持关闭")
	}
	a.emitState()
}

func (a *App) markStartupReady() {
	a.healthMu.Lock()
	a.startupReady = true
	a.healthMu.Unlock()
}

func shouldCleanupTunBeforeStartupRestore(autoStartOnLaunch bool, wasEnabled bool, settings Settings, tunHandoff bool) bool {
	if autoStartOnLaunch && wasEnabled && effectiveTun(settings) {
		return tunHandoff
	}
	return true
}

func (a *App) cleanupStartupResiduals(settings Settings, force bool, cleanupTun bool) {
	if a.manager != nil {
		if err := a.manager.CleanupStale(); err != nil {
			a.logger.Error("启动自检清理残留内核失败：%v", err)
		}
	}
	if runtime.GOOS != "windows" {
		return
	}
	if force || !settings.SystemProxy {
		ok, err := systemProxyEnabledForPort(settings.MixedPort)
		if err != nil {
			a.logger.Error("启动自检读取系统代理失败：%v", err)
		} else if ok {
			if err := DisableSystemProxy(); err != nil {
				a.logger.Error("启动自检关闭残留系统代理失败：%v", err)
			} else {
				a.logger.Info("启动自检已关闭残留系统代理：127.0.0.1:%d", settings.MixedPort)
			}
		}
	}
	if cleanupTun && (force || !effectiveTun(settings)) {
		cleanupTunInterface(a.logger)
	}
}

func (a *App) shutdown(ctx context.Context) {
	defer recoverPanic(a.logger, "关闭应用")
	a.cleanupAllProxyEffects("关闭应用")
	a.stopTray()
	if a.logger != nil {
		a.logger.Close()
	}
}

func (a *App) cleanupAllProxyEffects(reason string) {
	a.shutdownOnce.Do(func() {
		preserveSettings := a.shouldPreserveProxySettingsOnExit()
		if strings.TrimSpace(reason) != "" && a.logger != nil {
			a.logger.Info("%s：正在取消代理行为", reason)
		}
		a.stopResumeHealthChecks()
		a.stopRunningToolsForExit()
		a.proxyOpMu.Lock()
		defer a.proxyOpMu.Unlock()
		a.persistTraffic()
		if a.manager != nil {
			if a.manager.Running() {
				if err := a.manager.CloseConnections(); err != nil && a.logger != nil {
					a.logger.Error("%s：关闭现有代理连接失败，继续终止内核：%v", reason, err)
				}
			}
			if err := a.manager.Stop(); err != nil && a.logger != nil {
				a.logger.Error("%s：停止 sing-box 失败：%v", reason, err)
			}
			if err := a.manager.CleanupStale(); err != nil && a.logger != nil {
				a.logger.Error("%s：强制清理残留 sing-box 失败：%v", reason, err)
			}
		}
		if preserveSettings {
			if a.logger != nil {
				a.logger.Info("管理员 TUN 交接：旧实例已停止，保留待启动的 TUN 设置")
			}
		} else {
			a.cleanupExitResiduals()
			if err := a.disableStoredProxyEntrances(reason); err != nil && a.logger != nil {
				a.logger.Error("%s：保存代理关闭状态失败：%v", reason, err)
			}
		}
		a.resetTrafficTracking()
	})
}

func (a *App) disableStoredProxyEntrances(reason string) error {
	if a.store == nil {
		return nil
	}
	state := a.store.Snapshot()
	settings := state.Settings
	normalizeSettings(&settings)
	changed := settings.SystemProxy ||
		settings.Tun ||
		settings.ProxyMap ||
		settings.AppProxy
	settings.SystemProxy = false
	settings.Tun = false
	settings.ProxyMap = false
	settings.AppProxy = false
	if !changed {
		return nil
	}
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	if strings.TrimSpace(reason) != "" && a.logger != nil {
		a.logger.Info("%s：已保存代理入口为关闭状态", reason)
	}
	return nil
}

func (a *App) cleanupExitResiduals() {
	a.cleanupProxyResiduals(true)
}

func (a *App) cleanupProxyResiduals(cleanupTun bool) {
	if runtime.GOOS != "windows" {
		return
	}
	settings := defaultSettings()
	if a.store != nil {
		settings = a.store.Snapshot().Settings
		normalizeSettings(&settings)
	}
	ok, err := systemProxyEnabledForPort(settings.MixedPort)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("退出清理读取系统代理失败：%v", err)
		}
	} else if ok {
		if err := DisableSystemProxy(); err != nil {
			if a.logger != nil {
				a.logger.Error("退出清理关闭系统代理失败：%v", err)
			}
		} else if a.logger != nil {
			a.logger.Info("退出清理已关闭系统代理：127.0.0.1:%d", settings.MixedPort)
		}
	}
	if cleanupTun {
		cleanupTunInterface(a.logger)
	}
}

func (a *App) stopRunningToolsForExit() {
	_ = a.StopIperf3()
	_ = a.StopLocalSpeedtest()
	_ = a.StopMTR()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.iperfMu.Lock()
		iperfRunning := a.iperfCancel != nil
		a.iperfMu.Unlock()
		a.localSpeedtestMu.Lock()
		localSpeedtestRunning := a.localSpeedtestCancel != nil
		a.localSpeedtestMu.Unlock()
		a.mtrMu.Lock()
		mtrRunning := a.mtrCancel != nil
		a.mtrMu.Unlock()
		if !iperfRunning && !localSpeedtestRunning && !mtrRunning {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if a.logger != nil {
		a.logger.Error("退出清理等待测速工具结束超时，继续退出")
	}
}

func (a *App) cleanupForProcessExit(reason string) {
	defer recoverPanic(a.logger, reason)
	a.markClosingAllowed()
	a.cleanupAllProxyEffects(reason)
}

func (a *App) requestQuit(reason string) {
	if a.beginAsyncQuit(reason) {
		return
	}
	if a.isCloseAllowed() && a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
		return
	}
}

func (a *App) shutdownForSignal(reason string) {
	a.requestQuit(reason)
}

func (a *App) beginAsyncQuit(reason string) bool {
	a.quitMu.Lock()
	if a.quiting || a.allowClose {
		a.quitMu.Unlock()
		return false
	}
	a.quiting = true
	a.quitMu.Unlock()

	a.hideWindow()
	a.stopTray()
	time.AfterFunc(forcedQuitTimeout, func() {
		os.Exit(0)
	})
	safeGo(a.logger, reason, func() {
		a.cleanupAllProxyEffects(reason)
		a.markClosingAllowed()
		if a.logger != nil {
			a.logger.Close()
		}
		os.Exit(0)
	})
	return true
}

func (a *App) markClosingAllowed() {
	a.quitMu.Lock()
	a.quiting = true
	a.allowClose = true
	a.quitMu.Unlock()
}

func (a *App) markAdminTunHandoffExit() {
	a.quitMu.Lock()
	a.preserveProxySettingsOnExit = true
	a.quitMu.Unlock()
}

func (a *App) shouldPreserveProxySettingsOnExit() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	return a.preserveProxySettingsOnExit
}

func (a *App) isCloseAllowed() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	return a.allowClose
}

func (a *App) isQuitting() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	return a.quiting
}

func (a *App) stopProxyForUser() error {
	cleanupTun := false
	if a.store != nil {
		settings := a.store.Snapshot().Settings
		a.rememberProxyEntries(settings)
		cleanupTun = effectiveTun(settings)
	}
	a.persistTraffic()
	var result error
	if a.manager != nil {
		if err := a.manager.Stop(); err != nil {
			result = errors.Join(result, err)
			if cleanupErr := a.manager.CleanupStale(); cleanupErr != nil {
				result = errors.Join(result, fmt.Errorf("强制清理残留内核失败：%w", cleanupErr))
			}
		}
	}
	if err := a.disableStoredProxyEntrances("停止代理"); err != nil {
		result = errors.Join(result, fmt.Errorf("保存代理关闭状态失败：%w", err))
	}
	a.cleanupProxyResiduals(cleanupTun)
	a.resetTrafficTracking()
	return result
}

func (a *App) rememberProxyEntries(settings Settings) {
	normalizeSettings(&settings)
	if !shouldRunProxy(settings) {
		return
	}
	a.proxyResumeMu.Lock()
	a.proxyResume = proxyEntrySnapshot{
		SystemProxy: settings.SystemProxy,
		Tun:         settings.Tun,
		ProxyMap:    settings.ProxyMap,
		AppProxy:    settings.AppProxy,
	}
	a.proxyResumeValid = true
	a.proxyResumeMu.Unlock()
}

func (a *App) proxySettingsForStart(settings Settings) (Settings, bool) {
	normalizeSettings(&settings)
	if shouldRunProxy(settings) {
		return settings, false
	}

	a.proxyResumeMu.Lock()
	snapshot := a.proxyResume
	valid := a.proxyResumeValid
	a.proxyResumeMu.Unlock()
	if valid {
		settings.SystemProxy = snapshot.SystemProxy
		settings.Tun = snapshot.Tun
		settings.ProxyMap = snapshot.ProxyMap
		settings.AppProxy = snapshot.AppProxy
		normalizeSettings(&settings)
		if shouldRunProxy(settings) {
			return settings, true
		}
	}

	settings.SystemProxy = true
	settings.Tun = false
	normalizeSettings(&settings)
	return settings, false
}

func (a *App) beforeClose(ctx context.Context) (hide bool) {
	defer func() {
		if value := recover(); value != nil {
			logRecoveredPanic(a.logger, "关闭窗口", value)
			hide = true
		}
	}()
	if a.isCloseAllowed() {
		return false
	}
	a.hideWindow()
	return true
}

func (a *App) State() (result RuntimeState) {
	defer a.recoverState("读取运行状态", &result)
	state := PersistState{Settings: defaultSettings(), Groups: defaultGroups()}
	if a.store != nil {
		state = a.store.Snapshot()
	}
	nodes, groups, traffic := a.withTraffic(state.Nodes, state.Groups, state.SelectedID)
	nodes = nodesForState(nodes)
	configMode, tunActive, tunStatus := a.runtimeStateConfigInfo(state)
	latestCoreVersion, coreUpdateAvailable := a.coreUpdateStatus()
	var appLatestVersion string
	var appUpdateAvailable bool
	var appReleaseNotes string
	if a.appUpdater != nil && a.appUpdater.lastInfo != nil {
		appLatestVersion = a.appUpdater.lastInfo.LatestVersion
		appUpdateAvailable = a.appUpdater.lastInfo.Available
		appReleaseNotes = a.appUpdater.lastInfo.ReleaseNotes
	}
	return RuntimeState{
		Running:             a.manager != nil && a.manager.Running(),
		AppVersion:          AppVersion,
		AppLatestVersion:    appLatestVersion,
		AppUpdateAvailable:  appUpdateAvailable,
		AppReleaseNotes:     appReleaseNotes,
		CoreVersion:         a.coreVersion(),
		CoreLatestVersion:   latestCoreVersion,
		CoreUpdateAvailable: coreUpdateAvailable,
		ListenURL:           "127.0.0.1",
		SelectedID:          state.SelectedID,
		UploadRate:          traffic.UploadRate,
		DownloadRate:        traffic.DownloadRate,
		Settings:            state.Settings,
		RulePresets:         RulePresets(),
		Nodes:               nodes,
		Groups:              groups,
		Logs:                a.stateLogsPreview(),
		ConfigMode:          configMode,
		TunActive:            tunActive,
		TunStatus:            tunStatus,
		CircuitBreakerActive: a.isCircuitBreakerActive(),
		CircuitBreakerReason: a.circuitBreakerReason(),
	}
}

func (a *App) runtimeStateConfigInfo(state PersistState) (ConfigMode, bool, string) {
	settings := state.Settings
	normalizeSettings(&settings)
	configMode := configModeSummary(state)
	if !effectiveTun(settings) {
		return configMode, false, "TUN未开启"
	}
	if cachedMode, cachedActive, cachedStatus, ok := a.cachedTunStatus(); ok {
		return cachedMode, cachedActive, cachedStatus
	}
	if a == nil || a.manager == nil || !a.manager.Running() {
		return a.cacheTunStatus(configMode, false, "TUN未生效：内核未运行")
	}
	config, err := a.manager.readRuntimeConfig()
	if err != nil {
		return a.cacheTunStatus(configMode, false, "TUN未生效：无法读取运行配置")
	}
	configMode = configModeFromConfigDetails(config, settings.Mode, settings.ExternalProxyEndpoints, appProxyActive(settings))
	if !hasTunInbound(config) {
		return a.cacheTunStatus(configMode, false, "TUN未生效：运行配置缺少TUN")
	}
	return a.cacheTunStatus(configMode, true, "TUN已生效")
}

func (a *App) cachedTunStatus() (ConfigMode, bool, string, bool) {
	if a == nil {
		return ConfigMode{}, false, "", false
	}
	a.tunStatusMu.Lock()
	defer a.tunStatusMu.Unlock()
	if a.tunStatusCachedAt.IsZero() || time.Since(a.tunStatusCachedAt) > 3*time.Second {
		return ConfigMode{}, false, "", false
	}
	return a.tunStatusConfig, a.tunStatusActive, a.tunStatusText, true
}

func (a *App) cacheTunStatus(configMode ConfigMode, active bool, status string) (ConfigMode, bool, string) {
	if a == nil {
		return configMode, active, status
	}
	a.tunStatusMu.Lock()
	a.tunStatusCachedAt = time.Now()
	a.tunStatusConfig = configMode
	a.tunStatusActive = active
	a.tunStatusText = status
	a.tunStatusMu.Unlock()
	return configMode, active, status
}

func (a *App) runtimeTunStatus(settings Settings) (bool, string) {
	normalizeSettings(&settings)
	if !effectiveTun(settings) {
		return false, "TUN未开启"
	}
	if a == nil || a.manager == nil || !a.manager.Running() {
		return false, "TUN未生效：内核未运行"
	}
	config, err := a.manager.readRuntimeConfig()
	if err != nil {
		return false, "TUN未生效：无法读取运行配置"
	}
	if !hasTunInbound(config) {
		return false, "TUN未生效：运行配置缺少TUN"
	}
	return true, "TUN已生效"
}

func (a *App) withTraffic(nodes []Node, groups []NodeGroup, selectedID string) ([]Node, []NodeGroup, TrafficSnapshot) {
	resultNodes := append([]Node{}, nodes...)
	resultGroups := append([]NodeGroup{}, groups...)
	traffic := a.currentTrafficSnapshot()
	if selectedID == "" {
		return resultNodes, resultGroups, traffic
	}
	selectedGroupID := ""
	for i := range resultNodes {
		if resultNodes[i].ID != selectedID {
			continue
		}
		resultNodes[i].UploadRate = traffic.UploadRate
		resultNodes[i].DownloadRate = traffic.DownloadRate
		selectedGroupID = resultNodes[i].GroupID
		if selectedGroupID == "" {
			selectedGroupID = defaultGroupID
		}
		break
	}
	for i := range resultGroups {
		if resultGroups[i].ID != selectedGroupID {
			continue
		}
		resultGroups[i].UploadRate = traffic.UploadRate
		resultGroups[i].DownloadRate = traffic.DownloadRate
		break
	}
	return resultNodes, resultGroups, traffic
}

func nodesForState(nodes []Node) []Node {
	result := append([]Node{}, nodes...)
	for i := range result {
		result[i].Raw = ""
		result[i].Outbound = nil
	}
	return result
}

func (a *App) recordTrafficDelta(selectedID string, selectedGroupID string, traffic TrafficSnapshot) (int64, int64) {
	if selectedGroupID == "" {
		selectedGroupID = defaultGroupID
	}
	a.trafficMu.Lock()
	defer a.trafficMu.Unlock()
	a.trafficSnapshot = traffic
	if a.traffic.NodeID != selectedID || a.traffic.GroupID != selectedGroupID ||
		traffic.UploadTotal < a.traffic.UploadTotal || traffic.DownloadTotal < a.traffic.DownloadTotal {
		a.traffic = trafficTrack{
			NodeID:        selectedID,
			GroupID:       selectedGroupID,
			UploadTotal:   traffic.UploadTotal,
			DownloadTotal: traffic.DownloadTotal,
		}
		return 0, 0
	}
	uploadDelta := traffic.UploadTotal - a.traffic.UploadTotal
	downloadDelta := traffic.DownloadTotal - a.traffic.DownloadTotal
	if uploadDelta <= 0 && downloadDelta <= 0 {
		return 0, 0
	}
	if a.store != nil {
		if err := a.store.AddTraffic(selectedID, selectedGroupID, uploadDelta, downloadDelta); err != nil {
			a.logger.Error("保存流量统计失败：%v", err)
		}
	}
	a.traffic.UploadTotal = traffic.UploadTotal
	a.traffic.DownloadTotal = traffic.DownloadTotal
	return uploadDelta, downloadDelta
}

func (a *App) persistTraffic() {
	if a.store == nil {
		return
	}
	if a.manager == nil || !a.manager.Running() {
		_ = a.store.FlushTraffic()
		return
	}
	selectedID, selectedGroupID := a.store.TrafficTarget()
	if selectedID == "" {
		_ = a.store.FlushTraffic()
		return
	}
	traffic, err := a.manager.TrafficSnapshot()
	if err == nil {
		a.recordTrafficDelta(selectedID, selectedGroupID, traffic)
	}
	if err := a.store.FlushTraffic(); err != nil && a.logger != nil {
		a.logger.Error("保存流量统计失败：%v", err)
	}
}

func (a *App) startTrafficWatcher() {
	if a == nil || a.store == nil || a.manager == nil {
		return
	}
	ctx := a.ctx
	safeGo(a.logger, "流量统计", func() {
		sampleTicker := time.NewTicker(trafficSampleInterval)
		persistTicker := time.NewTicker(trafficPersistInterval)
		defer sampleTicker.Stop()
		defer persistTicker.Stop()
		for {
			select {
			case <-sampleTicker.C:
				a.sampleTraffic()
			case <-persistTicker.C:
				if err := a.store.FlushTraffic(); err != nil && a.logger != nil {
					a.logger.Error("定时保存流量统计失败：%v", err)
				}
			case <-ctx.Done():
				_ = a.store.FlushTraffic()
				return
			}
		}
	})
}

func (a *App) sampleTraffic() {
	if a.manager == nil || !a.manager.Running() || a.store == nil {
		a.publishTrafficSnapshot(TrafficSnapshot{})
		return
	}
	selectedID, selectedGroupID := a.store.TrafficTarget()
	if selectedID == "" {
		a.publishTrafficSnapshot(TrafficSnapshot{})
		return
	}
	traffic, err := a.manager.TrafficSnapshot()
	if err != nil {
		return
	}
	a.recordTrafficDelta(selectedID, selectedGroupID, traffic)
	a.emitTrafficSnapshot(traffic)
}

func (a *App) currentTrafficSnapshot() TrafficSnapshot {
	a.trafficMu.Lock()
	defer a.trafficMu.Unlock()
	return a.trafficSnapshot
}

func (a *App) setTrafficRates(traffic TrafficSnapshot) {
	a.trafficMu.Lock()
	a.trafficSnapshot = traffic
	a.trafficMu.Unlock()
}

func (a *App) publishTrafficSnapshot(traffic TrafficSnapshot) {
	a.setTrafficRates(traffic)
	a.emitTrafficSnapshot(traffic)
}

func (a *App) emitTrafficSnapshot(traffic TrafficSnapshot) {
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "traffic-snapshot", traffic)
	}
}

func (a *App) queueLogEvent(message LogMessage) {
	if a == nil || a.ctx == nil {
		return
	}
	a.logEventMu.Lock()
	a.pendingLogEvents = append(a.pendingLogEvents, message)
	if len(a.pendingLogEvents) > 200 {
		copy(a.pendingLogEvents, a.pendingLogEvents[len(a.pendingLogEvents)-200:])
		a.pendingLogEvents = a.pendingLogEvents[:200]
	}
	a.logEventMu.Unlock()
}

func (a *App) startLogEventWatcher() {
	ctx := a.ctx
	safeGo(a.logger, "日志界面推送", func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.flushLogEvents()
			case <-ctx.Done():
				return
			}
		}
	})
}

func (a *App) flushLogEvents() {
	a.logEventMu.Lock()
	if len(a.pendingLogEvents) == 0 {
		a.logEventMu.Unlock()
		return
	}
	messages := append([]LogMessage(nil), a.pendingLogEvents...)
	clear(a.pendingLogEvents)
	a.pendingLogEvents = a.pendingLogEvents[:0]
	a.logEventMu.Unlock()
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "logs-added", messages)
	}
}

func (a *App) resetTrafficTracking() {
	a.trafficMu.Lock()
	a.traffic = trafficTrack{}
	a.trafficSnapshot = TrafficSnapshot{}
	a.trafficMu.Unlock()
}

func (a *App) CreateGroup(name string) (group NodeGroup, err error) {
	defer a.recoverError("创建节点组", &err)
	if a.store == nil {
		return NodeGroup{}, errors.New("配置存储未初始化")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return NodeGroup{}, errors.New("组名不能为空")
	}
	state := a.store.Snapshot()
	group = NodeGroup{ID: groupIDFromName("manual|" + name), Name: name, Source: "manual"}
	for _, item := range state.Groups {
		if item.ID == group.ID || item.Name == name {
			return NodeGroup{}, errors.New("节点组已存在")
		}
	}
	groups := append(append([]NodeGroup{}, state.Groups...), group)
	if err := a.store.ReplaceGroupsAndNodes(groups, state.Nodes, state.SelectedID); err != nil {
		return NodeGroup{}, err
	}
	a.logger.Info("已添加节点组：%s", name)
	a.emitState()
	return group, nil
}

func (a *App) RenameGroup(id string, name string) (err error) {
	defer a.recoverError("rename group", &err)
	if a.store == nil {
		return errors.New("store is not ready")
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		return errors.New("group does not exist")
	}
	if name == "" {
		return errors.New("group name is required")
	}
	if id == defaultGroupID {
		return errors.New("default group cannot be renamed")
	}
	if err := a.store.RenameGroup(id, name); err != nil {
		return err
	}
	a.logger.Info("group renamed: %s", name)
	a.emitState()
	return nil
}

func (a *App) ReorderGroups(ids []string) (err error) {
	defer a.recoverError("节点组排序", &err)
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if err := a.store.ReorderGroups(ids); err != nil {
		return err
	}
	a.logger.Info("节点组排序已保存")
	a.emitState()
	return nil
}

func (a *App) ReorderNodes(groupID string, ids []string) (err error) {
	defer a.recoverError("节点排序", &err)
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if err := a.store.ReorderNodes(groupID, ids); err != nil {
		return err
	}
	a.logger.Info("节点排序已保存")
	a.emitState()
	return nil
}

func (a *App) ImportNodes(text string) (result map[string]interface{}, err error) {
	defer a.recoverError("导入节点", &err)
	return a.importNodesToGroup(text, defaultGroupID, false)
}

func (a *App) ImportNodesToGroup(text string, groupID string) (result map[string]interface{}, err error) {
	defer a.recoverError("导入节点到分组", &err)
	return a.importNodesToGroup(text, groupID, false)
}

func (a *App) ImportNodesToGroupWithDuplicates(text string, groupID string, allowDuplicates bool) (result map[string]interface{}, err error) {
	defer a.recoverError("导入重复节点到分组", &err)
	return a.importNodesToGroup(text, groupID, allowDuplicates)
}

func (a *App) importNodesToGroup(text string, groupID string, allowDuplicates bool) (map[string]interface{}, error) {
	if err := validateManualNodeInput(text); err != nil {
		return nil, err
	}
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		groupID = defaultGroupID
	}
	nodes, errs := ParseNodesPreserveDuplicates(text)
	if len(nodes) == 0 {
		return nil, errors.New("没有解析到可用节点")
	}
	state := a.store.Snapshot()
	groupExists := false
	for _, group := range state.Groups {
		if group.ID == groupID {
			groupExists = true
			break
		}
	}
	if !groupExists {
		return nil, errors.New("节点组不存在")
	}
	existingKeys := make(map[string]bool, len(state.Nodes)+len(nodes))
	for _, node := range state.Nodes {
		existingKeys[importNodeKey(node)] = true
	}
	seenKeys := make(map[string]bool, len(existingKeys)+len(nodes))
	for key := range existingKeys {
		seenKeys[key] = true
	}
	duplicateCount := 0
	for _, node := range nodes {
		key := importNodeKey(node)
		if seenKeys[key] {
			duplicateCount++
		}
		seenKeys[key] = true
	}
	if duplicateCount > 0 && !allowDuplicates {
		return map[string]interface{}{
			"added":                0,
			"duplicateCount":       duplicateCount,
			"errors":               errs,
			"requiresConfirmation": true,
			"total":                len(nodes),
		}, nil
	}
	previous := state
	merged := append([]Node{}, state.Nodes...)
	usedIDs := make(map[string]bool, len(merged)+len(nodes))
	for _, node := range merged {
		usedIDs[node.ID] = true
	}
	seenKeys = make(map[string]bool, len(existingKeys)+len(nodes))
	for key := range existingKeys {
		seenKeys[key] = true
	}
	for _, node := range nodes {
		node.GroupID = groupID
		key := importNodeKey(node)
		if seenKeys[key] || usedIDs[node.ID] {
			node.ID = nextImportedNodeID(node, groupID, usedIDs)
		}
		merged = append(merged, node)
		usedIDs[node.ID] = true
		seenKeys[key] = true
	}
	added := len(nodes)
	selected := state.SelectedID
	if err := a.store.ReplaceNodes(merged, selected); err != nil {
		return nil, err
	}
	state = a.store.Snapshot()
	if added > 0 {
		if err := a.syncRuntimeNodeList(previous, state, "导入节点后同步运行配置"); err != nil {
			return nil, err
		}
	}
	a.logger.Info("导入节点 %d 个，其中重复节点 %d 个", added, duplicateCount)
	a.emitState()
	return map[string]interface{}{
		"added":                added,
		"duplicateCount":       duplicateCount,
		"errors":               errs,
		"requiresConfirmation": false,
		"total":                len(nodes),
	}, nil
}

func importNodeKey(node Node) string {
	if raw := strings.TrimSpace(node.Raw); raw != "" {
		return tagFromRaw(raw)
	}
	return strings.TrimSpace(node.ID)
}

func nextImportedNodeID(node Node, groupID string, usedIDs map[string]bool) string {
	for index := 1; ; index++ {
		candidate := tagFromRaw(fmt.Sprintf("duplicate|%s|%s|%d", groupID, node.Raw, index))
		if !usedIDs[candidate] {
			return candidate
		}
	}
}

func (a *App) ImportSubscription(subURL string) (result map[string]interface{}, err error) {
	defer a.recoverError("导入订阅", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	state := a.store.Snapshot()
	previous := state
	proxyPort := 0
	if a.manager != nil && a.manager.Running() {
		proxyPort = state.Settings.MixedPort
	}
	subResult, err := FetchSubscription(ctx, subURL, proxyPort)
	if err != nil {
		return nil, err
	}
	if len(subResult.Nodes) == 0 {
		return nil, errors.New("订阅没有解析到可用节点")
	}

	groups := subResult.Groups
	if len(groups) == 0 {
		group := NodeGroup{
			ID:     groupIDFromName(subURL + "|" + subResult.GroupName),
			Name:   subResult.GroupName,
			Source: "subscription",
			URL:    strings.TrimSpace(subURL),
		}
		groups = []NodeGroup{group}
		for i := range subResult.Nodes {
			subResult.Nodes[i].GroupID = group.ID
		}
	} else {
		fallback := NodeGroup{
			ID:     groupIDFromName(subURL + "|" + subResult.GroupName),
			Name:   subResult.GroupName,
			Source: "subscription",
			URL:    strings.TrimSpace(subURL),
		}
		hasFallback := false
		for _, group := range groups {
			if group.ID == fallback.ID {
				hasFallback = true
				break
			}
		}
		for i := range subResult.Nodes {
			if subResult.Nodes[i].GroupID == "" {
				subResult.Nodes[i].GroupID = fallback.ID
				if !hasFallback {
					groups = append(groups, fallback)
					hasFallback = true
				}
			}
		}
	}

	subURL = strings.TrimSpace(subURL)
	removeGroups := map[string]bool{}
	mergedGroups := make([]NodeGroup, 0, len(state.Groups)+len(groups))
	for _, group := range state.Groups {
		if group.Source == "subscription" && group.URL == subURL {
			removeGroups[group.ID] = true
			continue
		}
		mergedGroups = append(mergedGroups, group)
	}
	for _, group := range groups {
		mergedGroups = append(mergedGroups, group)
	}

	mergedNodes := make([]Node, 0, len(state.Nodes)+len(subResult.Nodes))
	exists := map[string]bool{}
	for _, node := range state.Nodes {
		if removeGroups[node.GroupID] {
			continue
		}
		mergedNodes = append(mergedNodes, node)
		exists[node.ID] = true
	}
	added := 0
	for _, node := range subResult.Nodes {
		if oldNode, ok := findNodeByID(state.Nodes, node.ID); ok {
			node.DelayMS = oldNode.DelayMS
			node.LastTest = oldNode.LastTest
			node.UploadTotal = oldNode.UploadTotal
			node.DownloadTotal = oldNode.DownloadTotal
		}
		if !exists[node.ID] {
			mergedNodes = append(mergedNodes, node)
			exists[node.ID] = true
			added++
		}
	}
	selected := state.SelectedID
	if err := a.store.ReplaceGroupsAndNodes(mergedGroups, mergedNodes, selected); err != nil {
		return nil, err
	}
	state = a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "导入订阅后同步运行配置"); err != nil {
		return nil, err
	}
	a.logger.Info("导入订阅 %s：新增 %d 个节点，抓取方式=%s", subResult.GroupName, added, subResult.Mode)
	a.emitState()
	return map[string]interface{}{
		"added":     added,
		"groupName": subResult.GroupName,
		"mode":      subResult.Mode,
		"errors":    subResult.Errors,
	}, nil
}

func (a *App) UpdateSubscriptionGroup(groupID string) (result map[string]interface{}, err error) {
	defer a.recoverError("UpdateSubscriptionGroup", &err)
	return a.updateSubscriptionGroup(groupID, "")
}

func (a *App) UpdateSubscriptionGroupWithURL(groupID string, subURL string) (result map[string]interface{}, err error) {
	defer a.recoverError("UpdateSubscriptionGroupWithURL", &err)
	return a.updateSubscriptionGroup(groupID, subURL)
}

func (a *App) updateSubscriptionGroup(groupID string, nextURL string) (map[string]interface{}, error) {
	if a.store == nil {
		return nil, errors.New("store is not initialized")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group id is required")
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	state := a.store.Snapshot()
	previous := state

	var group NodeGroup
	found := false
	for _, item := range state.Groups {
		if item.ID == groupID {
			group = item
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("group does not exist")
	}
	subURL := strings.TrimSpace(group.URL)
	if trimmed := strings.TrimSpace(nextURL); trimmed != "" {
		subURL = trimmed
		group.URL = trimmed
	}
	if group.Source != "subscription" || subURL == "" {
		return nil, errors.New("group is not imported from subscription")
	}

	proxyPort := 0
	if a.manager != nil && a.manager.Running() {
		proxyPort = state.Settings.MixedPort
	}
	subResult, err := FetchSubscription(ctx, subURL, proxyPort)
	if err != nil {
		return nil, err
	}

	newNodes := nodesForSubscriptionGroup(subResult, group)
	if len(newNodes) == 0 {
		return nil, errors.New("subscription group has no available nodes")
	}
	oldNodeByID := map[string]Node{}
	for _, node := range state.Nodes {
		if node.GroupID == group.ID {
			oldNodeByID[node.ID] = node
		}
	}

	mergedNodes := make([]Node, 0, len(state.Nodes)+len(newNodes))
	exists := map[string]bool{}
	for _, node := range state.Nodes {
		if node.GroupID == group.ID {
			continue
		}
		mergedNodes = append(mergedNodes, node)
		exists[node.ID] = true
	}
	for _, node := range newNodes {
		node.GroupID = group.ID
		if oldNode, ok := oldNodeByID[node.ID]; ok {
			node.DelayMS = oldNode.DelayMS
			node.LastTest = oldNode.LastTest
			node.UploadTotal = oldNode.UploadTotal
			node.DownloadTotal = oldNode.DownloadTotal
		}
		if exists[node.ID] {
			node.ID = tagFromRaw(fmt.Sprintf("%s|%s|%s", group.ID, node.Name, node.Raw))
		}
		if exists[node.ID] {
			continue
		}
		exists[node.ID] = true
		mergedNodes = append(mergedNodes, node)
	}

	selected := state.SelectedID
	if selected != "" && oldNodeByID[selected].ID != "" {
		selected = ""
		for _, node := range mergedNodes {
			if node.GroupID == group.ID {
				selected = node.ID
				break
			}
		}
	}
	mergedGroups := append([]NodeGroup{}, state.Groups...)
	for i := range mergedGroups {
		if mergedGroups[i].ID == group.ID {
			mergedGroups[i].URL = group.URL
			mergedGroups[i].Source = "subscription"
			break
		}
	}
	if err := a.store.ReplaceGroupsAndNodes(mergedGroups, mergedNodes, selected); err != nil {
		return nil, err
	}
	state = a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "Update subscription group"); err != nil {
		return nil, err
	}
	if a.logger != nil {
		a.logger.Info("subscription group updated: %s, nodes=%d, mode=%s", group.Name, len(newNodes), subResult.Mode)
	}
	a.emitState()
	return map[string]interface{}{
		"updated":   len(newNodes),
		"groupName": group.Name,
		"mode":      subResult.Mode,
		"errors":    subResult.Errors,
	}, nil
}

func nodesForSubscriptionGroup(result SubscriptionResult, group NodeGroup) []Node {
	if len(result.Groups) == 0 {
		nodes := append([]Node{}, result.Nodes...)
		for i := range nodes {
			nodes[i].GroupID = group.ID
		}
		return nodes
	}
	matchIDs := map[string]bool{group.ID: true}
	for _, item := range result.Groups {
		if item.Name == group.Name {
			matchIDs[item.ID] = true
		}
	}
	nodes := make([]Node, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		if matchIDs[node.GroupID] {
			node.GroupID = group.ID
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func (a *App) DeleteGroup(id string) (err error) {
	defer a.recoverError("删除节点组", &err)
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if id == "" {
		return errors.New("节点组 ID 无效")
	}
	previous := a.store.Snapshot()
	if err := a.store.DeleteGroup(id); err != nil {
		return err
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "删除节点组后同步运行配置"); err != nil {
		return err
	}
	a.logger.Info("已删除节点组：%s", id)
	a.emitState()
	return nil
}

func (a *App) SelectNode(id string) (err error) {
	defer a.recoverError("选择节点", &err)
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("节点 ID 无效")
	}
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	state := a.store.Snapshot()
	if _, ok := findSelectedNode(state.Nodes, id); !ok {
		return errors.New("节点不存在")
	}
	if !shouldRunProxy(state.Settings) {
		if err := a.store.SelectNode(id); err != nil {
			return err
		}
		a.logger.Info("已选择节点（代理未运行）：%s", selectedNodeLogName(state.Nodes, id))
		a.emitState()
		return nil
	}
	if a.manager == nil {
		return errors.New("内核管理器未初始化")
	}
	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return errors.New("代理正在执行其他操作，节点未切换，请稍后重试")
	}
	defer a.proxyOpMu.Unlock()
	if a.isQuitting() {
		return errors.New("软件正在退出，节点未切换")
	}
	return a.selectNodeLocked(id)
}

func (a *App) selectNodeLocked(id string) error {
	state := a.store.Snapshot()
	node, ok := findSelectedNode(state.Nodes, id)
	if !ok {
		return errors.New("节点不存在")
	}
	if !shouldRunProxy(state.Settings) {
		if err := a.store.SelectNode(id); err != nil {
			return err
		}
		a.logger.Info("已选择节点（代理未运行）：%s", selectedNodeLogName(state.Nodes, id))
		a.emitState()
		return nil
	}
	if !a.manager.Running() {
		return errors.New("sing-box 未运行，节点未切换，请先恢复代理后重试")
	}
	selectedTag := nodeOutboundTag(node)
	if selectedTag == "" {
		return errors.New("所选节点缺少出站标识")
	}

	a.persistTraffic()
	if !a.manager.RuntimeHasOutbound(selectedTag) {
		if err := a.reloadRuntimeForSelectedNode(state, id, selectedTag); err != nil {
			a.logger.Error("节点切换失败，继续使用原节点：%v", err)
			return err
		}
	} else if err := switchOutboundThenCommit(
		selectedTag,
		a.manager.CurrentOutbound,
		a.manager.SelectOutboundVerified,
		func() error { return a.store.SelectNode(id) },
	); err != nil {
		a.logger.Error("节点切换失败，继续使用原节点：%v", err)
		return err
	}
	a.resetTrafficTracking()
	state = a.store.Snapshot()
	a.initTrafficBaseline(id, state.Nodes)
	a.setCircuitBreakerActive(false, "")
	a.logger.Info("节点切换已确认：%s", selectedNodeLogName(state.Nodes, id))
	a.emitState()
	return nil
}

func (a *App) reloadRuntimeForSelectedNode(previous PersistState, id string, selectedTag string) error {
	next := previous
	next.SelectedID = id
	a.logger.Info("运行内核尚未加载所选节点，正在同步节点配置：%s", selectedNodeLogName(next.Nodes, id))
	if err := a.manager.Restart(next, effectiveTun(next.Settings)); err != nil {
		restoreErr := a.manager.Restart(previous, effectiveTun(previous.Settings))
		if restoreErr != nil {
			return fmt.Errorf("同步所选节点失败：%v；恢复原节点代理失败：%w", err, restoreErr)
		}
		return fmt.Errorf("同步所选节点失败，已恢复原节点代理：%w", err)
	}
	current, err := a.manager.CurrentOutbound()
	if err != nil || strings.TrimSpace(current) != selectedTag {
		if switchErr := a.manager.SelectOutboundVerified(selectedTag); switchErr != nil {
			restoreErr := a.manager.Restart(previous, effectiveTun(previous.Settings))
			if restoreErr != nil {
				return fmt.Errorf("同步后节点确认失败：%v；恢复原节点代理失败：%w", switchErr, restoreErr)
			}
			return fmt.Errorf("同步后节点确认失败，已恢复原节点代理：%w", switchErr)
		}
	}
	if err := a.store.SelectNode(id); err != nil {
		restoreErr := a.manager.Restart(previous, effectiveTun(previous.Settings))
		if restoreErr != nil {
			return fmt.Errorf("保存所选节点失败：%v；恢复原节点代理失败：%w", err, restoreErr)
		}
		return fmt.Errorf("保存所选节点失败，已恢复原节点代理：%w", err)
	}
	return nil
}

func selectedNodeLogName(nodes []Node, id string) string {
	for _, node := range nodes {
		if node.ID != id {
			continue
		}
		name := strings.TrimSpace(node.Name)
		if name != "" {
			return name
		}
		break
	}
	return id
}

func switchOutboundThenCommit(
	targetTag string,
	currentOutbound func() (string, error),
	selectOutbound func(string) error,
	commit func() error,
) error {
	previousTag, err := currentOutbound()
	if err != nil {
		return fmt.Errorf("读取 sing-box 当前出站节点失败：%w", err)
	}
	previousTag = strings.TrimSpace(previousTag)
	if previousTag != targetTag {
		if err := selectOutbound(targetTag); err != nil {
			return fmt.Errorf("sing-box Clash API 切换失败：%w", err)
		}
	}
	if err := commit(); err != nil {
		if previousTag != "" && previousTag != targetTag {
			if rollbackErr := selectOutbound(previousTag); rollbackErr != nil {
				return fmt.Errorf("保存所选节点失败：%v；回滚实际出站节点失败：%w", err, rollbackErr)
			}
			return fmt.Errorf("保存所选节点失败，实际出站已回滚：%w", err)
		}
		return fmt.Errorf("保存所选节点失败：%w", err)
	}
	return nil
}

func (a *App) DeleteNode(id string) (err error) {
	defer a.recoverError("删除节点", &err)
	if id == "" {
		return errors.New("节点 ID 无效")
	}
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	previous := a.store.Snapshot()
	if err := a.store.DeleteNode(id); err != nil {
		return err
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "删除节点后同步运行配置"); err != nil {
		return err
	}
	a.logger.Info("已删除节点：%s", id)
	a.emitState()
	return nil
}

func (a *App) DeleteNodes(ids []string) (err error) {
	defer a.recoverError("批量删除节点", &err)
	if len(ids) == 0 {
		return errors.New("请选择需要删除的节点")
	}
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	previous := a.store.Snapshot()
	if err := a.store.DeleteNodes(ids); err != nil {
		return err
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "批量删除节点后同步运行配置"); err != nil {
		return err
	}
	a.logger.Info("已批量删除节点：%d 个", len(ids))
	a.emitState()
	return nil
}

func (a *App) MoveNodesToGroup(ids []string, groupID string) (err error) {
	defer a.recoverError("转移节点分组", &err)
	if len(ids) == 0 {
		return errors.New("请选择需要转移的节点")
	}
	if strings.TrimSpace(groupID) == "" {
		return errors.New("请选择目标分组")
	}
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	previous := a.store.Snapshot()
	if err := a.store.MoveNodesToGroup(ids, groupID); err != nil {
		return err
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "转移节点分组后同步运行配置"); err != nil {
		return err
	}
	a.logger.Info("已转移节点到分组：%d 个", len(ids))
	a.emitState()
	return nil
}

func (a *App) NodeDetail(id string) (node Node, err error) {
	defer a.recoverError("读取节点详情", &err)
	if a.store == nil {
		return Node{}, errors.New("配置存储未初始化")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Node{}, errors.New("节点 ID 无效")
	}
	state := a.store.Snapshot()
	for _, item := range state.Nodes {
		if item.ID == id {
			return item, nil
		}
	}
	return Node{}, errors.New("节点不存在")
}

func (a *App) ShareLink(id string) (link string, err error) {
	defer a.recoverError("生成分享链接", &err)
	node, err := a.NodeDetail(id)
	if err != nil {
		return "", err
	}
	return nodeShareLink(node)
}

func (a *App) ShareLinks(ids []string) (links []string, err error) {
	defer a.recoverError("生成批量分享链接", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil, errors.New("请选择节点")
	}
	state := a.store.Snapshot()
	links = make([]string, 0, len(idSet))
	for _, node := range state.Nodes {
		if _, ok := idSet[node.ID]; !ok {
			continue
		}
		link, err := nodeShareLink(node)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", node.Name, err)
		}
		if strings.TrimSpace(link) != "" {
			links = append(links, link)
		}
	}
	return links, nil
}

func (a *App) ShareGroupLinks(groupID string) (links []string, err error) {
	defer a.recoverError("生成分组分享链接", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		groupID = defaultGroupID
	}
	state := a.store.Snapshot()
	links = make([]string, 0)
	for _, node := range state.Nodes {
		if (node.GroupID == "" && groupID != defaultGroupID) || (node.GroupID != "" && node.GroupID != groupID) {
			continue
		}
		link, err := nodeShareLink(node)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", node.Name, err)
		}
		if strings.TrimSpace(link) != "" {
			links = append(links, link)
		}
	}
	return links, nil
}

func (a *App) UpdateNode(id string, raw string) (err error) {
	defer a.recoverError("编辑节点", &err)
	id = strings.TrimSpace(id)
	raw = strings.TrimSpace(raw)
	if id == "" {
		return errors.New("节点 ID 无效")
	}
	if raw == "" {
		return errors.New("节点内容不能为空")
	}
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	next, err := parseEditableNode(raw)
	if err != nil {
		return err
	}
	previous := a.store.Snapshot()
	nodes := append([]Node{}, previous.Nodes...)
	found := false
	selected := previous.SelectedID
	for i := range nodes {
		if nodes[i].ID != id {
			continue
		}
		old := nodes[i]
		next.GroupID = old.GroupID
		next.Country = old.Country
		next.DelayMS = old.DelayMS
		next.LastTest = old.LastTest
		next.UploadRate = old.UploadRate
		next.DownloadRate = old.DownloadRate
		next.UploadTotal = old.UploadTotal
		next.DownloadTotal = old.DownloadTotal
		if next.ID != id {
			for _, node := range nodes {
				if node.ID == next.ID {
					return errors.New("编辑后的节点与已有节点重复")
				}
			}
			if selected == id {
				selected = next.ID
			}
		}
		nodes[i] = next
		found = true
		break
	}
	if !found {
		return errors.New("节点不存在")
	}
	if err := a.store.ReplaceGroupsAndNodes(previous.Groups, nodes, selected); err != nil {
		return err
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "编辑节点后同步运行配置"); err != nil {
		return err
	}
	a.logger.Info("节点已编辑：%s", next.Name)
	a.emitState()
	return nil
}

func parseEditableNode(raw string) (Node, error) {
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return parseNode(raw)
	}
	nodes, errs := ParseNodes(raw)
	if len(nodes) == 0 {
		if len(errs) > 0 {
			return Node{}, errors.New(errs[0])
		}
		return Node{}, errors.New("没有解析到可用节点")
	}
	if len(nodes) > 1 {
		return Node{}, errors.New("一次只能编辑一个节点")
	}
	return nodes[0], nil
}

func (a *App) TestNode(id string) (result map[string]interface{}, err error) {
	return a.TestNodeWithMode(id, "tcp")
}

func (a *App) TestNodeWithMode(id string, mode string) (result map[string]interface{}, err error) {
	defer a.recoverError("测试节点", &err)
	if id == "" {
		return nil, errors.New("节点 ID 无效")
	}
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	state := a.store.Snapshot()
	node, ok := findSelectedNode(state.Nodes, id)
	if !ok || node.ID != id {
		return nil, errors.New("节点不存在")
	}
	metric, err := a.testNodeListDelay(node, mode)
	now := time.Now().Format("15:04:05")
	if err != nil {
		_ = a.store.UpdateNodeDelay(id, -1, now)
		a.logger.Info("节点测速失败：%s，%v", node.Name, err)
		a.emitState()
		return map[string]interface{}{"delayMs": -1, "error": err.Error()}, nil
	}
	delay := metric.DelayMS
	if err := a.store.UpdateNodeDelay(id, delay, now); err != nil {
		return nil, err
	}
	a.logger.Info("节点%s：%s，地址 %s，%d ms", nodeDelayModeLabel(mode), node.Name, metric.PingAddress, delay)
	a.emitState()
	return map[string]interface{}{"delayMs": delay}, nil
}

func (a *App) TestGroupNodes(groupID string) (result map[string]interface{}, err error) {
	return a.TestGroupNodesWithMode(groupID, "tcp")
}

func (a *App) TestGroupNodesWithMode(groupID string, mode string) (result map[string]interface{}, err error) {
	defer a.recoverError("测试节点组", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	if groupID == "" {
		return nil, errors.New("节点组 ID 无效")
	}
	state := a.store.Snapshot()
	nodes := make([]Node, 0)
	for _, node := range state.Nodes {
		if node.GroupID == groupID {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("该组没有节点")
	}
	return a.testNodes(nodes, mode)
}

func (a *App) TestNodes(ids []string) (result map[string]interface{}, err error) {
	return a.TestNodesWithMode(ids, "tcp")
}

func (a *App) TestNodesWithMode(ids []string, mode string) (result map[string]interface{}, err error) {
	defer a.recoverError("批量测试节点", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	if len(ids) == 0 {
		return nil, errors.New("请选择需要测试的节点")
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil, errors.New("请选择需要测试的节点")
	}
	if len(idSet) > maxBatchTestNodes {
		return nil, fmt.Errorf("批量测试一次最多支持 %d 个节点", maxBatchTestNodes)
	}
	state := a.store.Snapshot()
	nodes := make([]Node, 0, len(idSet))
	for _, node := range state.Nodes {
		if _, ok := idSet[node.ID]; ok {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("选中节点不存在")
	}
	return a.testNodes(nodes, mode)
}

func (a *App) testNodes(nodes []Node, mode string) (map[string]interface{}, error) {
	if normalizeNodeDelayMode(mode) == "real" {
		return a.testNodesRealDelay(nodes)
	}
	workerCount := batchNodeTestConcurrency(len(nodes))
	results := a.testNodeListBatch(nodes, mode, workerCount)
	failedNodes := failedNodeListTests(nodes, results)
	// 仅当全部节点均失败时（极可能为本机网络瞬断或刚切换物理网卡），才执行一次快速重试；
	// 若已有节点测试成功，表明网络正常，失败节点为真实失活节点，不再进行二次重试以极大提高批量测速效率。
	if len(failedNodes) > 0 && len(failedNodes) == len(nodes) && workerCount > 1 {
		retryConcurrency := workerCount / 2
		if retryConcurrency < 1 {
			retryConcurrency = 1
		}
		a.logger.Info("批量%s全部失败，怀疑网络瞬断，重试 %d 个节点...", nodeDelayModeLabel(mode), len(failedNodes))
		time.Sleep(120 * time.Millisecond)
		for id, result := range a.testNodeListBatch(failedNodes, mode, retryConcurrency) {
			results[id] = result
		}
	}

	now := time.Now().Format("15:04:05")
	updates := make(map[string]NodeDelayUpdate, len(nodes))
	tested := 0
	failed := 0
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil {
			updates[node.ID] = NodeDelayUpdate{DelayMS: -1, LastTest: now}
			failed++
			continue
		}
		updates[node.ID] = NodeDelayUpdate{DelayMS: item.metric.DelayMS, LastTest: now}
		tested++
	}
	if err := a.store.UpdateNodeDelays(updates); err != nil {
		return nil, err
	}
	a.logger.Info("批量%s完成：成功 %d 个，失败 %d 个", nodeDelayModeLabel(mode), tested, failed)
	a.emitState()
	return map[string]interface{}{"tested": tested, "failed": failed}, nil
}

type nodeListTestResult struct {
	metric SpeedResult
	err    error
}

func (a *App) testNodeListBatch(nodes []Node, mode string, concurrency int) map[string]nodeListTestResult {
	results := make(map[string]nodeListTestResult, len(nodes))
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
				metric, err := a.testNodeListDelay(node, mode)
				mu.Lock()
				results[node.ID] = nodeListTestResult{metric: metric, err: err}
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

func failedNodeListTests(nodes []Node, results map[string]nodeListTestResult) []Node {
	failed := make([]Node, 0)
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil {
			failed = append(failed, node)
		}
	}
	return failed
}

func (a *App) testNodesRealDelay(nodes []Node) (map[string]interface{}, error) {
	results := a.manager.TestNodesDelay(nodes, SpeedTestOptions{DirectOnly: true, FastDelay: true})
	now := time.Now().Format("15:04:05")
	updates := make(map[string]NodeDelayUpdate, len(nodes))
	tested := 0
	failed := 0
	for _, node := range nodes {
		item, ok := results[node.ID]
		if !ok || item.err != nil {
			updates[node.ID] = NodeDelayUpdate{DelayMS: -1, LastTest: now}
			failed++
			continue
		}
		updates[node.ID] = NodeDelayUpdate{DelayMS: item.result.DelayMS, LastTest: now}
		tested++
	}
	if err := a.store.UpdateNodeDelays(updates); err != nil {
		return nil, err
	}
	a.logger.Info("批量真连接延迟完成：成功 %d 个，失败 %d 个", tested, failed)
	a.emitState()
	return map[string]interface{}{"tested": tested, "failed": failed}, nil
}

func (a *App) testNodeListDelay(node Node, mode string) (SpeedResult, error) {
	if a == nil || a.manager == nil {
		return SpeedResult{}, errors.New("内核管理器未初始化")
	}
	if normalizeNodeDelayMode(mode) == "real" {
		return a.manager.TestNodeMetricWithProgressAndOptions(node, "delay", nil, SpeedTestOptions{DirectOnly: true, FastDelay: true})
	}
	return a.manager.TestNodeMetricWithProgressAndOptions(node, "ping", nil, SpeedTestOptions{DirectOnly: true})
}

func normalizeNodeDelayMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "real", "url", "delay":
		return "real"
	default:
		return "tcp"
	}
}

func nodeDelayModeLabel(mode string) string {
	if normalizeNodeDelayMode(mode) == "real" {
		return "真连接延迟"
	}
	return "TCP延迟"
}

func batchNodeTestConcurrency(total int) int {
	if total <= 0 {
		return 1
	}
	workers := runtime.NumCPU() * 8
	if workers < 16 {
		workers = 16
	}
	if workers > groupNodeTestMaxConcurrency {
		workers = groupNodeTestMaxConcurrency
	}
	if workers > total {
		return total
	}
	return workers
}

func (a *App) ClipboardText() (text string, err error) {
	defer a.recoverError("读取剪贴板", &err)
	if a.ctx == nil {
		return "", errors.New("窗口未就绪")
	}
	return wailsRuntime.ClipboardGetText(a.ctx)
}

func (a *App) PickAppProxyTarget(targetType string) (target AppProxyTarget, err error) {
	defer a.recoverError("选择应用接管目标", &err)
	if a.ctx == nil {
		return AppProxyTarget{}, errors.New("窗口未就绪")
	}
	targetType = strings.TrimSpace(strings.ToLower(targetType))
	switch targetType {
	case "app":
		path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择应用",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "Windows 应用 (*.exe)", Pattern: "*.exe"},
			},
		})
		if err != nil {
			return AppProxyTarget{}, err
		}
		if strings.TrimSpace(path) == "" {
			return AppProxyTarget{}, nil
		}
		return validateAppProxyTarget(AppProxyTarget{Type: "app", Path: path})
	case "folder":
		path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择文件夹",
		})
		if err != nil {
			return AppProxyTarget{}, err
		}
		if strings.TrimSpace(path) == "" {
			return AppProxyTarget{}, nil
		}
		return validateAppProxyTarget(AppProxyTarget{Type: "folder", Path: path})
	default:
		return AppProxyTarget{}, errors.New("选择类型无效")
	}
}

func (a *App) ExportGroupLinks(groupID string) (path string, err error) {
	defer a.recoverError("导出节点组链接", &err)
	if a.ctx == nil {
		return "", errors.New("窗口未就绪")
	}
	if a.store == nil {
		return "", errors.New("配置存储未初始化")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		groupID = defaultGroupID
	}
	state := a.store.Snapshot()
	groupName := "默认"
	for _, group := range state.Groups {
		if group.ID == groupID {
			groupName = group.Name
			break
		}
	}
	items := make([]exportLinkRow, 0)
	for _, node := range state.Nodes {
		if (node.GroupID == "" && groupID != defaultGroupID) || (node.GroupID != "" && node.GroupID != groupID) {
			continue
		}
		link, err := nodeShareLink(node)
		if err != nil {
			return "", fmt.Errorf("%s：%w", node.Name, err)
		}
		items = append(items, exportLinkRow{Name: node.Name, Link: link})
	}
	if len(items) == 0 {
		return "", errors.New("当前分组没有可导出的节点")
	}
	defaultName := safeExportFileName(groupName) + ".xlsx"
	path, err = wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "导出当前分组 Excel",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Excel 文件（含节点二维码） (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if ext := filepath.Ext(path); !strings.EqualFold(ext, ".xlsx") {
		path = strings.TrimSuffix(path, ext) + ".xlsx"
	}
	if err := writeGroupLinksXLSXWithQR(path, items); err != nil {
		return "", err
	}
	a.logger.Info("已导出节点组 Excel（含二维码）：%s", path)
	return path, nil
}

func (a *App) ExportNodesLinks(nodeIDs []string) (path string, err error) {
	defer a.recoverError("导出选中节点链接", &err)
	if a.ctx == nil {
		return "", errors.New("窗口未就绪")
	}
	if a.store == nil {
		return "", errors.New("配置存储未初始化")
	}
	idSet := make(map[string]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return "", errors.New("请选择需要导出的节点")
	}
	state := a.store.Snapshot()
	items := make([]exportLinkRow, 0, len(idSet))
	for _, node := range state.Nodes {
		if _, ok := idSet[node.ID]; !ok {
			continue
		}
		link, err := nodeShareLink(node)
		if err != nil {
			return "", fmt.Errorf("%s：%w", node.Name, err)
		}
		items = append(items, exportLinkRow{Name: node.Name, Link: link})
	}
	if len(items) == 0 {
		return "", errors.New("选中节点没有可导出的节点")
	}
	defaultName := safeExportFileName("选中节点") + ".xlsx"
	path, err = wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "导出选中节点 Excel",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Excel 文件（含节点二维码） (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if ext := filepath.Ext(path); !strings.EqualFold(ext, ".xlsx") {
		path = strings.TrimSuffix(path, ext) + ".xlsx"
	}
	if err := writeGroupLinksXLSXWithQR(path, items); err != nil {
		return "", err
	}
	a.logger.Info("已导出选中节点 Excel（含二维码）：%s", path)
	return path, nil
}

func (a *App) ParseTestNodes(text string) (result map[string]interface{}, err error) {
	defer a.recoverError("解析测试节点", &err)
	if err := validateManualNodeInput(text); err != nil {
		return nil, err
	}
	nodes, errs := ParseNodes(text)
	if len(nodes) == 0 {
		return nil, errors.New("没有解析到可用节点")
	}
	if len(nodes) > maxBatchTestNodes {
		return nil, fmt.Errorf("批量测试一次最多支持 %d 个节点", maxBatchTestNodes)
	}
	return map[string]interface{}{"nodes": nodes, "errors": errs}, nil
}

func (a *App) TestRawNode(raw string, metric string, options SpeedTestOptions) (result SpeedResult, err error) {
	defer a.recoverError("测试临时节点", &err)
	if err := validateManualNodeInput(raw); err != nil {
		return SpeedResult{}, err
	}
	if a.manager == nil {
		return SpeedResult{}, errors.New("内核管理器未初始化")
	}
	nodes, errs := ParseNodes(raw)
	if len(nodes) == 0 {
		if len(errs) > 0 {
			return SpeedResult{}, errors.New(errs[0])
		}
		return SpeedResult{}, errors.New("没有解析到可用节点")
	}
	state := PersistState{}
	if a.store != nil {
		state = a.store.Snapshot()
		a.manager.SetSOCKS5TestProxyPort(state.Settings.MixedPort)
	}
	progress := speedProgressFunc(nil)
	if metric == "speed" {
		progress = func(item SpeedProgress) {
			item.Raw = nodes[0].Raw
			a.emitSpeedProgress(item)
		}
	}
	options = a.speedTestOptionsForCurrentSettings(options, nodes[0])
	result, err = a.manager.TestNodeMetricWithProgressAndOptions(nodes[0], metric, progress, options)
	if err != nil {
		a.logger.Error("临时节点测速失败：%s，%v", nodes[0].Name, err)
		return SpeedResult{}, err
	}
	switch metric {
	case "ping":
		a.logger.Info("临时节点 Ping 延迟：%s，地址 %s，%d ms", nodes[0].Name, result.PingAddress, result.PingMS)
	case "url":
		a.logger.Info("临时节点 URL 延迟：%s，%d ms", nodes[0].Name, result.URLDelayMS)
	case "ip":
		a.logger.Info("临时节点出站 IP：%s，%s", nodes[0].Name, result.OutboundIP)
	case "delay":
		a.logger.Info("临时节点 URL 延迟：%s，%d ms", nodes[0].Name, result.DelayMS)
	case "speed":
		a.logger.Info("临时节点测速：%s，下载 %.2f Mbps，上传 %.2f Mbps", nodes[0].Name, result.DownloadMbps, result.UploadMbps)
	}
	return result, nil
}

func (a *App) TestRawNodes(raws []string, metric string, options SpeedTestOptions) (results []RawNodeTestResult, err error) {
	defer a.recoverError("批量测试临时节点", &err)
	if len(raws) > maxBatchTestNodes {
		return nil, fmt.Errorf("批量测试一次最多支持 %d 个节点", maxBatchTestNodes)
	}
	totalBytes := 0
	for _, raw := range raws {
		totalBytes += len(raw)
		if totalBytes > maxManualNodeInputBytes {
			return nil, fmt.Errorf("批量测试内容不能超过 %d MB", maxManualNodeInputBytes/(1024*1024))
		}
	}
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	metric = strings.ToLower(strings.TrimSpace(metric))
	if metric != "ping" && metric != "delay" && metric != "url" && metric != "ip" {
		return nil, errors.New("批量节点检测类型无效")
	}
	results = make([]RawNodeTestResult, len(raws))
	type rawTestItem struct {
		index int
		node  Node
	}
	groups := make(map[SpeedTestOptions][]rawTestItem)
	for index, raw := range raws {
		results[index] = RawNodeTestResult{Index: index, Raw: raw}
		nodes, parseErrs := ParseNodes(raw)
		if len(nodes) == 0 {
			message := "没有解析到可用节点"
			if len(parseErrs) > 0 {
				message = parseErrs[0]
			}
			results[index].Error = message
			continue
		}
		node := nodes[0]
		node.ID = fmt.Sprintf("raw-batch-%d-%s", index, node.ID)
		nodeOptions := a.speedTestOptionsForCurrentSettings(options, node)
		groups[nodeOptions] = append(groups[nodeOptions], rawTestItem{index: index, node: node})
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for groupOptions, items := range groups {
		groupOptions := groupOptions
		items := items
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodes := make([]Node, len(items))
			indexes := make(map[string]int, len(items))
			for index, item := range items {
				nodes[index] = item.node
				indexes[item.node.ID] = item.index
			}
			var batch map[string]nodeDelayTestResult
			if metric == "ping" {
				batch = a.manager.testNodesDelayConcurrently(nodes, metric, groupOptions, batchNodeTestConcurrency(len(nodes)))
			} else {
				batch = a.manager.TestNodesMetric(nodes, metric, groupOptions)
			}
			mu.Lock()
			defer mu.Unlock()
			for id, item := range batch {
				index, ok := indexes[id]
				if !ok {
					continue
				}
				if item.err != nil {
					results[index].Error = item.err.Error()
					continue
				}
				results[index].Result = item.result
			}
		}()
	}
	wg.Wait()
	failed := 0
	for index := range results {
		if results[index].Error == "" && results[index].Result.ID == "" {
			results[index].Error = "节点测试未返回结果"
		}
		if results[index].Error != "" {
			failed++
		}
	}
	if a.logger != nil {
		a.logger.Info("批量临时节点%s完成：成功 %d 个，失败 %d 个", rawNodeMetricLabel(metric), len(results)-failed, failed)
	}
	return results, nil
}

func validateManualNodeInput(text string) error {
	if len(text) > maxManualNodeInputBytes {
		return fmt.Errorf("节点内容不能超过 %d MB", maxManualNodeInputBytes/(1024*1024))
	}
	return nil
}

func rawNodeMetricLabel(metric string) string {
	switch metric {
	case "ping":
		return " TCP 延迟测试"
	case "ip":
		return "出站 IP 检测"
	default:
		return "真连接测试"
	}
}

func (a *App) speedTestOptionsForCurrentSettings(options SpeedTestOptions, node Node) SpeedTestOptions {
	if a == nil || a.store == nil {
		return options
	}
	settings := a.store.Snapshot().Settings
	normalizeSettings(&settings)
	if settings.MixedPort > 0 && a.manager != nil {
		a.manager.SetSOCKS5TestProxyPort(settings.MixedPort)
	}
	if options.UseSystemProxy {
		options.DirectOnly = false
		options.PreferCore = false
		return options
	}
	if options.DirectOnly {
		options.UseSystemProxy = false
		options.PreferCore = false
		return options
	}
	options.PreferCore = effectiveTun(settings) || isSOCKS5Node(node)
	return options
}

func (a *App) SaveSettings(settings Settings) (err error) {
	defer a.recoverError("保存设置", &err)
	externalProxyEndpointsProvided := settings.ExternalProxyEndpoints != nil
	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return errors.New("代理正在切换节点或执行自检，设置未应用，请稍后重试")
	}
	defer a.proxyOpMu.Unlock()
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if a.manager == nil {
		return errors.New("内核管理器未初始化")
	}
	if settings.Mode != "rule" && settings.Mode != "global" {
		return errors.New("模式只能是 rule/global")
	}
	if settings.MixedPort < 1024 || settings.MixedPort > 65535 {
		return fmt.Errorf("本地端口 %d 无效，范围必须是 1024-65535", settings.MixedPort)
	}
	if settings.MixedPort == clashAPIPort {
		return fmt.Errorf("本地端口不能使用内核控制端口 %d", clashAPIPort)
	}
	if err := validateExternalProxyEndpoints(settings.ExternalProxyEndpoints); err != nil {
		return err
	}
	state := a.store.Snapshot()
	current := state.Settings
	normalizeSettings(&current)
	enforceProxyEntryTransition(current, &settings)
	normalizeSettings(&settings)
	a.manager.SetSOCKS5TestProxyPort(settings.MixedPort)
	if !externalProxyEndpointsProvided {
		settings.ExternalProxyEndpoints = state.Settings.ExternalProxyEndpoints
		normalizeSettings(&settings)
	}
	if err := validateSelectedNodeForSettings(state, settings); err != nil {
		return err
	}
	if err := validateProxyMapSettings(settings, len(proxyMapNodesForSettings(state.Nodes, settings))); err != nil {
		return err
	}
	if settings.RuleUpdateInterval != "" && settings.RuleUpdateInterval != "168h" {
		return errors.New("规则检测间隔只能是 168h")
	}
	previousState := state
	previousState.Settings = current
	if a.logger != nil {
		a.logger.Info("收到设置变更：模式=%s，系统代理=%v，TUN=%v，代理映射=%v，应用接管=%v，当前运行=%v", settings.Mode, settings.SystemProxy, settings.Tun, settings.ProxyMap, settings.AppProxy, a.manager.Running())
	}
	if runtime.GOOS == "windows" && effectiveTun(settings) && !effectiveTun(current) && !hasAdminPrivileges() {
		if err := a.confirmTunAdminRestart(settings); err != nil {
			return err
		}
		return nil
	}
	if a.manager.Running() {
		if !canApplySettingsWhileRunning(current, settings) {
			return errors.New("运行中只能修改系统代理、TUN、规则/全局模式和自定义直连域名，请先停止后再修改其他设置")
		}
		if !shouldRunProxy(settings) {
			if err := a.store.UpdateSettings(settings); err != nil {
				return err
			}
			a.persistTraffic()
			if runtime.GOOS == "windows" && current.SystemProxy {
				if err := DisableSystemProxy(); err != nil {
					a.logger.Error("关闭系统代理失败：%v", err)
					a.emitState()
					return err
				}
			}
			if err := a.manager.StopWithoutSystemProxyCleanup(); err != nil {
				a.logger.Error("关闭入口后停止代理失败：%v", err)
			}
			a.resetTrafficTracking()
			a.logger.Info("代理入口已全部关闭，代理已停止")
			a.emitState()
			return nil
		}
		if err := a.store.UpdateSettings(settings); err != nil {
			return err
		}
		state.Settings = settings
		restartNeeded := needsRestartForRunningSettings(current, settings)
		if runtime.GOOS == "windows" && current.SystemProxy != settings.SystemProxy && !restartNeeded {
			if settings.SystemProxy {
				if err := EnableSystemProxy(settings.MixedPort); err != nil {
					return a.rollbackSettingsAfterApplyFailure(previousState, err)
				}
			} else if err := DisableSystemProxy(); err != nil {
				return a.rollbackSettingsAfterApplyFailure(previousState, err)
			}
		}
		if restartNeeded {
			cleanupTun := shouldCleanupTunOnRunningRestart(current, settings)
			if runtime.GOOS == "windows" && current.SystemProxy && !settings.SystemProxy {
				if err := DisableSystemProxy(); err != nil {
					return a.rollbackSettingsAfterApplyFailure(previousState, err)
				}
			}
			if err := a.manager.Restart(state, cleanupTun); err != nil {
				a.logger.Error("设置已保存，但代理重启失败：%v", err)
				return a.rollbackSettingsAfterApplyFailure(previousState, err)
			}
			if err := a.manager.ValidateRuntimeSettings(settings); err != nil {
				a.logger.Error("%v", err)
				return a.rollbackSettingsAfterApplyFailure(previousState, err)
			}
			a.logger.Info("运行中设置已保存并重启内核生效：模式=%s，系统代理=%v，TUN=%v", settings.Mode, settings.SystemProxy, settings.Tun)
		} else {
			if current.Mode != settings.Mode {
				if err := a.manager.SetMode(settings.Mode); err != nil {
					a.logger.Error("Clash API 切换模式失败，尝试重启同步：%v", err)
					if err := a.manager.Restart(state, false); err != nil {
						a.logger.Error("模式切换后重启内核失败：%v", err)
						return a.rollbackSettingsAfterApplyFailure(previousState, err)
					}
				} else if err := a.manager.CloseConnections(); err != nil {
					a.logger.Error("切换模式后清理旧连接失败，尝试重启同步：%v", err)
					if err := a.manager.Restart(state, false); err != nil {
						a.logger.Error("清理旧连接失败后重启内核失败：%v", err)
						return a.rollbackSettingsAfterApplyFailure(previousState, err)
					}
				} else {
					a.logger.Info("切换模式后已清理旧连接以立即生效")
				}
				if err := a.manager.ValidateRuntimeSettings(settings); err != nil {
					a.logger.Error("模式切换校验失败，尝试重启同步：%v", err)
					if err := a.manager.Restart(state, effectiveTun(settings)); err != nil {
						a.logger.Error("模式切换校验失败后重启内核失败：%v", err)
						return a.rollbackSettingsAfterApplyFailure(previousState, err)
					}
					if err := a.manager.ValidateRuntimeSettings(settings); err != nil {
						a.logger.Error("%v", err)
						return a.rollbackSettingsAfterApplyFailure(previousState, err)
					}
				}
			}
			a.logger.Info("运行中设置已保存并通过 Clash API 生效：模式=%s，系统代理=%v", settings.Mode, settings.SystemProxy)
		}
		a.markTunConnectivityHealthy(settings)
		a.emitState()
		return nil
	}
	settings.AutoStart = false
	if err := syncAutoStart(false); err != nil {
		return err
	}
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	a.logger.Info("设置已保存：模式=%s，系统代理=%v，TUN=%v，端口=%d，代理映射=%v，映射起始端口=%d，应用接管=%v，规则源=%s，FakeDNS=%v，自启动=%v", settings.Mode, settings.SystemProxy, settings.Tun, settings.MixedPort, settings.ProxyMap, settings.ProxyMapStartPort, settings.AppProxy, settings.RuleSource, settings.FakeDNS, settings.AutoStart)
	if shouldRunProxy(settings) {
		a.logger.Info("设置要求开启代理入口，开始启动 sing-box：模式=%s，系统代理=%v，TUN=%v，代理映射=%v", settings.Mode, settings.SystemProxy, settings.Tun, settings.ProxyMap)
		state.Settings = settings
		if !stateHasSelectedNode(state) {
			a.logger.Error("启动失败：请先选择节点")
			return a.rollbackSettingsAfterApplyFailure(previousState, errors.New("请先选择节点"))
		}
		if err := a.manager.CleanupStale(); err != nil {
			a.logger.Error("清理残留内核失败：%v", err)
			return a.rollbackSettingsAfterApplyFailure(previousState, err)
		}
		a.cleanupSystemProxyForStartup(state.Settings)
		if err := a.prepareStartupPorts(state); err != nil {
			a.logger.Error("启动失败：%v", err)
			a.cleanupSystemProxyForStartup(state.Settings)
			return a.rollbackSettingsAfterApplyFailure(previousState, err)
		}
		if err := a.manager.Start(state); err != nil {
			a.logger.Error("启动失败：%v", err)
			a.cleanupSystemProxyForStartup(state.Settings)
			return a.rollbackSettingsAfterApplyFailure(previousState, err)
		}
		a.markTunConnectivityHealthy(state.Settings)
		a.initTrafficBaseline(state.SelectedID, state.Nodes)
		a.logger.Info("代理入口已开启，代理已自动启动")
	}
	a.emitState()
	return nil
}

func (a *App) lockProxyOperation(timeout time.Duration) bool {
	if timeout <= 0 {
		return a.proxyOpMu.TryLock()
	}
	deadline := time.Now().Add(timeout)
	for {
		if a.proxyOpMu.TryLock() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func (a *App) SaveTheme(theme string) (err error) {
	defer a.recoverError("保存主题", &err)
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if err := a.store.UpdateTheme(theme); err != nil {
		return err
	}
	a.emitState()
	return nil
}

func shouldRunProxy(settings Settings) bool {
	return settings.SystemProxy || settings.Tun || settings.ProxyMap || appProxyActive(settings)
}

func validateSelectedNodeForSettings(state PersistState, settings Settings) error {
	if shouldRunProxy(settings) && !stateHasSelectedNode(state) {
		return errors.New("请先选择有效节点")
	}
	return nil
}

func (a *App) rollbackSettingsAfterApplyFailure(previous PersistState, applyErr error) error {
	if applyErr == nil {
		return nil
	}
	if a.logger != nil {
		a.logger.Error("代理设置应用失败，正在恢复切换前状态：%v", applyErr)
	}
	rollbackErr := a.restoreProxyState(previous)
	a.emitState()
	if rollbackErr != nil {
		return fmt.Errorf("%w；恢复切换前代理状态失败：%v", applyErr, rollbackErr)
	}
	return fmt.Errorf("%w；已恢复切换前代理状态", applyErr)
}

func (a *App) restoreProxyState(previous PersistState) error {
	if a.store == nil || a.manager == nil {
		return errors.New("代理状态恢复组件未初始化")
	}
	normalizeSettings(&previous.Settings)
	if err := a.store.UpdateSettings(previous.Settings); err != nil {
		return fmt.Errorf("恢复设置失败：%w", err)
	}
	if !shouldRunProxy(previous.Settings) {
		if a.manager.Running() {
			if err := a.manager.Stop(); err != nil {
				return fmt.Errorf("停止失败代理失败：%w", err)
			}
		}
		a.resetTrafficTracking()
		a.markTunConnectivityHealthy(previous.Settings)
		if a.logger != nil {
			a.logger.Info("已恢复切换前状态：代理入口关闭")
		}
		return nil
	}
	if !stateHasSelectedNode(previous) {
		return errors.New("切换前选择的节点不存在")
	}
	var err error
	if a.manager.Running() {
		err = a.manager.Restart(previous, runtime.GOOS == "windows")
	} else {
		if cleanupErr := a.manager.CleanupStale(); cleanupErr != nil {
			return cleanupErr
		}
		err = a.manager.Start(previous)
	}
	if err != nil {
		return fmt.Errorf("恢复切换前代理失败：%w", err)
	}
	a.initTrafficBaseline(previous.SelectedID, previous.Nodes)
	a.markTunConnectivityHealthy(previous.Settings)
	if a.logger != nil {
		a.logger.Info("已恢复切换前代理状态：模式=%s，系统代理=%v，TUN=%v", previous.Settings.Mode, previous.Settings.SystemProxy, previous.Settings.Tun)
	}
	return nil
}

func (a *App) disableTunAfterStartFailure(settings Settings, startErr error) {
	if a.store == nil {
		return
	}
	normalizeSettings(&settings)
	if !effectiveTun(settings) {
		return
	}
	if settings.CircuitBreaker {
		a.setCircuitBreakerActive(true, fmt.Sprintf("代理出网异常，智能熔断保持防护：%v", startErr))
		if a.logger != nil {
			a.logger.Error("TUN 代理出网异常，已触发智能分流熔断保护：保留代理接管，国内直连正常，阻断海外流量以防直连泄露：%v", startErr)
		}
		return
	}
	settings.Tun = false
	settings.SystemProxy = false
	if err := a.store.UpdateSettings(settings); err != nil {
		if a.logger != nil {
			a.logger.Error("TUN 启动失败后保存关闭状态失败：%v", err)
		}
		return
	}
	a.healthMu.Lock()
	a.lastTunConnectivityCheck = time.Time{}
	a.lastTunConnectivityFailure = time.Time{}
	a.lastTunConnectivityError = ""
	a.healthMu.Unlock()
	if a.logger != nil {
		a.logger.Error("TUN 启动失败，已自动关闭 TUN 防止反复中断网络：%v", startErr)
	}
}

func (a *App) setCircuitBreakerActive(active bool, reason string) {
	if a == nil {
		return
	}
	a.circuitBreakerMu.Lock()
	changed := a.circuitBreakerActive != active || (active && a.circuitBreakerText != reason)
	a.circuitBreakerActive = active
	if active {
		a.circuitBreakerText = reason
	} else {
		a.circuitBreakerText = ""
	}
	a.circuitBreakerMu.Unlock()
	if changed {
		if a.logger != nil {
			if active {
				a.logger.Error("【智能分流熔断】已触发安全防护：%s。国内应用（微信/百度等）保持正常畅通，海外访问已拦截以防真实 IP 泄露。", reason)
			} else {
				a.logger.Info("【智能分流熔断】已解除熔断防护，代理链路恢复正常。")
			}
		}
		a.emitState()
	}
}

func (a *App) isCircuitBreakerActive() bool {
	if a == nil {
		return false
	}
	a.circuitBreakerMu.Lock()
	defer a.circuitBreakerMu.Unlock()
	return a.circuitBreakerActive
}

func (a *App) circuitBreakerReason() string {
	if a == nil {
		return ""
	}
	a.circuitBreakerMu.Lock()
	defer a.circuitBreakerMu.Unlock()
	return a.circuitBreakerText
}

func (a *App) verifyTunConnectivityIfDue(port int) error {
	now := time.Now()
	a.healthMu.Lock()
	if !a.lastTunConnectivityFailure.IsZero() && now.Sub(a.lastTunConnectivityFailure) < tunConnectivityFailureCache {
		message := a.lastTunConnectivityError
		a.healthMu.Unlock()
		return errors.New(firstNonEmpty(message, "TUN 代理出网验证失败"))
	}
	interval := tunConnectivityCheckInterval
	if a.store != nil && a.store.Snapshot().Settings.AutoFailover {
		interval = 15 * time.Second
	}
	if !a.lastTunConnectivityCheck.IsZero() && now.Sub(a.lastTunConnectivityCheck) < interval {
		a.healthMu.Unlock()
		return nil
	}
	a.healthMu.Unlock()
	err := verifyLocalProxyConnectivity(port, tunConnectivityTimeout)
	a.healthMu.Lock()
	defer a.healthMu.Unlock()
	if err == nil {
		a.lastTunConnectivityCheck = now
		a.lastTunConnectivityFailure = time.Time{}
		a.lastTunConnectivityError = ""
	} else {
		a.lastTunConnectivityCheck = time.Time{}
		a.lastTunConnectivityFailure = now
		a.lastTunConnectivityError = err.Error()
	}
	return err
}

func (a *App) markTunConnectivityHealthy(settings Settings) {
	a.healthMu.Lock()
	if effectiveTun(settings) {
		a.lastTunConnectivityCheck = time.Now()
	} else {
		a.lastTunConnectivityCheck = time.Time{}
	}
	a.lastTunConnectivityFailure = time.Time{}
	a.lastTunConnectivityError = ""
	a.healthMu.Unlock()
	a.setCircuitBreakerActive(false, "")
}

func stateHasSelectedNode(state PersistState) bool {
	if strings.TrimSpace(state.SelectedID) == "" {
		return false
	}
	for _, node := range state.Nodes {
		if node.ID == state.SelectedID {
			return true
		}
	}
	return false
}

func (a *App) cleanupSystemProxyForStartup(settings Settings) {
	if runtime.GOOS != "windows" || !settings.SystemProxy {
		return
	}
	ok, err := systemProxyEnabledForPort(settings.MixedPort)
	if err != nil {
		a.logger.Error("读取系统代理失败：%v", err)
		return
	}
	if !ok {
		return
	}
	if err := DisableSystemProxy(); err != nil {
		a.logger.Error("关闭残留系统代理失败：%v", err)
		return
	}
	a.logger.Info("已关闭残留系统代理，等待内核监听成功后重新开启：127.0.0.1:%d", settings.MixedPort)
}

func (a *App) applyRuntimeSelectedNode(state PersistState, reason string) error {
	node, ok := findSelectedNode(state.Nodes, state.SelectedID)
	if !ok || node.ID != state.SelectedID {
		return errors.New("节点不存在")
	}
	selectedTag := nodeOutboundTag(node)
	if selectedTag == "" {
		return errors.New("所选节点缺少 ID")
	}
	if !a.manager.RuntimeHasOutbound(selectedTag) {
		if err := a.manager.Restart(state, effectiveTun(state.Settings)); err != nil {
			return err
		}
	} else if err := a.manager.SelectOutboundVerified(selectedTag); err != nil {
		a.logger.Error("%s：快速切换节点验证失败：%v", reason, err)
		if healthErr := a.manager.HealthCheck(state.Settings); healthErr != nil {
			a.logger.Error("%s：内核控制接口异常，正在重启内核恢复：%v", reason, healthErr)
			if restartErr := a.manager.Restart(state, effectiveTun(state.Settings)); restartErr != nil {
				a.logger.Error("%s：重启内核恢复失败：%v", reason, restartErr)
				a.emitState()
				return restartErr
			}
			if verifyErr := a.manager.SelectOutboundVerified(selectedTag); verifyErr != nil {
				a.logger.Error("%s：重启后仍未确认节点切换：%v", reason, verifyErr)
				a.emitState()
				return verifyErr
			}
		} else {
			a.logger.Error("%s：控制接口正常但节点未切换，正在强确认切换", reason)
			if reliableErr := a.manager.SelectOutboundReliable(selectedTag); reliableErr != nil {
				a.logger.Error("%s：强确认切换失败，正在重启内核加载新选择：%v", reason, reliableErr)
				if restartErr := a.manager.Restart(state, effectiveTun(state.Settings)); restartErr != nil {
					a.logger.Error("%s：重启内核加载新选择失败：%v", reason, restartErr)
					a.emitState()
					return restartErr
				}
				if verifyErr := a.manager.SelectOutboundVerified(selectedTag); verifyErr != nil {
					a.logger.Error("%s：重启后仍未确认节点切换：%v", reason, verifyErr)
					a.emitState()
					return verifyErr
				}
			}
		}
	}
	a.initTrafficBaseline(state.SelectedID, state.Nodes)
	a.logger.Info("%s：已快速切换到节点：%s", reason, node.Name)
	return nil
}

func (a *App) syncRuntimeNodeList(previous, state PersistState, reason string) error {
	a.proxyOpMu.Lock()
	defer a.proxyOpMu.Unlock()
	if a.manager == nil {
		return nil
	}
	if a.store != nil {
		state = a.store.Snapshot()
	}
	if !a.manager.Running() {
		if len(previous.Nodes) == 0 && len(state.Nodes) > 0 && shouldRunProxy(state.Settings) {
			if !stateHasSelectedNode(state) {
				a.logger.Error("%s后未自动启动代理：请先选择节点", reason)
				a.emitState()
				return nil
			}
			if err := a.startLocked(); err != nil {
				a.logger.Error("%s后自动启动代理失败：%v", reason, err)
				a.emitState()
				return err
			}
			a.logger.Info("%s：检测到新增节点，代理已自动启动", reason)
		}
		return nil
	}
	a.persistTraffic()
	if len(state.Nodes) == 0 {
		if err := a.manager.Stop(); err != nil {
			a.logger.Error("%s失败：%v", reason, err)
			a.emitState()
			return err
		}
		a.resetTrafficTracking()
		a.logger.Info("%s：已无可用节点，代理已停止", reason)
		return nil
	}
	if !nodeListAffectsRunningConfig(previous, state) {
		a.logger.Info("%s：当前运行配置未受影响，跳过重启", reason)
		return nil
	}
	settings := state.Settings
	normalizeSettings(&settings)
	previousSettings := previous.Settings
	normalizeSettings(&previousSettings)
	if !reflect.DeepEqual(previousSettings.ExternalProxyEndpoints, settings.ExternalProxyEndpoints) {
		if err := a.manager.Restart(state, shouldCleanupTunOnRunningRestart(previousSettings, settings)); err != nil {
			a.logger.Error("%s失败：%v", reason, err)
			a.emitState()
			return err
		}
		if err := a.manager.ValidateRuntimeSettings(settings); err != nil {
			a.logger.Error("%s运行配置校验失败：%v", reason, err)
			a.emitState()
			return err
		}
		a.initTrafficBaseline(state.SelectedID, state.Nodes)
		a.logger.Info("%s完成，外部代理入口已写入并回读验证", reason)
		return nil
	}
	if previous.SelectedID != state.SelectedID && !settings.ProxyMap {
		return a.applyRuntimeSelectedNode(state, reason)
	}
	if err := a.manager.Restart(state, false); err != nil {
		a.logger.Error("%s失败：%v", reason, err)
		a.emitState()
		return err
	}
	a.initTrafficBaseline(state.SelectedID, state.Nodes)
	a.logger.Info("%s完成", reason)
	return nil
}

func nodeListAffectsRunningConfig(previous, next PersistState) bool {
	prevSettings := previous.Settings
	nextSettings := next.Settings
	normalizeSettings(&prevSettings)
	normalizeSettings(&nextSettings)
	if prevSettings.ProxyMap || nextSettings.ProxyMap {
		return true
	}
	if !reflect.DeepEqual(prevSettings.ExternalProxyEndpoints, nextSettings.ExternalProxyEndpoints) {
		return true
	}
	if previous.SelectedID != next.SelectedID {
		return true
	}
	prevNode, prevOK := findSelectedNode(previous.Nodes, previous.SelectedID)
	nextNode, nextOK := findSelectedNode(next.Nodes, next.SelectedID)
	if prevOK != nextOK {
		return true
	}
	if !nextOK {
		return false
	}
	return !reflect.DeepEqual(prevNode.Outbound, nextNode.Outbound)
}

func shouldCleanupTunOnRunningRestart(current, next Settings) bool {
	return effectiveTun(current) || effectiveTun(next)
}

const (
	adminRestartRequiredTun         = "ADMIN_RESTART_REQUIRED:TUN"
	adminRestartRequiredNetworkTool = "ADMIN_RESTART_REQUIRED:NETWORK_TOOL"
)

func (a *App) confirmTunAdminRestart(settings Settings) error {
	return errors.New(adminRestartRequiredTun)
}

func (a *App) RestartTunAsAdmin(settings Settings) (err error) {
	defer a.recoverError("管理员重启启用 TUN", &err)
	if err := func() error {
		if !a.lockProxyOperation(saveSettingsLockTimeout) {
			return errProxyOperationBusy
		}
		defer a.proxyOpMu.Unlock()
		if a.store == nil || a.manager == nil {
			return errors.New("代理组件未初始化")
		}
		previous := a.store.Snapshot()
		current := previous.Settings
		normalizeSettings(&current)
		enforceProxyEntryTransition(current, &settings)
		normalizeSettings(&settings)
		if !effectiveTun(settings) {
			return errors.New("管理员重启仅用于启用 TUN")
		}
		if err := validateSelectedNodeForSettings(previous, settings); err != nil {
			return err
		}
		if err := a.store.UpdateSettings(settings); err != nil {
			return err
		}
		rollback := func(cause error) error {
			if rollbackErr := a.restoreProxyState(previous); rollbackErr != nil {
				return fmt.Errorf("%w；恢复切换前代理失败：%v", cause, rollbackErr)
			}
			return fmt.Errorf("%w；已恢复切换前代理", cause)
		}
		if a.manager.Running() {
			a.persistTraffic()
			if err := a.manager.Stop(); err != nil {
				return rollback(err)
			}
		}
		if err := a.manager.CleanupStale(); err != nil {
			return rollback(err)
		}
		if current.SystemProxy {
			if err := DisableSystemProxy(); err != nil {
				return rollback(fmt.Errorf("切换 TUN 前关闭系统代理失败：%w", err))
			}
		}
		if !waitTCPPortFreeStrict("127.0.0.1", settings.MixedPort, 5*time.Second) {
			return rollback(fmt.Errorf("切换 TUN 前本地代理端口 %d 未释放", settings.MixedPort))
		}
		if !waitTCPPortFreeStrict("127.0.0.1", clashAPIPort, 5*time.Second) {
			return rollback(fmt.Errorf("切换 TUN 前内核控制端口 %d 未释放", clashAPIPort))
		}
		if err := restartAsAdmin(adminRestartArg + " " + adminTunHandoffArg); err != nil {
			return rollback(err)
		}
		if err := markProxyWatchdogAdminHandoff(a.root, os.Getpid()); err != nil {
			return rollback(fmt.Errorf("写入管理员 TUN 交接标记失败：%w", err))
		}
		a.logger.Info("正在以管理员权限重启以启用 TUN 代理")
		a.markAdminTunHandoffExit()
		a.markClosingAllowed()
		return nil
	}(); err != nil {
		return err
	}
	wailsRuntime.Quit(a.ctx)
	return nil
}

func (a *App) confirmNetworkToolAdminRestart() error {
	return errors.New(adminRestartRequiredNetworkTool)
}

func (a *App) RestartNetworkToolAsAdmin() (err error) {
	defer a.recoverError("管理员重启执行网络工具", &err)
	if err := func() error {
		if !a.lockProxyOperation(saveSettingsLockTimeout) {
			return errProxyOperationBusy
		}
		defer a.proxyOpMu.Unlock()
		if a.ctx == nil {
			return errors.New("网络修复工具需要管理员权限，请以管理员身份重新启动应用")
		}
		if a.manager != nil && a.manager.Running() {
			a.persistTraffic()
			if err := a.manager.Stop(); err != nil {
				return err
			}
		}
		if err := restartAsAdmin(adminRestartArg); err != nil {
			return err
		}
		a.logger.Info("正在以管理员权限重启以执行网络修复工具")
		a.markClosingAllowed()
		return nil
	}(); err != nil {
		return err
	}
	wailsRuntime.Quit(a.ctx)
	return nil
}

func canApplySettingsWhileRunning(current, next Settings) bool {
	return current.MixedPort == next.MixedPort &&
		current.ProxyMapStartPort == next.ProxyMapStartPort &&
		current.RuleSource == next.RuleSource &&
		current.AutoUpdateRules == next.AutoUpdateRules &&
		current.RuleUpdateInterval == next.RuleUpdateInterval &&
		current.FakeDNS == next.FakeDNS &&
		current.AutoStart == next.AutoStart
}

func needsRestartForRunningSettings(current, next Settings) bool {
	return current.Tun != next.Tun ||
		appProxyActive(current) != appProxyActive(next) ||
		current.ProxyMap != next.ProxyMap ||
		current.AppProxy != next.AppProxy ||
		!reflect.DeepEqual(current.AppProxyTargets, next.AppProxyTargets) ||
		!reflect.DeepEqual(current.ExternalProxyEndpoints, next.ExternalProxyEndpoints) ||
		current.DirectDomains != next.DirectDomains
}

func sameProxyMapGroupIDs(current, next Settings) bool {
	currentIDs := normalizeProxyMapGroupIDs(current.ProxyMapGroupIDs, current.ProxyMapGroupID)
	nextIDs := normalizeProxyMapGroupIDs(next.ProxyMapGroupIDs, next.ProxyMapGroupID)
	if len(currentIDs) != len(nextIDs) {
		return false
	}
	for i := range currentIDs {
		if currentIDs[i] != nextIDs[i] {
			return false
		}
	}
	return true
}

func waitTCPPortFree(host string, port int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isTCPPortFree(host, port) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (a *App) UpdateCore() (result map[string]string, err error) {
	defer a.recoverError("更新内核", &err)
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	a.coreUpdateMu.Lock()
	defer a.coreUpdateMu.Unlock()

	version, candidate, err := a.manager.PrepareCoreUpdate()
	if err != nil {
		a.logger.Error("内核更新失败：%v", err)
		a.emitState()
		return nil, err
	}
	if candidate == nil {
		a.emitState()
		return map[string]string{"version": version, "status": "already_latest"}, nil
	}
	defer candidate.Cleanup()

	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return nil, errProxyOperationBusy
	}
	defer a.proxyOpMu.Unlock()
	state := a.store.Snapshot()
	err = a.manager.ApplyCoreUpdate(candidate, state)
	if err != nil {
		a.logger.Error("内核更新失败：%v", err)
		a.emitState()
		return nil, err
	}
	a.emitState()
	return map[string]string{"version": version, "status": "updated"}, nil
}

func (a *App) UpdateRules() (result map[string]string, err error) {
	defer a.recoverError("更新规则", &err)
	if a.manager == nil {
		return nil, errors.New("内核管理器未初始化")
	}
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	state := a.store.Snapshot()
	updateResult, err := a.manager.UpdateRuleSetsIfNeeded(state.Settings)
	if err != nil {
		a.logger.Error("规则更新失败：%v", err)
		a.emitState()
		return nil, err
	}
	if updateResult.Latest {
		a.logger.Info("规则已是最新版本：%s", updateResult.Version)
		a.emitState()
		return map[string]string{"status": "latest", "count": "0", "version": updateResult.Version}, nil
	}
	a.logger.Info("规则已后台更新：%d 个文件，版本：%s；当前代理不会被重启，重启代理后生效", updateResult.Count, updateResult.Version)
	a.emitState()
	return map[string]string{"status": "updated", "count": fmt.Sprintf("%d", updateResult.Count), "version": updateResult.Version}, nil
}

func (a *App) startupCheckAppUpdate() {
	time.Sleep(2500 * time.Millisecond)
	info, err := a.CheckAppUpdate(false)
	if err != nil {
		if a.logger != nil {
			a.logger.Info("启动检查软件更新：%v", err)
		}
		return
	}
	if info != nil && info.Available {
		if a.logger != nil {
			a.logger.Info("发现软件新版本：%s（当前版本：%s）", info.LatestVersion, info.CurrentVersion)
		}
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "app-update-available", info)
		}
		a.emitState()
	}
}

func (a *App) CheckAppUpdate(force bool) (result *AppUpdateInfo, err error) {
	defer a.recoverError("检查软件更新", &err)
	if a == nil || a.appUpdater == nil {
		return nil, errors.New("软件更新管理器未初始化")
	}
	proxyPort := 0
	if a.manager != nil && a.manager.Running() && a.store != nil {
		proxyPort = a.store.Snapshot().Settings.MixedPort
	}
	info, err := a.appUpdater.CheckUpdate(proxyPort)
	if err != nil {
		return nil, err
	}
	a.emitState()
	return info, nil
}

func (a *App) ApplyAppUpdate() (result *AppUpdateInfo, err error) {
	defer a.recoverError("执行软件更新", &err)
	if a == nil || a.appUpdater == nil {
		return nil, errors.New("软件更新管理器未初始化")
	}
	a.appUpdater.mu.Lock()
	if a.appUpdater.isUpdating {
		a.appUpdater.mu.Unlock()
		return nil, errors.New("软件更新正在进行中，请勿重复操作")
	}
	a.appUpdater.isUpdating = true
	a.appUpdater.mu.Unlock()

	defer func() {
		a.appUpdater.mu.Lock()
		a.appUpdater.isUpdating = false
		a.appUpdater.mu.Unlock()
	}()

	proxyPort := 0
	if a.manager != nil && a.manager.Running() && a.store != nil {
		proxyPort = a.store.Snapshot().Settings.MixedPort
	}

	info, err := a.appUpdater.CheckUpdate(proxyPort)
	if err != nil {
		return nil, err
	}
	if !info.Available || info.DownloadURL == "" {
		return info, errors.New("未检测到可更新的版本下载包")
	}

	updatesDir := filepath.Join(a.root, "data", "updates")
	_ = os.MkdirAll(updatesDir, 0755)
	zipPath := filepath.Join(updatesDir, "update.zip")
	stageDir := filepath.Join(updatesDir, "staging")
	_ = os.Remove(zipPath)
	_ = os.RemoveAll(stageDir)
	_ = os.MkdirAll(stageDir, 0755)

	if a.logger != nil {
		a.logger.Info("开始下载软件更新包：%s", info.DownloadURL)
	}

	onProgress := func(downloaded, total int64, speed string, percent float64) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
				Phase:       "downloading",
				Percent:     percent,
				Bytes:       downloaded,
				Total:       total,
				SpeedString: speed,
				Message:     fmt.Sprintf("正在下载：%.1f%% (%s)", percent, speed),
			})
		}
	}

	if err := downloadUpdateAsset(info.DownloadURL, zipPath, proxyPort, onProgress); err != nil {
		return nil, fmt.Errorf("下载更新包失败：%w", err)
	}

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
			Phase:   "extracting",
			Percent: 100,
			Message: "下载完成，正在解压更新包...",
		})
	}
	if a.logger != nil {
		a.logger.Info("更新包下载完成，正在解压...")
	}

	if err := extractZipArchive(zipPath, stageDir); err != nil {
		return nil, fmt.Errorf("解压更新包失败：%w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		exePath = filepath.Join(a.root, "SanTian-Singbox.exe")
	}
	exeName := filepath.Base(exePath)
	extractedAppDir := locateExtractedAppDir(stageDir, exeName)

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
			Phase:   "applying",
			Percent: 100,
			Message: "准备就绪，正在进行原子级热替换并重启...",
		})
	}
	if a.logger != nil {
		a.logger.Info("开始执行原子级更新文件替换...")
	}

	// 1. 停止 sing-box 核心内核以释放 bin/ 目录下的二进制文件句柄
	if a.manager != nil && a.manager.Running() {
		_ = a.Stop()
		time.Sleep(200 * time.Millisecond)
	}

	// 2. 进程内原子级替换文件（重命名旧程序，拷贝新程序）
	if err := performAtomicInPlaceUpdate(a.root, extractedAppDir, exePath); err != nil {
		return nil, fmt.Errorf("原子更新失败：%w", err)
	}

	// 3. 直接通过系统标准 API 启动新程序
	if err := launchNewAppProcess(exePath, a.root); err != nil {
		return nil, fmt.Errorf("启动新版本程序失败：%w", err)
	}

	if a.logger != nil {
		a.logger.Info("新版本程序已成功拉起，旧进程即将安全退出")
	}

	// 4. 旧进程安全退出
	go func() {
		time.Sleep(200 * time.Millisecond)
		if a.ctx != nil {
			wailsRuntime.Quit(a.ctx)
		}
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()

	return info, nil
}

func (a *App) Start() (err error) {
	defer a.recoverError("启动代理", &err)
	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return errProxyOperationBusy
	}
	defer a.proxyOpMu.Unlock()
	return a.startLocked()
}

func (a *App) startLocked() error {
	if a.store == nil {
		return errors.New("配置存储未初始化")
	}
	if a.manager == nil {
		return errors.New("内核管理器未初始化")
	}
	if a.manager.Running() {
		a.logger.Info("检测到旧内核正在运行，先停止旧进程")
		a.persistTraffic()
		if err := a.manager.Stop(); err != nil {
			a.logger.Error("停止旧内核失败：%v", err)
			a.emitState()
			return err
		}
		a.resetTrafficTracking()
	}
	state := a.store.Snapshot()
	if !stateHasSelectedNode(state) {
		return errors.New("请先选择节点")
	}
	if !shouldRunProxy(state.Settings) {
		var restored bool
		state.Settings, restored = a.proxySettingsForStart(state.Settings)
		if err := a.store.UpdateSettings(state.Settings); err != nil {
			a.logger.Error("保存代理启动设置失败：%v", err)
			a.emitState()
			return err
		}
		if restored {
			a.logger.Info("已恢复停止前使用的代理入口")
		} else {
			a.logger.Info("没有可恢复的代理入口，已自动开启系统代理")
		}
	}
	if err := a.manager.CleanupStale(); err != nil {
		a.logger.Error("清理残留内核失败：%v", err)
		a.emitState()
		return err
	}
	a.cleanupSystemProxyForStartup(state.Settings)
	if err := a.prepareStartupPorts(state); err != nil {
		a.logger.Error("启动失败：%v", err)
		a.cleanupSystemProxyForStartup(state.Settings)
		a.emitState()
		return err
	}
	if err := a.manager.Start(state); err != nil {
		a.logger.Error("启动失败：%v", err)
		a.cleanupSystemProxyForStartup(state.Settings)
		a.disableTunAfterStartFailure(state.Settings, err)
		a.emitState()
		return err
	}
	a.markTunConnectivityHealthy(state.Settings)
	a.initTrafficBaseline(state.SelectedID, state.Nodes)
	a.emitState()
	return nil
}

func (a *App) prepareStartupPorts(state PersistState) error {
	settings := state.Settings
	normalizeSettings(&settings)
	ports := []struct {
		name string
		port int
	}{
		{name: "本地代理", port: settings.MixedPort},
		{name: "内核控制", port: clashAPIPort},
	}
	if settings.ProxyMap {
		for index := range proxyMapNodesForSettings(state.Nodes, settings) {
			ports = append(ports, struct {
				name string
				port int
			}{name: "代理映射", port: proxyMapPort(settings, index)})
		}
	}
	seen := map[int]bool{}
	uniquePorts := make([]struct {
		name string
		port int
	}, 0, len(ports))
	portsToCheck := make([]int, 0, len(ports))
	for _, item := range ports {
		if seen[item.port] {
			continue
		}
		seen[item.port] = true
		uniquePorts = append(uniquePorts, item)
		// 端口已被占用时才纳入排查，端口空闲时 0ms 秒开无需任何系统查询
		if !isTCPPortFree("127.0.0.1", item.port) {
			portsToCheck = append(portsToCheck, item.port)
		}
	}
	if len(portsToCheck) == 0 {
		return nil
	}
	portProcesses, err := findTCPPortsProcesses("127.0.0.1", portsToCheck)
	if err != nil {
		a.logger.Info("查询端口占用进程失败：%v，尝试继续释放或验证端口", err)
	}
	for _, item := range uniquePorts {
		if isTCPPortFree("127.0.0.1", item.port) {
			continue
		}
		var procs []portProcess
		if portProcesses != nil {
			procs = portProcesses[item.port]
		}
		if err := a.releaseStartupPort(item.name, item.port, procs); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) releaseStartupPort(name string, port int, items []portProcess) error {
	if isTCPPortFree("127.0.0.1", port) {
		return nil
	}
	if len(items) == 0 {
		if waitTCPPortFreeStrict("127.0.0.1", port, 800*time.Millisecond) {
			return nil
		}
		return fmt.Errorf("%s端口 %d 已被占用，请在设置中修改端口或关闭占用该端口的程序", name, port)
	}
	a.logger.Info("%s端口 %d 被占用，开始处理占用进程", name, port)
	for _, item := range items {
		if !a.isOwnCoreProcess(item) {
			continue
		}
		a.logger.Info("发现本软件残留内核占用%s端口 %d：%s，开始强制清理", name, port, processLabel(item))
		if err := forceKillProcess(item.PID); err != nil {
			return fmt.Errorf("清理本软件残留进程失败：%s，%w", processLabel(item), err)
		}
	}
	if waitTCPPortFreeStrict("127.0.0.1", port, 800*time.Millisecond) {
		a.logger.Info("%s端口 %d 已释放", name, port)
		return nil
	}

	items, err := findTCPPortProcesses("127.0.0.1", port)
	if err != nil {
		return fmt.Errorf("重新查询%s端口 %d 占用进程失败：%w", name, port, err)
	}
	if len(items) == 0 {
		return fmt.Errorf("%s端口 %d 仍被占用，但未识别到占用进程", name, port)
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, processLabel(item))
	}
	return fmt.Errorf("%s端口 %d 被其他程序占用，为避免影响其他软件未强制结束进程。请关闭占用程序或修改端口。占用进程：%s", name, port, strings.Join(labels, "；"))
}

func (a *App) isOwnCoreProcess(item portProcess) bool {
	if a == nil || a.manager == nil {
		return false
	}
	return sameExecutablePath(item.Path, a.manager.corePath)
}

func (a *App) Stop() (err error) {
	defer a.recoverError("停止代理", &err)
	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return errProxyOperationBusy
	}
	defer a.proxyOpMu.Unlock()
	if a.manager == nil {
		return errors.New("内核管理器未初始化")
	}
	stopErr := a.stopProxyForUser()
	a.emitState()
	return stopErr
}

func (a *App) startProxyHealthWatcher() {
	ctx := a.ctx
	safeGo(a.logger, "代理健康检查", func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.ensureProxyHealthAsync("定时自检")
			case <-ctx.Done():
				return
			}
		}
	})
}

func (a *App) ensureProxyHealthAsync(reason string) {
	if a == nil || a.store == nil || a.manager == nil {
		return
	}
	a.healthMu.Lock()
	if !a.startupReady {
		a.healthMu.Unlock()
		return
	}
	if a.healthChecking {
		a.healthMu.Unlock()
		return
	}
	a.healthChecking = true
	a.healthMu.Unlock()
	safeGo(a.logger, "代理健康检查", func() {
		defer func() {
			a.healthMu.Lock()
			a.healthChecking = false
			a.healthMu.Unlock()
		}()
		a.ensureProxyHealth(reason)
	})
}

func (a *App) ensureProxyHealth(reason string) {
	state := a.store.Snapshot()
	normalizeSettings(&state.Settings)
	if !shouldRunProxy(state.Settings) || !stateHasSelectedNode(state) {
		return
	}
	if !a.manager.Running() {
		if !a.proxyOpMu.TryLock() {
			return
		}
		defer a.proxyOpMu.Unlock()
		state = a.store.Snapshot()
		normalizeSettings(&state.Settings)
		if !shouldRunProxy(state.Settings) || !stateHasSelectedNode(state) || a.manager.Running() {
			return
		}
		a.logger.Error("%s：代理入口仍开启，但内核未运行，正在自动恢复", reason)
		if err := a.startLocked(); err != nil {
			a.logger.Error("%s：自动恢复代理失败：%v", reason, err)
		}
		return
	}
	if err := a.manager.HealthCheck(state.Settings); err != nil {
		time.Sleep(500 * time.Millisecond)
		if retryErr := a.manager.HealthCheck(state.Settings); retryErr == nil {
			a.logger.Info("%s：代理健康检查瞬时异常已恢复，无需重启", reason)
			return
		} else {
			a.logger.Error("%s：代理健康检查连续失败：%v", reason, retryErr)
		}
	}
	if !a.proxyOpMu.TryLock() {
		return
	}
	defer a.proxyOpMu.Unlock()
	state = a.store.Snapshot()
	normalizeSettings(&state.Settings)
	if !shouldRunProxy(state.Settings) || !stateHasSelectedNode(state) {
		return
	}
	if !a.manager.Running() {
		a.logger.Error("%s：内核已停止，正在自动恢复", reason)
		if err := a.startLocked(); err != nil {
			a.logger.Error("%s：自动恢复代理失败：%v", reason, err)
		}
		return
	}
	if err := a.manager.HealthCheck(state.Settings); err == nil {
		if err := a.ensureProxyEntrancesLocked(state); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
		if retryErr := a.ensureProxyEntrancesLocked(state); retryErr == nil {
			a.logger.Info("%s：代理入口瞬时异常已恢复，无需重启", reason)
			return
		} else {
			a.logger.Error("%s：代理入口检查连续失败：%v", reason, retryErr)
			if errors.Is(retryErr, errSystemProxyEntrance) {
				a.logger.Error("%s：系统代理可能被其他软件持续覆盖，保持 sing-box 运行并等待下次自检", reason)
				return
			}
		}
	}
	if time.Since(a.lastHealthRestart) < 20*time.Second {
		a.logger.Error("%s：距离上次自动恢复过近，跳过本轮重启", reason)
		return
	}
	a.lastHealthRestart = time.Now()
	a.persistTraffic()
	if err := a.manager.Restart(state, effectiveTun(state.Settings)); err != nil {
		a.logger.Error("%s：自动重启代理失败：%v", reason, err)
		a.disableTunAfterStartFailure(state.Settings, err)
		a.emitState()
		return
	}
	a.markTunConnectivityHealthy(state.Settings)
	a.initTrafficBaseline(state.SelectedID, state.Nodes)
	a.logger.Info("%s：代理已自动恢复", reason)
	a.emitState()
}

func (a *App) ensureProxyEntrancesLocked(state PersistState) error {
	settings := state.Settings
	normalizeSettings(&settings)
	if runtime.GOOS == "windows" && settings.SystemProxy {
		issue, err := systemProxyConfigurationIssueForPort(settings.MixedPort)
		if err != nil {
			return fmt.Errorf("%w：%v", errSystemProxyEntrance, err)
		}
		if issue != "" {
			a.logger.Error("系统代理入口配置被清空或覆盖：%s，正在重新同步", issue)
			if err := EnableSystemProxy(settings.MixedPort); err != nil {
				return fmt.Errorf("%w：%v", errSystemProxyEntrance, err)
			}
			a.logger.Info("系统代理入口已自动重新同步：127.0.0.1:%d", settings.MixedPort)
		}
	}
	needsConnectivityCheck := (runtime.GOOS == "windows" && effectiveTun(settings)) || (settings.AutoFailover && shouldRunProxy(settings))
	if needsConnectivityCheck {
		if runtime.GOOS == "windows" && effectiveTun(settings) {
			if code, ok := tunReadyState(); !ok {
				return fmt.Errorf("TUN 未保持接管状态：%s", tunReadyMessage(code))
			}
		}
		if err := a.verifyTunConnectivityIfDue(settings.MixedPort); err != nil {
			if settings.AutoFailover {
				switchedName, failoverErr := a.autoFailoverLocked(state, err)
				if failoverErr == nil {
					a.logger.Info("【自动切换】已自动替换为同分组可用节点：%s", switchedName)
					a.markTunConnectivityHealthy(settings)
					return nil
				}
				a.logger.Error("【自动切换】节点替换尝试未成功：%v", failoverErr)
			}
			if settings.CircuitBreaker {
				a.setCircuitBreakerActive(true, "海外代理链路测试异常，已自动触发智能熔断保护")
				return nil
			}
			return fmt.Errorf("代理无法实际出网：%w", err)
		} else if settings.CircuitBreaker && a.isCircuitBreakerActive() {
			a.setCircuitBreakerActive(false, "")
		}
	}
	return nil
}

func filterAndRankCandidatesForAutoFailover(nodes []Node, groups []NodeGroup, currentID string) ([]Node, string, string, error) {
	if len(nodes) <= 1 {
		return nil, "", "", errors.New("没有其他备选节点可供切换")
	}
	var currentGroup string
	for _, n := range nodes {
		if n.ID == currentID {
			currentGroup = n.GroupID
			break
		}
	}

	normalizedGroup := normalizeGroupID(currentGroup)
	currentGroupName := "默认分组"
	for _, g := range groups {
		if normalizeGroupID(g.ID) == normalizedGroup {
			if strings.TrimSpace(g.Name) != "" {
				currentGroupName = g.Name
			}
			break
		}
	}

	var candidates []Node
	for _, n := range nodes {
		if n.ID == currentID || strings.TrimSpace(n.ID) == "" {
			continue
		}
		if normalizeGroupID(n.GroupID) == normalizedGroup {
			candidates = append(candidates, n)
		}
	}

	if len(candidates) == 0 {
		return nil, normalizedGroup, currentGroupName, fmt.Errorf("当前分组 [%s] 下没有其他备选节点可供自动切换（已限制只在同分组内切换）", currentGroupName)
	}

	// 优先按历史延迟排序：
	// 1. 历史延迟 > 0 的节点按延迟升序排在最前
	// 2. 延迟未测（0）居中
	// 3. 历史测试失败（< 0）垫后
	sort.SliceStable(candidates, func(i, j int) bool {
		di, dj := candidates[i].DelayMS, candidates[j].DelayMS
		if di > 0 && dj > 0 {
			return di < dj
		}
		if di > 0 && dj <= 0 {
			return true
		}
		if di <= 0 && dj > 0 {
			return false
		}
		if di == 0 && dj < 0 {
			return true
		}
		if di < 0 && dj == 0 {
			return false
		}
		return false
	})

	// 限制同分组内最多测试 16 个候选节点，防止过多节点并发或等待超时
	if len(candidates) > 16 {
		candidates = candidates[:16]
	}

	return candidates, normalizedGroup, currentGroupName, nil
}

func (a *App) autoFailoverLocked(state PersistState, triggerErr error) (string, error) {
	if a == nil || a.manager == nil || a.store == nil {
		return "", errors.New("服务未初始化")
	}
	now := time.Now()
	if !a.lastAutoFailover.IsZero() && now.Sub(a.lastAutoFailover) < 10*time.Second {
		return "", errors.New("距离上次自动故障切换时间过短，暂缓切换")
	}
	a.lastAutoFailover = now

	candidates, normalizedGroup, currentGroupName, err := filterAndRankCandidatesForAutoFailover(state.Nodes, state.Groups, state.SelectedID)
	if err != nil {
		return "", err
	}

	currentName := state.SelectedID
	for _, n := range state.Nodes {
		if n.ID == state.SelectedID {
			if strings.TrimSpace(n.Name) != "" {
				currentName = n.Name
			}
			break
		}
	}

	a.logger.Info("【自动切换检测】检测到当前节点 [%s] 断网或不可用（%v），正在测试同分组 [%s] 下 %d 个备选节点真实延迟...",
		currentName, triggerErr, currentGroupName, len(candidates))

	type probeResult struct {
		node    Node
		delayMS int
		err     error
	}

	numCandidates := len(candidates)
	taskCh := make(chan Node, numCandidates)
	resCh := make(chan probeResult, numCandidates)

	workerCount := 4
	if workerCount > numCandidates {
		workerCount = numCandidates
	}

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range taskCh {
				res, err := a.manager.TestNodeMetricWithProgressAndOptions(
					node,
					"delay",
					nil,
					SpeedTestOptions{
						DirectOnly:    true,
						FastDelay:     true,
						CustomTimeout: 2500 * time.Millisecond,
					},
				)
				if err == nil && res.DelayMS > 0 {
					resCh <- probeResult{node: node, delayMS: res.DelayMS, err: nil}
				} else {
					resCh <- probeResult{node: node, delayMS: 0, err: err}
				}
			}
		}()
	}

	for _, c := range candidates {
		taskCh <- c
	}
	close(taskCh)

	wg.Wait()
	close(resCh)

	var workingNodes []probeResult
	for r := range resCh {
		if r.err == nil && r.delayMS > 0 {
			workingNodes = append(workingNodes, r)
		}
	}

	if len(workingNodes) == 0 {
		return "", fmt.Errorf("当前分组 [%s] 下已测试 %d 个候选节点，但均未连通（可能该分组节点全部失效或本机网络已中断）", currentGroupName, len(candidates))
	}

	// 核心：严格按本次真实测试延迟从小到大排序，优先分配同分组下延迟最快的节点
	sort.Slice(workingNodes, func(i, j int) bool {
		return workingNodes[i].delayMS < workingNodes[j].delayMS
	})

	for _, w := range workingNodes {
		if err := a.selectNodeLocked(w.node.ID); err != nil {
			a.logger.Error("【自动切换】尝试切换至同分组候选节点 [%s] 失败：%v", w.node.Name, err)
			continue
		}
		_ = a.manager.CloseConnections()
		time.Sleep(200 * time.Millisecond)
		if shouldRunProxy(state.Settings) {
			if err := verifyLocalProxyConnectivity(state.Settings.MixedPort, 2500*time.Millisecond); err != nil {
				a.logger.Error("【自动切换】切换至候选节点 [%s]（延迟 %d ms）后网络校验未通过：%v，继续尝试同分组下一个延迟最低节点", w.node.Name, w.delayMS, err)
				continue
			}
		}

		a.logger.Info("【自动切换成功】原节点 [%s] 异常，已自动无缝切换至同分组 [%s] 最低延迟节点 [%s]（真连接延迟 %d ms），代理稳定运行中",
			currentName, currentGroupName, w.node.Name, w.delayMS)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "auto-failover-switched", map[string]interface{}{
				"from":      currentName,
				"to":        w.node.Name,
				"nodeId":    w.node.ID,
				"delayMs":   w.delayMS,
				"groupId":   normalizedGroup,
				"groupName": currentGroupName,
			})
		}
		return w.node.Name, nil
	}

	return "", fmt.Errorf("当前分组 [%s] 测通的候选节点切换后均未能通过本地网络校验", currentGroupName)
}

func (a *App) Restart() (err error) {
	defer a.recoverError("重启代理", &err)
	if !a.lockProxyOperation(saveSettingsLockTimeout) {
		return errProxyOperationBusy
	}
	defer a.proxyOpMu.Unlock()
	if a.manager == nil {
		return errors.New("内核管理器未初始化")
	}
	a.logger.Info("开始自检并重启代理")
	if a.manager.Running() {
		a.persistTraffic()
		if err := a.manager.Stop(); err != nil {
			a.logger.Error("重启前停止旧内核失败：%v", err)
			a.emitState()
			return err
		}
		a.resetTrafficTracking()
	}
	if err := a.startLocked(); err != nil {
		a.logger.Error("自检重启代理失败：%v", err)
		return err
	}
	a.setCircuitBreakerActive(false, "")
	a.logger.Info("自检重启代理完成")
	return nil
}

func (a *App) initTrafficBaseline(selectedID string, nodes []Node) {
	if a.manager == nil || !a.manager.Running() || selectedID == "" {
		a.resetTrafficTracking()
		return
	}
	traffic, err := a.manager.TrafficSnapshot()
	if err != nil {
		a.resetTrafficTracking()
		return
	}
	groupID := defaultGroupID
	for _, node := range nodes {
		if node.ID == selectedID {
			groupID = node.GroupID
			if groupID == "" {
				groupID = defaultGroupID
			}
			break
		}
	}
	a.trafficMu.Lock()
	a.traffic = trafficTrack{
		NodeID:        selectedID,
		GroupID:       groupID,
		UploadTotal:   traffic.UploadTotal,
		DownloadTotal: traffic.DownloadTotal,
	}
	a.trafficSnapshot = traffic
	a.trafficMu.Unlock()
}

func (a *App) emitState() {
	defer recoverPanic(a.logger, "发送状态事件")
	a.refreshTrayMenu()
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "state-changed")
	}
}

func (a *App) onSystemSuspend() {
	defer recoverPanic(a.logger, "处理系统挂起事件")
	a.stopResumeHealthChecks()
	if a.logger != nil {
		a.logger.Info("系统即将挂起，暂停界面高频刷新")
	}
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "system-suspend")
	}
}

func (a *App) onSystemResume() {
	defer recoverPanic(a.logger, "处理系统恢复事件")
	if a.logger != nil {
		a.logger.Info("系统已恢复，准备刷新界面状态并校验代理入口")
	}
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "system-resume")
	}
	if a.manager != nil && a.manager.Running() {
		_ = a.manager.CloseConnections()
	}
	a.ensureProxyHealthAsync("系统恢复自检")
	a.scheduleResumeHealthChecks()
}

func (a *App) scheduleResumeHealthChecks() {
	a.resumeCheckMu.Lock()
	defer a.resumeCheckMu.Unlock()
	for index, timer := range a.resumeCheckTimers {
		if timer != nil {
			timer.Stop()
		}
		a.resumeCheckTimers[index] = nil
	}
	delays := []time.Duration{3 * time.Second, 12 * time.Second}
	reasons := []string{"系统恢复延迟自检", "系统恢复稳定复检"}
	for index := range a.resumeCheckTimers {
		reason := reasons[index]
		a.resumeCheckTimers[index] = time.AfterFunc(delays[index], func() {
			defer recoverPanic(a.logger, reason)
			a.ensureProxyHealthAsync(reason)
		})
	}
}

func (a *App) stopResumeHealthChecks() {
	a.resumeCheckMu.Lock()
	defer a.resumeCheckMu.Unlock()
	for index, timer := range a.resumeCheckTimers {
		if timer != nil {
			timer.Stop()
		}
		a.resumeCheckTimers[index] = nil
	}
}

func (a *App) emitSpeedProgress(progress SpeedProgress) {
	defer recoverPanic(a.logger, "发送测速进度")
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "speed-progress", progress)
	}
}

func (a *App) coreVersion() string {
	if a == nil || a.manager == nil {
		return "未安装"
	}
	return a.manager.CoreVersion()
}

func (a *App) coreUpdateStatus() (string, bool) {
	if a == nil || a.manager == nil {
		return "", false
	}
	return a.manager.CoreUpdateStatus()
}

func (a *App) logs() []LogMessage {
	if a == nil || a.logger == nil {
		return nil
	}
	return a.logger.Logs()
}

func (a *App) Logs() (result []LogMessage) {
	defer recoverPanic(a.logger, "读取完整日志")
	return a.logs()
}

func (a *App) stateLogsPreview() []LogMessage {
	logs := a.logs()
	const stateLogPreviewLimit = 160
	if len(logs) <= stateLogPreviewLimit {
		return logs
	}
	return append([]LogMessage(nil), logs[len(logs)-stateLogPreviewLimit:]...)
}

func findNodeByID(nodes []Node, id string) (Node, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func hasRestartArg() bool {
	for _, arg := range os.Args[1:] {
		if arg == adminRestartArg {
			return true
		}
	}
	return false
}
