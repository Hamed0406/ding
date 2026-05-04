package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestJSONStore_New(t *testing.T) {
	dir := t.TempDir()
	store, err := New(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Fatal("want non-nil store")
	}
}

func TestJSONStore_StubMethods(t *testing.T) {
	dir := t.TempDir()
	store, err := New(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := store.SetNotify("192.168.1.1", true); err != nil {
		t.Errorf("SetNotify: %v", err)
	}
	if err := store.SaveSpeedtest(SpeedtestResult{TestedAt: time.Now()}); err != nil {
		t.Errorf("SaveSpeedtest: %v", err)
	}
	if h := store.SpeedtestHistory(10); h != nil {
		t.Errorf("SpeedtestHistory: want nil, got %v", h)
	}
	if err := store.SaveChanges(nil, time.Now()); err != nil {
		t.Errorf("SaveChanges: %v", err)
	}
	if c := store.Changes(10); c != nil {
		t.Errorf("Changes: want nil, got %v", c)
	}
	if changes, err := store.UpdateMACHistory(nil); err != nil || changes != nil {
		t.Errorf("UpdateMACHistory: err=%v changes=%v", err, changes)
	}
	if c := store.ARPConflicts(); c != nil {
		t.Errorf("ARPConflicts: want nil, got %v", c)
	}
	if err := store.PruneOldData(time.Hour, time.Hour); err != nil {
		t.Errorf("PruneOldData: %v", err)
	}
}
