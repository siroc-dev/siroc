package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const passwordChars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%&*"

func RandomPassword(n int) (string, error) {
	if n < 8 {
		n = 16
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = passwordChars[int(b)%len(passwordChars)]
	}
	return string(out), nil
}

func NewKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Encrypt(keyHex, plaintext string) (string, error) {
	key, err := parseKey(keyHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ct), nil
}

func Decrypt(keyHex, ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", fmt.Errorf("password is not stored")
	}
	key, err := parseKey(keyHex)
	if err != nil {
		return "", err
	}
	raw, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("corrupt password")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("corrupt password")
	}
	pt, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt password")
	}
	return string(pt), nil
}

func parseKey(keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid secret key")
	}
	return key, nil
}
