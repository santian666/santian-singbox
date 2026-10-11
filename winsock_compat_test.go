package main

import "testing"

func TestIncompatibleWinsockProviderDetectsAstrill(t *testing.T) {
	for _, output := range []string{
		`Provider Path: C:\Windows\System32\ASProxy64.dll`,
		`Catalog Entry: ASProxy Layered Service Provider`,
		`ASProxy over TCP/IPv4`,
	} {
		if got := incompatibleWinsockProvider(output); got == "" {
			t.Fatalf("未识别 Astrill Winsock LSP：%q", output)
		}
	}
}

func TestIncompatibleWinsockProviderIgnoresNormalCatalog(t *testing.T) {
	if got := incompatibleWinsockProvider("MSAFD Tcpip [TCP/IPv4]"); got != "" {
		t.Fatalf("正常 Winsock Provider 被误报：%q", got)
	}
}
