//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	getIPForwardTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIpForwardTable")
	getTcpTable       = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetTcpTable")
	setTcpEntry       = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SetTcpEntry")
)

type mibTcpRow struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
}

type mibIPForwardRow struct {
	destination uint32
	mask        uint32
	policy      uint32
	nextHop     uint32
	ifIndex     uint32
	typeCode    uint32
	protocol    uint32
	age         uint32
	nextHopAS   uint32
	metric1     uint32
	metric2     uint32
	metric3     uint32
	metric4     uint32
	metric5     uint32
}

type tunIPv4Route struct {
	interfaceIndex uint32
	destination    net.IP
	prefixLength   int
	nextHop        net.IP
	metric         uint32
}

type windowsAdapterState struct {
	interfaceIndex uint32
	operStatus     uint32
	hasDNS         bool
}

var tunLegacyAdapterNames = []string{"SanTianTun", "GoSingBoxTun"}

func isSingTunPnPDeviceMatch(friendlyName, deviceDesc string) bool {
	combined := strings.ToLower(strings.TrimSpace(friendlyName + " " + deviceDesc))
	if combined == "" {
		return false
	}
	return strings.Contains(combined, "sing-tun") ||
		strings.Contains(combined, "santian") ||
		strings.Contains(combined, "gosingbox")
}

