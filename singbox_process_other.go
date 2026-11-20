//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func runSendCtrlBreakHelperFromArgs() (bool, int) {
	return false, 0
}

func configureSingBoxCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func gracefulStopSingBox(pid int) error {
	return nil
}
