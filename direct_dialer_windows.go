//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type physicalInterfaceInfo struct {
	ifIndex uint32
	localIP net.IP
}

type physicalInterfaceCache struct {
	mu        sync.RWMutex
	info      physicalInterfaceInfo
	updatedAt time.Time
}

var globalPhysicalIfCache physicalInterfaceCache

// getCachedPhysicalInterface 获取缓存的物理网卡信息，默认缓存 5 秒
func getCachedPhysicalInterface() (uint32, net.IP) {
	globalPhysicalIfCache.mu.RLock()
	if time.Since(globalPhysicalIfCache.updatedAt) < 5*time.Second && globalPhysicalIfCache.info.localIP != nil {
		info := globalPhysicalIfCache.info
		globalPhysicalIfCache.mu.RUnlock()
		return info.ifIndex, info.localIP
	}
	globalPhysicalIfCache.mu.RUnlock()

	globalPhysicalIfCache.mu.Lock()
	defer globalPhysicalIfCache.mu.Unlock()
	if time.Since(globalPhysicalIfCache.updatedAt) < 5*time.Second && globalPhysicalIfCache.info.localIP != nil {
		return globalPhysicalIfCache.info.ifIndex, globalPhysicalIfCache.info.localIP
	}

	ifIndex, localIP, err := findPhysicalNetworkInterface()
	if err == nil && localIP != nil {
		globalPhysicalIfCache.info = physicalInterfaceInfo{
			ifIndex: ifIndex,
			localIP: localIP,
		}
		globalPhysicalIfCache.updatedAt = time.Now()
		return ifIndex, localIP
	}
	globalPhysicalIfCache.updatedAt = time.Now()
	return 0, nil
}

// findPhysicalNetworkInterface 检测当前承载互联网出口的真实物理网卡索引和本地 IP（排除 SanTian/Wintun 网卡）
func findPhysicalNetworkInterface() (uint32, net.IP, error) {
	// 1. 查找 TUN 网卡索引（用于显式排除）
	var tunIfIndex uint32
	if adapter, err := findWindowsAdapter(tunInterfaceName); err == nil && adapter != nil {
		tunIfIndex = adapter.interfaceIndex
	}

	// 2. 读取 Windows 路由表查找默认网关物理路由（取优先级最高/Metric最小的默认路由）
	var bestIfIndex uint32
	var minMetric uint32 = ^uint32(0)
	if routes, err := windowsIPv4Routes(); err == nil {
		for _, r := range routes {
			if r.prefixLength == 0 && r.destination.Equal(net.IPv4zero) {
				if r.interfaceIndex != tunIfIndex && r.interfaceIndex > 0 {
					if r.metric < minMetric {
						minMetric = r.metric
						bestIfIndex = r.interfaceIndex
					}
				}
			}
		}
	}

	// 3. 读取 Windows 网卡地址列表
	var size uint32
	err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, nil, &size)
	if err != windows.ERROR_BUFFER_OVERFLOW || size == 0 {
		return fallbackPhysicalInterfaceNet(tunIfIndex, bestIfIndex)
	}

	buffer := make([]byte, size)
	first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
	if err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, first, &size); err != nil {
		return fallbackPhysicalInterfaceNet(tunIfIndex, bestIfIndex)
	}

	// 第一优先级：与默认路由网卡索引完全匹配的活跃物理网卡
	if bestIfIndex > 0 {
		for adapter := first; adapter != nil; adapter = adapter.Next {
			if adapter.IfIndex == bestIfIndex && adapter.OperStatus == windows.IfOperStatusUp {
				if ip := getAdapterIPv4(adapter); ip != nil {
					return adapter.IfIndex, ip, nil
				}
			}
		}
	}

	// 第二优先级：扫描任意处于 UP 状态的非虚拟物理网卡（优先以太网和 Wi-Fi）
	var fallbackIndex uint32
	var fallbackIP net.IP
	for adapter := first; adapter != nil; adapter = adapter.Next {
		if adapter.IfIndex == tunIfIndex || adapter.OperStatus != windows.IfOperStatusUp {
			continue
		}
		name := strings.ToLower(windows.UTF16PtrToString(adapter.FriendlyName))
		desc := strings.ToLower(windows.UTF16PtrToString(adapter.Description))
		if strings.Contains(name, "santian") || strings.Contains(desc, "wintun") ||
			strings.Contains(name, "loopback") || strings.Contains(desc, "tap") {
			continue
		}
		ip := getAdapterIPv4(adapter)
		if ip == nil {
			continue
		}
		// 6 = Ethernet, 71 = Wi-Fi (IEEE 802.11)
		if adapter.IfType == 6 || adapter.IfType == 71 {
			return adapter.IfIndex, ip, nil
		}
		if fallbackIP == nil {
			fallbackIndex = adapter.IfIndex
			fallbackIP = ip
		}
	}

	if fallbackIP != nil {
		return fallbackIndex, fallbackIP, nil
	}

	return fallbackPhysicalInterfaceNet(tunIfIndex, bestIfIndex)
}

