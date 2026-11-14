//go:build !windows

package main

func enableKillSwitch(root string, logger *Logger, corePath string, appPath string) error {
	return nil
}

func disableKillSwitch(root string, logger *Logger) error {
	return nil
}

func isKillSwitchActive() bool {
	return false
}
