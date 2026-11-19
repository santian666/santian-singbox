//go:build !windows

package main

import "time"

func isRunAsAdmin() bool {
	return true
}

func hasAdminPrivileges() bool {
	return true
}

func restartAsAdmin(_ string) error {
	return nil
}

func cleanupTunInterface(logger *Logger) {}
func cleanupTunInterfaceWithSummary(logger *Logger) string { return "" }
func ensureCleanTunBeforeStart(_ *Logger) {}

func waitTunReady(timeout time.Duration) error {
	return nil
}

func waitTunReadyOrProcessExit(timeout time.Duration, _ <-chan error) error {
	return waitTunReady(timeout)
}

func flushWindowsDNSCache(_ *Logger) {}

func closeStaleFakeIPTCPConnections(_ *Logger) {}
