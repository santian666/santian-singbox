//go:build !windows

package main

func listProcesses() ([]processInfo, error) {
	return nil, nil
}
