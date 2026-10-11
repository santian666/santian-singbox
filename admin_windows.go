package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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

func isRunAsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func hasAdminPrivileges() bool {
	if !isRunAsAdmin() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "fltmc")
	hideCommandWindow(cmd)
	return cmd.Run() == nil
}

func restartAsAdmin(args string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root := detectAppRoot()
	exe = resolveAdminRestartExe(root, exe)
	if !fileExists(exe) {
		return fmt.Errorf("管理员重启文件不存在：%s", exe)
	}
	script, err := writeAdminRestartScript(root, exe, args, os.Getpid())
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(`/c "` + script + `"`)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, params, cwd, windows.SW_HIDE)
}

func resolveAdminRestartExe(root, exe string) string {
	exeDir := filepath.Dir(exe)
	base := filepath.Base(exe)
	withoutDev := strings.Replace(base, "-dev.exe", ".exe", 1)
	candidates := []string{
		filepath.Join(exeDir, withoutDev),
		filepath.Join(exeDir, "SanTian-Singbox.exe"),
		filepath.Join(root, withoutDev),
		filepath.Join(root, "SanTian-Singbox.exe"),
		exe,
		filepath.Join(exeDir, "go-singbox-gui-dev.exe"),
		filepath.Join(root, "go-singbox-gui-dev.exe"),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return exe
}

func writeAdminRestartScript(root, exe, args string, parentPID int) (string, error) {
	dir := filepath.Join(root, "data", "gui")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	script := filepath.Join(dir, "admin-restart.cmd")
	logPath := filepath.Join(root, "logs", "gui", "admin-restart.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`@echo off
cd /d "%s"
echo [%s] admin restart requested: "%s" %s, waiting pid %d>>"%s"
powershell -NoProfile -ExecutionPolicy Bypass -Command "$deadline=(Get-Date).AddSeconds(45); while((Get-Process -Id %d -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline){Start-Sleep -Milliseconds 100}; if(Get-Process -Id %d -ErrorAction SilentlyContinue){exit 1}"
if errorlevel 1 (
  echo parent process wait timeout: %d>>"%s"
  exit /b 1
)
start "" /D "%s" "%s" %s
echo start exit code %%ERRORLEVEL%%>>"%s"
exit /b %%ERRORLEVEL%%
`, root, time.Now().Format("2006-01-02 15:04:05"), exe, args, parentPID, logPath, parentPID, parentPID, parentPID, logPath, root, exe, args, logPath)
	if err := os.WriteFile(script, []byte(content), 0644); err != nil {
		return "", err
	}
	return script, nil
}

func cleanupTunInterface(logger *Logger) {
	_ = cleanupTunInterfaceWithSummary(logger)
}

func cleanupTunInterfaceWithSummary(logger *Logger) string {
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
    Disable-NetAdapter -Name $adapter.Name -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    Write-Output ("ADAPTER:" + $adapter.Name)
  } catch {
    Write-Output ("ERR:" + $_.Exception.Message)
  }
}
Clear-DnsClientCache -ErrorAction SilentlyContinue | Out-Null
$devices=Get-PnpDevice -ErrorAction SilentlyContinue | Where-Object {
  $_.Class -eq "Net" -and
  $_.InstanceId -like "SWD\WINTUN\*" -and
  ($_.FriendlyName -eq "sing-tun Tunnel" -or $_.FriendlyName -in $names)
}
foreach($device in $devices){
  try {
    Disable-PnpDevice -InstanceId "$($device.InstanceId)" -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    pnputil /remove-device "$($device.InstanceId)" | Out-Null
    Write-Output ("PNP:" + $device.InstanceId)
  } catch {
    Write-Output ("ERR:" + $_.Exception.Message)
  }
}
`
	out, err := runCommandOutputWithEnv(map[string]string{"GO_SINGBOX_TUN_NAME": tunInterfaceName}, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	out = strings.TrimSpace(out)
	if err != nil {
		if out != "" && logger != nil {
			logger.Error("清理 TUN 网卡失败：%s", out)
		}
		return ""
	}
	summary := cleanupTunSummary(out)
	if out != "" && logger != nil {
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
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		code, ok := tunReadyState()
		if ok {
			return nil
		}
		last = code
		time.Sleep(tunReadyPollInterval(time.Until(deadline)))
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
		cmd := exec.CommandContext(ctx, "ipconfig", "/flushdns")
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
	if size == 0 {
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
