package main

import (
	"context"
	_ "embed"
	"errors"
	"runtime"
	"strings"
	"time"

	"github.com/getlantern/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const trayProxyCommandQueueSize = 16

//go:embed build/windows/icon.ico
var trayIcon []byte

type trayProxyCommand uint8

const (
	trayProxyStop trayProxyCommand = iota
	trayProxyStart
	trayProxySystem
	trayProxyTun
	trayProxyGlobal
	trayProxyRule
)

type trayMenuItems struct {
	currentNode      *systray.MenuItem
	status           *systray.MenuItem
	start            *systray.MenuItem
	stop             *systray.MenuItem
	systemProxy      *systray.MenuItem
	tun              *systray.MenuItem
	global           *systray.MenuItem
	rule             *systray.MenuItem
	currentNodeTitle string
	statusTitle      string
	tooltip          string
}

func (a *App) startTray() {
	safeGo(a.logger, "系统托盘", func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		a.logger.Info("系统托盘初始化中")
		systray.SetClickHandler(func() {
			safeGo(a.logger, "托盘图标点击", func() {
				a.logger.Info("托盘图标点击：显示主窗口")
				a.showWindow()
			})
		})
		systray.Run(a.onTrayReady, func() {
			a.logger.Info("系统托盘已退出")
		})
		a.logger.Info("系统托盘消息循环已结束")
	})
}

func (a *App) stopTray() {
	a.trayMu.Lock()
	a.trayItems = nil
	a.trayMu.Unlock()
	systray.Quit()
}

