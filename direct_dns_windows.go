//go:build windows

package main

import (
	"context"
	"net"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func getDefaultRouteIfIndex() uint32 {
	ifIndex, _, _ := findPhysicalNetworkInterface()
	return ifIndex
}

func detectPhysicalDNSServers() []string {
	bestIfIndex, _, _ := findPhysicalNetworkInterface()

	var tunIfIndex uint32
	if adapter, err := findWindowsAdapter(tunInterfaceName); err == nil && adapter != nil {
		tunIfIndex = adapter.interfaceIndex
	}

	var size uint32
	err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, nil, &size)
	if err != windows.ERROR_BUFFER_OVERFLOW || size == 0 {
		return nil
	}

	buffer := make([]byte, size)
	first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
	if err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, first, &size); err != nil {
		return nil
	}

	var primaryDNS []string
	var secondaryDNS []string

	for adapter := first; adapter != nil; adapter = adapter.Next {
		if adapter.OperStatus != windows.IfOperStatusUp {
			continue
		}
		if adapter.IfIndex == tunIfIndex {
			continue
		}
		name := strings.ToLower(windows.UTF16PtrToString(adapter.FriendlyName))
		desc := strings.ToLower(windows.UTF16PtrToString(adapter.Description))
		if strings.Contains(name, "santian") || strings.Contains(desc, "wintun") ||
			strings.Contains(name, "loopback") || strings.Contains(desc, "tap") {
			continue
		}

		var adapterDNS []string
		for dns := adapter.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			ip := dns.Address.IP()
			if ip != nil {
				adapterDNS = append(adapterDNS, ip.String())
			}
		}

		if len(adapterDNS) == 0 {
			continue
		}

		if bestIfIndex > 0 && adapter.IfIndex == bestIfIndex {
			primaryDNS = append(primaryDNS, adapterDNS...)
		} else {
			secondaryDNS = append(secondaryDNS, adapterDNS...)
		}
	}

	allDNS := append(primaryDNS, secondaryDNS...)
	return filterDirectDNSServers(allDNS)
}

func newDirectPhysicalUDPDialerForTarget(targetIP net.IP, timeout time.Duration) *net.Dialer {
	dialer := &net.Dialer{
		Timeout: timeout,
	}
	if targetIP != nil && (targetIP.IsLoopback() || targetIP.IsUnspecified()) {
		return dialer
	}
	ifIndex, localIP := getCachedPhysicalInterface()
	if localIP != nil {
		dialer.LocalAddr = &net.UDPAddr{
			IP: localIP,
		}
	}
	if ifIndex > 0 {
		dialer.Control = func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				bindSocketToInterface(fd, ifIndex)
			})
		}
	}
	return dialer
}

func probeDirectUDPDNS(server string) (time.Duration, bool) {
	targetIP := net.ParseIP(server)
	start := time.Now()
	resolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := newDirectPhysicalUDPDialerForTarget(targetIP, 800*time.Millisecond)
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
