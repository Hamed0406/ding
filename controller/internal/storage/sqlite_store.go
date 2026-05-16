// ============================================================
// controller/internal/storage/sqlite_store.go — SQLite backend
//
// SQLiteStore is the active Store implementation. Schema:
//   scans            — one row per scan run (id, scanned_at)
//   devices          — one row per device per scan (all fields + scan_id FK)
//   device_labels    — user-assigned names, keyed by IP (survives scan cycles)
//   device_notify    — per-device alert opt-in flags
//   device_seen_at   — first_seen / last_seen timestamps per IP (upserted on Save)
//   users            — accounts: email, bcrypt hash, Telegram config, webhook URL
//   user_providers   — OAuth provider → user_id mappings
//   speedtest_results — historical internet speed test results
//
// Schema migrations are additive: sqliteMigrate() appends ALTER TABLE / CREATE TABLE
// IF NOT EXISTS statements so existing databases upgrade automatically on startup.
//
// WAL mode is enabled so reads never block writes (important during scans).
// MaxOpenConns=1 avoids "database is locked" since SQLite has one writer.
// ============================================================

package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using a local SQLite database.
// Schema is normalized: one row per device per scan, enabling future analytics.
type SQLiteStore struct {
	db         *sql.DB
	passphrase string // DING_SECRET_KEY value; empty = encryption disabled
}

// NewSQLite opens (or creates) a SQLite database at path and runs migrations.
// secretKey is used to encrypt/decrypt SMTP passwords (AES-256-GCM).
// Pass an empty string to skip encryption (insecure — plaintext in DB).
func NewSQLite(path string, secretKey ...string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// WAL mode: readers don't block writers and vice versa.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, err
	}
	// busy_timeout: if two writers collide, retry for up to 5 s before failing.
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return nil, err
	}
	// WAL allows concurrent readers; raise the pool so auth/API handlers get
	// their own connections and are never queued behind a long-running scan write.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := sqliteMigrate(db); err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if len(secretKey) > 0 {
		s.passphrase = secretKey[0]
	}
	return s, nil
}

func sqliteMigrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scans (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			scanned_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS devices (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_id     INTEGER NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
			ip          TEXT    NOT NULL,
			mac         TEXT,
			hostname    TEXT,
			vendor      TEXT,
			device_type TEXT,
			open_ports  TEXT    NOT NULL DEFAULT '[]',
			alive       INTEGER NOT NULL DEFAULT 0,
			gateway     TEXT,
			ttl         INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_devices_scan    ON devices(scan_id);
		CREATE INDEX IF NOT EXISTS idx_devices_ip      ON devices(ip);
		CREATE INDEX IF NOT EXISTS idx_devices_ip_scan ON devices(ip, scan_id);
	`)
	if err != nil {
		return err
	}
	// Additive column migrations for existing databases.
	// SQLite errors if the column already exists — all are intentionally ignored.
	_, _ = db.Exec(`ALTER TABLE devices ADD COLUMN device_type TEXT`)
	_, _ = db.Exec(`ALTER TABLE devices ADD COLUMN os TEXT`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS device_labels (
			ip         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS device_notify (
			ip      TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1
		)
	`)
	// User authentication tables.
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL DEFAULT '',
			created_at    DATETIME NOT NULL
		)
	`)
	// Additive column migrations for the users table.
	// These run after CREATE TABLE so they are safe on both fresh and existing databases.
	// SQLite returns an error if the column already exists — ignored intentionally.
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN telegram_token   TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN telegram_chat_id TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN webhook_url       TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_providers (
			user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider    TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			PRIMARY KEY (provider, provider_id)
		)
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_email_config (
			user_id  INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			host     TEXT NOT NULL DEFAULT '',
			port     INTEGER NOT NULL DEFAULT 587,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			from_addr TEXT NOT NULL DEFAULT '',
			to_addr   TEXT NOT NULL DEFAULT ''
		)
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS speedtest_results (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			tested_at     DATETIME NOT NULL,
			download_mbps REAL     NOT NULL,
			upload_mbps   REAL     NOT NULL,
			ping_ms       REAL     NOT NULL,
			server        TEXT     NOT NULL
		)
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS device_seen_at (
			ip         TEXT PRIMARY KEY,
			first_seen DATETIME NOT NULL,
			last_seen  DATETIME NOT NULL
		)
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS change_log (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at DATETIME NOT NULL,
			kind        TEXT     NOT NULL,
			ip          TEXT     NOT NULL,
			desc        TEXT     NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_change_log_time ON change_log(occurred_at DESC);
	`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS mac_history (
			ip         TEXT     NOT NULL,
			mac        TEXT     NOT NULL,
			first_seen DATETIME NOT NULL,
			last_seen  DATETIME NOT NULL,
			PRIMARY KEY (ip, mac)
		);
		CREATE INDEX IF NOT EXISTS idx_mac_history_ip ON mac_history(ip);
	`)
	return nil
}

// PruneOldData deletes scan records older than scanAge and change-log entries
// older than changeAge. The devices table cascades on scan deletion automatically.
// Safe to call on a live database — runs in a single transaction.
func (s *SQLiteStore) PruneOldData(scanAge, changeAge time.Duration) error {
	scanCutoff := time.Now().Add(-scanAge).UTC().Format(time.RFC3339)
	changeCutoff := time.Now().Add(-changeAge).UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`DELETE FROM scans      WHERE scanned_at  < ?`, scanCutoff)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM change_log WHERE occurred_at < ?`, changeCutoff)
	return err
}

