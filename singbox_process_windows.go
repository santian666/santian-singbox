//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const sendCtrlBreakArgPrefix = "--send-ctrl-break="

var (
	kernel32ConsoleDLL           = windows.NewLazySystemDLL("kernel32.dll")
	procFreeConsole              = kernel32ConsoleDLL.NewProc("FreeConsole")
	procAttachConsole            = kernel32ConsoleDLL.NewProc("AttachConsole")
	procSetConsoleCtrlHandler    = kernel32ConsoleDLL.NewProc("SetConsoleCtrlHandler")
	procGenerateConsoleCtrlEvent = kernel32ConsoleDLL.NewProc("GenerateConsoleCtrlEvent")
)

func init() {
	if handled, exitCode := runSendCtrlBreakHelperFromArgs(); handled {
		os.Exit(exitCode)
	}
}

func runSendCtrlBreakHelperFromArgs() (bool, int) {
	if len(os.Args) != 2 || !strings.HasPrefix(os.Args[1], sendCtrlBreakArgPrefix) {
		return false, 0
	}
	rawPID := strings.TrimSpace(strings.TrimPrefix(os.Args[1], sendCtrlBreakArgPrefix))
	pid, err := strconv.Atoi(rawPID)
	if err != nil || pid <= 0 {
		return true, 2
	}
	if err := sendCtrlBreakToPID(pid); err != nil {
		return true, 1
	}
	return true, 0
}

func sendCtrlBreakToPID(pid int) error {
	_, _, _ = procFreeConsole.Call()
	r1, _, err := procAttachConsole.Call(uintptr(uint32(pid)))
	if r1 == 0 {
		return fmt.Errorf("AttachConsole(%d) failed: %w", pid, err)
	}
	defer func() {
		_, _, _ = procFreeConsole.Call()
	}()
	_, _, _ = procSetConsoleCtrlHandler.Call(0, 1)
	r2, _, err := procGenerateConsoleCtrlEvent.Call(uintptr(windows.CTRL_BREAK_EVENT), uintptr(uint32(pid)))
	if r2 == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent(%d) failed: %w", pid, err)
	}
	return nil
}

func configureSingBoxCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NEW_CONSOLE
}

func gracefulStopSingBox(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, sendCtrlBreakArgPrefix+strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	return cmd.Run()
}
