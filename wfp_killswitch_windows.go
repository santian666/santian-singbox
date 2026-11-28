//go:build windows

package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Filtering Platform (WFP) constants verified against Windows SDK
// fwptypes.h / fwpmtypes.h / fwpmu.h (amd64 LLP64 ABI).
const (
	wfpRPCAuthnWinNT   uint32 = 10         // RPC_C_AUTHN_WINNT
	wfpRPCAuthnDefault uint32 = 0xffffffff // RPC_C_AUTHN_DEFAULT

	wfpSessionFlagDynamic uint32 = 0x00000001 // FWPM_SESSION_FLAG_DYNAMIC

	wfpFilterFlagNone             uint32 = 0x00000000 // FWPM_FILTER_FLAG_NONE (hard action)
	wfpFilterFlagClearActionRight uint32 = 0x00000008 // FWPM_FILTER_FLAG_CLEAR_ACTION_RIGHT (soft action)

	wfpActionFlagTerminating uint32 = 0x00001000
	wfpActionBlock           uint32 = 0x00000001 | wfpActionFlagTerminating // FWP_ACTION_BLOCK  (0x1001)
	wfpActionPermit          uint32 = 0x00000002 | wfpActionFlagTerminating // FWP_ACTION_PERMIT (0x1002)

	wfpDataTypeEmpty    uint32 = 0     // FWP_EMPTY
	wfpDataTypeUint8    uint32 = 1     // FWP_UINT8
	wfpDataTypeUint16   uint32 = 2     // FWP_UINT16
	wfpDataTypeUint32   uint32 = 3     // FWP_UINT32
	wfpDataTypeByteBlob uint32 = 12    // FWP_BYTE_BLOB_TYPE
	wfpDataTypeV4Mask   uint32 = 0x100 // FWP_V4_ADDR_MASK
	wfpDataTypeV6Mask   uint32 = 0x101 // FWP_V6_ADDR_MASK

	wfpMatchEqual       uint32 = 0 // FWP_MATCH_EQUAL
	wfpMatchFlagsAllSet uint32 = 6 // FWP_MATCH_FLAGS_ALL_SET

	wfpConditionFlagIsLoopback uint32 = 0x00000001 // FWP_CONDITION_FLAG_IS_LOOPBACK

	wfpIPProtoTCP uint8 = 6
	wfpIPProtoUDP uint8 = 17

	// Explicit FWP_UINT8 filter weights (WFP valid range: 0..15).
	wfpWeightAllowPriority uint8 = 15 // Loopback, sing-box.exe, SanTian-Singbox.exe, TUN local IP
	wfpWeightBlockDNSLeak  uint8 = 12 // Block non-TUN / non-core UDP & TCP port 53 (stops SMHNR / router DNS leak)
	wfpWeightAllowLocalNet uint8 = 10 // Private LAN CIDRs, local multicast/broadcast, DHCP v4/v6
	wfpWeightBlockCatchAll uint8 = 1  // Default catch-all outbound block

	wfpSublayerWeightMax uint16 = 0xffff

	wfpTunLocalIPv4 = "172.19.0.1/32"
	wfpTunLocalIPv6 = "fdfe:dcba:9876::1/128"
)

