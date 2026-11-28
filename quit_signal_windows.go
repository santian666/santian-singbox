package main

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const quitSignalName = `Local\SanTianSingboxQuit`

// restrictedLocalIPCSecurityAttributes returns a SECURITY_ATTRIBUTES granting
// full access only to the object owner, Builtin Administrators, and SYSTEM.
// SecurityDescriptorFromString allocates on the Go heap; callers must keep sd alive across Win32 calls.
func restrictedLocalIPCSecurityAttributes() (*windows.SecurityAttributes, *windows.SECURITY_DESCRIPTOR) {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;OW)(A;;GA;;;BA)(A;;GA;;;SY)")
	if err != nil || sd == nil {
		return nil, nil
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	return sa, sd
}

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
		sa, sd := restrictedLocalIPCSecurityAttributes()
		handle, err := windows.CreateEvent(sa, 0, 0, name)
		runtime.KeepAlive(sd)
		runtime.KeepAlive(sa)
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) && hasAdminPrivileges() {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			a.logger.Warn("检测到退出信号事件已被外部预占，管理员实例已忽略外部退出信号")
			return
		}
		if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
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
