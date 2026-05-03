// ============================================================
// controller/internal/storage/store.go — Store interface + JSONStore
//
// Store is the single interface everything outside this package uses
// for persistence. The active backend is SQLiteStore (sqlite_store.go).
// JSONStore is kept as a zero-dependency fallback; swap it in main.go.
//
// Key types:
//   Record             — one full scan (timestamp + all device results)
//   DeviceHistoryEntry — one per-device row: timestamp, alive, open ports
// ============================================================

package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
)

const maxRecords = 100

// Record is one complete scan snapshot: when it ran and every device found.
type Record struct {
	ScannedAt time.Time        `json:"scanned_at"`
	Results   []scanner.Result `json:"results"`
}

// DeviceHistoryEntry is one row in the per-device scan history.
type DeviceHistoryEntry struct {
	ScannedAt time.Time `json:"scanned_at"`
	Alive     bool      `json:"alive"`
	OpenPorts []uint16  `json:"open_ports"`
}

// MACChange is returned by UpdateMACHistory when an IP is seen with a different
// MAC than the last time — the classic signal of ARP cache poisoning.
type MACChange struct {
	IP     string // IP address that changed
	OldMAC string // MAC seen in the most recent previous scan
	NewMAC string // MAC seen in the current scan
}

// MACHistoryEntry is one row in the mac_history table for a given IP.
type MACHistoryEntry struct {
	MAC       string    `json:"mac"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// ARPWatchEntry groups the full MAC history for an IP that has been seen with
// more than one distinct MAC address (i.e. a potential spoof target).
type ARPWatchEntry struct {
	IP      string           `json:"ip"`
	History []MACHistoryEntry `json:"history"` // newest first; [0] is the current MAC
}

// ChangeLogEntry is one persisted network-change event.
type ChangeLogEntry struct {
	OccurredAt time.Time      `json:"occurred_at"`
	Kind       diff.ChangeKind `json:"kind"`
	IP         string         `json:"ip"`
	Desc       string         `json:"desc"`
}

// SpeedtestResult is one recorded internet speed test.
type SpeedtestResult struct {
	TestedAt     time.Time `json:"tested_at"`
	DownloadMbps float64   `json:"download_mbps"`
	UploadMbps   float64   `json:"upload_mbps"`
	PingMs       float64   `json:"ping_ms"`
	Server       string    `json:"server"`
}

// Store is the interface the rest of the application uses for persistence.
// Swap the implementation (JSON file, SQLite, Postgres) without touching
// any API handler or controller code — just satisfy this interface.
type Store interface {
	Save(results []scanner.Result) error
	Latest() []scanner.Result
	LatestRecord() *Record
	History(n int) []Record
	// AllKnownIPs returns every IP address ever recorded across all scans.
	// Used by the diff engine to distinguish truly new devices from ones
	// that are returning after a temporary absence.
	AllKnownIPs() map[string]bool
	// AllDevices returns one entry per IP ever seen, with the most recent
	// data for each device. alive is true only if the device appeared in
	// the latest scan. This gives a stable registry view — devices don't
	// vanish from the UI just because they missed one scan cycle.
	// User-assigned labels are included in each result's Label field.
	AllDevices() []scanner.Result
	// DeviceHistory returns the last n scan entries for the given IP, oldest first.
	DeviceHistory(ip string, n int) []DeviceHistoryEntry

	// SetLabel stores a user-defined name for the given IP address.
	SetLabel(ip, name string) error
	// DeleteLabel removes any user-defined name for the given IP address.
	DeleteLabel(ip string) error
	// GetLabels returns all stored ip→name mappings.
	GetLabels() map[string]string

	// SetNotify enables or disables change alerts for the given IP address.
	// Devices default to enabled; only a false value is stored persistently.
	SetNotify(ip string, enabled bool) error

	// SaveSpeedtest stores one speed test result.
	SaveSpeedtest(r SpeedtestResult) error
	// SpeedtestHistory returns the last n speed test results, newest first.
	SpeedtestHistory(n int) []SpeedtestResult

	// SaveChanges persists the diff.Change events produced after a scan.
	// occurredAt is stamped onto each entry so the log is queryable by time.
	SaveChanges(changes []diff.Change, occurredAt time.Time) error
	// Changes returns the last n change-log entries, newest first.
	Changes(n int) []ChangeLogEntry

	// UpdateMACHistory records the current IP→MAC mapping for every alive device
	// and returns a MACChange for any IP whose MAC differs from the last recorded one.
	// Call this once per scan cycle, after enrichment.
	UpdateMACHistory(results []scanner.Result) ([]MACChange, error)
	// ARPConflicts returns all IPs that have been seen with more than one distinct
	// MAC address, along with their full MAC history (newest first).
	ARPConflicts() []ARPWatchEntry
	// PruneOldData removes scan records older than scanAge and change-log entries
	// older than changeAge. Call periodically to prevent unbounded DB growth.
	PruneOldData(scanAge, changeAge time.Duration) error
}

// JSONStore is the legacy Store implementation: a single JSON file on disk.
// It is no longer the active backend (SQLiteStore is). Kept as a simple
// fallback — swap it back in main.go if you need a zero-dependency option.
type JSONStore struct {
	path string
}

// New creates a JSONStore backed by the given file path.
func New(path string) (*JSONStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &JSONStore{path: path}, nil
}

func (s *JSONStore) Save(results []scanner.Result) error {
	records, _ := s.load()
	records = append(records, Record{ScannedAt: time.Now().UTC(), Results: results})
	if len(records) > maxRecords {
		records = records[len(records)-maxRecords:]
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Latest returns results from the most recent scan, or nil if none exists.
func (s *JSONStore) Latest() []scanner.Result {
	r := s.LatestRecord()
	if r == nil {
		return nil
	}
	return r.Results
}

// LatestRecord returns the full most recent scan record, or nil if none exists.
func (s *JSONStore) LatestRecord() *Record {
	records, err := s.load()
	if err != nil || len(records) == 0 {
		return nil
	}
	r := records[len(records)-1]
	return &r
}

// History returns the last n scan records (oldest first).
func (s *JSONStore) History(n int) []Record {
	records, _ := s.load()
	if len(records) <= n {
		return records
	}
	return records[len(records)-n:]
}

// AllKnownIPs returns every IP ever seen across all stored scan records.
func (s *JSONStore) AllKnownIPs() map[string]bool {
	records, _ := s.load()
	known := make(map[string]bool)
	for _, rec := range records {
		for _, r := range rec.Results {
			known[r.IP] = true
		}
	}
	return known
}

// AllDevices returns one entry per IP (latest data), alive only if in the most recent scan.
// User-assigned labels are overlaid from the labels file.
func (s *JSONStore) AllDevices() []scanner.Result {
	records, _ := s.load()
	if len(records) == 0 {
		return nil
	}
	latestIPs := make(map[string]bool, len(records[len(records)-1].Results))
	for _, r := range records[len(records)-1].Results {
		latestIPs[r.IP] = true
	}
	latest := make(map[string]scanner.Result)
	for _, rec := range records {
		for _, r := range rec.Results {
			latest[r.IP] = r
		}
	}
	labels := s.GetLabels()
	out := make([]scanner.Result, 0, len(latest))
	for _, r := range latest {
		r.Alive = latestIPs[r.IP]
		if name, ok := labels[r.IP]; ok {
			r.Label = &name
		}
		out = append(out, r)
	}
	return out
}

// DeviceHistory returns the per-scan history for ip from the JSON record file.
func (s *JSONStore) DeviceHistory(ip string, n int) []DeviceHistoryEntry {
	records, _ := s.load()
	var entries []DeviceHistoryEntry
	for _, rec := range records {
		for _, r := range rec.Results {
			if r.IP == ip {
				entries = append(entries, DeviceHistoryEntry{
					ScannedAt: rec.ScannedAt,
					Alive:     r.Alive,
					OpenPorts: r.OpenPorts,
				})
				break
			}
		}
	}
	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries
}

func (s *JSONStore) SetLabel(ip, name string) error {
	labels := s.GetLabels()
	labels[ip] = name
	return s.saveLabels(labels)
}

func (s *JSONStore) DeleteLabel(ip string) error {
	labels := s.GetLabels()
	delete(labels, ip)
	return s.saveLabels(labels)
}

func (s *JSONStore) GetLabels() map[string]string {
	data, err := os.ReadFile(s.labelsPath())
	if err != nil {
		return map[string]string{}
	}
	var labels map[string]string
	if err := json.Unmarshal(data, &labels); err != nil {
		return map[string]string{}
	}
	return labels
}

func (s *JSONStore) SetNotify(_ string, _ bool) error                              { return nil }
func (s *JSONStore) SaveSpeedtest(_ SpeedtestResult) error                         { return nil }
func (s *JSONStore) SpeedtestHistory(_ int) []SpeedtestResult                      { return nil }
func (s *JSONStore) SaveChanges(_ []diff.Change, _ time.Time) error                { return nil }
func (s *JSONStore) Changes(_ int) []ChangeLogEntry                                { return nil }
func (s *JSONStore) UpdateMACHistory(_ []scanner.Result) ([]MACChange, error)      { return nil, nil }
func (s *JSONStore) ARPConflicts() []ARPWatchEntry                                 { return nil }
func (s *JSONStore) PruneOldData(_, _ time.Duration) error                         { return nil }

func (s *JSONStore) labelsPath() string { return s.path + ".labels" }

func (s *JSONStore) saveLabels(labels map[string]string) error {
	data, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.labelsPath(), data, 0o644)
}

func (s *JSONStore) load() ([]Record, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	return records, json.Unmarshal(data, &records)
}
