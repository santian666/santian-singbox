package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerListenerRunsOutsideLock(t *testing.T) {
	logger := NewLogger(filepath.Join(t.TempDir(), "app.log"))
	defer logger.Close()
	notified := make(chan LogMessage, 1)
	logger.SetListener(func(message LogMessage) {
		_ = logger.Logs()
		notified <- message
	})
	logger.Info("实时日志")
	select {
	case message := <-notified:
		if message.Text != "实时日志" || message.Level != "INFO" {
			t.Fatalf("日志监听内容错误：%+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("日志监听器未收到消息或发生锁等待")
	}
}

func TestLoggerErrorFlushesToDiskImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	logger := NewLogger(path)
	defer logger.Close()
	logger.Error("系统代理双层校验失败")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "系统代理双层校验失败") {
		t.Fatalf("ERROR 日志未立即写入文件：%q", string(data))
	}
}

func TestLoggerBufferKeepsBoundedTail(t *testing.T) {
	logger := NewLogger(filepath.Join(t.TempDir(), "app.log"))
	defer logger.Close()
	for index := 0; index < logBufferLimit+100; index++ {
		logger.Info("日志 %d", index)
	}
	logs := logger.Logs()
	if len(logs) != logBufferLimit {
		t.Fatalf("日志缓存数量未受限：got %d want %d", len(logs), logBufferLimit)
	}
}

func TestLoggerRotatesWhileRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	logger := NewLogger(path)
	defer logger.Close()
	logger.RotateIfLarge(256)
	for index := 0; index < 40; index++ {
		logger.Info("持续运行日志 %d：%s", index, strings.Repeat("x", 32))
	}
	logger.Error("轮转后的最新错误")

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("运行期间未生成轮转备份：%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "轮转后的最新错误") {
		t.Fatalf("轮转后当前日志缺少最新内容：%q", string(data))
	}
}
