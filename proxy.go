package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const systemProxyRASHelperArg = "--system-proxy-ras-helper"

const systemProxyRASHelperTimeout = 750 * time.Millisecond

const rasErrorBufferTooSmall = 603

func EnableSystemProxy(port int) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("系统代理端口 %d 无效", port)
	}
	if !tcpPortOpen("127.0.0.1", port, 500*time.Millisecond) {
		return fmt.Errorf("本地代理端口 %d 未监听，未修改系统代理", port)
	}
	server := systemProxyServer(port)
	bypass := systemProxyBypass()
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开系统代理注册表失败：%w", err)
	}
	defer key.Close()
	_ = key.DeleteValue("AutoConfigURL")
	if err := key.SetStringValue("ProxyServer", server); err != nil {
		return fmt.Errorf("设置系统代理地址失败：%w", err)
	}
	if err := key.SetStringValue("ProxyOverride", bypass); err != nil {
		return fmt.Errorf("设置系统代理例外失败：%w", err)
	}
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("开启系统代理失败：%w", err)
	}
	if err := setWinINetProxy(server, bypass); err != nil {
		return err
	}
	issue, err := systemProxyConfigurationIssueForPort(port)
	if err != nil {
		return err
	}
	if issue != "" {
		return fmt.Errorf("系统代理写入后未生效：%s，请检查是否被其他代理软件覆盖", issue)
	}
	return nil
}

func DisableSystemProxy() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开系统代理注册表失败：%w", err)
	}
	defer key.Close()
	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("关闭系统代理失败：%w", err)
	}
	if err := key.SetStringValue("ProxyServer", ""); err != nil {
		return fmt.Errorf("清理系统代理地址失败：%w", err)
	}
	if err := key.SetStringValue("ProxyOverride", ""); err != nil {
		return fmt.Errorf("清理系统代理例外失败：%w", err)
	}
	_ = key.DeleteValue("AutoConfigURL")
	if err := clearWinINetProxy(); err != nil {
		return err
	}
	settings, err := readSystemProxySettings()
	if err != nil {
		return err
	}
	connection, err := readWinINetConnectionProxy()
	if err != nil {
		return err
	}
	if issue := systemProxyDisabledIssue(settings, connection); issue != "" {
		return fmt.Errorf("系统代理关闭后仍有残留：%s，请检查是否被其他代理软件接管", issue)
	}
	return nil
}

func systemProxyEnabled() (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	settings, err := readSystemProxySettings()
	return settings.enabled, err
}

func systemProxyEnabledForPort(port int) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	settings, err := readSystemProxySettings()
	if err != nil {
		return false, err
	}
	if !settings.enabled {
		return false, nil
	}
	server := strings.ToLower(settings.server)
	target := "127.0.0.1:" + strconv.Itoa(port)
	localTarget := "localhost:" + strconv.Itoa(port)
	return strings.Contains(server, target) || strings.Contains(server, localTarget), nil
}

type systemProxySettings struct {
	enabled       bool
	server        string
	bypass        string
	autoConfigURL string
}

type winINetProxySettings struct {
	flags         uint32
	server        string
	bypass        string
	autoConfigURL string
}

func readSystemProxySettings() (systemProxySettings, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return systemProxySettings{}, fmt.Errorf("打开系统代理注册表失败：%w", err)
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil {
		enabled = 0
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		server = ""
	}
	bypass, _, err := key.GetStringValue("ProxyOverride")
	if err != nil {
		bypass = ""
	}
	autoConfigURL, _, err := key.GetStringValue("AutoConfigURL")
	if err != nil {
		autoConfigURL = ""
	}
	return systemProxySettings{
		enabled:       enabled != 0,
		server:        server,
		bypass:        bypass,
		autoConfigURL: autoConfigURL,
	}, nil
}

func systemProxyConfiguredForPort(port int) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	issue, err := systemProxyConfigurationIssueForPort(port)
	if err != nil {
		return false, err
	}
	return issue == "", nil
}

func systemProxyConfigurationIssueForPort(port int) (string, error) {
	settings, err := readSystemProxySettings()
	if err != nil {
		return "", err
	}
	connection, err := readWinINetConnectionProxy()
	if err != nil {
		return "", err
	}
	return systemProxyConfigurationIssue(settings, connection, port), nil
}