func getAdapterIPv4(adapter *windows.IpAdapterAddresses) net.IP {
	for uni := adapter.FirstUnicastAddress; uni != nil; uni = uni.Next {
		ip := uni.Address.IP()
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 != nil && !ip4.IsLoopback() && !ip4.IsLinkLocalUnicast() && !isTunSubnetIP(ip4) {
			return ip4
		}
	}
	return nil
}

func fallbackPhysicalInterfaceNet(tunIfIndex, bestIfIndex uint32) (uint32, net.IP, error) {
	ifis, err := net.Interfaces()
	if err != nil {
		return 0, nil, err
	}
	for _, ifi := range ifis {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if uint32(ifi.Index) == tunIfIndex {
			continue
		}
		name := strings.ToLower(ifi.Name)
		if strings.Contains(name, "santian") || strings.Contains(name, "wintun") {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() && !ip4.IsLinkLocalUnicast() && !isTunSubnetIP(ip4) {
				return uint32(ifi.Index), ip4, nil
			}
		}
	}
	return 0, nil, errors.New("未检测到可用的物理网络网卡")
}

func isTunSubnetIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// 172.19.0.0/16 is SanTian TUN, 198.18.0.0/15 is FakeIP
	return (ip4[0] == 172 && ip4[1] == 19) || (ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19))
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

