//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWFPStructABILayoutAMD64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("ABI layout assertions target amd64, current arch=%s", runtime.GOARCH)
	}

	assertSize := func(name string, got, want uintptr) {
		t.Helper()
		if got != want {
			t.Fatalf("sizeof(%s) = %d, want %d", name, got, want)
		}
	}
	assertOffset := func(name string, got, want uintptr) {
		t.Helper()
		if got != want {
			t.Fatalf("offsetof(%s) = %d, want %d", name, got, want)
		}
	}

	assertSize("wfpDisplayData0", unsafe.Sizeof(wfpDisplayData0{}), 16)
	assertOffset("wfpDisplayData0.name", unsafe.Offsetof(wfpDisplayData0{}.name), 0)
	assertOffset("wfpDisplayData0.description", unsafe.Offsetof(wfpDisplayData0{}.description), 8)

	assertSize("wfpByteBlob", unsafe.Sizeof(wfpByteBlob{}), 16)
	assertOffset("wfpByteBlob.size", unsafe.Offsetof(wfpByteBlob{}.size), 0)
	assertOffset("wfpByteBlob.data", unsafe.Offsetof(wfpByteBlob{}.data), 8)

	assertSize("wfpValue0", unsafe.Sizeof(wfpValue0{}), 16)
	assertOffset("wfpValue0.typ", unsafe.Offsetof(wfpValue0{}.typ), 0)
	assertOffset("wfpValue0.value", unsafe.Offsetof(wfpValue0{}.value), 8)

	assertSize("wfpConditionValue0", unsafe.Sizeof(wfpConditionValue0{}), 16)
	assertOffset("wfpConditionValue0.typ", unsafe.Offsetof(wfpConditionValue0{}.typ), 0)
	assertOffset("wfpConditionValue0.value", unsafe.Offsetof(wfpConditionValue0{}.value), 8)

	assertSize("wfpFilterCondition0", unsafe.Sizeof(wfpFilterCondition0{}), 40)
	assertOffset("wfpFilterCondition0.fieldKey", unsafe.Offsetof(wfpFilterCondition0{}.fieldKey), 0)
	assertOffset("wfpFilterCondition0.matchType", unsafe.Offsetof(wfpFilterCondition0{}.matchType), 16)
	assertOffset("wfpFilterCondition0.conditionValue", unsafe.Offsetof(wfpFilterCondition0{}.conditionValue), 24)

	assertSize("wfpAction0", unsafe.Sizeof(wfpAction0{}), 20)
	assertOffset("wfpAction0.typ", unsafe.Offsetof(wfpAction0{}.typ), 0)
	assertOffset("wfpAction0.filterType", unsafe.Offsetof(wfpAction0{}.filterType), 4)

	assertSize("wfpSession0", unsafe.Sizeof(wfpSession0{}), 72)
	assertOffset("wfpSession0.sessionKey", unsafe.Offsetof(wfpSession0{}.sessionKey), 0)
	assertOffset("wfpSession0.displayData", unsafe.Offsetof(wfpSession0{}.displayData), 16)
	assertOffset("wfpSession0.flags", unsafe.Offsetof(wfpSession0{}.flags), 32)
	assertOffset("wfpSession0.txnWaitTimeoutInMSec", unsafe.Offsetof(wfpSession0{}.txnWaitTimeoutInMSec), 36)
	assertOffset("wfpSession0.processID", unsafe.Offsetof(wfpSession0{}.processID), 40)
	assertOffset("wfpSession0.sid", unsafe.Offsetof(wfpSession0{}.sid), 48)
	assertOffset("wfpSession0.username", unsafe.Offsetof(wfpSession0{}.username), 56)
	assertOffset("wfpSession0.kernelMode", unsafe.Offsetof(wfpSession0{}.kernelMode), 64)

	assertSize("wfpSublayer0", unsafe.Sizeof(wfpSublayer0{}), 72)
	assertOffset("wfpSublayer0.subLayerKey", unsafe.Offsetof(wfpSublayer0{}.subLayerKey), 0)
	assertOffset("wfpSublayer0.displayData", unsafe.Offsetof(wfpSublayer0{}.displayData), 16)
	assertOffset("wfpSublayer0.flags", unsafe.Offsetof(wfpSublayer0{}.flags), 32)
	assertOffset("wfpSublayer0.providerKey", unsafe.Offsetof(wfpSublayer0{}.providerKey), 40)
	assertOffset("wfpSublayer0.providerData", unsafe.Offsetof(wfpSublayer0{}.providerData), 48)
	assertOffset("wfpSublayer0.weight", unsafe.Offsetof(wfpSublayer0{}.weight), 64)

	assertSize("wfpFilter0", unsafe.Sizeof(wfpFilter0{}), 200)
	assertOffset("wfpFilter0.filterKey", unsafe.Offsetof(wfpFilter0{}.filterKey), 0)
	assertOffset("wfpFilter0.displayData", unsafe.Offsetof(wfpFilter0{}.displayData), 16)
	assertOffset("wfpFilter0.flags", unsafe.Offsetof(wfpFilter0{}.flags), 32)
	assertOffset("wfpFilter0.providerKey", unsafe.Offsetof(wfpFilter0{}.providerKey), 40)
	assertOffset("wfpFilter0.providerData", unsafe.Offsetof(wfpFilter0{}.providerData), 48)
	assertOffset("wfpFilter0.layerKey", unsafe.Offsetof(wfpFilter0{}.layerKey), 64)
	assertOffset("wfpFilter0.subLayerKey", unsafe.Offsetof(wfpFilter0{}.subLayerKey), 80)
	assertOffset("wfpFilter0.weight", unsafe.Offsetof(wfpFilter0{}.weight), 96)
	assertOffset("wfpFilter0.numFilterConditions", unsafe.Offsetof(wfpFilter0{}.numFilterConditions), 112)
	assertOffset("wfpFilter0.filterCondition", unsafe.Offsetof(wfpFilter0{}.filterCondition), 120)
	assertOffset("wfpFilter0.action", unsafe.Offsetof(wfpFilter0{}.action), 128)
	assertOffset("wfpFilter0.rawContext", unsafe.Offsetof(wfpFilter0{}.rawContext), 152)
	assertOffset("wfpFilter0.reserved", unsafe.Offsetof(wfpFilter0{}.reserved), 168)
	assertOffset("wfpFilter0.filterID", unsafe.Offsetof(wfpFilter0{}.filterID), 176)
	assertOffset("wfpFilter0.effectiveWeight", unsafe.Offsetof(wfpFilter0{}.effectiveWeight), 184)

	assertSize("wfpV4AddrAndMask", unsafe.Sizeof(wfpV4AddrAndMask{}), 8)
	assertOffset("wfpV4AddrAndMask.addr", unsafe.Offsetof(wfpV4AddrAndMask{}.addr), 0)
	assertOffset("wfpV4AddrAndMask.mask", unsafe.Offsetof(wfpV4AddrAndMask{}.mask), 4)

	assertSize("wfpV6AddrAndMask", unsafe.Sizeof(wfpV6AddrAndMask{}), 17)
	assertOffset("wfpV6AddrAndMask.addr", unsafe.Offsetof(wfpV6AddrAndMask{}.addr), 0)
	assertOffset("wfpV6AddrAndMask.prefixLength", unsafe.Offsetof(wfpV6AddrAndMask{}.prefixLength), 16)
}