func systemProxySettingsMatch(settings systemProxySettings, port int) bool {
	if !settings.enabled || strings.TrimSpace(settings.autoConfigURL) != "" {
		return false
	}
	return normalizeSystemProxyValue(settings.server) == normalizeSystemProxyValue(systemProxyServer(port)) &&
		normalizeSystemProxyValue(settings.bypass) == normalizeSystemProxyValue(systemProxyBypass())
}

func winINetProxySettingsMatch(settings winINetProxySettings, port int) bool {
	return settings.flags == proxyTypeDirect|proxyTypeProxy &&
		normalizeSystemProxyValue(settings.server) == normalizeSystemProxyValue(systemProxyServer(port)) &&
		normalizeSystemProxyValue(settings.bypass) == normalizeSystemProxyValue(systemProxyBypass()) &&
		normalizeSystemProxyValue(settings.autoConfigURL) == ""
}

func normalizeSystemProxyValue(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ";")
}

func systemProxyConfigurationIssue(settings systemProxySettings, connection winINetProxySettings, port int) string {
	issues := make([]string, 0, 8)
	if !settings.enabled {
		issues = append(issues, "注册表 ProxyEnable=0")
	}
	if normalizeSystemProxyValue(settings.server) != normalizeSystemProxyValue(systemProxyServer(port)) {
		issues = append(issues, "注册表 ProxyServer 为空或不匹配")
	}
	if normalizeSystemProxyValue(settings.bypass) != normalizeSystemProxyValue(systemProxyBypass()) {
		issues = append(issues, "注册表 ProxyOverride 不匹配")
	}
	if strings.TrimSpace(settings.autoConfigURL) != "" {
		issues = append(issues, "注册表 AutoConfigURL 未清理")
	}
	if connection.flags&proxyTypeProxy == 0 {
		issues = append(issues, "WinINet 默认连接 PROXY 标志关闭")
	} else if connection.flags != proxyTypeDirect|proxyTypeProxy {
		issues = append(issues, fmt.Sprintf("WinINet 默认连接标志异常(%d)", connection.flags))
	}
	if normalizeSystemProxyValue(connection.server) != normalizeSystemProxyValue(systemProxyServer(port)) {
		issues = append(issues, "WinINet 默认连接代理地址为空或不匹配")
	}
	if normalizeSystemProxyValue(connection.bypass) != normalizeSystemProxyValue(systemProxyBypass()) {
		issues = append(issues, "WinINet 默认连接绕过列表不匹配")
	}
	if strings.TrimSpace(connection.autoConfigURL) != "" {
		issues = append(issues, "WinINet 默认连接 PAC 地址未清理")
	}
	return strings.Join(issues, "；")
}

func systemProxyDisabledIssue(settings systemProxySettings, connection winINetProxySettings) string {
	issues := make([]string, 0, 6)
	if settings.enabled {
		issues = append(issues, "注册表 ProxyEnable 仍开启")
	}
	if strings.TrimSpace(settings.server) != "" || strings.TrimSpace(settings.bypass) != "" || strings.TrimSpace(settings.autoConfigURL) != "" {
		issues = append(issues, "注册表代理内容未清空")
	}
	if connection.flags != proxyTypeDirect {
		issues = append(issues, fmt.Sprintf("WinINet 默认连接标志未恢复直连(%d)", connection.flags))
	}
	if strings.TrimSpace(connection.server) != "" || strings.TrimSpace(connection.bypass) != "" || strings.TrimSpace(connection.autoConfigURL) != "" {
		issues = append(issues, "WinINet 默认连接代理内容未清空")
	}
	return strings.Join(issues, "；")
}

func systemProxyServer(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

func systemProxyBypass() string {
	items := []string{
		"<local>",
		"localhost",
		"127.*",
		"10.*",
	}
	for index := 16; index <= 31; index++ {
		items = append(items, "172."+strconv.Itoa(index)+".*")
	}
	items = append(items, "192.168.*", "169.254.*")
	return strings.Join(items, ";")
}

func NotifyProxyChange() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	if err := wininet.Load(); err != nil {
		return errors.New("系统代理变更通知失败")
	}
	const (
		internetOptionSettingsChanged = 39
		internetOptionRefresh         = 37
	)
	changed, _, _ := internetSetOption.Call(0, uintptr(internetOptionSettingsChanged), 0, 0)
	refreshed, _, _ := internetSetOption.Call(0, uintptr(internetOptionRefresh), 0, 0)
	if changed == 0 || refreshed == 0 {
		return errors.New("系统代理变更通知失败")
	}
	return nil
}

