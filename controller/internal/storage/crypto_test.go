package storage

import (
	"testing"
	"time"
)

// testKey derives a key once for all crypto tests (avoids repeated PBKDF2 calls).
var testKey = DeriveKey("test-secret-key")

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plain := "hunter2"

	enc, err := encryptPassword(testKey, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("encrypted value should not equal plaintext")
	}
	if enc[:len(encPrefix)] != encPrefix {
		t.Fatalf("encrypted value missing prefix: %q", enc[:7])
	}

	got, err := decryptPassword(testKey, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Errorf("want %q, got %q", plain, got)
	}
}

func TestDecrypt_LegacyPlaintext(t *testing.T) {
	// Values stored before encryption was enabled pass through unchanged.
	got, err := decryptPassword(testKey, "my-plain-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-plain-password" {
		t.Errorf("want %q, got %q", "my-plain-password", got)
	}
}

func TestEncrypt_NilKey(t *testing.T) {
	// No key → returns plaintext unchanged (encryption disabled).
	got, err := encryptPassword(nil, "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret" {
		t.Errorf("want %q, got %q", "secret", got)
	}
}

func TestEncrypt_EmptyPassword(t *testing.T) {
	got, err := encryptPassword(testKey, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty password should stay empty, got %q", got)
	}
}

func TestDecrypt_NoKeyForEncrypted(t *testing.T) {
	// Encrypted value but no key → error, not gibberish.
	enc, err := encryptPassword(testKey, "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = decryptPassword(nil, enc)
	if err == nil {
		t.Fatal("expected error when decrypting without key")
	}
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	_, err := decryptPassword(testKey, encPrefix+"!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}

func TestDecrypt_TooShortCiphertext(t *testing.T) {
	// "tooshort" is 8 bytes — less than the 12-byte AES-GCM nonce.
	short := encPrefix + "dG9vc2hvcnQ="
	_, err := decryptPassword(testKey, short)
	if err == nil {
		t.Fatal("expected error for ciphertext shorter than nonce")
	}
}

func TestSQLite_EmailEncryption(t *testing.T) {
	store, err := NewSQLite(":memory:", "test-key-abc")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}

	user := mustCreateUser(t, store)

	cfg := EmailConfig{
		Host: "smtp.example.com", Port: 587,
		Username: "user", Password: "s3cret",
		From: "from@example.com", To: "to@example.com",
	}
	if err := store.SaveEmailConfig(user.ID, cfg); err != nil {
		t.Fatalf("SaveEmailConfig: %v", err)
	}

	// Verify the raw DB value is NOT the plaintext password.
	var raw string
	store.db.QueryRow(`SELECT password FROM user_email_config WHERE user_id = ?`, user.ID).Scan(&raw) //nolint:errcheck
	if raw == cfg.Password {
		t.Error("password should be encrypted in DB, found plaintext")
	}

	// Verify GetEmailConfig returns the decrypted value.
	got, err := store.GetEmailConfig(user.ID)
	if err != nil {
		t.Fatalf("GetEmailConfig: %v", err)
	}
	if got.Password != cfg.Password {
		t.Errorf("password: want %q, got %q", cfg.Password, got.Password)
	}

	// Verify GetAllEmailConfigs also decrypts.
	all := store.GetAllEmailConfigs()
	if len(all) != 1 || all[0].Password != cfg.Password {
		t.Errorf("GetAllEmailConfigs: want password %q, got %v", cfg.Password, all)
	}
}

func mustCreateUser(t *testing.T, s *SQLiteStore) *User {
	t.Helper()
	u, err := s.CreateUser("test@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func TestSQLite_PruneOldData(t *testing.T) {
	store, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}

	// Save two scan records.
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Prune with a negative age so the cutoff is 1 s in the future — removes everything.
	if err := store.PruneOldData(-time.Second, -time.Second); err != nil {
		t.Fatalf("PruneOldData: %v", err)
	}

	var count int
	store.db.QueryRow(`SELECT COUNT(*) FROM scans`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("want 0 scans after prune, got %d", count)
	}
}
