package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func ParseNodes(input string) ([]Node, []string) {
	return parseNodes(input, false)
}

func ParseNodesPreserveDuplicates(input string) ([]Node, []string) {
	return parseNodes(input, true)
}

func parseNodes(input string, preserveDuplicates bool) ([]Node, []string) {
	lines := ExtractNodeLines(input)
	nodes := make([]Node, 0, len(lines))
	errs := make([]string, 0)
	seen := map[string]bool{}

	for _, line := range lines {
		node, err := parseNode(line)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s：%v", trimForLog(line), err))
			continue
		}
		if !preserveDuplicates && seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		nodes = append(nodes, node)
	}
	return nodes, errs
}

func splitNodeInput(input string) []string {
	return ExtractNodeLines(input)
}

func ExtractNodeLines(input string) []string {
	replacer := strings.NewReplacer("\r", "\n", "\t", "\n", " ", "\n")
	raw := strings.Split(replacer.Replace(input), "\n")
	lines := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			lines = append(lines, item)
		}
	}
	return lines
}

func parseNode(raw string) (Node, error) {
	if strings.HasPrefix(raw, "{") {
		return parseJSONOutbound(raw)
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "ss://"):
		return parseShadowsocks(raw)
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMess(raw)
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESS(raw)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojan(raw)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2(raw)
	case strings.HasPrefix(lower, "socks://"), strings.HasPrefix(lower, "socks5://"):
		return parseSOCKS(raw)
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return parseHTTPProxy(raw)
	case strings.HasPrefix(lower, "tuic://"):
		return parseTUIC(raw)
	case strings.HasPrefix(lower, "anytls://"):
		return parseAnyTLS(raw)
	case strings.HasPrefix(lower, "wireguard://"):
		return parseWireGuard(raw)
	case strings.HasPrefix(lower, "naive://"), strings.HasPrefix(lower, "naive+https://"), strings.HasPrefix(lower, "naive+quic://"):
		return parseNaive(raw)
	default:
		if looksLikePlainSOCKS(raw) {
			return parsePlainSOCKS5(raw)
		}
		return Node{}, errors.New("暂不支持该节点格式")
	}
}

func parseJSONOutbound(raw string) (Node, error) {
	var outbound map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &outbound); err != nil {
		return Node{}, err
	}
	typ, _ := outbound["type"].(string)
	server, _ := outbound["server"].(string)
	if typ == "" || server == "" {
		return Node{}, errors.New("JSON 节点缺少 type 或 server")
	}
	port := intFromAny(outbound["server_port"])
	name, _ := outbound["tag"].(string)
	if name == "" {
		name = fmt.Sprintf("%s-%s:%d", typ, server, port)
	}
	outbound["tag"] = tagFromRaw(raw)
	return newNode(name, typ, server, port, raw, outbound), nil
}

func parseShadowsocks(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	name := decodeName(u.Fragment, "Shadowsocks")
	main := u.Host
	user := u.User.String()
	if user != "" && !strings.Contains(user, ":") {
		if decoded, err := decodeBase64URL(user); err == nil && strings.Contains(decoded, ":") {
			user = decoded
		}
	}
	if user == "" {
		decoded, err := decodeBase64URL(strings.TrimPrefix(raw, "ss://"))
		if err == nil {
			hash := strings.Index(decoded, "#")
			if hash >= 0 {
				name = decodeName(decoded[hash+1:], name)
				decoded = decoded[:hash]
			}
			at := strings.LastIndex(decoded, "@")
			if at >= 0 {
				user = decoded[:at]
				main = decoded[at+1:]
			}
		}
	}
	if user == "" && u.Opaque != "" {
		decoded, err := decodeBase64URL(u.Opaque)
		if err == nil {
			user = decoded
		}
	}
	method, password, ok := strings.Cut(user, ":")
	if !ok || method == "" || password == "" {
		return Node{}, errors.New("Shadowsocks 节点缺少加密方法或密码")
	}
	host, port, err := splitHostPort(main)
	if err != nil {
		return Node{}, err
	}
	outbound := map[string]interface{}{
		"type":        "shadowsocks",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"method":      method,
		"password":    password,
	}
	addNetworkUOTAndMux(outbound, u.Query(), true, true)
	return newNode(name, "shadowsocks", host, port, raw, outbound), nil
}

