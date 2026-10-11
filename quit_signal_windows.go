package main

import (
	"golang.org/x/sys/windows"
)

const quitSignalName = `Local\SanTianSingboxQuit`

func signalExistingInstanceToQuit() error {
	name, err := windows.UTF16PtrFromString(quitSignalName)
	if err != nil {
		return err
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.SetEvent(handle)
}

func (a *App) startQuitSignalWatcher() {
	safeGo(a.logger, "退出信号监听", func() {
		name, err := windows.UTF16PtrFromString(quitSignalName)
		if err != nil {
			a.logger.Error("创建退出信号名称失败：%v", err)
			return
		}
		handle, err := windows.CreateEvent(nil, 0, 0, name)
		if err != nil {
			handle, err = windows.OpenEvent(windows.SYNCHRONIZE, false, name)
			if err != nil {
				a.logger.Error("打开退出信号失败：%v", err)
				return
			}
		}
		defer windows.CloseHandle(handle)
		for {
			status, err := windows.WaitForSingleObject(handle, windows.INFINITE)
			if err != nil {
				a.logger.Error("等待退出信号失败：%v", err)
				return
			}
			if status != windows.WAIT_OBJECT_0 {
				continue
			}
			a.logger.Info("收到退出信号，正在关闭应用")
			a.requestQuit("退出信号")
			return
		}
	})
}
