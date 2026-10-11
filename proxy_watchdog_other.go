//go:build !windows

package main

func markProxyWatchdogAdminHandoff(_ string, _ int) error {
	return nil
}

func (a *App) startProxyWatchdog() error {
	return nil
}

func runProxyWatchdogFromArgs() (bool, int) {
	return false, 0
}
