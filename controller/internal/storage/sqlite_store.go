// ============================================================
// controller/internal/storage/sqlite_store.go — SQLite backend
//
// SQLiteStore is the active Store implementation. Schema:
//   scans   — one row per scan run (id, scanned_at)
//   devices — one row per device per scan (all fields + scan_id FK)
//   device_labels — user-assigned names, keyed by IP (survives scan cycles)
//
// WAL mode is enabled so reads never block writes (important during scans).
// MaxOpenConns=1 avoids "database is locked" since SQLite has one writer.
// ============================================================

package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ding/ding/internal/scanner"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using a local SQLite database.
// Schema is normalized: one row per device per scan, enabling future analytics.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLite opens (or creates) a SQLite database at path and runs migrations.
func NewSQLite(path string) (*SQLiteStore, error) {
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
	// SQLite supports one writer at a time; cap pool to avoid "database is locked".
	db.SetMaxOpenConns(1)

	if err := sqliteMigrate(db); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
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
		CREATE INDEX IF NOT EXISTS idx_devices_scan ON devices(scan_id);
		CREATE INDEX IF NOT EXISTS idx_devices_ip   ON devices(ip);
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
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_providers (
			user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider    TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			PRIMARY KEY (provider, provider_id)
		)
	`)
	return nil
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

	for _, r := range results {
		ports, _ := json.Marshal(r.OpenPorts)
		if _, err := stmt.Exec(
			scanID, r.IP, r.MAC, r.Hostname, r.Vendor, r.DeviceType, r.OS,
			string(ports), boolToInt(r.Alive), r.Gateway, r.TTL,
		); err != nil {
			return err
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
	rows, err := s.db.Query(`
		SELECT
			d.ip, d.mac, d.hostname, d.vendor, d.device_type, d.os, d.open_ports,
			CASE WHEN d.scan_id = (SELECT MAX(id) FROM scans) THEN d.alive ELSE 0 END,
			d.gateway, d.ttl, l.name, COALESCE(n.enabled, 0) AS notify
		FROM devices d
		LEFT JOIN device_labels l ON l.ip = d.ip
		LEFT JOIN device_notify n ON n.ip = d.ip
		WHERE d.scan_id = (
			SELECT MAX(d2.scan_id) FROM devices d2 WHERE d2.ip = d.ip
		)
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
		       d.alive, d.gateway, d.ttl, l.name, 1 AS notify
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
// Expects columns: ip, mac, hostname, vendor, device_type, os, open_ports, alive, gateway, ttl, label, notify.
func scanDeviceRows(rows *sql.Rows) ([]scanner.Result, error) {
	var results []scanner.Result
	for rows.Next() {
		var r scanner.Result
		var mac, hostname, vendor, deviceType, os, label, gateway sql.NullString
		var ttl sql.NullInt64
		var portsJSON string
		var alive, notify int

		if err := rows.Scan(
			&r.IP, &mac, &hostname, &vendor, &deviceType, &os,
			&portsJSON, &alive, &gateway, &ttl, &label, &notify,
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
