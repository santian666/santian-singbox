package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestNodeShareLinkFromShadowsocksOutboundJSON(t *testing.T) {
	raw := `{"method":"aes-128-gcm","password":"c36a0478-6ba9-41ef-99de-d2f9d6e99294","server":"gheianygh.owolist.cn","server_port":31004,"tag":"香港 04","type":"shadowsocks"}`
	node, err := parseJSONOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	link, err := nodeShareLink(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "ss://") {
		t.Fatalf("want ss:// link, got %s", link)
	}
	nodes, errs := ParseNodes(link)
	if len(errs) > 0 {
		t.Fatalf("generated link parse errors: %v", errs)
	}
	if len(nodes) != 1 {
		t.Fatalf("want one node, got %d", len(nodes))
	}
	got := nodes[0].Outbound
	if got["server"] != "gheianygh.owolist.cn" || intFromAny(got["server_port"]) != 31004 {
		t.Fatalf("server mismatch: %#v", got)
	}
	if got["method"] != "aes-128-gcm" || got["password"] != "c36a0478-6ba9-41ef-99de-d2f9d6e99294" {
		t.Fatalf("auth mismatch: %#v", got)
	}
}

func TestNodeShareLinkKeepsProtocolLinks(t *testing.T) {
	raw := "vless://uuid@example.com:443?security=tls&sni=example.com&extra=keep-me#demo"
	node, err := parseNode(raw)
	if err != nil {
		t.Fatal(err)
	}
	link, err := nodeShareLink(node)
	if err != nil {
		t.Fatal(err)
	}
	if link != raw {
		t.Fatalf("原始协议参数应完整保留，实际：%s", link)
	}
}

func TestNodeShareLinkUsesV2rayNWireGuardParameters(t *testing.T) {
	raw := `{"type":"wireguard","tag":"wg","server":"example.com","server_port":51820,"private_key":"private","peer_public_key":"public","pre_shared_key":"shared","local_address":["10.0.0.2/32"]}`
	node, err := parseJSONOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	link, err := nodeShareLink(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link, "publickey=public") || !strings.Contains(link, "presharedkey=shared") {
		t.Fatalf("WireGuard 分享参数不兼容 v2rayN：%s", link)
	}
	if nodes, errs := ParseNodes(link); len(nodes) != 1 || len(errs) != 0 {
		t.Fatalf("WireGuard 分享链接无法重新解析：nodes=%d errs=%v", len(nodes), errs)
	}
}

func TestNodeShareLinkUsesV2rayNNaiveFormat(t *testing.T) {
	raw := `{"type":"naive","tag":"naive","server":"example.com","server_port":443,"username":"user","password":"pass","insecure_concurrency":4,"quic":true,"tls":{"enabled":true}}`
	node, err := parseJSONOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	link, err := nodeShareLink(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "naive+quic://") || !strings.Contains(link, "insecure-concurrency=4") {
		t.Fatalf("Naive 分享格式不兼容 v2rayN：%s", link)
	}
	if nodes, errs := ParseNodes(link); len(nodes) != 1 || len(errs) != 0 {
		t.Fatalf("Naive 分享链接无法重新解析：nodes=%d errs=%v", len(nodes), errs)
	}
}

func TestNodeShareLinkUsesStandardVMessBase64AndKeepsFields(t *testing.T) {
	payload := `{"v":"2","ps":"香港 测试","add":"example.com","port":"443","id":"11111111-1111-4111-8111-111111111111","aid":"0","net":"tcp","type":"none","tls":"tls","unknown":"keep-me"}`
	raw := "vmess://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
	node, err := parseNode(raw)
	if err != nil {
		t.Fatal(err)
	}
	link, err := nodeShareLink(node)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
	if err != nil {
		t.Fatalf("VMess 分享链接不是标准 Base64：%v，链接=%s", err, link)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(decoded, &data); err != nil {
		t.Fatal(err)
	}
	if data["unknown"] != "keep-me" {
		t.Fatalf("VMess 原始扩展字段丢失：%s", decoded)
	}
}

func TestGeneratedShareLinksCanBeParsedAgain(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		raw    string
	}{
		{"Shadowsocks", "ss://", `{"type":"shadowsocks","tag":"SS","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"p@ss:word/测试"}`},
		{"VMess", "vmess://", `{"type":"vmess","tag":"VMess","server":"example.com","server_port":443,"uuid":"11111111-1111-4111-8111-111111111111","security":"auto","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"VLESS", "vless://", `{"type":"vless","tag":"VLESS","server":"example.com","server_port":443,"uuid":"11111111-1111-4111-8111-111111111111","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"example.com","reality":{"enabled":true,"public_key":"public","short_id":"abcd"}}}`},
		{"Trojan", "trojan://", `{"type":"trojan","tag":"Trojan","server":"example.com","server_port":443,"password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"Hysteria2", "hysteria2://", `{"type":"hysteria2","tag":"Hysteria2","server":"example.com","server_port":443,"password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"SOCKS5", "socks5://", `{"type":"socks","tag":"SOCKS5","server":"example.com","server_port":1080,"username":"user@name","password":"p@ss:word/测试"}`},
		{"HTTP", "https://", `{"type":"http","tag":"HTTP","server":"example.com","server_port":443,"username":"user@name","password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"TUIC", "tuic://", `{"type":"tuic","tag":"TUIC","server":"example.com","server_port":443,"uuid":"11111111-1111-4111-8111-111111111111","password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"AnyTLS", "anytls://", `{"type":"anytls","tag":"AnyTLS","server":"example.com","server_port":443,"password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
		{"WireGuard", "wireguard://", `{"type":"wireguard","tag":"WireGuard","server":"example.com","server_port":51820,"private_key":"private+/key=","peer_public_key":"public+/key=","local_address":["10.0.0.2/32"]}`},
		{"Naive", "naive+https://", `{"type":"naive","tag":"Naive","server":"example.com","server_port":443,"username":"user@name","password":"p@ss:word/测试","tls":{"enabled":true,"server_name":"example.com"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := parseJSONOutbound(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			link, err := nodeShareLink(node)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(strings.TrimSpace(link), "{") || !strings.HasPrefix(link, test.prefix) {
				t.Fatalf("分享链接格式错误：%s", link)
			}
			nodes, errs := ParseNodes(link)
			if len(nodes) != 1 || len(errs) != 0 {
				t.Fatalf("分享链接无法重新解析：nodes=%d errs=%v link=%s", len(nodes), errs, link)
			}
			got := nodes[0].Outbound
			for _, key := range []string{"server", "server_port", "username", "password", "uuid", "private_key", "peer_public_key"} {
				wantValue := stringFromAny(node.Outbound[key])
				if wantValue != "" && stringFromAny(got[key]) != wantValue {
					t.Fatalf("分享链接字段 %s 未完整保留：want=%q got=%q link=%s", key, wantValue, stringFromAny(got[key]), link)
				}
			}
		})
	}
}
