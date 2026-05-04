package main

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

func TestEnvOr_Missing(t *testing.T) {
	t.Setenv("DING_TEST_ABSENT_KEY", "")
	if got := envOr("DING_TEST_ABSENT_KEY_XXXXXXX", "fallback"); got != "fallback" {
		t.Errorf("missing key: want fallback, got %q", got)
	}
}

func TestEnvOr_Present(t *testing.T) {
	t.Setenv("DING_TEST_PRESENT_KEY", "value123")
	if got := envOr("DING_TEST_PRESENT_KEY", "fallback"); got != "value123" {
		t.Errorf("set key: want value123, got %q", got)
	}
}

func TestParseIntEnv_Valid(t *testing.T) {
	var dst int
	n, err := parseIntEnv("42", &dst)
	if err != nil {
		t.Fatalf("parseIntEnv: %v", err)
	}
	if n != 42 || dst != 42 {
		t.Errorf("want 42, got n=%d dst=%d", n, dst)
	}
}

func TestParseIntEnv_Invalid(t *testing.T) {
	var dst int
	_, err := parseIntEnv("not-a-number", &dst)
	if err == nil {
		t.Error("want error for non-integer input")
	}
}

func TestIPInLatestScan_Empty(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if ipInLatestScan(store, "192.168.1.1") {
		t.Error("want false for empty store")
	}
}

func TestIPInLatestScan_Found(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := store.Save([]scanner.Result{
		{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !ipInLatestScan(store, "192.168.1.1") {
		t.Error("want true for saved device")
	}
	if ipInLatestScan(store, "192.168.1.2") {
		t.Error("want false for unsaved device")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("DING_INTERFACE", "eth0")
	t.Setenv("DING_SUBNET", "192.168.1.0/24")
	t.Setenv("DING_TIMEOUT_MS", "")
	t.Setenv("DING_SCAN_INTERVAL", "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	if cfg.iface != "eth0" {
		t.Errorf("iface: want eth0, got %q", cfg.iface)
	}
	if cfg.subnet != "192.168.1.0/24" {
		t.Errorf("subnet: want 192.168.1.0/24, got %q", cfg.subnet)
	}
	if cfg.timeoutMs != 500 {
		t.Errorf("timeoutMs: want 500 (default), got %d", cfg.timeoutMs)
	}
	if cfg.scanInterval != 0 {
		t.Errorf("scanInterval: want 0 (disabled), got %v", cfg.scanInterval)
	}
	if cfg.ports != "22,80,443,554,8000,8080,8443" {
		t.Errorf("ports: unexpected default %q", cfg.ports)
	}
}

func TestConfigFromEnv_Custom(t *testing.T) {
	t.Setenv("DING_INTERFACE", "eth0")
	t.Setenv("DING_SUBNET", "10.0.0.0/24")
	t.Setenv("DING_TIMEOUT_MS", "300")
	t.Setenv("DING_SCAN_INTERVAL", "120s")
	t.Setenv("DING_PORTS", "22,80,443")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	if cfg.timeoutMs != 300 {
		t.Errorf("timeoutMs: want 300, got %d", cfg.timeoutMs)
	}
	if cfg.scanInterval.Seconds() != 120 {
		t.Errorf("scanInterval: want 120s, got %v", cfg.scanInterval)
	}
	if cfg.ports != "22,80,443" {
		t.Errorf("ports: want 22,80,443, got %q", cfg.ports)
	}
}

func TestConfigFromEnv_BadInterval(t *testing.T) {
	t.Setenv("DING_INTERFACE", "eth0")
	t.Setenv("DING_SUBNET", "192.168.1.0/24")
	t.Setenv("DING_SCAN_INTERVAL", "not-a-duration")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv with bad interval: %v", err)
	}
	if cfg.scanInterval != 0 {
		t.Errorf("bad interval should result in 0, got %v", cfg.scanInterval)
	}
}

func TestResolveInterface_NotFound(t *testing.T) {
	_, err := resolveInterface("ding-nonexistent-iface-xyz")
	if err == nil {
		t.Error("want error for nonexistent interface")
	}
}