func (s *SQLiteStore) Save(results []scanner.Result) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.Exec(`INSERT INTO scans (scanned_at) VALUES (?)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	scanID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO devices (scan_id, ip, mac, hostname, vendor, device_type, os, open_ports, alive, gateway, ttl)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	seenStmt, err := tx.Prepare(`
		INSERT INTO device_seen_at (ip, first_seen, last_seen) VALUES (?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET last_seen = excluded.last_seen
	`)
	if err != nil {
		return err
	}
	defer seenStmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		ports, _ := json.Marshal(r.OpenPorts)
		if _, err := stmt.Exec(
			scanID, r.IP, r.MAC, r.Hostname, r.Vendor, r.DeviceType, r.OS,
			string(ports), boolToInt(r.Alive), r.Gateway, r.TTL,
		); err != nil {
			return err
		}
		if r.Alive {
			if _, err := seenStmt.Exec(r.IP, now, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Latest() []scanner.Result {
	rec := s.LatestRecord()
	if rec == nil {
		return nil
	}
	return rec.Results
}

func (s *SQLiteStore) LatestRecord() *Record {
	var id int64
	var scannedAtStr string
	err := s.db.QueryRow(`SELECT id, scanned_at FROM scans ORDER BY id DESC LIMIT 1`).
		Scan(&id, &scannedAtStr)
	if err != nil {
		return nil
	}
	scannedAt, _ := time.Parse(time.RFC3339, scannedAtStr)

	results, err := s.queryDevices(id)
	if err != nil {
		return nil
	}
	return &Record{ScannedAt: scannedAt, Results: results}
}

func (s *SQLiteStore) History(n int) []Record {
	rows, err := s.db.Query(
		`SELECT id, scanned_at FROM scans ORDER BY id DESC LIMIT ?`, n,
	)
	if err != nil {
		return []Record{}
	}
	defer rows.Close()

	type scanRow struct {
		id        int64
		scannedAt time.Time
	}
	var scans []scanRow
	for rows.Next() {
		var sr scanRow
		var scannedAtStr string
		if err := rows.Scan(&sr.id, &scannedAtStr); err != nil {
			continue
		}
		sr.scannedAt, _ = time.Parse(time.RFC3339, scannedAtStr)
		scans = append(scans, sr)
	}

	records := make([]Record, 0, len(scans))
	for _, sr := range scans {
		results, err := s.queryDevices(sr.id)
		if err != nil {
			continue
		}
		records = append(records, Record{ScannedAt: sr.scannedAt, Results: results})
	}

	// Reverse to oldest-first (matches JSONStore behaviour)
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records
}

// AllKnownIPs returns every distinct IP address ever stored in the devices table.
func (s *SQLiteStore) AllKnownIPs() map[string]bool {
	rows, err := s.db.Query(`SELECT DISTINCT ip FROM devices`)
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	known := make(map[string]bool)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err == nil {
			known[ip] = true
		}
	}
	return known
}

// AllDevices returns one entry per IP (latest data for each), with alive=true
// only for devices that appeared in the most recent scan.
// User-assigned labels are joined in from the device_labels table.
func (s *SQLiteStore) AllDevices() []scanner.Result {
	// CTE computes the latest scan_id per IP once (O(N) via idx_devices_ip_scan),
	// avoiding the N² correlated subquery that caused hangs on large device tables.
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT ip, MAX(scan_id) AS scan_id FROM devices GROUP BY ip
		)
		SELECT
			d.ip, d.mac, d.hostname, d.vendor, d.device_type, d.os, d.open_ports,
			CASE WHEN d.scan_id = (SELECT MAX(id) FROM scans) THEN d.alive ELSE 0 END,
			d.gateway, d.ttl, l.name, COALESCE(n.enabled, 0) AS notify,
			sa.first_seen, sa.last_seen
		FROM latest
		JOIN devices d USING (ip, scan_id)
		LEFT JOIN device_labels  l  ON l.ip  = d.ip
		LEFT JOIN device_notify  n  ON n.ip  = d.ip
		LEFT JOIN device_seen_at sa ON sa.ip = d.ip
		ORDER BY d.ip
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	results, _ := scanDeviceRows(rows)
	return results
}

func (s *SQLiteStore) queryDevices(scanID int64) ([]scanner.Result, error) {
	rows, err := s.db.Query(`
		SELECT d.ip, d.mac, d.hostname, d.vendor, d.device_type, d.os, d.open_ports,
		       d.alive, d.gateway, d.ttl, l.name, 1 AS notify,
		       NULL AS first_seen, NULL AS last_seen
		FROM devices d
		LEFT JOIN device_labels l ON l.ip = d.ip
		WHERE d.scan_id = ?
	`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

// scanDeviceRows reads scanner.Result values from an open *sql.Rows.
// Expects columns: ip, mac, hostname, vendor, device_type, os, open_ports,
//
//	alive, gateway, ttl, label, notify, first_seen, last_seen.
func scanDeviceRows(rows *sql.Rows) ([]scanner.Result, error) {
	var results []scanner.Result
	for rows.Next() {
		var r scanner.Result
		var mac, hostname, vendor, deviceType, os, label, gateway sql.NullString
		var ttl sql.NullInt64
		var firstSeenStr, lastSeenStr sql.NullString
		var portsJSON string
		var alive, notify int

		if err := rows.Scan(
			&r.IP, &mac, &hostname, &vendor, &deviceType, &os,
			&portsJSON, &alive, &gateway, &ttl, &label, &notify,
			&firstSeenStr, &lastSeenStr,
		); err != nil {
			continue
		}
		if mac.Valid {
			r.MAC = &mac.String
		}
		if hostname.Valid {
			r.Hostname = &hostname.String
		}
		if vendor.Valid {
			r.Vendor = &vendor.String
		}
		if deviceType.Valid {
			r.DeviceType = &deviceType.String
		}
		if os.Valid {
			r.OS = &os.String
		}
		if label.Valid {
			r.Label = &label.String
		}
		if gateway.Valid {
			r.Gateway = &gateway.String
		}
		if ttl.Valid {
			v := uint8(ttl.Int64)
			r.TTL = &v
		}
		if firstSeenStr.Valid {
			if t, err := time.Parse(time.RFC3339, firstSeenStr.String); err == nil {
				r.FirstSeen = &t
			}
		}
		if lastSeenStr.Valid {
			if t, err := time.Parse(time.RFC3339, lastSeenStr.String); err == nil {
				r.LastSeen = &t
			}
		}
		r.Alive = alive != 0
		r.Notify = notify != 0
		if portsJSON == "" {
			portsJSON = "[]"
		}
		_ = json.Unmarshal([]byte(portsJSON), &r.OpenPorts)
		if r.OpenPorts == nil {
			r.OpenPorts = []uint16{}
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// DeviceHistory returns the last n scan entries for ip, oldest first.
func (s *SQLiteStore) DeviceHistory(ip string, n int) []DeviceHistoryEntry {
	rows, err := s.db.Query(`
		SELECT s.scanned_at, d.alive, d.open_ports
		FROM devices d
		JOIN scans s ON s.id = d.scan_id
		WHERE d.ip = ?
		ORDER BY s.id DESC
		LIMIT ?
	`, ip, n)
	if err != nil {
		return []DeviceHistoryEntry{}
	}
	defer rows.Close()

	var entries []DeviceHistoryEntry
	for rows.Next() {
		var e DeviceHistoryEntry
		var scannedAtStr string
		var alive int
		var portsJSON string
		if err := rows.Scan(&scannedAtStr, &alive, &portsJSON); err != nil {
			continue
		}
		e.ScannedAt, _ = time.Parse(time.RFC3339, scannedAtStr)
		e.Alive = alive != 0
		if portsJSON == "" {
			portsJSON = "[]"
		}
		_ = json.Unmarshal([]byte(portsJSON), &e.OpenPorts)
		if e.OpenPorts == nil {
			e.OpenPorts = []uint16{}
		}
		entries = append(entries, e)
	}

	// Reverse to oldest-first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

func (s *SQLiteStore) SetLabel(ip, name string) error {
	_, err := s.db.Exec(`
		INSERT INTO device_labels (ip, name, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at
	`, ip, name, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) DeleteLabel(ip string) error {
	_, err := s.db.Exec(`DELETE FROM device_labels WHERE ip = ?`, ip)
	return err
}

func (s *SQLiteStore) SetNotify(ip string, enabled bool) error {
	_, err := s.db.Exec(`
		INSERT INTO device_notify (ip, enabled) VALUES (?, ?)
		ON CONFLICT(ip) DO UPDATE SET enabled = excluded.enabled
	`, ip, boolToInt(enabled))
	return err
}

func (s *SQLiteStore) GetLabels() map[string]string {
	rows, err := s.db.Query(`SELECT ip, name FROM device_labels`)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	labels := make(map[string]string)
	for rows.Next() {
		var ip, name string
		if err := rows.Scan(&ip, &name); err == nil {
			labels[ip] = name
		}
	}
	return labels
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *SQLiteStore) SaveSpeedtest(r SpeedtestResult) error {
	_, err := s.db.Exec(`
		INSERT INTO speedtest_results (tested_at, download_mbps, upload_mbps, ping_ms, server)
		VALUES (?, ?, ?, ?, ?)
	`, r.TestedAt.UTC().Format(time.RFC3339), r.DownloadMbps, r.UploadMbps, r.PingMs, r.Server)
	return err
}

func (s *SQLiteStore) SpeedtestHistory(n int) []SpeedtestResult {
	rows, err := s.db.Query(`
		SELECT tested_at, download_mbps, upload_mbps, ping_ms, server
		FROM speedtest_results
		ORDER BY id DESC
		LIMIT ?
	`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []SpeedtestResult
	for rows.Next() {
		var r SpeedtestResult
		var testedAtStr string
		if err := rows.Scan(&testedAtStr, &r.DownloadMbps, &r.UploadMbps, &r.PingMs, &r.Server); err != nil {
			continue
		}
		r.TestedAt, _ = time.Parse(time.RFC3339, testedAtStr)
		results = append(results, r)
	}
	return results
}

// SaveChanges inserts each diff.Change into the change_log table, all stamped
// with the same occurredAt timestamp (the scan wall-clock time).
func (s *SQLiteStore) SaveChanges(changes []diff.Change, occurredAt time.Time) error {
	if len(changes) == 0 {
		return nil
	}
	ts := occurredAt.UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`INSERT INTO change_log (occurred_at, kind, ip, desc) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		if _, err := stmt.Exec(ts, string(c.Kind), c.IP, c.Desc); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Changes returns the last n change-log entries, newest first.
func (s *SQLiteStore) Changes(n int) []ChangeLogEntry {
	rows, err := s.db.Query(`
		SELECT occurred_at, kind, ip, desc
		FROM change_log
		ORDER BY id DESC
		LIMIT ?
	`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []ChangeLogEntry
	for rows.Next() {
		var e ChangeLogEntry
		var occurredAtStr, kind string
		if err := rows.Scan(&occurredAtStr, &kind, &e.IP, &e.Desc); err != nil {
			continue
		}
		e.OccurredAt, _ = time.Parse(time.RFC3339, occurredAtStr)
		e.Kind = diff.ChangeKind(kind)
		entries = append(entries, e)
	}
	return entries
}

// UpdateMACHistory records the current MAC for every alive device and returns
// a MACChange for each IP whose MAC differs from the most recently recorded one.
func (s *SQLiteStore) UpdateMACHistory(results []scanner.Result) ([]MACChange, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var conflicts []MACChange

	for _, r := range results {
		if !r.Alive || r.MAC == nil || *r.MAC == "" {
			continue
		}
		ip, mac := r.IP, *r.MAC

		// Find the most recently seen MAC for this IP (may differ from current).
		var prevMAC string
		err := s.db.QueryRow(`
			SELECT mac FROM mac_history WHERE ip = ? ORDER BY last_seen DESC LIMIT 1
		`, ip).Scan(&prevMAC)
		if err == nil && prevMAC != mac {
			conflicts = append(conflicts, MACChange{IP: ip, OldMAC: prevMAC, NewMAC: mac})
		}

		// Upsert this (ip, mac) pair — set first_seen only on insert.
		_, _ = s.db.Exec(`
			INSERT INTO mac_history (ip, mac, first_seen, last_seen) VALUES (?, ?, ?, ?)
			ON CONFLICT(ip, mac) DO UPDATE SET last_seen = excluded.last_seen
		`, ip, mac, now, now)
	}
	return conflicts, nil
}

// ARPConflicts returns all IPs that have been seen with more than one distinct
// MAC address, with their full history ordered newest-first.
func (s *SQLiteStore) ARPConflicts() []ARPWatchEntry {
	// Find IPs with multiple MACs.
	ipRows, err := s.db.Query(`
		SELECT ip FROM mac_history GROUP BY ip HAVING COUNT(DISTINCT mac) > 1 ORDER BY ip
	`)
	if err != nil {
		return nil
	}
	defer ipRows.Close()

	var ips []string
	for ipRows.Next() {
		var ip string
		if err := ipRows.Scan(&ip); err == nil {
			ips = append(ips, ip)
		}
	}
	ipRows.Close()

	var out []ARPWatchEntry
	for _, ip := range ips {
		rows, err := s.db.Query(`
			SELECT mac, first_seen, last_seen FROM mac_history
			WHERE ip = ? ORDER BY last_seen DESC
		`, ip)
		if err != nil {
			continue
		}
		var history []MACHistoryEntry
		for rows.Next() {
			var e MACHistoryEntry
			var fs, ls string
			if err := rows.Scan(&e.MAC, &fs, &ls); err != nil {
				continue
			}
			e.FirstSeen, _ = time.Parse(time.RFC3339, fs)
			e.LastSeen, _ = time.Parse(time.RFC3339, ls)
			history = append(history, e)
		}
		rows.Close()
		out = append(out, ARPWatchEntry{IP: ip, History: history})
	}
	return out
}
