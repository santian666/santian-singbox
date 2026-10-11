package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type portProcess struct {
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func processLabel(item portProcess) string {
	name := firstNonEmpty(item.Name, "未知进程")
	return fmt.Sprintf("PID=%d，进程=%s，路径=%s", item.PID, name, firstNonEmpty(item.Path, "未知"))
}

func findTCPPortProcesses(host string, port int) ([]portProcess, error) {
	items, err := findTCPPortsProcesses(host, []int{port})
	if err != nil {
		return nil, err
	}
	return items[port], nil
}

func findTCPPortsProcesses(host string, ports []int) (map[int][]portProcess, error) {
	if runtime.GOOS != "windows" {
		return map[int][]portProcess{}, nil
	}
	seen := map[int]bool{}
	portsToCheck := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || seen[port] {
			continue
		}
		seen[port] = true
		portsToCheck = append(portsToCheck, port)
	}
	if len(portsToCheck) == 0 {
		return map[int][]portProcess{}, nil
	}

	// 优先使用 Windows 原生 Win32 API 查询（耗时 < 1ms，无需外部进程，彻底杜绝 PowerShell 0xffffffff 退出错误）
	nativeResult, nativeErr := findTCPPortsProcessesNative(portsToCheck)
	if nativeErr == nil {
		return nativeResult, nil
	}

	// 兜底使用 netstat 命令行查询
	list, err := findTCPPortsProcessesByNetstat(host, seen)
	if err == nil {
		return groupPortProcesses(list), nil
	}

	return map[int][]portProcess{}, nil
}

// findTCPPortsProcessesNative 使用 iphlpapi.dll 的 GetExtendedTcpTable 原生接口批量检索端口和 PID
func findTCPPortsProcessesNative(ports []int) (map[int][]portProcess, error) {
	targetPorts := make(map[int]bool, len(ports))
	for _, p := range ports {
		if p > 0 {
			targetPorts[p] = true
		}
	}
	if len(targetPorts) == 0 {
		return map[int][]portProcess{}, nil
	}

	result := make(map[int][]portProcess)
	// 2 = AF_INET (IPv4), 23 = AF_INET6 (IPv6)
	for _, family := range []uint32{2, 23} {
		table, err := getExtendedTcpTableNative(family)
		if err != nil {
			continue
		}
		for port, pids := range table {
			if !targetPorts[port] {
				continue
			}
			for _, pid := range pids {
				if pid <= 0 {
					continue
				}
				name, path := getProcessInfoByPIDNative(pid)
				result[port] = append(result[port], portProcess{
					Port: port,
					PID:  pid,
					Name: name,
					Path: path,
				})
			}
		}
	}
	return result, nil
}

func getExtendedTcpTableNative(family uint32) (map[int][]int, error) {
	iphlpapi := syscall.NewLazyDLL("iphlpapi.dll")
	procGetExtendedTcpTable := iphlpapi.NewProc("GetExtendedTcpTable")

	var size uint32 = 0
	ret, _, _ := procGetExtendedTcpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		uintptr(family),
		uintptr(3), // TCP_TABLE_OWNER_PID_LISTENER
		0,
	)
	if size == 0 {
		return nil, syscall.Errno(ret)
	}

	buf := make([]byte, size)
	ret, _, _ = procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		uintptr(family),
		uintptr(3),
		0,
	)
	if ret != 0 {
		return nil, syscall.Errno(ret)
	}

	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	result := make(map[int][]int)

	if family == 2 { // AF_INET
		type rowType struct {
			State      uint32
			LocalAddr  uint32
			LocalPort  uint32
			RemoteAddr uint32
			RemotePort uint32
			OwningPid  uint32
		}
		rowSize := unsafe.Sizeof(rowType{})
		for i := uint32(0); i < numEntries; i++ {
			offset := 4 + uintptr(i)*rowSize
			if offset+rowSize > uintptr(len(buf)) {
				break
			}
			row := (*rowType)(unsafe.Pointer(&buf[offset]))
			port := int(binary.BigEndian.Uint16([]byte{byte(row.LocalPort), byte(row.LocalPort >> 8)}))
			result[port] = append(result[port], int(row.OwningPid))
		}
	} else if family == 23 { // AF_INET6
		type row6Type struct {
			LocalAddr     [16]byte
			LocalScopeId  uint32
			LocalPort     uint32
			RemoteAddr    [16]byte
			RemoteScopeId uint32
			RemotePort    uint32
			State         uint32
			OwningPid     uint32
		}
		rowSize := unsafe.Sizeof(row6Type{})
		for i := uint32(0); i < numEntries; i++ {
			offset := 4 + uintptr(i)*rowSize
			if offset+rowSize > uintptr(len(buf)) {
				break
			}
			row := (*row6Type)(unsafe.Pointer(&buf[offset]))
			port := int(binary.BigEndian.Uint16([]byte{byte(row.LocalPort), byte(row.LocalPort >> 8)}))
			result[port] = append(result[port], int(row.OwningPid))
		}
	}
	return result, nil
}

