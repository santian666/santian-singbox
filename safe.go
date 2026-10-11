package main

import (
	"errors"
	"fmt"
	"log"
	"runtime/debug"
)

func safeGo(logger *Logger, name string, fn func()) {
	go func() {
		defer recoverPanic(logger, name)
		fn()
	}()
}

func recoverPanic(logger *Logger, name string) {
	if value := recover(); value != nil {
		logRecoveredPanic(logger, name, value)
	}
}

func (a *App) recoverError(name string, err *error) {
	if value := recover(); value != nil {
		logRecoveredPanic(appLogger(a), name, value)
		if err != nil {
			*err = errors.New("程序异常已拦截：" + name)
		}
	}
}

func (a *App) recoverState(name string, state *RuntimeState) {
	if value := recover(); value != nil {
		logRecoveredPanic(appLogger(a), name, value)
		if state != nil {
			*state = a.fallbackState()
		}
	}
}

func (a *App) fallbackState() RuntimeState {
	settings := defaultSettings()
	groups := defaultGroups()
	logs := []LogMessage{}
	running := false
	coreVersion := "未安装"
	if a != nil {
		if a.logger != nil {
			logs = a.logger.Logs()
		}
		if a.manager != nil {
			running = a.manager.Running()
			coreVersion = a.manager.CoreVersion()
		}
	}
	return RuntimeState{
		Running:     running,
		AppVersion:  AppVersion,
		CoreVersion: coreVersion,
		ListenURL:   "127.0.0.1",
		Settings:    settings,
		RulePresets: RulePresets(),
		Groups:      groups,
		Logs:        logs,
		ConfigMode:  ConfigMode{Mode: settings.Mode},
	}
}

func appLogger(a *App) *Logger {
	if a == nil {
		return nil
	}
	return a.logger
}

func logRecoveredPanic(logger *Logger, name string, value interface{}) {
	message := fmt.Sprintf("程序异常已拦截：%s，%v\n%s", name, value, string(debug.Stack()))
	if logger != nil {
		logger.Error("%s", message)
		return
	}
	log.Print(message)
}