func parseVMess(raw string) (Node, error) {
	body := strings.TrimPrefix(raw, "vmess://")
	decoded, err := decodeBase64URL(body)
	if err != nil {
		return Node{}, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &data); err != nil {
		return Node{}, err
	}
	host := stringFromAny(data["add"])
	port, _ := strconv.Atoi(stringFromAny(data["port"]))
	uuid := stringFromAny(data["id"])
	if host == "" || port == 0 || uuid == "" {
		return Node{}, errors.New("VMess 节点缺少地址、端口或 UUID")
	}
	name := stringFromAny(data["ps"])
	if name == "" {
		name = "VMess"
	}
	security := stringFromAny(data["scy"])
	if security == "" {
		security = "auto"
	}
	outbound := map[string]interface{}{
		"type":        "vmess",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"uuid":        uuid,
		"security":    security,
	}
	if aid := intFromAny(data["aid"]); aid > 0 {
		outbound["alter_id"] = aid
	}
	q := queryLikeFromVMess(data)
	addTLSAndTransport(outbound, q, stringFromAny(data["net"]))
	addNetworkUOTAndMux(outbound, q, false, true)
	if packetEncoding := firstNonEmpty(stringFromAny(data["packetEncoding"]), stringFromAny(data["packet_encoding"])); packetEncoding != "" {
		outbound["packet_encoding"] = packetEncoding
	}
	return newNode(name, "vmess", host, port, raw, outbound), nil
}

func parseVLESS(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	if u.User.Username() == "" {
		return Node{}, errors.New("VLESS 节点缺少 UUID")
	}
	q := u.Query()
	outbound := map[string]interface{}{
		"type":        "vless",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"uuid":        u.User.Username(),
	}
	if flow := q.Get("flow"); flow != "" {
		outbound["flow"] = flow
	}
	if packetEncoding := firstNonEmpty(q.Get("packetEncoding"), q.Get("packet_encoding")); packetEncoding != "" {
		outbound["packet_encoding"] = packetEncoding
	}
	addTLSAndTransport(outbound, q, q.Get("type"))
	addRawTransportOptions(outbound, q)
	addNetworkUOTAndMux(outbound, q, false, true)
	return newNode(decodeName(u.Fragment, "VLESS"), "vless", host, port, raw, outbound), nil
}

func parseTrojan(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	password := u.User.Username()
	if password == "" {
		return Node{}, errors.New("Trojan 节点缺少密码")
	}
	q := u.Query()
	outbound := map[string]interface{}{
		"type":        "trojan",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"password":    password,
	}
	addTLSAndTransport(outbound, q, q.Get("type"))
	addNetworkUOTAndMux(outbound, q, false, true)
	return newNode(decodeName(u.Fragment, "Trojan"), "trojan", host, port, raw, outbound), nil
}

func parseHysteria2(raw string) (Node, error) {
	raw = strings.Replace(raw, "hy2://", "hysteria2://", 1)
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	password := u.User.Username()
	if password == "" {
		return Node{}, errors.New("Hysteria2 节点缺少密码")
	}
	q := u.Query()
	tls := map[string]interface{}{"enabled": true}
	if sni := firstNonEmpty(q.Get("sni"), q.Get("peer")); sni != "" {
		tls["server_name"] = sni
	}
	addTLSOptions(tls, q)
	if tls["certificate"] == nil && tls["certificate_public_key_sha256"] == nil && isTrue(q.Get("insecure")) {
		tls["insecure"] = true
	}
	outbound := map[string]interface{}{
		"type":        "hysteria2",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"password":    password,
		"tls":         tls,
	}
	addNetwork(outbound, q)
	if ports := firstNonEmpty(q.Get("mport"), q.Get("ports"), q.Get("server_ports")); ports != "" {
		outbound["server_ports"] = splitCSV(ports)
	}
	if hop := firstNonEmpty(q.Get("hop"), q.Get("hop_interval")); hop != "" {
		outbound["hop_interval"] = normalizeDurationText(hop)
	}
	if up := intFromString(firstNonEmpty(q.Get("upmbps"), q.Get("up_mbps"), q.Get("up"))); up > 0 {
		outbound["up_mbps"] = up
	}
	if down := intFromString(firstNonEmpty(q.Get("downmbps"), q.Get("down_mbps"), q.Get("down"))); down > 0 {
		outbound["down_mbps"] = down
	}
	if obfsPassword := firstNonEmpty(q.Get("obfs-password"), q.Get("obfs_password"), q.Get("obfs")); obfsPassword != "" {
		outbound["obfs"] = map[string]interface{}{"type": "salamander", "password": obfsPassword}
	}
	return newNode(decodeName(u.Fragment, "Hysteria2"), "hysteria2", host, port, raw, outbound), nil
}

