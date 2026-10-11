package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	maxExternalProxyEndpoints = 128
	externalProxyProbeTimeout = 6 * time.Second
	externalProxyProbeArg     = "--external-proxy-probe"
)

type externalProxyProbeRequest struct {
	ProxyAddress string                `json:"proxyAddress"`
	Path         string                `json:"path"`
	Endpoint     ExternalProxyEndpoint `json:"endpoint"`
}

type externalProxyProbeResponse struct {
	Result ExternalProxyTestResult `json:"result"`
	Error  string                  `json:"error,omitempty"`
}

func normalizeExternalProxyEndpoints(endpoints []ExternalProxyEndpoint) []ExternalProxyEndpoint {
	result := make([]ExternalProxyEndpoint, 0, len(endpoints))
	seen := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		host, err := normalizeExternalProxyHost(endpoint.Host)
		if err != nil || endpoint.Port < 1 || endpoint.Port > 65535 {
			continue
		}
		key := strings.ToLower(host) + ":" + strconv.Itoa(endpoint.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		endpoint.Host = host
		endpoint.ResolvedIPs = normalizeExternalProxyIPs(endpoint.ResolvedIPs)
		if ip := net.ParseIP(host); ip != nil {
			endpoint.ResolvedIPs = []string{ip.String()}
		}
		result = append(result, endpoint)
		if len(result) == maxExternalProxyEndpoints {
			break
		}
	}
	return result
}

func validateExternalProxyEndpoints(endpoints []ExternalProxyEndpoint) error {
	if len(endpoints) > maxExternalProxyEndpoints {
		return fmt.Errorf("外部代理入口最多允许 %d 个", maxExternalProxyEndpoints)
	}
	for index, endpoint := range endpoints {
		if _, err := normalizeExternalProxyHost(endpoint.Host); err != nil {
			return fmt.Errorf("第 %d 个外部代理入口无效：%w", index+1, err)
		}
		if endpoint.Port < 1 || endpoint.Port > 65535 {
			return fmt.Errorf("第 %d 个外部代理入口端口 %d 无效", index+1, endpoint.Port)
		}
	}
	return nil
}

func normalizeExternalProxyHost(value string) (string, error) {
	host := strings.TrimSpace(strings.ToLower(value))
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.ContainsAny(host, " /\\?#@") {
		return "", errors.New("请输入正确的域名或 IP")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	if len(host) > 253 || strings.Contains(host, "..") {
		return "", errors.New("域名格式无效")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("域名格式无效")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return "", errors.New("域名格式无效")
			}
		}
	}
	return host, nil
}

func normalizeExternalProxyIPs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		ip := net.ParseIP(strings.TrimSpace(value))
		if ip == nil {
			continue
		}
		value = ip.String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func resolveExternalProxyEndpoints(endpoints []ExternalProxyEndpoint) []ExternalProxyEndpoint {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for index := range endpoints {
		endpoint := &endpoints[index]
		if !endpoint.Enabled || net.ParseIP(endpoint.Host) != nil {
			continue
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, endpoint.Host)
		if err != nil {
			continue
		}
		resolved := make([]string, 0, len(addresses))
		for _, address := range addresses {
			resolved = append(resolved, address.IP.String())
		}
		if resolved = normalizeExternalProxyIPs(resolved); len(resolved) > 0 {
			endpoint.ResolvedIPs = resolved
		}
	}
	return endpoints
}

func buildExternalProxyDirectRules(settings Settings) []map[string]interface{} {
	rules := make([]map[string]interface{}, 0)
	for _, endpoint := range normalizeExternalProxyEndpoints(settings.ExternalProxyEndpoints) {
		if !endpoint.Enabled {
			continue
		}
		port := []int{endpoint.Port}
		if ip := net.ParseIP(endpoint.Host); ip != nil {
			rules = append(rules, externalProxyIPRule([]string{externalProxyCIDR(ip)}, port))
			continue
		}
		rules = append(rules, map[string]interface{}{
			"domain":   []string{endpoint.Host},
			"port":     port,
			"network":  "tcp",
			"action":   "route",
			"outbound": "direct",
		})
		cidrs := make([]string, 0, len(endpoint.ResolvedIPs))
		for _, value := range endpoint.ResolvedIPs {
			if ip := net.ParseIP(value); ip != nil {
				cidrs = append(cidrs, externalProxyCIDR(ip))
			}
		}
		if len(cidrs) > 0 {
			rules = append(rules, externalProxyIPRule(cidrs, port))
		}
	}
	return rules
}

