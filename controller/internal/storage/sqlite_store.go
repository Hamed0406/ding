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
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_id    INTEGER NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
			ip         TEXT    NOT NULL,
			mac        TEXT,
			hostname   TEXT,
			vendor     TEXT,
			open_ports TEXT    NOT NULL DEFAULT '[]',
			alive      INTEGER NOT NULL DEFAULT 0,
			gateway    TEXT,
			ttl        INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_devices_scan ON devices(scan_id);
		CREATE INDEX IF NOT EXISTS idx_devices_ip   ON devices(ip);
	`)
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
		INSERT INTO devices (scan_id, ip, mac, hostname, vendor, open_ports, alive, gateway, ttl)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range results {
		ports, _ := json.Marshal(r.OpenPorts)
		if _, err := stmt.Exec(
			scanID, r.IP, r.MAC, r.Hostname, r.Vendor,
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

func (s *SQLiteStore) queryDevices(scanID int64) ([]scanner.Result, error) {
	rows, err := s.db.Query(`
		SELECT ip, mac, hostname, vendor, open_ports, alive, gateway, ttl
		FROM devices WHERE scan_id = ?
	`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []scanner.Result
	for rows.Next() {
		var r scanner.Result
		var mac, hostname, vendor, gateway sql.NullString
		var ttl sql.NullInt64
		var portsJSON string
		var alive int

		if err := rows.Scan(&r.IP, &mac, &hostname, &vendor, &portsJSON, &alive, &gateway, &ttl); err != nil {
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
		if gateway.Valid {
			r.Gateway = &gateway.String
		}
		if ttl.Valid {
			v := uint8(ttl.Int64)
			r.TTL = &v
		}
		r.Alive = alive != 0
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
