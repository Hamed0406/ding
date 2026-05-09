package main

import (
	"context"
	"log/slog"
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

func TestResolveInterface_EmptyHint(t *testing.T) {
	// Auto-detect the first available interface; skip if none present (e.g. minimal CI).
	iface, err := resolveInterface("")
	if err != nil {
		t.Skip("no suitable network interface available:", err)
	}
	if iface.Name == "" {
		t.Error("want a non-empty interface name")
	}
	if iface.Subnet == "" {
		t.Error("want a non-empty subnet")
	}
}

func TestResolveInterface_ValidHint(t *testing.T) {
	// Find any available interface, then resolve by name explicitly.
	first, err := resolveInterface("")
	if err != nil {
		t.Skip("no suitable network interface available:", err)
	}
	result, err := resolveInterface(first.Name)
	if err != nil {
		t.Fatalf("resolveInterface(%q): %v", first.Name, err)
	}
	if result.Name != first.Name {
		t.Errorf("want %q, got %q", first.Name, result.Name)
	}
}

func TestConfigFromEnv_SubnetAutoDetect(t *testing.T) {
	// Set iface but not subnet — should auto-detect subnet from the interface.
	t.Setenv("DING_INTERFACE", "eth0")
	t.Setenv("DING_SUBNET", "")

	cfg, err := configFromEnv()
	if err != nil {
		// In CI without eth0, auto-detect may legitimately fail; skip.
		t.Skip("auto-detect failed (no eth0 or no subnet):", err)
	}
	if cfg.iface != "eth0" {
		t.Errorf("iface: want eth0, got %q", cfg.iface)
	}
	if cfg.subnet == "" {
		t.Error("subnet should have been auto-detected")
	}
}

// ── initLogger ────────────────────────────────────────────────────────────────

func TestInitLogger_DefaultsToTextInfoLevel(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_LEVEL", "")
	t.Setenv("DING_LOG_FORMAT", "")
	initLogger()

	ctx := context.Background()
	h := slog.Default().Handler()
	if _, ok := h.(*slog.TextHandler); !ok {
		t.Errorf("want TextHandler by default, got %T", h)
	}
	if !h.Enabled(ctx, slog.LevelInfo) {
		t.Error("info should be enabled at default level")
	}
	if h.Enabled(ctx, slog.LevelDebug) {
		t.Error("debug should not be enabled at default info level")
	}
}

func TestInitLogger_JSONFormat(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_FORMAT", "json")
	t.Setenv("DING_LOG_LEVEL", "")
	initLogger()

	if _, ok := slog.Default().Handler().(*slog.JSONHandler); !ok {
		t.Errorf("want JSONHandler for DING_LOG_FORMAT=json, got %T", slog.Default().Handler())
	}
}

func TestInitLogger_DebugLevel(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_LEVEL", "debug")
	t.Setenv("DING_LOG_FORMAT", "")
	initLogger()

	ctx := context.Background()
	if !slog.Default().Handler().Enabled(ctx, slog.LevelDebug) {
		t.Error("debug should be enabled when DING_LOG_LEVEL=debug")
	}
}

func TestInitLogger_WarnLevelSuppressesInfo(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_LEVEL", "warn")
	t.Setenv("DING_LOG_FORMAT", "")
	initLogger()

	ctx := context.Background()
	h := slog.Default().Handler()
	if h.Enabled(ctx, slog.LevelInfo) {
		t.Error("info should not be enabled at warn level")
	}
	if !h.Enabled(ctx, slog.LevelWarn) {
		t.Error("warn should be enabled at warn level")
	}
}

func TestInitLogger_ErrorLevelSuppressesWarn(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_LEVEL", "error")
	t.Setenv("DING_LOG_FORMAT", "")
	initLogger()

	ctx := context.Background()
	h := slog.Default().Handler()
	if h.Enabled(ctx, slog.LevelWarn) {
		t.Error("warn should not be enabled at error level")
	}
	if !h.Enabled(ctx, slog.LevelError) {
		t.Error("error should be enabled at error level")
	}
}

func TestInitLogger_CaseInsensitiveLevel(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	t.Setenv("DING_LOG_LEVEL", "DEBUG")
	t.Setenv("DING_LOG_FORMAT", "JSON")
	initLogger()

	ctx := context.Background()
	if !slog.Default().Handler().Enabled(ctx, slog.LevelDebug) {
		t.Error("DEBUG (uppercase) should be accepted")
	}
	if _, ok := slog.Default().Handler().(*slog.JSONHandler); !ok {
		t.Errorf("JSON (uppercase) should set JSONHandler, got %T", slog.Default().Handler())
	}
}