func parseSOCKS(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	outbound := map[string]interface{}{
		"type":        "socks",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"version":     "5",
	}
	if username := u.User.Username(); username != "" {
		outbound["username"] = username
		if password, ok := u.User.Password(); ok {
			outbound["password"] = password
		}
	}
	addNetworkUOTAndMux(outbound, u.Query(), true, false)
	return newNode(decodeName(u.Fragment, "SOCKS5"), "socks5", host, port, raw, outbound), nil
}

func parseHTTPProxy(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	outbound := map[string]interface{}{
		"type":        "http",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
	}
	if username := u.User.Username(); username != "" {
		outbound["username"] = username
		if password, ok := u.User.Password(); ok {
			outbound["password"] = password
		}
	}
	if strings.EqualFold(u.Scheme, "https") {
		tls := map[string]interface{}{"enabled": true}
		addTLSOptions(tls, u.Query())
		outbound["tls"] = tls
	}
	if path := u.Query().Get("path"); path != "" {
		outbound["path"] = path
	}
	if hostHeader := u.Query().Get("host"); hostHeader != "" {
		outbound["headers"] = map[string]interface{}{"Host": hostHeader}
	}
	return newNode(decodeName(u.Fragment, "HTTP"), "http", host, port, raw, outbound), nil
}

func parsePlainSOCKS5(raw string) (Node, error) {
	host, port, username, password, err := splitPlainSOCKS5(raw)
	if err != nil {
		return Node{}, err
	}
	outbound := map[string]interface{}{
		"type":        "socks",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"version":     "5",
		"username":    username,
		"password":    password,
	}
	name := fmt.Sprintf("SOCKS5-%s:%d", host, port)
	return newNode(name, "socks5", host, port, raw, outbound), nil
}

func parseTUIC(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	uuid := u.User.Username()
	password, _ := u.User.Password()
	if uuid == "" || password == "" {
		return Node{}, errors.New("TUIC 节点缺少 UUID 或密码")
	}
	q := u.Query()
	tls := map[string]interface{}{"enabled": true}
	addTLSOptions(tls, q)
	outbound := map[string]interface{}{
		"type":        "tuic",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"uuid":        uuid,
		"password":    password,
		"tls":         tls,
	}
	addNetwork(outbound, q)
	if cc := firstNonEmpty(q.Get("congestion_control"), q.Get("congestion")); cc != "" {
		outbound["congestion_control"] = cc
	}
	return newNode(decodeName(u.Fragment, "TUIC"), "tuic", host, port, raw, outbound), nil
}

func parseAnyTLS(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	password := u.User.Username()
	if password == "" {
		return Node{}, errors.New("AnyTLS 节点缺少密码")
	}
	q := u.Query()
	tls := map[string]interface{}{"enabled": true}
	addTLSOptions(tls, q)
	if strings.EqualFold(firstNonEmpty(q.Get("security"), q.Get("tls")), "reality") {
		tls["reality"] = map[string]interface{}{
			"enabled":    true,
			"public_key": q.Get("pbk"),
			"short_id":   q.Get("sid"),
		}
	}
	outbound := map[string]interface{}{
		"type":        "anytls",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"password":    password,
		"tls":         tls,
	}
	if interval := q.Get("idle_session_check_interval"); interval != "" {
		outbound["idle_session_check_interval"] = normalizeDurationText(interval)
	}
	if timeout := q.Get("idle_session_timeout"); timeout != "" {
		outbound["idle_session_timeout"] = normalizeDurationText(timeout)
	}
	if minIdle := intFromString(q.Get("min_idle_session")); minIdle > 0 {
		outbound["min_idle_session"] = minIdle
	}
	return newNode(decodeName(u.Fragment, "AnyTLS"), "anytls", host, port, raw, outbound), nil
}

