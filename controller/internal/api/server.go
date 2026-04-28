// ============================================================
// controller/internal/api/server.go — HTTP server setup
//
// This file wires everything together:
//   - Embeds the React UI files into the Go binary (go:embed)
//   - Registers all URL routes (which function handles which URL)
//   - Runs scans and publishes the results via SSE
// ============================================================

package api

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

// go:embed bundles the entire `static/` folder into the compiled binary.
// When you run `docker compose build`, the React app is built first and
// copied into static/ before Go compiles — so the UI travels inside the binary.
//
//go:embed static
var staticFiles embed.FS

// ScanFunc is the type of function that runs one network scan.
// It returns the list of found devices, what changed vs last scan, or an error.
type ScanFunc func() ([]scanner.Result, []diff.Change, error)

// Config holds the info the API layer needs to answer status requests
// and run targeted single-device port scans.
type Config struct {
	Iface     string // e.g. "wlp0s20f3"
	Subnet    string // e.g. "192.168.1.0/24"
	Ports     string // comma-separated, e.g. "22,80,443,554,8000,8080,8443"
	TimeoutMs int    // per-host TCP timeout in milliseconds
}

// sseEvent is the shape of every message we push to the browser.
// `omitempty` means empty fields are left out of the JSON.
type sseEvent struct {
	Type      string           `json:"type"`                 // "scan_start", "scan_result", "scan_error", "device_seen"
	Error     string           `json:"error,omitempty"`      // only set on scan_error
	Devices   []scanner.Result `json:"devices,omitempty"`    // only set on scan_result
	Changes   []diff.Change    `json:"changes,omitempty"`    // only set on scan_result
	ScannedAt string           `json:"scanned_at,omitempty"` // only set on scan_result
	IP        string           `json:"ip,omitempty"`         // only set on device_seen
	MAC       string           `json:"mac,omitempty"`        // only set on device_seen
	Kind      diff.ChangeKind  `json:"kind,omitempty"`       // only set on device_seen (NEW or BACK)
}

// Server is the main HTTP handler. It holds references to everything it needs.
type Server struct {
	cfg      Config         // interface and subnet (shown in UI header)
	store    storage.Store  // scan history (read & write)
	broker   *Broker        // delivers events to browser tabs
	scanFn   ScanFunc       // the actual scan logic (defined in main.go)
	scanning atomic.Bool    // prevents two scans from running at the same time
	mux      *http.ServeMux // URL router
}

// DeviceSeenEvent builds an sseEvent for a passively detected device.
// Called by the passive ARP listener goroutine in main.go.
func DeviceSeenEvent(ip, mac string, kind diff.ChangeKind) sseEvent {
	return sseEvent{Type: "device_seen", IP: ip, MAC: mac, Kind: kind}
}

// NewServer creates the server, registers all routes, and returns it.
func NewServer(cfg Config, store storage.Store, broker *Broker, scanFn ScanFunc) *Server {
	s := &Server{cfg: cfg, store: store, broker: broker, scanFn: scanFn}
	s.mux = http.NewServeMux()

	// API routes — these return JSON data
	s.mux.HandleFunc("GET /api/status", s.handleStatus)                   // interface, subnet, last scan time
	s.mux.HandleFunc("GET /api/devices", s.handleDevices)                 // current device list
	s.mux.HandleFunc("GET /api/devices/{ip}/history", s.handleDeviceHistory) // per-device scan history
	s.mux.HandleFunc("GET /api/history", s.handleHistory)                 // last 20 scan records
	s.mux.HandleFunc("GET /api/topology", s.handleTopology)               // network topology graph
	s.mux.HandleFunc("POST /api/scan", s.handleScan)                         // trigger a new scan
	s.mux.HandleFunc("POST /api/devices/{ip}/scan", s.handleDeviceScan)      // targeted single-device port scan
	s.mux.HandleFunc("PUT /api/devices/{ip}/label", s.handleSetLabel)        // set custom device name
	s.mux.HandleFunc("DELETE /api/devices/{ip}/label", s.handleDelLabel)     // remove custom device name
	s.mux.HandleFunc("GET /api/events", s.broker.serveSSE)                // SSE stream

	// Everything else (/, /assets/..., etc.) serves the React app
	sub, _ := fs.Sub(staticFiles, "static") // strip the "static/" prefix
	s.mux.Handle("/", spaHandler(sub))

	return s
}

// ServeHTTP makes *Server satisfy the http.Handler interface.
// Go's http.Server calls this for every incoming request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// TriggerScan starts a scan in the background if one isn't already running.
// Called on startup, by the timer, and by POST /api/scan.
func (s *Server) TriggerScan() {
	// CompareAndSwap: atomically set scanning=true only if it was false.
	// If it was already true, another scan is running — do nothing.
	if !s.scanning.CompareAndSwap(false, true) {
		return
	}
	go s.runScan() // run in a goroutine so we don't block the caller
}

// runScan executes the actual scan and broadcasts the result to all browser tabs.
func (s *Server) runScan() {
	defer s.scanning.Store(false) // mark scan as finished when this function exits

	// Tell all browsers the scan is starting (they can show a spinner)
	s.broker.Publish(sseEvent{Type: "scan_start"})

	// Run the full scan cycle (ARP + ICMP + TCP, save, diff, alert)
	_, changes, err := s.scanFn()
	if err != nil {
		// Something went wrong — tell the browsers
		s.broker.Publish(sseEvent{Type: "scan_error", Error: err.Error()})
		return
	}

	// Push the full registry (not just this scan's results) so the UI
	// always shows every known device, with alive=false for offline ones.
	s.broker.Publish(sseEvent{
		Type:      "scan_result",
		Devices:   s.store.AllDevices(),
		Changes:   changes,
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// spaHandler serves the React app's static files.
// For any URL that doesn't match a real file (e.g. /some/page),
// it returns index.html so the React app can handle routing itself.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// Try to find the file in the embedded filesystem
		_, err := fsys.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			// File not found → serve index.html (React will handle the route)
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		// File exists → serve it normally (JS, CSS, images, etc.)
		fileServer.ServeHTTP(w, r)
	})
}