func setWinINetProxy(server string, bypass string) error {
	return setWinINetConnectionProxy(proxyTypeDirect|proxyTypeProxy, server, bypass)
}

func clearWinINetProxy() error {
	return setWinINetConnectionProxy(proxyTypeDirect, "", "")
}

const (
	internetOptionPerConnectionOption = 75
	internetOptionSettingsChanged     = 39
	internetOptionRefresh             = 37

	internetPerConnFlags       = 1
	internetPerConnProxyServer = 2
	internetPerConnProxyBypass = 3
	internetPerConnAutoConfig  = 4

	proxyTypeDirect = 1
	proxyTypeProxy  = 2
)

type internetPerConnOption struct {
	option uint32
	value  uintptr
}

type internetPerConnOptionList struct {
	size        uint32
	connection  uintptr
	optionCount uint32
	optionError uint32
	options     uintptr
}

func setWinINetConnectionProxy(flags uint32, server string, bypass string) error {
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	if err := wininet.Load(); err != nil {
		return errors.New("系统代理变更通知失败")
	}
	options := []internetPerConnOption{{option: internetPerConnFlags, value: uintptr(flags)}}
	serverPtr, err := syscall.UTF16PtrFromString(server)
	if err != nil {
		return fmt.Errorf("系统代理地址无效：%w", err)
	}
	bypassPtr, err := syscall.UTF16PtrFromString(bypass)
	if err != nil {
		return fmt.Errorf("系统代理例外无效：%w", err)
	}
	autoConfigPtr, err := syscall.UTF16PtrFromString("")
	if err != nil {
		return fmt.Errorf("系统代理 PAC 地址无效：%w", err)
	}
	options = append(options,
		internetPerConnOption{option: internetPerConnProxyServer, value: uintptr(unsafe.Pointer(serverPtr))},
		internetPerConnOption{option: internetPerConnProxyBypass, value: uintptr(unsafe.Pointer(bypassPtr))},
		internetPerConnOption{option: internetPerConnAutoConfig, value: uintptr(unsafe.Pointer(autoConfigPtr))},
	)
	// 只更新默认 WinINet 连接，异常的 VPN/拨号项可能让逐项写入长期阻塞。
	if err := setWinINetOptionsForConnection(internetSetOption, 0, options); err != nil {
		return err
	}
	runtime.KeepAlive(serverPtr)
	runtime.KeepAlive(bypassPtr)
	runtime.KeepAlive(autoConfigPtr)
	if err := refreshWinINetProxy(internetSetOption); err != nil {
		return err
	}
	_ = syncRASSystemProxy(flags, server, bypass)
	return nil
}

func syncRASSystemProxy(flags uint32, server string, bypass string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := buildSystemProxyRASHelperArgs(flags, server, bypass)
	ctx, cancel := context.WithTimeout(context.Background(), systemProxyRASHelperTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	hideCommandWindow(cmd)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("同步 VPN/拨号代理设置超时")
		}
		return err
	}
	return nil
}

func buildSystemProxyRASHelperArgs(flags uint32, server string, bypass string) []string {
	encode := func(value string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(value))
	}
	return []string{
		systemProxyRASHelperArg,
		strconv.FormatUint(uint64(flags), 10),
		encode(server),
		encode(bypass),
	}
}

func runSystemProxyRASHelperFromArgs() (bool, int) {
	if len(os.Args) < 2 || os.Args[1] != systemProxyRASHelperArg {
		return false, 0
	}
	flags, server, bypass, err := parseSystemProxyRASHelperArgs(os.Args[1:])
	if err != nil {
		return true, 2
	}
	if err := setRASWinINetConnectionProxy(flags, server, bypass); err != nil {
		return true, 1
	}
	return true, 0
}

