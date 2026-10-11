package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestVerifyLocalProxyConnectivityAcceptsAnyHealthyProbe(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxyServer.Close()

	port := testServerPort(t, proxyServer.URL)
	err := verifyLocalProxyConnectivityURLs(port, time.Second, []string{
		"http://probe.invalid/fail",
		"http://probe.invalid/ok",
	})
	if err != nil {
		t.Fatalf("存在可用验证地址时不应失败：%v", err)
	}
}

func TestVerifyLocalProxyConnectivityRejectsProxyFailures(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxyServer.Close()

	port := testServerPort(t, proxyServer.URL)
	if err := verifyLocalProxyConnectivityURLs(port, time.Second, []string{"http://probe.invalid/fail"}); err == nil {
		t.Fatal("代理返回 5xx 时应判定实际出网失败")
	}
}

func testServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
