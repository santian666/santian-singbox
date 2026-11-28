//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32DLL             = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData   = crypt32DLL.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
)

type dpapiDataBlob struct {
	cbData uint32
	pbData *byte
}

func dpapiProtect(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return []byte{}, nil
	}
	inBlob := dpapiDataBlob{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}
	var outBlob dpapiDataBlob
	// CRYPTPROTECT_UI_FORBIDDEN = 0x1
	r1, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		uintptr(0x1),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	runtime.KeepAlive(plaintext)
	if r1 == 0 {
		return nil, fmt.Errorf("DPAPI 加密失败: %w", callErr)
	}
	if outBlob.pbData == nil {
		return nil, errors.New("DPAPI 返回空密文")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.pbData)))
	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

func dpapiUnprotect(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return []byte{}, nil
	}
	inBlob := dpapiDataBlob{
		cbData: uint32(len(ciphertext)),
		pbData: &ciphertext[0],
	}
	var outBlob dpapiDataBlob
	r1, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		uintptr(0x1),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	runtime.KeepAlive(ciphertext)
	if r1 == 0 {
		return nil, fmt.Errorf("DPAPI 解密失败: %w", callErr)
	}
	if outBlob.pbData == nil {
		return []byte{}, nil
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.pbData)))
	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}