func externalProxyIPRule(cidrs []string, port []int) map[string]interface{} {
	return map[string]interface{}{
		"ip_cidr":  cidrs,
		"port":     port,
		"network":  "tcp",
		"action":   "route",
		"outbound": "direct",
	}
}

func externalProxyCIDR(ip net.IP) string {
	if ip.To4() != nil {
		return ip.String() + "/32"
	}
	return ip.String() + "/128"
}

func isExternalProxyDirectRule(rule map[string]interface{}) bool {
	if rule["outbound"] != "direct" || rule["action"] != "route" || !configStringSetEqual(rule["network"], []string{"tcp"}) {
		return false
	}
	if rule["clash_mode"] != nil || rule["inbound"] != nil || rule["process_name"] != nil || rule["process_path"] != nil || rule["process_path_regex"] != nil {
		return false
	}
	return rule["domain"] != nil || rule["ip_cidr"] != nil
}

func hasExternalProxyRoutes(config map[string]interface{}, endpoints []ExternalProxyEndpoint) bool {
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return false
	}
	rules := configRules(route["rules"])
	globalRuleIndex := -1
	for index, rule := range rules {
		if rule["clash_mode"] == "global" && rule["action"] == "route" && rule["outbound"] == "proxy" {
			globalRuleIndex = index
			break
		}
	}
	for _, endpoint := range normalizeExternalProxyEndpoints(endpoints) {
		if !endpoint.Enabled {
			continue
		}
		if globalRuleIndex < 0 {
			return false
		}
		expected := buildExternalProxyDirectRules(Settings{ExternalProxyEndpoints: []ExternalProxyEndpoint{endpoint}})
		for _, expectedRule := range expected {
			found := false
			for _, rule := range rules[:globalRuleIndex] {
				if externalProxyRuleMatches(rule, expectedRule) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func externalProxyRuleMatches(rule map[string]interface{}, expected map[string]interface{}) bool {
	if !isExternalProxyDirectRule(rule) || !configPortExact(rule["port"], expected["port"]) {
		return false
	}
	return configStringSetEqual(rule["domain"], configStringValues(expected["domain"])) &&
		configStringSetEqual(rule["ip_cidr"], configStringValues(expected["ip_cidr"]))
}

func isSavedExternalProxyDirectRule(rule map[string]interface{}, endpoints []ExternalProxyEndpoint) bool {
	for _, expected := range buildExternalProxyDirectRules(Settings{ExternalProxyEndpoints: endpoints}) {
		if externalProxyRuleMatches(rule, expected) {
			return true
		}
	}
	return false
}

func configPortExact(actual interface{}, expected interface{}) bool {
	actualPorts := configIntValues(actual)
	expectedPorts := configIntValues(expected)
	if len(actualPorts) != len(expectedPorts) {
		return false
	}
	for index, port := range actualPorts {
		if port != expectedPorts[index] {
			return false
		}
	}
	return true
}

func configIntValues(value interface{}) []int {
	switch values := value.(type) {
	case int:
		return []int{values}
	case float64:
		return []int{int(values)}
	case []int:
		return append([]int(nil), values...)
	case []interface{}:
		result := make([]int, 0, len(values))
		for _, item := range values {
			result = append(result, intFromAny(item))
		}
		return result
	default:
		return nil
	}
}

func configStringSetEqual(actual interface{}, expected []string) bool {
	actualValues := configStringValues(actual)
	if len(actualValues) != len(expected) {
		return false
	}
	found := make(map[string]int, len(actualValues))
	for _, value := range actualValues {
		found[value]++
	}
	for _, value := range expected {
		if found[value] == 0 {
			return false
		}
		found[value]--
	}
	return true
}

func configPortContains(value interface{}, want int) bool {
	switch ports := value.(type) {
	case int:
		return ports == want
	case float64:
		return int(ports) == want
	case []int:
		for _, port := range ports {
			if port == want {
				return true
			}
		}
	case []interface{}:
		for _, port := range ports {
			if intFromAny(port) == want {
				return true
			}
		}
	}
	return false
}

func (a *App) SaveExternalProxyEndpoints(endpoints []ExternalProxyEndpoint) (result []ExternalProxyEndpoint, err error) {
	defer a.recoverError("保存外部代理入口", &err)
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	if err := validateExternalProxyEndpoints(endpoints); err != nil {
		return nil, err
	}
	endpoints = resolveExternalProxyEndpoints(normalizeExternalProxyEndpoints(endpoints))
	state := a.store.Snapshot()
	state.Settings.ExternalProxyEndpoints = endpoints
	if err := a.SaveSettings(state.Settings); err != nil {
		return nil, err
	}
	enabled := 0
	for _, endpoint := range endpoints {
		if endpoint.Enabled {
			enabled++
		}
	}
	if a.manager != nil && a.manager.Running() {
		if err := a.manager.ValidateRuntimeSettings(a.store.Snapshot().Settings); err != nil {
			return nil, fmt.Errorf("外部代理入口已保存，但运行配置校验失败：%w", err)
		}
		if a.logger != nil {
			a.logger.Info("外部代理入口已保存并验证生效：启用 %d 个", enabled)
		}
	} else if a.logger != nil {
		a.logger.Info("外部代理入口已保存：启用 %d 个，将在下次启动代理时生效", enabled)
	}
	return endpoints, nil
}

func (a *App) TestExternalProxyEndpoint(endpoint ExternalProxyEndpoint) (result ExternalProxyTestResult, err error) {
	defer a.recoverError("检测外部代理入口", &err)
	if err := validateExternalProxyEndpoints([]ExternalProxyEndpoint{endpoint}); err != nil {
		return ExternalProxyTestResult{}, err
	}
	endpoint = normalizeExternalProxyEndpoints([]ExternalProxyEndpoint{endpoint})[0]
	request := externalProxyProbeRequest{Endpoint: endpoint, Path: "本机直连"}
	if a.manager != nil && a.manager.Running() && a.store != nil {
		state := a.store.Snapshot()
		request.ProxyAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Settings.MixedPort))
		request.Path = "SanTian 路由"
	}
	result, err = runExternalProxyProbeSubprocess(request)
	if err != nil {
		return ExternalProxyTestResult{}, err
	}
	if a.logger != nil {
		a.logger.Info("外部代理入口检测成功：路径=%s，认证=%s，耗时=%dms", result.Path, result.Auth, result.LatencyMS)
	}
	return result, nil
}

func runExternalProxyProbeSubprocess(request externalProxyProbeRequest) (ExternalProxyTestResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return ExternalProxyTestResult{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return ExternalProxyTestResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), externalProxyProbeTimeout+3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, externalProxyProbeArg)
	hideCommandWindow(cmd)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ExternalProxyTestResult{}, errors.New("外部代理检测子进程超时")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return ExternalProxyTestResult{}, fmt.Errorf("外部代理检测子进程异常退出：%s", message)
	}
	var response externalProxyProbeResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return ExternalProxyTestResult{}, fmt.Errorf("外部代理检测子进程返回无效：%w", err)
	}
	if response.Error != "" {
		return ExternalProxyTestResult{}, errors.New(response.Error)
	}
	return response.Result, nil
}

