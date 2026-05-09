// ============================================================
// controller/cmd/ding/main.go — Entry point for the Go binary
//
// This is the brain of Ding. It:
//   1. Reads configuration from environment variables
//   2. Auto-detects the network interface if not configured
//   3. Starts the web server (UI + REST API + real-time SSE)
//   4. Runs an initial scan immediately on startup
//   5. Keeps scanning on a timer (DING_SCAN_INTERVAL)
//   6. Shuts down gracefully when you press Ctrl+C
// ============================================================

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ding/ding/internal/alert"
	"github.com/ding/ding/internal/api"
	"github.com/ding/ding/internal/classify"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/email"
	"github.com/ding/ding/internal/enrich"
	"github.com/ding/ding/internal/iface"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
	"github.com/ding/ding/internal/vendor"
)

func main() {
	initLogger()

	// Load all settings from environment variables (with sensible defaults)
	cfg, err := configFromEnv()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	// Fail fast if the scanner binary is missing rather than letting the first
	// scan silently fail and show a cryptic error in the UI.
	scanBin := envOr("DING_SCANNER_BIN", "/usr/local/bin/scanner")
	if _, err := os.Stat(scanBin); err != nil {
		slog.Error("scanner binary not found", "path", scanBin, "hint", "set DING_SCANNER_BIN to the correct path")
		os.Exit(1)
	}

	// Open (or create) the SQLite database where we save scan history
	store, err := storage.NewSQLite(cfg.dataPath, os.Getenv("DING_SECRET_KEY"))
	if err != nil {
		slog.Error("storage error", "err", err)
		os.Exit(1)
	}
	if os.Getenv("DING_SECRET_KEY") == "" {
		slog.Warn("DING_SECRET_KEY not set — SMTP passwords stored in plaintext")
	}

	// scanFn is a reusable function that runs one complete scan cycle:
	//   Run Rust scanner → compare to last scan → save → send alerts
	// Both the timer and the "Scan Now" button call this same function.
	scanFn := func() ([]scanner.Result, []diff.Change, error) {
		// Run the ARP/ICMP/TCP scan and the mDNS discovery in parallel.
		// mDNS listens for 3 s — same order as the active scan — so both
		// finish at roughly the same time with no added latency.
		var (
			results    []scanner.Result
			mdnsEvents []scanner.MdnsEvent
			scanErr    error
		)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			results, scanErr = scanner.Run(cfg.iface, cfg.subnet, cfg.ports, cfg.timeoutMs)
		}()
		go func() {
			defer wg.Done()
			var err error
			mdnsEvents, err = scanner.RunMDNS(cfg.iface, 3000)
			if err != nil {
				slog.Warn("mDNS scan failed, continuing without mDNS data", "err", err)
			}
		}()
		wg.Wait()

		if scanErr != nil {
			return nil, nil, scanErr
		}

		// Resolve hostnames via reverse DNS (best-effort, runs in parallel).
		// Devices without a PTR record simply stay unnamed.
		enrich.Hostnames(results, 16, 300*time.Millisecond)

		// For devices DNS couldn't name, try NetBIOS Node Status (UDP 137).
		// Catches Windows PCs, NAS boxes, and printers that skip PTR records.
		enrich.NetBIOSNames(results, 16, 1*time.Second)

		// Tag each device with its MAC vendor (Apple, Cisco, etc.).
		// Pure in-memory lookup against the embedded IEEE OUI database.
		vendor.Annotate(results)

		// Guess device category from vendor name and open ports.
		// Must run before ApplyMDNS so mDNS can override the guess.
		classify.Annotate(results)

		// Probe HTTP on port 80/8000/8080 for devices classify couldn't identify.
		// Reads the Server header and <title> tag — catches cameras, routers,
		// and NAS boxes that don't have a recognised vendor OUI.
		// Runs with 8 workers and a short timeout so it adds minimal latency.
		enrich.BannerDeviceType(results, 8, 800*time.Millisecond)

		// Query SNMP sysDescr + sysName (UDP 161, community "public") on devices
		// that still have no DeviceType or Hostname. Catches routers, switches,
		// NAS boxes, and printers that expose an SNMP agent but not HTTP/RTSP.
		enrich.SNMPDeviceType(results, 16, 800*time.Millisecond)

		// Probe RTSP on port 554 for remaining unclassified devices.
		// Sends OPTIONS * RTSP/1.0 and marks confirmed RTSP servers as "IP Camera".
		// Belt-and-suspenders: classify already catches 554 open → IP Camera,
		// but this catches cameras that need a protocol handshake to self-identify.
		enrich.RTSPDeviceType(results, 8, 600*time.Millisecond)

		// Override DeviceType with authoritative mDNS service data where available.
		enrich.ApplyMDNS(results, mdnsEvents)

		// Infer OS from TTL (rounded to nearest standard initial value) and
		// open port hints. Runs after classify so it can't clobber DeviceType.
		enrich.AnnotateOS(results)

		// Check for ARP spoofing: detect IPs that answered with a different MAC
		// than the last recorded one and tag the affected results immediately so
		// the UI can show a warning badge.
		macChanges, err := store.UpdateMACHistory(results)
		if err != nil {
			slog.Error("arp watch", "err", err)
		}
		for i := range results {
			for _, mc := range macChanges {
				if results[i].IP == mc.IP {
					results[i].MACConflict = &mc.OldMAC
				}
			}
		}

		// Load what we found last time so we can compare
		previous := store.Latest()

		// Collect all IPs ever seen — must happen before Save so that
		// truly new devices aren't counted as known yet.
		knownIPs := store.AllKnownIPs()

		// Figure out what changed: new devices, gone devices, returning devices, port changes
		changes := diff.Compare(previous, results, knownIPs)

		// Append MAC_CHANGE events — these are always included in alerts regardless
		// of the per-device notify preference because they are security events.
		for _, mc := range macChanges {
			changes = append(changes, diff.Change{
				Kind: diff.KindMACChange,
				IP:   mc.IP,
				Desc: fmt.Sprintf("was %s now %s — possible ARP spoofing", mc.OldMAC, mc.NewMAC),
			})
		}

		// Save the new results to disk
		if err := store.Save(results); err != nil {
			slog.Error("save scan results", "err", err)
		}

		// Persist the change events so the history log survives page reloads.
		if err := store.SaveChanges(changes, time.Now()); err != nil {
			slog.Error("save changes", "err", err)
		}

		// Build the filtered change list: devices with notifications enabled, plus
		// MAC_CHANGE events which always alert regardless of per-device preference.
		var alertChanges []diff.Change
		if len(changes) > 0 {
			enabled := make(map[string]bool)
			for _, d := range store.AllDevices() {
				if d.Notify {
					enabled[d.IP] = true
				}
			}
			for _, c := range changes {
				// NEW and MAC_CHANGE are unconditional — a genuinely new device
				// has no per-device preference yet, and a MAC change is a
				// security event; both should always alert.
				if c.Kind == diff.KindNew || c.Kind == diff.KindMACChange || enabled[c.IP] {
					alertChanges = append(alertChanges, c)
				}
			}
		}
		// Send to all users who have configured Telegram in the UI.
		// Fall back to the env-var config if no user has set one.
		telegramCfgs := store.GetAllTelegramConfigs()
		if len(telegramCfgs) == 0 && cfg.alert.TelegramToken != "" {
			telegramCfgs = []storage.TelegramConfig{{Token: cfg.alert.TelegramToken, ChatID: cfg.alert.TelegramChatID}}
		}
		for _, tc := range telegramCfgs {
			if err := alert.Send(alert.Config{TelegramToken: tc.Token, TelegramChatID: tc.ChatID}, alertChanges); err != nil {
				slog.Error("telegram alert", "err", err)
			}
		}
		for _, webhookURL := range store.GetAllWebhookURLs() {
			if err := alert.Send(alert.Config{WebhookURL: webhookURL}, alertChanges); err != nil {
				slog.Error("webhook alert", "err", err)
			}
		}
		if len(alertChanges) > 0 {
			for _, ec := range store.GetAllEmailConfigs() {
				if err := email.Send(email.Config{
					Host:     ec.Host,
					Port:     ec.Port,
					Username: ec.Username,
					Password: ec.Password,
					From:     ec.From,
					To:       ec.To,
				}, alertChanges); err != nil {
					slog.Error("email alert", "err", err)
				}
			}
		}

		return results, changes, nil
	}

	// The broker is the "messenger" — it pushes scan events to all open browser tabs
	broker := api.NewBroker()

	// Create the HTTP server: serves the web UI, REST API, and SSE stream
	srv := api.NewServer(api.Config{
		Iface:     cfg.iface,
		Subnet:    cfg.subnet,
		Ports:     cfg.ports,
		TimeoutMs: cfg.timeoutMs,
		BaseURL:   os.Getenv("DING_BASE_URL"),
		Google: api.OAuthConfig{
			ClientID:     os.Getenv("DING_GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("DING_GOOGLE_CLIENT_SECRET"),
		},
		GitHub: api.OAuthConfig{
			ClientID:     os.Getenv("DING_GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("DING_GITHUB_CLIENT_SECRET"),
		},
	}, store, store, broker, scanFn)

	// Run one scan immediately so the UI has data as soon as you open it
	srv.TriggerScan()

	// If DING_SCAN_INTERVAL is set (e.g. "60s"), keep scanning automatically
	if cfg.scanInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.scanInterval) // fires every N seconds
			defer ticker.Stop()
			for range ticker.C {
				srv.TriggerScan() // each tick triggers a new scan
			}
		}()
	}

	// Start the HTTP server in the background (it blocks inside its goroutine)
	httpSrv := &http.Server{
		Addr:        cfg.httpAddr,
		Handler:     srv,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 120 * time.Second,
		// No WriteTimeout: the SSE endpoint (/api/events) streams indefinitely.
	}
	slog.Info("listening", "addr", cfg.httpAddr, "interface", cfg.iface, "subnet", cfg.subnet)

	// Listen for Ctrl+C (SIGINT) or docker stop (SIGTERM) so we can shut down cleanly
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start background cleanup goroutines (expired sessions, OAuth state tokens).
	srv.StartCleanup(ctx)

	// Prune old DB records daily: scans older than 90 days, change-log older than 30 days.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := store.PruneOldData(90*24*time.Hour, 30*24*time.Hour); err != nil {
					slog.Error("prune old data", "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start passive ARP listener — watches ARP traffic and instantly notifies
	// the UI when a device appears without waiting for the next scheduled scan.
	// Restarts automatically if the process dies (e.g. temporary interface error).
	go runPassiveListener(ctx, cfg.iface, store, broker)

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	// Block here until a shutdown signal arrives
	<-ctx.Done()
	slog.Info("shutting down")

	// Give in-flight requests up to 5 seconds to finish before we exit
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutCtx)
}

