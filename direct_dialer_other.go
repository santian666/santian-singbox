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
