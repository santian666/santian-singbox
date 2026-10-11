package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type SubscriptionResult struct {
	GroupName string
	Groups    []NodeGroup
	Nodes     []Node
	Errors    []string
	Mode      string
}

const (
	maxSubscriptionBytes       = 8 * 1024 * 1024
	subscriptionProxyAttempts  = 2
	subscriptionProxyRetryWait = 300 * time.Millisecond
)

func FetchSubscription(ctx context.Context, subURL string, proxyPort int) (SubscriptionResult, error) {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return SubscriptionResult{}, errors.New("订阅链接不能为空")
	}
	if _, err := url.ParseRequestURI(subURL); err != nil {
		return SubscriptionResult{}, errors.New("订阅链接无效")
	}
	body, headers, mode, err := fetchSubscriptionWithFallback(ctx, subURL, proxyPort)
	if err != nil {
		return SubscriptionResult{}, err
	}
	result := ParseSubscriptionContent(body, subURL, headers)
	result.Mode = mode
	return result, nil
}

func fetchSubscriptionWithFallback(ctx context.Context, subURL string, proxyPort int) (string, http.Header, string, error) {
	if proxyPort > 0 && localProxyReady(proxyPort) {
		var proxyErr error
		for attempt := 1; attempt <= subscriptionProxyAttempts; attempt++ {
			body, headers, mode, err := fetchSubscriptionBody(ctx, subURL, proxyPort)
			if err == nil {
				return body, headers, mode, nil
			}
			proxyErr = err
			if attempt == subscriptionProxyAttempts || !isRetryableNetworkError(err) {
				break
			}
			select {
			case <-ctx.Done():
				return "", nil, mode, ctx.Err()
			case <-time.After(subscriptionProxyRetryWait):
			}
		}
		body, headers, mode, err := fetchSubscriptionBody(ctx, subURL, 0)
		if err == nil {
			return body, headers, mode, nil
		}
		return "", nil, mode, fmt.Errorf("代理抓取失败：%v；直连失败：%w", proxyErr, err)
	}
	return fetchSubscriptionBody(ctx, subURL, 0)
}

func localProxyReady(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func ParseSubscriptionContent(content, sourceURL string, headers http.Header) SubscriptionResult {
	groupName := subscriptionGroupName(sourceURL, headers)
	nodes, groups, errs := parseSubscriptionPayload(content, sourceURL)
	if len(nodes) == 0 {
		if decoded, err := decodeBase64Text(content); err == nil && decoded != content {
			nodes, groups, errs = parseSubscriptionPayload(decoded, sourceURL)
		}
	}
	return SubscriptionResult{
		GroupName: groupName,
		Groups:    groups,
		Nodes:     dedupeNodes(nodes),
		Errors:    compactErrors(errs),
	}
}

func parseSubscriptionPayload(content, sourceURL string) ([]Node, []NodeGroup, []string) {
	var groups []NodeGroup
	var nodes []Node
	var errs []string

	parsed, parsedGroups, parseErrs := parseClashSubscription(content, sourceURL)
	if len(parsed) > 0 {
		nodes = append(nodes, parsed...)
		groups = append(groups, parsedGroups...)
	}
	errs = append(errs, parseErrs...)
	if len(nodes) == 0 {
		parsed, parseErrs := parseJSONSubscription(content)
		nodes = append(nodes, parsed...)
		errs = append(errs, parseErrs...)
	}
	if len(nodes) == 0 {
		parsed, parseErrs := ParseNodes(content)
		nodes = append(nodes, parsed...)
		errs = append(errs, parseErrs...)
	}
	return dedupeNodes(nodes), dedupeGroups(groups), compactErrors(errs)
}

func fetchSubscriptionBody(ctx context.Context, subURL string, proxyPort int) (string, http.Header, string, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 18 * time.Second, Transport: transport}
	mode := "direct"
	if proxyPort > 0 {
		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		transport.Proxy = http.ProxyURL(proxyURL)
		mode = "proxy"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return "", nil, mode, err
	}
	req.Header.Set("User-Agent", "ClashforWindows/0.20 GoSingBox")
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, mode, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.Header, mode, fmt.Errorf("订阅返回状态：%d", resp.StatusCode)
	}
	if resp.ContentLength > maxSubscriptionBytes {
		return "", resp.Header, mode, errors.New("订阅内容超过 8 MB 限制")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBytes+1))
	if err != nil {
		return "", resp.Header, mode, err
	}
	if len(data) > maxSubscriptionBytes {
		return "", resp.Header, mode, errors.New("订阅内容超过 8 MB 限制")
	}
	return string(data), resp.Header, mode, nil
}

