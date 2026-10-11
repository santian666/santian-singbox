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

	app.disableTunAfterStartFailure(settings, errors.New("simulated tun timeout"))

	// With CircuitBreaker = true, TUN must NOT be turned off (which would expose traffic to physical NIC)
	currentSettings := store.Snapshot().Settings
	if !currentSettings.Tun {
		t.Fatalf("expected TUN to remain true under CircuitBreaker, but was set to %v", currentSettings.Tun)
	}
	if !app.isCircuitBreakerActive() {
		t.Fatal("expected CircuitBreakerActive to be true")
	}
	if reason := app.circuitBreakerReason(); reason == "" {
		t.Fatal("expected CircuitBreakerReason to not be empty")
	}

	// State() should also report circuit breaker active
	st := app.State()
	if !st.CircuitBreakerActive {
		t.Fatal("expected app.State().CircuitBreakerActive to be true")
	}
	if st.CircuitBreakerReason == "" {
		t.Fatal("expected app.State().CircuitBreakerReason to be non-empty")
	}

	// Reset circuit breaker
	app.setCircuitBreakerActive(false, "")
	if app.isCircuitBreakerActive() {
		t.Fatal("expected CircuitBreakerActive to be false after clearing")
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

	app.disableTunAfterStartFailure(settings, errors.New("simulated tun timeout"))

	// With CircuitBreaker = false, TUN should be disabled to prevent repeated crashes
	currentSettings := store.Snapshot().Settings
	if currentSettings.Tun {
		t.Fatalf("expected TUN to be false when CircuitBreaker is disabled, but was %v", currentSettings.Tun)
	}
	if app.isCircuitBreakerActive() {
		t.Fatal("expected CircuitBreakerActive to remain false when CircuitBreaker is disabled")
	}
}