func parseSystemProxyRASHelperArgs(args []string) (uint32, string, string, error) {
	if len(args) != 4 || args[0] != systemProxyRASHelperArg {
		return 0, "", "", errors.New("RAS 系统代理辅助参数无效")
	}
	flags, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return 0, "", "", err
	}
	decode := func(value string) (string, error) {
		data, err := base64.RawURLEncoding.DecodeString(value)
		return string(data), err
	}
	server, err := decode(args[2])
	if err != nil {
		return 0, "", "", err
	}
	bypass, err := decode(args[3])
	if err != nil {
		return 0, "", "", err
	}
	return uint32(flags), server, bypass, nil
}

func setRASWinINetConnectionProxy(flags uint32, server string, bypass string) error {
	connections, err := rasConnectionNames()
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		return nil
	}
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	if err := wininet.Load(); err != nil {
		return errors.New("加载 WinINet 失败")
	}
	var firstErr error
	for _, connection := range connections {
		connectionPtr, err := syscall.UTF16PtrFromString(connection)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		err = setWinINetConnectionProxyForName(internetSetOption, uintptr(unsafe.Pointer(connectionPtr)), flags, server, bypass)
		runtime.KeepAlive(connectionPtr)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	refreshErr := refreshWinINetProxy(internetSetOption)
	if firstErr != nil {
		return firstErr
	}
	return refreshErr
}

func setWinINetConnectionProxyForName(internetSetOption *syscall.LazyProc, connection uintptr, flags uint32, server string, bypass string) error {
	options := []internetPerConnOption{{option: internetPerConnFlags, value: uintptr(flags)}}
	serverPtr, err := syscall.UTF16PtrFromString(server)
	if err != nil {
		return fmt.Errorf("系统代理地址无效：%w", err)
	}
	bypassPtr, err := syscall.UTF16PtrFromString(bypass)
	if err != nil {
		return fmt.Errorf("系统代理例外无效：%w", err)
	}
	autoConfigPtr, err := syscall.UTF16PtrFromString("")
	if err != nil {
		return fmt.Errorf("系统代理 PAC 地址无效：%w", err)
	}
	options = append(options,
		internetPerConnOption{option: internetPerConnProxyServer, value: uintptr(unsafe.Pointer(serverPtr))},
		internetPerConnOption{option: internetPerConnProxyBypass, value: uintptr(unsafe.Pointer(bypassPtr))},
		internetPerConnOption{option: internetPerConnAutoConfig, value: uintptr(unsafe.Pointer(autoConfigPtr))},
	)
	err = setWinINetOptionsForConnection(internetSetOption, connection, options)
	runtime.KeepAlive(serverPtr)
	runtime.KeepAlive(bypassPtr)
	runtime.KeepAlive(autoConfigPtr)
	return err
}

func readWinINetConnectionProxy() (winINetProxySettings, error) {
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetQueryOption := wininet.NewProc("InternetQueryOptionW")
	if err := wininet.Load(); err != nil {
		return winINetProxySettings{}, errors.New("加载 WinINet 失败")
	}
	options := []internetPerConnOption{
		{option: internetPerConnFlags},
		{option: internetPerConnProxyServer},
		{option: internetPerConnProxyBypass},
		{option: internetPerConnAutoConfig},
	}
	list := internetPerConnOptionList{
		size:        uint32(unsafe.Sizeof(internetPerConnOptionList{})),
		optionCount: uint32(len(options)),
		options:     uintptr(unsafe.Pointer(&options[0])),
	}
	bufferSize := list.size
	ok, _, callErr := internetQueryOption.Call(
		0,
		uintptr(internetOptionPerConnectionOption),
		uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&bufferSize)),
	)
	runtime.KeepAlive(options)
	if ok == 0 {
		freeWinINetOptionStrings(options)
		return winINetProxySettings{}, fmt.Errorf("读取 WinINet 默认连接代理失败：%v", callErr)
	}
	settings := winINetProxySettings{
		flags:         uint32(options[0].value),
		server:        winINetOptionString(&options[1]),
		bypass:        winINetOptionString(&options[2]),
		autoConfigURL: winINetOptionString(&options[3]),
	}
	return settings, nil
}

func freeWinINetOptionStrings(options []internetPerConnOption) {
	for index := 1; index < len(options); index++ {
		freeWinINetString(&options[index])
	}
}

