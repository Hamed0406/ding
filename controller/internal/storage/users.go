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

// TelegramConfig holds a user's Telegram notification credentials.
type TelegramConfig struct {
	Token  string
	ChatID string
}

// EmailConfig holds a user's SMTP email alert settings.
type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
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
	// SaveTelegramConfig stores the Telegram bot token and chat ID for a user.
	// Pass empty strings to clear the configuration.
	SaveTelegramConfig(userID int64, token, chatID string) error
	// GetTelegramConfig returns the Telegram config for a specific user.
	GetTelegramConfig(userID int64) (TelegramConfig, error)
	// GetAllTelegramConfigs returns the Telegram configs for every user that has
	// configured one. Used by the alert pipeline to notify all opted-in users.
	GetAllTelegramConfigs() []TelegramConfig

	// SaveWebhookURL stores (or clears) a webhook URL for a user.
	SaveWebhookURL(userID int64, webhookURL string) error
	// GetWebhookURL returns the webhook URL for a specific user.
	GetWebhookURL(userID int64) (string, error)
	// GetAllWebhookURLs returns every non-empty webhook URL across all users.
	// Used by the alert pipeline to fire all configured webhooks.
	GetAllWebhookURLs() []string

	// SaveEmailConfig stores or clears the SMTP email config for a user.
	SaveEmailConfig(userID int64, cfg EmailConfig) error
	// GetEmailConfig returns the email config for a specific user.
	GetEmailConfig(userID int64) (EmailConfig, error)
	// GetAllEmailConfigs returns configs for every user that has email alerts configured.
	GetAllEmailConfigs() []EmailConfig
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

// SaveTelegramConfig stores or clears a user's Telegram bot token and chat ID.
func (s *SQLiteStore) SaveTelegramConfig(userID int64, token, chatID string) error {
	_, err := s.db.Exec(
		`UPDATE users SET telegram_token = ?, telegram_chat_id = ? WHERE id = ?`,
		token, chatID, userID,
	)
	return err
}

// GetTelegramConfig returns the Telegram config for the given user.
func (s *SQLiteStore) GetTelegramConfig(userID int64) (TelegramConfig, error) {
	var cfg TelegramConfig
	err := s.db.QueryRow(
		`SELECT telegram_token, telegram_chat_id FROM users WHERE id = ?`, userID,
	).Scan(&cfg.Token, &cfg.ChatID)
	return cfg, err
}

// SaveWebhookURL stores or clears the webhook URL for the given user.
func (s *SQLiteStore) SaveWebhookURL(userID int64, webhookURL string) error {
	_, err := s.db.Exec(
		`UPDATE users SET webhook_url = ? WHERE id = ?`, webhookURL, userID,
	)
	return err
}

// GetWebhookURL returns the webhook URL for the given user, or "" if not set.
func (s *SQLiteStore) GetWebhookURL(userID int64) (string, error) {
	var u string
	err := s.db.QueryRow(`SELECT webhook_url FROM users WHERE id = ?`, userID).Scan(&u)
	return u, err
}

// GetAllWebhookURLs returns all non-empty webhook URLs across all users.
func (s *SQLiteStore) GetAllWebhookURLs() []string {
	rows, err := s.db.Query(`SELECT webhook_url FROM users WHERE webhook_url != ''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			urls = append(urls, u)
		}
	}
	return urls
}

// SaveEmailConfig upserts the SMTP config for the given user.
// Passing an empty Host clears the config (disables email alerts for that user).
// The password is encrypted with AES-256-GCM if DING_SECRET_KEY is configured.
func (s *SQLiteStore) SaveEmailConfig(userID int64, cfg EmailConfig) error {
	enc, err := encryptPassword(s.derivedKey, cfg.Password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO user_email_config (user_id, host, port, username, password, from_addr, to_addr)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			host      = excluded.host,
			port      = excluded.port,
			username  = excluded.username,
			password  = excluded.password,
			from_addr = excluded.from_addr,
			to_addr   = excluded.to_addr
	`, userID, cfg.Host, cfg.Port, cfg.Username, enc, cfg.From, cfg.To)
	return err
}

// GetEmailConfig returns the email config for the given user ID.
// Returns a zero-value config (Host="") if none is stored.
func (s *SQLiteStore) GetEmailConfig(userID int64) (EmailConfig, error) {
	var cfg EmailConfig
	var storedPw string
	err := s.db.QueryRow(`
		SELECT host, port, username, password, from_addr, to_addr
		FROM user_email_config WHERE user_id = ?
	`, userID).Scan(&cfg.Host, &cfg.Port, &cfg.Username, &storedPw, &cfg.From, &cfg.To)
	if err == sql.ErrNoRows {
		return EmailConfig{Port: 587}, nil
	}
	if err != nil {
		return cfg, err
	}
	cfg.Password, err = decryptPassword(s.derivedKey, storedPw)
	return cfg, err
}

// GetAllEmailConfigs returns the email configs for every user that has a host and To address set.
func (s *SQLiteStore) GetAllEmailConfigs() []EmailConfig {
	rows, err := s.db.Query(`
		SELECT host, port, username, password, from_addr, to_addr
		FROM user_email_config
		WHERE host != '' AND to_addr != ''
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cfgs []EmailConfig
	for rows.Next() {
		var cfg EmailConfig
		var storedPw string
		if err := rows.Scan(&cfg.Host, &cfg.Port, &cfg.Username, &storedPw, &cfg.From, &cfg.To); err != nil {
			continue
		}
		cfg.Password, _ = decryptPassword(s.derivedKey, storedPw)
		cfgs = append(cfgs, cfg)
	}
	return cfgs
}

// GetAllTelegramConfigs returns configs for all users who have both a token and chat ID set.
func (s *SQLiteStore) GetAllTelegramConfigs() []TelegramConfig {
	rows, err := s.db.Query(
		`SELECT telegram_token, telegram_chat_id FROM users
		 WHERE telegram_token != '' AND telegram_chat_id != ''`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var configs []TelegramConfig
	for rows.Next() {
		var cfg TelegramConfig
		if err := rows.Scan(&cfg.Token, &cfg.ChatID); err == nil {
			configs = append(configs, cfg)
		}
	}
	return configs
}
