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

// ── Speedtest ─────────────────────────────────────────────────────────────────

func TestSQLiteStore_SpeedtestSaveAndHistory(t *testing.T) {
	s := newTestStore(t)

	r1 := SpeedtestResult{DownloadMbps: 100, UploadMbps: 50, PingMs: 10, Server: "cf"}
	r2 := SpeedtestResult{DownloadMbps: 200, UploadMbps: 80, PingMs: 8, Server: "cf"}

	if err := s.SaveSpeedtest(r1); err != nil {
		t.Fatalf("SaveSpeedtest r1: %v", err)
	}
	if err := s.SaveSpeedtest(r2); err != nil {
		t.Fatalf("SaveSpeedtest r2: %v", err)
	}

	// Should return 2 results newest-first
	all := s.SpeedtestHistory(10)
	if len(all) != 2 {
		t.Fatalf("want 2 results, got %d", len(all))
	}
	// Newest first: r2 was inserted last
	if all[0].DownloadMbps != 200 {
		t.Errorf("first result: want 200 Mbps (newest), got %v", all[0].DownloadMbps)
	}

	// Limit to 1
	limited := s.SpeedtestHistory(1)
	if len(limited) != 1 {
		t.Fatalf("want 1 result with limit 1, got %d", len(limited))
	}
}

func TestSQLiteStore_SpeedtestHistory_Empty(t *testing.T) {
	s := newTestStore(t)
	results := s.SpeedtestHistory(10)
	if len(results) != 0 {
		t.Errorf("expected nil or empty, got %d results", len(results))
	}
}

// ── Email config ──────────────────────────────────────────────────────────────

func TestSQLiteStore_EmailConfig_Default(t *testing.T) {
	s := newTestStore(t)
	// Non-existent user ID — should return default config with Port 587
	cfg, err := s.GetEmailConfig(9999)
	if err != nil {
		t.Fatalf("GetEmailConfig: %v", err)
	}
	if cfg.Host != "" {
		t.Errorf("Host: want empty, got %q", cfg.Host)
	}
	if cfg.Port != 587 {
		t.Errorf("Port: want 587, got %d", cfg.Port)
	}
}

func TestSQLiteStore_EmailConfig_SaveAndGet(t *testing.T) {
	s := newTestStore(t)
	user, err := s.CreateUser("email@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cfg := EmailConfig{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user",
		Password: "secret",
		From:     "from@example.com",
		To:       "to@example.com",
	}
	if err := s.SaveEmailConfig(user.ID, cfg); err != nil {
		t.Fatalf("SaveEmailConfig: %v", err)
	}

	got, err := s.GetEmailConfig(user.ID)
	if err != nil {
		t.Fatalf("GetEmailConfig: %v", err)
	}
	if got.Host != cfg.Host {
		t.Errorf("Host: want %q, got %q", cfg.Host, got.Host)
	}
	if got.Port != cfg.Port {
		t.Errorf("Port: want %d, got %d", cfg.Port, got.Port)
	}
	if got.Username != cfg.Username {
		t.Errorf("Username: want %q, got %q", cfg.Username, got.Username)
	}
	if got.Password != cfg.Password {
		t.Errorf("Password: want %q, got %q", cfg.Password, got.Password)
	}
	if got.From != cfg.From {
		t.Errorf("From: want %q, got %q", cfg.From, got.From)
	}
	if got.To != cfg.To {
		t.Errorf("To: want %q, got %q", cfg.To, got.To)
	}
}