func winINetOptionString(option *internetPerConnOption) string {
	value := internetPerConnOptionPointer(option)
	if value == nil {
		return ""
	}
	text := windows.UTF16PtrToString((*uint16)(value))
	freeWinINetString(option)
	return text
}

func internetPerConnOptionPointer(option *internetPerConnOption) unsafe.Pointer {
	if option == nil || option.value == 0 {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(&option.value))
}

func freeWinINetString(option *internetPerConnOption) {
	value := internetPerConnOptionPointer(option)
	if value == nil {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalFree := kernel32.NewProc("GlobalFree")
	_, _, _ = globalFree.Call(uintptr(value))
	option.value = 0
}

type rasEntryName struct {
	size          uint32
	entryName     [257]uint16
	flags         uint32
	phonebookPath [261]uint16
}

func rasConnectionNames() ([]string, error) {
	rasapi := syscall.NewLazyDLL("rasapi32.dll")
	rasEnumEntries := rasapi.NewProc("RasEnumEntriesW")
	if err := rasapi.Load(); err != nil {
		return nil, errors.New("加载 RAS 连接接口失败")
	}
	entrySize := uint32(unsafe.Sizeof(rasEntryName{}))
	entries := make([]rasEntryName, 1)
	entries[0].size = entrySize
	bufferSize := entrySize
	entryCount := uint32(0)
	result, _, _ := rasEnumEntries.Call(
		0,
		0,
		uintptr(unsafe.Pointer(&entries[0])),
		uintptr(unsafe.Pointer(&bufferSize)),
		uintptr(unsafe.Pointer(&entryCount)),
	)
	if uint32(result) == rasErrorBufferTooSmall {
		capacity := int((bufferSize + entrySize - 1) / entrySize)
		if capacity < int(entryCount) {
			capacity = int(entryCount)
		}
		if capacity < 1 {
			capacity = 1
		}
		entries = make([]rasEntryName, capacity)
		bufferSize = uint32(capacity) * entrySize
		for index := range entries {
			entries[index].size = entrySize
		}
		result, _, _ = rasEnumEntries.Call(
			0,
			0,
			uintptr(unsafe.Pointer(&entries[0])),
			uintptr(unsafe.Pointer(&bufferSize)),
			uintptr(unsafe.Pointer(&entryCount)),
		)
	}
	if result != 0 {
		return nil, fmt.Errorf("枚举 VPN/拨号连接失败：错误码 %d", result)
	}
	if entryCount > uint32(len(entries)) {
		entryCount = uint32(len(entries))
	}
	names := make([]string, 0, entryCount)
	seen := make(map[string]bool, entryCount)
	for index := 0; index < int(entryCount); index++ {
		name := strings.TrimSpace(syscall.UTF16ToString(entries[index].entryName[:]))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func setWinINetOptionsForConnection(internetSetOption *syscall.LazyProc, connection uintptr, options []internetPerConnOption) error {
	list := internetPerConnOptionList{
		size:        uint32(unsafe.Sizeof(internetPerConnOptionList{})),
		connection:  connection,
		optionCount: uint32(len(options)),
		options:     uintptr(unsafe.Pointer(&options[0])),
	}
	ok, _, callErr := internetSetOption.Call(0, uintptr(internetOptionPerConnectionOption), uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(options)
	if ok == 0 {
		return fmt.Errorf("写入 WinINet 系统代理失败：%w", callErr)
	}
	return nil
}

func refreshWinINetProxy(internetSetOption *syscall.LazyProc) error {
	changed, _, _ := internetSetOption.Call(0, uintptr(internetOptionSettingsChanged), 0, 0)
	refreshed, _, _ := internetSetOption.Call(0, uintptr(internetOptionRefresh), 0, 0)
	if changed == 0 || refreshed == 0 {
		return errors.New("刷新 WinINet 系统代理失败")
	}
	broadcastSystemProxyChange()
	return nil
}

func broadcastSystemProxyChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendNotifyMessage := user32.NewProc("SendNotifyMessageW")
	if err := user32.Load(); err != nil {
		return
	}
	setting, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return
	}
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
	)
	_, _, _ = sendNotifyMessage.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(setting)),
	)
	runtime.KeepAlive(setting)
}