func parseWireGuard(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	privateKey := firstNonEmpty(u.User.Username(), q.Get("private_key"), q.Get("privateKey"))
	publicKey := firstNonEmpty(q.Get("publickey"), q.Get("public_key"), q.Get("publicKey"), q.Get("peer_public_key"), q.Get("peerPublicKey"))
	if privateKey == "" || publicKey == "" {
		return Node{}, errors.New("WireGuard 节点缺少 PrivateKey 或 PublicKey")
	}
	localAddress := splitCSV(firstNonEmpty(q.Get("address"), q.Get("local_address"), q.Get("localAddress")))
	if len(localAddress) == 0 {
		return Node{}, errors.New("WireGuard 节点缺少本地地址")
	}
	outbound := map[string]interface{}{
		"type":            "wireguard",
		"tag":             tagFromRaw(raw),
		"server":          host,
		"server_port":     port,
		"private_key":     privateKey,
		"peer_public_key": publicKey,
		"local_address":   localAddress,
	}
	if preSharedKey := firstNonEmpty(q.Get("presharedkey"), q.Get("pre_shared_key"), q.Get("preSharedKey"), q.Get("psk")); preSharedKey != "" {
		outbound["pre_shared_key"] = preSharedKey
	}
	if reserved := parseReserved(q.Get("reserved")); len(reserved) == 3 {
		outbound["reserved"] = reserved
	}
	if mtu := intFromString(q.Get("mtu")); mtu > 0 {
		outbound["mtu"] = mtu
	}
	addNetwork(outbound, q)
	return newNode(decodeName(u.Fragment, "WireGuard"), "wireguard", host, port, raw, outbound), nil
}

func parseNaive(raw string) (Node, error) {
	quicScheme := strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "naive+quic://")
	normalized := strings.Replace(raw, "naive+https://", "naive://", 1)
	normalized = strings.Replace(normalized, "naive+quic://", "naive://", 1)
	u, err := url.Parse(normalized)
	if err != nil {
		return Node{}, err
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Node{}, err
	}
	username := u.User.Username()
	password, _ := u.User.Password()
	if username == "" || password == "" {
		return Node{}, errors.New("Naive 节点缺少用户名或密码")
	}
	q := u.Query()
	tls := map[string]interface{}{"enabled": true}
	addNaiveTLSOptions(tls, q)
	outbound := map[string]interface{}{
		"type":        "naive",
		"tag":         tagFromRaw(raw),
		"server":      host,
		"server_port": port,
		"username":    username,
		"password":    password,
		"tls":         tls,
	}
	if concurrency := intFromString(firstNonEmpty(q.Get("insecure-concurrency"), q.Get("insecure_concurrency"))); concurrency > 0 {
		outbound["insecure_concurrency"] = concurrency
	}
	if isTrue(q.Get("udp_over_tcp")) || isTrue(q.Get("uot")) {
		outbound["udp_over_tcp"] = map[string]interface{}{"enabled": true}
	}
	if quicScheme || isTrue(q.Get("quic")) {
		outbound["quic"] = true
	}
	if cc := firstNonEmpty(q.Get("quic_congestion_control"), q.Get("congestion")); cc != "" {
		outbound["quic_congestion_control"] = cc
	}
	return newNode(decodeName(u.Fragment, "Naive"), "naive", host, port, raw, outbound), nil
}

