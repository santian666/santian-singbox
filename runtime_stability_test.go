package main

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProxyMapPortChecksCoverMiddlePort(t *testing.T) {
	listeners := make([]net.Listener, 0, 3)
	ports := make([]int, 0, 3)
	for range 3 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("创建测试监听端口失败：%v", err)
		}
		listeners = append(listeners, listener)
		port, err := strconv.Atoi(strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:"))
		if err != nil {
			t.Fatalf("读取测试监听端口失败：%v", err)
		}
		ports = append(ports, port)
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	if port := firstUnboundTCPPort("127.0.0.1", ports); port != 0 {
		t.Fatalf("全部端口监听时不应报告异常端口，实际：%d", port)
	}
	if err := listeners[1].Close(); err != nil {
		t.Fatalf("关闭中间测试端口失败：%v", err)
	}
	if port := firstUnboundTCPPort("127.0.0.1", ports); port != ports[1] {
		t.Fatalf("应检测到中间端口 %d，实际：%d", ports[1], port)
	}
}

func TestRunCommandOutputTimeout(t *testing.T) {
	if os.Getenv("SANTIAN_COMMAND_TIMEOUT_HELPER") == "1" {
		time.Sleep(3 * time.Second)
		return
	}

	started := time.Now()
	_, err := runCommandOutputWithEnvTimeout(
		150*time.Millisecond,
		map[string]string{"SANTIAN_COMMAND_TIMEOUT_HELPER": "1"},
		os.Args[0],
		"-test.run=^TestRunCommandOutputTimeout$",
	)
	if err == nil || !strings.Contains(err.Error(), "命令执行超时") {
		t.Fatalf("应返回命令执行超时，实际：%v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("命令超时返回过慢：%s", elapsed)
	}
}

func TestWaitOutputReadersTimeout(t *testing.T) {
	done := make(chan struct{}, 2)
	done <- struct{}{}
	done <- struct{}{}
	if !waitOutputReaders(done, 2, 100*time.Millisecond) {
		t.Fatal("已完成的输出读取不应超时")
	}

	started := time.Now()
	if waitOutputReaders(make(chan struct{}), 1, 50*time.Millisecond) {
		t.Fatal("未完成的输出读取应返回超时")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("输出读取超时返回过慢：%s", elapsed)
	}
}

func TestSwitchOutboundThenCommitSwitchesBeforeCommit(t *testing.T) {
	actual := "old"
	order := make([]string, 0, 2)
	err := switchOutboundThenCommit(
		"new",
		func() (string, error) { return actual, nil },
		func(tag string) error {
			order = append(order, "switch:"+tag)
			actual = tag
			return nil
		},
		func() error {
			order = append(order, "commit")
			if actual != "new" {
				t.Fatalf("提交前实际出站节点应已切换，实际：%s", actual)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("切换节点失败：%v", err)
	}
	if got := strings.Join(order, ","); got != "switch:new,commit" {
		t.Fatalf("节点切换顺序错误：%s", got)
	}
}

func TestSwitchOutboundThenCommitDoesNotCommitWhenAPISelectionFails(t *testing.T) {
	committed := false
	err := switchOutboundThenCommit(
		"new",
		func() (string, error) { return "old", nil },
		func(string) error { return errors.New("selector 未切换") },
		func() error {
			committed = true
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "Clash API 切换失败") {
		t.Fatalf("应返回 Clash API 切换失败，实际：%v", err)
	}
	if committed {
		t.Fatal("Clash API 切换失败时不应提交已选择状态")
	}
}

func TestSwitchOutboundThenCommitRollsBackWhenCommitFails(t *testing.T) {
	actual := "old"
	err := switchOutboundThenCommit(
		"new",
		func() (string, error) { return actual, nil },
		func(tag string) error {
			actual = tag
			return nil
		},
		func() error { return errors.New("磁盘写入失败") },
	)
	if err == nil || !strings.Contains(err.Error(), "实际出站已回滚") {
		t.Fatalf("应返回保存失败并说明已回滚，实际：%v", err)
	}
	if actual != "old" {
		t.Fatalf("保存失败后应恢复原出站节点，实际：%s", actual)
	}
}

func TestConfirmOutboundSelectionSendsCommandOnceAndPolls(t *testing.T) {
	selectCount := 0
	readCount := 0
	err := confirmOutboundSelection(
		"new",
		func() error {
			selectCount++
			return nil
		},
		func() (string, error) {
			readCount++
			if readCount < 3 {
				return "old", nil
			}
			return "new", nil
		},
		200*time.Millisecond,
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("selector 延迟更新不应判定失败：%v", err)
	}
	if selectCount != 1 {
		t.Fatalf("切换命令发送次数 = %d，期望 1", selectCount)
	}
}

func TestConfirmOutboundSelectionAcceptsAppliedCommandAfterEOF(t *testing.T) {
	err := confirmOutboundSelection(
		"new",
		func() error { return io.EOF },
		func() (string, error) { return "new", nil },
		50*time.Millisecond,
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("命令返回 EOF 但 selector 已切换时应成功：%v", err)
	}
}

func TestRetryableClashAPIErrorsIncludeTransientFailures(t *testing.T) {
	for _, err := range []error{
		io.EOF,
		errors.New("context deadline exceeded"),
		errors.New("An existing connection was forcibly closed by the remote host"),
	} {
		if !isRetryableClashAPIError(err) {
			t.Fatalf("Clash API 瞬时错误应允许重试：%v", err)
		}
	}
}

func TestLockProxyOperationTimeout(t *testing.T) {
	app := &App{}
	app.proxyOpMu.Lock()
	started := time.Now()
	if app.lockProxyOperation(50 * time.Millisecond) {
		t.Fatal("代理操作锁被占用时不应获取成功")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("等待代理操作锁返回过慢：%s", elapsed)
	}
	app.proxyOpMu.Unlock()

	if !app.lockProxyOperation(50 * time.Millisecond) {
		t.Fatal("代理操作锁空闲时应获取成功")
	}
	app.proxyOpMu.Unlock()
}

func TestOwnCoreProcessRequiresExactPath(t *testing.T) {
	root := t.TempDir()
	ownCore := filepath.Join(root, "bin", "sing-box.exe")
	otherCore := filepath.Join(root, "v2rayN", "sing-box.exe")
	app := &App{manager: NewSingBoxManager(root, ownCore, "", "", nil)}

	if !app.isOwnCoreProcess(portProcess{PID: 100, Name: "sing-box.exe", Path: ownCore}) {
		t.Fatal("完整路径一致时应识别为本软件内核")
	}
	if app.isOwnCoreProcess(portProcess{PID: 101, Name: "sing-box.exe", Path: otherCore}) {
		t.Fatal("其他软件的同名 sing-box 不应识别为本软件内核")
	}
	if app.isOwnCoreProcess(portProcess{PID: 102, Name: "sing-box.exe"}) {
		t.Fatal("无法读取路径时不得仅按进程名判定为本软件内核")
	}
}
