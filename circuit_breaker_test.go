package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCircuitBreakerDefaultSettings(t *testing.T) {
	defaults := defaultSettings()
	if !defaults.CircuitBreaker {
		t.Fatalf("expected defaultSettings().CircuitBreaker to be true, got %v", defaults.CircuitBreaker)
	}

	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if !store.Snapshot().Settings.CircuitBreaker {
		t.Fatalf("expected store initialized CircuitBreaker to be true, got %v", store.Snapshot().Settings.CircuitBreaker)
	}
}

func TestCircuitBreakerPreventsDisablingTunOnFailure(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	settings := store.Snapshot().Settings
	settings.Tun = true
	settings.CircuitBreaker = true
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	app := &App{
		store:  store,
		logger: NewLogger(filepath.Join(t.TempDir(), "test.log")),
	}

	app.recordTunStartFailureNoFallback(settings, errors.New("simulated tun timeout"))

	// TUN must NOT be turned off (which would expose traffic to physical NIC), and active circuit breaker must stay disabled
	currentSettings := store.Snapshot().Settings
	if !currentSettings.Tun {
		t.Fatalf("expected TUN to remain true, but was set to %v", currentSettings.Tun)
	}
	if app.isCircuitBreakerActive() {
		t.Fatal("expected CircuitBreakerActive to remain false")
	}
	st := app.State()
	if st.CircuitBreakerActive {
		t.Fatal("expected app.State().CircuitBreakerActive to remain false")
	}
}

func TestCircuitBreakerDisabledDisablesTunOnFailure(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.dat")
	store, err := NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	settings := store.Snapshot().Settings
	settings.Tun = true
	settings.CircuitBreaker = false
	if err := store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	app := &App{
		store:  store,
		logger: NewLogger(filepath.Join(t.TempDir(), "test.log")),
	}

	app.recordTunStartFailureNoFallback(settings, errors.New("simulated tun timeout"))

	// Even when CircuitBreaker is false, TUN must NOT auto-disable to direct (prevents real IP leak)
	currentSettings := store.Snapshot().Settings
	if !currentSettings.Tun {
		t.Fatalf("expected TUN to remain true to prevent real IP leak, but was set to %v", currentSettings.Tun)
	}
	if app.isCircuitBreakerActive() {
		t.Fatal("expected CircuitBreakerActive to remain false")
	}
}
