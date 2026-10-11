package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

const encryptedPrefix = "GSB1:"

func encryptBytes(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(localKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nonce, nonce, data, nil)
	return []byte(encryptedPrefix + base64.StdEncoding.EncodeToString(sealed)), nil
}

func decryptBytes(data []byte) ([]byte, error) {
	text := string(data)
	if len(text) < len(encryptedPrefix) || text[:len(encryptedPrefix)] != encryptedPrefix {
		return data, nil
	}
	raw, err := base64.StdEncoding.DecodeString(text[len(encryptedPrefix):])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(localKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("加密数据损坏")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func localKey() []byte {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	sum := sha256.Sum256([]byte(hostname + "|" + home + "|go-singbox"))
	return sum[:]
}
