//go:build !windows

package main

func enableKillSwitch(logger *Logger, corePath string, appPath string) error {
	return nil
}

func disableKillSwitch(logger *Logger) error {
	return nil
}

func isKillSwitchActive() bool {
	return false
}