func hasOrphanedSingTunPnPDevice() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\SWD\Wintun`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}
	for _, id := range subkeys {
		sub, err := registry.OpenKey(k, id, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		fn, _, _ := sub.GetStringValue("FriendlyName")
		dd, _, _ := sub.GetStringValue("DeviceDesc")
		_ = sub.Close()
		if isSingTunPnPDeviceMatch(fn, dd) {
			return true
		}
	}
	return false
}

func removeOrphanedSingTunPnPDevices(logger *Logger) []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\SWD\Wintun`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var removed []string
	pnpUtil := systemBinaryPath("pnputil.exe")
	for _, id := range subkeys {
		sub, err := registry.OpenKey(k, id, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		fn, _, _ := sub.GetStringValue("FriendlyName")
		dd, _, _ := sub.GetStringValue("DeviceDesc")
		_ = sub.Close()
		if !isSingTunPnPDeviceMatch(fn, dd) {
			continue
		}
		instanceID := `SWD\Wintun\` + id
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, pnpUtil, "/remove-device", instanceID)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, rmErr := cmd.CombinedOutput()
		cancel()
		if rmErr != nil {
			if logger != nil {
				logger.Error("移除残留 PnP 虚拟网卡 (%s) 失败：%v %s", instanceID, rmErr, strings.TrimSpace(string(out)))
			}
			continue
		}
		removed = append(removed, "PNP:"+instanceID)
	}
	return removed
}

func waitWindowsAdapterGone(name string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if adapter, err := findWindowsAdapter(name); err == nil && adapter == nil {
			return
		}
		time.Sleep(60 * time.Millisecond)
	}
}

func hasAnyTunAdapterResidual() bool {
	if adapter, err := findWindowsAdapter(tunInterfaceName); err == nil && adapter != nil {
		return true
	}
	for _, legacy := range tunLegacyAdapterNames {
		if adapter, err := findWindowsAdapter(legacy); err == nil && adapter != nil {
			return true
		}
	}
	return false
}

func ensureCleanTunBeforeStart(logger *Logger) {
	if !hasAnyTunAdapterResidual() && !hasOrphanedSingTunPnPDevice() {
		return
	}
	if logger != nil {
		logger.Info("检测到残留 TUN 虚拟网卡设备，启动前正在彻底移除...")
	}
	cleanupTunInterface(logger)
}

func cleanupTunInterface(logger *Logger) {
	_ = cleanupTunInterfaceWithSummary(logger)
}

func cleanupTunInterfaceWithSummary(logger *Logger) string {
	pnpItems := removeOrphanedSingTunPnPDevices(logger)
	var outputs []string
	outputs = append(outputs, pnpItems...)
	hasAdapter := hasAnyTunAdapterResidual()
	if len(pnpItems) == 0 && !hasAdapter {
		return ""
	}
	if hasAdapter {
		script := `
$name=$env:GO_SINGBOX_TUN_NAME
$oldName="SanTianTun"
$legacyName="GoSingBoxTun"
$names=@($name,$oldName,$legacyName) | Sort-Object -Unique
$adapters=@()
foreach($item in $names){
  $adapters += Get-NetAdapter -Name $item -IncludeHidden -ErrorAction SilentlyContinue
}
foreach($adapter in $adapters){
  try {
    Get-NetRoute -InterfaceIndex $adapter.ifIndex -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    Get-NetRoute -InterfaceIndex $adapter.ifIndex -PolicyStore PersistentStore -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    Set-DnsClientServerAddress -InterfaceIndex $adapter.ifIndex -AddressFamily IPv4 -ResetServerAddresses -ErrorAction SilentlyContinue | Out-Null
    Set-DnsClientServerAddress -InterfaceIndex $adapter.ifIndex -AddressFamily IPv6 -ResetServerAddresses -ErrorAction SilentlyContinue | Out-Null
    if ($adapter.PnPDeviceID) {
      & "$env:SystemRoot\System32\pnputil.exe" /remove-device "$($adapter.PnPDeviceID)" 2>&1 | Out-Null
    }
    $still = Get-NetAdapter -Name $adapter.Name -IncludeHidden -ErrorAction SilentlyContinue
    if ($still -and $still.Status -eq "Disabled") {
      Enable-NetAdapter -Name $adapter.Name -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    }
    Write-Output ("ADAPTER:" + $adapter.Name)
  } catch {
    Write-Output ("ERR:" + $_.Exception.Message)
  }
}
Clear-DnsClientCache -ErrorAction SilentlyContinue | Out-Null
`
		psExe := systemBinaryPath(`WindowsPowerShell\v1.0\powershell.exe`)
		out, err := runCommandOutputWithEnv(map[string]string{"GO_SINGBOX_TUN_NAME": tunInterfaceName}, psExe, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		out = strings.TrimSpace(out)
		if err != nil {
			if out != "" && logger != nil {
				logger.Error("清理 TUN 网卡失败：%s", out)
			}
		} else if out != "" {
			outputs = append(outputs, strings.Fields(out)...)
		}
	}
	waitWindowsAdapterGone(tunInterfaceName, 1500*time.Millisecond)
	combinedOut := strings.TrimSpace(strings.Join(outputs, " "))
	summary := cleanupTunSummary(combinedOut)
	if summary != "" && logger != nil {
		logger.Info("已清理 TUN 残留：%s", summary)
	}
	return summary
}

func cleanupTunSummary(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "ADAPTER:"):
			items = append(items, "网卡 "+strings.TrimPrefix(field, "ADAPTER:"))
		case strings.HasPrefix(field, "PNP:"):
			items = append(items, "异常设备 "+strings.TrimPrefix(field, "PNP:"))
		default:
			items = append(items, field)
		}
	}
	return strings.Join(items, "，")
}

func waitTunReady(timeout time.Duration) error {
	return waitTunReadyOrProcessExit(timeout, nil)
}

func waitTunReadyOrProcessExit(timeout time.Duration, done <-chan error) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		if done != nil {
			select {
			case err, ok := <-done:
				if !ok || err == nil {
					return errors.New("sing-box 启动后已退出")
				}
				return fmt.Errorf("sing-box 启动后已退出：%w", err)
			default:
			}
		}
		code, ok := tunReadyState()
		if ok {
			return nil
		}
		last = code
		wait := tunReadyPollInterval(time.Until(deadline))
		if rem := time.Until(deadline); rem < wait {
			wait = rem
		}
		if wait <= 0 {
			break
		}
		if done != nil {
			timer := time.NewTimer(wait)
			select {
			case err, ok := <-done:
				timer.Stop()
				if !ok || err == nil {
					return errors.New("sing-box 启动后已退出")
				}
				return fmt.Errorf("sing-box 启动后已退出：%w", err)
			case <-timer.C:
			}
		} else {
			time.Sleep(wait)
		}
	}
	return fmt.Errorf(tunReadyMessage(last))
}

func tunReadyPollInterval(remaining time.Duration) time.Duration {
	if remaining > 10*time.Second {
		return 300 * time.Millisecond
	}
	if remaining > 5*time.Second {
		return 500 * time.Millisecond
	}
	return 800 * time.Millisecond
}

func tunReadyState() (string, bool) {
	adapter, err := findWindowsAdapter(tunInterfaceName)
	if err != nil {
		return err.Error(), false
	}
	if adapter == nil {
		return "MISSING_ADAPTER", false
	}
	if adapter.operStatus != windows.IfOperStatusUp {
		return "ADAPTER_DOWN:" + adapterStatusName(adapter.operStatus), false
	}
	routes, err := windowsIPv4Routes()
	if err != nil {
		return err.Error(), false
	}
	routeOK, prefixes := tunIPv4RoutesReady(routes, adapter.interfaceIndex)
	if !routeOK {
		return "MISSING_ROUTE:" + strings.Join(prefixes, ","), false
	}
	if !adapter.hasDNS {
		return "MISSING_DNS", false
	}
	return "OK", true
}

func findWindowsAdapter(name string) (*windowsAdapterState, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var size uint32
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, nil, &size)
		if err == windows.ERROR_NO_DATA {
			return nil, nil
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil, fmt.Errorf("读取 Windows 网卡失败：%w", err)
		}
		if size == 0 {
			return nil, nil
		}
		buffer := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
		err = windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_ALL_INTERFACES, 0, first, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取 Windows 网卡失败：%w", err)
		}
		for adapter := first; adapter != nil; adapter = adapter.Next {
			if strings.EqualFold(strings.TrimSpace(windows.UTF16PtrToString(adapter.FriendlyName)), name) {
				return &windowsAdapterState{
					interfaceIndex: adapter.IfIndex,
					operStatus:     adapter.OperStatus,
					hasDNS:         adapterHasDNS(adapter),
				}, nil
			}
		}
		return nil, nil
	}
	return nil, errors.New("读取 Windows 网卡时网络状态持续变化")
}

func adapterHasDNS(adapter *windows.IpAdapterAddresses) bool {
	for current := adapter.FirstDnsServerAddress; current != nil; current = current.Next {
		if ip := current.Address.IP(); ip != nil && !ip.IsUnspecified() {
			return true
		}
	}
	return false
}

func adapterStatusName(status uint32) string {
	switch status {
	case windows.IfOperStatusDown:
		return "Down"
	case windows.IfOperStatusTesting:
		return "Testing"
	case windows.IfOperStatusDormant:
		return "Dormant"
	case windows.IfOperStatusNotPresent:
		return "NotPresent"
	case windows.IfOperStatusLowerLayerDown:
		return "LowerLayerDown"
	default:
		return "Unknown(" + strconv.FormatUint(uint64(status), 10) + ")"
	}
}

func windowsIPv4Routes() ([]tunIPv4Route, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var size uint32
		result, _, _ := getIPForwardTable.Call(0, uintptr(unsafe.Pointer(&size)), 1)
		if result != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, fmt.Errorf("读取 Windows 路由表失败：错误码 %d", result)
		}
		if size < 4 {
			return nil, nil
		}
		buffer := make([]byte, size)
		result, _, _ = getIPForwardTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 1)
		if result == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if result != 0 {
			return nil, fmt.Errorf("读取 Windows 路由表失败：错误码 %d", result)
		}
		count := *(*uint32)(unsafe.Pointer(&buffer[0]))
		rowSize := uint32(unsafe.Sizeof(mibIPForwardRow{}))
		available := (uint32(len(buffer)) - 4) / rowSize
		if count > available {
			count = available
		}
		routes := make([]tunIPv4Route, 0, count)
		for index := uint32(0); index < count; index++ {
			offset := 4 + index*rowSize
			row := (*mibIPForwardRow)(unsafe.Pointer(&buffer[offset]))
			mask := net.IPMask(ipv4FromWindowsDWORD(row.mask))
			prefixLength, bits := mask.Size()
			if bits != 32 {
				continue
			}
			destination := ipv4FromWindowsDWORD(row.destination).Mask(mask)
			routes = append(routes, tunIPv4Route{
				interfaceIndex: row.ifIndex,
				destination:    destination,
				prefixLength:   prefixLength,
				nextHop:        ipv4FromWindowsDWORD(row.nextHop),
				metric:         row.metric1,
			})
		}
		return routes, nil
	}
	return nil, errors.New("读取 Windows 路由表时网络状态持续变化")
}

func ipv4FromWindowsDWORD(value uint32) net.IP {
	return net.IPv4(byte(value), byte(value>>8), byte(value>>16), byte(value>>24)).To4()
}

func tunIPv4RoutesReady(routes []tunIPv4Route, interfaceIndex uint32) (bool, []string) {
	prefixes := make([]string, 0, 8)
	hasDefault := false
	hasLowerHalf := false
	hasUpperHalf := false
	publicRouteCount := 0
	for _, route := range routes {
		if route.interfaceIndex != interfaceIndex || route.destination == nil {
			continue
		}
		prefix := route.destination.String() + "/" + strconv.Itoa(route.prefixLength)
		if len(prefixes) < 8 {
			prefixes = append(prefixes, prefix)
		}
		switch {
		case route.prefixLength == 0 && route.destination.Equal(net.IPv4zero):
			hasDefault = true
		case route.prefixLength == 1 && route.destination.Equal(net.IPv4zero):
			hasLowerHalf = true
		case route.prefixLength == 1 && route.destination.Equal(net.IPv4(128, 0, 0, 0)):
			hasUpperHalf = true
		}
		if route.prefixLength <= 12 && isPublicIPv4Route(route.destination) {
			publicRouteCount++
		}
	}
	return hasDefault || (hasLowerHalf && hasUpperHalf) || publicRouteCount >= 4, prefixes
}

func isPublicIPv4Route(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || ip[0] == 0 || ip[0] == 10 || ip[0] == 127 || ip[0] >= 224 {
		return false
	}
	if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
		return false
	}
	if ip[0] == 169 && ip[1] == 254 {
		return false
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return false
	}
	return ip[0] != 192 || ip[1] != 168
}

func tunReadyMessage(code string) string {
	code = strings.TrimSpace(code)
	switch {
	case code == "", code == "MISSING_ADAPTER":
		return "未检测到 TUN 网卡"
	case strings.HasPrefix(code, "ADAPTER_DOWN:"):
		return "TUN 网卡状态为 " + strings.TrimPrefix(code, "ADAPTER_DOWN:")
	case code == "MISSING_ROUTE":
		return "TUN 默认路由未生效"
	case strings.HasPrefix(code, "MISSING_ROUTE:"):
		detail := strings.TrimSpace(strings.TrimPrefix(code, "MISSING_ROUTE:"))
		if detail == "" {
			return "TUN 默认路由未生效"
		}
		return "TUN 默认路由未生效，当前路由：" + detail
	case code == "MISSING_DNS":
		return "TUN DNS 未生效"
	default:
		return code
	}
}

func flushWindowsDNSCache(logger *Logger) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, systemBinaryPath("ipconfig.exe"), "/flushdns")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Run(); err != nil {
			if logger != nil {
				logger.Info("刷新系统 DNS 缓存略过：%v", err)
			}
		} else {
			if logger != nil {
				logger.Info("TUN 已就绪，已自动刷新系统 DNS 缓存")
			}
		}
	}()
}

func closeStaleFakeIPTCPConnections(logger *Logger) {
	var size uint32
	getTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if size < 4 {
		return
	}
	buf := make([]byte, size+4096)
	size = uint32(len(buf))
	ret, _, _ := getTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if ret != 0 {
		return
	}
	numEntries := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := uint32(unsafe.Sizeof(mibTcpRow{}))
	if uint32(len(buf)) < 4+numEntries*rowSize {
		return
	}
	closedCount := 0
	for i := uint32(0); i < numEntries; i++ {
		rowPtr := (*mibTcpRow)(unsafe.Pointer(&buf[4+i*rowSize]))
		ipBytes := (*[4]byte)(unsafe.Pointer(&rowPtr.RemoteAddr))
		if ipBytes[0] == 198 && (ipBytes[1] == 18 || ipBytes[1] == 19) {
			row := *rowPtr
			row.State = 12 // MIB_TCP_STATE_DELETE_TCB
			setRet, _, _ := setTcpEntry.Call(uintptr(unsafe.Pointer(&row)))
			if setRet == 0 {
				closedCount++
			}
		}
	}
	if logger != nil && closedCount > 0 {
		logger.Info("TUN 维护：已重置 %d 个残留旧 FakeIP 僵尸连接", closedCount)
	}
}