func TestSQLiteStore_EmailConfig_Update(t *testing.T) {
	s := newTestStore(t)
	user, err := s.CreateUser("update@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	first := EmailConfig{Host: "smtp1.example.com", Port: 587, To: "a@example.com"}
	if err := s.SaveEmailConfig(user.ID, first); err != nil {
		t.Fatalf("SaveEmailConfig first: %v", err)
	}

	second := EmailConfig{Host: "smtp2.example.com", Port: 465, To: "b@example.com"}
	if err := s.SaveEmailConfig(user.ID, second); err != nil {
		t.Fatalf("SaveEmailConfig second: %v", err)
	}

	got, err := s.GetEmailConfig(user.ID)
	if err != nil {
		t.Fatalf("GetEmailConfig: %v", err)
	}
	if got.Host != second.Host {
		t.Errorf("Host: want %q (second value), got %q", second.Host, got.Host)
	}
	if got.To != second.To {
		t.Errorf("To: want %q (second value), got %q", second.To, got.To)
	}
}

func TestSQLiteStore_GetAllEmailConfigs_Empty(t *testing.T) {
	s := newTestStore(t)
	cfgs := s.GetAllEmailConfigs()
	if len(cfgs) != 0 {
		t.Errorf("want nil or empty, got %d configs", len(cfgs))
	}
}

func TestSQLiteStore_GetAllEmailConfigs_OnlyComplete(t *testing.T) {
	s := newTestStore(t)

	// User 1: config with empty host — should NOT be returned
	u1, _ := s.CreateUser("incomplete@example.com", "hash")
	s.SaveEmailConfig(u1.ID, EmailConfig{Host: "", Port: 587, To: "to@example.com"}) //nolint:errcheck

	// User 2: complete config with host and to — SHOULD be returned
	u2, _ := s.CreateUser("complete@example.com", "hash")
	s.SaveEmailConfig(u2.ID, EmailConfig{Host: "smtp.example.com", Port: 587, To: "alerts@example.com"}) //nolint:errcheck

	cfgs := s.GetAllEmailConfigs()
	if len(cfgs) != 1 {
		t.Fatalf("want 1 complete config, got %d", len(cfgs))
	}
	if cfgs[0].Host != "smtp.example.com" {
		t.Errorf("Host: want smtp.example.com, got %q", cfgs[0].Host)
	}
}

// ── Telegram all-users ────────────────────────────────────────────────────────

func TestSQLiteStore_GetAllTelegramConfigs(t *testing.T) {
	s := newTestStore(t)

	if cfgs := s.GetAllTelegramConfigs(); len(cfgs) != 0 {
		t.Errorf("want empty before any config, got %d", len(cfgs))
	}

	u, _ := s.CreateUser("tg@example.com", "hash")
	if err := s.SaveTelegramConfig(u.ID, "bot123token", "chatid456"); err != nil {
		t.Fatalf("SaveTelegramConfig: %v", err)
	}

	cfgs := s.GetAllTelegramConfigs()
	if len(cfgs) != 1 {
		t.Fatalf("want 1 config, got %d", len(cfgs))
	}
	if cfgs[0].Token != "bot123token" {
		t.Errorf("Token: want bot123token, got %q", cfgs[0].Token)
	}
	if cfgs[0].ChatID != "chatid456" {
		t.Errorf("ChatID: want chatid456, got %q", cfgs[0].ChatID)
	}
}

// ── Webhook all-users ─────────────────────────────────────────────────────────

func TestSQLiteStore_GetAllWebhookURLs(t *testing.T) {
	s := newTestStore(t)

	if urls := s.GetAllWebhookURLs(); len(urls) != 0 {
		t.Errorf("want empty before any config, got %d", len(urls))
	}

	u1, _ := s.CreateUser("wh1@example.com", "hash")
	u2, _ := s.CreateUser("wh2@example.com", "hash")
	s.SaveWebhookURL(u1.ID, "https://hooks.example.com/1") //nolint:errcheck
	s.SaveWebhookURL(u2.ID, "https://hooks.example.com/2") //nolint:errcheck

	urls := s.GetAllWebhookURLs()
	if len(urls) != 2 {
		t.Fatalf("want 2 URLs, got %d", len(urls))
	}
}

func TestSQLiteStore_WebhookURL_GetSet(t *testing.T) {
	s := newTestStore(t)
	u, _ := s.CreateUser("wh@example.com", "hash")

	if err := s.SaveWebhookURL(u.ID, "https://example.com/hook"); err != nil {
		t.Fatalf("SaveWebhookURL: %v", err)
	}
	got, err := s.GetWebhookURL(u.ID)
	if err != nil {
		t.Fatalf("GetWebhookURL: %v", err)
	}
	if got != "https://example.com/hook" {
		t.Errorf("want https://example.com/hook, got %q", got)
	}
}
