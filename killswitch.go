package main

import (
	"path/filepath"
)

const killSwitchRulePrefix = "SanTianKillSwitch_"

type KillSwitchProfilePolicy struct {
	Domain  string `json:"domain"`
	Private string `json:"private"`
	Public  string `json:"public"`
}

type KillSwitchState struct {
	Active         bool                    `json:"active"`
	CreatedAt      string                  `json:"createdAt"`
	PID            int                     `json:"pid"`
	OriginalPolicy KillSwitchProfilePolicy `json:"originalPolicy"`
}

func killSwitchStatePath(root string) string {
	return filepath.Join(root, "data", "killswitch-state.json")
}

func hasKillSwitchResidual(root string) bool {
	return fileExists(killSwitchStatePath(root)) || hasAnyKillSwitchRule()
}

func shouldApplyFirewallKillSwitch(settings Settings, circuitBreaker bool) bool {
	if effectiveTun(settings) {
		return false
	}
	if circuitBreaker {
		return true
	}
	return settings.KillSwitch && shouldRunProxy(settings)
}