func addTLSAndTransport(outbound map[string]interface{}, q url.Values, network string) {
	security := strings.ToLower(firstNonEmpty(q.Get("security"), q.Get("tls")))
	if security == "tls" || security == "reality" || security == "xtls" || security == "1" {
		tls := map[string]interface{}{"enabled": true}
		if serverName := firstNonEmpty(q.Get("sni"), q.Get("servername"), q.Get("peer")); serverName != "" {
			tls["server_name"] = serverName
		}
		addTLSOptions(tls, q)
		if security == "reality" {
			tls["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": q.Get("pbk"),
				"short_id":   q.Get("sid"),
			}
			utls := map[string]interface{}{"enabled": true}
			if fingerprint := firstNonEmpty(q.Get("fp"), q.Get("fingerprint")); fingerprint != "" {
				utls["fingerprint"] = fingerprint
			}
			tls["utls"] = utls
		}
		outbound["tls"] = tls
	}

	switch strings.ToLower(network) {
	case "ws", "websocket":
		transport := map[string]interface{}{"type": "ws"}
		if path := q.Get("path"); path != "" {
			transport["path"] = path
		}
		if host := q.Get("host"); host != "" {
			transport["headers"] = map[string]interface{}{"Host": host}
		}
		outbound["transport"] = transport
	case "grpc":
		transport := map[string]interface{}{"type": "grpc"}
		if serviceName := firstNonEmpty(q.Get("serviceName"), q.Get("service_name")); serviceName != "" {
			transport["service_name"] = serviceName
		}
		outbound["transport"] = transport
	case "http", "h2":
		outbound["transport"] = map[string]interface{}{"type": "http"}
	case "httpupgrade":
		outbound["transport"] = map[string]interface{}{"type": "httpupgrade"}
	}
}

func addRawTransportOptions(outbound map[string]interface{}, q url.Values) {
	rawType := strings.ToLower(firstNonEmpty(q.Get("rawType"), q.Get("headerType")))
	finalMask := strings.TrimSpace(q.Get("finalMask"))
	if rawType == "" || rawType == "none" {
		if finalMask == "" {
			return
		}
		rawType = "http"
	}
	if rawType != "http" {
		return
	}
	transport, _ := outbound["transport"].(map[string]interface{})
	if transport == nil || transport["type"] == nil || strings.EqualFold(stringFromAny(transport["type"]), "tcp") {
		transport = map[string]interface{}{"type": "raw"}
	}
	headers := map[string]interface{}{"type": "http"}
	if finalMask != "" {
		headers["request"] = map[string]interface{}{
			"path": splitLinesOrCSV(finalMask),
		}
	}
	transport["headers"] = headers
	outbound["transport"] = transport
}

func queryLikeFromVMess(data map[string]interface{}) url.Values {
	q := url.Values{}
	for key, value := range map[string]string{
		"security": stringFromAny(data["tls"]),
		"sni":      firstNonEmpty(stringFromAny(data["sni"]), stringFromAny(data["host"])),
		"host":     stringFromAny(data["host"]),
		"path":     stringFromAny(data["path"]),
		"serviceName": firstNonEmpty(
			stringFromAny(data["serviceName"]),
			stringFromAny(data["service_name"]),
		),
		"mux":            boolLikeString(data["mux"]),
		"multiplex":      boolLikeString(data["multiplex"]),
		"packetEncoding": firstNonEmpty(stringFromAny(data["packetEncoding"]), stringFromAny(data["packet_encoding"])),
		"alpn":           stringFromAny(data["alpn"]),
		"fp":             firstNonEmpty(stringFromAny(data["fp"]), stringFromAny(data["fingerprint"])),
		"allowInsecure":  firstNonEmpty(boolLikeString(data["allowInsecure"]), boolLikeString(data["insecure"])),
		"certificate":    firstNonEmpty(stringFromAny(data["certificate"]), stringFromAny(data["cert"]), stringFromAny(data["fixed_certificate"])),
		"certificate_public_key_sha256": firstNonEmpty(
			stringFromAny(data["certificate_public_key_sha256"]),
			stringFromAny(data["cert_fp"]),
			stringFromAny(data["certFingerprint"]),
		),
	} {
		if value != "" {
			q.Set(key, value)
		}
	}
	echVal := firstNonEmpty(stringFromAny(data["ech_config"]), stringFromAny(data["echConfigList"]))
	echFlag := boolLikeString(data["ech"])
	if isExplicitFalse(echFlag) || isExplicitFalse(echVal) {
		q.Set("ech", "false")
	} else if isExplicitTrue(echFlag) {
		q.Set("ech", "true")
		if echVal != "" && !isExplicitTrue(echVal) && !isExplicitFalse(echVal) {
			q.Set("ech_config", echVal)
		}
	} else if echVal != "" && !isExplicitFalse(echVal) {
		q.Set("ech_config", echVal)
	}
	return q
}

