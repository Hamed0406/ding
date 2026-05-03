package storage

import (
	"testing"
	"time"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := "test-secret-key"
	plain := "hunter2"

	enc, err := encryptPassword(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("encrypted value should not equal plaintext")
	}
	if enc[:len(encPrefix)] != encPrefix {
		t.Fatalf("encrypted value missing prefix: %q", enc[:7])
	}

	got, err := decryptPassword(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Errorf("want %q, got %q", plain, got)
	}
}

func TestDecrypt_LegacyPlaintext(t *testing.T) {
	// Values stored before encryption was enabled pass through unchanged.
	got, err := decryptPassword("any-key", "my-plain-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-plain-password" {
		t.Errorf("want %q, got %q", "my-plain-password", got)
	}
}

func TestEncrypt_EmptyKey(t *testing.T) {
	// No key → returns plaintext unchanged (encryption disabled).
	got, err := encryptPassword("", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret" {
		t.Errorf("want %q, got %q", "secret", got)
	}
}

func TestEncrypt_EmptyPassword(t *testing.T) {
	got, err := encryptPassword("key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty password should stay empty, got %q", got)
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
