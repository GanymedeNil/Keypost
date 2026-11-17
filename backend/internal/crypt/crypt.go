// Package crypt provides AES-256-GCM encryption for secrets stored in the
// database, HMAC signing helpers for URLs / tokens, and random generators.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const encPrefix = "enc:v1:"

var (
	mu  sync.RWMutex
	key [32]byte
	set bool
)

// SetSecret derives the process-wide key from the master secret.
func SetSecret(secret string) {
	mu.Lock()
	defer mu.Unlock()
	key = sha256.Sum256([]byte("df/enc/" + secret))
	set = true
	macKey = sha256.Sum256([]byte("df/mac/" + secret))
}

var macKey [32]byte

func Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	mu.RLock()
	k, ok := key, set
	mu.RUnlock()
	if !ok {
		return "", errors.New("crypt: secret not initialised")
	}
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Decrypt returns the plaintext. Values without the encryption prefix are
// returned untouched so plain values written by older versions keep working.
func Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, encPrefix) {
		return s, nil
	}
	mu.RLock()
	k, ok := key, set
	mu.RUnlock()
	if !ok {
		return "", errors.New("crypt: secret not initialised")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, encPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("crypt: ciphertext too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("crypt: decrypt failed (APP_SECRET changed?): %w", err)
	}
	return string(plain), nil
}

// MAC returns hex(HMAC-SHA256(macKey, parts joined by "\n")).
func MAC(parts ...string) string {
	mu.RLock()
	k := macKey
	mu.RUnlock()
	h := hmac.New(sha256.New, k[:])
	h.Write([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h.Sum(nil))
}

func MACEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// HMACHex signs data with an arbitrary secret (used for outgoing webhooks).
func HMACHex(secret string, data []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RandomToken returns n random bytes encoded as URL-safe base64 (no padding).
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
