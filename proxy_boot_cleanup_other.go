//go:build !windows

package main

func registerSystemProxyBootCleanup(port int) error {
	return nil
}

func unregisterSystemProxyBootCleanup() error {
	return nil
}

func runBootProxyCleanupFromArgs() (bool, int) {
	return false, 0
}
