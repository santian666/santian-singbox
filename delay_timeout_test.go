package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDelayTimeoutReturnsWithoutBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	originalURLs := delayTestURLs
	delayTestURLs = []string{server.URL}
	defer func() { delayTestURLs = originalURLs }()

	done := make(chan error, 1)
	go func() {
		_, err := testDelay(&http.Client{Timeout: 40 * time.Millisecond}, delayRuntimeOptions{
			timeout:     40 * time.Millisecond,
			sampleCount: 1,
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("延迟测试超时应返回错误")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("延迟测试超时后仍未返回")
	}
}

func TestRequestBestDelayUsesLowerSample(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			time.Sleep(120 * time.Millisecond)
		} else {
			time.Sleep(10 * time.Millisecond)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	delay, err := requestBestDelay(context.Background(), &http.Client{Timeout: time.Second}, server.URL, 2)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("请求次数 = %d，期望 2", requests)
	}
	if delay >= 80 {
		t.Fatalf("未取较小延迟样本：%d ms", delay)
	}
}

func TestRequestBestDelayKeepsTimeForRetry(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			<-r.Context().Done()
			return
		}
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	delay, err := requestBestDelay(ctx, &http.Client{Timeout: time.Second}, server.URL, 2)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("请求次数 = %d，期望 2", requests)
	}
	if delay <= 0 || delay >= 100 {
		t.Fatalf("重试延迟异常：%d ms", delay)
	}
}

func TestBuildLocalSOCKS5ClientRejectsInvalidPort(t *testing.T) {
	client, err := buildLocalSOCKS5Client(0, time.Second)
	if err == nil || client != nil {
		t.Fatalf("无效 SOCKS5 端口不得回退直连：client=%v err=%v", client, err)
	}
}

func TestTCPPingResolvesOnceAndExcludesDNSDelay(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	originalLookup := lookupTCPPingIP
	defer func() { lookupTCPPingIP = originalLookup }()
	tcpPingResolveState.Lock()
	tcpPingResolveState.calls = make(map[string]*tcpPingResolveCall)
	tcpPingResolveState.Unlock()
	var lookupCount int32
	lookupTCPPingIP = func(context.Context, string) ([]net.IPAddr, error) {
		atomic.AddInt32(&lookupCount, 1)
		time.Sleep(120 * time.Millisecond)
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}

	port := listener.Addr().(*net.TCPAddr).Port
	const total = 12
	delays := make(chan int, total)
	errors := make(chan error, total)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			delay, err := testTCPAddress("shared-delay.test", port)
			if err != nil {
				errors <- err
				return
			}
			delays <- delay
		}()
	}
	wg.Wait()
	close(delays)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if count := atomic.LoadInt32(&lookupCount); count != 1 {
		t.Fatalf("同域名并发解析次数 = %d，期望 1", count)
	}
	for delay := range delays {
		if delay >= 100 {
			t.Fatalf("TCP 延迟包含了 DNS 等待时间：%d ms", delay)
		}
	}
}