// Official WFP Layer and Condition GUIDs from Windows SDK fwpmu.h.
var (
	// {c38d57d1-05a7-4c33-904f-7fbceee60e82}
	wfpLayerALEAuthConnectV4 = windows.GUID{
		Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33,
		Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82},
	}
	// {4a72393b-319f-44bc-84c3-ba54dcb3b6b4}
	wfpLayerALEAuthConnectV6 = windows.GUID{
		Data1: 0x4a72393b, Data2: 0x319f, Data3: 0x44bc,
		Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4},
	}
	// {d78e1e87-8644-4ea5-9437-d809ecefc971}
	wfpConditionALEAppID = windows.GUID{
		Data1: 0xd78e1e87, Data2: 0x8644, Data3: 0x4ea5,
		Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71},
	}
	// {d9ee00de-c1ef-4617-bfe3-ffd8f5a08957}
	wfpConditionIPLocalAddress = windows.GUID{
		Data1: 0xd9ee00de, Data2: 0xc1ef, Data3: 0x4617,
		Data4: [8]byte{0xbf, 0xe3, 0xff, 0xd8, 0xf5, 0xa0, 0x89, 0x57},
	}
	// {b235ae9a-1d64-49b8-a44c-5ff3d9095045}
	wfpConditionIPRemoteAddress = windows.GUID{
		Data1: 0xb235ae9a, Data2: 0x1d64, Data3: 0x49b8,
		Data4: [8]byte{0xa4, 0x4c, 0x5f, 0xf3, 0xd9, 0x09, 0x50, 0x45},
	}
	// {632ce23b-5167-435c-86d7-e903684aa80c}
	wfpConditionFlags = windows.GUID{
		Data1: 0x632ce23b, Data2: 0x5167, Data3: 0x435c,
		Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c},
	}
	// {3971ef2b-623e-4f9a-8cb1-6e79b806b9a7}
	wfpConditionIPProtocol = windows.GUID{
		Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a,
		Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7},
	}
	// {0c1ba1af-5765-453f-af22-a8f791ac775b}
	wfpConditionIPLocalPort = windows.GUID{
		Data1: 0x0c1ba1af, Data2: 0x5765, Data3: 0x453f,
		Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b},
	}
	// {c35a604d-d22b-4e1a-91b4-68f674ee674b}
	wfpConditionIPRemotePort = windows.GUID{
		Data1: 0xc35a604d, Data2: 0xd22b, Data3: 0x4e1a,
		Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b},
	}
)

var (
	wfpLocalAllowedIPv4CIDRs = []string{
		"10.0.0.0/8",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"224.0.0.0/24",
		"239.255.255.250/32",
		"255.255.255.255/32",
	}
	wfpLocalAllowedIPv6CIDRs = []string{
		"fc00::/7",
		"fe80::/10",
		"ff02::/16",
	}
)

// ABI-matched Windows amd64 WFP C structures.
// Field sizes and offsets are verified in wfp_killswitch_windows_test.go.

type wfpDisplayData0 struct {
	name        *uint16
	description *uint16
}

type wfpByteBlob struct {
	size uint32
	data *byte
}

type wfpValue0 struct {
	typ   uint32
	value uintptr
}

type wfpConditionValue0 struct {
	typ   uint32
	value uintptr
}

type wfpFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      uint32
	conditionValue wfpConditionValue0
}

type wfpAction0 struct {
	typ        uint32
	filterType windows.GUID
}

type wfpSession0 struct {
	sessionKey           windows.GUID
	displayData          wfpDisplayData0
	flags                uint32
	txnWaitTimeoutInMSec uint32
	processID            uint32
	sid                  *windows.SID
	username             *uint16
	kernelMode           int32
}

type wfpSublayer0 struct {
	subLayerKey  windows.GUID
	displayData  wfpDisplayData0
	flags        uint32
	providerKey  *windows.GUID
	providerData wfpByteBlob
	weight       uint16
}

type wfpFilter0 struct {
	filterKey           windows.GUID
	displayData         wfpDisplayData0
	flags               uint32
	providerKey         *windows.GUID
	providerData        wfpByteBlob
	layerKey            windows.GUID
	subLayerKey         windows.GUID
	weight              wfpValue0
	numFilterConditions uint32
	filterCondition     *wfpFilterCondition0
	action              wfpAction0
	// Anonymous C union { UINT64 rawContext; GUID providerContextKey; }.
	// Using [2]uint64 preserves both 8-byte alignment (offset 152 on amd64)
	// and 16-byte union size, yielding total sizeof(FWPM_FILTER0) == 200.
	rawContext      [2]uint64
	reserved        *windows.GUID
	filterID        uint64
	effectiveWeight wfpValue0
}

type wfpV4AddrAndMask struct {
	addr uint32 // Host byte order (per Microsoft FWP_V4_ADDR_AND_MASK spec)
	mask uint32 // Host byte order
}

type wfpV6AddrAndMask struct {
	addr         [16]byte // Network byte order
	prefixLength uint8
}

// wfpBackend abstracts native fwpuclnt.dll syscalls for deterministic unit testing.
type wfpBackend interface {
	EngineOpen(session *wfpSession0) (uintptr, error)
	EngineClose(engine uintptr) error
	TransactionBegin(engine uintptr) error
	TransactionCommit(engine uintptr) error
	TransactionAbort(engine uintptr) error
	SubLayerAdd(engine uintptr, sublayer *wfpSublayer0) error
	FilterAdd(engine uintptr, filter *wfpFilter0) (uint64, error)
	GetAppIDFromFileName(path string) (*wfpByteBlob, error)
	FreeMemory(blob **wfpByteBlob)
}