func runExternalProxyProbeFromArgs() (bool, int) {
	if !hasArg(externalProxyProbeArg) {
		return false, 0
	}
	payload, readErr := io.ReadAll(io.LimitReader(os.Stdin, 64*1024))
	if readErr != nil {
		_ = json.NewEncoder(os.Stdout).Encode(externalProxyProbeResponse{Error: "检测参数无效：" + readErr.Error()})
		return true, 2
	}
	request, err := decodeExternalProxyProbeRequest(payload)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(externalProxyProbeResponse{Error: "检测参数无效：" + err.Error()})
		return true, 2
	}
	result, err := executeExternalProxyProbe(request)
	response := externalProxyProbeResponse{Result: result}
	if err != nil {
		response.Error = err.Error()
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		return true, 1
	}
	return true, 0
}

func decodeExternalProxyProbeRequest(payload []byte) (externalProxyProbeRequest, error) {
	var request externalProxyProbeRequest
	payload = bytes.TrimPrefix(payload, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(payload, &request); err != nil {
		return externalProxyProbeRequest{}, err
	}
	return request, nil
}

func executeExternalProxyProbe(request externalProxyProbeRequest) (ExternalProxyTestResult, error) {
	endpoint := request.Endpoint
	if err := validateExternalProxyEndpoints([]ExternalProxyEndpoint{endpoint}); err != nil {
		return ExternalProxyTestResult{}, err
	}
	endpoint = normalizeExternalProxyEndpoints([]ExternalProxyEndpoint{endpoint})[0]
	path := strings.TrimSpace(request.Path)
	if path == "" {
		path = "本机直连"
	}
	started := time.Now()
	var conn net.Conn
	var err error
	if strings.TrimSpace(request.ProxyAddress) != "" {
		conn, err = dialSOCKS5Target(request.ProxyAddress, endpoint, externalProxyProbeTimeout)
	} else {
		var dialer net.Dialer
		ctx, cancel := context.WithTimeout(context.Background(), externalProxyProbeTimeout)
		defer cancel()
		conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	}
	if err != nil {
		return ExternalProxyTestResult{}, fmt.Errorf("%s无法连接远程 SOCKS5：%w", path, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(externalProxyProbeTimeout)); err != nil {
		return ExternalProxyTestResult{}, err
	}
	auth, err := probeRemoteSOCKS5(conn)
	if err != nil {
		return ExternalProxyTestResult{}, fmt.Errorf("%s已连接端口，但远程 SOCKS5 握手失败：%w", path, err)
	}
	message := "SOCKS5 可用"
	if auth == "用户名密码" {
		message += "，需要用户名密码认证"
	} else {
		message += "，无需认证"
	}
	return ExternalProxyTestResult{
		Host:      endpoint.Host,
		Port:      endpoint.Port,
		Path:      path,
		Auth:      auth,
		LatencyMS: int(time.Since(started).Milliseconds()),
		Message:   message,
	}, nil
}

func dialSOCKS5Target(proxyAddress string, endpoint ExternalProxyEndpoint, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddress, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 SanTian 本地入口失败：%w", err)
	}
	fail := func(err error) (net.Conn, error) {
		conn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fail(err)
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fail(fmt.Errorf("SanTian 本地 SOCKS5 握手失败：%w", err))
	}
	if response[0] != 5 || response[1] != 0 {
		return fail(fmt.Errorf("SanTian 本地 SOCKS5 拒绝连接：%v", response))
	}
	request, err := socks5ConnectRequest(endpoint.Host, endpoint.Port)
	if err != nil {
		return fail(err)
	}
	if _, err := conn.Write(request); err != nil {
		return fail(err)
	}
	if err := readSOCKS5ConnectReply(conn); err != nil {
		return fail(err)
	}
	return conn, nil
}

func socks5ConnectRequest(host string, port int) ([]byte, error) {
	request := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			request = append(request, 1)
			request = append(request, ipv4...)
		} else {
			request = append(request, 4)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("远程 SOCKS5 域名过长")
		}
		request = append(request, 3, byte(len(host)))
		request = append(request, host...)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	return append(request, portBytes...), nil
}

