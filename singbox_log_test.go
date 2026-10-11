package main

import "testing"

func TestFormatSingboxLogLineKeepsOriginalOrder(t *testing.T) {
	input := "+0800 2026-06-05 10:50:46 INFO [4071732497 0ms] inbound/tun[tun-in]: inbound packet connection to 198.18.0.5:443"
	want := input
	if got := formatSingboxLogLine(input); got != want {
		t.Fatalf("formatSingboxLogLine() = %q, want %q", got, want)
	}
}

func TestFormatSingboxLogLineStripsAnsiAndKeepsOriginalOrder(t *testing.T) {
	input := "+0800 2026-06-05 10:50:46 \x1b[36mINFO\x1b[0m [\x1b[38;5;44m4071732497\x1b[0m 0ms] inbound/tun[tun-in]: inbound packet connection to 198.18.0.5:443"
	want := "+0800 2026-06-05 10:50:46 INFO [4071732497 0ms] inbound/tun[tun-in]: inbound packet connection to 198.18.0.5:443"
	if got := formatSingboxLogLine(input); got != want {
		t.Fatalf("formatSingboxLogLine() = %q, want %q", got, want)
	}
}

func TestFormatSingboxLogLineKeepsUnmatchedLine(t *testing.T) {
	input := "remote host."
	if got := formatSingboxLogLine(input); got != input {
		t.Fatalf("formatSingboxLogLine() = %q, want %q", got, input)
	}
}

func TestFormatSingboxLogLineHandlesPrefixOnly(t *testing.T) {
	input := "+0800 2026-06-05 10:50:46 ERROR [4071732497 0ms]"
	if got := formatSingboxLogLine(input); got != input {
		t.Fatalf("formatSingboxLogLine() = %q, want %q", got, input)
	}
}

func TestSingboxBlockLogIsClassifiedForDedupe(t *testing.T) {
	input := "+0800 2026-06-07 14:27:21 ERROR [802063955 0ms] connection: open connection to ads.tiktok.com:443 using outbound/block[block]: operation not permitted"
	if !isSingboxBlockLog(input) {
		t.Fatalf("isSingboxBlockLog() = false, want true")
	}
	if got := extractBlockTarget(input); got != "ads.tiktok.com:443" {
		t.Fatalf("extractBlockTarget() = %q, want ads.tiktok.com:443", got)
	}
}

func TestSingboxRemoteNodeOpenConnectionLogIsNotClassifiedAsLocalError(t *testing.T) {
	input := "+0800 2026-06-07 14:27:29 ERROR [3642491709 5.18s] connection: connection download closed: remote: open connection to 112.65.193.155:80 using outbound/direct[SG-1-out]: dial tcp 112.65.193.155:80: i/o timeout"
	if isSingboxErrorLog(input) {
		t.Fatalf("isSingboxErrorLog() = true, want false")
	}
	if isSingboxBlockLog(input) {
		t.Fatalf("isSingboxBlockLog() = true, want false")
	}
	if !isSingboxRemoteNodeOpenConnectionError(input) {
		t.Fatalf("isSingboxRemoteNodeOpenConnectionError() = false, want true")
	}
}

func TestSingboxFatalLogStillClassifiedAsError(t *testing.T) {
	input := "+0800 2026-06-07 14:27:29 FATAL start service: default DNS server not found"
	if !isSingboxErrorLog(input) {
		t.Fatalf("isSingboxErrorLog() = false, want true")
	}
}

func TestSingboxLocalClientCloseLogIsNotClassifiedAsError(t *testing.T) {
	input := "+0800 2026-06-08 00:45:20 ERROR [2606073405 143ms] connection: connection upload closed: raw-read tcp 127.0.0.1:20808->127.0.0.1:37997: An existing connection was forcibly closed by the remote host."
	if isSingboxErrorLog(input) {
		t.Fatalf("isSingboxErrorLog() = true, want false")
	}
}

func TestSingboxLocalClientCloseLogWithUnicodeArrowIsNotClassifiedAsError(t *testing.T) {
	input := "+0800 2026-06-08 00:58:25 ERROR [1702495385 138ms] connection: connection upload closed: raw-read tcp 127.0.0.1:20808→127.0.0.1:39020: An existing connection was forcibly closed by the remote host."
	if isSingboxErrorLog(input) {
		t.Fatalf("isSingboxErrorLog() = true, want false")
	}
}

func TestSingboxTunLocalClientCloseLogIsNotClassifiedAsError(t *testing.T) {
	input := "+0800 2026-07-29 21:13:47 ERROR [2166825236 1.96s] connection: connection upload closed: raw-read tcp4 172.19.0.1:29482->172.19.0.2:10196: An existing connection was forcibly closed by the remote host."
	if !isSingboxLocalClientCloseLog(input) {
		t.Fatal("TUN 本机连接重置应识别为本地应用主动关闭")
	}
	if isSingboxErrorLog(input) {
		t.Fatal("TUN 本机连接重置不应显示为严重错误")
	}
}

func TestSingboxExternalClientCloseLogRemainsError(t *testing.T) {
	input := "+0800 2026-07-29 21:13:47 ERROR [2166825236 1.96s] connection: connection upload closed: raw-read tcp4 203.0.113.10:29482->198.51.100.20:10196: An existing connection was forcibly closed by the remote host."
	if isSingboxLocalClientCloseLog(input) {
		t.Fatal("外部地址连接重置不应误判为本机连接回收")
	}
	if !isSingboxErrorLog(input) {
		t.Fatal("外部地址连接重置仍应保留错误级别")
	}
}

func TestSingboxInboundLocalClientAbortedLogIsNotClassifiedAsError(t *testing.T) {
	input := "+0800 2026-09-21 19:54:06 ERROR [3726099787 311ms] inbound/mixed[mixed-in]: process connection from 127.0.0.1:13237: write tcp 127.0.0.1:20808->127.0.0.1:13237: wsasend: An established connection was aborted by the software in your host machine."
	if !isSingboxLocalClientCloseLog(input) {
		t.Fatal("入站回环连接本地主动中止应识别为本地应用主动关闭")
	}
	if isSingboxErrorLog(input) {
		t.Fatal("入站回环连接本地主动中止不应显示为严重错误")
	}
}