var (
	fwpuclntDLL                    = windows.NewLazySystemDLL("fwpuclnt.dll")
	procFwpmEngineOpen0            = fwpuclntDLL.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0           = fwpuclntDLL.NewProc("FwpmEngineClose0")
	procFwpmTransactionBegin0      = fwpuclntDLL.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0     = fwpuclntDLL.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort0      = fwpuclntDLL.NewProc("FwpmTransactionAbort0")
	procFwpmSubLayerAdd0           = fwpuclntDLL.NewProc("FwpmSubLayerAdd0")
	procFwpmFilterAdd0             = fwpuclntDLL.NewProc("FwpmFilterAdd0")
	procFwpmGetAppIdFromFileName0  = fwpuclntDLL.NewProc("FwpmGetAppIdFromFileName0")
	procFwpmFreeMemory0            = fwpuclntDLL.NewProc("FwpmFreeMemory0")
)

type wfpNativeBackend struct{}

func (wfpNativeBackend) EngineOpen(session *wfpSession0) (uintptr, error) {
	if err := procFwpmEngineOpen0.Find(); err != nil {
		return 0, fmt.Errorf("定位 FwpmEngineOpen0 失败: %w", err)
	}
	var handle uintptr
	r1, _, _ := procFwpmEngineOpen0.Call(
		0,
		uintptr(wfpRPCAuthnWinNT),
		0,
		uintptr(unsafe.Pointer(session)),
		uintptr(unsafe.Pointer(&handle)),
	)
	runtime.KeepAlive(session)
	if r1 != 0 {
		return 0, fmt.Errorf("FwpmEngineOpen0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	if handle == 0 {
		return 0, errors.New("FwpmEngineOpen0 返回空句柄")
	}
	return handle, nil
}

func (wfpNativeBackend) EngineClose(engine uintptr) error {
	if engine == 0 {
		return nil
	}
	if err := procFwpmEngineClose0.Find(); err != nil {
		return fmt.Errorf("定位 FwpmEngineClose0 失败: %w", err)
	}
	r1, _, _ := procFwpmEngineClose0.Call(engine)
	if r1 != 0 {
		return fmt.Errorf("FwpmEngineClose0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return nil
}

func (wfpNativeBackend) TransactionBegin(engine uintptr) error {
	if err := procFwpmTransactionBegin0.Find(); err != nil {
		return fmt.Errorf("定位 FwpmTransactionBegin0 失败: %w", err)
	}
	r1, _, _ := procFwpmTransactionBegin0.Call(engine, 0)
	if r1 != 0 {
		return fmt.Errorf("FwpmTransactionBegin0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return nil
}

func (wfpNativeBackend) TransactionCommit(engine uintptr) error {
	if err := procFwpmTransactionCommit0.Find(); err != nil {
		return fmt.Errorf("定位 FwpmTransactionCommit0 失败: %w", err)
	}
	r1, _, _ := procFwpmTransactionCommit0.Call(engine)
	if r1 != 0 {
		return fmt.Errorf("FwpmTransactionCommit0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return nil
}

func (wfpNativeBackend) TransactionAbort(engine uintptr) error {
	if err := procFwpmTransactionAbort0.Find(); err != nil {
		return fmt.Errorf("定位 FwpmTransactionAbort0 失败: %w", err)
	}
	r1, _, _ := procFwpmTransactionAbort0.Call(engine)
	if r1 != 0 {
		return fmt.Errorf("FwpmTransactionAbort0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return nil
}

func (wfpNativeBackend) SubLayerAdd(engine uintptr, sublayer *wfpSublayer0) error {
	if err := procFwpmSubLayerAdd0.Find(); err != nil {
		return fmt.Errorf("定位 FwpmSubLayerAdd0 失败: %w", err)
	}
	r1, _, _ := procFwpmSubLayerAdd0.Call(
		engine,
		uintptr(unsafe.Pointer(sublayer)),
		0,
	)
	runtime.KeepAlive(sublayer)
	if r1 != 0 {
		return fmt.Errorf("FwpmSubLayerAdd0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return nil
}

func (wfpNativeBackend) FilterAdd(engine uintptr, filter *wfpFilter0) (uint64, error) {
	if err := procFwpmFilterAdd0.Find(); err != nil {
		return 0, fmt.Errorf("定位 FwpmFilterAdd0 失败: %w", err)
	}
	var filterID uint64
	r1, _, _ := procFwpmFilterAdd0.Call(
		engine,
		uintptr(unsafe.Pointer(filter)),
		0,
		uintptr(unsafe.Pointer(&filterID)),
	)
	runtime.KeepAlive(filter)
	if r1 != 0 {
		return 0, fmt.Errorf("FwpmFilterAdd0 失败: 0x%08x (%w)", uint32(r1), windows.Errno(r1))
	}
	return filterID, nil
}

func (wfpNativeBackend) GetAppIDFromFileName(path string) (*wfpByteBlob, error) {
	if err := procFwpmGetAppIdFromFileName0.Find(); err != nil {
		return nil, fmt.Errorf("定位 FwpmGetAppIdFromFileName0 失败: %w", err)
	}
	utf16Path, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("转换应用路径失败 (%s): %w", path, err)
	}
	var blob *wfpByteBlob
	r1, _, _ := procFwpmGetAppIdFromFileName0.Call(
		uintptr(unsafe.Pointer(utf16Path)),
		uintptr(unsafe.Pointer(&blob)),
	)
	runtime.KeepAlive(utf16Path)
	if r1 != 0 {
		return nil, fmt.Errorf("FwpmGetAppIdFromFileName0 失败 (%s): 0x%08x (%w)", path, uint32(r1), windows.Errno(r1))
	}
	if blob == nil || blob.size == 0 || blob.data == nil {
		return nil, fmt.Errorf("FwpmGetAppIdFromFileName0 返回空 AppID (%s)", path)
	}
	return blob, nil
}

func (wfpNativeBackend) FreeMemory(blob **wfpByteBlob) {
	if blob == nil || *blob == nil {
		return
	}
	if err := procFwpmFreeMemory0.Find(); err != nil {
		return
	}
	_, _, _ = procFwpmFreeMemory0.Call(uintptr(unsafe.Pointer(blob)))
}

type wfpKillSwitchState struct {
	mu          sync.Mutex
	active      bool
	engine      uintptr
	sublayerKey windows.GUID
	corePath    string
	appPath     string
	filterCount int
}

var (
	wfpKS              = &wfpKillSwitchState{}
	activeWFPBackend   wfpBackend = wfpNativeBackend{}
)

func shouldUseAlwaysOnWFPKillSwitch(settings Settings) bool {
	normalizeSettings(&settings)
	return runtime.GOOS == "windows" && settings.Tun && settings.CircuitBreaker
}

func normalizeWFPExecutablePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil && strings.TrimSpace(resolved) != "" {
		trimmed = resolved
	}
	return filepath.Clean(trimmed)
}

func newRandomWFPGUID() (windows.GUID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return windows.GUID{}, err
	}
	// RFC 4122 version 4 UUID bits
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var g windows.GUID
	g.Data1 = binary.LittleEndian.Uint32(raw[0:4])
	g.Data2 = binary.LittleEndian.Uint16(raw[4:6])
	g.Data3 = binary.LittleEndian.Uint16(raw[6:8])
	copy(g.Data4[:], raw[8:16])
	return g, nil
}

func parseWFPV4AddrMask(cidr string) (wfpV4AddrAndMask, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return wfpV4AddrAndMask{}, fmt.Errorf("无效 IPv4 CIDR %q: %w", cidr, err)
	}
	addr := prefix.Addr().Unmap()
	if !addr.Is4() {
		return wfpV4AddrAndMask{}, fmt.Errorf("期望 IPv4 CIDR，实际为 %q", cidr)
	}
	bits := prefix.Bits()
	var mask uint32
	if bits == 0 {
		mask = 0
	} else {
		mask = ^uint32(0) << uint(32-bits)
	}
	raw := addr.As4()
	hostAddr := binary.BigEndian.Uint32(raw[:]) & mask
	return wfpV4AddrAndMask{
		addr: hostAddr,
		mask: mask,
	}, nil
}

func parseWFPV6AddrMask(cidr string) (wfpV6AddrAndMask, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return wfpV6AddrAndMask{}, fmt.Errorf("无效 IPv6 CIDR %q: %w", cidr, err)
	}
	addr := prefix.Addr()
	if !addr.Is6() || addr.Is4In6() {
		return wfpV6AddrAndMask{}, fmt.Errorf("期望 IPv6 CIDR，实际为 %q", cidr)
	}
	return wfpV6AddrAndMask{
		addr:         prefix.Masked().Addr().As16(),
		prefixLength: uint8(prefix.Bits()),
	}, nil
}

func enableWFPKillSwitch(logger *Logger, corePath string, appPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if !checkAdminPrivileges() {
		return errors.New("缺少管理员权限，无法启动 WFP 内核防泄露")
	}

	resolvedCore := normalizeWFPExecutablePath(corePath)
	if resolvedCore == "" {
		return errors.New("缺少代理核心路径，中止启用 WFP 防泄露以防锁死核心出网")
	}
	resolvedApp := normalizeWFPExecutablePath(appPath)
	if resolvedApp == "" {
		if exe, err := os.Executable(); err == nil {
			resolvedApp = normalizeWFPExecutablePath(exe)
		}
	}
	if resolvedApp == "" {
		return errors.New("缺少主程序路径，中止启用 WFP 防泄露")
	}

	wfpKS.mu.Lock()
	defer wfpKS.mu.Unlock()

	if wfpKS.active && wfpKS.engine != 0 &&
		strings.EqualFold(wfpKS.corePath, resolvedCore) &&
		strings.EqualFold(wfpKS.appPath, resolvedApp) {
		return nil
	}

	backend := activeWFPBackend
	if backend == nil {
		return errors.New("WFP 后端未初始化")
	}

	coreBlob, err := backend.GetAppIDFromFileName(resolvedCore)
	if err != nil {
		return fmt.Errorf("解析代理核心 WFP AppID 失败: %w", err)
	}
	defer backend.FreeMemory(&coreBlob)

	var appBlob *wfpByteBlob
	if !strings.EqualFold(resolvedApp, resolvedCore) {
		appBlob, err = backend.GetAppIDFromFileName(resolvedApp)
		if err != nil {
			return fmt.Errorf("解析主程序 WFP AppID 失败: %w", err)
		}
		defer backend.FreeMemory(&appBlob)
	}

	sublayerKey, err := newRandomWFPGUID()
	if err != nil {
		return fmt.Errorf("生成 WFP 子层标识失败: %w", err)
	}

	sessionName, err := windows.UTF16PtrFromString("SanTian-Singbox WFP Dynamic Anti-Leak Session")
	if err != nil {
		return err
	}
	sessionDesc, err := windows.UTF16PtrFromString("Kernel RAM dynamic anti-leak session (auto-cleaned on exit or reboot)")
	if err != nil {
		return err
	}
	session := wfpSession0{
		displayData: wfpDisplayData0{
			name:        sessionName,
			description: sessionDesc,
		},
		flags: wfpSessionFlagDynamic,
	}

	engine, err := backend.EngineOpen(&session)
	runtime.KeepAlive(sessionName)
	runtime.KeepAlive(sessionDesc)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = backend.TransactionAbort(engine)
			_ = backend.EngineClose(engine)
		}
	}()

	if err := backend.TransactionBegin(engine); err != nil {
		return err
	}

	subName, err := windows.UTF16PtrFromString("SanTian-Singbox WFP Anti-Leak Sublayer")
	if err != nil {
		return err
	}
	subDesc, err := windows.UTF16PtrFromString("Dynamic TUN anti-leak filter container")
	if err != nil {
		return err
	}
	sublayer := wfpSublayer0{
		subLayerKey: sublayerKey,
		displayData: wfpDisplayData0{
			name:        subName,
			description: subDesc,
		},
		weight: wfpSublayerWeightMax,
	}
	if err := backend.SubLayerAdd(engine, &sublayer); err != nil {
		return err
	}
	runtime.KeepAlive(subName)
	runtime.KeepAlive(subDesc)

	filterCount, err := installWFPKillSwitchFilters(backend, engine, sublayerKey, coreBlob, appBlob)
	if err != nil {
		return err
	}

	if err := backend.TransactionCommit(engine); err != nil {
		return err
	}
	committed = true

	// Replace any older session atomically after the new dynamic session is live.
	oldEngine := wfpKS.engine
	wfpKS.active = true
	wfpKS.engine = engine
	wfpKS.sublayerKey = sublayerKey
	wfpKS.corePath = resolvedCore
	wfpKS.appPath = resolvedApp
	wfpKS.filterCount = filterCount
	if oldEngine != 0 && oldEngine != engine {
		_ = backend.EngineClose(oldEngine)
	}

	if logger != nil {
		logger.Info("【WFP 内核防泄露】已启用动态内存防护会话（%d 条内核规则，纯内存运行：0ms 零泄漏拦截 + 断电/重启自动释放不锁网）", filterCount)
	}
	return nil
}

func installWFPKillSwitchFilters(
	backend wfpBackend,
	engine uintptr,
	sublayerKey windows.GUID,
	coreBlob *wfpByteBlob,
	appBlob *wfpByteBlob,
) (int, error) {
	count := 0
	addFilter := func(name string, layer windows.GUID, weight uint8, actionType uint32, conds []wfpFilterCondition0, keepAlive ...any) error {
		namePtr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		flags := wfpFilterFlagNone
		if actionType == wfpActionPermit {
			// Soft permit in our sublayer so Windows Defender Firewall's own blocks remain respected;
			// BLOCK filters leave CLEAR_ACTION_RIGHT unset (hard block) so default AllowOutbound cannot override them.
			flags = wfpFilterFlagClearActionRight
		}
		var condPtr *wfpFilterCondition0
		if len(conds) > 0 {
			condPtr = &conds[0]
		}
		filter := wfpFilter0{
			displayData: wfpDisplayData0{
				name: namePtr,
			},
			flags:       flags,
			layerKey:    layer,
			subLayerKey: sublayerKey,
			weight: wfpValue0{
				typ:   wfpDataTypeUint8,
				value: uintptr(weight),
			},
			numFilterConditions: uint32(len(conds)),
			filterCondition:     condPtr,
			action: wfpAction0{
				typ: actionType,
			},
		}
		_, err = backend.FilterAdd(engine, &filter)
		runtime.KeepAlive(namePtr)
		runtime.KeepAlive(conds)
		runtime.KeepAlive(keepAlive)
		if err != nil {
			return fmt.Errorf("添加 WFP 规则 [%s] 失败: %w", name, err)
		}
		count++
		return nil
	}

	// 1. Weight 15 (Highest): Loopback (127.0.0.1 / ::1) on V4 & V6
	loopbackCond := []wfpFilterCondition0{
		{
			fieldKey:  wfpConditionFlags,
			matchType: wfpMatchFlagsAllSet,
			conditionValue: wfpConditionValue0{
				typ:   wfpDataTypeUint32,
				value: uintptr(wfpConditionFlagIsLoopback),
			},
		},
	}
	if err := addFilter("SanTian WFP Permit Loopback V4", wfpLayerALEAuthConnectV4, wfpWeightAllowPriority, wfpActionPermit, loopbackCond); err != nil {
		return count, err
	}
	if err := addFilter("SanTian WFP Permit Loopback V6", wfpLayerALEAuthConnectV6, wfpWeightAllowPriority, wfpActionPermit, loopbackCond); err != nil {
		return count, err
	}

	// 2. Weight 15: Core sing-box.exe (V4 & V6)
	coreCond := []wfpFilterCondition0{
		{
			fieldKey:  wfpConditionALEAppID,
			matchType: wfpMatchEqual,
			conditionValue: wfpConditionValue0{
				typ:   wfpDataTypeByteBlob,
				value: uintptr(unsafe.Pointer(coreBlob)),
			},
		},
	}
	if err := addFilter("SanTian WFP Permit Core V4", wfpLayerALEAuthConnectV4, wfpWeightAllowPriority, wfpActionPermit, coreCond, coreBlob); err != nil {
		return count, err
	}
	if err := addFilter("SanTian WFP Permit Core V6", wfpLayerALEAuthConnectV6, wfpWeightAllowPriority, wfpActionPermit, coreCond, coreBlob); err != nil {
		return count, err
	}

	// 3. Weight 15: GUI SanTian-Singbox.exe (V4 & V6)
	if appBlob != nil {
		appCond := []wfpFilterCondition0{
			{
				fieldKey:  wfpConditionALEAppID,
				matchType: wfpMatchEqual,
				conditionValue: wfpConditionValue0{
					typ:   wfpDataTypeByteBlob,
					value: uintptr(unsafe.Pointer(appBlob)),
				},
			},
		}
		if err := addFilter("SanTian WFP Permit App V4", wfpLayerALEAuthConnectV4, wfpWeightAllowPriority, wfpActionPermit, appCond, appBlob); err != nil {
			return count, err
		}
		if err := addFilter("SanTian WFP Permit App V6", wfpLayerALEAuthConnectV6, wfpWeightAllowPriority, wfpActionPermit, appCond, appBlob); err != nil {
			return count, err
		}
	}

	// 4. Weight 15: SanTian TUN interface local addresses (172.19.0.1/32 & fdfe:dcba:9876::1/128).
	// When sing-box crashes and Wintun tears down SanTian, these source IPs vanish immediately -> 0ms drop on physical NICs.
	tunV4, err := parseWFPV4AddrMask(wfpTunLocalIPv4)
	if err != nil {
		return count, err
	}
	tunV4Cond := []wfpFilterCondition0{
		{
			fieldKey:  wfpConditionIPLocalAddress,
			matchType: wfpMatchEqual,
			conditionValue: wfpConditionValue0{
				typ:   wfpDataTypeV4Mask,
				value: uintptr(unsafe.Pointer(&tunV4)),
			},
		},
	}
	if err := addFilter("SanTian WFP Permit TUN Local V4", wfpLayerALEAuthConnectV4, wfpWeightAllowPriority, wfpActionPermit, tunV4Cond, &tunV4); err != nil {
		return count, err
	}

	tunV6, err := parseWFPV6AddrMask(wfpTunLocalIPv6)
	if err != nil {
		return count, err
	}
	tunV6Cond := []wfpFilterCondition0{
		{
			fieldKey:  wfpConditionIPLocalAddress,
			matchType: wfpMatchEqual,
			conditionValue: wfpConditionValue0{
				typ:   wfpDataTypeV6Mask,
				value: uintptr(unsafe.Pointer(&tunV6)),
			},
		},
	}
	if err := addFilter("SanTian WFP Permit TUN Local V6", wfpLayerALEAuthConnectV6, wfpWeightAllowPriority, wfpActionPermit, tunV6Cond, &tunV6); err != nil {
		return count, err
	}

	// 5. Weight 12: Anti-DNS-Leak Guard (blocks UDP/TCP port 53 outside TUN/Core/Loopback, even to LAN router 192.168.1.1:53)
	for _, proto := range []struct {
		name string
		num  uint8
	}{
		{"UDP", wfpIPProtoUDP},
		{"TCP", wfpIPProtoTCP},
	} {
		dnsCond := []wfpFilterCondition0{
			{
				fieldKey:  wfpConditionIPProtocol,
				matchType: wfpMatchEqual,
				conditionValue: wfpConditionValue0{
					typ:   wfpDataTypeUint8,
					value: uintptr(proto.num),
				},
			},
			{
				fieldKey:  wfpConditionIPRemotePort,
				matchType: wfpMatchEqual,
				conditionValue: wfpConditionValue0{
					typ:   wfpDataTypeUint16,
					value: uintptr(53),
				},
			},
		}
		if err := addFilter("SanTian WFP Block Non-TUN DNS "+proto.name+" V4", wfpLayerALEAuthConnectV4, wfpWeightBlockDNSLeak, wfpActionBlock, dnsCond); err != nil {
			return count, err
		}
		if err := addFilter("SanTian WFP Block Non-TUN DNS "+proto.name+" V6", wfpLayerALEAuthConnectV6, wfpWeightBlockDNSLeak, wfpActionBlock, dnsCond); err != nil {
			return count, err
		}
	}

	// 6. Weight 10: Private LAN, link-local, local multicast/broadcast
	for _, cidr := range wfpLocalAllowedIPv4CIDRs {
		v4Mask, err := parseWFPV4AddrMask(cidr)
		if err != nil {
			return count, err
		}
		cond := []wfpFilterCondition0{
			{
				fieldKey:  wfpConditionIPRemoteAddress,
				matchType: wfpMatchEqual,
				conditionValue: wfpConditionValue0{
					typ:   wfpDataTypeV4Mask,
					value: uintptr(unsafe.Pointer(&v4Mask)),
				},
			},
		}
		if err := addFilter("SanTian WFP Permit LAN V4 "+cidr, wfpLayerALEAuthConnectV4, wfpWeightAllowLocalNet, wfpActionPermit, cond, &v4Mask); err != nil {
			return count, err
		}
	}
	for _, cidr := range wfpLocalAllowedIPv6CIDRs {
		v6Mask, err := parseWFPV6AddrMask(cidr)
		if err != nil {
			return count, err
		}
		cond := []wfpFilterCondition0{
			{
				fieldKey:  wfpConditionIPRemoteAddress,
				matchType: wfpMatchEqual,
				conditionValue: wfpConditionValue0{
					typ:   wfpDataTypeV6Mask,
					value: uintptr(unsafe.Pointer(&v6Mask)),
				},
			},
		}
		if err := addFilter("SanTian WFP Permit LAN V6 "+cidr, wfpLayerALEAuthConnectV6, wfpWeightAllowLocalNet, wfpActionPermit, cond, &v6Mask); err != nil {
			return count, err
		}
	}

	// 7. Weight 10: DHCPv4 (UDP 68 -> 67) & DHCPv6 (UDP 546 -> 547) so physical NIC leases never expire
	dhcpV4Cond := []wfpFilterCondition0{
		{
			fieldKey:       wfpConditionIPProtocol,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint8, value: uintptr(wfpIPProtoUDP)},
		},
		{
			fieldKey:       wfpConditionIPLocalPort,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint16, value: uintptr(68)},
		},
		{
			fieldKey:       wfpConditionIPRemotePort,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint16, value: uintptr(67)},
		},
	}
	if err := addFilter("SanTian WFP Permit DHCPv4", wfpLayerALEAuthConnectV4, wfpWeightAllowLocalNet, wfpActionPermit, dhcpV4Cond); err != nil {
		return count, err
	}

	dhcpV6Cond := []wfpFilterCondition0{
		{
			fieldKey:       wfpConditionIPProtocol,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint8, value: uintptr(wfpIPProtoUDP)},
		},
		{
			fieldKey:       wfpConditionIPLocalPort,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint16, value: uintptr(546)},
		},
		{
			fieldKey:       wfpConditionIPRemotePort,
			matchType:      wfpMatchEqual,
			conditionValue: wfpConditionValue0{typ: wfpDataTypeUint16, value: uintptr(547)},
		},
	}
	if err := addFilter("SanTian WFP Permit DHCPv6", wfpLayerALEAuthConnectV6, wfpWeightAllowLocalNet, wfpActionPermit, dhcpV6Cond); err != nil {
		return count, err
	}

	// 8. Weight 1 (Lowest): Catch-all hard BLOCK for any remaining outbound IPv4 & IPv6 traffic
	if err := addFilter("SanTian WFP Block All Outbound V4", wfpLayerALEAuthConnectV4, wfpWeightBlockCatchAll, wfpActionBlock, nil); err != nil {
		return count, err
	}
	if err := addFilter("SanTian WFP Block All Outbound V6", wfpLayerALEAuthConnectV6, wfpWeightBlockCatchAll, wfpActionBlock, nil); err != nil {
		return count, err
	}

	return count, nil
}

func disableWFPKillSwitch(logger *Logger) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	wfpKS.mu.Lock()
	defer wfpKS.mu.Unlock()

	if !wfpKS.active && wfpKS.engine == 0 {
		return nil
	}

	backend := activeWFPBackend
	engine := wfpKS.engine
	wfpKS.active = false
	wfpKS.engine = 0
	wfpKS.sublayerKey = windows.GUID{}
	wfpKS.corePath = ""
	wfpKS.appPath = ""
	wfpKS.filterCount = 0

	if backend != nil && engine != 0 {
		if err := backend.EngineClose(engine); err != nil {
			if logger != nil {
				logger.Error("释放 WFP 动态防泄露会话失败: %v", err)
			}
			return err
		}
	}
	if logger != nil {
		logger.Info("【WFP 内核防泄露】已释放动态内存防护会话，系统网络恢复正常直连。")
	}
	return nil
}

func isWFPKillSwitchActive() bool {
	wfpKS.mu.Lock()
	defer wfpKS.mu.Unlock()
	return wfpKS.active && wfpKS.engine != 0
}