func getProcessInfoByPIDNative(pid int) (name string, exePath string) {
	if pid <= 0 {
		return "", ""
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess := kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW := kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle := kernel32.NewProc("CloseHandle")

	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

	hProcess, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_LIMITED_INFORMATION),
		0,
		uintptr(pid),
	)
	if hProcess == 0 {
		return "", ""
	}
	defer procCloseHandle.Call(hProcess)

	var buf [1024]uint16
	var size uint32 = uint32(len(buf))

	ret, _, _ := procQueryFullProcessImageNameW.Call(
		hProcess,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return "", ""
	}

	exePath = syscall.UTF16ToString(buf[:size])
	name = filepath.Base(exePath)
	return name, exePath
}

func findTCPPortsProcessesByNetstat(host string, ports map[int]bool) ([]portProcess, error) {
	netstatPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "netstat.exe")
	if _, err := os.Stat(netstatPath); err != nil {
		netstatPath = "netstat"
	}
	out, err := runCommandOutput(netstatPath, "-ano", "-p", "tcp")
	if err != nil {
		return nil, err
	}
	pidSet := map[int]bool{}
	items := make([]portProcess, 0)
	linePattern := regexp.MustCompile(`(?i)^\s*TCP\s+(.+):(\d+)\s+\S+\s+LISTENING\s+(\d+)\s*$`)
	for _, line := range strings.Split(out, "\n") {
		match := linePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 4 {
			continue
		}
		port, err := strconv.Atoi(match[2])
		if err != nil || !ports[port] {
			continue
		}
		if !isLocalListenAddress(match[1], host) {
			continue
		}
		pid, err := strconv.Atoi(match[3])
		if err != nil || pid <= 0 {
			continue
		}
		pidSet[pid] = true
		items = append(items, portProcess{Port: port, PID: pid})
	}
	if len(items) == 0 {
		return nil, nil
	}

	// 优先通过原生 Win32 查询进程名和路径
	for index := range items {
		name, path := getProcessInfoByPIDNative(items[index].PID)
		items[index].Name = name
		items[index].Path = path
	}

	// 针对未通过原生 API 获取到的 PID，使用 tasklist 补充进程名
	fillProcessNamesByTasklist(items)

	return items, nil
}

func isLocalListenAddress(address string, host string) bool {
	address = strings.Trim(address, "[] ")
	host = strings.Trim(host, "[] ")
	return address == "127.0.0.1" || address == "0.0.0.0" || address == "::" || address == "::1" || strings.EqualFold(address, host)
}

