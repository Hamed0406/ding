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
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ding/ding/internal/alert"
	"github.com/ding/ding/internal/api"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/enrich"
	"github.com/ding/ding/internal/iface"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

func main() {
	// Load all settings from environment variables (with sensible defaults)
	cfg, err := configFromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Open (or create) the JSON file where we save scan history
	store, err := storage.New(cfg.dataPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	// scanFn is a reusable function that runs one complete scan cycle:
	//   Run Rust scanner → compare to last scan → save → send alerts
	// Both the timer and the "Scan Now" button call this same function.
	scanFn := func() ([]scanner.Result, []diff.Change, error) {
		// Call the Rust binary and get back a list of devices
		results, err := scanner.Run(cfg.iface, cfg.subnet, cfg.ports, cfg.timeoutMs)
		if err != nil {
			return nil, nil, err
		}

		// Resolve hostnames via reverse DNS (best-effort, runs in parallel).
		// Devices without a PTR record simply stay unnamed.
		enrich.Hostnames(results, 16, 300*time.Millisecond)

		// Load what we found last time so we can compare
		previous := store.Latest()

		// Figure out what changed: new devices, gone devices, port changes
		changes := diff.Compare(previous, results)

		// Save the new results to disk
		if err := store.Save(results); err != nil {
			log.Printf("save: %v", err)
		}

		// Send Telegram alert if a token is configured and something changed
		if err := alert.Send(cfg.alert, changes); err != nil {
			log.Printf("alert: %v", err)
		}

		return results, changes, nil
	}

	// The broker is the "messenger" — it pushes scan events to all open browser tabs
	broker := api.NewBroker()

	// Create the HTTP server: serves the web UI, REST API, and SSE stream
	srv := api.NewServer(api.Config{Iface: cfg.iface, Subnet: cfg.subnet}, store, broker, scanFn)

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
	httpSrv := &http.Server{Addr: cfg.httpAddr, Handler: srv}
	log.Printf("listening on %s  (interface=%s  subnet=%s)", cfg.httpAddr, cfg.iface, cfg.subnet)

	// Listen for Ctrl+C (SIGINT) or docker stop (SIGTERM) so we can shut down cleanly
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Block here until a shutdown signal arrives
	<-ctx.Done()
	log.Println("shutting down…")

	// Give in-flight requests up to 5 seconds to finish before we exit
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutCtx)
}

// config holds all runtime settings for the application.
type config struct {
	iface        string        // network interface, e.g. "eth0"
	subnet       string        // CIDR range to scan, e.g. "192.168.1.0/24"
	ports        string        // comma-separated ports, e.g. "22,80,443"
	timeoutMs    int           // how long to wait per host (milliseconds)
	dataPath     string        // where to save scan history JSON file
	httpAddr     string        // address for the web server, e.g. ":8081"
	scanInterval time.Duration // how often to auto-scan (0 = on-demand only)
	alert        alert.Config  // Telegram credentials
}

// configFromEnv reads environment variables and returns a populated config.
// If DING_INTERFACE or DING_SUBNET are not set, it auto-detects them.
func configFromEnv() (config, error) {
	cfg := config{
		ports:    envOr("DING_PORTS", "22,80,443,8080,8443"),
		dataPath: envOr("DING_DATA_PATH", "/data/ding.json"),
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
			log.Printf("invalid DING_SCAN_INTERVAL %q: %v (ignored)", raw, err)
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
		log.Printf("auto-detected interface=%s subnet=%s", cfg.iface, cfg.subnet)
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
