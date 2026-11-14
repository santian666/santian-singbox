//go:build !windows

package main

import (
	"context"
	"net"
	"time"
)

func getDefaultRouteIfIndex() uint32 {
	return 0
}

func detectPhysicalDNSServers() (primary, secondary []string) {
	return nil, nil
}

func probeDirectUDPDNS(server string) (time.Duration, bool) {
	start := time.Now()
	resolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 800 * time.Millisecond}
			return dialer.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()
	_, err := resolver.LookupHost(ctx, "www.microsoft.com")
	if err != nil {
		return 0, false
	}
	return time.Since(start), true
}

func currentNetworkFingerprint() networkFingerprint {
	return networkFingerprint{}
}