func readSOCKS5ConnectReply(conn net.Conn) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("远程 SOCKS5 连接请求失败：%w", err)
	}
	if header[0] != 5 || header[1] != 0 {
		return fmt.Errorf("远程 SOCKS5 连接被拒绝，错误码 %d", header[1])
	}
	length := 0
	switch header[3] {
	case 1:
		length = 4
	case 4:
		length = 16
	case 3:
		value := []byte{0}
		if _, err := io.ReadFull(conn, value); err != nil {
			return err
		}
		length = int(value[0])
	default:
		return errors.New("远程 SOCKS5 返回了未知地址类型")
	}
	_, err := io.CopyN(io.Discard, conn, int64(length+2))
	return err
}

func probeRemoteSOCKS5(conn net.Conn) (string, error) {
	if _, err := conn.Write([]byte{5, 2, 0, 2}); err != nil {
		return "", err
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return "", err
	}
	if response[0] != 5 {
		return "", fmt.Errorf("响应不是 SOCKS5：%v", response)
	}
	switch response[1] {
	case 0:
		return "无认证", nil
	case 2:
		return "用户名密码", nil
	case 255:
		return "", errors.New("远程 SOCKS5 不接受无认证或用户名密码认证")
	default:
		return "", fmt.Errorf("远程 SOCKS5 要求不支持的认证方式 %d", response[1])
	}
}