// runPassiveListener starts the scanner in passive ARP listen mode and pushes
// device_seen SSE events when a device appears that isn't in the last scan.
// It restarts the scanner process automatically if it exits unexpectedly.
func runPassiveListener(ctx context.Context, iface string, store storage.Store, broker *api.Broker) {
	const debounce = 2 * time.Minute
	lastReported := make(map[string]time.Time)

	for {
		// Exit cleanly when the app shuts down
		select {
		case <-ctx.Done():
			return
		default:
		}

		events, err := scanner.Listen(ctx, iface)
		if err != nil {
			slog.Warn("passive ARP listener failed, retrying in 5s", "err", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}

		for event := range events {
			// Debounce: don't re-report the same IP within the debounce window
			if time.Since(lastReported[event.IP]) < debounce {
				continue
			}
			// Skip devices already present in the most recent full scan —
			// they are already tracked and the UI knows about them
			if ipInLatestScan(store, event.IP) {
				continue
			}

			lastReported[event.IP] = time.Now()

			kind := diff.KindNew
			if store.AllKnownIPs()[event.IP] {
				kind = diff.KindBack
			}

			broker.Publish(api.DeviceSeenEvent(event.IP, event.MAC, kind))
		}

		// events channel closed — scanner process exited; retry after a pause
		slog.Warn("passive ARP listener exited, retrying in 5s")
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

// ipInLatestScan reports whether ip appears in the most recent full scan.
func ipInLatestScan(store storage.Store, ip string) bool {
	for _, r := range store.Latest() {
		if r.IP == ip {
			return true
		}
	}
	return false
}

// config holds all runtime settings for the application.
type config struct {
	iface        string        // network interface, e.g. "eth0"
	subnet       string        // CIDR range to scan, e.g. "192.168.1.0/24"
	ports        string        // comma-separated ports, e.g. "22,80,443"
	timeoutMs    int           // how long to wait per host (milliseconds)
	dataPath     string        // path to the SQLite database file
	httpAddr     string        // address for the web server, e.g. ":8081"
	scanInterval time.Duration // how often to auto-scan (0 = on-demand only)
	alert        alert.Config  // Telegram credentials
}

// configFromEnv reads environment variables and returns a populated config.
// If DING_INTERFACE or DING_SUBNET are not set, it auto-detects them.
func configFromEnv() (config, error) {
	cfg := config{
		ports:    envOr("DING_PORTS", "22,80,443,554,8000,8080,8443"),
		dataPath: envOr("DING_DATA_PATH", "/data/ding.db"),
		httpAddr: envOr("DING_HTTP_ADDR", ":8081"),
	}

	// Parse timeout — default 500ms if not set or invalid
	if raw := os.Getenv("DING_TIMEOUT_MS"); raw != "" {
		var n int
		if _, err := parseIntEnv(raw, &n); err == nil {
			cfg.timeoutMs = n
		}
	}
	if cfg.timeoutMs == 0 {
		cfg.timeoutMs = 500
	}

	// Parse scan interval — e.g. "60s", "5m", "1h"
	// If not set or invalid, scanInterval stays 0 (on-demand only)
	if raw := os.Getenv("DING_SCAN_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			slog.Warn("invalid DING_SCAN_INTERVAL, ignored", "value", raw, "err", err)
		} else {
			cfg.scanInterval = d
		}
	}

	// Telegram alert credentials (optional — leave empty to disable)
	cfg.alert = alert.Config{
		TelegramToken:  os.Getenv("DING_TELEGRAM_TOKEN"),
		TelegramChatID: os.Getenv("DING_TELEGRAM_CHAT_ID"),
	}

	// Read interface and subnet from env — may be empty (auto-detect below)
	cfg.iface = os.Getenv("DING_INTERFACE")
	cfg.subnet = os.Getenv("DING_SUBNET")

	// If either is missing, auto-detect from the available network interfaces
	if cfg.iface == "" || cfg.subnet == "" {
		detected, err := resolveInterface(cfg.iface)
		if err != nil {
			return config{}, err
		}
		if cfg.iface == "" {
			cfg.iface = detected.Name
		}
		if cfg.subnet == "" {
			cfg.subnet = detected.Subnet
		}
		slog.Info("auto-detected network interface", "interface", cfg.iface, "subnet", cfg.subnet)
	}

	return cfg, nil
}

// resolveInterface finds the right network interface to scan on.
// If `hint` is set, it looks for that specific interface by name.
// If `hint` is empty, it picks the first suitable interface automatically.
func resolveInterface(hint string) (iface.Interface, error) {
	all, err := iface.All()
	if err != nil {
		return iface.Interface{}, err
	}
	if len(all) == 0 {
		return iface.Interface{}, fmt.Errorf("no usable network interface found")
	}
	if hint != "" {
		// User specified an interface — find it or fail with a clear error
		for _, i := range all {
			if i.Name == hint {
				return i, nil
			}
		}
		return iface.Interface{}, fmt.Errorf("interface %q not found or has no IPv4 address", hint)
	}
	// No preference — use the first one (usually the main LAN interface)
	return all[0], nil
}

// envOr reads an environment variable, returning `fallback` if it's not set.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseIntEnv parses a string into an integer and writes it to `dst`.
func parseIntEnv(s string, dst *int) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err == nil {
		*dst = n
	}
	return n, err
}

// initLogger configures the global slog logger from DING_LOG_LEVEL and DING_LOG_FORMAT.
// DING_LOG_LEVEL: debug | info (default) | warn | error
// DING_LOG_FORMAT: text (default) | json
func initLogger() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("DING_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.ToLower(os.Getenv("DING_LOG_FORMAT")) == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}
