//go:build !windows

package main

import (
	"context"
	"net"
	"time"
)

func newDirectPhysicalDialerForTarget(targetIP net.IP, timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
	}
}

func newDirectPhysicalDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
	}
}

func resolveDirectPhysicalIP(ctx context.Context, host string) (string, error) {
	return resolveTCPPingAddress(host)
}

func isFakeIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
}