func fillProcessNamesByTasklist(items []portProcess) {
	tasklistPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "tasklist.exe")
	if _, err := os.Stat(tasklistPath); err != nil {
		tasklistPath = "tasklist"
	}
	for index := range items {
		if strings.TrimSpace(items[index].Name) != "" {
			continue
		}
		out, err := runCommandOutput(tasklistPath, "/FI", "PID eq "+strconv.Itoa(items[index].PID), "/FO", "CSV", "/NH")
		if err != nil {
			continue
		}
		reader := csv.NewReader(strings.NewReader(out))
		record, err := reader.Read()
		if err == nil && len(record) >= 2 {
			items[index].Name = strings.TrimSpace(record[0])
		}
	}
}

func groupPortProcesses(list []portProcess) map[int][]portProcess {
	result := make(map[int][]portProcess)
	for _, item := range list {
		if item.Port <= 0 || item.PID <= 0 {
			continue
		}
		result[item.Port] = append(result[item.Port], item)
	}
	return result
}

func forceKillProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("PID 无效：%d", pid)
	}
	if runtime.GOOS == "windows" {
		if !isWindowsProcessRunning(pid) {
			return nil
		}
		// 优先使用 Win32 原生 TerminateProcess 强杀
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		procOpenProcess := kernel32.NewProc("OpenProcess")
		procTerminateProcess := kernel32.NewProc("TerminateProcess")
		procCloseHandle := kernel32.NewProc("CloseHandle")

		const PROCESS_TERMINATE = 0x0001
		hProcess, _, _ := procOpenProcess.Call(uintptr(PROCESS_TERMINATE), 0, uintptr(pid))
		if hProcess != 0 {
			procTerminateProcess.Call(hProcess, 1)
			procCloseHandle.Call(hProcess)
		}
		if !isWindowsProcessRunning(pid) {
			return nil
		}

		// 兜底使用 taskkill 强杀
		taskkillPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe")
		if _, err := os.Stat(taskkillPath); err != nil {
			taskkillPath = "taskkill"
		}
		out, err := runCommandOutput(taskkillPath, "/PID", strconv.Itoa(pid), "/T", "/F")
		if err == nil || !isWindowsProcessRunning(pid) {
			return nil
		}
		return fmt.Errorf("结束进程失败：%s，%w", strings.TrimSpace(out), err)
	}
	return fmt.Errorf("当前系统不支持强杀端口占用进程")
}

func isWindowsProcessRunning(pid int) bool {
	if runtime.GOOS != "windows" || pid <= 0 {
		return false
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess := kernel32.NewProc("OpenProcess")
	procGetExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
	procCloseHandle := kernel32.NewProc("CloseHandle")

	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	const STILL_ACTIVE = 259

	hProcess, _, _ := procOpenProcess.Call(uintptr(PROCESS_QUERY_LIMITED_INFORMATION), 0, uintptr(pid))
	if hProcess != 0 {
		defer procCloseHandle.Call(hProcess)
		var exitCode uint32
		ret, _, _ := procGetExitCodeProcess.Call(hProcess, uintptr(unsafe.Pointer(&exitCode)))
		if ret != 0 {
			return exitCode == STILL_ACTIVE
		}
	}

	tasklistPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "tasklist.exe")
	if _, err := os.Stat(tasklistPath); err != nil {
		tasklistPath = "tasklist"
	}
	out, err := runCommandOutput(tasklistPath, "/FI", "PID eq "+strconv.Itoa(pid), "/NH")
	if err != nil {
		return true
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return false
	}
	if strings.Contains(strings.ToLower(text), "no tasks") || strings.Contains(text, "没有运行") || strings.Contains(text, "信息:") {
		return false
	}
	return strings.Contains(text, strconv.Itoa(pid))
}

func waitTCPPortFreeStrict(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isTCPPortFree(host, port) {
			return true
		}
		time.Sleep(120 * time.Millisecond)
	}
	return isTCPPortFree(host, port)
}

func sameExecutablePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	ap, errA := filepath.Abs(a)
	bp, errB := filepath.Abs(b)
	if errA == nil {
		a = ap
	}
	if errB == nil {
		b = bp
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
