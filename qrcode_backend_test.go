package main

import (
	"bytes"
	"image/png"
	"sync"
	"testing"
)

func TestQRPNGProducesValidImage(t *testing.T) {
	data, err := qrPNG("socks5://user:pass@example.com:1080#test")
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("二维码 PNG 无法解码：%v", err)
	}
	if image.Bounds().Dx() <= 0 || image.Bounds().Dy() <= 0 {
		t.Fatalf("二维码尺寸无效：%v", image.Bounds())
	}
}

func TestQRPNGConcurrentEncoding(t *testing.T) {
	var wait sync.WaitGroup
	errors := make(chan error, 24)
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := qrPNG("vmess://并发二维码测试")
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
