//go:build !windows

package main

import "errors"

func dpapiProtect(plaintext []byte) ([]byte, error) {
	return nil, errors.New("DPAPI unavailable on non-Windows")
}

func dpapiUnprotect(ciphertext []byte) ([]byte, error) {
	return nil, errors.New("DPAPI unavailable on non-Windows")
}
