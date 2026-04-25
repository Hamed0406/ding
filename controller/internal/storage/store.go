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

type Store struct {
	path string
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

func (s *Store) Save(results []scanner.Result) error {
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
func (s *Store) Latest() []scanner.Result {
	r := s.LatestRecord()
	if r == nil {
		return nil
	}
	return r.Results
}

// LatestRecord returns the full most recent scan record, or nil if none exists.
func (s *Store) LatestRecord() *Record {
	records, err := s.load()
	if err != nil || len(records) == 0 {
		return nil
	}
	r := records[len(records)-1]
	return &r
}

// History returns the last n scan records (oldest first).
func (s *Store) History(n int) []Record {
	records, _ := s.load()
	if len(records) <= n {
		return records
	}
	return records[len(records)-n:]
}

func (s *Store) load() ([]Record, error) {
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