func TestWFPNativeDLLProcsAndRealAppIDLookup(t *testing.T) {
	procs := []*windows.LazyProc{
		procFwpmEngineOpen0,
		procFwpmEngineClose0,
		procFwpmTransactionBegin0,
		procFwpmTransactionCommit0,
		procFwpmTransactionAbort0,
		procFwpmSubLayerAdd0,
		procFwpmFilterAdd0,
		procFwpmGetAppIdFromFileName0,
		procFwpmFreeMemory0,
	}
	for _, p := range procs {
		if err := p.Find(); err != nil {
			t.Fatalf("fwpuclnt.dll proc %s lookup failed: %v", p.Name, err)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	backend := wfpNativeBackend{}
	blob, err := backend.GetAppIDFromFileName(exe)
	if err != nil {
		t.Fatalf("real FwpmGetAppIdFromFileName0 failed for %s: %v", exe, err)
	}
	if blob == nil || blob.size < 4 || blob.data == nil {
		t.Fatalf("unexpected empty blob from FwpmGetAppIdFromFileName0: %+v", blob)
	}
	raw := unsafe.Slice(blob.data, blob.size)
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = uint16(raw[2*i]) | (uint16(raw[2*i+1]) << 8)
	}
	ntPath := strings.TrimRight(string(utf16.Decode(u16)), "\x00")
	if !strings.HasPrefix(strings.ToLower(ntPath), `\device\`) {
		t.Fatalf("expected NT device path starting with \\device\\, got %q", ntPath)
	}

	backend.FreeMemory(&blob)
	if blob != nil {
		t.Fatalf("expected FwpmFreeMemory0 to zero pointer, got %v", blob)
	}
}

func TestWFPCIDRMaskHostByteOrder(t *testing.T) {
	tunV4, err := parseWFPV4AddrMask(wfpTunLocalIPv4)
	if err != nil {
		t.Fatalf("parseWFPV4AddrMask(%s) failed: %v", wfpTunLocalIPv4, err)
	}
	if tunV4.addr != 0xac130001 || tunV4.mask != 0xffffffff {
		t.Fatalf("got tunV4={0x%08x, 0x%08x}, want {0xac130001, 0xffffffff}", tunV4.addr, tunV4.mask)
	}

	lan10, err := parseWFPV4AddrMask("10.0.0.0/8")
	if err != nil {
		t.Fatalf("parseWFPV4AddrMask(10.0.0.0/8) failed: %v", err)
	}
	if lan10.addr != 0x0a000000 || lan10.mask != 0xff000000 {
		t.Fatalf("got lan10={0x%08x, 0x%08x}, want {0x0a000000, 0xff000000}", lan10.addr, lan10.mask)
	}

	tunV6, err := parseWFPV6AddrMask(wfpTunLocalIPv6)
	if err != nil {
		t.Fatalf("parseWFPV6AddrMask(%s) failed: %v", wfpTunLocalIPv6, err)
	}
	wantV6 := [16]byte{0xfd, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	if tunV6.addr != wantV6 || tunV6.prefixLength != 128 {
		t.Fatalf("got tunV6={%x /%d}, want {%x /128}", tunV6.addr, tunV6.prefixLength, wantV6)
	}
}

type recordedWFPFilter struct {
	name       string
	layer      windows.GUID
	sublayer   windows.GUID
	flags      uint32
	weightType uint32
	weightVal  uint8
	action     uint32
	conds      []wfpFilterCondition0
	v4Mask     *wfpV4AddrAndMask
	v6Mask     *wfpV6AddrAndMask
}

type fakeWFPBackend struct {
	openErr         error
	filterFailAt    int
	nextHandle      uintptr
	openCalls       int
	closeCalls      int
	txnBegins       int
	txnCommits      int
	txnAborts       int
	freeCalls       int
	allocatedBlobs  int
	sessionFlags    uint32
	sublayers       []wfpSublayer0
	filters         []recordedWFPFilter
	closedHandles   []uintptr
}

func (f *fakeWFPBackend) EngineOpen(session *wfpSession0) (uintptr, error) {
	f.openCalls++
	if session != nil {
		f.sessionFlags = session.flags
	}
	if f.openErr != nil {
		return 0, f.openErr
	}
	f.nextHandle++
	return f.nextHandle + 0x1000, nil
}

func (f *fakeWFPBackend) EngineClose(engine uintptr) error {
	f.closeCalls++
	f.closedHandles = append(f.closedHandles, engine)
	return nil
}

func (f *fakeWFPBackend) TransactionBegin(engine uintptr) error {
	f.txnBegins++
	return nil
}

func (f *fakeWFPBackend) TransactionCommit(engine uintptr) error {
	f.txnCommits++
	return nil
}

func (f *fakeWFPBackend) TransactionAbort(engine uintptr) error {
	f.txnAborts++
	return nil
}

func (f *fakeWFPBackend) SubLayerAdd(engine uintptr, sublayer *wfpSublayer0) error {
	if sublayer != nil {
		f.sublayers = append(f.sublayers, *sublayer)
	}
	return nil
}

func (f *fakeWFPBackend) FilterAdd(engine uintptr, filter *wfpFilter0) (uint64, error) {
	if f.filterFailAt > 0 && len(f.filters)+1 == f.filterFailAt {
		return 0, errors.New("injected filter add failure")
	}
	rec := recordedWFPFilter{
		name:       windows.UTF16PtrToString(filter.displayData.name),
		layer:      filter.layerKey,
		sublayer:   filter.subLayerKey,
		flags:      filter.flags,
		weightType: filter.weight.typ,
		weightVal:  uint8(filter.weight.value),
		action:     filter.action.typ,
	}
	if filter.numFilterConditions > 0 && filter.filterCondition != nil {
		slice := unsafe.Slice(filter.filterCondition, filter.numFilterConditions)
		rec.conds = append([]wfpFilterCondition0(nil), slice...)
		for i := range rec.conds {
			c := &rec.conds[i]
			if c.conditionValue.typ == wfpDataTypeV4Mask && c.conditionValue.value != 0 {
				ptr := *(*unsafe.Pointer)(unsafe.Pointer(&c.conditionValue.value))
				m := *(*wfpV4AddrAndMask)(ptr)
				rec.v4Mask = &m
			}
			if c.conditionValue.typ == wfpDataTypeV6Mask && c.conditionValue.value != 0 {
				ptr := *(*unsafe.Pointer)(unsafe.Pointer(&c.conditionValue.value))
				m := *(*wfpV6AddrAndMask)(ptr)
				rec.v6Mask = &m
			}
		}
	}
	f.filters = append(f.filters, rec)
	return uint64(len(f.filters)), nil
}

func (f *fakeWFPBackend) GetAppIDFromFileName(path string) (*wfpByteBlob, error) {
	f.allocatedBlobs++
	b := []byte(path)
	return &wfpByteBlob{size: uint32(len(b)), data: &b[0]}, nil
}

func (f *fakeWFPBackend) FreeMemory(blob **wfpByteBlob) {
	if blob != nil && *blob != nil {
		f.freeCalls++
		*blob = nil
	}
}

func resetWFPTestState(t *testing.T, backend wfpBackend, isAdmin bool) {
	t.Helper()
	origBackend := activeWFPBackend
	origAdmin := checkAdminPrivileges
	origRun := runFirewallCommand

	wfpKS.mu.Lock()
	wfpKS.active = false
	wfpKS.engine = 0
	wfpKS.sublayerKey = windows.GUID{}
	wfpKS.corePath = ""
	wfpKS.appPath = ""
	wfpKS.filterCount = 0
	wfpKS.mu.Unlock()

	activeWFPBackend = backend
	checkAdminPrivileges = func() bool { return isAdmin }

	t.Cleanup(func() {
		wfpKS.mu.Lock()
		wfpKS.active = false
		wfpKS.engine = 0
		wfpKS.sublayerKey = windows.GUID{}
		wfpKS.corePath = ""
		wfpKS.appPath = ""
		wfpKS.filterCount = 0
		wfpKS.mu.Unlock()
		activeWFPBackend = origBackend
		checkAdminPrivileges = origAdmin
		runFirewallCommand = origRun
	})
}

func TestWFPKillSwitchLifecycleAndFilterRules(t *testing.T) {
	fake := &fakeWFPBackend{}
	resetWFPTestState(t, fake, true)

	corePath := `C:\SanTian\bin\sing-box.exe`
	appPath := `C:\SanTian\SanTian-Singbox.exe`

	if err := enableWFPKillSwitch(nil, corePath, appPath); err != nil {
		t.Fatalf("enableWFPKillSwitch failed: %v", err)
	}
	if !isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch to be active")
	}
	if fake.sessionFlags != wfpSessionFlagDynamic {
		t.Fatalf("expected FWPM_SESSION_FLAG_DYNAMIC (0x1), got 0x%x", fake.sessionFlags)
	}
	if fake.txnBegins != 1 || fake.txnCommits != 1 || fake.txnAborts != 0 {
		t.Fatalf("unexpected txn counts: begin=%d commit=%d abort=%d", fake.txnBegins, fake.txnCommits, fake.txnAborts)
	}
	if len(fake.sublayers) != 1 || fake.sublayers[0].weight != wfpSublayerWeightMax {
		t.Fatalf("expected 1 max-weight sublayer, got %+v", fake.sublayers)
	}
	if fake.freeCalls != fake.allocatedBlobs {
		t.Fatalf("AppID blob leak: allocated=%d freed=%d", fake.allocatedBlobs, fake.freeCalls)
	}

	// Verify critical security invariants across installed filters.
	var (
		hasTunV4Permit   bool
		hasTunV6Permit   bool
		hasLoopbackV4    bool
		hasCoreV4        bool
		hasAppV4         bool
		dnsBlockCount    int
		catchAllBlockV4  bool
		catchAllBlockV6  bool
		hasDHCPv4        bool
		hasDHCPv6        bool
	)
	for _, flt := range fake.filters {
		if flt.weightType != wfpDataTypeUint8 || flt.weightVal > 15 {
			t.Fatalf("filter %q has invalid FWP_UINT8 weight type=%d val=%d (must be 0..15)", flt.name, flt.weightType, flt.weightVal)
		}
		if flt.action == wfpActionPermit && flt.flags != wfpFilterFlagClearActionRight {
			t.Fatalf("PERMIT filter %q must set FWPM_FILTER_FLAG_CLEAR_ACTION_RIGHT", flt.name)
		}
		if flt.action == wfpActionBlock && flt.flags != wfpFilterFlagNone {
			t.Fatalf("BLOCK filter %q must be hard block (flags=0), got 0x%x", flt.name, flt.flags)
		}
		switch flt.name {
		case "SanTian WFP Permit TUN Local V4":
			if flt.weightVal != wfpWeightAllowPriority || flt.v4Mask == nil || flt.v4Mask.addr != 0xac130001 || flt.v4Mask.mask != 0xffffffff {
				t.Fatalf("unexpected TUN V4 permit filter: %+v", flt)
			}
			hasTunV4Permit = true
		case "SanTian WFP Permit TUN Local V6":
			if flt.weightVal != wfpWeightAllowPriority || flt.v6Mask == nil || flt.v6Mask.prefixLength != 128 {
				t.Fatalf("unexpected TUN V6 permit filter: %+v", flt)
			}
			hasTunV6Permit = true
		case "SanTian WFP Permit Loopback V4":
			hasLoopbackV4 = flt.weightVal == wfpWeightAllowPriority
		case "SanTian WFP Permit Core V4":
			hasCoreV4 = flt.weightVal == wfpWeightAllowPriority
		case "SanTian WFP Permit App V4":
			hasAppV4 = flt.weightVal == wfpWeightAllowPriority
		case "SanTian WFP Permit DHCPv4":
			hasDHCPv4 = flt.weightVal == wfpWeightAllowLocalNet && len(flt.conds) == 3
		case "SanTian WFP Permit DHCPv6":
			hasDHCPv6 = flt.weightVal == wfpWeightAllowLocalNet && len(flt.conds) == 3
		case "SanTian WFP Block All Outbound V4":
			catchAllBlockV4 = flt.layer == wfpLayerALEAuthConnectV4 && flt.action == wfpActionBlock && len(flt.conds) == 0 && flt.weightVal == wfpWeightBlockCatchAll
		case "SanTian WFP Block All Outbound V6":
			catchAllBlockV6 = flt.layer == wfpLayerALEAuthConnectV6 && flt.action == wfpActionBlock && len(flt.conds) == 0 && flt.weightVal == wfpWeightBlockCatchAll
		}
		if strings.HasPrefix(flt.name, "SanTian WFP Block Non-TUN DNS ") && flt.action == wfpActionBlock && flt.weightVal == wfpWeightBlockDNSLeak {
			dnsBlockCount++
		}
	}
	if !hasTunV4Permit || !hasTunV6Permit || !hasLoopbackV4 || !hasCoreV4 || !hasAppV4 {
		t.Fatalf("missing priority permit rules: tunV4=%v tunV6=%v loopback=%v core=%v app=%v", hasTunV4Permit, hasTunV6Permit, hasLoopbackV4, hasCoreV4, hasAppV4)
	}
	if dnsBlockCount != 4 {
		t.Fatalf("expected 4 non-TUN DNS block rules (UDP/TCP x V4/V6), got %d", dnsBlockCount)
	}
	if !hasDHCPv4 || !hasDHCPv6 || !catchAllBlockV4 || !catchAllBlockV6 {
		t.Fatalf("missing DHCP/catch-all rules: dhcp4=%v dhcp6=%v block4=%v block6=%v", hasDHCPv4, hasDHCPv6, catchAllBlockV4, catchAllBlockV6)
	}

	// Idempotent re-enable with same paths should NOT open a second session.
	if err := enableWFPKillSwitch(nil, corePath, appPath); err != nil {
		t.Fatalf("idempotent enableWFPKillSwitch failed: %v", err)
	}
	if fake.openCalls != 1 {
		t.Fatalf("expected idempotent enable to reuse session (openCalls=1), got %d", fake.openCalls)
	}

	// Disable should atomically close the engine handle.
	if err := disableWFPKillSwitch(nil); err != nil {
		t.Fatalf("disableWFPKillSwitch failed: %v", err)
	}
	if isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch inactive after disable")
	}
	if fake.closeCalls != 1 {
		t.Fatalf("expected 1 EngineClose call, got %d", fake.closeCalls)
	}
	// Second disable is a no-op.
	if err := disableWFPKillSwitch(nil); err != nil || fake.closeCalls != 1 {
		t.Fatalf("expected second disable to be no-op, err=%v closeCalls=%d", err, fake.closeCalls)
	}
}

func TestWFPKillSwitchAbortsAndClosesOnFilterFailure(t *testing.T) {
	fake := &fakeWFPBackend{filterFailAt: 5}
	resetWFPTestState(t, fake, true)

	err := enableWFPKillSwitch(nil, `C:\SanTian\bin\sing-box.exe`, `C:\SanTian\SanTian-Singbox.exe`)
	if err == nil {
		t.Fatal("expected error when FilterAdd fails mid-transaction")
	}
	if isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch to remain inactive after failed transaction")
	}
	if fake.txnAborts != 1 || fake.txnCommits != 0 || fake.closeCalls != 1 {
		t.Fatalf("expected abort=1 commit=0 close=1 on rollback, got abort=%d commit=%d close=%d", fake.txnAborts, fake.txnCommits, fake.closeCalls)
	}
	if fake.freeCalls != fake.allocatedBlobs {
		t.Fatalf("expected all AppID blobs freed on rollback, allocated=%d freed=%d", fake.allocatedBlobs, fake.freeCalls)
	}
}

func TestWFPAppIntegrationAndNetshFallback(t *testing.T) {
	// 1. Predicate checks: Full TUN + CircuitBreaker only (NOT AppProxy-only TUN)
	if !shouldUseAlwaysOnWFPKillSwitch(Settings{Tun: true, CircuitBreaker: true}) {
		t.Fatal("expected full TUN + CircuitBreaker to use Always-On WFP")
	}
	if shouldUseAlwaysOnWFPKillSwitch(Settings{Tun: true, CircuitBreaker: false}) {
		t.Fatal("expected CircuitBreaker=false to disable Always-On WFP")
	}
	if shouldUseAlwaysOnWFPKillSwitch(Settings{
		Tun:      false,
		AppProxy: true,
		AppProxyTargets: []AppProxyTarget{
			{ID: "chrome", Type: "process", Name: "chrome.exe", Enabled: true},
		},
		CircuitBreaker: true,
	}) {
		t.Fatal("expected AppProxy-only TUN to NOT enable whole-machine WFP block")
	}

	tempRoot := t.TempDir()
	corePath := filepath.Join(tempRoot, "bin", "sing-box.exe")

	// 2. When WFP succeeds, enableTunKillSwitchIfConfigured MUST use WFP and skip netsh entirely.
	fakeOK := &fakeWFPBackend{}
	resetWFPTestState(t, fakeOK, true)
	netshCalls := 0
	runFirewallCommand = func(name string, args ...string) (string, error) {
		netshCalls++
		return "", nil
	}

	app := &App{
		root:    tempRoot,
		manager: &SingBoxManager{corePath: corePath},
	}
	app.enableTunKillSwitchIfConfigured(Settings{Tun: true, CircuitBreaker: true})
	if !isWFPKillSwitchActive() {
		t.Fatal("expected WFP killswitch active via enableTunKillSwitchIfConfigured")
	}
	if netshCalls != 0 {
		t.Fatalf("expected 0 netsh calls when WFP succeeds, got %d", netshCalls)
	}

	app.disableAllKillSwitches()
	if isWFPKillSwitchActive() {
		t.Fatal("expected disableAllKillSwitches to close WFP session")
	}

	// 3. When WFP engine open fails, enableTunKillSwitchIfConfigured MUST fall back to netsh Kill Switch.
	fakeFail := &fakeWFPBackend{openErr: errors.New("BFE service stopped")}
	resetWFPTestState(t, fakeFail, true)
	killSwitchMu.Lock()
	killSwitchActive = false
	killSwitchMu.Unlock()

	var recordedNetsh []string
	runFirewallCommand = func(name string, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		recordedNetsh = append(recordedNetsh, cmd)
		if strings.Contains(cmd, "show") && strings.Contains(cmd, "firewallpolicy") {
			return "Firewall Policy                       BlockInbound,AllowOutbound", nil
		}
		if strings.Contains(cmd, "show rule") {
			return "No rules match", errors.New("not found")
		}
		return "Ok.", nil
	}

	app.enableTunKillSwitchIfConfigured(Settings{Tun: true, CircuitBreaker: true})
	if isWFPKillSwitchActive() {
		t.Fatal("expected WFP inactive when EngineOpen fails")
	}
	if !isKillSwitchActive() {
		t.Fatal("expected netsh Kill Switch fallback to activate when WFP fails")
	}
	foundBlockPolicy := false
	for _, cmd := range recordedNetsh {
		if strings.Contains(cmd, "blockinbound,blockoutbound") {
			foundBlockPolicy = true
			break
		}
	}
	if !foundBlockPolicy {
		t.Fatalf("expected fallback netsh blockoutbound policy command, got %v", recordedNetsh)
	}

	// Clean up fallback state
	_ = disableKillSwitch(tempRoot, nil)
}
