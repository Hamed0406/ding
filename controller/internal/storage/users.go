package storage

import (
	"database/sql"
	"time"
)

// User is a registered account in the users table.
type User struct {
	ID           int64
	Email        string
	PasswordHash string // bcrypt hash; empty for OAuth-only users
	CreatedAt    time.Time
}

// UserStore is the persistence interface for user accounts.
// *SQLiteStore implements this alongside Store.
type UserStore interface {
	// CreateUser inserts a new user. passwordHash should be a bcrypt hash, or
	// empty string for OAuth-only accounts.
	CreateUser(email, passwordHash string) (*User, error)
	// FindUserByEmail returns the user with that email, or nil if not found.
	FindUserByEmail(email string) (*User, error)
	// FindUserByProvider returns the user linked to the given OAuth provider + ID,
	// or nil if not found.
	FindUserByProvider(provider, providerID string) (*User, error)
	// LinkProvider associates an OAuth provider identity with an existing user.
	LinkProvider(userID int64, provider, providerID string) error
	// UserCount returns the total number of registered users.
	UserCount() (int, error)
}

// CreateUser inserts a new user row and returns the created user.
func (s *SQLiteStore) CreateUser(email, passwordHash string) (*User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO users (email, password_hash, created_at) VALUES (?, ?, ?)`,
		email, passwordHash, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Email: email, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}, nil
}

// FindUserByEmail returns the user matching email, or nil if none exists.
func (s *SQLiteStore) FindUserByEmail(email string) (*User, error) {
	var u User
	var createdAtStr string
	err := s.db.QueryRow(
		`SELECT id, email, password_hash, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &u, nil
}

// FindUserByProvider returns the user linked to the given OAuth provider + provider user ID.
func (s *SQLiteStore) FindUserByProvider(provider, providerID string) (*User, error) {
	var u User
	var createdAtStr string
	err := s.db.QueryRow(`
		SELECT u.id, u.email, u.password_hash, u.created_at
		FROM users u
		JOIN user_providers p ON p.user_id = u.id
		WHERE p.provider = ? AND p.provider_id = ?
	`, provider, providerID).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &u, nil
}

// LinkProvider associates an OAuth provider identity with an existing user account.
func (s *SQLiteStore) LinkProvider(userID int64, provider, providerID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO user_providers (user_id, provider, provider_id) VALUES (?, ?, ?)`,
		userID, provider, providerID,
	)
	return err
}

// UserCount returns the total number of registered users.
func (s *SQLiteStore) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
