package storage

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	return s
}

func strp(s string) *string { return &s }

func TestSQLiteStore_EmptyLatest(t *testing.T) {
	s := newTestStore(t)
	if s.Latest() != nil {
		t.Error("expected nil before any scan")
	}
	if s.LatestRecord() != nil {
		t.Error("expected nil before any scan")
	}
}

func TestSQLiteStore_EmptyHistory(t *testing.T) {
	s := newTestStore(t)
	h := s.History(10)
	if len(h) != 0 {
		t.Errorf("expected empty history, got %d records", len(h))
	}
}

func TestSQLiteStore_SaveAndLatest(t *testing.T) {
	s := newTestStore(t)

	results := []scanner.Result{
		{IP: "192.168.1.1", MAC: strp("aa:bb:cc:dd:ee:01"), Alive: true, OpenPorts: []uint16{22, 80}},
		{IP: "192.168.1.2", Alive: false, OpenPorts: []uint16{}},
	}
	if err := s.Save(results); err != nil {
		t.Fatalf("Save: %v", err)
	}

	latest := s.Latest()
	if len(latest) != 2 {
		t.Fatalf("want 2 results, got %d", len(latest))
	}

	// Verify first device
	found := false
	for _, r := range latest {
		if r.IP == "192.168.1.1" {
			found = true
			if !r.Alive {
				t.Error("want alive=true")
			}
			if r.MAC == nil || *r.MAC != "aa:bb:cc:dd:ee:01" {
				t.Errorf("want mac aa:bb:cc:dd:ee:01, got %v", r.MAC)
			}
			if len(r.OpenPorts) != 2 {
				t.Errorf("want 2 open ports, got %d", len(r.OpenPorts))
			}
		}
	}
	if !found {
		t.Error("192.168.1.1 not found in latest results")
	}
}

func TestSQLiteStore_NullableFields(t *testing.T) {
	s := newTestStore(t)

	ttl := uint8(64)
	results := []scanner.Result{{
		IP:       "10.0.0.1",
		Hostname: strp("router.lan"),
		Vendor:   strp("Cisco"),
		Gateway:  strp("10.0.0.1"),
		TTL:      &ttl,
		Alive:    true,
		OpenPorts: []uint16{},
	}}
	if err := s.Save(results); err != nil {
		t.Fatalf("Save: %v", err)
	}

	latest := s.Latest()
	if len(latest) != 1 {
		t.Fatalf("want 1 result, got %d", len(latest))
	}
	r := latest[0]
	if r.Hostname == nil || *r.Hostname != "router.lan" {
		t.Errorf("hostname: got %v", r.Hostname)
	}
	if r.Vendor == nil || *r.Vendor != "Cisco" {
		t.Errorf("vendor: got %v", r.Vendor)
	}
	if r.TTL == nil || *r.TTL != 64 {
		t.Errorf("ttl: got %v", r.TTL)
	}
}

func TestSQLiteStore_History_Order(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 3; i++ {
		if err := s.Save([]scanner.Result{{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{}}}); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	h := s.History(10)
	if len(h) != 3 {
		t.Fatalf("want 3 records, got %d", len(h))
	}
	// History should be oldest-first
	for i := 1; i < len(h); i++ {
		if h[i].ScannedAt.Before(h[i-1].ScannedAt) {
			t.Errorf("record %d is older than record %d — want oldest-first", i, i-1)
		}
	}
}

func TestSQLiteStore_History_Limit(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		if err := s.Save([]scanner.Result{{IP: "10.0.0.1", OpenPorts: []uint16{}}}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	h := s.History(3)
	if len(h) != 3 {
		t.Errorf("want 3 records (limit), got %d", len(h))
	}
}

func TestSQLiteStore_MultipleScans_LatestIsNewest(t *testing.T) {
	s := newTestStore(t)

	if err := s.Save([]scanner.Result{{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{22}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save([]scanner.Result{{IP: "192.168.1.2", Alive: true, OpenPorts: []uint16{80}}}); err != nil {
		t.Fatal(err)
	}

	latest := s.Latest()
	if len(latest) != 1 || latest[0].IP != "192.168.1.2" {
		t.Errorf("Latest() should return newest scan; got %v", latest)
	}
}

func TestSQLiteStore_AllDevices_StableRegistry(t *testing.T) {
	s := newTestStore(t)

	// Scan 1: two devices
	if err := s.Save([]scanner.Result{
		{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{22}},
		{IP: "192.168.1.2", Alive: true, OpenPorts: []uint16{80}},
	}); err != nil {
		t.Fatal(err)
	}

	// Scan 2: only one device (192.168.1.2 is "gone")
	if err := s.Save([]scanner.Result{
		{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{22}},
	}); err != nil {
		t.Fatal(err)
	}

	all := s.AllDevices()
	if len(all) != 2 {
		t.Fatalf("AllDevices() want 2 (registry), got %d", len(all))
	}

	byIP := make(map[string]scanner.Result)
	for _, r := range all {
		byIP[r.IP] = r
	}

	if !byIP["192.168.1.1"].Alive {
		t.Error("192.168.1.1 should be alive (in latest scan)")
	}
	if byIP["192.168.1.2"].Alive {
		t.Error("192.168.1.2 should be offline (not in latest scan)")
	}
}

func TestSQLiteStore_AllDevices_Empty(t *testing.T) {
	s := newTestStore(t)
	if all := s.AllDevices(); all != nil {
		t.Errorf("AllDevices() on empty store should return nil, got %v", all)
	}
}
