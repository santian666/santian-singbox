package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func nodeShareLink(node Node) (string, error) {
	if raw, ok := originalProtocolShareLink(node.Raw); ok {
		return raw, nil
	}
	outbound := node.Outbound
	if len(outbound) == 0 && strings.HasPrefix(strings.TrimSpace(node.Raw), "{") {
		if err := json.Unmarshal([]byte(node.Raw), &outbound); err != nil {
			return "", err
		}
	}
	if len(outbound) == 0 {
		raw := strings.TrimSpace(node.Raw)
		if strings.Contains(raw, "://") {
			return raw, nil
		}
		return "", errors.New("节点缺少出站配置")
	}
	typ := normalizeShareType(firstNonEmpty(node.Type, stringFromAny(outbound["type"])))
	switch typ {
	case "shadowsocks":
		return shareShadowsocks(node, outbound)
	case "vmess":
		return shareVMess(node, outbound)
	case "vless":
		return shareVLESS(node, outbound)
	case "trojan":
		return shareTrojan(node, outbound)
	case "hysteria2":
		return shareHysteria2(node, outbound)
	case "socks5":
		return shareSOCKS(node, outbound)
	case "http":
		return shareHTTP(node, outbound)
	case "tuic":
		return shareTUIC(node, outbound)
	case "anytls":
		return shareAnyTLS(node, outbound)
	case "wireguard":
		return shareWireGuard(node, outbound)
	case "naive":
		return shareNaive(node, outbound)
	default:
		raw := strings.TrimSpace(node.Raw)
		if strings.Contains(raw, "://") {
			return raw, nil
		}
		return "", fmt.Errorf("暂不支持生成 %s 分享链接", typ)
	}
}

func originalProtocolShareLink(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "{") || !strings.Contains(raw, "://") {
		return "", false
	}
	if _, err := parseNode(raw); err != nil {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(raw), "vmess://") {
		decoded, err := decodeBase64URL(raw[len("vmess://"):])
		if err != nil || !json.Valid([]byte(decoded)) {
			return "", false
		}
		return "vmess://" + base64.StdEncoding.EncodeToString([]byte(decoded)), true
	}
	return raw, true
}