func parseJSONSubscription(content string) ([]Node, []string) {
	var data interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, nil
	}
	var items []interface{}
	switch value := data.(type) {
	case []interface{}:
		items = value
	case map[string]interface{}:
		if raw, ok := value["outbounds"].([]interface{}); ok {
			items = raw
		} else if raw, ok := value["proxies"].([]interface{}); ok {
			items = raw
		}
	}
	nodes := make([]Node, 0, len(items))
	errs := make([]string, 0)
	for _, item := range items {
		outbound, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typ := strings.ToLower(stringFromAny(outbound["type"]))
		if typ == "selector" || typ == "urltest" || typ == "direct" || typ == "block" || typ == "dns" {
			continue
		}
		raw, err := json.Marshal(outbound)
		if err != nil {
			errs = append(errs, fmt.Sprintf("JSON 节点：%v", err))
			continue
		}
		node, err := parseJSONOutbound(string(raw))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s：%v", firstNonEmpty(stringFromAny(outbound["tag"]), "JSON 节点"), err))
			continue
		}
		nodes = append(nodes, node)
	}
	return dedupeNodes(nodes), errs
}

func parseClashSubscription(content, sourceURL string) ([]Node, []NodeGroup, []string) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		return nil, nil, nil
	}
	rawProxies, ok := data["proxies"].([]interface{})
	if !ok || len(rawProxies) == 0 {
		return nil, nil, nil
	}
	groupNames := clashGroupNames(data["proxy-groups"])
	groups := make([]NodeGroup, 0)
	groupExists := map[string]bool{}
	nodes := make([]Node, 0, len(rawProxies))
	errs := make([]string, 0)
	for _, item := range rawProxies {
		proxy, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		node, err := parseClashProxy(proxy)
		if err != nil {
			name := stringFromAny(proxy["name"])
			errs = append(errs, fmt.Sprintf("%s：%v", firstNonEmpty(name, "Clash 节点"), err))
			continue
		}
		if groupName := groupNames[node.Name]; groupName != "" {
			groupID := groupIDFromName(sourceURL + "|" + groupName)
			node.GroupID = groupID
			if !groupExists[groupID] {
				groups = append(groups, NodeGroup{ID: groupID, Name: groupName, Source: "subscription", URL: sourceURL})
				groupExists[groupID] = true
			}
		}
		nodes = append(nodes, node)
	}
	return dedupeNodes(nodes), groups, errs
}

func parseClashProxy(proxy map[string]interface{}) (Node, error) {
	typ := strings.ToLower(strings.ReplaceAll(stringFromAny(proxy["type"]), "-", ""))
	switch typ {
	case "ss", "shadowsocks":
		raw := buildClashSS(proxy)
		return parseShadowsocks(raw)
	case "vmess":
		raw, err := buildClashVMess(proxy)
		if err != nil {
			return Node{}, err
		}
		return parseVMess(raw)
	case "vless":
		raw := buildClashStandardURL("vless", proxy, firstNonEmpty(stringFromAny(proxy["uuid"]), stringFromAny(proxy["id"])))
		return parseVLESS(raw)
	case "trojan":
		raw := buildClashStandardURL("trojan", proxy, stringFromAny(proxy["password"]))
		return parseTrojan(raw)
	case "hysteria2", "hy2":
		raw := buildClashStandardURL("hysteria2", proxy, stringFromAny(proxy["password"]))
		return parseHysteria2(raw)
	case "tuic":
		raw := buildClashTUIC(proxy)
		return parseTUIC(raw)
	case "anytls":
		raw := buildClashStandardURL("anytls", proxy, firstNonEmpty(stringFromAny(proxy["password"]), stringFromAny(proxy["uuid"]), stringFromAny(proxy["id"])))
		return parseAnyTLS(raw)
	case "socks5", "socks":
		raw := buildClashSOCKS(proxy)
		return parseSOCKS(raw)
	case "http":
		return parseClashHTTP(proxy)
	default:
		return Node{}, fmt.Errorf("暂不支持 Clash 协议：%s", firstNonEmpty(stringFromAny(proxy["type"]), "未知"))
	}
}

func buildClashSS(proxy map[string]interface{}) string {
	method := stringFromAny(proxy["cipher"])
	password := stringFromAny(proxy["password"])
	main := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
	return fmt.Sprintf("ss://%s@%s:%d#%s", main, stringFromAny(proxy["server"]), intFromAny(proxy["port"]), url.QueryEscape(stringFromAny(proxy["name"])))
}

