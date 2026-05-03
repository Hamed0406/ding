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

	"golang.org/x/crypto/pbkdf2"
)

const encPrefix = "enc:v1:"

// encryptPassword encrypts plaintext using AES-256-GCM with a pre-derived key.
// Returns plaintext unchanged when key is nil (encryption disabled).
func encryptPassword(key []byte, plaintext string) (string, error) {
	if len(key) == 0 || plaintext == "" {
		return plaintext, nil
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
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptPassword reverses encryptPassword. Values without the "enc:v1:" prefix
// are returned unchanged (legacy plaintext — backwards compatible).
// Returns an error only when the value is prefixed but decryption fails.
func decryptPassword(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil // plaintext (no key was set when it was saved)
	}
	if len(key) == 0 {
		return "", errors.New("DING_SECRET_KEY required to decrypt stored password")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
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

// DeriveKey produces a 32-byte AES-256 key from a passphrase using PBKDF2-SHA256.
// The salt is application-specific and fixed; security comes from the passphrase entropy
// (DING_SECRET_KEY is documented as a 32-byte random value from openssl rand -base64 32).
// 260000 iterations matches the 2023 OWASP PBKDF2-SHA256 recommendation.
// Call once at startup and reuse the returned bytes — do not call per request.
func DeriveKey(passphrase string) []byte {
	return pbkdf2.Key([]byte(passphrase), []byte("ding:v1:email-password-key"), 260000, 32, sha256.New)
}
