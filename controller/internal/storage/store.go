package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ding/ding/internal/scanner"
)

const maxRecords = 100

type Record struct {
	ScannedAt time.Time        `json:"scanned_at"`
	Results   []scanner.Result `json:"results"`
}

// Store is the interface the rest of the application uses for persistence.
// Swap the implementation (JSON file, SQLite, Postgres) without touching
// any API handler or controller code — just satisfy this interface.
type Store interface {
	Save(results []scanner.Result) error
	Latest() []scanner.Result
	LatestRecord() *Record
	History(n int) []Record
}

// JSONStore is the default Store implementation: a single JSON file on disk.
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
