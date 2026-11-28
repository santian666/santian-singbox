//go:build !windows

package main

func shouldUseAlwaysOnWFPKillSwitch(settings Settings) bool {
	return false
}

func enableWFPKillSwitch(logger *Logger, corePath string, appPath string) error {
	return nil
}

func disableWFPKillSwitch(logger *Logger) error {
	return nil
}

func isWFPKillSwitchActive() bool {
	return false
}
