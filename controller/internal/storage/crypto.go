package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encPrefix = "enc:v1:"

// encryptPassword encrypts plaintext using AES-256-GCM with a key derived
// from secretKey. The result is prefixed with "enc:v1:" so callers can
// distinguish encrypted values from legacy plaintext.
// Returns plaintext unchanged if secretKey is empty.
func encryptPassword(secretKey, plaintext string) (string, error) {
	if secretKey == "" || plaintext == "" {
		return plaintext, nil
	}
	block, err := aes.NewCipher(deriveKey(secretKey))
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
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptPassword reverses encryptPassword. If the value has no "enc:v1:"
// prefix it is returned as-is (legacy plaintext — backwards compatible).
// Returns an error only when the value is prefixed but decryption fails.
func decryptPassword(secretKey, value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil // plaintext (no key was set when it was saved)
	}
	if secretKey == "" {
		// Stored encrypted but no key — return empty rather than gibberish.
		return "", errors.New("DING_SECRET_KEY required to decrypt stored password")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveKey(secretKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// deriveKey produces a 32-byte AES-256 key from an arbitrary passphrase.
func deriveKey(passphrase string) []byte {
	h := sha256.Sum256([]byte(passphrase))
	return h[:]
}