// newDirectPhysicalDialerForTarget 根据目标 IP 创建连接器。若目标为回环地址，则使用标准连接器；若为外网地址，则强制绑定本地物理网卡绕过 TUN
func newDirectPhysicalDialerForTarget(targetIP net.IP, timeout time.Duration) *net.Dialer {
	dialer := &net.Dialer{
		Timeout: timeout,
	}
	if targetIP != nil && (targetIP.IsLoopback() || targetIP.IsUnspecified()) {
		return dialer
	}
	ifIndex, localIP := getCachedPhysicalInterface()
	if localIP != nil {
		dialer.LocalAddr = &net.TCPAddr{
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

func newDirectPhysicalDialer(timeout time.Duration) *net.Dialer {
	return newDirectPhysicalDialerForTarget(nil, timeout)
}

// bindSocketToInterface 通过 Winsock IP_UNICAST_IF / IPV6_UNICAST_IF 强制将 Socket 路由绑定至指定物理网卡
func bindSocketToInterface(fd uintptr, ifIndex uint32) {
	if ifIndex == 0 {
		return
	}
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], ifIndex)
	// IPPROTO_IP = 0, IP_UNICAST_IF = 31 (RFC/Winsock ws2ipdef.h)
	_ = windows.Setsockopt(windows.Handle(fd), windows.IPPROTO_IP, 31, &idx[0], 4)
	// IPPROTO_IPV6 = 41, IPV6_UNICAST_IF = 31
	_ = windows.Setsockopt(windows.Handle(fd), 41, 31, &idx[0], 4)
}

var directPublicDNSServers = []string{
	"223.5.5.5:53",
	"119.29.29.29:53",
	"114.114.114.114:53",
	"8.8.8.8:53",
}

type dohAnswerItem struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

type dohResponse struct {
	Status int             `json:"Status"`
	Answer []dohAnswerItem `json:"Answer"`
}

func resolveViaDoH(ctx context.Context, host string) (string, error) {
	dohEndpoints := []string{
		"https://dns.alidns.com/resolve?name=" + url.QueryEscape(host) + "&type=1",
		"https://doh.pub/resolve?name=" + url.QueryEscape(host) + "&type=1",
	}
	client := &http.Client{
		Timeout: 1500 * time.Millisecond,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
				hostPart, portPart, err := net.SplitHostPort(addr)
				if err == nil {
					if hostPart == "dns.alidns.com" {
						addr = net.JoinHostPort("223.5.5.5", portPart)
					} else if hostPart == "doh.pub" {
						addr = net.JoinHostPort("1.12.12.12", portPart)
					}
				}
				dialer := newDirectPhysicalDialer(1500 * time.Millisecond)
				return dialer.DialContext(dialCtx, network, addr)
			},
		},
	}
	type res struct {
		ip  string
		err error
	}
	ch := make(chan res, len(dohEndpoints))
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, ep := range dohEndpoints {
		go func(endpoint string) {
			req, err := http.NewRequestWithContext(subCtx, http.MethodGet, endpoint, nil)
			if err != nil {
				ch <- res{err: err}
				return
			}
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				ch <- res{err: err}
				return
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK {
				ch <- res{err: errors.New("DoH 请求异常")}
				return
			}
			var parsed dohResponse
			if err := json.Unmarshal(body, &parsed); err != nil {
				ch <- res{err: err}
				return
			}
			for _, ans := range parsed.Answer {
				if ans.Type == 1 {
					ip := net.ParseIP(strings.TrimSpace(ans.Data))
					if ip != nil && ip.To4() != nil && !isFakeIPv4(ip) && !ip.IsLoopback() {
						ch <- res{ip: ip.String()}
						return
					}
				}
			}
			ch <- res{err: errors.New("DoH 无结果")}
		}(ep)
	}

	for i := 0; i < len(dohEndpoints); i++ {
		select {
		case r := <-ch:
			if r.ip != "" {
				cancel()
				return r.ip, nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "", errors.New("DoH 解析无结果")
}

// resolveDirectPhysicalIP 优先通过本地网络解析真实 IP，阻断 TUN 模式下 FakeIP 污染
func resolveDirectPhysicalIP(ctx context.Context, host string) (string, error) {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if ip := net.ParseIP(host); ip != nil {
		if isFakeIPv4(ip) {
			return "", fmt.Errorf("目标地址属于 TUN FakeIP 保留段，无法用于本地测试：%s", host)
		}
		return ip.String(), nil
	}

	// 1. 先尝试通过常规解析器解析（单飞去重）
	addresses, err := lookupTCPPingIP(ctx, host)
	if err == nil {
		if preferred, pErr := preferredTCPPingIP(addresses); pErr == nil && preferred != "" {
			if !isFakeIPv4(net.ParseIP(preferred)) {
				return preferred, nil
			}
		}
	}

	// 2. 若常规解析被 FakeIP 污染（198.18.0.0/15）或解析失败，通过公共 DoH (HTTPS 443) 获取真实 IP（彻底避开 UDP 53 劫持）
	if realIP, dErr := resolveViaDoH(ctx, host); dErr == nil && realIP != "" {
		if !isFakeIPv4(net.ParseIP(realIP)) {
			return realIP, nil
		}
	}

	// 3. 备选物理网卡 UDP 53 直连解析
	if realIP, rErr := resolveViaPhysicalDNS(ctx, host); rErr == nil && realIP != "" {
		if !isFakeIPv4(net.ParseIP(realIP)) {
			return realIP, nil
		}
	}

	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("本地网络解析失败：%s", host)
}

func resolveViaPhysicalDNS(ctx context.Context, host string) (string, error) {
	dialer := newDirectPhysicalDialer(2 * time.Second)
	for _, dnsAddr := range directPublicDNSServers {
		dnsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
				return dialer.DialContext(dialCtx, "udp", dnsAddr)
			},
		}
		ips, err := r.LookupIPAddr(dnsCtx, host)
		cancel()
		if err == nil {
			for _, ip := range ips {
				if ip4 := ip.IP.To4(); ip4 != nil && !isFakeIPv4(ip4) && !ip4.IsLoopback() {
					return ip4.String(), nil
				}
			}
		}
	}
	return "", errors.New("物理网络 DNS 查询无响应")
}
