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

const (
	encPrefix        = "enc:v1:"
	pbkdf2Iterations = 600_000
	saltSize         = 16
)

// encryptPassword encrypts plaintext with AES-256-GCM. A random salt is generated
// per call and embedded in the output so each stored value is independently protected.
// Output format: "enc:v1:" + base64(salt[16] + nonce[12] + gcm-ciphertext).
// Returns plaintext unchanged when passphrase is empty (encryption disabled).
func encryptPassword(passphrase, plaintext string) (string, error) {
	if passphrase == "" || plaintext == "" {
		return plaintext, nil
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
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
	payload := append(salt, sealed...)
	return encPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// decryptPassword reverses encryptPassword. Values without the "enc:v1:" prefix are
// returned unchanged (legacy plaintext — backwards compatible with pre-encryption data).
func decryptPassword(passphrase, value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}
	if passphrase == "" {
		return "", errors.New("DING_SECRET_KEY required to decrypt stored password")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", err
	}
	if len(payload) < saltSize {
		return "", errors.New("ciphertext too short")
	}
	salt, data := payload[:saltSize], payload[saltSize:]
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
