//go:build !windows

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func (a *App) startShutdownSignalWatcher() {
	var once sync.Once
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	safeGo(a.logger, "系统退出信号监听", func() {
		sig := <-signals
		reason := "系统退出信号"
		if sig != nil {
			reason += "：" + sig.String()
		}
		once.Do(func() {
			a.shutdownForSignal(reason)
		})
	})
}
