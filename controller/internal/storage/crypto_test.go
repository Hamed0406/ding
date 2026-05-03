package storage

import (
	"testing"
	"time"
)

const testPass = "test-secret-key"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	enc, err := encryptPassword(testPass, "hunter2")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "hunter2" {
		t.Fatal("encrypted value should not equal plaintext")
	}
	if !hasPrefix(enc, encPrefix) {
		t.Fatalf("missing enc:v1: prefix: %q", enc)
	}

	got, err := decryptPassword(testPass, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("want %q, got %q", "hunter2", got)
	}
}

func TestEncrypt_DifferentCiphertextEachCall(t *testing.T) {
	// Each call should produce a different ciphertext (random salt + nonce).
	a, _ := encryptPassword(testPass, "same")
	b, _ := encryptPassword(testPass, "same")
	if a == b {
		t.Error("two encryptions of the same value should differ (random salt)")
	}
}

func TestDecrypt_LegacyPlaintext(t *testing.T) {
	got, err := decryptPassword(testPass, "my-plain-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-plain-password" {
		t.Errorf("want %q, got %q", "my-plain-password", got)
	}
}

func TestEncrypt_EmptyPassphrase(t *testing.T) {
	got, err := encryptPassword("", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret" {
		t.Errorf("empty passphrase should return plaintext, got %q", got)
	}
}

func TestEncrypt_EmptyPassword(t *testing.T) {
	got, err := encryptPassword(testPass, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty password should stay empty, got %q", got)
	}
}

func TestDecrypt_NoPassphraseForEncrypted(t *testing.T) {
	enc, err := encryptPassword(testPass, "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = decryptPassword("", enc)
	if err == nil {
		t.Fatal("expected error when decrypting without passphrase")
	}
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	_, err := decryptPassword(testPass, encPrefix+"!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for corrupted base64")
	}
}

func TestDecrypt_TooShortPayload(t *testing.T) {
	// "tooshort" is 8 bytes — less than saltSize (16).
	short := encPrefix + "dG9vc2hvcnQ="
	_, err := decryptPassword(testPass, short)
	if err == nil {
		t.Fatal("expected error for payload shorter than salt")
	}
}

func TestSQLite_EmailEncryption(t *testing.T) {
	store, err := NewSQLite(":memory:", testPass)
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

	// Raw DB value must not be plaintext.
	var raw string
	store.db.QueryRow(`SELECT password FROM user_email_config WHERE user_id = ?`, user.ID).Scan(&raw) //nolint:errcheck
	if raw == cfg.Password {
		t.Error("password should be encrypted in DB, found plaintext")
	}

	got, err := store.GetEmailConfig(user.ID)
	if err != nil {
		t.Fatalf("GetEmailConfig: %v", err)
	}
	if got.Password != cfg.Password {
		t.Errorf("password: want %q, got %q", cfg.Password, got.Password)
	}

	all := store.GetAllEmailConfigs()
	if len(all) != 1 || all[0].Password != cfg.Password {
		t.Errorf("GetAllEmailConfigs: unexpected result %v", all)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
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
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.PruneOldData(-time.Second, -time.Second); err != nil {
		t.Fatalf("PruneOldData: %v", err)
	}
	var count int
	store.db.QueryRow(`SELECT COUNT(*) FROM scans`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("want 0 scans after prune, got %d", count)
	}
}