func (a *App) onTrayReady() {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("SanTian-Singbox")
	a.logger.Info("系统托盘初始化完成")

	currentNodeItem := systray.AddMenuItem("当前节点：未选择", "当前选中的代理节点")
	currentNodeItem.Disable()
	statusItem := systray.AddMenuItem("代理状态：已停止", "当前代理入口和分流模式")
	statusItem.Disable()
	systray.AddSeparator()
	startItem := systray.AddMenuItem("启动代理", "恢复停止前使用的代理模式")
	stopItem := systray.AddMenuItem("停止代理", "关闭全部代理入口")
	systray.AddSeparator()
	systemProxyItem := systray.AddMenuItemCheckbox("系统代理", "使用 Windows 系统代理", false)
	tunItem := systray.AddMenuItemCheckbox("TUN 代理", "使用 TUN 虚拟网卡代理", false)
	globalItem := systray.AddMenuItemCheckbox("全局模式", "全部流量使用代理", false)
	ruleItem := systray.AddMenuItemCheckbox("规则模式", "根据规则智能分流", false)
	systray.AddSeparator()
	showItem := systray.AddMenuItem("显示窗口", "显示 SanTian-Singbox 主窗口")
	hideItem := systray.AddMenuItem("隐藏窗口", "隐藏到系统托盘")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出", "退出 SanTian-Singbox")
	a.trayMu.Lock()
	a.trayItems = &trayMenuItems{
		currentNode: currentNodeItem,
		status:      statusItem,
		start:       startItem,
		stop:        stopItem,
		systemProxy: systemProxyItem,
		tun:         tunItem,
		global:      globalItem,
		rule:        ruleItem,
	}
	a.trayMu.Unlock()
	a.refreshTrayMenu()

	proxyCommands := make(chan trayProxyCommand, trayProxyCommandQueueSize)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	safeGo(a.logger, "系统托盘代理操作", func() {
		processTrayProxyCommands(ctx, proxyCommands, func(command trayProxyCommand) {
			a.executeTrayProxyCommand(command)
		})
	})

	safeGo(a.logger, "系统托盘菜单", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-showItem.ClickedCh:
				a.showWindow()
			case <-hideItem.ClickedCh:
				a.hideWindow()
			case <-startItem.ClickedCh:
				a.logger.Info("托盘操作：已收到启动代理请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxyStart)
			case <-stopItem.ClickedCh:
				a.logger.Info("托盘操作：已收到停止代理请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxyStop)
			case <-systemProxyItem.ClickedCh:
				a.logger.Info("托盘操作：已收到系统代理请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxySystem)
			case <-tunItem.ClickedCh:
				a.logger.Info("托盘操作：已收到 TUN 代理请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxyTun)
			case <-globalItem.ClickedCh:
				a.logger.Info("托盘操作：已收到全局模式请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxyGlobal)
			case <-ruleItem.ClickedCh:
				a.logger.Info("托盘操作：已收到规则模式请求")
				enqueueTrayProxyCommand(proxyCommands, trayProxyRule)
			case <-quitItem.ClickedCh:
				a.quitFromTray()
				return
			}
		}
	})
}

func enqueueTrayProxyCommand(commands chan trayProxyCommand, command trayProxyCommand) {
	select {
	case commands <- command:
		return
	default:
	}
	select {
	case <-commands:
	default:
	}
	select {
	case commands <- command:
	default:
	}
}

func processTrayProxyCommands(ctx context.Context, commands <-chan trayProxyCommand, execute func(trayProxyCommand)) {
	for {
		select {
		case <-ctx.Done():
			return
		case command, ok := <-commands:
			if !ok {
				return
			}
			execute(command)
		}
	}
}

func (a *App) executeTrayProxyCommand(command trayProxyCommand) {
	switch command {
	case trayProxyStart:
		a.trayStart()
	case trayProxyStop:
		a.trayStop()
	case trayProxySystem, trayProxyTun, trayProxyGlobal, trayProxyRule:
		a.trayApplySettings(command)
	}
}

func (a *App) trayApplySettings(command trayProxyCommand) {
	if a.store == nil {
		a.logger.Error("托盘切换失败：配置存储未初始化")
		return
	}
	settings := a.store.Snapshot().Settings
	switch command {
	case trayProxySystem:
		settings.SystemProxy = true
		settings.Tun = false
	case trayProxyTun:
		settings.SystemProxy = false
		settings.Tun = true
	case trayProxyGlobal:
		settings.Mode = "global"
	case trayProxyRule:
		settings.Mode = "rule"
	default:
		return
	}

	if err := a.runTrayProxyOperation(func() error { return a.SaveSettings(settings) }); err != nil {
		if command == trayProxyTun && err.Error() == adminRestartRequiredTun {
			if restartErr := a.RestartTunAsAdmin(settings); restartErr != nil {
				a.logger.Error("托盘切换 TUN 失败：%v", restartErr)
				a.refreshTrayMenu()
			}
			return
		}
		a.logger.Error("托盘切换代理失败：%v", err)
		a.refreshTrayMenu()
		return
	}
	a.logger.Info("托盘代理模式切换完成")
	a.refreshTrayMenu()
}

func (a *App) refreshTrayMenu() {
	if a == nil || a.store == nil {
		return
	}
	state := a.store.Snapshot()
	normalizeSettings(&state.Settings)
	running := a.manager != nil && a.manager.Running()
	nodeName := selectedNodeLogName(state.Nodes, state.SelectedID)
	if strings.TrimSpace(state.SelectedID) == "" || strings.TrimSpace(nodeName) == "" {
		nodeName = "未选择"
	}
	nodeName = truncateTrayText(nodeName, 28)
	entrance := trayProxyEntranceLabel(state.Settings)
	mode := trayProxyModeLabel(state.Settings.Mode)
	status := "代理状态：已停止"
	if a.isCircuitBreakerActive() {
		status = "代理状态：⚠️ 安全熔断保护中（国内正常通，外网已拦截）"
	} else if running && entrance != "" {
		status = "代理状态：" + entrance + " · " + mode
	} else if running {
		status = "代理状态：内核运行 · 入口已关闭"
	} else if shouldRunProxy(state.Settings) {
		status = "代理状态：正在连接 · " + entrance + " · " + mode
	}
	currentNodeTitle := "当前节点：" + nodeName
	tooltip := "SanTian-Singbox | " + strings.TrimPrefix(status, "代理状态：") + " | " + nodeName

	a.trayMu.Lock()
	defer a.trayMu.Unlock()
	items := a.trayItems
	if items == nil {
		return
	}
	setTrayMenuTitle(items.currentNode, &items.currentNodeTitle, currentNodeTitle)
	setTrayMenuTitle(items.status, &items.statusTitle, status)
	setTrayMenuEnabled(items.start, !running)
	setTrayMenuEnabled(items.stop, running || shouldRunProxy(state.Settings))
	setTrayMenuChecked(items.systemProxy, state.Settings.SystemProxy)
	setTrayMenuChecked(items.tun, state.Settings.Tun)
	setTrayMenuChecked(items.global, state.Settings.Mode == "global")
	setTrayMenuChecked(items.rule, state.Settings.Mode == "rule")
	if items.tooltip != tooltip {
		systray.SetTooltip(tooltip)
		items.tooltip = tooltip
	}
}

func trayProxyEntranceLabel(settings Settings) string {
	labels := make([]string, 0, 4)
	if settings.SystemProxy {
		labels = append(labels, "系统代理")
	}
	if settings.Tun {
		labels = append(labels, "TUN 代理")
	}
	if settings.ProxyMap {
		labels = append(labels, "代理映射")
	}
	if appProxyActive(settings) {
		labels = append(labels, "应用代理")
	}
	return strings.Join(labels, " + ")
}

func trayProxyModeLabel(mode string) string {
	if mode == "rule" {
		return "规则模式"
	}
	return "全局模式"
}

func truncateTrayText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

func setTrayMenuTitle(item *systray.MenuItem, current *string, title string) {
	if item == nil || *current == title {
		return
	}
	item.SetTitle(title)
	*current = title
}

func setTrayMenuEnabled(item *systray.MenuItem, enabled bool) {
	if item == nil || item.Disabled() == !enabled {
		return
	}
	if enabled {
		item.Enable()
		return
	}
	item.Disable()
}

func setTrayMenuChecked(item *systray.MenuItem, checked bool) {
	if item == nil || item.Checked() == checked {
		return
	}
	if checked {
		item.Check()
		return
	}
	item.Uncheck()
}

func (a *App) showWindow() {
	defer recoverPanic(a.logger, "显示主窗口")
	if a.ctx == nil {
		a.logger.Error("显示主窗口失败：窗口上下文未就绪")
		return
	}
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowCenter(a.ctx)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
	a.logger.Info("主窗口已请求显示")
}

func (a *App) hideWindow() {
	defer recoverPanic(a.logger, "隐藏主窗口")
	if a.ctx == nil {
		a.logger.Error("隐藏主窗口失败：窗口上下文未就绪")
		return
	}
	wailsRuntime.WindowHide(a.ctx)
}

func (a *App) trayStart() {
	a.logger.Info("托盘操作：开始启动代理")
	if err := a.runTrayProxyOperation(a.Start); err != nil {
		a.logger.Error("托盘启动失败：%v", err)
		a.refreshTrayMenu()
		return
	}
	a.logger.Info("托盘操作：代理启动完成")
}

func (a *App) trayStop() {
	a.logger.Info("托盘操作：开始停止代理")
	if err := a.runTrayProxyOperation(a.Stop); err != nil {
		a.logger.Error("托盘停止失败：%v", err)
		a.refreshTrayMenu()
		return
	}
	a.logger.Info("托盘操作：代理停止完成")
}

func (a *App) runTrayProxyOperation(operation func() error) error {
	err := operation()
	if !isTrayProxyOperationBusy(err) {
		return err
	}
	if a.logger != nil {
		a.logger.Info("托盘操作：代理正在执行其他任务，等待后自动重试")
	}
	time.Sleep(250 * time.Millisecond)
	return operation()
}

func isTrayProxyOperationBusy(err error) bool {
	return errors.Is(err, errProxyOperationBusy) ||
		(err != nil && strings.Contains(err.Error(), "代理正在"))
}

func (a *App) quitFromTray() {
	a.requestQuit("托盘退出")
}