func buildClashVMess(proxy map[string]interface{}) (string, error) {
	data := map[string]interface{}{
		"v":    "2",
		"ps":   stringFromAny(proxy["name"]),
		"add":  stringFromAny(proxy["server"]),
		"port": strconv.Itoa(intFromAny(proxy["port"])),
		"id":   firstNonEmpty(stringFromAny(proxy["uuid"]), stringFromAny(proxy["id"])),
		"aid":  strconv.Itoa(firstNonZero(intFromAny(proxy["alterId"]), intFromAny(proxy["alter-id"]))),
		"scy":  firstNonEmpty(stringFromAny(proxy["cipher"]), "auto"),
		"net":  firstNonEmpty(stringFromAny(proxy["network"]), stringFromAny(proxy["net"])),
		"tls":  clashTLSValue(proxy),
		"sni":  firstNonEmpty(stringFromAny(proxy["servername"]), stringFromAny(proxy["sni"])),
	}
	if reality, ok := proxy["reality-opts"].(map[string]interface{}); ok {
		if publicKey := firstNonEmpty(stringFromAny(reality["public-key"]), stringFromAny(reality["publicKey"])); publicKey != "" {
			data["pbk"] = publicKey
		}
		if shortID := firstNonEmpty(stringFromAny(reality["short-id"]), stringFromAny(reality["shortId"])); shortID != "" {
			data["sid"] = shortID
		}
	}
	if ws, ok := proxy["ws-opts"].(map[string]interface{}); ok {
		data["path"] = stringFromAny(ws["path"])
		if headers, ok := ws["headers"].(map[string]interface{}); ok {
			data["host"] = stringFromAny(headers["Host"])
		}
	}
	if grpc, ok := proxy["grpc-opts"].(map[string]interface{}); ok {
		data["path"] = firstNonEmpty(stringFromAny(grpc["grpc-service-name"]), stringFromAny(grpc["serviceName"]), stringFromAny(grpc["service-name"]))
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
}

func buildClashStandardURL(scheme string, proxy map[string]interface{}, user string) string {
	q := url.Values{}
	if tls := clashTLSValue(proxy); tls != "" {
		q.Set("security", tls)
	}
	if sni := firstNonEmpty(stringFromAny(proxy["servername"]), stringFromAny(proxy["sni"])); sni != "" {
		q.Set("sni", sni)
	}
	if network := firstNonEmpty(stringFromAny(proxy["network"]), stringFromAny(proxy["net"])); network != "" {
		q.Set("type", network)
	}
	if flow := stringFromAny(proxy["flow"]); flow != "" {
		q.Set("flow", flow)
	}
	if reality, ok := proxy["reality-opts"].(map[string]interface{}); ok {
		if publicKey := firstNonEmpty(stringFromAny(reality["public-key"]), stringFromAny(reality["publicKey"])); publicKey != "" {
			q.Set("pbk", publicKey)
		}
		if shortID := firstNonEmpty(stringFromAny(reality["short-id"]), stringFromAny(reality["shortId"])); shortID != "" {
			q.Set("sid", shortID)
		}
	}
	if isTruthy(proxy["skip-cert-verify"]) {
		q.Set("allowInsecure", "1")
	}
	if ws, ok := proxy["ws-opts"].(map[string]interface{}); ok {
		if path := stringFromAny(ws["path"]); path != "" {
			q.Set("path", path)
		}
		if headers, ok := ws["headers"].(map[string]interface{}); ok {
			if host := stringFromAny(headers["Host"]); host != "" {
				q.Set("host", host)
			}
		}
	}
	if grpc, ok := proxy["grpc-opts"].(map[string]interface{}); ok {
		if serviceName := firstNonEmpty(stringFromAny(grpc["grpc-service-name"]), stringFromAny(grpc["serviceName"]), stringFromAny(grpc["service-name"])); serviceName != "" {
			q.Set("serviceName", serviceName)
		}
	}
	return fmt.Sprintf("%s://%s@%s:%d?%s#%s", scheme, url.PathEscape(user), stringFromAny(proxy["server"]), intFromAny(proxy["port"]), q.Encode(), url.QueryEscape(stringFromAny(proxy["name"])))
}

func buildClashTUIC(proxy map[string]interface{}) string {
	user := firstNonEmpty(stringFromAny(proxy["uuid"]), stringFromAny(proxy["id"]))
	password := stringFromAny(proxy["password"])
	q := url.Values{}
	if sni := firstNonEmpty(stringFromAny(proxy["servername"]), stringFromAny(proxy["sni"])); sni != "" {
		q.Set("sni", sni)
	}
	if isTruthy(proxy["skip-cert-verify"]) {
		q.Set("allowInsecure", "1")
	}
	return fmt.Sprintf("tuic://%s:%s@%s:%d?%s#%s", url.PathEscape(user), url.PathEscape(password), stringFromAny(proxy["server"]), intFromAny(proxy["port"]), q.Encode(), url.QueryEscape(stringFromAny(proxy["name"])))
}

func buildClashSOCKS(proxy map[string]interface{}) string {
	user := stringFromAny(proxy["username"])
	password := stringFromAny(proxy["password"])
	auth := ""
	if user != "" {
		auth = url.PathEscape(user)
		if password != "" {
			auth += ":" + url.PathEscape(password)
		}
		auth += "@"
	}
	return fmt.Sprintf("socks5://%s%s:%d#%s", auth, stringFromAny(proxy["server"]), intFromAny(proxy["port"]), url.QueryEscape(stringFromAny(proxy["name"])))
}

func parseClashHTTP(proxy map[string]interface{}) (Node, error) {
	host := stringFromAny(proxy["server"])
	port := intFromAny(proxy["port"])
	if host == "" || port == 0 {
		return Node{}, errors.New("HTTP 节点缺少地址或端口")
	}
	outbound := map[string]interface{}{
		"type":        "http",
		"tag":         tagFromRaw(fmt.Sprintf("http|%s|%d|%s", host, port, stringFromAny(proxy["name"]))),
		"server":      host,
		"server_port": port,
	}
	if username := stringFromAny(proxy["username"]); username != "" {
		outbound["username"] = username
		if password := stringFromAny(proxy["password"]); password != "" {
			outbound["password"] = password
		}
	}
	raw, _ := json.Marshal(outbound)
	return newNode(firstNonEmpty(stringFromAny(proxy["name"]), "HTTP"), "http", host, port, string(raw), outbound), nil
}

func clashTLSValue(proxy map[string]interface{}) string {
	if isTruthy(proxy["reality-opts"]) {
		return "reality"
	}
	if isTruthy(proxy["tls"]) {
		return "tls"
	}
	return stringFromAny(proxy["tls"])
}

func clashGroupNames(raw interface{}) map[string]string {
	result := map[string]string{}
	groups, ok := raw.([]interface{})
	if !ok {
		return result
	}
	for _, item := range groups {
		group, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringFromAny(group["name"])
		proxies, ok := group["proxies"].([]interface{})
		if name == "" || !ok {
			continue
		}
		for _, proxyName := range proxies {
			pn := stringFromAny(proxyName)
			if pn != "" && result[pn] == "" {
				result[pn] = name
			}
		}
	}
	return result
}

func subscriptionGroupName(sourceURL string, headers http.Header) string {
	for _, key := range []string{"Profile-Title", "profile-title", "Subscription-Title", "Group-Name", "group-name"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return decodeHeaderName(value)
		}
	}
	if u, err := url.Parse(sourceURL); err == nil {
		name := strings.TrimSpace(u.Query().Get("name"))
		if name == "" {
			name = strings.TrimSpace(u.Query().Get("tag"))
		}
		if name != "" {
			return decodeName(name, name)
		}
		if u.Host != "" {
			return u.Host
		}
	}
	return "订阅组"
}