func parseECHFromQuery(q url.Values) map[string]interface{} {
	echFlag := firstNonEmpty(q.Get("ech"), q.Get("enable_ech"))
	echConfig := firstNonEmpty(q.Get("ech_config"), q.Get("echConfigList"))

	if isExplicitFalse(echFlag) || isExplicitFalse(echConfig) {
		return nil
	}

	enabled := isExplicitTrue(echFlag)
	if !enabled && echConfig != "" {
		enabled = true
	}
	if !enabled {
		return nil
	}

	ech := map[string]interface{}{"enabled": true}
	if echConfig != "" && !isExplicitTrue(echConfig) && !isExplicitFalse(echConfig) {
		lines := splitLinesOrCSV(echConfig)
		if len(lines) > 0 {
			ech["config"] = lines
		}
	}
	return ech
}

func addTLSOptions(tls map[string]interface{}, q url.Values) {
	if serverName := firstNonEmpty(q.Get("sni"), q.Get("servername"), q.Get("peer")); serverName != "" {
		tls["server_name"] = serverName
	}
	if isTrue(q.Get("allowInsecure")) || isTrue(q.Get("insecure")) {
		tls["insecure"] = true
	}
	if alpn := splitCSV(q.Get("alpn")); len(alpn) > 0 {
		tls["alpn"] = alpn
	}
	if fingerprint := firstNonEmpty(q.Get("fp"), q.Get("fingerprint")); fingerprint != "" {
		tls["utls"] = map[string]interface{}{"enabled": true, "fingerprint": fingerprint}
	}
	if certificate := firstNonEmpty(q.Get("certificate"), q.Get("cert"), q.Get("fixed_certificate")); certificate != "" {
		tls["certificate"] = splitLinesOrCSV(certificate)
	}
	if fingerprint := firstNonEmpty(q.Get("certificate_public_key_sha256"), q.Get("cert_fp"), q.Get("certFingerprint")); fingerprint != "" {
		tls["certificate_public_key_sha256"] = splitLinesOrCSV(fingerprint)
	}
	if ech := parseECHFromQuery(q); ech != nil {
		tls["ech"] = ech
	}
}

func addNaiveTLSOptions(tls map[string]interface{}, q url.Values) {
	if serverName := firstNonEmpty(q.Get("sni"), q.Get("servername"), q.Get("peer")); serverName != "" {
		tls["server_name"] = serverName
	}
	if certificate := firstNonEmpty(q.Get("certificate"), q.Get("cert"), q.Get("fixed_certificate")); certificate != "" {
		tls["certificate"] = splitLinesOrCSV(certificate)
	}
	if fingerprint := firstNonEmpty(q.Get("certificate_public_key_sha256"), q.Get("cert_fp"), q.Get("certFingerprint")); fingerprint != "" {
		tls["certificate_public_key_sha256"] = splitLinesOrCSV(fingerprint)
	}
	if ech := parseECHFromQuery(q); ech != nil {
		tls["ech"] = ech
	}
}

func addNetwork(outbound map[string]interface{}, q url.Values) {
	if network := strings.ToLower(firstNonEmpty(q.Get("network"), q.Get("net"))); network == "tcp" || network == "udp" {
		outbound["network"] = network
	}
}

