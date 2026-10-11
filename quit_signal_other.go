//go:build !windows

package main

func signalExistingInstanceToQuit() error { return nil }

func (a *App) startQuitSignalWatcher() {}