func decodeHeaderName(value string) string {
	if decoded, err := url.QueryUnescape(value); err == nil {
		return decoded
	}
	return value
}

func decodeBase64Text(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, " ", "")
	if value == "" {
		return "", errors.New("内容为空")
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if data, err := enc.DecodeString(value); err == nil && len(data) > 0 {
			return string(data), nil
		}
	}
	return "", errors.New("Base64 解码失败")
}

func groupIDFromName(name string) string {
	hash := tagFromRaw("group|" + name)
	return "group-" + strings.TrimPrefix(hash, "node-")
}

func dedupeNodes(nodes []Node) []Node {
	result := make([]Node, 0, len(nodes))
	seen := map[string]bool{}
	for _, node := range nodes {
		if node.ID == "" || seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		result = append(result, node)
	}
	return result
}

func dedupeGroups(groups []NodeGroup) []NodeGroup {
	result := make([]NodeGroup, 0, len(groups))
	seen := map[string]bool{}
	for _, group := range groups {
		if group.ID == "" || seen[group.ID] {
			continue
		}
		seen[group.ID] = true
		result = append(result, group)
	}
	return result
}

func compactErrors(errs []string) []string {
	result := make([]string, 0, len(errs))
	seen := map[string]bool{}
	for _, err := range errs {
		err = strings.TrimSpace(err)
		if err == "" || seen[err] {
			continue
		}
		seen[err] = true
		result = append(result, err)
		if len(result) >= 20 {
			break
		}
	}
	return result
}

func isTruthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return isTrue(v)
	default:
		return value != nil
	}
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