func addNetworkUOTAndMux(outbound map[string]interface{}, q url.Values, allowUOT bool, allowMux bool) {
	addNetwork(outbound, q)
	if allowUOT && (isTrue(q.Get("udp_over_tcp")) || isTrue(q.Get("uot"))) {
		outbound["udp_over_tcp"] = map[string]interface{}{"enabled": true}
	}
	if allowMux && (isTrue(q.Get("mux")) || isTrue(q.Get("multiplex"))) {
		outbound["multiplex"] = map[string]interface{}{"enabled": true}
	}
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	result := make([]string, 0, len(fields))
	for _, item := range fields {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func splitLinesOrCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == '|'
	})
	result := make([]string, 0, len(fields))
	for _, item := range fields {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func parseReserved(value string) []int {
	parts := splitCSV(value)
	if len(parts) != 3 {
		return nil
	}
	result := make([]int, 0, 3)
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		result = append(result, n)
	}
	return result
}

func intFromString(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func normalizeDurationText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value + "s"
	}
	return value
}

func newNode(name, typ, address string, port int, raw string, outbound map[string]interface{}) Node {
	if name == "" {
		name = fmt.Sprintf("%s-%s:%d", typ, address, port)
	}
	return Node{
		ID:       tagFromRaw(raw),
		Name:     name,
		Type:     typ,
		Address:  address,
		Port:     port,
		Raw:      raw,
		Outbound: outbound,
	}
}

func splitHostPort(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		lastColon := strings.LastIndex(value, ":")
		if lastColon <= 0 {
			return "", 0, errors.New("节点缺少端口")
		}
		host = strings.Trim(value[:lastColon], "[]")
		portText = value[lastColon+1:]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("节点端口无效")
	}
	if host == "" {
		return "", 0, errors.New("节点地址为空")
	}
	return host, port, nil
}

func looksLikePlainSOCKS(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") || strings.HasPrefix(raw, "{") {
		return false
	}
	if strings.HasPrefix(raw, "[") {
		end := strings.Index(raw, "]")
		return end > 0 && strings.Count(raw[end+1:], ":") >= 3
	}
	return strings.Count(raw, ":") >= 3
}

func splitPlainSOCKS5(raw string) (string, int, string, string, error) {
	raw = strings.TrimSpace(raw)
	host := ""
	var parts []string
	if strings.HasPrefix(raw, "[") {
		end := strings.Index(raw, "]")
		if end <= 0 {
			return "", 0, "", "", errors.New("SOCKS5 节点地址无效")
		}
		host = raw[1:end]
		rest := raw[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", 0, "", "", errors.New("SOCKS5 节点缺少端口")
		}
		parts = strings.SplitN(rest[1:], ":", 3)
	} else {
		parts = strings.SplitN(raw, ":", 4)
		if len(parts) == 4 {
			host = parts[0]
			parts = parts[1:]
		}
	}
	if len(parts) != 3 || host == "" || parts[1] == "" || parts[2] == "" {
		return "", 0, "", "", errors.New("SOCKS5 格式应为 ip:port:user:pass")
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, "", "", errors.New("节点端口无效")
	}
	return host, port, parts[1], parts[2], nil
}

func decodeBase64URL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if i := strings.IndexAny(value, "#?"); i >= 0 {
		value = value[:i]
	}
	value = strings.ReplaceAll(value, "-", "+")
	value = strings.ReplaceAll(value, "_", "/")
	switch len(value) % 4 {
	case 2:
		value += "=="
	case 3:
		value += "="
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeName(value, fallback string) string {
	if value == "" {
		return fallback
	}
	name, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return name
}

func tagFromRaw(raw string) string {
	hash := sha1.Sum([]byte(raw))
	return "node-" + hex.EncodeToString(hash[:])[:12]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func isTrue(value string) bool {
	value = strings.ToLower(value)
	return value == "1" || value == "true" || value == "yes"
}

func isExplicitFalse(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "0" || value == "false" || value == "off" || value == "no" || value == "disable" || value == "disabled" || value == "none"
}

func isExplicitTrue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes" || value == "on" || value == "enable" || value == "enabled"
}

func boolLikeString(value interface{}) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return stringFromAny(value)
	}
}

func stringFromAny(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.Itoa(int(v))
	default:
		return ""
	}
}

func intFromAny(value interface{}) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func trimForLog(value string) string {
	if len(value) <= 48 {
		return value
	}
	return value[:48] + "..."
}