func normalizeShareType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ss", "shadowsocks":
		return "shadowsocks"
	case "vmess":
		return "vmess"
	case "vless":
		return "vless"
	case "trojan":
		return "trojan"
	case "hy2", "hysteria2":
		return "hysteria2"
	case "socks", "socks5":
		return "socks5"
	case "http", "https":
		return "http"
	case "tuic":
		return "tuic"
	case "anytls":
		return "anytls"
	case "wireguard":
		return "wireguard"
	case "naive", "naive+https", "naive+quic":
		return "naive"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func shareShadowsocks(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	method := stringFromAny(outbound["method"])
	password := stringFromAny(outbound["password"])
	if method == "" || password == "" {
		return "", errors.New("Shadowsocks 缺少加密方式或密码")
	}
	q := url.Values{}
	addShareNetwork(q, outbound)
	addShareBool(q, "udp_over_tcp", mapEnabled(outbound["udp_over_tcp"]))
	addShareBool(q, "mux", mapEnabled(outbound["multiplex"]))
	user := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
	return "ss://" + user + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareVMess(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	uuid := stringFromAny(outbound["uuid"])
	if uuid == "" {
		return "", errors.New("VMess 缺少 UUID")
	}
	tls := shareMap(outbound["tls"])
	transport := shareMap(outbound["transport"])
	data := map[string]interface{}{
		"v":    "2",
		"ps":   node.Name,
		"add":  server,
		"port": strconv.Itoa(port),
		"id":   uuid,
		"aid":  intFromAny(outbound["alter_id"]),
		"scy":  firstNonEmpty(stringFromAny(outbound["security"]), "auto"),
		"net":  shareTransportType(transport),
		"type": "none",
		"host": shareTransportHost(transport),
		"path": stringFromAny(transport["path"]),
	}
	if serviceName := stringFromAny(transport["service_name"]); serviceName != "" {
		data["serviceName"] = serviceName
	}
	if tlsEnabled(tls) {
		data["tls"] = "tls"
		data["sni"] = stringFromAny(tls["server_name"])
		data["alpn"] = shareStringList(tls["alpn"])
		data["fp"] = stringFromAny(shareMap(tls["utls"])["fingerprint"])
		data["allowInsecure"] = boolFromAny(tls["insecure"])
		addTLSShareFields(data, tls)
	}
	if mapEnabled(outbound["multiplex"]) {
		data["mux"] = true
	}
	if packetEncoding := stringFromAny(outbound["packet_encoding"]); packetEncoding != "" {
		data["packetEncoding"] = packetEncoding
	}
	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(body), nil
}

func shareVLESS(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	uuid := stringFromAny(outbound["uuid"])
	if uuid == "" {
		return "", errors.New("VLESS 缺少 UUID")
	}
	q := shareV2RayQuery(outbound, true)
	addShareValue(q, "encryption", firstNonEmpty(stringFromAny(outbound["encryption"]), "none"))
	addShareValue(q, "flow", stringFromAny(outbound["flow"]))
	addShareValue(q, "packetEncoding", stringFromAny(outbound["packet_encoding"]))
	return "vless://" + url.User(uuid).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareTrojan(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	password := stringFromAny(outbound["password"])
	if password == "" {
		return "", errors.New("Trojan 缺少密码")
	}
	q := shareV2RayQuery(outbound, false)
	return "trojan://" + url.User(password).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareHysteria2(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	password := stringFromAny(outbound["password"])
	if password == "" {
		return "", errors.New("Hysteria2 缺少密码")
	}
	q := url.Values{}
	addShareTLSQuery(q, shareMap(outbound["tls"]), false)
	addShareNetwork(q, outbound)
	addShareValue(q, "mport", shareStringList(outbound["server_ports"]))
	addShareValue(q, "hop", stringFromAny(outbound["hop_interval"]))
	addShareValue(q, "upmbps", stringFromAny(outbound["up_mbps"]))
	addShareValue(q, "downmbps", stringFromAny(outbound["down_mbps"]))
	if obfs := shareMap(outbound["obfs"]); strings.EqualFold(stringFromAny(obfs["type"]), "salamander") {
		addShareValue(q, "obfs-password", stringFromAny(obfs["password"]))
	}
	return "hysteria2://" + url.User(password).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareSOCKS(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	addShareNetwork(q, outbound)
	addShareBool(q, "udp_over_tcp", mapEnabled(outbound["udp_over_tcp"]))
	user := shareUserInfo(stringFromAny(outbound["username"]), stringFromAny(outbound["password"]))
	return "socks5://" + user + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareHTTP(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	tls := shareMap(outbound["tls"])
	scheme := "http"
	q := url.Values{}
	if tlsEnabled(tls) {
		scheme = "https"
		addShareTLSQuery(q, tls, false)
	}
	headers := shareMap(outbound["headers"])
	addShareValue(q, "host", stringFromAny(headers["Host"]))
	addShareValue(q, "path", stringFromAny(outbound["path"]))
	user := shareUserInfo(stringFromAny(outbound["username"]), stringFromAny(outbound["password"]))
	return scheme + "://" + user + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareTUIC(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	uuid := stringFromAny(outbound["uuid"])
	password := stringFromAny(outbound["password"])
	if uuid == "" || password == "" {
		return "", errors.New("TUIC 缺少 UUID 或密码")
	}
	q := url.Values{}
	addShareTLSQuery(q, shareMap(outbound["tls"]), false)
	addShareNetwork(q, outbound)
	addShareValue(q, "congestion_control", stringFromAny(outbound["congestion_control"]))
	return "tuic://" + url.UserPassword(uuid, password).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareAnyTLS(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	password := stringFromAny(outbound["password"])
	if password == "" {
		return "", errors.New("AnyTLS 缺少密码")
	}
	q := url.Values{}
	tls := shareMap(outbound["tls"])
	if realityEnabled(tls) {
		q.Set("security", "reality")
	} else {
		q.Set("security", "tls")
	}
	addShareTLSQuery(q, tls, true)
	addShareValue(q, "idle_session_check_interval", stringFromAny(outbound["idle_session_check_interval"]))
	addShareValue(q, "idle_session_timeout", stringFromAny(outbound["idle_session_timeout"]))
	addShareValue(q, "min_idle_session", stringFromAny(outbound["min_idle_session"]))
	return "anytls://" + url.User(password).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareWireGuard(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	privateKey := stringFromAny(outbound["private_key"])
	publicKey := firstNonEmpty(stringFromAny(outbound["peer_public_key"]), stringFromAny(outbound["public_key"]))
	if privateKey == "" || publicKey == "" {
		return "", errors.New("WireGuard 缺少 PrivateKey 或 PublicKey")
	}
	q := url.Values{}
	addShareValue(q, "publickey", publicKey)
	addShareValue(q, "presharedkey", stringFromAny(outbound["pre_shared_key"]))
	addShareValue(q, "reserved", shareStringList(outbound["reserved"]))
	addShareValue(q, "address", shareStringList(outbound["local_address"]))
	addShareValue(q, "mtu", stringFromAny(outbound["mtu"]))
	addShareNetwork(q, outbound)
	return "wireguard://" + url.User(privateKey).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareNaive(node Node, outbound map[string]interface{}) (string, error) {
	server, port, err := shareServerPort(node, outbound)
	if err != nil {
		return "", err
	}
	username := stringFromAny(outbound["username"])
	password := stringFromAny(outbound["password"])
	if username == "" || password == "" {
		return "", errors.New("Naive 缺少用户名或密码")
	}
	q := url.Values{}
	addShareTLSQuery(q, shareMap(outbound["tls"]), false)
	addShareBool(q, "udp_over_tcp", mapEnabled(outbound["udp_over_tcp"]))
	addShareValue(q, "quic_congestion_control", stringFromAny(outbound["quic_congestion_control"]))
	addShareValue(q, "insecure-concurrency", stringFromAny(outbound["insecure_concurrency"]))
	scheme := "naive+https://"
	if boolFromAny(outbound["quic"]) {
		scheme = "naive+quic://"
	}
	return scheme + url.UserPassword(username, password).String() + "@" + shareHostPort(server, port) + shareQuery(q) + shareFragment(node.Name), nil
}

func shareV2RayQuery(outbound map[string]interface{}, includeEncryption bool) url.Values {
	q := url.Values{}
	tls := shareMap(outbound["tls"])
	if tlsEnabled(tls) {
		if realityEnabled(tls) {
			q.Set("security", "reality")
		} else {
			q.Set("security", "tls")
		}
		addShareTLSQuery(q, tls, true)
	}
	transport := shareMap(outbound["transport"])
	typ := shareTransportType(transport)
	if typ != "" && typ != "tcp" {
		q.Set("type", typ)
	}
	addShareValue(q, "host", shareTransportHost(transport))
	addShareValue(q, "path", stringFromAny(transport["path"]))
	addShareValue(q, "serviceName", stringFromAny(transport["service_name"]))
	addShareNetwork(q, outbound)
	addShareBool(q, "mux", mapEnabled(outbound["multiplex"]))
	if includeEncryption {
		addShareValue(q, "encryption", stringFromAny(outbound["encryption"]))
	}
	return q
}

func addShareTLSQuery(q url.Values, tls map[string]interface{}, includeReality bool) {
	addShareValue(q, "sni", stringFromAny(tls["server_name"]))
	addShareBool(q, "allowInsecure", boolFromAny(tls["insecure"]))
	addShareBool(q, "insecure", boolFromAny(tls["insecure"]))
	addShareValue(q, "alpn", shareStringList(tls["alpn"]))
	addShareValue(q, "fp", stringFromAny(shareMap(tls["utls"])["fingerprint"]))
	addShareValue(q, "certificate", shareStringList(tls["certificate"]))
	addShareValue(q, "certificate_public_key_sha256", shareStringList(tls["certificate_public_key_sha256"]))
	if ech := shareMap(tls["ech"]); boolFromAny(ech["enabled"]) || ech["config"] != nil {
		if cfg := shareStringList(ech["config"]); cfg != "" {
			addShareValue(q, "ech_config", cfg)
		} else {
			addShareValue(q, "ech", "1")
		}
	}
	if includeReality {
		reality := shareMap(tls["reality"])
		addShareValue(q, "pbk", stringFromAny(reality["public_key"]))
		addShareValue(q, "sid", stringFromAny(reality["short_id"]))
	}
}

func addTLSShareFields(data map[string]interface{}, tls map[string]interface{}) {
	if v := shareStringList(tls["certificate"]); v != "" {
		data["certificate"] = v
	}
	if v := shareStringList(tls["certificate_public_key_sha256"]); v != "" {
		data["certificate_public_key_sha256"] = v
	}
	if ech := shareMap(tls["ech"]); boolFromAny(ech["enabled"]) || ech["config"] != nil {
		if v := shareStringList(ech["config"]); v != "" {
			data["ech_config"] = v
		} else {
			data["ech"] = true
		}
	}
}

func shareServerPort(node Node, outbound map[string]interface{}) (string, int, error) {
	server := firstNonEmpty(node.Address, stringFromAny(outbound["server"]))
	port := node.Port
	if port == 0 {
		port = intFromAny(outbound["server_port"])
	}
	if server == "" || port <= 0 || port > 65535 {
		return "", 0, errors.New("节点缺少地址或端口")
	}
	return server, port, nil
}

func shareHostPort(server string, port int) string {
	return net.JoinHostPort(server, strconv.Itoa(port))
}

func shareQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func shareFragment(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "#" + url.QueryEscape(name)
}

func shareUserInfo(username, password string) string {
	if username == "" {
		return ""
	}
	if password == "" {
		return url.User(username).String() + "@"
	}
	return url.UserPassword(username, password).String() + "@"
}

func addShareValue(q url.Values, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		q.Set(key, value)
	}
}

func addShareBool(q url.Values, key string, value bool) {
	if value {
		q.Set(key, "1")
	}
}

func addShareNetwork(q url.Values, outbound map[string]interface{}) {
	network := strings.ToLower(stringFromAny(outbound["network"]))
	if network != "" && network != "tcp" {
		q.Set("network", network)
	}
}

func shareTransportType(transport map[string]interface{}) string {
	typ := strings.ToLower(stringFromAny(transport["type"]))
	switch typ {
	case "", "tcp", "raw":
		return "tcp"
	case "websocket":
		return "ws"
	default:
		return typ
	}
}

func shareTransportHost(transport map[string]interface{}) string {
	headers := shareMap(transport["headers"])
	return firstNonEmpty(stringFromAny(headers["Host"]), stringFromAny(headers["host"]))
}

func tlsEnabled(tls map[string]interface{}) bool {
	return len(tls) > 0 && (boolFromAny(tls["enabled"]) || tls["server_name"] != nil || tls["utls"] != nil || tls["reality"] != nil)
}

func realityEnabled(tls map[string]interface{}) bool {
	reality := shareMap(tls["reality"])
	return boolFromAny(reality["enabled"]) || stringFromAny(reality["public_key"]) != ""
}

func mapEnabled(value interface{}) bool {
	return boolFromAny(shareMap(value)["enabled"])
}

func shareMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if item, ok := value.(map[string]interface{}); ok {
		return item
	}
	return nil
}

func boolFromAny(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return isTrue(v)
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

func shareStringList(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.Join(v, ",")
	case []int:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, strconv.Itoa(item))
		}
		return strings.Join(parts, ",")
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := stringFromAny(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ",")
	default:
		return stringFromAny(value)
	}
}
