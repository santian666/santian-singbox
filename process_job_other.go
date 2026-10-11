//go:build !windows

package main

import "os"

func createKillOnCloseJob(*os.Process) (uintptr, error) {
	return 0, nil
}

func closeKillOnCloseJob(uintptr) {}
