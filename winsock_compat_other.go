//go:build !windows

package main

func detectIncompatibleWinsockProvider() (string, error) {
	return "", nil
}

func showIncompatibleWinsockProvider(string) {}
