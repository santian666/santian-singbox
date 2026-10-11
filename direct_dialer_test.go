package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsFakeIPv4(t *testing.T) {
	cases := []struct {
		ip   string
		fake bool
	}{
		{"198.18.0.1", true},
		{"198.18.254.12", true},
		{"198.19.1.5", true},
		{"198.19.255.255", true},
		{"198.17.255.255", false},
		{"198.20.0.1", false},
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"192.168.1.1", false},
		{"127.0.0.1", false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if got := isFakeIPv4(ip); got != tc.fake {
			t.Errorf("isFakeIPv4(%s) = %v; want %v", tc.ip, got, tc.fake)
		}
	}
}

func TestDirectPhysicalDialerCreation(t *testing.T) {
	dialer := newDirectPhysicalDialer(2 * time.Second)
	if dialer == nil {
		t.Fatal("newDirectPhysicalDialer should not return nil")
	}
	if dialer.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v; want 2s", dialer.Timeout)
	}
}

func TestTunBypassIncludesSanTianGUI(t *testing.T) {
	names := tunBypassProcessNames()
	found := false
	for _, n := range names {
		if n == "SanTian-Singbox.exe" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tunBypassProcessNames must include SanTian-Singbox.exe, got %v", names)
	}

	regexes := tunBypassProcessPathRegex()
	foundRegex := false
	for _, r := range regexes {
		if r == `(?i)\\SanTian-Singbox(\.exe)?$` {
			foundRegex = true
			break
		}
	}
	if !foundRegex {
		t.Fatalf("tunBypassProcessPathRegex must include SanTian-Singbox, got %v", regexes)
	}
}

func TestResolveDirectPhysicalIPRejectsFakeIP(t *testing.T) {
	_, err := resolveDirectPhysicalIP(context.Background(), "198.18.0.1")
	if err == nil {
		t.Fatal("resolveDirectPhysicalIP should reject FakeIP addresses")
	}
}

func TestLiveNodeISUS1(t *testing.T) {
	st, err := NewStore(`C:\Users\lei\Desktop\SanTian-Singbox-v6.0.74\data\gui\state.dat`)
	if err != nil {
		t.Fatalf("failed to load store: %v", err)
	}
	snap := st.Snapshot()
	for _, n := range snap.Nodes {
		if strings.Contains(n.Name, "IS") || strings.Contains(n.Name, "US") {
			t.Logf("Node: %s (id=%s, type=%s, delay=%d, lastTest=%s)", n.Name, n.ID, n.Type, n.DelayMS, n.LastTest)
		}
	}
}



