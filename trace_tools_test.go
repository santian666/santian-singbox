package main

import "testing"

func TestLocalSpeedtestHTTPProxyDoesNotFallbackToDirect(t *testing.T) {
	env, _, err := (&App{}).localSpeedtestEnv("http_proxy")
	if err == nil || env != nil {
		t.Fatalf("HTTP 代理不可用时不得回退直连：env=%v err=%v", env, err)
	}
}
