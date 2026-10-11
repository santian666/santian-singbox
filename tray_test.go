package main

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTrayProxyCommandKeepsStartClickedDuringStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	commands := make(chan trayProxyCommand, 1)
	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	executed := make(chan trayProxyCommand, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		processTrayProxyCommands(ctx, commands, func(command trayProxyCommand) {
			executed <- command
			if command == trayProxyStop {
				close(stopStarted)
				<-releaseStop
			}
		})
	}()

	enqueueTrayProxyCommand(commands, trayProxyStop)
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("停止代理命令未开始执行")
	}
	enqueueTrayProxyCommand(commands, trayProxyStart)
	close(releaseStop)

	for index, want := range []trayProxyCommand{trayProxyStop, trayProxyStart} {
		select {
		case got := <-executed:
			if got != want {
				t.Fatalf("第 %d 个托盘命令错误：want=%v got=%v", index+1, want, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("第 %d 个托盘命令未执行", index+1)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("托盘命令处理未退出")
	}
}

func TestTrayProxyCommandKeepsLatestPendingIntent(t *testing.T) {
	commands := make(chan trayProxyCommand, 1)
	enqueueTrayProxyCommand(commands, trayProxyStart)
	enqueueTrayProxyCommand(commands, trayProxyStop)

	select {
	case got := <-commands:
		if got != trayProxyStop {
			t.Fatalf("应保留最后一次托盘操作：want=%v got=%v", trayProxyStop, got)
		}
	default:
		t.Fatal("托盘命令未入队")
	}
}

func TestTrayProxyCommandKeepsIndependentPendingIntents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands := make(chan trayProxyCommand, trayProxyCommandQueueSize)
	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	executed := make(chan trayProxyCommand, 3)
	go processTrayProxyCommands(ctx, commands, func(command trayProxyCommand) {
		executed <- command
		if command == trayProxyStop {
			close(stopStarted)
			<-releaseStop
		}
	})

	enqueueTrayProxyCommand(commands, trayProxyStop)
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("停止代理命令未开始执行")
	}
	enqueueTrayProxyCommand(commands, trayProxyTun)
	enqueueTrayProxyCommand(commands, trayProxyRule)
	close(releaseStop)

	for index, want := range []trayProxyCommand{trayProxyStop, trayProxyTun, trayProxyRule} {
		select {
		case got := <-executed:
			if got != want {
				t.Fatalf("第 %d 个托盘命令错误：want=%v got=%v", index+1, want, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("第 %d 个托盘命令未执行", index+1)
		}
	}
}

func TestTrayProxyOperationRetriesBusyOnce(t *testing.T) {
	app := &App{}
	calls := 0
	err := app.runTrayProxyOperation(func() error {
		calls++
		if calls == 1 {
			return errProxyOperationBusy
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("托盘忙碌操作未正确重试：calls=%d err=%v", calls, err)
	}
}

func TestTrayProxyCommandQueueStressKeepsFinalIntent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands := make(chan trayProxyCommand, trayProxyCommandQueueSize)
	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	finalExecuted := make(chan struct{})
	go processTrayProxyCommands(ctx, commands, func(command trayProxyCommand) {
		if command == trayProxyStop {
			close(stopStarted)
			<-releaseStop
		}
		if command == trayProxyStart {
			close(finalExecuted)
		}
	})

	enqueueTrayProxyCommand(commands, trayProxyStop)
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("停止代理命令未开始执行")
	}
	for index := 0; index < 10_000; index++ {
		command := trayProxySystem
		if index%2 == 1 {
			command = trayProxyGlobal
		}
		enqueueTrayProxyCommand(commands, command)
	}
	enqueueTrayProxyCommand(commands, trayProxyStart)
	close(releaseStop)
	select {
	case <-finalExecuted:
	case <-time.After(2 * time.Second):
		t.Fatal("高频托盘操作后最后一次启动意图丢失")
	}
}

func TestProxySettingsForStartRestoresStoppedSystemProxy(t *testing.T) {
	app := &App{}
	previous := defaultSettings()
	previous.Mode = "rule"
	previous.SystemProxy = true
	app.rememberProxyEntries(previous)

	stopped := previous
	stopped.SystemProxy = false
	stopped.Mode = "global"
	got, restored := app.proxySettingsForStart(stopped)
	if !restored {
		t.Fatal("应恢复停止前的系统代理入口")
	}
	if !got.SystemProxy || got.Tun {
		t.Fatalf("恢复后的代理入口错误：%+v", got)
	}
	if got.Mode != "global" {
		t.Fatalf("恢复入口不应覆盖停止后选择的分流模式：%+v", got)
	}
}

func TestProxySettingsForStartRestoresStoppedTunExclusively(t *testing.T) {
	app := &App{}
	previous := defaultSettings()
	previous.SystemProxy = true
	previous.Tun = true
	app.rememberProxyEntries(previous)

	got, restored := app.proxySettingsForStart(defaultSettings())
	if !restored {
		t.Fatal("应恢复停止前的 TUN 代理入口")
	}
	if !got.Tun || got.SystemProxy {
		t.Fatalf("TUN 与系统代理必须保持二选一：%+v", got)
	}
}

func TestProxySettingsForStartKeepsLastSnapshotAfterRepeatedStop(t *testing.T) {
	app := &App{}
	previous := defaultSettings()
	previous.Tun = true
	app.rememberProxyEntries(previous)
	app.rememberProxyEntries(defaultSettings())

	got, restored := app.proxySettingsForStart(defaultSettings())
	if !restored || !got.Tun {
		t.Fatalf("重复停止不应清除最后一次有效代理模式：%+v", got)
	}
}

func TestProxySettingsForStartDefaultsToSystemProxy(t *testing.T) {
	app := &App{}
	settings := defaultSettings()
	settings.Mode = "rule"
	got, restored := app.proxySettingsForStart(settings)
	if restored {
		t.Fatal("没有停止快照时不应报告已恢复")
	}
	if !got.SystemProxy || got.Tun || got.Mode != "rule" {
		t.Fatalf("默认启动应使用系统代理并保留分流模式：%+v", got)
	}
}

func TestTruncateTrayTextKeepsUTF8Valid(t *testing.T) {
	got := truncateTrayText("香港高速代理节点-01", 8)
	if !utf8.ValidString(got) {
		t.Fatalf("托盘节点名截断后不是有效 UTF-8：%q", got)
	}
	if utf8.RuneCountInString(got) != 8 || got[len(got)-3:] != "…" {
		t.Fatalf("托盘节点名截断长度错误：%q", got)
	}
}

func TestTrayProxyEntranceLabelShowsActiveEntrances(t *testing.T) {
	settings := defaultSettings()
	settings.SystemProxy = true
	settings.ProxyMap = true
	if got := trayProxyEntranceLabel(settings); got != "系统代理 + 代理映射" {
		t.Fatalf("托盘代理入口显示错误：%q", got)
	}
}
