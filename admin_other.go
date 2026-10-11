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

func waitTunReady(timeout time.Duration) error {
	return nil
}

func flushWindowsDNSCache(_ *Logger) {}

func closeStaleFakeIPTCPConnections(_ *Logger) {}
